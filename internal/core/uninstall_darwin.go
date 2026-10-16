//go:build darwin

package core

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"howett.net/plist"
)

type bundleMetadata struct {
	Name       string `plist:"CFBundleDisplayName"`
	BundleName string `plist:"CFBundleName"`
	Identifier string `plist:"CFBundleIdentifier"`
	Version    string `plist:"CFBundleShortVersionString"`
	IconFile   string `plist:"CFBundleIconFile"`
	IconName   string `plist:"CFBundleIconName"`
}

var (
	applicationServicesOnce sync.Once
	applicationServicesErr  error
)

// FullDiskAccessError 表示目标受 macOS 隐私保护，需要用户授予完全磁盘访问权限。
type FullDiskAccessError struct {
	Path string
}

func (e *FullDiskAccessError) Error() string {
	return fmt.Sprintf(
		"cannot remove macOS-protected data:\n%s\n\ngrant Full Disk Access to Vitals and retry. the app bundle has not been removed",
		e.Path,
	)
}

// ListApplications 返回用户可管理的应用，不包含 macOS 系统应用。
// locales 是按优先级排列的 .lproj 目录名，用于读取本地化显示名。
// 应用体积与图标不在此处读取，需通过 BundleSize 与 ApplicationIconPNG 按需获取。
// ctx 超时或取消时不报错，返回已扫描到的部分结果；调用方可通过 ctx.Err() 判断是否截断。
func ListApplications(ctx context.Context, locales []string) ([]AppInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	roots := []string{"/Applications", filepath.Join(home, "Applications")}
	running := runningExecutables(ctx)
	var apps []AppInfo
	for _, root := range roots {
		found, walkErr := applicationsIn(root, 2)
		if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
			return nil, walkErr
		}
		for _, path := range found {
			if ctx.Err() != nil {
				return sortApplications(apps), nil
			}
			meta, readErr := readBundleMetadata(path)
			if readErr != nil {
				continue
			}
			apps = append(apps, AppInfo{
				Name:     localizedBundleName(path, meta, locales),
				Path:     path,
				BundleID: meta.Identifier,
				Version:  meta.Version,
				Running:  anyExecutableInBundle(running, path),
				Selected: true,
			})
		}
	}
	return sortApplications(apps), nil
}

func sortApplications(apps []AppInfo) []AppInfo {
	slices.SortFunc(apps, func(a, b AppInfo) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return apps
}

// BundleSize 统计应用包占用的磁盘空间。目录较大时耗时较长，应在后台调用。
func BundleSize(appPath string) uint64 {
	return pathSize(appPath)
}

// runningExecutables 一次性读取所有进程的可执行文件路径，一次 ps 即可判断全部应用是否运行。
func runningExecutables(ctx context.Context) []string {
	output, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "comm=").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(string(output), "\n")
	executables := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			executables = append(executables, line)
		}
	}
	return executables
}

// anyExecutableInBundle 判断应用主程序目录下是否有进程在运行。
func anyExecutableInBundle(executables []string, appPath string) bool {
	prefix := appPath + "/Contents/MacOS/"
	for _, executable := range executables {
		if strings.HasPrefix(executable, prefix) {
			return true
		}
	}
	return false
}

// localizedBundleName 按 locales 顺序读取 <lproj>/InfoPlist.strings 中的显示名，
// 没有本地化时回退到 Info.plist 的 CFBundleDisplayName / CFBundleName / 目录名。
func localizedBundleName(appPath string, meta bundleMetadata, locales []string) string {
	resources := filepath.Join(appPath, "Contents", "Resources")
	for _, locale := range locales {
		localized, err := readBundlePlist(filepath.Join(resources, locale+".lproj", "InfoPlist.strings"))
		if err != nil {
			continue
		}
		if name := firstNonEmpty(localized.Name, localized.BundleName); name != "" {
			return name
		}
	}
	return firstNonEmpty(meta.Name, meta.BundleName, strings.TrimSuffix(filepath.Base(appPath), ".app"))
}

