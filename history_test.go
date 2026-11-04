package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *historyStore {
	t.Helper()
	store, err := openHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func addText(t *testing.T, s *historyStore, text string) clipEntry {
	t.Helper()
	if err := s.add(clipCapture{Kind: kindText, Text: text}, 100); err != nil {
		t.Fatal(err)
	}
	return s.entries[0]
}

func TestHistoryDeduplicationAndRestart(t *testing.T) {
	s := testStore(t)
	first := addText(t, s, "first")
	if err := s.favorite(first.ID, 100); err != nil {
		t.Fatal(err)
	}
	addText(t, s, "second")
	now := time.Now().Add(time.Second)
	if err := s.add(clipCapture{Kind: kindText, Text: "first", At: now, Source: "Safari"}, 100); err != nil {
		t.Fatal(err)
	}
	reopened, err := openHistory(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.entries
	if len(got) != 2 || got[0].ID != first.ID || !got[0].Favorite || !got[0].CopiedAt.Equal(now) || got[0].Source != "Safari" {
		t.Fatalf("deduplicated history = %+v", got)
	}
}

func TestEditingMergesDuplicateWithoutLosingFavorites(t *testing.T) {
	s := testStore(t)
	first := addText(t, s, "first")
	_ = s.favorite(first.ID, 100)
	second := addText(t, s, "second")
	if err := s.edit(second.ID, "first", 100); err != nil {
		t.Fatal(err)
	}
	if len(s.entries) != 1 || s.entries[0].ID != second.ID || !s.entries[0].Favorite {
		t.Fatalf("edit merge = %+v", s.entries)
	}
	// A copy of the old value must get a different ID than the edited record.
	addText(t, s, "second")
	if len(s.entries) != 2 || s.entries[0].ID == s.entries[1].ID {
		t.Fatalf("duplicate identity after editing: %+v", s.entries)
	}
}

func TestRetentionAndClearKeepFavorites(t *testing.T) {
	s := testStore(t)
	first := addText(t, s, "favorite")
	_ = s.favorite(first.ID, 100)
	addText(t, s, "older")
	if err := s.add(clipCapture{Kind: kindText, Text: "newer"}, 1); err != nil {
		t.Fatal(err)
	}
	if len(s.entries) != 2 || s.entries[0].Text != "newer" {
		t.Fatalf("retention = %+v", s.entries)
	}
	if err := s.clear(); err != nil {
		t.Fatal(err)
	}
	if len(s.entries) != 1 || s.entries[0].ID != first.ID {
		t.Fatalf("clear = %+v", s.entries)
	}
}

func TestImageAssetsAndOriginalFiles(t *testing.T) {
	s := testStore(t)
	var data bytes.Buffer
	_ = png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 40, 30)))
	if err := s.add(clipCapture{Kind: kindImage, PNG: data.Bytes()}, 100); err != nil {
		t.Fatal(err)
	}
	entry := s.entries[0]
	if entry.Width != 40 || entry.Height != 30 {
		t.Fatalf("image metadata = %+v", entry)
	}
	if _, err := os.Stat(s.imagePath(entry)); err != nil {
		t.Fatal(err)
	}
	if err := s.remove(entry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.imagePath(entry)); !os.IsNotExist(err) {
		t.Fatalf("unreferenced asset was not removed: %v", err)
	}
	original := filepath.Join(t.TempDir(), "original.txt")
	_ = os.WriteFile(original, []byte("original"), 0o600)
	if err := s.add(clipCapture{Kind: kindFiles, Files: []string{original}}, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.clear(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(original); err != nil || string(data) != "original" {
		t.Fatal("clearing history must not remove original files")
	}
}

func TestFailedSaveDoesNotChangeMemory(t *testing.T) {
	s := testStore(t)
	entry := addText(t, s, "original")
	badDir := filepath.Join(t.TempDir(), "not-a-directory")
	_ = os.WriteFile(badDir, []byte("x"), 0o600)
	s.dir = badDir
	if err := s.edit(entry.ID, "changed", 100); err == nil {
		t.Fatal("expected disk error")
	}
	if s.entries[0].Text != "original" {
		t.Fatal("failed write changed memory")
	}
}

func TestHistoryFilters(t *testing.T) {
	entries := []clipEntry{
		{ID: "a", Kind: kindText, Text: "Go", Source: "Safari"},
		{ID: "b", Kind: kindImage, Favorite: true},
		{ID: "c", Kind: kindFiles, Files: []string{"/tmp/report.pdf"}},
	}
	for _, tc := range []struct {
		tab   int
		query string
		count int
	}{
		{0, "", 3}, {1, "", 1}, {2, "", 2}, {3, "", 1},
		{0, "SAFARI", 1}, {2, "report", 1}, {2, "图片", 1}, {1, "report", 0},
	} {
		if got := filterHistory(entries, tc.tab, tc.query); len(got) != tc.count {
			t.Fatalf("filter (%d, %q) = %+v", tc.tab, tc.query, got)
		}
	}
}

func TestCorruptHistoryIsNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	_ = os.WriteFile(path, []byte("{broken"), 0o600)
	if _, err := openHistory(dir); err == nil {
		t.Fatal("corrupt history must report an error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{broken" {
		t.Fatal("corrupt history was modified")
	}
}
