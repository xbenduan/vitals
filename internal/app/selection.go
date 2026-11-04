package app

import (
	"errors"
	"strings"
	"time"

	"clipnest/internal/clipboard"
	"clipnest/internal/platform"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

var (
	errSelectionFocus     = errors.New("原应用已失去焦点，请重新划词后使用快捷键")
	errSelectionClipboard = errors.New("剪贴板在取词时发生变化，请重新划词后重试")
	errNoSelection        = errors.New("未获取到选中文字，请回到原应用重新划词，再按翻译快捷键")
)

// selectionDesktop keeps native/clipboard operations on the UI thread; the
// polling state machine can also run against a synthetic desktop in tests.
type selectionDesktop interface {
	Frontmost() platform.Target
	ModifiersReleased() bool
	SelectedText(platform.Target) (string, bool, error)
	Copy(platform.Target) error
	ChangeCount() int
	ReadClipboard(...transfer.Format) (transfer.Data, error)
	WriteClipboard(transfer.Data) error
}

type nativeSelectionDesktop struct{ *platform.Bridge }

func (nativeSelectionDesktop) ReadClipboard(formats ...transfer.Format) (transfer.Data, error) {
	return mygo.Clipboard.Read(formats...)
}

func (nativeSelectionDesktop) WriteClipboard(data transfer.Data) error {
	return mygo.Clipboard.Write(data)
}

type selectionCapture struct {
	desktop       selectionDesktop
	target        platform.Target
	deadline      time.Time
	copied        bool
	before        int
	snapshot      transfer.Data
	consumed      bool
	consumedCount int
}

func (s *selectionCapture) step(now time.Time) (string, bool, error) {
	if !s.desktop.Frontmost().Matches(s.target) {
		return "", true, errSelectionFocus
	}
	if !s.copied {
		if now.After(s.deadline) {
			return "", true, errors.New("请松开快捷键后重试")
		}
		if !s.desktop.ModifiersReleased() {
			return "", false, nil
		}
		text, supported, err := s.desktop.SelectedText(s.target)
		if err != nil {
			return "", true, err
		}
		if !s.desktop.Frontmost().Matches(s.target) {
			return "", true, errSelectionFocus
		}
		if supported {
			if strings.TrimSpace(text) == "" {
				return "", true, errNoSelection
			}
			return text, true, nil
		}
		s.before = s.desktop.ChangeCount()
		s.snapshot, err = s.desktop.ReadClipboard()
		if err != nil {
			return "", true, errors.New("暂时无法备份剪贴板，请回到原应用重新划词后重试")
		}
		if !s.desktop.Frontmost().Matches(s.target) {
			return "", true, errSelectionFocus
		}
		if s.desktop.ChangeCount() != s.before {
			return "", true, errSelectionClipboard
		}
		if err := s.desktop.Copy(s.target); err != nil {
			return "", true, err
		}
		s.copied = true
		s.deadline = now.Add(time.Second)
		return "", false, nil
	}
	count := s.desktop.ChangeCount()
	if count == s.before {
		if now.After(s.deadline) {
			return "", true, errNoSelection
		}
		return "", false, nil
	}
	// Only a fresh copy can be translated. Never fall back to previous content.
	data, err := s.desktop.ReadClipboard(transfer.Text)
	if !s.desktop.Frontmost().Matches(s.target) {
		return "", true, errSelectionFocus
	}
	if s.desktop.ChangeCount() != count {
		return "", true, errSelectionClipboard
	}
	var text []byte
	if err == nil {
		text, err = data.Read(transfer.Text)
	}
	// Restore all representations MyGo could snapshot, including rich text,
	// images and file lists. Do not overwrite a newer copy or changed focus.
	restoreErr := s.desktop.WriteClipboard(s.snapshot)
	s.consumed, s.consumedCount = true, s.desktop.ChangeCount()
	if restoreErr != nil {
		return "", true, errors.New("恢复剪贴板失败，请重试；选中文字仍可能留在剪贴板中")
	}
	if err != nil || strings.TrimSpace(string(text)) == "" {
		return "", true, errNoSelection
	}
	return string(text), true, nil
}

func (a *application) translateSelection() {
	if a.native == nil || a.win == nil || a.quitting || a.recording || a.selectionCapture != nil {
		return
	}
	target := a.native.Frontmost()
	a.resetForShow()
	a.message = ""
	a.target = target
	if target.PID == 0 {
		a.openSelectionTranslation("", errors.New("请先在其他应用中选中文字，再按 "+shortcutLabel(a.settings.TranslationShortcut)))
		a.presentWindow()
		return
	}
	// Keep the source focused until Accessibility/copy has supplied the text.
	capture := &selectionCapture{
		desktop: nativeSelectionDesktop{a.native}, target: target,
		deadline: time.Now().Add(2 * time.Second),
	}
	a.selectionCapture = capture
	a.awaitSelection(capture)
}

func (a *application) awaitSelection(capture *selectionCapture) {
	if a.quitting || a.selectionCapture != capture {
		return
	}
	text, done, err := capture.step(time.Now())
	if !done {
		time.AfterFunc(40*time.Millisecond, func() {
			a.win.Update(func() { a.awaitSelection(capture) })
		})
		return
	}
	a.selectionCapture = nil
	if capture.consumed {
		a.lastChange = capture.consumedCount
	}
	if errors.Is(err, errSelectionFocus) {
		return // The user switched away; do not steal the new window's focus.
	}
	a.openSelectionTranslation(text, err)
	a.presentWindow()
}

func (a *application) openSelectionTranslation(text string, err error) {
	a.openTranslation(clipboard.Entry{Kind: clipboard.KindText, Text: text, Source: a.target.Name})
	if err != nil {
		a.translationError = err.Error()
		a.sourceHandle.Focus()
		return
	}
	a.startTranslation()
}
