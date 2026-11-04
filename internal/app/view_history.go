package app

import (
	"fmt"
	"strings"

	"clipnest/internal/clipboard"

	"github.com/egoist/mygo/ui"
)

var historyTabs = [...]string{"全部", "文字", "文件", "收藏"}

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
			ui.Text(c, primaryShortcutLabel("F")).FontSize(10).TextColor(muted)
		}
	})
	ui.Row(c).Margin(0, 14, 8).Height(29).Shrink(0).Gap(5).AlignItems(ui.Center).Children(func() {
		for tab, label := range historyTabs {
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
					ui.Text(c, fmt.Sprint(len(clipboard.Filter(a.entries, tab, "")))).FontSize(10).TextColor(accent.Alpha(.75))
				}
			}).OnClick(func() { a.selectHistoryTab(tab) })
		}
	})
	items := clipboard.Filter(a.entries, a.tab, a.query)
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
				title, help = "还没有文件记录", "复制一张图片，或从文件夹中复制文件"
			case a.settings.Paused:
				title, help = "监听已暂停", "点击右上角「已暂停」恢复"
			}
			ui.Text(c, title).FontSize(13).FontWeight(550)
			ui.Text(c, help).FontSize(11).TextColor(muted)
		})
		return
	}
	ui.List(c.Key("history"), &a.list, len(items)).
		Bind(&a.historyHandle).Focusable().
		Grow(1).MinHeight(0).Padding(0, 10, 10).Gap(4).
		ItemKey(func(i int) any { return items[i].ID }).
		Rows(func(row ui.ListRow) { a.entryRow(row.Context, items[row.Index], row.Index) })
}

func (a *application) entryRow(c *ui.Context, entry clipboard.Entry, index int) {
	active := index == a.selected
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
				ui.Text(c, entry.Title()).FontSize(13).FontWeight(550).SingleLine()
				source := entry.Source
				if source == "" {
					source = "剪贴板"
				}
				ui.Text(c, source+" · "+entry.CopiedAt.Local().Format("2006-01-02 15:04:05")).
					FontSize(10).TextColor(muted).SingleLine()
			})
			if index < 9 {
				ui.Box(c).Size(18, 20).Shrink(0).Radius(5).Background(ui.Hex("#FFFFFF").Alpha(.7)).
					Center().Children(func() {
					ui.Text(c, fmt.Sprint(index+1)).FontSize(10).TextColor(muted)
				})
			}
		})
		button.OnClick(func() { a.clickEntry(entry, index) })
		star, label := iconStar, "收藏 "
		if entry.Favorite {
			star, label = iconStarFill, "取消收藏 "
		}
		limit := a.settings.HistoryLimit
		control := iconButton(c, label+entry.ID, star, func() {
			a.mutate(func(s *clipboard.Store) error { return s.ToggleFavorite(entry.ID, limit) }, nil)
		})
		if entry.Favorite {
			control.TextColor(accent)
		}
		control.Tooltip(strings.TrimSpace(label))
	})
	row.ContextMenu(func(m *ui.Menu) {
		label := "编辑"
		if entry.Kind != clipboard.KindText {
			label = "预览"
		}
		if m.Item(label).Chosen() {
			a.openEntry(entry)
		}
		if entry.Kind == clipboard.KindText && m.Item("翻译").Chosen() {
			a.translateEntry(entry)
		}
		m.Separator()
		if m.Item("删除").Chosen() {
			a.cancelPending()
			a.deleteID = entry.ID
			a.confirmDelete = true
		}
	})
}

func (a *application) entryIcon(c *ui.Context, entry clipboard.Entry) {
	ui.Box(c).Size(36, 36).Shrink(0).Radius(10).Background(ui.Hex("#FFFFFF")).Border(1, hairline).Center().Clip().Children(func() {
		icon, tint := iconText, ui.Hex("#A094BE")
		if entry.Kind == clipboard.KindImage || entry.Kind == clipboard.KindFiles && len(entry.Files) == 1 && isImagePath(entry.Files[0]) {
			a.requestImage(entry, false)
			if bitmap := a.images[entry.ID]; bitmap != nil {
				ui.Image(c, bitmap).Size(34, 34).Fit(ui.Cover)
				return
			}
			icon, tint = iconImage, ui.Hex("#C79972")
		} else if entry.Kind == clipboard.KindFiles {
			icon, tint = iconFile, ui.Hex("#90A0B5")
		}
		ui.Icon(c, icon).Size(22, 22).TextColor(tint)
	})
}

func (a *application) selectHistoryTab(tab int) {
	a.cancelPending()
	a.tab = (tab + len(historyTabs)) % len(historyTabs)
	a.selected = 0
	a.list.ScrollTo(0, ui.Start)
	a.historyHandle.Focus()
}

func (a *application) moveSelection(delta int) {
	a.cancelPending()
	items := clipboard.Filter(a.entries, a.tab, a.query)
	a.selected = max(0, min(a.selected+delta, len(items)-1))
	a.list.ScrollIntoView(a.selected)
	a.historyHandle.Focus()
}

func (a *application) pasteNumber(index int) {
	items := clipboard.Filter(a.entries, a.tab, a.query)
	if index >= 0 && index < len(items) {
		a.clickEntry(items[index], index)
	}
}

func (a *application) selectedEntry() (clipboard.Entry, bool) {
	items := clipboard.Filter(a.entries, a.tab, a.query)
	if a.selected < 0 || a.selected >= len(items) {
		return clipboard.Entry{}, false
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
		a.mutate(func(s *clipboard.Store) error { return s.ToggleFavorite(entry.ID, limit) }, nil)
	}
}
