package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"vitals/internal/core"
	"vitals/internal/i18n"
)

type application struct {
	win          *mygo.Window
	tray         *mygo.Tray
	quitting     bool
	showOnLaunch bool

	sampler      *core.Sampler
	snapshot     core.Snapshot
	page         int
	settings     appSettings
	settingsPath string
	sampleEvery  atomic.Int64
	localizer    i18n.Localizer

	pendingMemory core.MemoryConsumer
	confirmMemory bool
	releasingApp  string
	memoryResult  string
	actionError   string
	errorOpen     bool
	errorSettings bool

	apps             []core.AppInfo
	appIcons         map[string]*ui.Bitmap
	iconRequested    map[string]bool
	iconSlots        chan struct{}
	appQuery         string
	selectedApp      int
	loadingApps      bool
	scanningRelated  bool
	uninstalling     bool
	quittingApp      bool
	confirmUninstall bool

	startupItems   []core.StartupItem
	startupLoaded  bool
	startupLoading bool
	startupError   string

	health              healthMonitor
	testingNotification bool
	notificationResult  string
}

const (
	panelWidth  = 480
	panelHeight = 620
	iconWorkers = 4
	iconTimeout = 10 * time.Second
)

func newApplication() *application {
	app := &application{
		sampler:       core.NewSampler(),
		selectedApp:   -1,
		loadingApps:   true,
		appIcons:      make(map[string]*ui.Bitmap),
		iconRequested: make(map[string]bool),
		iconSlots:     make(chan struct{}, iconWorkers),
		settings:      defaultSettings(),
		showOnLaunch:  launchWindowVisible(),
	}
	app.localizer = i18n.New(i18n.Lang(app.settings.Language))
	app.sampleEvery.Store(int64(app.settings.refreshInterval()))
	return app
}

// tr 返回当前语言下 key 对应的文案。
func (a *application) tr(key string) string {
	return a.localizer.T(key)
}

// trf 返回格式化后的文案。
func (a *application) trf(key string, args ...any) string {
	return a.localizer.Tf(key, args...)
}

// setLanguage 切换界面语言并持久化设置。
func (a *application) setLanguage(lang string) {
	a.settings.Language = lang
	a.localizer = a.localizer.SetLang(i18n.Lang(lang))
	a.persistSettings()
	if a.tray != nil {
		a.tray.SetToolTip(menuBarToolTip(a.snapshot, a.settings, a.localizer))
	}
}

func (a *application) ready() {
	if dir, err := mygo.App.Path(mygo.PathUserData); err == nil {
		a.settingsPath = filepath.Join(dir, "settings.json")
		if settings, loadErr := loadSettings(a.settingsPath); loadErr == nil {
			a.settings = settings
		} else {
			log.Println(loadErr)
		}
	}
	a.localizer = i18n.New(i18n.Lang(a.settings.Language))
	a.settings.OpenAtLogin = mygo.App.OpenAtLogin()
	a.sampleEvery.Store(int64(a.settings.refreshInterval()))
	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{
		Icon:           menuBarStatusImage(core.Snapshot{}, a.settings),
		IconIsTemplate: true,
		ToolTip:        a.tr("menu.tooltip.reading"),
	})
	if err != nil {
		log.Fatal("create menu bar icon: ", err)
	}
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "Vitals",
		Width:             panelWidth,
		Height:            panelHeight,
		Hidden:            true,
		Frameless:         true,
		Transparent:       true,
		Vibrancy:          mygo.VibrancyPopover,
		DisableResize:     true,
		DisableMove:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		Content:           ui.View(a.view),
	})
	a.win.OnClose(a.handleWindowClose)
	a.win.OnBlur(a.win.Hide)
	a.tray.OnClick(a.toggleWindow)
	a.tray.OnRightClick(a.openTrayMenu)
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.tray.Destroy() })
	if a.showOnLaunch {
		go func() {
			time.Sleep(500 * time.Millisecond)
			a.win.Center()
			a.win.Show()
			a.win.Focus()
		}()
	}

	go a.sampleLoop()
	go a.loadApplications()
	a.refreshStartupItems()
}

