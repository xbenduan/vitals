package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"vitals/internal/core"
)

func TestFilteredAppIndices(t *testing.T) {
	apps := []core.AppInfo{
		{Name: "Visual Studio Code", BundleID: "com.microsoft.VSCode"},
		{Name: "Safari", BundleID: "com.apple.Safari"},
	}
	got := filteredApplications(apps, "microsoft")
	if len(got) != 1 || got[0] != 0 {
		t.Fatalf("filtered indices = %v, want [0]", got)
	}
}

func TestSortedMemoryConsumers(t *testing.T) {
	consumers := []core.MemoryConsumer{
		{Name: "Memory Heavy", RSS: 200, CPUPercent: 4},
		{Name: "CPU Heavy", RSS: 100, CPUPercent: 18},
	}
	if got := sortedMemoryConsumers(consumers, "memory"); got[0].Name != "Memory Heavy" {
		t.Fatalf("memory sort = %q, want Memory Heavy", got[0].Name)
	}
	if got := sortedMemoryConsumers(consumers, "cpu"); got[0].Name != "CPU Heavy" {
		t.Fatalf("cpu sort = %q, want CPU Heavy", got[0].Name)
	}
}

func TestFullDiskAccessErrorOffersSettings(t *testing.T) {
	app := newApplication()
	app.showActionError(&core.FullDiskAccessError{Path: "/protected"})
	if !app.errorOpen || !app.errorSettings {
		t.Fatalf("error state = open %v, settings %v; want both true", app.errorOpen, app.errorSettings)
	}
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	if !tester.HasText("需要完全磁盘访问") || !tester.HasText("打开设置") {
		t.Fatalf("dialog texts = %q", tester.Texts())
	}
}

func TestSelectedFiles(t *testing.T) {
	app := core.AppInfo{
		Size: 100,
		Related: []core.RelatedFile{
			{Size: 20, Selected: true},
			{Size: 30, Selected: false},
		},
	}
	if got := selectedFileCount(app); got != 2 {
		t.Fatalf("selected count = %d, want 2", got)
	}
	if got := selectedBytes(app); got != 120 {
		t.Fatalf("selected bytes = %d, want 120", got)
	}
}

func TestApplicationMemoryReleaseRequiresConfirmation(t *testing.T) {
	app := newApplication()
	app.page = 1
	app.loadingApps = false
	app.apps = []core.AppInfo{{
		Name:     "飞书",
		Path:     "/Applications/Lark.app",
		BundleID: "com.larksuite.suite",
	}}
	app.snapshot.MemoryConsumers = []core.MemoryConsumer{{
		Name:         "Lark",
		AppPath:      "/Applications/Lark.app",
		RSS:          2 * 1024 * 1024 * 1024,
		ProcessCount: 15,
		Percent:      6.3,
	}}
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	if err := tester.Click("飞书释放"); err != nil {
		t.Fatal(err)
	}
	if !app.confirmMemory || !tester.HasText("退出并释放") {
		t.Fatalf("confirmation = %v, texts = %q", app.confirmMemory, tester.Texts())
	}
}

func TestUninstallerNavigatesBetweenListAndDetail(t *testing.T) {
	app := newApplication()
	app.page = 2
	app.loadingApps = false
	app.apps = []core.AppInfo{{
		Name:     "Example",
		Path:     "/Applications/Example.app",
		BundleID: "dev.example.app",
		Scanned:  true,
	}}
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	if !tester.HasText("Example") {
		t.Fatalf("list texts = %q", tester.Texts())
	}
	if _, ok := tester.Find("刷新应用列表"); !ok {
		t.Fatal("app list refresh button not found")
	}
	app.selectedApp = 0
	tester.Frame()
	if !tester.HasText("应用健康档案") {
		t.Fatalf("detail texts = %q", tester.Texts())
	}
	if err := tester.Click("返回应用列表"); err != nil {
		t.Fatal(err)
	}
	if app.selectedApp != -1 || !tester.HasText("Example") {
		t.Fatalf("selected app = %d, texts = %q", app.selectedApp, tester.Texts())
	}
}

