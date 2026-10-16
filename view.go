package main

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"vitals/internal/core"
	"vitals/internal/i18n"
)

const startupSettingsURL = "x-apple.systempreferences:com.apple.LoginItems-Settings.extension"

var (
	iconPulse   = vectorIcon(`<path d="M3 12h4l2-7 4 14 2-7h6"/>`)
	iconMemory  = vectorIcon(`<rect x="5" y="5" width="14" height="14" rx="2"/><path d="M9 1v4M15 1v4M9 19v4M15 19v4M1 9h4M1 15h4M19 9h4M19 15h4"/>`)
	iconGPU     = vectorIcon(`<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M8 9h8v6H8zM7 22h10"/>`)
	iconNetwork = vectorIcon(`<path d="M12 20h.01M5 12.6a10 10 0 0 1 14 0M8.5 16a5 5 0 0 1 7 0M2 9a15 15 0 0 1 20 0"/>`)
	iconBack    = vectorIcon(`<path d="m15 18-6-6 6-6"/>`)
	iconRocket  = vectorIcon(`<path d="M4.5 16.5c-1.5 1.3-2 5-2 5s3.7-.5 5-2M9 15l-3-3c2.2-5.2 6.1-8.7 12-9l3 3c-.3 5.9-3.8 9.8-9 12l-3-3zM15 9h.01M9 18l-3 3"/>`)
	iconRefresh = vectorIcon(`<path d="M20 12a8 8 0 1 1-2.3-5.7L20 8M20 3v5h-5"/>`)
	iconBattery = vectorIcon(`<rect x="2" y="7" width="18" height="10" rx="2"/><path d="M22 10v4M6 10v4M10 10v4M14 10v4"/>`)
	iconShield  = vectorIcon(`<path d="M12 3 5 6v5c0 4.6 2.8 8.1 7 10 4.2-1.9 7-5.4 7-10V6l-7-3z"/><path d="m9 12 2 2 4-4"/>`)
)

func vectorIcon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

// iconChevronDown 是 ui.Select 默认触发器右侧的下拉箭头。
var iconChevronDown = vectorIcon(`<path d="M6 9l6 6 6-6"/>`)

// styledSelect 渲染一个与 ui.Select 视觉完全一致的下拉框，
// 但支持「存储值」与「显示文案」分离，避免语言切换后存储值被覆盖。
func (a *application) styledSelect(
	c *ui.Context,
	selected *string,
	label string,
	values []string,
	labelFn func(value string) string,
	onChange func(),
) {
	t := c.Theme()
	sel := ui.SelectBase(c, selected)

	// 触发器：复刻 styleButton(c, b, false) + chevron 的样式。
	b := sel.Trigger
	b.Padding(t.Space(1.5), t.Space(3.5)).Gap(t.Space(1.5)).Radius(t.Radius)
	bg := t.Surface
	switch {
	case b.Pressed():
		bg = t.SurfacePressed
	case b.Hovered():
		bg = t.SurfaceHover
	}
	b.Background(bg).TextColor(t.Text).Border(1, t.Border)
	b.Justify(ui.SpaceBetween).MinWidth(t.Space(35))
	b.Label(label).Children(func() {
		ui.Text(c, labelFn(*selected))
		ui.Box(c).Size(t.Space(2.5), t.Space(2.5)).Shrink(0).
			TextColor(t.TextMuted).Children(func() {
			ui.Icon(c, iconChevronDown)
		})
	})

	// 弹出面板：复刻 stylePanel 的样式。
	sel.Popup(func(panel ui.Element) {
		panel.Margin(t.Space(1), 0, 0, 0).Padding(t.Space(1)).
			Radius(t.Radius+2).Background(t.Background).Border(1, t.Border)
		panel.Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.18))
		for _, value := range values {
			item := sel.Item(value).Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius)
			switch {
			case item.Highlighted():
				item.Background(t.Accent).TextColor(t.AccentText)
			case value == *selected:
				item.Background(t.Surface)
			}
			item.Children(func() { ui.Text(c, labelFn(value)) })
		}
	})

	if b.Changed() && onChange != nil {
		onChange()
	}
}