func launchWindowVisible() bool {
	for _, argument := range os.Args[1:] {
		if argument == "--show" {
			return true
		}
	}
	return false
}

func (a *application) handleWindowClose(event *mygo.CloseEvent) {
	if a.quitting {
		return
	}
	event.PreventDefault()
	if a.win != nil {
		a.win.Hide()
	}
}

func (a *application) loadStartupItems() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	items, err := core.ScanStartupItems(ctx)
	a.win.Update(func() {
		a.startupLoading = false
		a.startupLoaded = true
		if err != nil {
			a.startupError = a.tr("error.startup_scan")
			log.Println("scan startup items:", err)
			return
		}
		a.startupError = ""
		a.startupItems = items
	})
}

func (a *application) refreshStartupItems() {
	if a.startupLoading {
		return
	}
	a.startupLoading = true
	go a.loadStartupItems()
}

func (a *application) sampleLoop() {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		snapshot, _ := a.sampler.Sample(ctx)
		cancel()
		a.win.Update(func() {
			a.snapshot = snapshot
			_ = a.tray.SetIcon(menuBarStatusImage(snapshot, a.settings), true)
			a.tray.SetToolTip(menuBarToolTip(snapshot, a.settings, a.localizer))
			if a.settings.Alerts {
				for _, alert := range a.health.Observe(snapshot, time.Now(), a.localizer) {
					go a.showHealthAlert(alert)
				}
			}
		})
		timer := time.NewTimer(time.Duration(a.sampleEvery.Load()))
		<-timer.C
	}
}

func (a *application) loadApplications() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	apps, err := core.ListApplications(ctx, a.localizer.Lang().BundleLocales())
	if ctx.Err() != nil {
		log.Println("list applications: timed out, showing partial results")
	}
	a.win.Update(func() {
		a.loadingApps = false
		a.apps = apps
		if err != nil {
			a.showActionError(err)
		}
	})
}

// requestAppIcon 在首次渲染某个应用时异步转换其图标，期间界面显示占位头像。
// 每个路径只请求一次，失败不重试。只能在 UI 线程调用：iconRequested 与 appIcons 都只在 build 回调和 win.Update 内读写。
func (a *application) requestAppIcon(appPath string) {
	if appPath == "" || a.iconRequested[appPath] {
		return
	}
	a.iconRequested[appPath] = true
	go func() {
		a.iconSlots <- struct{}{}
		defer func() { <-a.iconSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), iconTimeout)
		defer cancel()
		icon, err := ui.DecodeBitmap(core.ApplicationIconPNG(ctx, appPath))
		if err != nil {
			return
		}
		a.win.Update(func() { a.appIcons[appPath] = icon })
	}()
}

func (a *application) selectApplication(index int) {
	if a.uninstalling || index < 0 || index >= len(a.apps) {
		return
	}
	a.selectedApp = index
	if !a.startupLoaded && !a.startupLoading && a.win != nil {
		a.refreshStartupItems()
	}
	if a.apps[index].Scanned || a.scanningRelated {
		return
	}
	a.scanningRelated = true
	app := a.apps[index]
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		sizeDone := make(chan uint64, 1)
		go func() { sizeDone <- core.BundleSize(app.Path) }()
		files, err := core.ScanRelatedFiles(ctx, app)
		size := <-sizeDone
		a.win.Update(func() {
			a.scanningRelated = false
			if index >= len(a.apps) || a.apps[index].Path != app.Path {
				return
			}
			a.apps[index].Scanned = true
			a.apps[index].Size = size
			a.apps[index].Related = files
			a.apps[index].TotalBytes = a.apps[index].Size
			for _, file := range files {
				if file.Selected {
					a.apps[index].TotalBytes += file.Size
				}
			}
			if err != nil {
				a.apps[index].ScanError = err.Error()
			}
		})
	}()
}

