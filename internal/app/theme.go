package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"runtime"

	"github.com/egoist/mygo/ui"
)

var (
	ink          = ui.Hex("#363542")
	muted        = ui.Hex("#898697")
	accent       = ui.Hex("#8066D9")
	lavender     = ui.Hex("#F0ECFC")
	paper        = ui.Hex("#FCFBFE")
	hairline     = ui.Hex("#EAE7F0")
	selectedLine = ui.Hex("#DCD2F8")

	iconClipboard = vectorIcon(`<rect x="5" y="5" width="14" height="16" rx="3"/><rect x="9" y="3" width="6" height="4" rx="1.5"/><path d="M9 11h6M9 15h4"/>`)
	iconSettings  = vectorIcon(`<path d="m9 3-.6 2.1-2 .9-2-.6-2 3.4 1.5 1.6v2.3L2.4 14l2 3.5 2-.5 2 .9L9 20h4l.6-2.1 2-.9 2 .5 2-3.5-1.5-1.5v-2.3l1.5-1.6-2-3.4-2 .6-2-.9L13 3z"/><circle cx="11" cy="11.5" r="3"/>`)
	iconSearch    = vectorIcon(`<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/>`)
	iconStar      = vectorIcon(`<path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2-5.6-3-5.6 3 1.1-6.2L3 9.6l6.2-.9z"/>`)
	iconStarFill  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2-5.6-3-5.6 3 1.1-6.2L3 9.6l6.2-.9z"/></svg>`))
	iconText      = vectorIcon(`<path d="M5 6h14M12 6v13M8 19h8M5 6v3M19 6v3"/>`)
	iconImage     = vectorIcon(`<rect x="3" y="3" width="18" height="18" rx="4"/><circle cx="8" cy="8" r="1.5"/><path d="m4 17 5-5 4 4 3-6 5 7"/>`)
	iconFile      = vectorIcon(`<path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9zM14 3v6h6M8 14h8M8 17h5"/>`)
	iconBack      = vectorIcon(`<path d="m14 6-6 6 6 6"/>`)
	iconClose     = vectorIcon(`<path d="m6 6 12 12M6 18 18 6"/>`)
	iconPause     = vectorIcon(`<path d="M9 5v14M15 5v14"/>`)
	iconListen    = vectorIcon(`<circle cx="12" cy="12" r="3"/><path d="M6.3 6.3a8 8 0 0 0 0 11.4M17.7 6.3a8 8 0 0 1 0 11.4"/>`)
	iconTrash     = vectorIcon(`<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 10v7M14 10v7"/>`)
)

func vectorIcon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

func clipboardTheme() *ui.Theme {
	t := ui.LightTheme()
	t.Background, t.Surface = paper, ui.Hex("#F6F4FA")
	t.SurfaceHover, t.SurfacePressed = lavender, selectedLine
	t.Text, t.TextMuted, t.Border = ink, muted, hairline
	t.Accent, t.AccentHover, t.AccentPressed = accent, ui.Hex("#7256CE"), ui.Hex("#664BC2")
	t.Focus, t.Selection = accent.Alpha(.45), accent.Alpha(.2)
	t.Spacing, t.Radius, t.FontSize = 3, 8, 13
	return t
}

func iconButton(c *ui.Context, label string, icon *ui.SVG, action func()) ui.Element {
	b := ui.ButtonBase(c).Size(28, 28).Radius(7).Center().TextColor(muted).Label(label).Tooltip(label)
	if b.Hovered() {
		b.Background(lavender).TextColor(accent)
	}
	return b.Children(func() { ui.Icon(c, icon).Size(17, 17) }).OnClick(action)
}

func menuBarIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 36, 36))
	ink := color.NRGBA{A: 255}
	if runtime.GOOS == "windows" {
		// Windows ignores template tinting. A fixed purple tile keeps the white
		// clipboard readable on both light and dark notification areas.
		for y := 1; y < 35; y++ {
			for x := 1; x < 35; x++ {
				img.SetNRGBA(x, y, color.NRGBA{R: 128, G: 102, B: 217, A: 255})
			}
		}
		ink = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
	line := func(x, y, w, h int) {
		for dx := range w {
			for dy := range h {
				img.Set(x+dx, y+dy, ink)
			}
		}
	}
	line(9, 7, 4, 25)
	line(9, 28, 21, 4)
	line(27, 7, 3, 25)
	line(9, 7, 6, 3)
	line(23, 7, 7, 3)
	line(14, 4, 10, 3)
	line(14, 4, 3, 8)
	line(21, 4, 3, 8)
	line(14, 10, 10, 3)
	line(16, 17, 8, 2)
	line(16, 22, 6, 2)
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, img)
	return buffer.Bytes()
}