// ApplicationIconPNG 把应用图标转为 128x128 PNG。依赖 sips 子进程，应在后台调用；失败返回 nil。
func ApplicationIconPNG(ctx context.Context, appPath string) []byte {
	meta, err := readBundleMetadata(appPath)
	if err != nil {
		return nil
	}
	resources := filepath.Join(appPath, "Contents", "Resources")
	var candidates []string
	for _, name := range []string{meta.IconFile, meta.IconName} {
		if name == "" {
			continue
		}
		if filepath.Ext(name) == "" {
			name += ".icns"
		}
		candidates = append(candidates, filepath.Join(resources, name))
	}
	if matches, _ := filepath.Glob(filepath.Join(resources, "*.icns")); len(matches) > 0 {
		candidates = append(candidates, matches...)
	}
	iconPath := ""
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			iconPath = candidate
			break
		}
	}
	if iconPath == "" {
		return nil
	}
	temp, err := os.CreateTemp("", "vitals-app-icon-*.png")
	if err != nil {
		return nil
	}
	outputPath := temp.Name()
	_ = temp.Close()
	_ = os.Remove(outputPath)
	defer os.Remove(outputPath)
	command := exec.CommandContext(ctx, "/usr/bin/sips", "-s", "format", "png", "-z", "128", "128", iconPath, "--out", outputPath)
	if command.Run() != nil {
		return nil
	}
	data, _ := os.ReadFile(outputPath)
	return data
}

func applicationsIn(root string, depth int) ([]string, error) {
	var apps []string
	var visit func(string, int) error
	visit = func(dir string, level int) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if strings.HasSuffix(strings.ToLower(entry.Name()), ".app") {
				apps = append(apps, path)
				continue
			}
			if level < depth {
				_ = visit(path, level+1)
			}
		}
		return nil
	}
	return apps, visit(root, 0)
}

func readBundleMetadata(appPath string) (bundleMetadata, error) {
	return readBundlePlist(filepath.Join(appPath, "Contents", "Info.plist"))
}

// readBundlePlist 解析 Info.plist 或 InfoPlist.strings 中与 bundleMetadata 同名的键。
func readBundlePlist(path string) (bundleMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return bundleMetadata{}, err
	}
	var meta bundleMetadata
	if _, err := plist.Unmarshal(data, &meta); err != nil {
		return bundleMetadata{}, err
	}
	return meta, nil
}

// ApplicationRunning 判断应用包当前是否仍有主程序或辅助进程。
func ApplicationRunning(ctx context.Context, appPath string) bool {
	return anyExecutableInBundle(runningExecutables(ctx), appPath)
}

// ScanRelatedFiles 根据 bundle id 与应用名扫描关联文件，并标注命中置信度。
func ScanRelatedFiles(ctx context.Context, app AppInfo) ([]RelatedFile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	type scanRoot struct {
		path  string
		depth int
	}
	roots := []scanRoot{
		{filepath.Join(home, "Library", "Application Support"), 1},
		{filepath.Join(home, "Library", "Application Scripts"), 0},
		{filepath.Join(home, "Library", "Caches"), 1},
		{filepath.Join(home, "Library", "Cookies"), 0},
		{filepath.Join(home, "Library", "HTTPStorages"), 0},
		{filepath.Join(home, "Library", "Logs"), 1},
		{filepath.Join(home, "Library", "Preferences"), 1},
		{filepath.Join(home, "Library", "Saved Application State"), 0},
		{filepath.Join(home, "Library", "WebKit"), 0},
		{filepath.Join(home, "Library", "Containers"), 0},
		{filepath.Join(home, "Library", "Group Containers"), 0},
		{filepath.Join(home, "Library", "LaunchAgents"), 0},
		{"/Library/Application Support", 1},
		{"/Library/LaunchAgents", 0},
		{"/Library/LaunchDaemons", 0},
		{"/Library/PrivilegedHelperTools", 0},
	}
	sharedVendor := hasSiblingVendorApplication(ctx, app)
	var files []RelatedFile
	seen := make(map[string]bool)
	for _, root := range roots {
		found, scanErr := scanRelatedRoot(ctx, root.path, root.depth, app, sharedVendor, seen)
		if scanErr != nil {
			return nil, scanErr
		}
		files = append(files, found...)
	}
	slices.SortFunc(files, func(a, b RelatedFile) int {
		if a.Confidence != b.Confidence {
			return strings.Compare(a.Confidence, b.Confidence)
		}
		if cleanupPriority(a.Path) != cleanupPriority(b.Path) {
			return cleanupPriority(a.Path) - cleanupPriority(b.Path)
		}
		return strings.Compare(a.Path, b.Path)
	})
	return files, nil
}

