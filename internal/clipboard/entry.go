// Package clipboard models clipboard entries and persists their history.
package clipboard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Kind identifies the content format stored in an entry.
type Kind string

const (
	KindText  Kind = "text"
	KindImage Kind = "image"
	KindFiles Kind = "files"

	maxTextBytes = 2 << 20
	// MaxImageBytes bounds image storage and preview input size.
	MaxImageBytes = 24 << 20
	// MaxImagePixels bounds decoded image dimensions before allocation.
	MaxImagePixels = 40_000_000
)

// Entry is a saved clipboard item with a stable identity and copy metadata.
type Entry struct {
	ID       string    `json:"id"`
	Hash     string    `json:"hash"`
	Kind     Kind      `json:"kind"`
	Text     string    `json:"text,omitempty"`
	Files    []string  `json:"files,omitempty"`
	Image    string    `json:"image,omitempty"`
	Width    int       `json:"width,omitempty"`
	Height   int       `json:"height,omitempty"`
	Source   string    `json:"source,omitempty"`
	CopiedAt time.Time `json:"copiedAt"`
	Favorite bool      `json:"favorite"`
}

// Capture holds clipboard content before validation and deduplication.
// A zero At uses the current time; Files must contain absolute paths.
type Capture struct {
	Kind   Kind
	Text   string
	Files  []string
	PNG    []byte
	Source string
	At     time.Time
}

func captureEntry(c Capture) (Entry, error) {
	entry := Entry{Kind: c.Kind, Text: c.Text, Files: slices.Clone(c.Files), Source: c.Source, CopiedAt: c.At}
	if entry.CopiedAt.IsZero() {
		entry.CopiedAt = time.Now()
	}
	var payload []byte
	switch c.Kind {
	case KindText:
		if c.Text == "" {
			return entry, errors.New("文字内容不能为空")
		}
		if len(c.Text) > maxTextBytes {
			return entry, errors.New("这段文字超过 2 MB，未加入历史")
		}
		payload = []byte(c.Text)
	case KindImage:
		if len(c.PNG) > MaxImageBytes {
			return entry, errors.New("这张图片超过 24 MB，未加入历史")
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(c.PNG))
		if err != nil || format != "png" || config.Width <= 0 || config.Height <= 0 {
			return entry, errors.New("无法读取剪贴板图片")
		}
		if int64(config.Width)*int64(config.Height) > MaxImagePixels {
			return entry, errors.New("这张图片超过 4000 万像素，未加入历史")
		}
		entry.Width, entry.Height = config.Width, config.Height
		payload = c.PNG
	case KindFiles:
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
	if entry.Kind == KindImage {
		entry.Image = entry.Hash + ".png"
	}
	return entry, nil
}

// Title summarizes an entry for display, truncating text to 160 characters.
func (e Entry) Title() string {
	switch e.Kind {
	case KindImage:
		return fmt.Sprintf("图片 · %d × %d", e.Width, e.Height)
	case KindFiles:
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
