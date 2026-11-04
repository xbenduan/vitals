package app

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"clipnest/internal/clipboard"

	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
)

func (a *application) requestImage(entry clipboard.Entry, preview bool) {
	if a.win == nil || entry.Kind == clipboard.KindText {
		return
	}
	if preview {
		if a.previewID == entry.ID {
			return
		}
		a.previewID, a.previewImage, a.previewError = entry.ID, nil, ""
	} else {
		if a.imageRequested[entry.ID] {
			return
		}
		a.imageRequested[entry.ID] = true
	}
	if !a.submit(func(store *clipboard.Store) {
		var data []byte
		var err error
		path := ""
		if entry.Kind == clipboard.KindImage && store != nil {
			path = store.ImagePath(entry)
		} else if entry.Kind == clipboard.KindFiles && len(entry.Files) == 1 && isImagePath(entry.Files[0]) {
			path = entry.Files[0]
		}
		if path == "" {
			return
		}
		info, err := os.Stat(path)
		if err == nil && info.Size() > clipboard.MaxImageBytes {
			err = errors.New("图片超过预览大小限制")
		}
		if err == nil {
			data, err = os.ReadFile(path)
		}
		var bitmap *ui.Bitmap
		if err == nil {
			config, _, configErr := image.DecodeConfig(bytes.NewReader(data))
			if configErr != nil || int64(config.Width)*int64(config.Height) > clipboard.MaxImagePixels {
				err = errors.New("无法预览此图片")
			} else {
				var decoded image.Image
				decoded, _, err = image.Decode(bytes.NewReader(data))
				if err == nil {
					size := 72
					if preview {
						size = 1280
					}
					bitmap = ui.NewBitmap(resizePreview(decoded, size))
				}
			}
		}
		a.win.Update(func() {
			if preview {
				if a.previewID != entry.ID {
					return
				}
				if err != nil {
					a.previewError = "图片无法预览，文件可能已移动或不受支持"
				} else {
					a.previewImage = bitmap
				}
			} else {
				if !a.imageRequested[entry.ID] {
					return
				}
				if err != nil {
					a.imageErrors[entry.ID] = "图片无法预览"
				} else {
					a.images[entry.ID] = bitmap
				}
			}
		})
	}) {
		delete(a.imageRequested, entry.ID)
		if preview {
			a.previewID = ""
		}
	}
}

func resizePreview(source image.Image, size int) image.Image {
	bounds := source.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if max(w, h) > size {
		ratio := float64(size) / float64(max(w, h))
		w, h = max(1, int(float64(w)*ratio)), max(1, int(float64(h)*ratio))
	}
	target := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Src, nil)
	return target
}

func isImagePath(path string) bool {
	return slices.Contains([]string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"}, strings.ToLower(filepath.Ext(path)))
}