func scanRelatedRoot(
	ctx context.Context,
	root string,
	depth int,
	app AppInfo,
	sharedVendor bool,
	seen map[string]bool,
) ([]RelatedFile, error) {
	var files []RelatedFile
	var visit func(string, int) error
	visit = func(dir string, level int) error {
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			path := filepath.Join(dir, entry.Name())
			confidence, matched := relatedConfidence(entry.Name(), app)
			if !matched {
				confidence, matched = launchItemConfidence(path, app)
			}
			if !matched {
				confidence, matched = updaterConfidence(path, app, sharedVendor)
			}
			if matched {
				if !seen[path] {
					seen[path] = true
					files = append(files, RelatedFile{
						Path:       path,
						Size:       pathSize(path),
						Kind:       filepath.Base(root),
						Confidence: confidence,
						Selected:   confidence == confidenceHigh,
					})
				}
				continue
			}
			if entry.IsDir() && level < depth {
				if err := visit(path, level+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return files, visit(root, 0)
}

const (
	confidenceHigh   = "high"
	confidenceMedium = "medium"
)

func relatedConfidence(filename string, app AppInfo) (string, bool) {
	value := strings.ToLower(filename)
	bundleID := strings.ToLower(strings.TrimSpace(app.BundleID))
	if bundleID != "" && (value == bundleID || strings.HasPrefix(value, bundleID+".") || strings.HasPrefix(value, bundleID+"-")) {
		return confidenceHigh, true
	}
	if leaf := bundleLeaf(bundleID); len(leaf) >= 4 && nameMatches(value, leaf) {
		return confidenceHigh, true
	}
	name := strings.ToLower(strings.TrimSpace(app.Name))
	if len(name) >= 4 && nameMatches(value, name) {
		return confidenceMedium, true
	}
	return "", false
}

func nameMatches(value, name string) bool {
	return value == name ||
		strings.HasPrefix(value, name+".") ||
		strings.HasPrefix(value, name+" ") ||
		strings.HasPrefix(value, name+"-")
}

func bundleLeaf(bundleID string) string {
	if index := strings.LastIndex(bundleID, "."); index >= 0 {
		return bundleID[index+1:]
	}
	return bundleID
}

func vendorNamespace(bundleID string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(bundleID)), ".")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "." + parts[1]
}

func updaterConfidence(path string, app AppInfo, sharedVendor bool) (string, bool) {
	namespace := vendorNamespace(app.BundleID)
	if namespace == "" {
		return "", false
	}
	vendor := strings.TrimPrefix(namespace, strings.Split(namespace, ".")[0]+".")
	compactVendor := compactIdentity(vendor)
	vendorMatched := false
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if strings.Contains(compactIdentity(part), compactVendor) {
			vendorMatched = true
			break
		}
	}
	if !vendorMatched {
		return "", false
	}
	name := compactIdentity(filepath.Base(path))
	for _, keyword := range []string{"update", "updater", "keystone"} {
		if strings.Contains(name, keyword) {
			if sharedVendor {
				return confidenceMedium, true
			}
			return confidenceHigh, true
		}
	}
	return "", false
}

func compactIdentity(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, value)
}

