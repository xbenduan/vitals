package app

import (
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
				OnClick(func() { a.setRecording(false); a.settingsTab = i })
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
				ui.Text(c, status).FontSize(11).TextColor(accent)
			})
			ui.Text(c, "点击记录或按 1–9，回到之前的应用并粘贴。").FontSize(11).TextColor(muted)
			if !a.trusted {
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
			ui.Text(c, "收藏永久保留，历史记录存储在这台 Mac。").FontSize(10).TextColor(muted)
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
				ui.Text(c, "模型服务").FontSize(12).FontWeight(600).Grow(1)
				ui.Select(c.Key("provider"), &a.translationDraft.Provider, []string{translation.Ollama, translation.OpenAI}).
					Width(195).Height(30).Label("翻译服务").OnChange(a.changeProvider)
			})
			ui.TextInput(c.Key("base-url"), &a.translationDraft.BaseURL).Height(31).
				Label("服务地址").Placeholder("服务地址").OnChange(a.changeTranslationDraft)
			help := "Ollama 默认在本机运行，无需 API Key"
			if a.translationDraft.Provider == translation.OpenAI {
				help = "兼容 OpenAI 的服务地址需包含 /v1 等 API 前缀"
			}
			ui.Text(c, help).FontSize(10).TextColor(muted)
			if a.translationDraft.Provider == translation.OpenAI {
				ui.TextInput(c.Key("api-key"), &a.translationDraft.APIKey).Password().Height(31).
					Label("API Key").Placeholder("API Key（本地服务可留空）").OnChange(a.changeTranslationDraft)
			}
			ui.Row(c).Gap(7).Children(func() {
				ui.TextInput(c.Key("model"), &a.translationDraft.Model).Grow(1).MinWidth(0).Height(31).
					Label("模型名称").Placeholder("模型名称，或点击检测").OnChange(func() {
					a.cancelModelDetection()
					a.modelStatus = ""
				})
				ui.Button(c, "检测模型").Height(31).Disabled(a.detectingModels).OnClick(func() { a.detectModels(false) })
			})
			if len(a.models) > 0 {
				ui.Select(c.Key("detected-model"), &a.translationDraft.Model, a.modelOptions()).Height(30).
					Label("已安装模型").OnChange(func() { a.cancelModelDetection(); a.modelStatus = "" })
			}
		})
		settingsGroup(c, func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "翻译提示词").FontSize(12).FontWeight(600).Grow(1)
				ui.Button(c, "恢复提示词").Height(26).OnClick(func() {
					a.translationDraft.Prompt = translation.DefaultPrompt
					a.cancelModelDetection()
					a.modelStatus = ""
				})
			})
			ui.TextArea(c.Key("translation-prompt"), &a.translationDraft.Prompt).Height(137).Padding(9).FontSize(11).
				Label("自定义翻译提示词").OnChange(func() { a.cancelModelDetection(); a.modelStatus = "" })
			ui.Text(c, "每次翻译自动注入；默认中英互译，保留前端术语与代码。").FontSize(10).TextColor(muted)
		})
	})
	ui.Row(c).Padding(0, 16, 12).AlignItems(ui.Center).Gap(9).Shrink(0).Children(func() {
		if a.detectingModels {
			ui.Spinner(c).Size(15, 15)
		}
		status := a.modelStatus
		if status == "" {
			status = "修改后保存，下一次翻译生效"
		}
		ui.Text(c, status).FontSize(10).TextColor(muted).Grow(1).MinWidth(0).MaxLines(3)
		ui.PrimaryButton(c, "保存翻译配置").Height(31).OnClick(a.saveTranslationSettings)
	})
}
