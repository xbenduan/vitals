package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type clipKind string

const (
	kindText       clipKind = "text"
	kindImage      clipKind = "image"
	kindFiles      clipKind = "files"
	maxTextBytes            = 2 << 20
	maxImageBytes           = 24 << 20
	maxImagePixels          = 40_000_000
)

type clipEntry struct {
	ID       string    `json:"id"`
	Hash     string    `json:"hash"`
	Kind     clipKind  `json:"kind"`
	Text     string    `json:"text,omitempty"`
	Files    []string  `json:"files,omitempty"`
	Image    string    `json:"image,omitempty"`
	Width    int       `json:"width,omitempty"`
	Height   int       `json:"height,omitempty"`
	Source   string    `json:"source,omitempty"`
	CopiedAt time.Time `json:"copiedAt"`
	Favorite bool      `json:"favorite"`
}

type clipCapture struct {
	Kind   clipKind
	Text   string
	Files  []string
	PNG    []byte
	Source string
	At     time.Time
}

// historyStore is owned by the disk worker. UI receives immutable snapshots.
type historyStore struct {
	dir     string
	entries []clipEntry
}

func openHistory(dir string) (*historyStore, error) {
	store := &historyStore{dir: dir}
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
		if entry.ID == "" || seen[entry.ID] || (entry.Kind != kindText && entry.Kind != kindImage && entry.Kind != kindFiles) {
			return store, errors.New("历史记录格式无效，原文件已保留")
		}
		seen[entry.ID] = true
		if entry.Image != "" && (filepath.Base(entry.Image) != entry.Image || !strings.HasSuffix(entry.Image, ".png")) {
			return store, errors.New("历史图片路径无效，原文件已保留")
		}
	}
	slices.SortStableFunc(store.entries, func(a, b clipEntry) int { return b.CopiedAt.Compare(a.CopiedAt) })
	return store, nil
}

func (s *historyStore) snapshot() []clipEntry {
	return slices.Clone(s.entries)
}

func captureEntry(c clipCapture) (clipEntry, error) {
	entry := clipEntry{Kind: c.Kind, Text: c.Text, Files: slices.Clone(c.Files), Source: c.Source, CopiedAt: c.At}
	if entry.CopiedAt.IsZero() {
		entry.CopiedAt = time.Now()
	}
	var payload []byte
	switch c.Kind {
	case kindText:
		if c.Text == "" {
			return entry, errors.New("文字内容不能为空")
		}
		if len(c.Text) > maxTextBytes {
			return entry, errors.New("这段文字超过 2 MB，未加入历史")
		}
		payload = []byte(c.Text)
	case kindImage:
		if len(c.PNG) > maxImageBytes {
			return entry, errors.New("这张图片超过 24 MB，未加入历史")
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(c.PNG))
		if err != nil || format != "png" || config.Width <= 0 || config.Height <= 0 {
			return entry, errors.New("无法读取剪贴板图片")
		}
		if int64(config.Width)*int64(config.Height) > maxImagePixels {
			return entry, errors.New("这张图片超过 4000 万像素，未加入历史")
		}
		entry.Width, entry.Height = config.Width, config.Height
		payload = c.PNG
	case kindFiles:
		if len(c.Files) == 0 {
			return entry, errors.New("文件列表为空")
		}
		for _, path := range c.Files {
			if !filepath.IsAbs(path) {
				return entry, errors.New("文件路径不是绝对路径")
			}
		}
		payload, _ = json.Marshal(c.Files)
	default:
		return entry, errors.New("不支持的剪贴板内容")
	}
	sum := sha256.Sum256(append([]byte(string(c.Kind)+"\x00"), payload...))
	entry.Hash = hex.EncodeToString(sum[:])
	entry.ID = entry.Hash
	if entry.Kind == kindImage {
		entry.Image = entry.Hash + ".png"
	}
	return entry, nil
}

func (s *historyStore) add(c clipCapture, limit int) error {
	entry, err := captureEntry(c)
	if err != nil {
		return err
	}
	next := make([]clipEntry, 0, len(s.entries)+1)
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
	if entry.Kind == kindImage {
		if err := atomicWrite(s.imagePath(entry), c.PNG); err != nil {
			return err
		}
	}
	return s.commit(trimHistory(append([]clipEntry{entry}, next...), limit))
}

func trimHistory(entries []clipEntry, limit int) []clipEntry {
	kept := make([]clipEntry, 0, len(entries))
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

func (s *historyStore) edit(id, text string, limit int) error {
	edited, err := captureEntry(clipCapture{Kind: kindText, Text: text})
	if err != nil {
		return err
	}
	next := slices.Clone(s.entries)
	index := slices.IndexFunc(next, func(e clipEntry) bool { return e.ID == id && e.Kind == kindText })
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

func (s *historyStore) favorite(id string, limit int) error {
	next := slices.Clone(s.entries)
	index := slices.IndexFunc(next, func(e clipEntry) bool { return e.ID == id })
	if index < 0 {
		return errors.New("这条记录已不存在")
	}
	next[index].Favorite = !next[index].Favorite
	return s.commit(trimHistory(next, limit))
}

func (s *historyStore) remove(id string) error {
	return s.commit(slices.DeleteFunc(slices.Clone(s.entries), func(e clipEntry) bool { return e.ID == id }))
}

func (s *historyStore) clear() error {
	return s.commit(slices.DeleteFunc(slices.Clone(s.entries), func(e clipEntry) bool { return !e.Favorite }))
}

func (s *historyStore) commit(next []clipEntry) error {
	if err := writeJSON(filepath.Join(s.dir, "history.json"), next); err != nil {
		return fmt.Errorf("保存历史记录：%w", err)
	}
	used := make(map[string]bool)
	for _, entry := range next {
		used[entry.Image] = true
	}
	for _, entry := range s.entries {
		if entry.Image != "" && !used[entry.Image] {
			_ = os.Remove(s.imagePath(entry))
		}
	}
	s.entries = next
	return nil
}

func (s *historyStore) imagePath(entry clipEntry) string {
	return filepath.Join(s.dir, "images", entry.Image)
}

func filterHistory(entries []clipEntry, tab int, query string) []clipEntry {
	query = strings.ToLower(strings.TrimSpace(query))
	var result []clipEntry
	for _, entry := range entries {
		if tab == 1 && entry.Kind != kindText || tab == 2 && entry.Kind == kindText || tab == 3 && !entry.Favorite {
			continue
		}
		searchable := entry.Text + "\n" + strings.Join(entry.Files, "\n") + "\n" + entry.Source
		if entry.Kind == kindImage {
			searchable += "\n图片 PNG"
		}
		if query == "" || strings.Contains(strings.ToLower(searchable), query) {
			result = append(result, entry)
		}
	}
	return result
}

func (e clipEntry) title() string {
	switch e.Kind {
	case kindImage:
		return fmt.Sprintf("图片 · %d × %d", e.Width, e.Height)
	case kindFiles:
		if len(e.Files) == 0 {
			return "文件"
		}
		name := filepath.Base(e.Files[0])
		if len(e.Files) > 1 {
			return fmt.Sprintf("%s 等 %d 个文件", name, len(e.Files))
		}
		return name
	default:
		text := strings.Join(strings.Fields(e.Text), " ")
		runes := []rune(text)
		if len(runes) > 160 {
			return string(runes[:160]) + "…"
		}
		return text
	}
}
