package app

import (
	"runtime"
	"strconv"

	"vitals/internal/clipboard"
	"vitals/internal/config"
	"vitals/internal/translation"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *application) settingsView(c *ui.Context) {
	ui.Row(c).Margin(0, 16, 12).Padding(3).Height(34).Shrink(0).
		Radius(9).Background(ui.Hex("#F1EEF7")).Gap(3).Children(func() {
		for i, label := range []string{"通用", "翻译"} {
			button := ui.ButtonBase(c.Key(label)).Label(label).Grow(1).Height(28).Radius(7).Center()
			if a.settingsTab == i {
				button.Background(ui.Hex("#FFFFFF")).TextColor(accent)
			} else {
				button.TextColor(muted)
			}
			button.Children(func() { ui.Text(c, label).FontSize(12).FontWeight(600) }).
				OnClick(func() {
					a.setRecording(false)
					if i != 1 {
						a.cancelTranslationCheck()
					}
					a.settingsTab = i
				})
		}
	})
	if a.settingsTab == 1 {
		a.translationSettingsView(c)
		return
	}
	ui.Scroll(c.Key("general-settings")).Grow(1).MinHeight(0).Padding(0, 16, 14).Gap(10).Children(func() {
		settingsGroup(c, func() {
			ui.Text(c, "打开剪贴板").FontSize(12).FontWeight(600)
			ui.Row(c).Gap(7).Children(func() {
				ui.TextInput(c.Key("shortcut"), &a.shortcutDraft).Grow(1).MinWidth(0).Height(31).
					Label("全局快捷键").Disabled(a.recording)
				label := "录制"
				if a.recording {
					label = "请按快捷键…"
				}
				ui.Button(c, label).Height(31).OnClick(func() { a.cancelPending(); a.setRecording(!a.recording) })
			})
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				label := "点击录制，或直接输入组合键"
				if a.shortcutActive != "" {
					label = "当前 " + shortcutLabel(a.shortcutActive)
				}
				ui.Text(c, label).FontSize(10).TextColor(muted).Grow(1)
				ui.Button(c, "恢复默认").Height(28).OnClick(func() { a.shortcutDraft = config.DefaultShortcut; a.applyShortcut() })
				ui.PrimaryButton(c, "保存快捷键").Height(28).Disabled(a.recording).OnClick(a.applyShortcut)
			})
		})
		settingsGroup(c, func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "自动粘贴").FontSize(12).FontWeight(600).Grow(1)
				status := "待授权"
				if a.trusted {
					status = "已允许"
				}
				if runtime.GOOS == "windows" {
					status = "已启用"
				}
				ui.Text(c, status).FontSize(11).TextColor(accent)
			})
			ui.Text(c, "点击记录或按 1–9，回到之前的应用并粘贴。").FontSize(11).TextColor(muted)
			if runtime.GOOS == "darwin" && !a.trusted {
				services := c.Services()
				ui.Button(c, "打开辅助功能设置").Height(29).OnClick(func() { services.OpenURL(accessibilityURL) })
			}
		})
		settingsGroup(c, func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "保留历史").FontSize(12).FontWeight(600).Grow(1)
				ui.Select(c.Key("limit"), &a.limitChoice, []string{"100", "500", "1000"}).Width(86).Height(29).Label("保留条数").OnChange(func() {
					limit, _ := strconv.Atoi(a.limitChoice)
					a.settings.HistoryLimit = limit
					a.persistSettings()
					a.mutate(func(s *clipboard.Store) error { return s.SetLimit(limit) }, nil)
				})
				ui.Text(c, "条").FontSize(11).TextColor(muted).Margin(0, 0, 0, 7)
			})
			ui.Text(c, "收藏永久保留，历史记录存储在本机。").FontSize(10).TextColor(muted)
			ui.Divider(c).Margin(2, 0)
			ui.Checkbox(c, &a.settings.OpenAtLogin, "登录时启动").OnChange(a.setOpenAtLogin)
		})
		ui.Row(c).Gap(8).Padding(2, 0).Children(func() {
			ui.Button(c, "清空历史").Height(30).TextColor(c.Theme().Danger).OnClick(func() { a.confirmClear = true })
			ui.Spacer(c)
			ui.Button(c, "退出应用").Height(30).OnClick(mygo.App.Quit)
		})
	})
}

