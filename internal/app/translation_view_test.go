package app

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestTranslationSourceReadOnlyAndResultEditable(t *testing.T) {
	for _, entryPoint := range []string{"selection", "history"} {
		t.Run(entryPoint, func(t *testing.T) {
			app := previewApplication()
			app.entries[0].Text = strings.Repeat("A long source paragraph. 原文只读。\n", 30)
			original := app.entries[0]
			tester := ui.NewTester(app.view, panelWidth, panelHeight)
			if entryPoint == "history" {
				if err := tester.RightClick("记录 one"); err != nil {
					t.Fatal(err)
				}
				if err := tester.ChooseMenuItem("翻译"); err != nil {
					t.Fatal(err)
				}
			} else {
				app.resetForShow()
				app.openSelectionTranslation(original.Text, nil)
			}
			app.finishTranslation(app.translationSequence, app.detail.ID, "Initial translation", nil)
			tester.Frame()
			if err := tester.Click("原文内容"); err != nil {
				t.Fatal(err)
			}
			tester.Key(primaryModifier(), ui.KeyA)
			tester.Key(primaryModifier(), ui.KeyC)
			if tester.Clipboard() != original.Text {
				t.Fatal("read-only source must still support selecting and copying all text")
			}
			tester.Type("replacement")
			tester.SetClipboard("replacement")
			tester.Command("paste")
			tester.Command("cut")
			tester.Key(0, ui.KeyBackspace)
			tester.Frame()
			tester.Frame()
			if app.translationSource != original.Text || app.detail.Text != original.Text || app.entries[0].Text != original.Text {
				t.Fatal("typing, paste, cut and delete must not modify the source or history")
			}
			source, _ := tester.Find("原文内容")
			tester.Scroll(source.X+20, source.Y+20, 0, -500)
			assertTranslationButtonPlacement(t, tester, "重新翻译")
			if err := tester.Click("编辑内容"); err != nil {
				t.Fatal(err)
			}
			tester.Key(primaryModifier(), ui.KeyA)
			tester.Type("Manually corrected result")
			tester.Frame()
			if app.editDraft != "Manually corrected result" {
				t.Fatal("the translation result must remain editable")
			}
			app.win = &mygo.Window{}
			tester.Key(primaryModifier(), ui.KeyS)
			if !app.saving || len(app.jobs) != 1 {
				t.Fatal("the corrected result must remain saveable")
			}
		})
	}
}

func TestTranslationStopAndRetryStayAtBottom(t *testing.T) {
	app := previewApplication()
	original := app.entries[0]
	app.openTranslation(original)
	app.editDraft = "Previous editable translation"
	app.translating = true
	ctx, cancel := context.WithCancel(context.Background())
	app.translationCancel = cancel
	seq := app.translationSequence
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	assertTranslationButtonPlacement(t, tester, "停止")
	if !tester.HasText("正在翻译…") {
		t.Fatal("loading status must remain visible in the result area")
	}
	if err := tester.Click("停止"); err != nil {
		t.Fatal(err)
	}
	app.finishTranslation(seq, original.ID, "Old response", nil)
	if ctx.Err() != context.Canceled || app.translating || app.editDraft != "Previous editable translation" {
		t.Fatal("stopping must cancel stale responses and retain the existing result")
	}
	tester.Frame()
	assertTranslationButtonPlacement(t, tester, "重试")
	if err := tester.Click("重试"); err != nil {
		t.Fatal(err)
	}
	if app.translationSource != original.Text || app.translationSequence <= seq || app.translationError == "" {
		t.Fatal("retry must use the same source and explain missing credentials")
	}
	assertTranslationButtonPlacement(t, tester, "重试")
	tester.Click("编辑内容")
	tester.Key(primaryModifier(), ui.KeyA)
	tester.Type("Corrected after failure")
	if app.editDraft != "Corrected after failure" {
		t.Fatal("a failed request must not disable editing the result")
	}
}

func TestTranslationHeaderSettingsReturnsToSameResult(t *testing.T) {
	app := previewApplication()
	app.openTranslation(app.entries[0])
	detail := app.detail
	app.editDraft = "Keep this result"
	app.translating = true
	ctx, cancel := context.WithCancel(context.Background())
	app.translationCancel = cancel
	seq := app.translationSequence
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	if err := tester.Click("翻译设置"); err != nil {
		t.Fatal(err)
	}
	if !app.settingsOpen || app.settingsTab != 1 || ctx.Err() != context.Canceled || !tester.HasText("腾讯交互翻译") {
		t.Fatal("the header gear must open translation settings and cancel pending work")
	}
	if err := tester.Click("返回翻译"); err != nil {
		t.Fatal(err)
	}
	app.finishTranslation(seq, detail.ID, "Old response", nil)
	tester.Frame()
	tester.Frame()
	if app.settingsOpen || app.detail != detail || app.translationSource != detail.Text || app.editDraft != "Keep this result" {
		t.Fatal("returning from settings must preserve the source and edited translation")
	}
	if !tester.Focused("编辑内容") {
		t.Fatal("returning from settings must focus the translation result")
	}
	assertTranslationButtonPlacement(t, tester, "重试")
}

func TestEmptySelectionRequiresReselection(t *testing.T) {
	app := previewApplication()
	app.openSelectionTranslation("", errNoSelection)
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	if !tester.HasText("未获取到选中文字") {
		t.Fatal("empty selection must explain recovery")
	}
	tester.Click("原文内容")
	tester.Type("Cannot enter a source here")
	tester.Frame()
	if app.translationSource != "" || !tester.HasText("重新划词") || tester.HasText("输入原文") {
		t.Fatal("empty selection must stay read-only and explain how to select again")
	}
	before := app.translationSequence
	tester.Click("重试")
	if app.translationSequence != before || app.translating {
		t.Fatal("retry without source text must be disabled")
	}
	app.resetForShow()
	app.openSelectionTranslation("A new selection", nil)
	tester.Frame()
	if app.translationSource != "A new selection" || !tester.HasText("翻译结果") {
		t.Fatal("a new selection must populate the shared translation page")
	}
	app.resetForShow()
	tester.Frame()
	tester.Frame()
	tester.Key(primaryModifier(), ui.KeyE)
	if app.translationMode || app.editDraft != app.entries[0].Text {
		t.Fatal("ordinary reopening must restore the normal history editor")
	}
}

func assertTranslationButtonPlacement(t *testing.T, tester *ui.Tester, action string) {
	t.Helper()
	source, sourceOK := tester.Find("原文内容")
	result, resultOK := tester.Find("编辑内容")
	settings, settingsOK := tester.Find("翻译设置")
	if !sourceOK || !resultOK || !settingsOK || result.Y <= source.Y+source.H {
		t.Fatal("the translation page must have separate source and result areas")
	}
	if settings.Y+settings.H >= source.Y || settings.X <= source.X+source.W/2 {
		t.Fatal("translation settings must sit at the right of the page header")
	}
	for _, label := range []string{action, "取消", "保存", "保存并粘贴"} {
		button, ok := tester.Find(label)
		if !ok || button.Y < result.Y+result.H || button.Y+button.H > panelHeight {
			t.Fatalf("%s must remain below both text areas: %+v", label, button)
		}
	}
}
