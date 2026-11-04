package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var (
	ink          = ui.Hex("#363542")
	muted        = ui.Hex("#898697")
	accent       = ui.Hex("#8066D9")
	lavender     = ui.Hex("#F0ECFC")
	paper        = ui.Hex("#FCFBFE")
	hairline     = ui.Hex("#EAE7F0")
	selectedLine = ui.Hex("#DCD2F8")

	iconClipboard = vectorIcon(`<rect x="5" y="5" width="14" height="16" rx="3"/><rect x="9" y="3" width="6" height="4" rx="1.5"/><path d="M9 11h6M9 15h4"/>`)
	iconSettings  = vectorIcon(`<path d="m9 3-.6 2.1-2 .9-2-.6-2 3.4 1.5 1.6v2.3L2.4 14l2 3.5 2-.5 2 .9L9 20h4l.6-2.1 2-.9 2 .5 2-3.5-1.5-1.5v-2.3l1.5-1.6-2-3.4-2 .6-2-.9L13 3z"/><circle cx="11" cy="11.5" r="3"/>`)
	iconSearch    = vectorIcon(`<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/>`)
	iconStar      = vectorIcon(`<path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2-5.6-3-5.6 3 1.1-6.2L3 9.6l6.2-.9z"/>`)
	iconStarFill  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2-5.6-3-5.6 3 1.1-6.2L3 9.6l6.2-.9z"/></svg>`))
	iconText      = vectorIcon(`<path d="M5 6h14M12 6v13M8 19h8M5 6v3M19 6v3"/>`)
	iconImage     = vectorIcon(`<rect x="3" y="3" width="18" height="18" rx="4"/><circle cx="8" cy="8" r="1.5"/><path d="m4 17 5-5 4 4 3-6 5 7"/>`)
	iconFile      = vectorIcon(`<path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9zM14 3v6h6M8 14h8M8 17h5"/>`)
	iconBack      = vectorIcon(`<path d="m14 6-6 6 6 6"/>`)
	iconClose     = vectorIcon(`<path d="m6 6 12 12M6 18 18 6"/>`)
	iconPause     = vectorIcon(`<path d="M9 5v14M15 5v14"/>`)
	iconListen    = vectorIcon(`<circle cx="12" cy="12" r="3"/><path d="M6.3 6.3a8 8 0 0 0 0 11.4M17.7 6.3a8 8 0 0 1 0 11.4"/>`)
	iconTrash     = vectorIcon(`<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 10v7M14 10v7"/>`)
)

func vectorIcon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

func clipboardTheme() *ui.Theme {
	t := ui.LightTheme()
	t.Background, t.Surface = paper, ui.Hex("#F6F4FA")
	t.SurfaceHover, t.SurfacePressed = lavender, selectedLine
	t.Text, t.TextMuted, t.Border = ink, muted, hairline
	t.Accent, t.AccentHover, t.AccentPressed = accent, ui.Hex("#7256CE"), ui.Hex("#664BC2")
	t.Focus, t.Selection = accent.Alpha(.45), accent.Alpha(.2)
	t.Spacing, t.Radius, t.FontSize = 3, 8, 13
	return t
}

func (a *application) view(c *ui.Context) {
	c.SetTheme(clipboardTheme())
	c.Root().Background(ui.Transparent)
	if a.recording {
		a.recordShortcut(c)
	} else {
		c.OnShortcut(0, ui.KeyEscape, a.back)
		c.OnShortcut(ui.Cmd, ui.KeyW, a.hideWindow)
		c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)
		if !a.settingsOpen && a.detail == nil {
			c.OnShortcut(ui.Cmd, ui.KeyF, func() { a.cancelPending(); a.searchHandle.Focus() })
			c.OnShortcut(0, ui.KeyDown, func() { a.moveSelection(1) })
			c.OnShortcut(0, ui.KeyUp, func() { a.moveSelection(-1) })
			c.OnShortcut(0, ui.KeyEnter, a.pasteSelected)
			c.OnShortcut(ui.Cmd, ui.KeyE, a.editSelected)
			c.OnShortcut(ui.Cmd, ui.KeyD, a.favoriteSelected)
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
		a.mutate(func(s *historyStore) error { return s.clear() }, func() { a.selected = 0 })
	}
	if ui.AlertDialog(c, &a.confirmDelete, "删除这条记录？", "只删除剪贴板历史，不会删除原始文件。", "取消", "删除记录") == 1 && a.detail != nil {
		id := a.detail.ID
		a.mutate(func(s *historyStore) error { return s.remove(id) }, func() { a.detail = nil })
	}
}