func (a *application) view(c *ui.Context) {
	if c.Shortcut(0, ui.KeyEscape) || c.Shortcut(ui.Cmd, ui.KeyW) {
		if a.win != nil {
			a.win.Hide()
		}
	}
	c.OnShortcut(ui.Cmd, ui.KeyQ, mygoQuit)
	t := c.Theme()
	c.Root().Background(ui.Transparent)
	surface := t.Background
	if c.Vibrancy() {
		surface = surface.Alpha(0.92)
	}
	ui.Column(c).Fill().Margin(1).Radius(10).Clip().Background(surface).Border(1, t.Border).Children(func() {
		a.panelHeader(c)
		if a.page == 2 {
			ui.Column(c).Grow(1).MinHeight(0).Padding(12).Children(func() {
				a.uninstaller(c)
			})
		} else {
			ui.Scroll(c).Grow(1).Padding(12).Gap(12).Children(func() {
				switch a.page {
				case 0:
					a.overview(c)
				case 1:
					a.memory(c)
				case 3:
					a.settingsView(c)
				}
			})
		}
	})
	errorResult := -1
	if a.errorSettings {
		errorResult = ui.AlertDialog(c, &a.errorOpen, a.tr("error.full_disk_title"), a.actionError, a.tr("common.cancel"), a.tr("common.open_settings"))
	} else {
		errorResult = ui.AlertDialog(c, &a.errorOpen, a.tr("error.action_title"), a.actionError, a.tr("common.confirm"))
	}
	if errorResult >= 0 {
		openSettings := a.errorSettings && errorResult == 1
		a.actionError = ""
		a.errorSettings = false
		if openSettings {
			if err := mygo.Shell.OpenExternal("x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"); err != nil {
				a.showActionError(err)
			}
		}
	}
	if ui.AlertDialog(
		c,
		&a.confirmMemory,
		a.trf("memory.release_title", a.localizedApplicationName(a.pendingMemory.AppPath, a.pendingMemory.Name)),
		a.trf("memory.release_body", a.pendingMemory.ProcessCount),
		a.tr("common.cancel"),
		a.tr("memory.release_confirm"),
	) == 1 {
		a.releaseApplicationMemory()
	}
	if ui.AlertDialog(c, &a.confirmUninstall, a.tr("uninstall.title"), a.uninstallSummary(), a.tr("common.cancel"), a.tr("uninstall.confirm")) == 1 {
		a.uninstallSelected()
	}
}

func mygoQuit() {
	// Kept as a named callback so keyboard and menu actions share one command.
	mygo.App.Quit()
}

func (a *application) panelHeader(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(48).Padding(0, 12).AlignItems(ui.End).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Tabs(c, &a.page, a.tr("tabs.status"), a.tr("tabs.memory"), a.tr("tabs.apps"), a.tr("tabs.settings")).Grow(1)
	})
}

func (a *application) overview(c *ui.Context) {
	snapshot := a.snapshot
	ui.Grid(c).Columns(3).Gap(10).Children(func() {
		realtimeMetric(c, "CPU", fmt.Sprintf("%.0f%%", snapshot.CPUPercent), a.tr("overview.cpu_detail"), iconPulse, ui.Hex("#2563EB"))
		realtimeMetric(c, "RAM", fmt.Sprintf("%.0f%%", snapshot.MemoryPercent), formatBytes(snapshot.MemoryUsed), iconMemory, ui.Hex("#0F9F8F"))
		realtimeMetric(c, "GPU", fmt.Sprintf("%.0f%%", snapshot.GPUPercent), a.tr("overview.ram_detail"), iconGPU, ui.Hex("#D97706"))
	})
	networkMetric(c, snapshot, a.tr("overview.network_title"))
	section(c, func() {
		t := c.Theme()
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(3).Grow(1).Children(func() {
				ui.Text(c, "Macintosh HD").FontSize(14).Bold()
				ui.Text(c, a.trf("overview.disk_used", formatBytes(snapshot.DiskUsed), formatBytes(snapshot.DiskTotal))).FontSize(12).TextColor(t.TextMuted)
			})
			ui.Text(c, fmt.Sprintf("%.0f%%", snapshot.DiskPercent)).FontSize(16).FontWeight(600)
		})
		ui.Progress(c, snapshot.DiskPercent/100).Label(a.tr("overview.disk_label")).Height(5)
		if snapshot.Battery.Present {
			ui.Divider(c)
			batterySummary(c, snapshot.Battery, a.localizer)
		}
	}).Padding(14).Gap(8)
	a.startupHealth(c)
}

func (a *application) startupHealth(c *ui.Context) {
	t := c.Theme()
	services := c.Services()
	_, review := startupCounts(a.startupItems)
	accent := ui.Hex("#16A34A")
	detail := a.trf("overview.startup_healthy", len(a.startupItems))
	if a.startupLoading {
		detail = a.tr("overview.startup_loading")
	} else if a.startupError != "" {
		accent = ui.Hex("#D97706")
		detail = a.startupError
	} else if review > 0 {
		accent = ui.Hex("#D97706")
		detail = a.trf("overview.startup_review", len(a.startupItems), review)
	}
	section(c, func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(30, 30).Radius(7).Background(accent.Alpha(0.12)).Center().Children(func() {
				ui.Icon(c, iconRocket).FontSize(16).TextColor(accent)
			})
			ui.Column(c).MinWidth(0).Grow(1).Gap(2).Children(func() {
				ui.Text(c, a.tr("overview.startup_title")).FontSize(13).Bold()
				ui.Text(c, detail).FontSize(10).TextColor(t.TextMuted).SingleLine()
			})
			ui.Button(c, "").
				Label(a.tr("overview.startup_refresh")).
				Tooltip(a.tr("overview.startup_refresh_tooltip")).
				Disabled(a.startupLoading).
				Children(func() {
					if a.startupLoading {
						ui.Spinner(c)
					} else {
						ui.Icon(c, iconRefresh).FontSize(15)
					}
				}).
				OnClick(a.refreshStartupItems)
			ui.Button(c, a.tr("overview.system_manage")).OnClick(func() { services.OpenURL(startupSettingsURL) })
		})
	}).Padding(12)
}

