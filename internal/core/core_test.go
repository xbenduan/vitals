package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRelatedConfidence(t *testing.T) {
	app := AppInfo{Name: "Example App", BundleID: "dev.example.app"}
	tests := []struct {
		name      string
		want      string
		wantMatch bool
	}{
		{"dev.example.app", "high", true},
		{"dev.example.app.savedState", "high", true},
		{"app", "", false},
		{"Example App", "medium", true},
		{"Example App.log", "medium", true},
		{"example", "", false},
		{"dev.example.other", "", false},
	}
	for _, test := range tests {
		got, matched := relatedConfidence(test.name, app)
		if got != test.want || matched != test.wantMatch {
			t.Errorf("relatedConfidence(%q) = %q, %v; want %q, %v", test.name, got, matched, test.want, test.wantMatch)
		}
	}
}

func TestRelatedConfidenceUsesBundleLeafForNestedSupportFolders(t *testing.T) {
	app := AppInfo{Name: "Google Chrome", BundleID: "com.google.Chrome"}
	if confidence, matched := relatedConfidence("Chrome", app); !matched || confidence != confidenceHigh {
		t.Fatalf("Chrome = %q, %v; want high, true", confidence, matched)
	}
}

func TestUpdaterConfidenceProtectsSharedVendorComponents(t *testing.T) {
	app := AppInfo{Name: "Google Chrome", BundleID: "com.google.Chrome"}
	path := "/Users/example/Library/Application Support/Google/GoogleUpdater"
	if confidence, matched := updaterConfidence(path, app, false); !matched || confidence != confidenceHigh {
		t.Fatalf("exclusive updater = %q, %v; want high, true", confidence, matched)
	}
	if confidence, matched := updaterConfidence(path, app, true); !matched || confidence != confidenceMedium {
		t.Fatalf("shared updater = %q, %v; want medium, true", confidence, matched)
	}
}

func TestUpdaterConfidenceRejectsUnrelatedUpdater(t *testing.T) {
	app := AppInfo{Name: "Google Chrome", BundleID: "com.google.Chrome"}
	if confidence, matched := updaterConfidence("/Library/Application Support/Microsoft/Updater", app, false); matched {
		t.Fatalf("unrelated updater = %q, true; want no match", confidence)
	}
}

func TestLaunchItemConfidenceUsesProgramPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "LaunchAgents", "opaque.agent.plist")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>Label</key><string>opaque.agent</string>
<key>ProgramArguments</key><array>
<string>/Applications/Example.app/Contents/MacOS/helper</string>
</array>
</dict></plist>`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	app := AppInfo{Name: "Example", Path: "/Applications/Example.app", BundleID: "dev.example.app"}
	if confidence, matched := launchItemConfidence(path, app); !matched || confidence != confidenceHigh {
		t.Fatalf("launch item = %q, %v; want high, true", confidence, matched)
	}
}

func TestQuitApplicationAcceptsUnregisteredBundleID(t *testing.T) {
	err := QuitApplication(t.Context(), AppInfo{Name: "Missing", BundleID: "dev.vitals.missing-test-app"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRateHandlesCounterReset(t *testing.T) {
	if got := rate(1200, 1000, 2); got != 100 {
		t.Fatalf("rate = %d, want 100", got)
	}
	if got := rate(10, 1000, 2); got != 0 {
		t.Fatalf("reset rate = %d, want 0", got)
	}
}

func TestHistoryIsBounded(t *testing.T) {
	var values []float64
	for i := 0; i < historyLength+12; i++ {
		values = appendHistory(values, float64(i))
	}
	if len(values) != historyLength {
		t.Fatalf("history length = %d, want %d", len(values), historyLength)
	}
	if values[0] != 12 {
		t.Fatalf("first history value = %v, want 12", values[0])
	}
}

func TestRootApplicationPath(t *testing.T) {
	tests := map[string]string{
		"/Applications/Lark.app/Contents/MacOS/Lark": "/Applications/Lark.app",
		"/Applications/Google Chrome.app/Contents/Frameworks/Google Chrome Framework.framework/Helpers/Google Chrome Helper.app/Contents/MacOS/Google Chrome Helper": "/Applications/Google Chrome.app",
		"/usr/local/bin/node": "",
	}
	for executable, want := range tests {
		if got := rootApplicationPath(executable); got != want {
			t.Errorf("rootApplicationPath(%q) = %q, want %q", executable, got, want)
		}
	}
}

func TestAggregateMemoryConsumersGroupsApplicationProcesses(t *testing.T) {
	samples := []processMemorySample{
		{
			name:       "Electron",
			executable: "/Applications/Trae CN.app/Contents/MacOS/Electron",
			rss:        100,
		},
		{
			name:       "Trae CN Helper",
			executable: "/Applications/Trae CN.app/Contents/Frameworks/Trae CN Helper.app/Contents/MacOS/Trae CN Helper",
			rss:        250,
		},
		{
			name:       "node",
			executable: "/usr/local/bin/node",
			rss:        50,
		},
	}
	consumers := aggregateMemoryConsumers(samples, 1_000)
	if len(consumers) != 2 {
		t.Fatalf("consumers = %#v, want 2 groups", consumers)
	}
	trae := consumers[0]
	if trae.Name != "Trae CN" || trae.AppPath != "/Applications/Trae CN.app" {
		t.Fatalf("Trae identity = %#v", trae)
	}
	if trae.RSS != 350 || trae.ProcessCount != 2 || trae.Percent != 35 {
		t.Fatalf("Trae totals = %#v", trae)
	}
}

func TestCloneSnapshotCopiesMemoryConsumers(t *testing.T) {
	original := Snapshot{MemoryConsumers: []MemoryConsumer{{Name: "Example", RSS: 100}}}
	cloned := cloneSnapshot(original)
	cloned.MemoryConsumers[0].RSS = 200
	if original.MemoryConsumers[0].RSS != 100 {
		t.Fatal("cloneSnapshot shares MemoryConsumers backing storage")
	}
}

func TestMoveToTrashStopsBeforeApplication(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first")
	blocked := filepath.Join(root, "blocked")
	app := filepath.Join(root, "Example.app")
	for _, path := range []string{first, blocked, app} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	var moved []string
	err := MoveToTrash(t.Context(), []string{first, blocked, app}, func(path string) error {
		moved = append(moved, path)
		if path == blocked {
			return errors.New("operation not permitted")
		}
		return nil
	})
	if err == nil {
		t.Fatal("MoveToTrash succeeded, want an error")
	}
	if len(moved) != 2 || moved[0] != first || moved[1] != blocked {
		t.Fatalf("moved = %v, want first and blocked only", moved)
	}
}

func TestMoveToTrashSkipsMissingItems(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "Example.app")
	if err := os.Mkdir(app, 0o755); err != nil {
		t.Fatal(err)
	}
	var moved []string
	err := MoveToTrash(t.Context(), []string{filepath.Join(root, "missing"), app}, func(path string) error {
		moved = append(moved, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 1 || moved[0] != app {
		t.Fatalf("moved = %v, want app only", moved)
	}
}

func TestProtectedLibraryPath(t *testing.T) {
	home := "/Users/example"
	if !protectedLibraryPath("/Users/example/Library/Containers/dev.example.app", home) {
		t.Fatal("container path should be protected")
	}
	if !protectedLibraryPath("/Users/example/Library/Application Support/Example", home) {
		t.Fatal("application support path should be protected")
	}
	if protectedLibraryPath("/Users/example/Documents/Example", home) {
		t.Fatal("documents path should not be protected")
	}
}

func TestScanLaunchPlistsBuildsReadOnlyUserItem(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "dev.example.sync.plist")
	data := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>Label</key><string>dev.example.sync</string>
<key>ProgramArguments</key><array>
<string>/Applications/Example.app/Contents/MacOS/Example</string>
<string>--background</string>
</array>
<key>RunAtLoad</key><true/>
</dict></plist>`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := scanLaunchPlists(
		t.Context(),
		[]startupRoot{{root, "用户代理", "当前用户"}},
		map[string]bool{"dev.example.sync": true},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	item := items[0]
	if item.Label != "dev.example.sync" || item.Name != "sync" {
		t.Fatalf("identity = %q / %q", item.Label, item.Name)
	}
	if item.Enabled || item.Trigger != "登录时" {
		t.Fatalf("state = enabled %v, trigger %q", item.Enabled, item.Trigger)
	}
	if item.AppPath != "/Applications/Example.app" {
		t.Fatalf("app path = %q", item.AppPath)
	}
}