func iconButton(c *ui.Context, label string, icon *ui.SVG, action func()) ui.Element {
	b := ui.ButtonBase(c).Size(28, 28).Radius(7).Center().TextColor(muted).Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(lavender).TextColor(accent)
	}
	return b.Children(func() { ui.Icon(c, icon).Size(17, 17) }).OnClick(action)
}

func (a *application) header(c *ui.Context) {
	ui.Row(c).Height(53).Shrink(0).Padding(0, 16).Gap(9).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(29, 29).Radius(9).Background(lavender).Center().Children(func() {
			ui.Icon(c, iconClipboard).Size(20, 20).TextColor(accent)
		})
		ui.Text(c, "Vitals").FontSize(16).FontWeight(650)
		ui.Text(c, "剪贴板").FontSize(11).TextColor(muted).Grow(1)
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
		iconButton(c, "设置", iconSettings, func() {
			a.cancelPending()
			a.settingsOpen = !a.settingsOpen
			a.setRecording(false)
			a.message = ""
		})
	})
}

func (a *application) messageView(c *ui.Context) {
	ui.Row(c).Margin(0, 13, 8).Padding(8, 10).Radius(8).Background(lavender).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.message).FontSize(11).TextColor(accent).Grow(1).MinWidth(0).MaxLines(3)
		iconButton(c, "关闭提示", iconClose, func() { a.message = "" })
	})
}

func (a *application) historyView(c *ui.Context) {
	ui.Row(c).Margin(0, 14, 10).Height(35).Shrink(0).Radius(9).Border(1, hairline).
		Background(ui.Hex("#F6F4F9")).Padding(0, 10).Gap(7).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconSearch).Size(16, 16).TextColor(muted)
		ui.TextInputBase(c.Key("search"), &a.query).Bind(&a.searchHandle).Grow(1).MinWidth(0).Height(33).
			Placeholder("搜索文字、文件或来源应用…").Label("搜索剪贴板").
			OnChange(func() { a.cancelPending(); a.selected = 0; a.list.ScrollTo(0, ui.Start) }).
			OnSubmit(a.pasteSelected)
		if a.query != "" {
			iconButton(c, "清除搜索", iconClose, func() {
				a.cancelPending()
				a.query = ""
				a.selected = 0
				a.searchHandle.Focus()
			}).Size(22, 22)
		} else {
			ui.Text(c, "⌘ F").FontSize(10).TextColor(muted)
		}
	})
	ui.Row(c).Margin(0, 14, 8).Height(29).Shrink(0).Gap(5).AlignItems(ui.Center).Children(func() {
		for tab, label := range []string{"全部", "文字", "文件", "收藏"} {
			button := ui.ButtonBase(c.Key(label)).Padding(5, 11).Radius(8).Gap(5).
				Label(label).TextColor(muted).AlignItems(ui.Center)
			if a.tab == tab {
				button.Background(lavender).TextColor(accent)
			} else if button.Hovered() {
				button.Background(ui.Hex("#F5F2FA"))
			}
			button.Children(func() {
				ui.Text(c, label).FontSize(12).FontWeight(550)
				if a.tab == tab {
					ui.Text(c, fmt.Sprint(len(filterHistory(a.entries, tab, "")))).FontSize(10).TextColor(accent.Alpha(.75))
				}
			}).OnClick(func() {
				a.cancelPending()
				a.tab = tab
				a.selected = 0
				a.list.ScrollTo(0, ui.Start)
			})
		}
	})
	items := filterHistory(a.entries, a.tab, a.query)
	a.selected = max(0, min(a.selected, len(items)-1))
	ui.Row(c).Padding(0, 18, 7).Shrink(0).Children(func() {
		label := "最近复制"
		if a.query != "" {
			label = fmt.Sprintf("找到 %d 条记录", len(items))
		} else if a.tab == 3 {
			label = "我的收藏"
		}
		ui.Text(c, label).FontSize(10).TextColor(muted).Grow(1)
		ui.Text(c, "最新在前").FontSize(10).TextColor(muted)
	})
	if len(items) == 0 {
		ui.Column(c).Grow(1).Center().Gap(9).Children(func() {
			ui.Box(c).Size(49, 49).Radius(14).Background(lavender).Center().Children(func() {
				ui.Icon(c, iconClipboard).Size(26, 26).TextColor(accent.Alpha(.7))
			})
			title, help := "复制一点灵感，从这里找回", "复制的文字、图片和文件会自动出现在这里"
			switch {
			case a.loading:
				title, help = "正在读取历史记录", "稍等片刻"
			case a.storageFailed:
				title, help = "历史记录暂时不可用", "原始数据已保留，请检查上方提示"
			case a.query != "":
				title, help = "没有找到匹配内容", "换个关键词，或试试「全部」"
			case a.tab == 3:
				title, help = "把常用内容留在手边", "点击记录右侧的星标即可收藏"
			case a.tab == 2:
				title, help = "还没有文件记录", "复制一张图片，或在 Finder 中复制文件"
			case a.settings.Paused:
				title, help = "监听已暂停", "点击右上角「已暂停」恢复"
			}
			ui.Text(c, title).FontSize(13).FontWeight(550)
			ui.Text(c, help).FontSize(11).TextColor(muted)
		})
		return
	}
	ui.List(c.Key("history"), &a.list, len(items)).
		Grow(1).MinHeight(0).Padding(0, 10, 10).Gap(4).
		ItemKey(func(i int) any { return items[i].ID }).
		Rows(func(row ui.ListRow) { a.entryRow(row.Context, items[row.Index], row.Index) })
}