func (a *application) showHealthAlert(alert healthAlert) {
	if !mygo.NotificationsSupported() {
		return
	}
	notification := mygo.NewNotification(mygo.NotificationOptions{
		ID:    alert.ID,
		Group: "health",
		Title: alert.Title,
		Body:  alert.Message,
	})
	if err := notification.Show(); err != nil && !errors.Is(err, mygo.ErrNotificationsDenied) {
		log.Println("show health alert:", err)
	}
}

func (a *application) testHealthNotification() {
	if a.testingNotification {
		return
	}
	a.testingNotification = true
	a.notificationResult = ""
	go func() {
		err := mygo.NewNotification(mygo.NotificationOptions{
			ID:    "health-test",
			Group: "health",
			Title: a.tr("alert.test_title"),
			Body:  a.tr("alert.test_body"),
		}).Show()
		a.win.Update(func() {
			a.testingNotification = false
			if err != nil {
				a.showActionError(fmt.Errorf("send test alert: %w", err))
				return
			}
			a.notificationResult = a.tr("alert.test_sent")
		})
	}()
}

func (a *application) openHealthNotification(string) {
	a.page = 0
	if a.win != nil {
		a.showWindow()
	}
}

func (a *application) requestApplicationMemoryRelease(consumer core.MemoryConsumer) {
	a.pendingMemory = consumer
	a.confirmMemory = true
}

func (a *application) releaseApplicationMemory() {
	consumer := a.pendingMemory
	app, ok := a.applicationByPath(consumer.AppPath)
	if !ok || app.BundleID == "" || app.BundleID == "dev.vitals.mac" || a.releasingApp != "" {
		return
	}
	a.releasingApp = consumer.AppPath
	a.memoryResult = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := core.QuitApplication(ctx, app); err != nil {
			a.win.Update(func() {
				a.releasingApp = ""
				a.showActionError(err)
			})
			return
		}
		for range 6 {
			if !core.ApplicationRunning(ctx, app.Path) {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		stillRunning := core.ApplicationRunning(ctx, app.Path)
		snapshot := a.sampler.RefreshMemoryConsumers(ctx)
		remaining := uint64(0)
		for _, current := range snapshot.MemoryConsumers {
			if current.AppPath == consumer.AppPath {
				remaining = current.RSS
				break
			}
		}
		released := uint64(0)
		if consumer.RSS > remaining {
			released = consumer.RSS - remaining
		}
		a.win.Update(func() {
			a.releasingApp = ""
			a.snapshot = snapshot
			for index := range a.apps {
				if a.apps[index].Path == app.Path {
					a.apps[index].Running = stillRunning
					break
				}
			}
			if stillRunning {
				a.memoryResult = a.trf("memory.release_pending", app.Name)
			} else {
				a.memoryResult = a.trf("memory.release_done", app.Name, formatBytes(released))
			}
		})
	}()
}

func (a *application) applicationByPath(path string) (core.AppInfo, bool) {
	for index := range a.apps {
		if a.apps[index].Path == path {
			return a.apps[index], true
		}
	}
	return core.AppInfo{}, false
}

func (a *application) canReleaseApplicationMemory(path string) bool {
	app, ok := a.applicationByPath(path)
	return ok && app.BundleID != "" && app.BundleID != "dev.vitals.mac"
}

func (a *application) uninstallSelected() {
	if a.selectedApp < 0 || a.selectedApp >= len(a.apps) || a.uninstalling {
		return
	}
	index := a.selectedApp
	app := a.apps[a.selectedApp]
	var paths []string
	for _, file := range app.Related {
		if file.Selected {
			paths = append(paths, file.Path)
		}
	}
	paths = append(paths, app.Path)
	a.uninstalling = true
	a.actionError = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := core.MoveToTrash(ctx, paths, mygo.Shell.TrashItem)
		a.win.Update(func() {
			a.uninstalling = false
			if err != nil {
				a.showActionError(err)
				return
			}
			if index < len(a.apps) && a.apps[index].Path == app.Path {
				a.apps = append(a.apps[:index], a.apps[index+1:]...)
				a.selectedApp = -1
			}
		})
	}()
}

