package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
)

const (
	panelWidth       = 480
	panelHeight      = 588
	accessibilityURL = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
)

type application struct {
	win      *mygo.Window
	tray     *mygo.Tray
	native   *nativeBridge
	quitting bool

	settings          appSettings
	settingsPath      string
	settingsOpen      bool
	shortcutDraft     string
	shortcutActive    string
	shortcutSuspended bool
	limitChoice       string
	recording         bool
	trusted           bool

	entries       []clipEntry
	loading       bool
	storageFailed bool
	query         string
	tab           int
	selected      int
	list          ui.ListState
	searchHandle  ui.Handle
	editorHandle  ui.Handle
	detail        *clipEntry
	editDraft     string
	saving        bool
	message       string
	confirmClear  bool
	confirmDelete bool

	images         map[string]*ui.Bitmap
	imageRequested map[string]bool
	imageErrors    map[string]string
	previewID      string
	previewImage   *ui.Bitmap
	previewError   string
	jobs           chan func(*historyStore)
	done           chan struct{}
	workerDone     chan struct{}
	lastChange     int
	target         pasteTarget
	clickDelay     time.Duration
	clickTimer     *time.Timer
	actionSequence uint64
	pasting        bool
}

func newApplication() *application {
	return &application{
		settings:       defaultSettings(),
		shortcutDraft:  defaultShortcut,
		limitChoice:    "500",
		loading:        true,
		images:         make(map[string]*ui.Bitmap),
		imageRequested: make(map[string]bool),
		imageErrors:    make(map[string]string),
		jobs:           make(chan func(*historyStore), 32),
		done:           make(chan struct{}),
		workerDone:     make(chan struct{}),
		clickDelay:     530 * time.Millisecond,
	}
}

func (a *application) ready() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Fatal(err)
	}
	// Keep the old system-monitor settings separate from the clipboard app.
	a.settingsPath = filepath.Join(dir, "clipboard-settings.json")
	if settings, err := loadSettings(a.settingsPath); err == nil {
		a.settings = settings
	} else {
		a.message = err.Error()
	}
	a.shortcutDraft = a.settings.Shortcut
	a.limitChoice = fmt.Sprint(a.settings.HistoryLimit)
	a.settings.OpenAtLogin = mygo.App.OpenAtLogin()
	a.native, err = newNativeBridge()
	if err != nil {
		log.Fatal("macOS integration: ", err)
	}
	a.trusted = a.native.trusted()
	a.clickDelay = max(a.native.doubleClickDelay(), 530*time.Millisecond)
	a.target = a.native.frontmost()
	a.lastChange = a.native.changeCount()
	a.tray, err = mygo.NewTray(mygo.TrayOptions{
		Icon: menuBarIcon(), IconIsTemplate: true, ToolTip: "Vitals · 剪贴板",
	})
	if err != nil {
		log.Fatal(err)
	}
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title: "Vitals · 剪贴板", Width: panelWidth, Height: panelHeight,
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
	a.win.OnFocus(func() { a.trusted = a.native.trusted(); a.win.Invalidate() })
	a.tray.OnClick(a.toggleWindow)
	a.tray.OnRightClick(a.toggleWindow)
	if err := mygo.GlobalShortcut.Register(a.settings.Shortcut, a.toggleWindow); err != nil {
		a.message = "快捷键注册失败，请在设置中更换组合：" + err.Error()
	} else {
		a.shortcutActive = a.settings.Shortcut
	}
	go a.diskLoop(filepath.Join(dir, "Clipboard"))
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
	store, err := openHistory(dir)
	a.win.Update(func() {
		a.loading = false
		if err != nil {
			a.storageFailed = true
			a.message = err.Error()
		}
	})
	if err == nil {
		entries := store.snapshot()
		a.win.Update(func() { a.entries = entries })
	} else {
		store = nil
	}
	for job := range a.jobs {
		job(store)
	}
}

