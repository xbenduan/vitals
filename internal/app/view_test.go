package app

import (
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"vitals/internal/clipboard"
	"vitals/internal/platform/macos"
	"vitals/internal/translation"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func previewApplication() *application {
	app := newApplication()
	app.loading = false
	at := time.Date(2026, 11, 4, 14, 23, 8, 0, time.Local)
	app.entries = []clipboard.Entry{
		{ID: "one", Kind: clipboard.KindText, Text: "把灵感留在手边，随时接着写。", Source: "备忘录", CopiedAt: at},
		{ID: "two", Kind: clipboard.KindText, Text: "Command + Shift + V，找回刚刚复制的内容", Source: "TRAE CN", CopiedAt: at.Add(-time.Minute), Favorite: true},
		{ID: "three", Kind: clipboard.KindFiles, Files: []string{"/Users/example/Documents/项目计划.pdf"}, Source: "Finder", CopiedAt: at.Add(-3 * time.Minute)},
		{ID: "four", Kind: clipboard.KindText, Text: "https://go.dev/doc/", Source: "Safari", CopiedAt: at.Add(-5 * time.Minute)},
		{ID: "five", Kind: clipboard.KindText, Text: "周五 15:00 · 产品方案讨论\n记得带上新的界面草稿。", Source: "飞书", CopiedAt: at.Add(-8 * time.Minute), Favorite: true},
		{ID: "six", Kind: clipboard.KindText, Text: "func main() { fmt.Println(\"Hello, clipboard\") }", Source: "TRAE CN", CopiedAt: at.Add(-11 * time.Minute)},
	}
	app.target = macos.Target{PID: 1234, Name: "备忘录"}
	return app
}

func TestCompactPanelSearchAndTabs(t *testing.T) {
	app := previewApplication()
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	for _, text := range []string{"Clipboard", "正在监听", "全部", "文字", "文件", "收藏", "2026-11-04 14:23:08"} {
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

func TestClickPastesImmediatelyAndRightClickEdits(t *testing.T) {
	app := previewApplication()
	// A non-nil window allows dispatch without running native clipboard work.
	app.win = &mygo.Window{}
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	if err := tester.Click("记录 one"); err != nil {
		t.Fatal(err)
	}
	if len(app.jobs) != 1 || !app.pasting {
		t.Fatal("single click must dispatch paste without a timer")
	}
	if err := tester.Click("记录 one"); err != nil {
		t.Fatal(err)
	}
	if app.detail != nil {
		t.Fatal("a second click must not open the editor")
	}
	jobs := len(app.jobs)
	if err := tester.RightClick("记录 one"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tester.Menu(), []string{"编辑", "翻译", "-", "删除"}) || len(app.jobs) != jobs {
		t.Fatalf("text menu = %v; right click must not paste", tester.Menu())
	}
	if err := tester.ChooseMenuItem("编辑"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	tester.Frame()
	if app.detail == nil || app.detail.ID != "one" || !tester.HasText("编辑文字") || tester.HasText("删除记录") || tester.HasText("Clipboard") {
		t.Fatal("context menu must open the editor with a secondary-page header")
	}
	_ = tester.Click("返回历史")
	if err := tester.RightClick("记录 three"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tester.Menu(), []string{"预览", "-", "删除"}) {
		t.Fatalf("file menu = %v", tester.Menu())
	}
	_ = tester.ChooseMenuItem("删除")
	if !app.confirmDelete || app.deleteID != "three" || app.detail != nil {
		t.Fatal("delete must capture the context entry independently of the editor")
	}
	app.selected = 0
	tester.Frame()
	if app.deleteID != "three" {
		t.Fatal("selection must not change the pending deletion target")
	}
}

func TestSettingsAndShortcutRecording(t *testing.T) {
	app := previewApplication()
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	_ = tester.Click("设置")
	for _, text := range []string{"打开剪贴板", "自动粘贴", "保留历史", "登录时启动", "退出应用"} {
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

func TestNumberShortcutsRespectFiltersAndInputs(t *testing.T) {
	app := previewApplication()
	app.win = &mygo.Window{}
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	_ = tester.Click("收藏")
	tester.TypeKey(0, ui.Key2, "2")
	if entry, ok := app.selectedEntry(); !ok || entry.ID != "five" || len(app.jobs) != 1 {
		t.Fatalf("2 must paste the second filtered entry: %+v, jobs=%d", entry, len(app.jobs))
	}
	tester.Key(0, ui.Key9)
	if len(app.jobs) != 1 {
		t.Fatal("out-of-range number must be ignored")
	}
	tester.Key(ui.Cmd, ui.KeyF)
	tester.TypeKey(0, ui.Key1, "1")
	if app.query != "1" || len(app.jobs) != 1 {
		t.Fatalf("search must accept digits without paste: query=%q", app.query)
	}
	app.openEntry(app.entries[0])
	tester.Frame()
	_ = tester.Click("编辑内容")
	tester.Key(ui.Cmd, ui.KeyA)
	tester.TypeKey(0, ui.Key9, "9")
	if app.editDraft != "9" || len(app.jobs) != 1 {
		t.Fatal("editor must accept digits")
	}
	app.openSettings()
	tester.Frame()
	_ = tester.Click("全局快捷键")
	tester.Key(ui.Cmd, ui.KeyA)
	tester.TypeKey(0, ui.Key1, "1")
	if app.shortcutDraft != "1" || len(app.jobs) != 1 {
		t.Fatal("settings must accept digits")
	}
}

func TestTranslationSettingsAndEditableResult(t *testing.T) {
	app := previewApplication()
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	_ = tester.RightClick("记录 one")
	if err := tester.ChooseMenuItem("翻译"); err != nil {
		t.Fatal(err)
	}
	if !app.translationMode || !tester.HasText("翻译结果") {
		t.Fatal("translate must reuse the editor")
	}
	app.finishTranslation(app.translationSequence, "one", "Keep the idea close.", "", nil)
	tester.Frame()
	_ = tester.Click("编辑内容")
	tester.Key(ui.Cmd, ui.KeyA)
	tester.TypeKey(0, ui.Key1, "1")
	tester.Type(" edited translation")
	if app.editDraft != "1 edited translation" || app.entries[0].Text != "把灵感留在手边，随时接着写。" {
		t.Fatal("translation must be editable without changing the source")
	}
	_ = tester.Click("返回历史")
	_ = tester.Click("设置")
	_ = tester.Click("翻译")
	for _, label := range []string{"模型服务", "翻译提示词", "检测模型", "保存翻译配置"} {
		if !tester.HasText(label) {
			t.Fatalf("missing translation setting %q", label)
		}
	}
	_ = tester.Click("模型名称")
	tester.Type("local-model")
	_ = tester.Click("自定义翻译提示词")
	tester.Key(ui.Cmd, ui.KeyA)
	tester.Type("Translate frontend documentation.")
	tester.Frame()
	if app.settings.Translation.Model != "" {
		t.Fatal("draft must not apply before saving")
	}
	_ = tester.Click("保存翻译配置")
	if app.settings.Translation.Model != "local-model" || app.settings.Translation.Prompt != "Translate frontend documentation." {
		t.Fatalf("translation settings must persist both model and prompt: model=%q prompt=%q status=%q",
			app.settings.Translation.Model, app.settings.Translation.Prompt, app.modelStatus)
	}
	_ = tester.Click("恢复提示词")
	if app.translationDraft.Prompt != translation.DefaultPrompt {
		t.Fatal("restore must recover the frontend translation prompt")
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
	app.openSettings()
	tester.Frame()
	save("settings")
	app.settingsTab = 1
	app.translationDraft.Model = "Ornith-1.5-9B-GGUF:latest"
	tester.Frame()
	save("translation-settings")
	app.translationDraft.Provider = translation.OpenAI
	app.translationDraft.BaseURL = "http://127.0.0.1:1234/v1"
	tester.Frame()
	save("openai-settings")
	app.settingsOpen = false
	app.openEntry(app.entries[0])
	tester.Frame()
	save("editor")
	app.translationMode = true
	app.editDraft = "Keep your ideas close at hand, ready to pick up where you left off."
	tester.Frame()
	save("translation")
	app.translating = true
	tester.Frame()
	save("translation-loading")
	app.translating = false
	app.translationMode = false
	app.detail = nil
	app.entries = nil
	tester.Frame()
	save("empty")
}