func batterySummary(c *ui.Context, battery core.BatteryInfo, tr i18n.Localizer) {
	t := c.Theme()
	state := tr.T("battery.on_battery")
	if battery.Charging {
		state = tr.T("battery.charging")
	} else if battery.ExternalPower {
		state = tr.T("battery.external")
	}
	health := tr.T("battery.detecting")
	if battery.HealthPercent > 0 {
		health = tr.Tf("battery.health", battery.HealthPercent)
	}
	ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconBattery).FontSize(17).TextColor(ui.Hex("#16A34A"))
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, tr.Tf("battery.status", battery.ChargePercent, state)).FontSize(12).FontWeight(600)
			ui.Text(c, tr.Tf("battery.cycles", health, battery.CycleCount)).FontSize(10).TextColor(t.TextMuted)
		})
		if battery.TemperatureC > 0 {
			ui.Text(c, fmt.Sprintf("%.0f°C", battery.TemperatureC)).FontSize(11).TextColor(t.TextMuted)
		}
	})
}

func realtimeMetric(c *ui.Context, title, value, detail string, icon *ui.SVG, accent ui.Color) {
	t := c.Theme()
	ui.Column(c).Height(96).Padding(12).Gap(7).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(26, 26).Radius(6).Background(accent.Alpha(0.12)).Center().Children(func() {
				ui.Icon(c, icon).FontSize(15).TextColor(accent)
			})
			ui.Text(c, title).FontSize(12).Bold().TextColor(t.TextMuted)
		})
		ui.Text(c, value).FontSize(21).Bold().FontFeatures("tnum").SingleLine()
		ui.Text(c, detail).FontSize(10).TextColor(t.TextMuted).SingleLine()
	})
}

func networkMetric(c *ui.Context, snapshot core.Snapshot, title string) {
	t := c.Theme()
	section(c, func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(26, 26).Radius(6).Background(ui.Hex("#DB2777").Alpha(0.12)).Center().Children(func() {
				ui.Icon(c, iconNetwork).FontSize(15).TextColor(ui.Hex("#DB2777"))
			})
			ui.Text(c, title).FontSize(12).Bold().TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Text(c, "↓ "+compactRate(snapshot.NetworkDown)).FontSize(12).FontFeatures("tnum").TextColor(ui.Hex("#2563EB"))
			ui.Text(c, "↑ "+compactRate(snapshot.NetworkUp)).FontSize(12).FontFeatures("tnum").TextColor(ui.Hex("#0F9F8F"))
		})
		networkSparkline(c, snapshot.HistoryNetworkDown, snapshot.HistoryNetworkUp).Height(72)
	})
}

