package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"

	"vitals/internal/clipboard"
	"vitals/internal/config"
	"vitals/internal/platform/macos"
	"vitals/internal/translation"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	panelWidth       = 480
	panelHeight      = 588
	accessibilityURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
)

// application owns UI state on MyGo's main thread. The disk worker owns Store
// and publishes read-only snapshots through Window.Update.
type application struct {
	win      *mygo.Window
	tray     *mygo.Tray
	native   *macos.Bridge
	quitting bool

	settings          config.Settings
	settingsPath      string
	settingsOpen      bool
	settingsTab       int
	shortcutDraft     string
	shortcutActive    string
	shortcutSuspended bool
	limitChoice       string
	recording         bool
	trusted           bool

	entries       []clipboard.Entry
	loading       bool
	storageFailed bool
	query         string
	tab           int
	selected      int
	list          ui.ListState
	searchHandle  ui.Handle
	historyHandle ui.Handle
	editorHandle  ui.Handle
	detail        *clipboard.Entry
	editDraft     string
	saving        bool
	message       string
	confirmClear  bool
	confirmDelete bool
	deleteID      string

	translationDraft         translation.Config
	translationMode          bool
	translating              bool
	translationError         string
	translationCancel        context.CancelFunc
	translationSequence      uint64
	checkingTranslation      bool
	translationStatus        string
	translationCheckCancel   context.CancelFunc
	translationCheckSequence uint64

	images         map[string]*ui.Bitmap
	imageRequested map[string]bool
	imageErrors    map[string]string
	previewID      string
	previewImage   *ui.Bitmap
	previewError   string
	jobs           chan func(*clipboard.Store)
	done           chan struct{}
	workerDone     chan struct{}
	lastChange     int
	target         macos.Target
	actionSequence uint64
	pasting        bool
}

func newApplication() *application {
	return &application{
		settings:         config.Default(),
		shortcutDraft:    config.DefaultShortcut,
		limitChoice:      "500",
		loading:          true,
		images:           make(map[string]*ui.Bitmap),
		imageRequested:   make(map[string]bool),
		imageErrors:      make(map[string]string),
		jobs:             make(chan func(*clipboard.Store), 32),
		done:             make(chan struct{}),
		workerDone:       make(chan struct{}),
		translationDraft: translation.Default(),
	}
}

func (a *application) ready() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Fatal(err)
	}
	// Keep the old system-monitor settings separate from the clipboard app.
	a.settingsPath = filepath.Join(dir, "clipboard-settings.json")
	settingsLoaded := false
	if settings, err := config.Load(a.settingsPath); err == nil {
		a.settings = settings
		settingsLoaded = true
	} else {
		a.message = err.Error()
	}
	a.shortcutDraft = a.settings.Shortcut
	a.limitChoice = fmt.Sprint(a.settings.HistoryLimit)
	a.settings.OpenAtLogin = mygo.App.OpenAtLogin()
	a.translationDraft = a.settings.Translation
	a.native, err = macos.NewBridge()
	if err != nil {
		log.Fatal("macOS integration: ", err)
	}
	a.trusted = a.native.Trusted()
	a.target = a.native.Frontmost()
	a.lastChange = a.native.ChangeCount()
	a.tray, err = mygo.NewTray(mygo.TrayOptions{
		Icon: menuBarIcon(), IconIsTemplate: true, ToolTip: "Clipboard",
	})
	if err != nil {
		log.Fatal(err)
	}
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title: "Clipboard", Width: panelWidth, Height: panelHeight,
		Hidden: true, Frameless: true, Transparent: true,
		DisableResize: true, DisableMove: true, DisableMinimize: true,
		DisableMaximize: true, DisableFullScreen: true, AlwaysOnTop: true,
		SkipTaskbar: true, Content: ui.View(a.view),
	})
	a.win.OnClose(a.handleWindowClose)
	a.win.OnBlur(func() {
		if !a.pasting {
			a.hideWindow()
		}
	})
	a.win.OnFocus(func() { a.trusted = a.native.Trusted(); a.win.Invalidate() })
	a.tray.OnClick(a.toggleWindow)
	a.tray.OnRightClick(a.toggleWindow)
	if err := mygo.GlobalShortcut.Register(a.settings.Shortcut, a.toggleWindow); err != nil {
		a.message = "快捷键注册失败，请在设置中更换组合：" + err.Error()
	} else {
		a.shortcutActive = a.settings.Shortcut
	}
	go a.diskLoop(filepath.Join(dir, "Clipboard"))
	if settingsLoaded {
		// Persist the normalized schema to remove obsolete translation settings.
		a.persistSettings()
	}
	go a.monitorLoop()
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.shutdown() })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		mygo.GlobalShortcut.UnregisterAll()
		a.tray.Destroy()
	})
	if launchWindowVisible() {
		a.showWindow()
	}
}

func launchWindowVisible() bool {
	return slices.Contains(os.Args[1:], "--show")
}

func (a *application) diskLoop(dir string) {
	defer close(a.workerDone)
	store, err := clipboard.Open(dir)
	a.win.Update(func() {
		a.loading = false
		if err != nil {
			a.storageFailed = true
			a.message = err.Error()
		}
	})
	if err == nil {
		entries := store.Snapshot()
		a.win.Update(func() { a.entries = entries })
	} else {
		store = nil
	}
	for job := range a.jobs {
		job(store)
	}
}

func (a *application) submit(job func(*clipboard.Store)) bool {
	if a.quitting || a.win == nil {
		return false
	}
	select {
	case a.jobs <- job:
		return true
	default:
		a.message = "正在处理剪贴板，请稍后重试"
		return false
	}
}

func (a *application) mutate(fn func(*clipboard.Store) error, after func()) {
	a.cancelPending()
	if !a.submit(func(store *clipboard.Store) {
		if store == nil {
			a.win.Update(func() {
				a.saving = false
				a.message = "历史存储不可用，请检查数据目录后重新启动"
			})
			return
		}
		err := fn(store)
		entries := store.Snapshot()
		a.win.Update(func() {
			if err != nil {
				a.saving = false
				a.message = err.Error()
				return
			}
			a.setEntries(entries)
			if after != nil {
				after()
			}
		})
	}) {
		a.saving = false
	}
}

func (a *application) setEntries(entries []clipboard.Entry) {
	a.entries = entries
	used := make(map[string]bool)
	for _, entry := range entries {
		used[entry.ID] = true
	}
	for id := range a.imageRequested {
		if !used[id] {
			delete(a.images, id)
			delete(a.imageRequested, id)
			delete(a.imageErrors, id)
		}
	}
}

func (a *application) shutdown() {
	if a.quitting {
		return
	}
	a.quitting = true
	a.cancelPending()
	a.cancelTranslation()
	a.cancelTranslationCheck()
	close(a.done)
	close(a.jobs)
	// Jobs only post asynchronous UI updates, so the main thread can safely
	// wait for pending disk writes before exiting.
	<-a.workerDone
}
