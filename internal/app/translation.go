package app

import (
	"context"
	"errors"
	"strings"

	"vitals/internal/clipboard"
	"vitals/internal/translation"
)

func (a *application) cancelTranslation() {
	a.translationSequence++
	if a.translationCancel != nil {
		a.translationCancel()
		a.translationCancel = nil
	}
	if a.translating {
		a.translating = false
		a.translationError = "翻译已取消，可重试"
	}
}

func (a *application) translateEntry(entry clipboard.Entry) {
	if entry.Kind != clipboard.KindText {
		return
	}
	a.openTranslation(entry)
	a.startTranslation()
}

func (a *application) openTranslation(entry clipboard.Entry) {
	a.openEntry(entry)
	a.settingsOpen = false
	a.translationMode = true
	a.translationSource = entry.Text
	a.editDraft = ""
	a.editorHandle.CancelFocus()
	a.historyHandle.CancelFocus()
	a.searchHandle.CancelFocus()
	a.sourceHandle.Focus()
}

func (a *application) startTranslation() {
	if !a.translationMode || a.detail == nil || a.saving {
		return
	}
	a.cancelTranslation()
	a.translationError = ""
	if strings.TrimSpace(a.translationSource) == "" {
		a.translationError = errNoSelection.Error()
		return
	}
	source, id := a.translationSource, a.detail.ID
	a.translating = true
	seq := a.translationSequence
	client, err := translation.New(a.settings.Translation)
	if err != nil {
		a.finishTranslation(seq, id, "", err)
		return
	}
	if a.win == nil {
		a.finishTranslation(seq, id, "", errors.New("翻译服务尚未就绪"))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.translationCancel = cancel
	go func() {
		defer cancel()
		text, err := client.Translate(ctx, source)
		a.win.Update(func() { a.finishTranslation(seq, id, text, err) })
	}()
}

func translationErrorMessage(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "翻译已取消，可重试"
	case errors.Is(err, context.DeadlineExceeded):
		return "翻译超时，请检查网络后重试"
	default:
		return err.Error()
	}
}

func (a *application) finishTranslation(seq uint64, id, text string, err error) {
	if a.quitting || seq != a.translationSequence || !a.translationMode || a.detail == nil || a.detail.ID != id {
		return
	}
	a.translating = false
	a.translationCancel = nil
	if err != nil {
		a.translationError = translationErrorMessage(err)
		return
	}
	a.editDraft = text
	a.translationError = ""
	a.sourceHandle.CancelFocus()
	a.editorHandle.Focus()
}

func (a *application) cancelTranslationCheck() {
	a.translationCheckSequence++
	if a.translationCheckCancel != nil {
		a.translationCheckCancel()
		a.translationCheckCancel = nil
	}
	if a.checkingTranslation {
		a.translationStatus = "连接测试已取消"
	}
	a.checkingTranslation = false
}

func (a *application) testTranslationConnection() {
	a.cancelTranslationCheck()
	cfg := a.translationDraft
	probe := cfg
	probe.TargetLanguage = translation.TargetChinese
	client, err := translation.New(probe)
	if err != nil {
		a.translationStatus = err.Error()
		return
	}
	if a.win == nil {
		return
	}
	a.checkingTranslation = true
	a.translationStatus = "正在测试连接…"
	seq := a.translationCheckSequence
	ctx, cancel := context.WithCancel(context.Background())
	a.translationCheckCancel = cancel
	go func() {
		defer cancel()
		// A fixed sample tests detection and translation without reading the clipboard.
		_, err := client.Translate(ctx, "Hello, world!")
		a.win.Update(func() { a.finishTranslationCheck(seq, cfg, err) })
	}()
}

func (a *application) finishTranslationCheck(seq uint64, cfg translation.Config, err error) {
	if a.quitting || seq != a.translationCheckSequence || cfg != a.translationDraft {
		return
	}
	a.checkingTranslation = false
	a.translationCheckCancel = nil
	if err != nil {
		a.translationStatus = translationErrorMessage(err)
		return
	}
	a.translationStatus = "连接成功 · 测试译文已返回"
}

func (a *application) changeTranslationDraft() {
	a.cancelTranslationCheck()
	a.translationStatus = ""
}

func (a *application) saveTranslationSettings() {
	cfg := a.translationDraft.WithDefaults()
	// Allow clearing both credentials to disconnect the account.
	if cfg.User != "" || cfg.Token != "" {
		if _, err := translation.New(cfg); err != nil {
			a.translationStatus = err.Error()
			return
		}
	}
	a.cancelTranslationCheck()
	a.settings.Translation = cfg
	a.translationDraft = cfg
	a.persistSettings()
	a.translationStatus = "翻译配置已保存"
}