func (a *application) memory(c *ui.Context) {
	t := c.Theme()
	snapshot := a.snapshot
	section(c, func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(1).Grow(1).Children(func() {
				ui.Text(c, a.tr("memory.free")).FontSize(12).TextColor(t.TextMuted)
				ui.Text(c, formatBytes(snapshot.MemoryFree)).FontSize(32).Bold().FontFeatures("tnum")
			})
			ui.Text(c, fmt.Sprintf("%.0f%%", snapshot.MemoryPercent)).FontSize(24).Bold().FontFeatures("tnum").TextColor(t.TextMuted)
		})
		ui.Progress(c, snapshot.MemoryPercent/100).Label(a.tr("memory.usage_label")).Height(5)
		ui.Row(c).Children(func() {
			ui.Text(c, a.trf("memory.used", formatBytes(snapshot.MemoryUsed))).FontSize(12).TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Text(c, a.trf("memory.total", formatBytes(snapshot.MemoryTotal))).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("memory.composition")).FontSize(14).Bold()
			ui.Spacer(c)
			ui.Text(c, a.trf("memory.swap", formatBytes(snapshot.SwapUsed), formatBytes(snapshot.SwapTotal))).FontSize(11).TextColor(t.TextMuted)
		})
		memoryComposition(c, snapshot)
		ui.Grid(c).Columns(3).Gap(8).Children(func() {
			memoryLegend(c, a.tr("memory.active"), snapshot.MemoryActive, ui.Hex("#2563EB"))
			memoryLegend(c, a.tr("memory.inactive"), snapshot.MemoryInactive, ui.Hex("#60A5FA"))
			memoryLegend(c, a.tr("memory.wired"), snapshot.MemoryWired, ui.Hex("#0F9F8F"))
			memoryLegend(c, a.tr("memory.compressed"), snapshot.MemoryCompressed, ui.Hex("#D97706"))
			memoryLegend(c, a.tr("memory.kernel_free"), snapshot.MemoryKernelFree, ui.Hex("#D1D5DB"))
			memoryLegend(c, a.tr("memory.free_legend"), snapshot.MemoryFree, ui.Hex("#16A34A"))
		})
	}).Padding(14).Gap(9)

	section(c, func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("memory.by_app")).FontSize(14).Bold()
			ui.Spacer(c)
			a.styledSelect(c, &a.memorySort, a.tr("memory.sort"), []string{"memory", "cpu"}, func(value string) string {
				return a.tr("memory.sort_" + value)
			}, nil)
		})
		consumers := sortedMemoryConsumers(snapshot.MemoryConsumers, a.memorySort)
		if a.memoryResult != "" {
			ui.Text(c, a.memoryResult).FontSize(11).FontWeight(600).TextColor(ui.Hex("#16A34A"))
		}
		if len(consumers) == 0 {
			ui.Row(c).Gap(8).PaddingY(12).Children(func() {
				ui.Spinner(c)
				ui.Text(c, a.tr("memory.summarizing")).FontSize(12).TextColor(t.TextMuted)
			})
		}
		for _, consumer := range consumers {
			consumer := consumer
			name := a.localizedApplicationName(consumer.AppPath, consumer.Name)
			ui.Column(c.Key("memory:" + consumer.AppPath + ":" + consumer.Name)).Gap(3).PaddingY(2).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					a.applicationImage(c, consumer.AppPath, name, 26)
					ui.Column(c).MinWidth(0).Grow(1).Gap(1).Children(func() {
						ui.Text(c, name).FontSize(12).FontWeight(600).SingleLine()
						ui.Text(c, a.trf("memory.process_count", consumer.ProcessCount)).FontSize(10).TextColor(t.TextMuted)
					})
					ui.Column(c).AlignItems(ui.End).Gap(1).Children(func() {
						ui.Text(c, formatBytes(consumer.RSS)).FontSize(12).FontWeight(600).FontFeatures("tnum")
						ui.Text(c, a.trf("memory.ram_cpu", consumer.Percent, consumer.CPUPercent)).FontSize(10).TextColor(t.TextMuted).FontFeatures("tnum")
					})
					if a.canReleaseApplicationMemory(consumer.AppPath) {
						label := a.tr("memory.release")
						if a.releasingApp == consumer.AppPath {
							label = a.tr("memory.releasing")
						}
						ui.Button(c, label).
							Label(name + a.tr("memory.release")).
							Disabled(a.releasingApp != "").
							Tooltip(a.tr("memory.release_action")).
							OnClick(func() { a.requestApplicationMemoryRelease(consumer) })
					}
				})
				ui.Progress(c, min(consumer.Percent/100, 1)).Label(name + a.tr("memory.release")).Height(3)
			})
		}
	}).Padding(14).Gap(8)
}

func sortedMemoryConsumers(values []core.MemoryConsumer, sortBy string) []core.MemoryConsumer {
	consumers := slices.Clone(values)
	slices.SortStableFunc(consumers, func(a, b core.MemoryConsumer) int {
		switch sortBy {
		case "cpu":
			if a.CPUPercent != b.CPUPercent {
				if a.CPUPercent > b.CPUPercent {
					return -1
				}
				return 1
			}
		default:
			if a.RSS != b.RSS {
				if a.RSS > b.RSS {
					return -1
				}
				return 1
			}
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return consumers
}

func (a *application) settingsView(c *ui.Context) {
	t := c.Theme()
	section(c, func() {
		ui.Text(c, a.tr("settings.startup")).FontSize(14).Bold()
		settingsRow(c, a.tr("settings.open_at_login"), a.tr("settings.open_at_login_detail"), func() {
			if ui.Switch(c, &a.settings.OpenAtLogin).Label(a.tr("settings.open_at_login")).Changed() {
				a.setOpenAtLogin(a.settings.OpenAtLogin)
			}
		})
	}).Padding(14).Gap(10)

	section(c, func() {
		ui.Text(c, a.tr("settings.status_bar")).FontSize(14).Bold()
		a.metricSetting(c, "CPU", a.tr("settings.cpu_detail"), &a.settings.ShowCPU)
		a.metricSetting(c, "RAM", a.tr("settings.ram_detail"), &a.settings.ShowRAM)
		a.metricSetting(c, "GPU", a.tr("settings.gpu_detail"), &a.settings.ShowGPU)
		a.metricSetting(c, a.tr("overview.network_title"), a.tr("settings.network_detail"), &a.settings.ShowNetwork)
	}).Padding(14).Gap(8)

	section(c, func() {
		ui.Text(c, a.tr("settings.update_appearance")).FontSize(14).Bold()
		settingsRow(c, a.tr("settings.refresh_rate"), a.tr("settings.refresh_detail"), func() {
			a.refreshRateSelect(c)
		})
		settingsRow(c, a.tr("settings.appearance"), a.tr("settings.appearance_detail"), func() {
			ui.Text(c, a.tr("settings.follow_system")).FontSize(11).TextColor(t.TextMuted)
		})
		settingsRow(c, a.tr("settings.language"), a.tr("settings.language_detail"), func() {
			a.languageSelect(c)
		})
	}).Padding(14).Gap(10)

	section(c, func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Text(c, "Vitals").FontSize(13).Bold()
				version := mygo.App.Version()
				if version == "" {
					version = a.tr("settings.version_debug")
				} else {
					version = a.trf("settings.version_format", version)
				}
				ui.Text(c, a.trf("settings.local_data", version)).FontSize(10).TextColor(t.TextMuted)
			})
			ui.Button(c, a.tr("common.quit_vitals")).OnClick(mygo.App.Quit)
		})
	}).Padding(14)
}