func launchItemConfidence(path string, app AppInfo) (string, bool) {
	if !strings.EqualFold(filepath.Ext(path), ".plist") ||
		!strings.Contains(path, string(filepath.Separator)+"Launch") {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var config launchdConfig
	if _, err := plist.Unmarshal(data, &config); err != nil {
		return "", false
	}
	bundleID := strings.ToLower(strings.TrimSpace(app.BundleID))
	for _, associated := range config.AssociatedBundleIDs {
		value := strings.ToLower(strings.TrimSpace(associated))
		if bundleID != "" && (value == bundleID || strings.HasPrefix(value, bundleID+".")) {
			return confidenceHigh, true
		}
	}
	appPath := filepath.Clean(app.Path)
	for _, value := range append([]string{config.Program}, config.ProgramArguments...) {
		clean := filepath.Clean(value)
		if app.Path != "" && (clean == appPath || strings.HasPrefix(clean, appPath+string(filepath.Separator))) {
			return confidenceHigh, true
		}
	}
	return "", false
}

func hasSiblingVendorApplication(ctx context.Context, app AppInfo) bool {
	namespace := vendorNamespace(app.BundleID)
	if namespace == "" {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	for _, root := range []string{"/Applications", filepath.Join(home, "Applications")} {
		paths, _ := applicationsIn(root, 2)
		for _, path := range paths {
			if ctx.Err() != nil {
				return true
			}
			if filepath.Clean(path) == filepath.Clean(app.Path) {
				continue
			}
			meta, err := readBundleMetadata(path)
			if err == nil && strings.HasPrefix(strings.ToLower(meta.Identifier), namespace+".") {
				return true
			}
		}
	}
	return false
}

func cleanupPriority(path string) int {
	if strings.Contains(path, string(filepath.Separator)+"LaunchAgents"+string(filepath.Separator)) ||
		strings.Contains(path, string(filepath.Separator)+"LaunchDaemons"+string(filepath.Separator)) {
		return 0
	}
	return 1
}

// QuitApplication 请求目标应用正常退出，不强制终止进程。
func QuitApplication(ctx context.Context, app AppInfo) error {
	bundleID := strings.TrimSpace(app.BundleID)
	if bundleID == "" {
		return fmt.Errorf("app is missing bundle id, cannot quit safely")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	applicationServicesOnce.Do(func() {
		for _, framework := range []string{
			"/System/Library/Frameworks/Foundation.framework/Foundation",
			"/System/Library/Frameworks/AppKit.framework/AppKit",
		} {
			if _, err := purego.Dlopen(framework, purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
				applicationServicesErr = err
				return
			}
		}
	})
	if applicationServicesErr != nil {
		return fmt.Errorf("quit %s: load macOS application services: %w", app.Name, applicationServicesErr)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	poolClass := objc.ID(objc.GetClass("NSAutoreleasePool"))
	runningClass := objc.ID(objc.GetClass("NSRunningApplication"))
	stringClass := objc.ID(objc.GetClass("NSString"))
	if poolClass == 0 || runningClass == 0 || stringClass == 0 {
		return fmt.Errorf("quit %s: macOS application services are unavailable", app.Name)
	}
	pool := poolClass.Send(objc.RegisterName("alloc")).Send(objc.RegisterName("init"))
	defer pool.Send(objc.RegisterName("drain"))

	identifier := stringClass.Send(objc.RegisterName("stringWithUTF8String:"), bundleID)
	running := runningClass.Send(objc.RegisterName("runningApplicationsWithBundleIdentifier:"), identifier)
	count := objc.Send[uint64](running, objc.RegisterName("count"))
	for index := uint64(0); index < count; index++ {
		process := running.Send(objc.RegisterName("objectAtIndex:"), index)
		if !objc.Send[bool](process, objc.RegisterName("terminate")) {
			return fmt.Errorf("quit %s: macOS rejected the quit request", app.Name)
		}
	}
	return nil
}

// MoveToTrash 按给定顺序将路径移入废纸篓，遇到失败立即停止。
// 调用方应将应用本体放在最后，确保关联文件失败时应用仍可用于重试。
func MoveToTrash(ctx context.Context, paths []string, trash func(string) error) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == "" || path == "/" || path == home {
			return fmt.Errorf("refusing to process unsafe path %q", path)
		}
		if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		unloadLaunchItem(ctx, path, home)
		if err := trash(path); err != nil {
			if protectedLibraryPath(path, home) {
				return &FullDiskAccessError{Path: path}
			}
			return fmt.Errorf("cannot move the following item to Trash:\n%s\n\n%s", path, err)
		}
	}
	return nil
}

func unloadLaunchItem(ctx context.Context, path, home string) {
	clean := filepath.Clean(path)
	var domain string
	switch {
	case strings.HasPrefix(clean, filepath.Join(home, "Library", "LaunchAgents")+string(filepath.Separator)),
		strings.HasPrefix(clean, "/Library/LaunchAgents"+string(filepath.Separator)):
		domain = fmt.Sprintf("gui/%d", os.Getuid())
	case strings.HasPrefix(clean, "/Library/LaunchDaemons"+string(filepath.Separator)):
		domain = "system"
	default:
		return
	}
	if !strings.EqualFold(filepath.Ext(clean), ".plist") {
		return
	}
	// bootout 同时停止已加载的更新器；未加载或无权停止时仍继续移走配置文件。
	_ = exec.CommandContext(ctx, "/bin/launchctl", "bootout", domain, clean).Run()
}

func protectedLibraryPath(path, home string) bool {
	protected := []string{
		filepath.Join(home, "Library"),
		"/Library/LaunchDaemons",
		"/Library/PrivilegedHelperTools",
	}
	clean := filepath.Clean(path)
	for _, root := range protected {
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func pathSize(path string) uint64 {
	var total uint64
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			if info, statErr := entry.Info(); statErr == nil && info.Size() > 0 {
				total += uint64(info.Size())
			}
		}
		return nil
	})
	return total
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
