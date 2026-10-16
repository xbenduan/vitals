package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := appSettings{
		ShowCPU:     true,
		ShowRAM:     false,
		ShowGPU:     true,
		ShowNetwork: false,
		RefreshRate: "5s",
	}
	if err := saveSettings(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ShowCPU != want.ShowCPU || got.ShowRAM != want.ShowRAM ||
		got.ShowGPU != want.ShowGPU || got.ShowNetwork != want.ShowNetwork ||
		got.RefreshRate != want.RefreshRate {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
	if got.refreshInterval() != 5*time.Second {
		t.Fatalf("refresh interval = %s", got.refreshInterval())
	}
}

func TestSettingsAlwaysKeepOneMetric(t *testing.T) {
	settings := appSettings{RefreshRate: "invalid"}
	settings.normalize()
	if settings.visibleMetricCount() != 4 {
		t.Fatalf("visible metric count = %d, want 4", settings.visibleMetricCount())
	}
	if settings.RefreshRate != "2s" {
		t.Fatalf("refresh rate = %q, want 2s", settings.RefreshRate)
	}
}
