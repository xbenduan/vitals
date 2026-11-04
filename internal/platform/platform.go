// Package platform provides native clipboard, focus, and window integration.
package platform

// Target identifies the external application and, on Windows, its paste window.
type Target struct {
	PID    int
	Window uintptr
	Name   string
}

// Matches reports whether two valid targets refer to the same paste destination.
// Display names are ignored; distinct windows of one Windows process are not equal.
func (t Target) Matches(other Target) bool {
	return t.PID > 0 && t.PID == other.PID && t.Window == other.Window
}

// Bounds describes a rectangle in a platform's screen coordinate system.
type Bounds struct {
	X, Y, Width, Height float64
}

// PanelBounds positions a panel beside the cursor and keeps it inside the work area.
// All inputs must use the same units: DIPs on macOS, physical pixels on Windows.
// Near an edge the panel flips left/up; small screens shrink it to fit.
func PanelBounds(x, y, width, height, gap float64, work Bounds) Bounds {
	width = min(width, work.Width)
	height = min(height, work.Height)
	left, top := x+gap, y+gap
	if left+width > work.X+work.Width {
		left = x - gap - width
	}
	if top+height > work.Y+work.Height {
		top = y - gap - height
	}
	return Bounds{
		X:     max(work.X, min(left, work.X+work.Width-width)),
		Y:     max(work.Y, min(top, work.Y+work.Height-height)),
		Width: width, Height: height,
	}
}
