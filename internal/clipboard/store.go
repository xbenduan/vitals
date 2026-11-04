package clipboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"clipnest/internal/atomicfile"
)

// Store manages history metadata and owned image assets on disk.
// It must be used by one disk worker; it is not safe for concurrent access.
type Store struct {
	dir     string
	entries []Entry
}

// Open loads history from dir, preserving invalid files and reporting an error.
// A missing history file starts an empty store; callers must not use it on error.
func Open(dir string) (*Store, error) {
	store := &Store{dir: dir}
	data, err := os.ReadFile(filepath.Join(dir, "history.json"))
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return store, err
	}
	if err := json.Unmarshal(data, &store.entries); err != nil {
		return store, fmt.Errorf("历史记录无法读取，原文件已保留：%w", err)
	}
	seen := make(map[string]bool)
	for _, entry := range store.entries {
		if entry.ID == "" || seen[entry.ID] || (entry.Kind != KindText && entry.Kind != KindImage && entry.Kind != KindFiles) {
			return store, errors.New("历史记录格式无效，原文件已保留")
		}
		seen[entry.ID] = true
		if entry.Image != "" && (filepath.Base(entry.Image) != entry.Image || !strings.HasSuffix(entry.Image, ".png")) {
			return store, errors.New("历史图片路径无效，原文件已保留")
		}
	}
	slices.SortStableFunc(store.entries, func(a, b Entry) int { return b.CopiedAt.Compare(a.CopiedAt) })
	return store, nil
}

// Snapshot returns entries in copy order for read-only use by the UI.
// The outer slice is copied; nested file lists must remain immutable.
func (s *Store) Snapshot() []Entry {
	return slices.Clone(s.entries)
}

// Add validates and saves a capture, merging duplicates and retaining favorites.
// limit caps only the number of non-favorite entries.
func (s *Store) Add(c Capture, limit int) error {
	entry, err := captureEntry(c)
	if err != nil {
		return err
	}
	next := make([]Entry, 0, len(s.entries)+1)
	for _, existing := range s.entries {
		if existing.Hash == entry.Hash {
			entry.ID, entry.Favorite = existing.ID, existing.Favorite
		} else {
			next = append(next, existing)
		}
	}
	// IDs remain stable while text is edited; a new copy of its old value must
	// not reuse that ID.
	for _, existing := range next {
		if existing.ID == entry.ID {
			entry.ID += fmt.Sprintf("-%d", time.Now().UnixNano())
			break
		}
	}
	if entry.Kind == KindImage {
		if err := atomicfile.Write(s.ImagePath(entry), c.PNG); err != nil {
			return err
		}
	}
	return s.commit(trimHistory(append([]Entry{entry}, next...), limit))
}

func trimHistory(entries []Entry, limit int) []Entry {
	kept := make([]Entry, 0, len(entries))
	ordinary := 0
	for _, entry := range entries {
		if entry.Favorite {
			kept = append(kept, entry)
		} else if ordinary < limit {
			kept = append(kept, entry)
			ordinary++
		}
	}
	return kept
}

// Edit updates text while preserving identity and merging any resulting duplicate.
// limit caps non-favorites after the merge.
func (s *Store) Edit(id, text string, limit int) error {
	edited, err := captureEntry(Capture{Kind: KindText, Text: text})
	if err != nil {
		return err
	}
	next := slices.Clone(s.entries)
	index := slices.IndexFunc(next, func(e Entry) bool { return e.ID == id && e.Kind == KindText })
	if index < 0 {
		return errors.New("这条记录已不存在")
	}
	next[index].Text, next[index].Hash = text, edited.Hash
	// Editing into an existing value merges the duplicate, preserving favorites.
	for i := len(next) - 1; i >= 0; i-- {
		if i != index && next[i].Hash == edited.Hash {
			next[index].Favorite = next[index].Favorite || next[i].Favorite
			next = slices.Delete(next, i, i+1)
			if i < index {
				index--
			}
		}
	}
	return s.commit(trimHistory(next, limit))
}

// ToggleFavorite changes an entry's favorite state and reapplies the non-favorite limit.
func (s *Store) ToggleFavorite(id string, limit int) error {
	next := slices.Clone(s.entries)
	index := slices.IndexFunc(next, func(e Entry) bool { return e.ID == id })
	if index < 0 {
		return errors.New("这条记录已不存在")
	}
	next[index].Favorite = !next[index].Favorite
	return s.commit(trimHistory(next, limit))
}

// Remove deletes a history entry and its owned image, never an original file.
func (s *Store) Remove(id string) error {
	return s.commit(slices.DeleteFunc(slices.Clone(s.entries), func(e Entry) bool { return e.ID == id }))
}

// Clear removes all non-favorite entries and their owned images.
func (s *Store) Clear() error {
	return s.commit(slices.DeleteFunc(slices.Clone(s.entries), func(e Entry) bool { return !e.Favorite }))
}

// SetLimit retains at most limit non-favorite entries and saves the result.
// The caller supplies the same limit to subsequent mutations.
func (s *Store) SetLimit(limit int) error {
	return s.commit(trimHistory(s.entries, limit))
}

func (s *Store) commit(next []Entry) error {
	if err := atomicfile.WriteJSON(filepath.Join(s.dir, "history.json"), next); err != nil {
		return fmt.Errorf("保存历史记录：%w", err)
	}
	used := make(map[string]bool)
	for _, entry := range next {
		used[entry.Image] = true
	}
	for _, entry := range s.entries {
		if entry.Image != "" && !used[entry.Image] {
			_ = os.Remove(s.ImagePath(entry))
		}
	}
	s.entries = next
	return nil
}

// ImagePath resolves an image asset belonging to an entry from this store.
func (s *Store) ImagePath(entry Entry) string {
	return filepath.Join(s.dir, "images", entry.Image)
}