func (a *application) quitSelectedApplication() {
	if a.selectedApp < 0 || a.selectedApp >= len(a.apps) || a.quittingApp {
		return
	}
	index := a.selectedApp
	app := a.apps[index]
	a.quittingApp = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := core.QuitApplication(ctx, app)
		a.win.Update(func() {
			a.quittingApp = false
			if err != nil {
				a.showActionError(err)
				return
			}
			if index < len(a.apps) && a.apps[index].Path == app.Path {
				a.apps[index].Running = false
			}
		})
	}()
}

func (a *application) showActionError(err error) {
	var permissionError *core.FullDiskAccessError
	a.errorSettings = errors.As(err, &permissionError)
	if a.errorSettings {
		a.actionError = a.trf("error.full_disk_body", permissionError.Path)
	} else {
		a.actionError = err.Error()
	}
	a.errorOpen = true
}

func (a *application) persistSettings() {
	a.settings.normalize()
	a.sampleEvery.Store(int64(a.settings.refreshInterval()))
	if a.settingsPath != "" {
		if err := saveSettings(a.settingsPath, a.settings); err != nil {
			a.showActionError(fmt.Errorf("save settings: %w", err))
		}
	}
	if a.tray != nil {
		_ = a.tray.SetIcon(menuBarStatusImage(a.snapshot, a.settings), true)
		a.tray.SetToolTip(menuBarToolTip(a.snapshot, a.settings, a.localizer))
	}
}

func (a *application) setOpenAtLogin(enabled bool) {
	if err := mygo.App.SetOpenAtLogin(enabled); err != nil {
		a.settings.OpenAtLogin = !enabled
		a.showActionError(fmt.Errorf("update login item: %w", err))
		return
	}
	a.settings.OpenAtLogin = enabled
}

func (a *application) toggleWindow() {
	if a.win.IsVisible() {
		a.win.Hide()
		return
	}
	a.showWindow()
}

func (a *application) showWindow() {
	anchor := a.tray.Bounds()
	if anchor.Width > 0 && anchor.Height > 0 {
		area := mygo.Screen.DisplayNearestPoint(mygo.Point{
			X: anchor.X + anchor.Width/2,
			Y: anchor.Y + anchor.Height/2,
		}).WorkArea
		bounds := a.win.Bounds()
		x := anchor.X + (anchor.Width-bounds.Width)/2
		y := anchor.Y + anchor.Height + 8
		x = max(area.X, min(x, area.X+area.Width-bounds.Width))
		y = max(area.Y, min(y, area.Y+area.Height-bounds.Height))
		a.win.SetPosition(x, y)
	} else {
		a.win.Center()
	}
	a.win.Show()
	a.win.Focus()
}

func (a *application) openTrayMenu() {
	a.win.Hide()
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: a.tr("common.open_vitals"), Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	})
	a.tray.SetMenu(menu)
	defer a.tray.SetMenu(nil)
	a.tray.PopUpMenu()
}

func filteredApplications(apps []core.AppInfo, query string) []int {
	query = strings.ToLower(strings.TrimSpace(query))
	var indices []int
	for index, app := range apps {
		if query == "" || strings.Contains(strings.ToLower(app.Name), query) || strings.Contains(strings.ToLower(app.BundleID), query) {
			indices = append(indices, index)
		}
	}
	return indices
}