func TestPanelHeaderAndPages(t *testing.T) {
	app := newApplication()
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	statusTab, ok := tester.Find("状态")
	if !ok || statusTab.X < 8 {
		t.Fatalf("status tab = %+v", statusTab)
	}
	settingsTab, ok := tester.Find("设置")
	if !ok || settingsTab.X+settingsTab.W >= panelWidth-8 {
		t.Fatalf("settings tab = %+v", settingsTab)
	}
	if tester.HasText("Vitals") {
		t.Fatal("brand should not be rendered in the tab header")
	}
	if slices.Contains(tester.Texts(), "启动") {
		t.Fatal("startup management should not be a top-level page")
	}

	app.page = 1
	tester.Frame()
	if !tester.HasText("按应用占用") {
		t.Fatalf("memory texts = %q", tester.Texts())
	}
	if tester.HasText("最近 60 秒") {
		t.Fatal("memory page still contains the 60-second section")
	}

	app.page = 3
	tester.Frame()
	for _, text := range []string{"登录时启动", "状态栏", "刷新频率", "退出 Vitals"} {
		if !tester.HasText(text) {
			t.Fatalf("settings page missing %q: %q", text, tester.Texts())
		}
	}
}

func TestOverviewShowsBatteryAndStartupHealth(t *testing.T) {
	app := newApplication()
	app.loadingApps = false
	app.startupLoaded = true
	app.startupItems = []core.StartupItem{
		{Enabled: true},
		{Enabled: true, NeedsReview: true},
	}
	app.snapshot = core.Snapshot{
		Battery: core.BatteryInfo{
			Present:       true,
			ChargePercent: 78,
			HealthPercent: 91,
			CycleCount:    120,
			ExternalPower: true,
		},
	}
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	for _, text := range []string{"电池 78% · 已连接电源", "健康度约 91% · 120 次循环", "启动项健康", "2 个启动项 · 1 个需关注", "系统管理"} {
		if !tester.HasText(text) {
			t.Fatalf("overview missing %q: %q", text, tester.Texts())
		}
	}
	for _, removed := range []string{"磁盘体检", "应用管理"} {
		if slices.Contains(tester.Texts(), removed) {
			t.Fatalf("overview still contains removed shortcut %q: %q", removed, tester.Texts())
		}
	}
	if err := tester.Click("系统管理"); err != nil {
		t.Fatal(err)
	}
	if got := tester.OpenedURLs(); !slices.Equal(got, []string{startupSettingsURL}) {
		t.Fatalf("opened URLs = %q, want %q", got, []string{startupSettingsURL})
	}
}

func TestApplicationHealthProfileCombinesExistingData(t *testing.T) {
	app := newApplication()
	app.page = 2
	app.loadingApps = false
	app.selectedApp = 0
	app.apps = []core.AppInfo{{
		Name:       "Example",
		Path:       "/Applications/Example.app",
		BundleID:   "dev.example.app",
		Size:       1024,
		TotalBytes: 2048,
		Scanned:    true,
	}}
	app.snapshot.MemoryConsumers = []core.MemoryConsumer{{
		AppPath:      "/Applications/Example.app",
		RSS:          4096,
		ProcessCount: 2,
	}}
	app.startupItems = []core.StartupItem{{
		AppPath: "/Applications/Example.app",
		Enabled: true,
	}}
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	for _, text := range []string{"应用健康档案", "常驻内存", "2 个进程", "启动项", "共 1 项", "系统管理", "磁盘占用"} {
		if !tester.HasText(text) {
			t.Fatalf("profile missing %q: %q", text, tester.Texts())
		}
	}
}

func TestStartupCounts(t *testing.T) {
	items := []core.StartupItem{
		{Enabled: true},
		{Enabled: true, NeedsReview: true},
		{NeedsReview: true},
	}
	enabled, review := startupCounts(items)
	if enabled != 2 || review != 2 {
		t.Fatalf("startup counts = %d enabled / %d review", enabled, review)
	}
}

func TestNetworkHoverIndex(t *testing.T) {
	if got := networkHoverIndex(6, 106, 5, 5); got != 0 {
		t.Fatalf("left edge index = %d, want 0", got)
	}
	if got := networkHoverIndex(56, 106, 5, 5); got != 2 {
		t.Fatalf("middle index = %d, want 2", got)
	}
	if got := networkHoverIndex(100, 106, 5, 5); got != 4 {
		t.Fatalf("right edge index = %d, want 4", got)
	}
	if got := networkHoverIndex(20, 106, 1, 1); got != -1 {
		t.Fatalf("single-point index = %d, want -1", got)
	}
}

func TestNetworkTooltip(t *testing.T) {
	got := networkTooltip([]float64{1024}, []float64{2048}, 0)
	if got != "下载 1.0 KB/s\n上传 2.0 KB/s" {
		t.Fatalf("tooltip = %q", got)
	}
	if got := networkTooltip(nil, nil, 0); got != "" {
		t.Fatalf("empty tooltip = %q", got)
	}
}