// refreshRateSelect 存储值（1s/2s/5s）与显示文案分离，语言切换后显示随之变化。
func (a *application) refreshRateSelect(c *ui.Context) {
	a.styledSelect(
		c,
		&a.settings.RefreshRate,
		a.tr("settings.refresh_rate"),
		[]string{"1s", "2s", "5s"},
		func(value string) string { return a.tr("refresh." + value) },
		a.persistSettings,
	)
}

// languageSelect 提供 auto/zh/en 三档，显示文案随当前语言变化。
func (a *application) languageSelect(c *ui.Context) {
	a.styledSelect(
		c,
		&a.settings.Language,
		a.tr("settings.language"),
		[]string{string(i18n.LangAuto), string(i18n.LangZH), string(i18n.LangEN)},
		a.languageLabel,
		func() { a.setLanguage(a.settings.Language) },
	)
}

func (a *application) languageLabel(value string) string {
	switch i18n.Lang(value) {
	case i18n.LangZH:
		return a.tr("settings.lang_zh")
	case i18n.LangEN:
		return a.tr("settings.lang_en")
	default:
		return a.tr("settings.lang_auto")
	}
}

func startupCounts(items []core.StartupItem) (enabled, review int) {
	for _, item := range items {
		if item.Enabled {
			enabled++
		}
		if item.NeedsReview {
			review++
		}
	}
	return enabled, review
}

func (a *application) metricSetting(c *ui.Context, title, detail string, value *bool) {
	disabled := *value && a.settings.visibleMetricCount() == 1
	settingsRow(c, title, detail, func() {
		if ui.Switch(c, value).Label(title).Disabled(disabled).Changed() {
			a.persistSettings()
		}
	})
}

func settingsRow(c *ui.Context, title, detail string, control func()) {
	t := c.Theme()
	ui.Row(c).MinHeight(38).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(12).FontWeight(600)
			ui.Text(c, detail).FontSize(10).TextColor(t.TextMuted).SingleLine()
		})
		control()
	})
}

func memoryComposition(c *ui.Context, snapshot core.Snapshot) {
	total := snapshot.MemoryTotal
	if total == 0 {
		total = 1
	}
	parts := []struct {
		value uint64
		color ui.Color
	}{
		{snapshot.MemoryActive, ui.Hex("#2563EB")},
		{snapshot.MemoryInactive, ui.Hex("#60A5FA")},
		{snapshot.MemoryWired, ui.Hex("#0F9F8F")},
		{snapshot.MemoryCompressed, ui.Hex("#D97706")},
		{snapshot.MemoryKernelFree, ui.Hex("#D1D5DB")},
	}
	ui.Row(c).Height(14).Radius(4).Clip().Background(c.Theme().Border).Children(func() {
		for _, part := range parts {
			width := float32(part.value) / float32(total) * 100
			if width <= 0 {
				continue
			}
			ui.Box(c).WidthPercent(min(width, 100)).Height(14).Shrink(0).Background(part.color)
		}
	})
}

func memoryLegend(c *ui.Context, label string, value uint64, color ui.Color) {
	t := c.Theme()
	ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(8, 8).Radius(2).Background(color).Shrink(0)
		ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
		ui.Spacer(c)
		ui.Text(c, formatBytes(value)).FontSize(11).FontWeight(600).FontFeatures("tnum")
	})
}

func (a *application) localizedApplicationName(appPath, fallback string) string {
	for index := range a.apps {
		if a.apps[index].Path == appPath {
			return a.apps[index].Name
		}
	}
	return fallback
}

func (a *application) applicationImage(c *ui.Context, appPath, name string, size float32) {
	if icon := a.appIcons[appPath]; icon != nil {
		ui.Image(c, icon).Size(size, size).Fit(ui.Contain).Radius(6).Label(name)
		return
	}
	a.requestAppIcon(appPath)
	ui.Avatar(c, name, nil).Size(size, size)
}

func (a *application) uninstaller(c *ui.Context) {
	if a.selectedApp >= 0 && a.selectedApp < len(a.apps) {
		a.uninstallerDetail(c)
		return
	}
	a.uninstallerList(c)
}

