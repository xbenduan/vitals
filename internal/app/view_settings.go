package app

import (
	"strconv"

	"vitals/internal/clipboard"
	"vitals/internal/config"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

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
				ui.Button(c, "恢复默认").OnClick(func() { a.shortcutDraft = config.DefaultShortcut; a.applyShortcut() })
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
					a.mutate(func(s *clipboard.Store) error { return s.SetLimit(limit) }, nil)
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