func (a *application) submit(job func(*historyStore)) bool {
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

func (a *application) mutate(fn func(*historyStore) error, after func()) {
	a.cancelPending()
	if !a.submit(func(store *historyStore) {
		if store == nil {
			a.win.Update(func() {
				a.saving = false
				a.message = "历史存储不可用，请检查数据目录后重新启动"
			})
			return
		}
		err := fn(store)
		entries := store.snapshot()
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

func (a *application) setEntries(entries []clipEntry) {
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

func (a *application) monitorLoop() {
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-a.done:
			return
		case <-ticker.C:
			a.win.Update(a.pollClipboard)
		}
	}
}

func (a *application) pollClipboard() {
	if a.quitting {
		return
	}
	front := a.native.frontmost()
	if !a.win.IsVisible() && !a.pasting && front.PID != 0 {
		a.target = front
	}
	count := a.native.changeCount()
	if a.settings.Paused || a.storageFailed || a.loading {
		a.lastChange = count
		return
	}
	if count == a.lastChange || len(a.jobs) == cap(a.jobs) {
		return
	}
	capture := clipCapture{Source: front.Name, At: time.Now()}
	formats := mygo.Clipboard.Formats()
	// Files take precedence over an accompanying text URL or image preview.
	if slices.Contains(formats, transfer.FileList) || slices.Contains(formats, transfer.URIList) {
		files, err := mygo.Clipboard.ReadFiles()
		if err == nil && len(files) > 0 {
			capture.Kind, capture.Files = kindFiles, files
		}
	}
	if capture.Kind == "" && slices.Contains(formats, transfer.PNG) {
		if data := mygo.Clipboard.ReadImage(); len(data) > 0 {
			capture.Kind, capture.PNG = kindImage, data
		}
	}
	if capture.Kind == "" && slices.Contains(formats, transfer.Text) {
		if text := mygo.Clipboard.ReadText(); text != "" {
			capture.Kind, capture.Text = kindText, text
		}
	}
	// A copy arriving during the read is retried in the next poll.
	if a.native.changeCount() != count {
		return
	}
	a.lastChange = count
	if capture.Kind == "" {
		return
	}
	limit := a.settings.HistoryLimit
	a.submit(func(store *historyStore) {
		if store == nil {
			return
		}
		err := store.add(capture, limit)
		entries := store.snapshot()
		a.win.Update(func() {
			if err != nil {
				a.message = err.Error()
				return
			}
			a.setEntries(entries)
		})
	})
}

func (a *application) cancelPending() {
	a.actionSequence++
	if a.clickTimer != nil {
		a.clickTimer.Stop()
		a.clickTimer = nil
	}
	a.pasting = false
}

func (a *application) clickEntry(entry clipEntry, index int) {
	a.cancelPending()
	a.selected = index
	seq := a.actionSequence
	if a.win == nil {
		return
	}
	a.clickTimer = time.AfterFunc(a.clickDelay, func() {
		a.win.Update(func() {
			if a.actionSequence == seq && a.win.IsVisible() && a.detail == nil && !a.settingsOpen {
				a.pasteEntry(entry)
			}
		})
	})
}

func (a *application) openEntry(entry clipEntry) {
	a.cancelPending()
	a.detail = &entry
	a.editDraft = entry.Text
	a.message = ""
	if entry.Kind == kindText {
		a.editorHandle.Focus()
	}
}

func (a *application) saveEdit(paste bool) {
	if a.detail == nil || a.detail.Kind != kindText || a.saving {
		return
	}
	entry, draft, limit := *a.detail, a.editDraft, a.settings.HistoryLimit
	a.saving = true
	a.mutate(func(store *historyStore) error { return store.edit(entry.ID, draft, limit) }, func() {
		a.saving = false
		a.detail = nil
		a.searchHandle.Focus()
		if paste {
			entry.Text = draft
			a.pasteEntry(entry)
		}
	})
}

func (a *application) pasteEntry(entry clipEntry) {
	a.cancelPending()
	if a.win == nil {
		return
	}
	a.pasting = true
	seq, target := a.actionSequence, a.target
	if !a.submit(func(store *historyStore) {
		var data transfer.Data
		var err error
		switch entry.Kind {
		case kindText:
			data = transfer.TextData(entry.Text)
		case kindImage:
			if store == nil {
				err = errors.New("图片存储不可用")
			} else {
				var pngData []byte
				pngData, err = os.ReadFile(store.imagePath(entry))
				if err == nil {
					data = transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, pngData)))
				}
			}
		case kindFiles:
			for _, path := range entry.Files {
				if _, statErr := os.Stat(path); statErr != nil {
					err = fmt.Errorf("文件已移动或不可访问：%s", filepath.Base(path))
					break
				}
			}
			if err == nil {
				data, err = transfer.FileData(entry.Files...)
			}
		}
		a.win.Update(func() {
			if seq != a.actionSequence {
				return
			}
			if err == nil {
				err = mygo.Clipboard.Write(data)
			}
			if err != nil {
				a.pasting = false
				a.message = err.Error()
				return
			}
			a.lastChange = a.native.changeCount()
			a.trusted = a.native.trusted()
			if !a.trusted {
				a.pasting = false
				a.message = "内容已复制。开启「辅助功能」后即可自动粘贴"
				a.settingsOpen = true
				return
			}
			if target.PID == 0 {
				a.pasting = false
				a.message = "内容已复制。请先切换到目标应用，再打开剪贴板"
				return
			}
			a.win.Hide()
			if !a.native.activate(target) {
				a.pasteFailed("目标应用已退出或无法激活，内容已复制")
				return
			}
			a.awaitPaste(target, seq, time.Now().Add(2*time.Second))
		})
	}) {
		a.pasting = false
	}
}

