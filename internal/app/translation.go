package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

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
	a.openEntry(entry)
	a.translationMode = true
	a.editDraft = ""
	a.translating = true
	cfg, seq := a.settings.Translation, a.translationSequence
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	a.translationCancel = cancel
	if a.win == nil {
		cancel()
		a.finishTranslation(seq, entry.ID, "", "", errors.New("模型服务尚未就绪"))
		return
	}
	go func() {
		defer cancel()
		client, err := translation.New(cfg)
		if err == nil && cfg.Model == "" {
			var models []string
			models, err = client.Models(ctx)
			if err == nil {
				cfg.Model = models[0]
				client, err = translation.New(cfg)
			}
		}
		var text string
		if err == nil {
			text, err = client.Translate(ctx, entry.Text)
		}
		a.win.Update(func() { a.finishTranslation(seq, entry.ID, text, cfg.Model, err) })
	}()
}

func (a *application) finishTranslation(seq uint64, id, text, model string, err error) {
	if a.quitting || seq != a.translationSequence || !a.translationMode || a.detail == nil || a.detail.ID != id {
		return
	}
	a.translating = false
	a.translationCancel = nil
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			a.translationError = "翻译已取消，可重试"
		case errors.Is(err, context.DeadlineExceeded):
			a.translationError = "翻译超时，请重试或更换较小的模型"
		default:
			a.translationError = err.Error()
		}
		return
	}
	if a.settings.Translation.Model == "" && model != "" {
		a.settings.Translation.Model = model
		a.translationDraft = a.settings.Translation
		a.persistSettings()
	}
	a.editDraft = text
	a.translationError = ""
	a.editorHandle.Focus()
}

func (a *application) cancelModelDetection() {
	a.modelSequence++
	if a.modelCancel != nil {
		a.modelCancel()
		a.modelCancel = nil
	}
	if a.detectingModels {
		a.modelStatus = "检测已取消，可重试"
	}
	a.detectingModels = false
}

func (a *application) detectModels(startup bool) {
	a.cancelModelDetection()
	cfg := a.translationDraft
	client, err := translation.New(cfg)
	if err != nil {
		a.modelStatus = err.Error()
		return
	}
	if a.win == nil {
		return
	}
	a.detectingModels = true
	a.modelStatus = "正在连接模型服务…"
	seq := a.modelSequence
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	a.modelCancel = cancel
	go func() {
		defer cancel()
		models, err := client.Models(ctx)
		a.win.Update(func() {
			if a.quitting || seq != a.modelSequence || cfg != a.translationDraft {
				return
			}
			a.detectingModels = false
			a.modelCancel = nil
			if err != nil {
				a.modelStatus = err.Error()
				return
			}
			a.models = models
			if a.translationDraft.Model == "" {
				a.translationDraft.Model = models[0]
			}
			a.modelStatus = fmt.Sprintf("已连接 · 找到 %d 个模型", len(models))
			if startup && a.settings.Translation == cfg && cfg.Model == "" {
				a.settings.Translation.Model = a.translationDraft.Model
				a.persistSettings()
			}
		})
	}()
}

func (a *application) changeTranslationDraft() {
	a.cancelModelDetection()
	a.models = nil
	a.modelStatus = ""
}

func (a *application) changeProvider() {
	a.changeTranslationDraft()
	a.translationDraft.Model = ""
	a.translationDraft.APIKey = ""
	if a.translationDraft.Provider == translation.Ollama {
		a.translationDraft.BaseURL = translation.Default().BaseURL
	} else {
		a.translationDraft.BaseURL = "http://127.0.0.1:1234/v1"
	}
}

func (a *application) saveTranslationSettings() {
	cfg := a.translationDraft
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.Model = strings.TrimSpace(cfg.Model)
	if _, err := translation.New(cfg); err != nil {
		a.modelStatus = err.Error()
		return
	}
	if cfg.Model == "" {
		a.modelStatus = "请先检测或填写模型名称"
		return
	}
	a.cancelModelDetection()
	a.settings.Translation = cfg
	a.translationDraft = cfg
	a.persistSettings()
	a.modelStatus = "翻译配置已保存"
}

func (a *application) modelOptions() []string {
	models := slices.Clone(a.models)
	if a.translationDraft.Model != "" && !slices.Contains(models, a.translationDraft.Model) {
		models = append([]string{a.translationDraft.Model}, models...)
	}
	return models
}