func menuBarStatusImage(snapshot core.Snapshot, settings appSettings) []byte {
	const (
		height       = 36
		metricWidth  = 54
		networkWidth = 108
		leftPadding  = 4
	)
	settings.normalize()
	width := leftPadding
	for _, visible := range []bool{settings.ShowGPU, settings.ShowCPU, settings.ShowRAM} {
		if visible {
			width += metricWidth
		}
	}
	if settings.ShowNetwork {
		width += networkWidth
	}
	down, up := compactRate(snapshot.NetworkDown), compactRate(snapshot.NetworkUp)
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	ink := image.NewUniform(color.NRGBA{A: 255})
	text := font.Drawer{Dst: img, Src: ink, Face: menuBarFont()}
	drawText := func(x, baseline int, value string) {
		text.Dot = fixed.P(x, baseline)
		text.DrawString(value)
	}

	x := leftPadding
	if settings.ShowGPU {
		drawText(x, 15, "GPU")
		drawText(x, 33, fmt.Sprintf("%.0f%%", snapshot.GPUPercent))
		x += metricWidth
	}
	if settings.ShowCPU {
		drawText(x, 15, "CPU")
		drawText(x, 33, fmt.Sprintf("%.0f%%", snapshot.CPUPercent))
		x += metricWidth
	}
	if settings.ShowRAM {
		drawText(x, 15, "RAM")
		drawText(x, 33, fmt.Sprintf("%.0f%%", snapshot.MemoryPercent))
		x += metricWidth
	}
	if settings.ShowNetwork {
		drawArrow(img, x, 5, true)
		drawArrow(img, x, 22, false)
		drawText(x+13, 15, down)
		drawText(x+13, 33, up)
	}

	var buffer bytes.Buffer
	_ = png.Encode(&buffer, img)
	return buffer.Bytes()
}

func menuBarToolTip(snapshot core.Snapshot, settings appSettings, tr i18n.Localizer) string {
	parts := make([]string, 0, 5)
	if settings.ShowGPU {
		parts = append(parts, fmt.Sprintf("GPU %.0f%%", snapshot.GPUPercent))
	}
	if settings.ShowCPU {
		parts = append(parts, fmt.Sprintf("CPU %.0f%%", snapshot.CPUPercent))
	}
	if settings.ShowRAM {
		parts = append(parts, fmt.Sprintf("RAM %.0f%%", snapshot.MemoryPercent))
	}
	if settings.ShowNetwork {
		parts = append(parts, tr.Tf("menu.tooltip.download", formatRate(snapshot.NetworkDown)), tr.Tf("menu.tooltip.upload", formatRate(snapshot.NetworkUp)))
	}
	return strings.Join(parts, " · ") + " · " + tr.Tf("menu.tooltip.updated", snapshot.UpdatedAt.Format("15:04:05"))
}

var statusFont struct {
	once sync.Once
	face font.Face
}

func menuBarFont() font.Face {
	statusFont.once.Do(func() {
		data, err := os.ReadFile("/System/Library/Fonts/Supplemental/Arial Bold.ttf")
		if err != nil {
			return
		}
		parsed, err := opentype.Parse(data)
		if err != nil {
			return
		}
		statusFont.face, _ = opentype.NewFace(parsed, &opentype.FaceOptions{
			Size:    8,
			DPI:     144,
			Hinting: font.HintingFull,
		})
	})
	if statusFont.face != nil {
		return statusFont.face
	}
	return basicfont.Face7x13
}

func drawArrow(img draw.Image, x, y int, down bool) {
	ink := color.NRGBA{A: 255}
	if down {
		for offset := 0; offset < 8; offset++ {
			img.Set(x+4, y+offset, ink)
			img.Set(x+5, y+offset, ink)
		}
		for offset := 0; offset < 4; offset++ {
			img.Set(x+offset+1, y+7-offset, ink)
			img.Set(x+8-offset, y+7-offset, ink)
		}
		return
	}
	for offset := 0; offset < 8; offset++ {
		img.Set(x+4, y+offset, ink)
		img.Set(x+5, y+offset, ink)
	}
	for offset := 0; offset < 4; offset++ {
		img.Set(x+offset+1, y+offset, ink)
		img.Set(x+8-offset, y+offset, ink)
	}
}

func compactRate(value uint64) string {
	switch {
	case value >= 1024*1024*1024:
		return fmt.Sprintf("%.1fg/s", float64(value)/(1024*1024*1024))
	case value >= 1024*1024:
		return fmt.Sprintf("%.1fm/s", float64(value)/(1024*1024))
	case value >= 100*1024:
		return fmt.Sprintf("%.0fKB/s", float64(value)/1024)
	case value >= 1024:
		return fmt.Sprintf("%.1fKB/s", float64(value)/1024)
	default:
		return fmt.Sprintf("%dB/s", value)
	}
}
