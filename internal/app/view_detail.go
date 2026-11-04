package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"vitals/internal/clipboard"
	"vitals/internal/translation"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *application) detailView(c *ui.Context) {
	entry := *a.detail
	if entry.Kind == clipboard.KindText {
		c.OnShortcut(primaryModifier(), ui.KeyS, func() { a.saveEdit(false) })
		c.OnShortcut(primaryModifier(), ui.KeyEnter, func() { a.saveEdit(true) })
		if a.translationMode {
			a.translationView(c)
			return
		}
		blocked := a.saving
		ui.TextArea(c.Key("editor"), &a.editDraft).Bind(&a.editorHandle).Grow(1).MinHeight(0).
			Margin(0, 16, 10).Padding(12).FontSize(13).Background(ui.Hex("#FFFFFF")).
			Label("编辑内容").Disabled(blocked)
		ui.Row(c).Padding(0, 16, 14).AlignItems(ui.Center).Gap(7).Children(func() {
			ui.Text(c, fmt.Sprintf("%d 字符", len([]rune(a.editDraft)))).FontSize(10).TextColor(muted).Grow(1)
			ui.Button(c, "取消").Disabled(a.saving).OnClick(a.back)
			ui.Button(c, "保存").Disabled(blocked || a.editDraft == "").OnClick(func() { a.saveEdit(false) })
			ui.PrimaryButton(c, "保存并粘贴").Disabled(blocked || a.editDraft == "").OnClick(func() { a.saveEdit(true) })
		})
		return
	}
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, 16, 12).Gap(10).Children(func() {
		a.requestImage(entry, true)
		if bitmap := a.previewImage; bitmap != nil && a.previewID == entry.ID {
			ui.Box(c).Height(280).Radius(10).Background(lavender).Center().Clip().Children(func() {
				ui.Image(c, bitmap).Fill().Fit(ui.Contain)
			})
		} else if message := a.previewError; message != "" {
			ui.Text(c, message).FontSize(12).TextColor(muted).Padding(20)
		} else if entry.Kind == clipboard.KindImage {
			ui.Box(c).Height(230).Center().Children(func() { ui.Spinner(c) })
		}
		ui.Text(c, entry.Title()).FontSize(14).FontWeight(600)
		ui.Text(c, entry.CopiedAt.Local().Format("2006-01-02 15:04:05")).FontSize(11).TextColor(muted)
		for _, path := range entry.Files {
			ui.Column(c).Padding(10).Gap(5).Radius(8).Background(lavender).Children(func() {
				ui.Text(c, filepath.Base(path)).FontSize(12).SingleLine()
				ui.Text(c, path).FontSize(10).TextColor(muted).MaxLines(2)
				ui.Button(c, "在文件夹中显示").OnClick(func() {
					mygo.Shell.ShowItemInFolder(path)
				})
			})
		}
	})
	ui.Row(c).Padding(0, 16, 14).Gap(8).Justify(ui.End).Children(func() {
		ui.Button(c, "返回").OnClick(a.back)
		ui.PrimaryButton(c, "粘贴到原应用").OnClick(func() { a.pasteEntry(entry) })
	})
}

func (a *application) translationView(c *ui.Context) {
	c.OnShortcut(primaryModifier()|ui.Shift, ui.KeyEnter, a.startTranslation)
	ui.Column(c).Grow(1).MinHeight(0).Margin(0, 16, 10).Gap(10).Children(func() {
		ui.Column(c).Grow(1).MinHeight(0).Radius(10).Border(1, hairline).Background(lavender).Clip().Children(func() {
			ui.Row(c).Padding(9, 12, 5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "原文").FontSize(12).FontWeight(600).TextColor(accent).Grow(1)
				ui.Text(c, fmt.Sprintf("%d 字符 · 只读", len([]rune(a.translationSource)))).FontSize(10).TextColor(muted)
			})
			ui.TextArea(c.Key("translation-source"), &a.translationSource).Bind(&a.sourceHandle).
				Label("原文内容").Placeholder("暂无原文").ReadOnly(true).Grow(1).MinHeight(0).
				Padding(8, 12).FontSize(13).Background(ui.Transparent).BorderWidth(0).
				Disabled(a.saving)
		})
		ui.Column(c).Grow(1.15).MinHeight(0).Radius(10).Border(1, hairline).Background(ui.Hex("#FFFFFF")).Clip().Children(func() {
			ui.Row(c).Padding(9, 12, 5).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "译文").FontSize(12).FontWeight(600)
				target := "自动互译"
				switch a.settings.Translation.TargetLanguage {
				case translation.TargetChinese:
					target = "简体中文"
				case translation.TargetEnglish:
					target = "英文"
				}
				ui.Text(c, target).FontSize(10).TextColor(muted).Grow(1)
				label := fmt.Sprintf("%d 字符 · 可编辑", len([]rune(a.editDraft)))
				if a.translating {
					ui.Spinner(c).Size(12, 12)
					label = "正在翻译…"
				}
				ui.Text(c, label).FontSize(10).TextColor(muted)
			})
			if a.translationError != "" {
				ui.Text(c, a.translationError).Margin(4, 12).FontSize(11).TextColor(c.Theme().Danger).MaxLines(3)
			}
			ui.TextArea(c.Key("translation-result"), &a.editDraft).Bind(&a.editorHandle).
				Label("编辑内容").Placeholder("译文会显示在这里，也可直接编辑…").Grow(1).MinHeight(0).
				Padding(8, 12).FontSize(13).Background(ui.Transparent).BorderWidth(0).
				Disabled(a.saving || a.translating)
		})
	})
	ui.Row(c).Padding(0, 16, 14).AlignItems(ui.Center).Gap(7).Shrink(0).Children(func() {
		if a.translating {
			ui.Button(c, "停止").OnClick(a.cancelTranslation)
		} else {
			label := "重新翻译"
			if a.translationError != "" {
				label = "重试"
			}
			ui.Button(c, label).Disabled(a.saving || strings.TrimSpace(a.translationSource) == "").
				OnClick(a.startTranslation)
		}
		ui.Spacer(c)
		ui.Button(c, "取消").Disabled(a.saving).OnClick(a.back)
		blocked := a.saving || a.translating || strings.TrimSpace(a.editDraft) == ""
		ui.Button(c, "保存").Disabled(blocked).OnClick(func() { a.saveEdit(false) })
		ui.PrimaryButton(c, "保存并粘贴").Disabled(blocked).OnClick(func() { a.saveEdit(true) })
	})
}
