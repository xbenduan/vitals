package app

import (
	"fmt"
	"path/filepath"

	"vitals/internal/clipboard"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *application) detailView(c *ui.Context) {
	entry := *a.detail
	if entry.Kind == clipboard.KindText {
		c.OnShortcut(ui.Cmd, ui.KeyS, func() { a.saveEdit(false) })
		c.OnShortcut(ui.Cmd, ui.KeyEnter, func() { a.saveEdit(true) })
		if a.translationMode {
			ui.Column(c).Margin(0, 16, 10).Padding(10).Radius(8).Background(lavender).Gap(5).Children(func() {
				ui.Text(c, "原文").FontSize(10).TextColor(muted)
				ui.Text(c, entry.Title()).FontSize(11).MaxLines(2)
				switch {
				case a.translating:
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c).Size(14, 14)
						ui.Text(c, "正在通过腾讯交互翻译…").FontSize(11).TextColor(accent).Grow(1)
						ui.Button(c, "停止").OnClick(a.cancelTranslation)
					})
				case a.translationError != "":
					ui.Text(c, a.translationError).FontSize(11).TextColor(c.Theme().Danger).MaxLines(3)
					ui.Row(c).Gap(7).Children(func() {
						ui.Button(c, "重试").OnClick(func() { a.translateEntry(entry) })
						ui.Button(c, "翻译设置").OnClick(func() { a.openSettings(); a.settingsTab = 1 })
					})
				default:
					ui.Text(c, "译文可继续编辑，保存后加入历史记录").FontSize(10).TextColor(accent)
				}
			})
		}
		blocked := a.saving || a.translating || a.translationError != ""
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
				ui.Button(c, "在 Finder 中显示").OnClick(func() {
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
