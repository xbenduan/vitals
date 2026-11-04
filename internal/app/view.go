package app

import (
	"clipnest/internal/clipboard"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *application) view(c *ui.Context) {
	c.SetTheme(clipboardTheme())
	c.Root().Background(ui.Transparent)
	if a.recording {
		a.recordShortcut(c)
	} else {
		c.OnShortcut(0, ui.KeyEscape, a.back)
		c.OnShortcut(primaryModifier(), ui.KeyW, a.hideWindow)
		c.OnShortcut(primaryModifier(), ui.KeyQ, mygo.App.Quit)
		if !a.settingsOpen && a.detail == nil && !a.confirmDelete && !a.confirmClear {
			c.OnShortcut(primaryModifier(), ui.KeyF, func() { a.cancelPending(); a.searchHandle.Focus() })
			c.OnShortcut(0, ui.KeyDown, func() { a.moveSelection(1) })
			c.OnShortcut(0, ui.KeyUp, func() { a.moveSelection(-1) })
			c.OnShortcut(0, ui.KeyEnter, a.pasteSelected)
			c.OnShortcut(primaryModifier(), ui.KeyE, a.editSelected)
			c.OnShortcut(primaryModifier(), ui.KeyD, a.favoriteSelected)
			if !a.searchHandle.Focused(c) {
				c.OnShortcut(0, ui.KeyLeft, func() { a.selectHistoryTab(a.tab - 1) })
				c.OnShortcut(0, ui.KeyRight, func() { a.selectHistoryTab(a.tab + 1) })
				for i, key := range []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9} {
					c.OnShortcut(0, key, func() { a.pasteNumber(i) })
				}
			}
		}
	}
	ui.Column(c).Fill().Margin(1).Radius(13).Clip().Background(paper).Border(1, hairline).Children(func() {
		a.header(c)
		if a.message != "" {
			a.messageView(c)
		}
		switch {
		case a.settingsOpen:
			a.settingsView(c)
		case a.detail != nil:
			a.detailView(c)
		default:
			a.historyView(c)
		}
		a.footer(c)
	})
	if ui.AlertDialog(c, &a.confirmClear, "清空历史记录？", "将删除未收藏的记录及保存的图片，收藏内容会保留。", "取消", "清空历史") == 1 {
		a.mutate(func(s *clipboard.Store) error { return s.Clear() }, func() { a.selected = 0 })
	}
	if ui.AlertDialog(c, &a.confirmDelete, "删除这条记录？", "只删除剪贴板历史，不会删除原始文件。", "取消", "删除记录") == 1 && a.deleteID != "" {
		id := a.deleteID
		a.deleteID = ""
		a.mutate(func(s *clipboard.Store) error { return s.Remove(id) }, func() { a.historyHandle.Focus() })
	}
}

func (a *application) header(c *ui.Context) {
	ui.Row(c).Height(53).Shrink(0).Padding(0, 16).Gap(9).AlignItems(ui.Center).Children(func() {
		if a.settingsOpen || a.detail != nil {
			backLabel := "返回历史"
			if a.settingsOpen && a.translationMode && a.detail != nil {
				backLabel = "返回翻译"
			}
			iconButton(c, backLabel, iconBack, a.back).Disabled(a.saving)
			title := "设置"
			if !a.settingsOpen {
				switch {
				case a.translationMode:
					title = "翻译结果"
				case a.detail.Kind == clipboard.KindText:
					title = "编辑文字"
				default:
					title = "预览内容"
				}
			}
			ui.Text(c, title).FontSize(15).FontWeight(650).Grow(1)
			if a.translationMode && !a.settingsOpen {
				iconButton(c, "翻译设置", iconSettings, func() {
					a.openSettings()
					a.settingsTab = 1
				}).Disabled(a.saving)
			}
			return
		}
		ui.Box(c).Size(29, 29).Radius(9).Background(lavender).Center().Children(func() {
			ui.Icon(c, iconClipboard).Size(20, 20).TextColor(accent)
		})
		ui.Text(c, appTitle).FontSize(16).FontWeight(650).Grow(1)
		label, icon, tint := "正在监听", iconListen, ui.Hex("#6F9B85")
		if a.settings.Paused {
			label, icon, tint = "已暂停", iconPause, muted
		} else if a.storageFailed {
			label, tint = "存储异常", c.Theme().Danger
		} else if a.loading {
			label = "正在载入"
		}
		ui.ButtonBase(c).Label(label).Tooltip("点击暂停或恢复监听").
			Padding(5, 7).Radius(7).Gap(5).AlignItems(ui.Center).TextColor(tint).
			Children(func() {
				ui.Icon(c, icon).Size(14, 14)
				ui.Text(c, label).FontSize(11)
			}).OnClick(a.toggleListening)
		iconButton(c, "设置", iconSettings, a.openSettings)
	})
}

func (a *application) messageView(c *ui.Context) {
	ui.Row(c).Margin(0, 13, 8).Padding(8, 10).Radius(8).Background(lavender).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.message).FontSize(11).TextColor(accent).Grow(1).MinWidth(0).MaxLines(3)
		iconButton(c, "关闭提示", iconClose, func() { a.message = "" })
	})
}

func (a *application) footer(c *ui.Context) {
	ui.Row(c).Height(32).Shrink(0).Padding(0, 16).BorderWidth(1, 0, 0, 0).BorderColor(hairline).
		Gap(10).AlignItems(ui.Center).Children(func() {
		if a.settingsOpen {
			ui.Text(c, "Esc 返回").FontSize(10).TextColor(muted).Grow(1)
			ui.Text(c, appTitle).FontSize(10).TextColor(muted)
		} else if a.detail != nil {
			ui.Text(c, "Esc 返回").FontSize(10).TextColor(muted).Grow(1)
			if a.detail.Kind == clipboard.KindText {
				ui.Text(c, primaryShortcutLabel("S")+" 保存").FontSize(10).TextColor(muted)
			}
		} else {
			ui.Text(c, "↑ ↓ 选择").FontSize(10).TextColor(muted)
			ui.Text(c, "↵ 粘贴").FontSize(10).TextColor(muted)
			ui.Text(c, "1–9 快捷粘贴").FontSize(10).TextColor(muted)
			ui.Text(c, "右键操作").FontSize(10).TextColor(muted).Grow(1)
			if a.target.Name != "" {
				ui.Text(c, "粘贴至 "+a.target.Name).FontSize(10).TextColor(accent).MaxWidth(140).SingleLine()
			} else {
				ui.Text(c, shortcutLabel(a.settings.Shortcut)).FontSize(10).TextColor(muted)
			}
		}
	})
}

func (a *application) back() {
	a.cancelPending()
	switch {
	case a.saving:
		return
	case a.settingsOpen:
		a.cancelTranslationCheck()
		a.settingsOpen = false
		a.setRecording(false)
	case a.detail != nil:
		a.cancelTranslation()
		a.translationMode = false
		a.translationError = ""
		a.detail = nil
		a.previewID, a.previewImage, a.previewError = "", nil, ""
	default:
		a.hideWindow()
		return
	}
	a.message = ""
	if a.translationMode && a.detail != nil {
		a.editorHandle.Focus()
		return
	}
	a.historyHandle.Focus()
}