func (a *application) awaitPaste(target pasteTarget, seq uint64, deadline time.Time) {
	time.AfterFunc(45*time.Millisecond, func() {
		a.win.Update(func() {
			if a.quitting || seq != a.actionSequence {
				return
			}
			if a.native.frontmost().PID == target.PID && a.native.modifiersReleased() {
				err := a.native.paste(target)
				a.pasting = false
				if err != nil {
					a.pasteFailed(err.Error())
				}
				return
			}
			if time.Now().After(deadline) {
				a.pasteFailed("未能恢复目标应用焦点，内容已复制，可使用 ⌘V 粘贴")
				return
			}
			a.awaitPaste(target, seq, deadline)
		})
	})
}

func (a *application) pasteFailed(message string) {
	a.pasting = false
	a.message = message
	a.showWindow()
}

func (a *application) requestImage(entry clipEntry, preview bool) {
	if a.win == nil || entry.Kind == kindText {
		return
	}
	if preview {
		if a.previewID == entry.ID {
			return
		}
		a.previewID, a.previewImage, a.previewError = entry.ID, nil, ""
	} else {
		if a.imageRequested[entry.ID] {
			return
		}
		a.imageRequested[entry.ID] = true
	}
	if !a.submit(func(store *historyStore) {
		var data []byte
		var err error
		path := ""
		if entry.Kind == kindImage && store != nil {
			path = store.imagePath(entry)
		} else if entry.Kind == kindFiles && len(entry.Files) == 1 && isImagePath(entry.Files[0]) {
			path = entry.Files[0]
		}
		if path == "" {
			return
		}
		info, err := os.Stat(path)
		if err == nil && info.Size() > maxImageBytes {
			err = errors.New("图片超过预览大小限制")
		}
		if err == nil {
			data, err = os.ReadFile(path)
		}
		var bitmap *ui.Bitmap
		if err == nil {
			config, _, configErr := image.DecodeConfig(bytes.NewReader(data))
			if configErr != nil || int64(config.Width)*int64(config.Height) > maxImagePixels {
				err = errors.New("无法预览此图片")
			} else {
				var decoded image.Image
				decoded, _, err = image.Decode(bytes.NewReader(data))
				if err == nil {
					size := 72
					if preview {
						size = 1280
					}
					bitmap = ui.NewBitmap(resizePreview(decoded, size))
				}
			}
		}
		a.win.Update(func() {
			if preview {
				if a.previewID != entry.ID {
					return
				}
				if err != nil {
					a.previewError = "图片无法预览，文件可能已移动或不受支持"
				} else {
					a.previewImage = bitmap
				}
			} else {
				if !a.imageRequested[entry.ID] {
					return
				}
				if err != nil {
					a.imageErrors[entry.ID] = "图片无法预览"
				} else {
					a.images[entry.ID] = bitmap
				}
			}
		})
	}) {
		delete(a.imageRequested, entry.ID)
		if preview {
			a.previewID = ""
		}
	}
}

func resizePreview(source image.Image, size int) image.Image {
	bounds := source.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if max(w, h) > size {
		ratio := float64(size) / float64(max(w, h))
		w, h = max(1, int(float64(w)*ratio)), max(1, int(float64(h)*ratio))
	}
	target := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Src, nil)
	return target
}