func TestScanStartupItemsIntegration(t *testing.T) {
	if os.Getenv("VITALS_INTEGRATION") != "1" {
		t.Skip("set VITALS_INTEGRATION=1 to inspect this Mac")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	items, err := ScanStartupItems(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("no startup items found")
	}
	seen := make(map[string]bool)
	for _, item := range items {
		if item.ID == "" || item.Name == "" || item.Kind == "" || item.Scope == "" {
			t.Fatalf("incomplete item: %#v", item)
		}
		if seen[item.ID] {
			t.Fatalf("duplicate item id: %s", item.ID)
		}
		seen[item.ID] = true
	}
	t.Logf("found %d startup items", len(items))
}

func TestBatteryInfoFromProperties(t *testing.T) {
	info := batteryInfoFromProperties(map[string]any{
		"CurrentCapacity":   73,
		"CycleCount":        uint64(245),
		"IsCharging":        true,
		"ExternalConnected": true,
		"BatteryData": map[string]any{
			"DesignCapacity":     int64(6000),
			"FullChargeCapacity": int64(4800),
			"Temperature":        int64(3031),
		},
	})
	if !info.Present || info.ChargePercent != 73 || info.HealthPercent != 80 || info.CycleCount != 245 {
		t.Fatalf("battery info = %#v", info)
	}
	if !info.Charging || !info.ExternalPower || info.TemperatureC < 29 || info.TemperatureC > 31 {
		t.Fatalf("battery state = %#v", info)
	}
}

func TestBatteryInfoIntegration(t *testing.T) {
	if os.Getenv("VITALS_INTEGRATION") != "1" {
		t.Skip("set VITALS_INTEGRATION=1 to inspect this Mac")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	info, ok := batteryInfo(ctx)
	if !ok {
		t.Skip("this Mac has no readable battery")
	}
	if info.ChargePercent < 0 || info.ChargePercent > 100 || info.CycleCount < 0 {
		t.Fatalf("invalid battery info: %#v", info)
	}
	t.Logf("battery: charge=%d health=%d cycles=%d", info.ChargePercent, info.HealthPercent, info.CycleCount)
}

func TestAnyExecutableInBundle(t *testing.T) {
	executables := []string{
		"/Applications/Foo.app/Contents/MacOS/Foo",
		"/Applications/Bar.app/Contents/Frameworks/Helper.app/Contents/MacOS/Helper",
	}
	tests := []struct {
		appPath string
		want    bool
	}{
		{"/Applications/Foo.app", true},
		{"/Applications/Bar.app", false},
		{"/Applications/Foo", false},
	}
	for _, test := range tests {
		if got := anyExecutableInBundle(executables, test.appPath); got != test.want {
			t.Errorf("anyExecutableInBundle(%q) = %v, want %v", test.appPath, got, test.want)
		}
	}
}

func TestLocalizedBundleName(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Demo.app")
	lproj := filepath.Join(app, "Contents", "Resources", "zh-Hans.lproj")
	if err := os.MkdirAll(lproj, 0o755); err != nil {
		t.Fatal(err)
	}
	strings := `"CFBundleDisplayName" = "演示";` + "\n"
	if err := os.WriteFile(filepath.Join(lproj, "InfoPlist.strings"), []byte(strings), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := bundleMetadata{Name: "Demo"}
	if got := localizedBundleName(app, meta, []string{"zh-Hans"}); got != "演示" {
		t.Fatalf("zh name = %q, want 演示", got)
	}
	if got := localizedBundleName(app, meta, []string{"en"}); got != "Demo" {
		t.Fatalf("en name = %q, want Demo", got)
	}
	if got := localizedBundleName(app, bundleMetadata{}, nil); got != "Demo" {
		t.Fatalf("fallback name = %q, want Demo", got)
	}
}