func (a *application) uninstallerList(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().MinHeight(0).Gap(10).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("apps.title")).FontSize(15).Bold()
			ui.Spacer(c)
			ui.Button(c, "").
				Label(a.tr("apps.refresh")).
				Tooltip(a.tr("apps.refresh_tooltip")).
				Disabled(a.loadingApps).
				Children(func() {
					if a.loadingApps {
						ui.Spinner(c)
					} else {
						ui.Icon(c, iconRefresh).FontSize(15)
					}
				}).
				OnClick(a.refreshApplications)
		})
		ui.TextInput(c, &a.appQuery).Label(a.tr("apps.search_label")).Placeholder(a.tr("apps.search_placeholder"))
		if a.loadingApps {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Spinner(c)
				ui.Text(c, a.tr("apps.loading")).FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		indices := filteredApplications(a.apps, a.appQuery)
		ui.Scroll(c).Grow(1).Gap(3).Label(a.tr("apps.title")).Children(func() {
			for _, index := range indices {
				index := index
				app := a.apps[index]
				row := ui.Row(c.Key(index)).Height(50).Padding(7, 9).Gap(10).Radius(7).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				if row.Hovered() {
					row.Background(t.Surface)
				}
				row.OnClick(func() { a.selectApplication(index) }).Children(func() {
					a.applicationImage(c, app.Path, app.Name, 30)
					ui.Column(c).MinWidth(0).Grow(1).Gap(1).Children(func() {
						ui.Text(c, app.Name).FontSize(13).FontWeight(600).SingleLine()
						detail := app.Version
						if app.Running {
							detail = a.trf("apps.running", detail)
						}
						ui.Text(c, detail).FontSize(10).TextColor(t.TextMuted).SingleLine()
					})
				})
			}
		})
		ui.Text(c, a.trf("apps.count", len(indices))).FontSize(11).TextColor(t.TextMuted)
	})
}

func (a *application) uninstallerDetail(c *ui.Context) {
	t := c.Theme()
	services := c.Services()
	app := &a.apps[a.selectedApp]
	memory, processes := a.applicationMemory(app.Path)
	startupEnabled, startupTotal := a.applicationStartupCounts(*app)
	ui.Column(c).Fill().MinHeight(0).Gap(12).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Button(c, "").Label(a.tr("apps.back_label")).Tooltip(a.tr("apps.back_tooltip")).Children(func() {
				ui.Icon(c, iconBack).FontSize(17)
			}).OnClick(func() {
				a.selectedApp = -1
			})
			ui.Text(c, a.tr("apps.profile_title")).FontSize(15).Bold()
		})
		ui.Row(c).Gap(12).AlignItems(ui.Center).Padding(10, 12).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
			a.applicationImage(c, app.Path, app.Name, 42)
			ui.Column(c).MinWidth(0).Grow(1).Gap(2).Children(func() {
				ui.Text(c, app.Name).FontSize(17).Bold().SingleLine()
				ui.Text(c, app.BundleID).FontSize(10).TextColor(t.TextMuted).SingleLine()
			})
			if app.Running {
				label := a.tr("apps.quit")
				if a.quittingApp {
					label = a.tr("apps.quitting")
				}
				ui.Button(c, label).Disabled(a.quittingApp).OnClick(a.quitSelectedApplication)
			}
		})
		ui.Grid(c).Columns(3).Gap(8).Children(func() {
			healthMetric(c, a.tr("apps.resident_memory"), formatBytes(memory), a.trf("memory.process_count", processes))
			healthMetric(c, a.tr("apps.startup_items"), fmt.Sprint(startupEnabled), a.trf("apps.startup_total", startupTotal))
			storage := app.TotalBytes
			if storage == 0 {
				storage = app.Size
			}
			healthMetric(c, a.tr("apps.disk_usage"), formatBytes(storage), a.tr("apps.disk_detail"))
		})
		if startupTotal > 0 {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.trf("apps.startup_linked", startupTotal, startupEnabled)).
					FontSize(10).
					TextColor(t.TextMuted).
					Grow(1)
				ui.Button(c, a.tr("overview.system_manage")).OnClick(func() { services.OpenURL(startupSettingsURL) })
			})
		} else if a.startupLoading {
			ui.Text(c, a.tr("apps.startup_linking")).FontSize(10).TextColor(t.TextMuted)
		}
		ui.Row(c).Children(func() {
			ui.Text(c, a.tr("apps.to_trash")).FontSize(13).Bold()
			ui.Spacer(c)
			ui.Text(c, a.trf("apps.trash_summary", selectedFileCount(*app), formatBytes(selectedBytes(*app)))).FontSize(11).TextColor(t.TextMuted)
		})
		ui.Scroll(c).Grow(1).Gap(6).Padding(10, 12).Radius(8).Background(t.Surface).Border(1, t.Border).Label(a.tr("apps.title")).Children(func() {
			locked := true
			ui.Checkbox(c, &locked, app.Path).Disabled(true).Label(a.tr("apps.app_body"))
			if a.scanningRelated && !app.Scanned {
				ui.Row(c).Gap(8).PaddingY(8).Children(func() {
					ui.Spinner(c)
					ui.Text(c, a.tr("apps.scanning")).FontSize(12).TextColor(t.TextMuted)
				})
			}
			for index := range app.Related {
				file := &app.Related[index]
				ui.Row(c.Key(file.Path)).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Checkbox(c, &file.Selected, file.Path).Grow(1)
					ui.Text(c, confidenceLabel(file.Confidence, a.localizer)).FontSize(10).TextColor(t.TextMuted)
					ui.Text(c, formatBytes(file.Size)).Width(58).TextAlign(ui.End).FontSize(10).TextColor(t.TextMuted)
				})
			}
			if app.Scanned && len(app.Related) == 0 {
				ui.Text(c, a.tr("apps.no_related")).FontSize(12).TextColor(t.TextMuted).PaddingY(8)
			}
		})
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("apps.recoverable")).FontSize(11).TextColor(t.TextMuted).Grow(1)
			label := a.tr("uninstall.confirm")
			if a.uninstalling {
				label = a.tr("apps.moving")
			}
			ui.PrimaryButton(c, label).Disabled(app.Running || !app.Scanned || a.uninstalling).OnClick(func() {
				a.confirmUninstall = true
			})
		})
	})
}