func isImagePath(path string) bool {
	return slices.Contains([]string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"}, lowerExtension(path))
}

func (a *application) applyShortcut() {
	if a.native == nil {
		return
	}
	shortcut, err := normalizeShortcut(a.shortcutDraft)
	if err != nil {
		a.message = err.Error()
		return
	}
	if shortcut == a.shortcutActive {
		a.setRecording(false)
		return
	}
	// Register the new binding before releasing the old one. A conflict leaves
	// the working shortcut intact.
	if err := mygo.GlobalShortcut.Register(shortcut, a.toggleWindow); err != nil {
		a.message = "此快捷键无法注册，请换一个组合"
		return
	}
	if a.shortcutActive != "" {
		mygo.GlobalShortcut.Unregister(a.shortcutActive)
	}
	a.shortcutActive, a.settings.Shortcut, a.shortcutDraft = shortcut, shortcut, shortcut
	a.recording = false
	a.message = "快捷键已更新"
	a.persistSettings()
}

func (a *application) setRecording(recording bool) {
	a.recording = recording
	if a.native == nil || a.shortcutActive == "" {
		return
	}
	if recording && !a.shortcutSuspended {
		mygo.GlobalShortcut.Unregister(a.shortcutActive)
		a.shortcutSuspended = true
	} else if !recording && a.shortcutSuspended {
		a.shortcutSuspended = false
		if err := mygo.GlobalShortcut.Register(a.shortcutActive, a.toggleWindow); err != nil {
			a.message = "原快捷键无法恢复，请重新设置"
			a.shortcutActive = ""
		}
	}
}

func (a *application) persistSettings() {
	settings, path := a.settings, a.settingsPath
	if path == "" {
		return
	}
	a.submit(func(*historyStore) {
		if err := saveSettings(path, settings); err != nil {
			a.win.Update(func() { a.message = "设置未能保存：" + err.Error() })
		}
	})
}

func (a *application) toggleListening() {
	a.cancelPending()
	a.settings.Paused = !a.settings.Paused
	if a.native != nil {
		// Resuming starts with the next copy, not content copied while paused.
		a.lastChange = a.native.changeCount()
	}
	a.persistSettings()
}

func (a *application) setOpenAtLogin() {
	if err := mygo.App.SetOpenAtLogin(a.settings.OpenAtLogin); err != nil {
		a.settings.OpenAtLogin = !a.settings.OpenAtLogin
		a.message = "登录启动设置失败：" + err.Error()
	}
}

func (a *application) handleWindowClose(event *mygo.CloseEvent) {
	if !a.quitting {
		event.PreventDefault()
		a.hideWindow()
	}
}

func (a *application) hideWindow() {
	a.cancelPending()
	a.setRecording(false)
	if a.win != nil {
		a.win.Hide()
	}
}

func (a *application) toggleWindow() {
	if a.win.IsVisible() {
		a.hideWindow()
	} else {
		a.showWindow()
	}
}

func (a *application) showWindow() {
	a.cancelPending()
	if target := a.native.frontmost(); target.PID != 0 {
		a.target = target
	}
	a.trusted = a.native.trusted()
	anchor := a.tray.Bounds()
	if anchor.Width > 0 {
		area := mygo.Screen.DisplayNearestPoint(mygo.Point{X: anchor.X, Y: anchor.Y}).WorkArea
		bounds := a.win.Bounds()
		x := max(area.X, min(anchor.X+(anchor.Width-bounds.Width)/2, area.X+area.Width-bounds.Width))
		y := max(area.Y, min(anchor.Y+anchor.Height+6, area.Y+area.Height-bounds.Height))
		a.win.SetPosition(x, y)
	} else {
		a.win.Center()
	}
	if a.detail == nil && !a.settingsOpen {
		a.searchHandle.Focus()
	}
	a.win.Show()
	a.win.Focus()
}

func (a *application) shutdown() {
	if a.quitting {
		return
	}
	a.quitting = true
	a.cancelPending()
	close(a.done)
	close(a.jobs)
	// Jobs only post asynchronous UI updates, so the main thread can safely
	// wait for pending disk writes before exiting.
	<-a.workerDone
}

func menuBarIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 36, 36))
	ink := color.NRGBA{A: 255}
	line := func(x, y, w, h int) {
		for dx := range w {
			for dy := range h {
				img.Set(x+dx, y+dy, ink)
			}
		}
	}
	line(9, 7, 4, 25)
	line(9, 28, 21, 4)
	line(27, 7, 3, 25)
	line(9, 7, 6, 3)
	line(23, 7, 7, 3)
	line(14, 4, 10, 3)
	line(14, 4, 3, 8)
	line(21, 4, 3, 8)
	line(14, 10, 10, 3)
	line(16, 17, 8, 2)
	line(16, 22, 6, 2)
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, img)
	return buffer.Bytes()
}
