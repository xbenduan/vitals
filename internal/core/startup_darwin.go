//go:build darwin

package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"howett.net/plist"
)

// StartupItem 描述一个由 macOS 在登录、启动或后台条件满足时加载的项目。
type StartupItem struct {
	ID           string
	Name         string
	Label        string
	Kind         string
	Scope        string
	SourcePath   string
	Executable   string
	AppPath      string
	BundleID     string
	Developer    string
	TeamID       string
	Trigger      string
	Enabled      bool
	Running      bool
	NeedsReview  bool
	ReviewReason string
}

type launchdConfig struct {
	Label                 string         `plist:"Label"`
	AssociatedBundleIDs   []string       `plist:"AssociatedBundleIdentifiers"`
	Program               string         `plist:"Program"`
	ProgramArguments      []string       `plist:"ProgramArguments"`
	RunAtLoad             bool           `plist:"RunAtLoad"`
	KeepAlive             any            `plist:"KeepAlive"`
	StartInterval         int            `plist:"StartInterval"`
	StartCalendarInterval any            `plist:"StartCalendarInterval"`
	WatchPaths            []string       `plist:"WatchPaths"`
	QueueDirectories      []string       `plist:"QueueDirectories"`
	MachServices          map[string]any `plist:"MachServices"`
	Disabled              bool           `plist:"Disabled"`
}

type startupRoot struct {
	Path  string
	Kind  string
	Scope string
}

// ScanStartupItems 只读扫描当前用户和系统可直接访问的第三方 launchd 配置。
func ScanStartupItems(ctx context.Context) ([]StartupItem, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	uid := os.Getuid()
	userDomain := fmt.Sprintf("gui/%d", uid)
	userDisabled := launchctlDisabled(ctx, userDomain)
	systemDisabled := launchctlDisabled(ctx, "system")
	roots := []startupRoot{
		{filepath.Join(home, "Library", "LaunchAgents"), "用户代理", "当前用户"},
		{"/Library/LaunchAgents", "登录代理", "所有用户"},
		{"/Library/LaunchDaemons", "系统守护进程", "系统"},
	}
	items, err := scanLaunchPlists(ctx, roots, userDisabled, systemDisabled)
	if err != nil {
		return nil, err
	}

	for index := range items {
		enrichStartupItem(ctx, &items[index])
	}
	slices.SortFunc(items, compareStartupItems)
	return items, nil
}

func scanLaunchPlists(
	ctx context.Context,
	roots []startupRoot,
	userDisabled map[string]bool,
	systemDisabled map[string]bool,
) ([]StartupItem, error) {
	var items []StartupItem
	for _, root := range roots {
		entries, err := os.ReadDir(root.Path)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read startup directory %s: %w", root.Path, err)
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".plist") {
				continue
			}
			path := filepath.Join(root.Path, entry.Name())
			item, err := startupItemFromPlist(path, root)
			if err != nil {
				continue
			}
			disabled := systemDisabled
			domain := "system"
			if root.Kind != "系统守护进程" {
				disabled = userDisabled
				domain = fmt.Sprintf("gui/%d", os.Getuid())
			}
			item.Enabled = item.Enabled && !disabled[item.Label]
			item.Running = launchctlLoaded(ctx, domain, item.Label)
			items = append(items, item)
		}
	}
	return items, nil
}

func startupItemFromPlist(path string, root startupRoot) (StartupItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return StartupItem{}, err
	}
	var config launchdConfig
	if _, err := plist.Unmarshal(data, &config); err != nil {
		return StartupItem{}, err
	}
	if config.Label == "" {
		config.Label = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	executable := config.Program
	if executable == "" && len(config.ProgramArguments) > 0 {
		executable = config.ProgramArguments[0]
	}
	item := StartupItem{
		ID:         "launchd:" + path,
		Name:       displayStartupName(config.Label),
		Label:      config.Label,
		Kind:       root.Kind,
		Scope:      root.Scope,
		SourcePath: path,
		Executable: executable,
		AppPath:    applicationPathFromArguments(config.ProgramArguments, executable),
		Trigger:    launchdTrigger(config),
		Enabled:    !config.Disabled,
	}
	return item, nil
}

func launchdTrigger(config launchdConfig) string {
	switch {
	case keepAliveEnabled(config.KeepAlive):
		return "持续运行"
	case config.StartInterval > 0 || config.StartCalendarInterval != nil:
		return "定时运行"
	case len(config.WatchPaths) > 0 || len(config.QueueDirectories) > 0:
		return "文件变化"
	case config.RunAtLoad:
		return "登录时"
	case len(config.MachServices) > 0:
		return "按需运行"
	default:
		return "条件触发"
	}
}

