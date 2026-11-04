package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func previewApplication() *application {
	app := newApplication()
	app.loading = false
	at := time.Date(2026, 11, 4, 14, 23, 8, 0, time.Local)
	app.entries = []clipEntry{
		{ID: "one", Kind: kindText, Text: "把灵感留在手边，随时接着写。", Source: "备忘录", CopiedAt: at},
		{ID: "two", Kind: kindText, Text: "Command + Shift + V，找回刚刚复制的内容", Source: "TRAE CN", CopiedAt: at.Add(-time.Minute), Favorite: true},
		{ID: "three", Kind: kindFiles, Files: []string{"/Users/example/Documents/项目计划.pdf"}, Source: "Finder", CopiedAt: at.Add(-3 * time.Minute)},
		{ID: "four", Kind: kindText, Text: "https://go.dev/doc/", Source: "Safari", CopiedAt: at.Add(-5 * time.Minute)},
		{ID: "five", Kind: kindText, Text: "周五 15:00 · 产品方案讨论\n记得带上新的界面草稿。", Source: "飞书", CopiedAt: at.Add(-8 * time.Minute), Favorite: true},
		{ID: "six", Kind: kindText, Text: "func main() { fmt.Println(\"Hello, clipboard\") }", Source: "TRAE CN", CopiedAt: at.Add(-11 * time.Minute)},
	}
	app.target = pasteTarget{PID: 1234, Name: "备忘录"}
	return app
}

func TestCompactPanelSearchAndTabs(t *testing.T) {
	app := previewApplication()
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	for _, text := range []string{"Vitals", "正在监听", "全部", "文字", "文件", "收藏", "2026-11-04 14:23:08"} {
		if !tester.HasText(text) {
			t.Fatalf("missing %q: %q", text, tester.Texts())
		}
	}
	if err := tester.Click("文件"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("项目计划.pdf") || tester.HasText("把灵感留在手边") {
		t.Fatalf("file filter failed: %q", tester.Texts())
	}
	if err := tester.Click("收藏"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Command + Shift + V") || tester.HasText("项目计划.pdf") {
		t.Fatalf("favorite filter failed: %q", tester.Texts())
	}
	_ = tester.Click("全部")
	_ = tester.Click("搜索剪贴板")
	tester.Type("Safari")
	if !tester.HasText("https://go.dev/doc/") || tester.HasText("把灵感留在手边") {
		t.Fatalf("search failed: %q", tester.Texts())
	}
	tester.Frame()
	if app.query != "Safari" {
		t.Fatalf("search binding did not survive a rebuild: %q", app.query)
	}
}

func TestEditorSurvivesRebuildAndCancel(t *testing.T) {
	app := previewApplication()
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	tester.Key(ui.Cmd, ui.KeyE)
	if app.detail == nil || !tester.HasText("编辑文字") {
		t.Fatal("keyboard editing did not open")
	}
	_ = tester.Click("编辑内容")
	tester.Key(ui.Cmd, ui.KeyA)
	tester.Type("修改后的内容\n第二行")
	tester.Frame()
	tester.Frame()
	if app.editDraft != "修改后的内容\n第二行" {
		t.Fatalf("edit binding lost: %q", app.editDraft)
	}
	_ = tester.Click("取消")
	tester.Frame()
	if app.detail != nil || app.entries[0].Text != "把灵感留在手边，随时接着写。" {
		t.Fatal("cancel must keep the original entry")
	}
}

func TestDoubleClickOpensEditorBeforeSecondRelease(t *testing.T) {
	app := previewApplication()
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	if err := tester.Click("记录 one"); err != nil {
		t.Fatal(err)
	}
	sequence := app.actionSequence
	if err := tester.Click("记录 one"); err != nil {
		t.Fatal(err)
	}
	if app.detail == nil || app.detail.ID != "one" || app.actionSequence <= sequence {
		t.Fatalf("double click did not open editor and cancel paste: detail=%+v", app.detail)
	}
	tester.Frame()
	tester.Frame()
	if !tester.HasText("编辑文字") {
		t.Fatal("editor disappeared after double click")
	}
}

func TestSettingsAndShortcutRecording(t *testing.T) {
	app := previewApplication()
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	_ = tester.Click("设置")
	for _, text := range []string{"打开剪贴板", "自动粘贴", "保留历史", "登录时启动", "退出 Vitals"} {
		if !tester.HasText(text) {
			t.Fatalf("missing settings control %q", text)
		}
	}
	_ = tester.Click("录制")
	tester.Key(ui.Cmd|ui.Alt, ui.KeyK)
	if app.recording || app.shortcutDraft != "Command+Option+K" {
		t.Fatalf("recorded shortcut = %q, recording %v", app.shortcutDraft, app.recording)
	}
	_ = tester.Click("打开辅助功能设置")
	if got := tester.OpenedURLs(); len(got) != 1 || got[0] != accessibilityURL {
		t.Fatalf("permission URL = %q", got)
	}
	tester.Key(0, ui.KeyEscape)
	tester.Frame()
	if app.settingsOpen || !tester.HasText("最近复制") {
		t.Fatal("settings must return to clipboard")
	}
}

func TestKeyboardSelectionAndPause(t *testing.T) {
	app := previewApplication()
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	_ = tester.Click("搜索剪贴板")
	tester.Key(0, ui.KeyDown)
	if app.selected != 1 {
		t.Fatalf("down selection = %d", app.selected)
	}
	tester.Key(0, ui.KeyUp)
	if app.selected != 0 {
		t.Fatalf("up selection = %d", app.selected)
	}
	_ = tester.Click("正在监听")
	if !app.settings.Paused || !tester.HasText("已暂停") {
		t.Fatal("pause control did not change state")
	}
}

func TestRenderClipboardStates(t *testing.T) {
	dir := os.Getenv("VITALS_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set VITALS_PREVIEW_DIR to export design previews")
	}
	app := previewApplication()
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	tester.SetScale(2)
	save := func(name string) {
		t.Helper()
		file, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tester.Image()); err != nil {
			t.Fatal(err)
		}
	}
	save("clipboard")
	app.settingsOpen = true
	tester.Frame()
	save("settings")
	app.settingsOpen = false
	app.openEntry(app.entries[0])
	tester.Frame()
	save("editor")
	app.detail = nil
	app.entries = nil
	tester.Frame()
	save("empty")
}