// confidenceLabel 将 core 层的语言中立置信度（high/medium）翻译为当前语言。
func confidenceLabel(confidence string, tr i18n.Localizer) string {
	switch confidence {
	case "high":
		return tr.T("apps.confidence_high")
	case "medium":
		return tr.T("apps.confidence_medium")
	default:
		return confidence
	}
}

func healthMetric(c *ui.Context, label, value, detail string) {
	t := c.Theme()
	ui.Column(c).Padding(8).Gap(1).Radius(7).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Text(c, value).FontSize(13).Bold().FontFeatures("tnum").SingleLine()
		ui.Text(c, label).FontSize(10).FontWeight(600)
		ui.Text(c, detail).FontSize(9).TextColor(t.TextMuted).SingleLine()
	})
}

func (a *application) applicationMemory(path string) (uint64, int) {
	for _, consumer := range a.snapshot.MemoryConsumers {
		if consumer.AppPath == path {
			return consumer.RSS, consumer.ProcessCount
		}
	}
	return 0, 0
}

func (a *application) applicationStartupCounts(app core.AppInfo) (enabled, total int) {
	for _, item := range a.startupItems {
		matches := item.AppPath == app.Path ||
			app.BundleID != "" && (item.BundleID == app.BundleID || strings.HasPrefix(item.Label, app.BundleID+"."))
		if !matches {
			continue
		}
		total++
		if item.Enabled {
			enabled++
		}
	}
	return enabled, total
}

func section(c *ui.Context, body func()) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(18).Gap(14).Radius(8).Background(t.Surface).Border(1, t.Border).Children(body)
}

func sparkline(c *ui.Context, values []float64, ceiling float64, accent ui.Color) ui.Element {
	t := c.Theme()
	return ui.Box(c).MinWidth(40).Background(t.Border.Alpha(0.16)).Radius(5).Draw(func(p *ui.Painter, rect ui.Rect) {
		if len(values) < 2 || ceiling <= 0 {
			return
		}
		const inset = float32(6)
		graphWidth := rect.W - inset*2
		graphHeight := rect.H - inset*2
		var line, area ui.Path
		for index, value := range values {
			x := rect.X + inset + float32(index)/float32(len(values)-1)*graphWidth
			normalized := float32(math.Min(1, math.Max(0, value/ceiling)))
			y := rect.Y + rect.H - inset - normalized*graphHeight
			if index == 0 {
				line.MoveTo(x, y)
				area.MoveTo(x, rect.Y+rect.H-inset)
				area.LineTo(x, y)
			} else {
				line.LineTo(x, y)
				area.LineTo(x, y)
			}
		}
		lastX := rect.X + rect.W - inset
		area.LineTo(lastX, rect.Y+rect.H-inset).Close()
		p.Line(rect.X+inset, rect.Y+rect.H/2, lastX, rect.Y+rect.H/2, 1, t.Border.Alpha(0.35))
		p.Line(rect.X+inset, rect.Y+rect.H-inset, lastX, rect.Y+rect.H-inset, 1, t.Border.Alpha(0.5))
		p.FillPath(&area, accent.Alpha(0.14))
		p.StrokePath(&line, 2.5, accent)
	})
}