func keepAliveEnabled(value any) bool {
	switch value := value.(type) {
	case bool:
		return value
	case map[string]any:
		return len(value) > 0
	case map[any]any:
		return len(value) > 0
	default:
		return false
	}
}

func applicationPathFromArguments(arguments []string, executable string) string {
	for _, value := range append([]string{executable}, arguments...) {
		if path := rootApplicationPath(value); path != "" {
			return path
		}
	}
	return ""
}

func launchctlDisabled(ctx context.Context, domain string) map[string]bool {
	output, err := exec.CommandContext(ctx, "/bin/launchctl", "print-disabled", domain).CombinedOutput()
	if err != nil {
		return nil
	}
	disabled := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, `"`) || !strings.Contains(line, "=>") {
			continue
		}
		parts := strings.SplitN(line, "=>", 2)
		label := strings.Trim(strings.TrimSpace(parts[0]), `"`)
		value := strings.TrimSpace(parts[1])
		disabled[label] = strings.HasPrefix(value, "disabled")
	}
	return disabled
}

func launchctlLoaded(ctx context.Context, domain, label string) bool {
	if label == "" || strings.Contains(label, "/") {
		return false
	}
	return exec.CommandContext(ctx, "/bin/launchctl", "print", domain+"/"+label).Run() == nil
}

func enrichStartupItem(ctx context.Context, item *StartupItem) {
	if item.AppPath == "" {
		item.AppPath = rootApplicationPath(item.Executable)
	}
	if item.AppPath != "" {
		if meta, err := readBundleMetadata(item.AppPath); err == nil {
			item.Name = firstNonEmpty(meta.Name, meta.BundleName, item.Name)
			item.BundleID = firstNonEmpty(item.BundleID, meta.Identifier)
		}
	}
	signPath := item.Executable
	if signPath == "" {
		signPath = item.AppPath
	}
	if developer, teamID, identifier, ok := codeSigningInfo(ctx, signPath); ok {
		item.Developer = firstNonEmpty(item.Developer, developer)
		item.TeamID = firstNonEmpty(item.TeamID, teamID)
		item.BundleID = firstNonEmpty(item.BundleID, identifier)
	}
	item.Name = firstNonEmpty(item.Name, displayStartupName(item.Label), filepath.Base(signPath))
	item.NeedsReview, item.ReviewReason = startupReview(*item)
}

func codeSigningInfo(ctx context.Context, path string) (developer, teamID, identifier string, ok bool) {
	if path == "" {
		return "", "", "", false
	}
	output, err := exec.CommandContext(ctx, "/usr/bin/codesign", "-dvvv", path).CombinedOutput()
	if err != nil {
		return "", "", "", false
	}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		switch key {
		case "Authority":
			if developer == "" {
				developer = value
			}
		case "TeamIdentifier":
			teamID = value
		case "Identifier":
			identifier = value
		}
	}
	return developer, teamID, identifier, true
}

func startupReview(item StartupItem) (bool, string) {
	if item.Executable != "" && filepath.IsAbs(item.Executable) {
		if _, err := os.Stat(item.Executable); errors.Is(err, fs.ErrNotExist) {
			return true, "executable no longer exists"
		}
	}
	switch strings.ToLower(filepath.Base(item.Executable)) {
	case "sh", "bash", "zsh", "python", "python3", "osascript":
		return true, "launched through a script interpreter"
	}
	if item.Executable != "" && item.Developer == "" && item.TeamID == "" {
		return true, "no code signature recognized"
	}
	return false, ""
}

func displayStartupName(label string) string {
	parts := strings.Split(strings.TrimSpace(label), ".")
	for len(parts) > 1 {
		last := strings.ToLower(parts[len(parts)-1])
		switch last {
		case "agent", "daemon", "helper", "launcher", "service", "loginitem", "updater":
			parts = parts[:len(parts)-1]
		default:
			return parts[len(parts)-1]
		}
	}
	return strings.TrimSpace(strings.Join(parts, "."))
}

func compareStartupItems(a, b StartupItem) int {
	if a.NeedsReview != b.NeedsReview {
		if a.NeedsReview {
			return -1
		}
		return 1
	}
	if a.Enabled != b.Enabled {
		if a.Enabled {
			return -1
		}
		return 1
	}
	return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
}