func (a *application) entryRow(c *ui.Context, entry clipEntry, index int) {
	active := index == a.selected
	double := false
	row := ui.Row(c).Height(62).Radius(10).Border(1, ui.Transparent).AlignItems(ui.Center).Padding(0, 9, 0, 0)
	if active {
		row.Background(lavender).BorderColor(selectedLine)
	}
	row.Children(func() {
		button := ui.ButtonBase(c.Key("content")).Grow(1).MinWidth(0).Height(60).Padding(8, 10).
			Gap(11).AlignItems(ui.Center).Label("记录 " + entry.ID).Radius(9)
		if button.Hovered() && !active {
			row.Background(ui.Hex("#F6F3FC"))
		}
		button.Children(func() {
			a.entryIcon(c, entry)
			ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
				ui.Text(c, entry.title()).FontSize(13).FontWeight(550).SingleLine()
				source := entry.Source
				if source == "" {
					source = "剪贴板"
				}
				ui.Text(c, source+" · "+entry.CopiedAt.Local().Format("2006-01-02 15:04:05")).
					FontSize(10).TextColor(muted).SingleLine()
			})
		})
		double = button.DoubleClicked()
		button.OnClick(func() { a.clickEntry(entry, index) })
		star, label := iconStar, "收藏 "
		if entry.Favorite {
			star, label = iconStarFill, "取消收藏 "
		}
		limit := a.settings.HistoryLimit
		control := iconButton(c, label+entry.ID, star, func() {
			a.mutate(func(s *historyStore) error { return s.favorite(entry.ID, limit) }, nil)
		})
		if entry.Favorite {
			control.TextColor(accent)
		}
		control.Tooltip(strings.TrimSpace(label))
	})
	// MyGo reports the second press before the release that produces Clicked.
	// Handle it in this build so the first click's pending paste is cancelled.
	if double {
		a.openEntry(entry)
	}
}

