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
	ui.Scroll(c.Key("general-settings")).Grow(1).MinHeight(0).Padding(0, 16, 10).Gap(10).Children(func() {
		a.shortcutSettingsGroup(c, false)
		settingsGroup(c, func() {
			ui.Row(c).Height(28).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "应用行为").FontSize(12).FontWeight(600).Grow(1)
				ui.Checkbox(c, &a.settings.OpenAtLogin, "登录时启动").OnChange(a.setOpenAtLogin)
			})
			ui.Divider(c)
			ui.Row(c).Height(30).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "自动粘贴").FontSize(11).Grow(1)
				status := "待授权"
				if a.trusted {
					status = "已允许"
				}
				if runtime.GOOS == "windows" {
					status = "已启用"
				}
				ui.Text(c, status).FontSize(11).TextColor(accent)
				if runtime.GOOS == "darwin" && !a.trusted {
					services := c.Services()
					ui.Button(c, "打开辅助功能设置").Height(28).OnClick(func() { services.OpenURL(accessibilityURL) })
				}
			})
			ui.Text(c, "点击记录或按 1–9，回到原应用粘贴。").FontSize(10).TextColor(muted)
		})
		settingsGroup(c, func() {
			ui.Row(c).Height(24).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "历史记录").FontSize(12).FontWeight(600).Grow(1)
				ui.ButtonBase(c).Label("清空历史").Height(24).Padding(0, 4).TextColor(c.Theme().Danger).
					Children(func() { ui.Text(c, "清空历史").FontSize(11) }).
					OnClick(func() { a.confirmClear = true })
			})
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "保留历史").FontSize(11).Width(64)
				ui.Select(c.Key("limit"), &a.limitChoice, []string{"100", "500", "1000"}).Width(86).Height(30).Label("保留条数").OnChange(func() {
					limit, _ := strconv.Atoi(a.limitChoice)
					a.settings.HistoryLimit = limit
					a.persistSettings()
					a.mutate(func(s *clipboard.Store) error { return s.SetLimit(limit) }, nil)
				})
				ui.Text(c, "条").FontSize(11).TextColor(muted)
			})
			ui.Text(c, "记录仅保存在本机；收藏不受条数限制，清空时保留。").FontSize(10).TextColor(muted)
		})
	})
	status := a.shortcutStatus
	if status == "" {
		status = "快捷键需保存，其他设置即时生效"
	}
	settingsActions(c, status, false, func() {
		ui.Button(c, "退出应用").Height(32).OnClick(mygo.App.Quit)
		ui.Spacer(c)
		ui.PrimaryButton(c, "保存快捷键").Height(32).Disabled(a.recording).OnClick(a.applyShortcut)
	})
}

func settingsGroup(c *ui.Context, children func()) {
	ui.Column(c).Padding(12).Gap(8).Radius(10).Border(1, hairline).Background(ui.Hex("#FFFFFF")).Shrink(0).Children(children)
}

func (a *application) shortcutSettingsGroup(c *ui.Context, forTranslation bool) {
	title, label, key := "打开剪贴板", "全局快捷键", "shortcut"
	draft, active, errorText, defaultValue := &a.shortcutDraft, a.shortcutActive, a.shortcutError, config.DefaultShortcut
	if forTranslation {
		title, label, key = "划词翻译", "划词翻译快捷键", "translation-shortcut"
		draft, active, errorText, defaultValue = &a.translationShortcutDraft, a.translationShortcutActive, a.translationShortcutError, config.DefaultTranslationShortcut
	}
	recording := a.recording && a.recordingTranslation == forTranslation
	settingsGroup(c, func() {
		ui.Row(c).Height(24).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(12).FontWeight(600).Grow(1)
			ui.ButtonBase(c).Label("恢复默认").Height(24).Padding(0, 4).TextColor(accent).Disabled(a.recording).
				Children(func() { ui.Text(c, "恢复默认").FontSize(11) }).
				OnClick(func() {
					*draft = defaultValue
					if forTranslation {
						a.translationStatus = "已恢复默认组合，保存后生效"
					} else {
						a.shortcutStatus = "已恢复默认组合，保存后生效"
					}
				})
		})
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c.Key(key), draft).Grow(1).MinWidth(0).Height(32).
				Label(label).Disabled(recording).OnChange(func() {
				if forTranslation {
					a.translationStatus = ""
				} else {
					a.shortcutStatus = ""
				}
			})
			recordLabel := "录制"
			if recording {
				recordLabel = "取消录制"
			}
			ui.Button(c, recordLabel).Width(76).Height(32).OnClick(func() { a.toggleRecording(forTranslation) })
		})
		hint := "未启用 · 保存后生效"
		if active != "" {
			hint = "当前 " + shortcutLabel(active)
			if forTranslation {
				hint += " · 在其他应用中选中文字后使用"
			}
		}
		if recording {
			hint = "请按下组合键，Esc 取消"
		}
		if errorText != "" && !recording {
			ui.Text(c, errorText).FontSize(10).TextColor(c.Theme().Danger).MaxLines(2)
		} else {
			ui.Text(c, hint).FontSize(10).TextColor(muted).MaxLines(2)
		}
	})
}