func networkSparkline(c *ui.Context, down, up []float64) ui.Element {
	t := c.Theme()
	ceiling := max(maxHistory(down), maxHistory(up))
	chart := ui.Box(c).MinWidth(40).Background(t.Border.Alpha(0.16)).Radius(5)
	x, _, over := chart.PointerPosition()
	hoverIndex := -1
	if over {
		hoverIndex = networkHoverIndex(x, chart.Bounds().W, len(down), len(up))
	}
	chart.Draw(func(p *ui.Painter, rect ui.Rect) {
		const inset = float32(6)
		graphWidth := rect.W - inset*2
		graphHeight := rect.H - inset*2
		lastX := rect.X + rect.W - inset
		p.Line(rect.X+inset, rect.Y+rect.H/2, lastX, rect.Y+rect.H/2, 1, t.Border.Alpha(0.35))
		p.Line(rect.X+inset, rect.Y+rect.H-inset, lastX, rect.Y+rect.H-inset, 1, t.Border.Alpha(0.5))
		draw := func(values []float64, accent ui.Color) {
			if len(values) < 2 {
				return
			}
			var line, area ui.Path
			for index, value := range values {
				x := rect.X + inset + float32(index)/float32(len(values)-1)*graphWidth
				normalized := float32(math.Min(1, math.Max(0, value/ceiling)))
				y := rect.Y + rect.H - inset - normalized*graphHeight
				if index == 0 {
					line.MoveTo(x, y)
					area.MoveTo(x, rect.Y+rect.H-inset)
					area.LineTo(x, y)
				} else {
					line.LineTo(x, y)
					area.LineTo(x, y)
				}
			}
			area.LineTo(lastX, rect.Y+rect.H-inset).Close()
			p.FillPath(&area, accent.Alpha(0.1))
			p.StrokePath(&line, 2.5, accent)
		}
		draw(down, ui.Hex("#2563EB"))
		draw(up, ui.Hex("#0F9F8F"))
		if hoverIndex >= 0 {
			hoverX := rect.X + inset + float32(hoverIndex)/float32(max(len(down), len(up))-1)*graphWidth
			p.Line(hoverX, rect.Y+inset, hoverX, rect.Y+rect.H-inset, 1, t.TextMuted.Alpha(0.55))
			drawHoverPoint(p, rect, down, hoverIndex, ceiling, inset, ui.Hex("#2563EB"))
			drawHoverPoint(p, rect, up, hoverIndex, ceiling, inset, ui.Hex("#0F9F8F"))
		}
	})
	if tooltip := networkTooltip(down, up, hoverIndex); tooltip != "" {
		chart.Tooltip(tooltip)
	}
	return chart
}

func networkHoverIndex(x float32, width float32, downCount, upCount int) int {
	count := max(downCount, upCount)
	const inset = float32(6)
	graphWidth := width - inset*2
	if count < 2 || graphWidth <= 0 || x < inset || x > width-inset {
		return -1
	}
	index := int(math.Round(float64((x - inset) / graphWidth * float32(count-1))))
	return min(count-1, max(0, index))
}

func networkTooltip(down, up []float64, index int) string {
	if index < 0 || index >= max(len(down), len(up)) {
		return ""
	}
	var downRate, upRate uint64
	if index < len(down) {
		downRate = uint64(max(0, down[index]))
	}
	if index < len(up) {
		upRate = uint64(max(0, up[index]))
	}
	return fmt.Sprintf("下载 %s\n上传 %s", formatRate(downRate), formatRate(upRate))
}

func drawHoverPoint(p *ui.Painter, rect ui.Rect, values []float64, index int, ceiling float64, inset float32, accent ui.Color) {
	if index < 0 || index >= len(values) || len(values) < 2 {
		return
	}
	graphWidth := rect.W - inset*2
	graphHeight := rect.H - inset*2
	x := rect.X + inset + float32(index)/float32(len(values)-1)*graphWidth
	normalized := float32(math.Min(1, math.Max(0, values[index]/ceiling)))
	y := rect.Y + rect.H - inset - normalized*graphHeight
	var outer, inner ui.Path
	outer.Circle(x, y, 4)
	inner.Circle(x, y, 2)
	p.FillPath(&outer, accent)
	p.FillPath(&inner, ui.RGBA(255, 255, 255, 1))
}

func maxHistory(values []float64) float64 {
	maximum := 1.0
	for _, value := range values {
		maximum = max(maximum, value)
	}
	return maximum
}

func selectedFileCount(app core.AppInfo) int {
	count := 1
	for _, file := range app.Related {
		if file.Selected {
			count++
		}
	}
	return count
}

func selectedBytes(app core.AppInfo) uint64 {
	total := app.Size
	for _, file := range app.Related {
		if file.Selected {
			total += file.Size
		}
	}
	return total
}

func (a *application) uninstallSummary() string {
	if a.selectedApp < 0 || a.selectedApp >= len(a.apps) {
		return ""
	}
	app := a.apps[a.selectedApp]
	return a.trf("uninstall.summary", app.Name, selectedFileCount(app)-1)
}

func formatBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor, exponent := uint64(unit), 0
	for quotient := value / unit; quotient >= unit && exponent < 4; quotient /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(divisor), "KMGTPE"[exponent])
}

func formatRate(value uint64) string {
	text := formatBytes(value)
	return strings.Replace(text, "iB", "B", 1) + "/s"
}