func (a *application) entryIcon(c *ui.Context, entry clipEntry) {
	ui.Box(c).Size(36, 36).Shrink(0).Radius(10).Background(ui.Hex("#FFFFFF")).Border(1, hairline).Center().Clip().Children(func() {
		icon, tint := iconText, ui.Hex("#A094BE")
		if entry.Kind == kindImage || entry.Kind == kindFiles && len(entry.Files) == 1 && isImagePath(entry.Files[0]) {
			a.requestImage(entry, false)
			if bitmap := a.images[entry.ID]; bitmap != nil {
				ui.Image(c, bitmap).Size(34, 34).Fit(ui.Cover)
				return
			}
			icon, tint = iconImage, ui.Hex("#C79972")
		} else if entry.Kind == kindFiles {
			icon, tint = iconFile, ui.Hex("#90A0B5")
		}
		ui.Icon(c, icon).Size(22, 22).TextColor(tint)
	})
}

func (a *application) detailView(c *ui.Context) {
	entry := *a.detail
	ui.Row(c).Padding(0, 14, 10).Gap(5).AlignItems(ui.Center).Children(func() {
		iconButton(c, "返回历史", iconBack, a.back)
		title := "编辑文字"
		if entry.Kind != kindText {
			title = "预览内容"
		}
		ui.Text(c, title).FontSize(13).FontWeight(600).Grow(1)
		iconButton(c, "删除记录", iconTrash, func() { a.confirmDelete = true })
	})
	if entry.Kind == kindText {
		c.OnShortcut(ui.Cmd, ui.KeyS, func() { a.saveEdit(false) })
		c.OnShortcut(ui.Cmd, ui.KeyEnter, func() { a.saveEdit(true) })
		ui.TextArea(c.Key("editor"), &a.editDraft).Bind(&a.editorHandle).Grow(1).MinHeight(0).
			Margin(0, 16, 10).Padding(12).FontSize(13).Background(ui.Hex("#FFFFFF")).
			Label("编辑内容").Disabled(a.saving)
		ui.Row(c).Padding(0, 16, 14).AlignItems(ui.Center).Gap(7).Children(func() {
			ui.Text(c, fmt.Sprintf("%d 字符", len([]rune(a.editDraft)))).FontSize(10).TextColor(muted).Grow(1)
			ui.Button(c, "取消").Disabled(a.saving).OnClick(a.back)
			ui.Button(c, "保存").Disabled(a.saving || a.editDraft == "").OnClick(func() { a.saveEdit(false) })
			ui.PrimaryButton(c, "保存并粘贴").Disabled(a.saving || a.editDraft == "").OnClick(func() { a.saveEdit(true) })
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
		} else if entry.Kind == kindImage {
			ui.Box(c).Height(230).Center().Children(func() { ui.Spinner(c) })
		}
		ui.Text(c, entry.title()).FontSize(14).FontWeight(600)
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

func (a *application) settingsView(c *ui.Context) {
	ui.Row(c).Padding(0, 14, 8).Gap(5).AlignItems(ui.Center).Children(func() {
		iconButton(c, "返回剪贴板", iconBack, a.back)
		ui.Text(c, "设置").FontSize(14).FontWeight(600)
	})
	ui.Scroll(c).Grow(1).MinHeight(0).Padding(2, 18, 16).Gap(18).Children(func() {
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, "打开剪贴板").FontSize(12).FontWeight(600)
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c.Key("shortcut"), &a.shortcutDraft).Grow(1).MinWidth(0).
					Label("全局快捷键").Disabled(a.recording)
				label := "录制"
				if a.recording {
					label = "请按快捷键…"
				}
				ui.Button(c, label).OnClick(func() { a.cancelPending(); a.setRecording(!a.recording) })
			})
			ui.Text(c, "点击录制后按下组合键，或直接输入 Command+Shift+V").FontSize(10).TextColor(muted)
			ui.Row(c).Gap(8).Children(func() {
				ui.PrimaryButton(c, "保存快捷键").Disabled(a.recording).OnClick(a.applyShortcut)
				ui.Button(c, "恢复默认").OnClick(func() { a.shortcutDraft = defaultShortcut; a.applyShortcut() })
			})
			if a.shortcutActive != "" {
				ui.Text(c, "当前快捷键："+shortcutLabel(a.shortcutActive)).FontSize(10).TextColor(muted)
			}
		})
		ui.Divider(c)
		ui.Column(c).Gap(8).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "自动粘贴").FontSize(12).FontWeight(600).Grow(1)
				status := "需要辅助功能权限"
				if a.trusted {
					status = "已允许"
				}
				ui.Text(c, status).FontSize(11).TextColor(accent)
			})
			ui.Text(c, "选中历史内容后，自动回到之前的应用并粘贴。").FontSize(11).TextColor(muted)
			if !a.trusted {
				services := c.Services()
				ui.Button(c, "打开辅助功能设置").OnClick(func() { services.OpenURL(accessibilityURL) })
			}
		})
		ui.Divider(c)
		ui.Column(c).Gap(11).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "保留历史").FontSize(12).Grow(1)
				ui.Select(c.Key("limit"), &a.limitChoice, []string{"100", "500", "1000"}).Width(90).Label("保留条数").OnChange(func() {
					limit, _ := strconv.Atoi(a.limitChoice)
					a.settings.HistoryLimit = limit
					a.persistSettings()
					a.mutate(func(s *historyStore) error { return s.commit(trimHistory(s.entries, limit)) }, nil)
				})
				ui.Text(c, "条").FontSize(11).TextColor(muted).Margin(0, 0, 0, 7)
			})
			ui.Text(c, "收藏不受条数限制；所有内容仅保存在这台 Mac。").FontSize(10).TextColor(muted)
			ui.Checkbox(c, &a.settings.OpenAtLogin, "登录时启动").OnChange(a.setOpenAtLogin)
		})
		ui.Divider(c)
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, "清空历史").TextColor(c.Theme().Danger).OnClick(func() { a.confirmClear = true })
			ui.Spacer(c)
			ui.Button(c, "退出 Vitals").OnClick(mygo.App.Quit)
		})
	})
}