func (a *application) translationSettingsView(c *ui.Context) {
	ui.Scroll(c.Key("translation-settings")).Grow(1).MinHeight(0).Padding(0, 16, 10).Gap(10).Children(func() {
		a.shortcutSettingsGroup(c, true)
		settingsGroup(c, func() {
			ui.Row(c).Height(24).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "腾讯交互翻译").FontSize(12).FontWeight(600).Grow(1)
				ui.Text(c, "TranSmart API").FontSize(10).TextColor(muted)
				services := c.Services()
				ui.ButtonBase(c).Label("获取凭据").Height(24).Padding(0, 4).TextColor(accent).
					Children(func() { ui.Text(c, "获取凭据").FontSize(11) }).
					OnClick(func() { services.OpenURL("https://transmart.qq.com/") })
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "User").FontSize(11).Width(64)
				ui.TextInput(c.Key("transmart-user"), &a.translationDraft.User).Grow(1).MinWidth(0).Height(32).
					Label("开发者 User").Placeholder("开发者账号 User").OnChange(a.changeTranslationDraft)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Token").FontSize(11).Width(64)
				ui.TextInput(c.Key("transmart-token"), &a.translationDraft.Token).Password().Grow(1).MinWidth(0).Height(32).
					Label("开发者 Token").Placeholder("粘贴 API Token").OnChange(a.changeTranslationDraft)
			})
		})
		settingsGroup(c, func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "目标语言").FontSize(12).FontWeight(600).Grow(1)
				a.translationLanguageSelect(c)
			})
			ui.Text(c, "自动互译：中文 → 英文，其他语言 → 简体中文。").FontSize(10).TextColor(muted)
		})
		ui.Column(c).Padding(0, 4).Gap(3).Shrink(0).Children(func() {
			ui.Text(c, "凭据保存在本机，主动翻译时原文发送至腾讯。").FontSize(10).TextColor(muted)
			ui.Text(c, "单次最多 5,000 字符；连接测试仅发送示例文字。").FontSize(10).TextColor(muted)
		})
	})
	status := a.translationStatus
	if status == "" {
		status = "快捷键、凭据与目标语言统一保存后生效"
	}
	settingsActions(c, status, a.checkingTranslation, func() {
		ui.Button(c, "测试连接").Height(32).Disabled(a.checkingTranslation || a.recording).OnClick(a.testTranslationConnection)
		ui.Spacer(c)
		ui.PrimaryButton(c, "保存翻译配置").Height(32).Disabled(a.recording).OnClick(a.saveTranslationSettings)
	})
}

func settingsActions(c *ui.Context, status string, busy bool, buttons func()) {
	ui.Column(c).Padding(10, 16, 12).Gap(8).Shrink(0).BorderWidth(1, 0, 0, 0).BorderColor(hairline).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(7).Children(func() {
			if busy {
				ui.Spinner(c).Size(14, 14)
			}
			ui.Text(c, status).FontSize(10).TextColor(muted).Grow(1).MinWidth(0).MaxLines(2)
		})
		ui.Row(c).Gap(8).Children(buttons)
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
