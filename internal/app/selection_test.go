package app

import (
	"errors"
	"testing"
	"time"

	"vitals/internal/platform"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

type selectionFixture struct {
	front             platform.Target
	modifiersReleased bool
	selected          string
	supported         bool
	selectionErr      error
	data              transfer.Data
	count             int
	copies, reads     int
	writes            int
	afterRead         func()
}

func (f *selectionFixture) Frontmost() platform.Target { return f.front }
func (f *selectionFixture) ModifiersReleased() bool    { return f.modifiersReleased }
func (f *selectionFixture) SelectedText(platform.Target) (string, bool, error) {
	return f.selected, f.supported, f.selectionErr
}
func (f *selectionFixture) ChangeCount() int { return f.count }
func (f *selectionFixture) Copy(platform.Target) error {
	f.copies++
	return nil
}
func (f *selectionFixture) ReadClipboard(...transfer.Format) (transfer.Data, error) {
	f.reads++
	data := f.data
	if f.afterRead != nil {
		f.afterRead()
	}
	return data, nil
}
func (f *selectionFixture) WriteClipboard(data transfer.Data) error {
	f.writes++
	f.data = data
	f.count++
	return nil
}
func selectionTestCapture() (*selectionCapture, *selectionFixture, time.Time) {
	now := time.Now()
	desktop := &selectionFixture{
		front: platform.Target{PID: 123, Name: "Synthetic editor"}, modifiersReleased: true,
		data: transfer.New(transfer.NewItem(
			transfer.Bytes(transfer.Text, []byte("previous clipboard")),
			transfer.Bytes(transfer.HTML, []byte("<b>previous clipboard</b>")),
		)),
		count: 5,
	}
	return &selectionCapture{desktop: desktop, target: desktop.front, deadline: now.Add(2 * time.Second)}, desktop, now
}

func TestSelectionReadsAccessibilityWithoutClipboard(t *testing.T) {
	capture, desktop, now := selectionTestCapture()
	desktop.supported, desktop.selected = true, "A selected phrase\n第二行"
	text, done, err := capture.step(now)
	if err != nil || !done || text != desktop.selected || desktop.copies != 0 || desktop.reads != 0 || desktop.writes != 0 {
		t.Fatalf("accessibility selection = %q, done=%v, err=%v", text, done, err)
	}
	desktop.selected = ""
	if _, done, err := capture.step(now); !done || !errors.Is(err, errNoSelection) || desktop.copies != 0 {
		t.Fatal("an explicit empty selection must not trigger whole-line copy")
	}
}

func TestSelectionCopyWaitsForReleaseAndRestoresFormats(t *testing.T) {
	capture, desktop, now := selectionTestCapture()
	desktop.modifiersReleased = false
	if _, done, _ := capture.step(now); done || desktop.copies != 0 || desktop.reads != 0 {
		t.Fatal("copy must wait for every shortcut modifier")
	}
	desktop.modifiersReleased = true
	if _, done, err := capture.step(now); done || err != nil || desktop.copies != 1 {
		t.Fatal("released shortcut must start a single copy")
	}
	if _, done, _ := capture.step(now.Add(50 * time.Millisecond)); done || desktop.copies != 1 {
		t.Fatal("unchanged clipboard must wait without copying again")
	}
	desktop.count++
	desktop.data = transfer.TextData("Freshly selected text")
	text, done, err := capture.step(now.Add(100 * time.Millisecond))
	plain, _ := desktop.data.Read(transfer.Text)
	rich, _ := desktop.data.Read(transfer.HTML)
	if !done || err != nil || text != "Freshly selected text" || string(plain) != "previous clipboard" ||
		string(rich) != "<b>previous clipboard</b>" || desktop.writes != 1 ||
		!capture.consumed || capture.consumedCount != desktop.count {
		t.Fatal("capture must return fresh text and restore the complete snapshot")
	}
}

func TestSelectionNeverTranslatesOldClipboardOrChangedFocus(t *testing.T) {
	t.Run("unchanged clipboard", func(t *testing.T) {
		capture, desktop, now := selectionTestCapture()
		capture.step(now)
		text, done, err := capture.step(now.Add(2 * time.Second))
		if !done || !errors.Is(err, errNoSelection) || text != "" || desktop.reads != 1 || desktop.writes != 0 {
			t.Fatal("a copy timeout must not read or translate the previous clipboard")
		}
	})
	for _, duringRead := range []bool{false, true} {
		t.Run(map[bool]string{false: "switch before read", true: "new copy during read"}[duringRead], func(t *testing.T) {
			capture, desktop, now := selectionTestCapture()
			capture.step(now)
			desktop.count++
			wantErr := errSelectionFocus
			if duringRead {
				desktop.afterRead = func() { desktop.count++; desktop.data = transfer.TextData("new user copy") }
				wantErr = errSelectionClipboard
			} else {
				desktop.front.PID++
			}
			text, done, err := capture.step(now.Add(50 * time.Millisecond))
			if !done || !errors.Is(err, wantErr) || text != "" || desktop.writes != 0 {
				t.Fatal("interrupted capture must not return text or overwrite a newer clipboard")
			}
		})
	}
	t.Run("permission denied", func(t *testing.T) {
		capture, desktop, now := selectionTestCapture()
		desktop.selectionErr = errors.New("permission denied")
		if _, done, err := capture.step(now); !done || err == nil || desktop.copies != 0 || desktop.reads != 0 {
			t.Fatal("missing permission must leave the clipboard untouched")
		}
	})
	t.Run("non-text copy", func(t *testing.T) {
		capture, desktop, now := selectionTestCapture()
		capture.step(now)
		desktop.count++
		desktop.data = transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, []byte("synthetic image"))))
		text, done, err := capture.step(now.Add(50 * time.Millisecond))
		if !done || !errors.Is(err, errNoSelection) || text != "" || desktop.writes != 1 {
			t.Fatal("non-text copies must restore the clipboard and open an empty editor")
		}
	})
}

func TestSelectionTranslationUsesExternalTextAndResetCancelsCapture(t *testing.T) {
	app := previewApplication()
	app.openSettings()
	app.query = "old filter"
	app.resetForShow()
	app.openSelectionTranslation("Selected in another app", nil)
	if !app.translationMode || app.settingsOpen || app.translationSource != "Selected in another app" ||
		app.detail == nil || app.detail.Text == app.entries[0].Text || app.query != "" {
		t.Fatal("selected-text translation must prepare an independent translation page")
	}
	if app.translationError == "" {
		t.Fatal("automatic translation must attempt to start and explain missing credentials")
	}
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	if !tester.HasText("原文") || !tester.HasText("译文") {
		t.Fatal("translation must expose both editing areas")
	}
	capture, desktop, _ := selectionTestCapture()
	app.selectionCapture = capture
	app.resetForShow()
	app.awaitSelection(capture)
	if app.selectionCapture != nil || app.detail != nil || app.translationMode || desktop.reads != 0 || desktop.copies != 0 {
		t.Fatal("ordinary reopening must cancel selection acquisition before its next callback")
	}
}