func (a *application) footer(c *ui.Context) {
	ui.Row(c).Height(32).Shrink(0).Padding(0, 16).BorderWidth(1, 0, 0, 0).BorderColor(hairline).
		Gap(10).AlignItems(ui.Center).Children(func() {
		if a.settingsOpen {
			ui.Text(c, "Esc 返回").FontSize(10).TextColor(muted).Grow(1)
			ui.Text(c, "Vitals · 本地剪贴板").FontSize(10).TextColor(muted)
		} else if a.detail != nil {
			ui.Text(c, "Esc 返回").FontSize(10).TextColor(muted).Grow(1)
			if a.detail.Kind == kindText {
				ui.Text(c, "⌘ S 保存").FontSize(10).TextColor(muted)
			}
		} else {
			ui.Text(c, "↑ ↓ 选择").FontSize(10).TextColor(muted)
			ui.Text(c, "↵ 粘贴").FontSize(10).TextColor(muted)
			ui.Text(c, "双击编辑 / 预览").FontSize(10).TextColor(muted).Grow(1)
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
		a.settingsOpen = false
		a.setRecording(false)
	case a.detail != nil:
		a.detail = nil
		a.previewID, a.previewImage, a.previewError = "", nil, ""
	default:
		a.hideWindow()
		return
	}
	a.message = ""
	a.searchHandle.Focus()
}

func (a *application) moveSelection(delta int) {
	a.cancelPending()
	items := filterHistory(a.entries, a.tab, a.query)
	a.selected = max(0, min(a.selected+delta, len(items)-1))
	a.list.ScrollIntoView(a.selected)
}

func (a *application) selectedEntry() (clipEntry, bool) {
	items := filterHistory(a.entries, a.tab, a.query)
	if a.selected < 0 || a.selected >= len(items) {
		return clipEntry{}, false
	}
	return items[a.selected], true
}

func (a *application) pasteSelected() {
	if entry, ok := a.selectedEntry(); ok {
		a.pasteEntry(entry)
	}
}

func (a *application) editSelected() {
	if entry, ok := a.selectedEntry(); ok {
		a.openEntry(entry)
	}
}

func (a *application) favoriteSelected() {
	if entry, ok := a.selectedEntry(); ok {
		limit := a.settings.HistoryLimit
		a.mutate(func(s *historyStore) error { return s.favorite(entry.ID, limit) }, nil)
	}
}