func settingsGroup(c *ui.Context, children func()) {
	ui.Column(c).Padding(12).Gap(9).Radius(10).Border(1, hairline).Background(ui.Hex("#FFFFFF")).Children(children)
}

func (a *application) translationSettingsView(c *ui.Context) {
	ui.Scroll(c.Key("translation-settings")).Grow(1).MinHeight(0).Padding(0, 16, 10).Gap(10).Children(func() {
		settingsGroup(c, func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Column(c).Gap(3).Grow(1).Children(func() {
					ui.Text(c, "腾讯交互翻译").FontSize(13).FontWeight(600)
					ui.Text(c, "TranSmart API").FontSize(10).TextColor(muted)
				})
				services := c.Services()
				ui.Button(c, "获取凭据").Height(28).OnClick(func() { services.OpenURL("https://transmart.qq.com/") })
			})
			ui.Divider(c).Margin(2, 0)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "User").FontSize(11).Width(40)
				ui.TextInput(c.Key("transmart-user"), &a.translationDraft.User).Grow(1).MinWidth(0).Height(32).
					Label("开发者 User").Placeholder("开发者账号 User").OnChange(a.changeTranslationDraft)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Token").FontSize(11).Width(40)
				ui.TextInput(c.Key("transmart-token"), &a.translationDraft.Token).Password().Grow(1).MinWidth(0).Height(32).
					Label("开发者 Token").Placeholder("粘贴 API Token").OnChange(a.changeTranslationDraft)
			})
			ui.Text(c, "在腾讯交互翻译的个人中心获取 User 和 Token。").FontSize(10).TextColor(muted)
		})
		settingsGroup(c, func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "目标语言").FontSize(12).FontWeight(600).Grow(1)
				a.translationLanguageSelect(c)
			})
			ui.Text(c, "自动互译：中文 → 英文，其他语言 → 简体中文。").FontSize(10).TextColor(muted)
			ui.Text(c, "单次最多 5,000 字符，译文可继续编辑。").FontSize(10).TextColor(muted)
		})
		ui.Column(c).Padding(2, 4).Gap(6).Children(func() {
			ui.Text(c, "主动翻译时，所选原文会发送到腾讯交互翻译。").FontSize(10).TextColor(muted)
			ui.Text(c, "连接测试仅发送示例文字；凭据保存在本机设置中。").FontSize(10).TextColor(muted)
		})
	})
	ui.Column(c).Padding(0, 16, 12).Gap(8).Shrink(0).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(7).Children(func() {
			if a.checkingTranslation {
				ui.Spinner(c).Size(14, 14)
			}
			status := a.translationStatus
			if status == "" {
				status = "修改后保存，下一次翻译生效"
			}
			ui.Text(c, status).FontSize(10).TextColor(muted).Grow(1).MinWidth(0).MaxLines(3)
		})
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, "测试连接").Height(31).Disabled(a.checkingTranslation).OnClick(a.testTranslationConnection)
			ui.Spacer(c)
			ui.PrimaryButton(c, "保存翻译配置").Height(31).OnClick(a.saveTranslationSettings)
		})
	})
}

func (a *application) translationLanguageSelect(c *ui.Context) {
	options := []struct{ value, label string }{
		{translation.TargetAuto, "自动互译"},
		{translation.TargetChinese, "简体中文"},
		{translation.TargetEnglish, "英文"},
	}
	sel := ui.SelectBase(c.Key("target-language"), &a.translationDraft.TargetLanguage)
	sel.Trigger.Width(160).Height(30).Padding(0, 10).Radius(7).Border(1, hairline).
		Background(ui.Hex("#FFFFFF")).AlignItems(ui.Center).Gap(10).Label("选择目标语言").
		OnChange(a.changeTranslationDraft).Children(func() {
		for _, option := range options {
			if option.value == a.translationDraft.TargetLanguage {
				ui.Text(c, option.label).FontSize(11).Grow(1)
			}
		}
		ui.Text(c, "⌄").FontSize(12).TextColor(muted)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Padding(4).Radius(8).Border(1, hairline).Background(ui.Hex("#FFFFFF"))
		for _, option := range options {
			item := sel.Item(option.value).Padding(7, 10).Radius(5).Label(option.label)
			if item.Highlighted() {
				item.Background(lavender).TextColor(accent)
			}
			item.Children(func() { ui.Text(c, option.label).FontSize(11) })
		}
	})
}
