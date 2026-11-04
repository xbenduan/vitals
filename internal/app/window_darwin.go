package app

import (
	"clipnest/internal/platform"

	"github.com/egoist/mygo"
)

func (a *application) positionWindow() {
	cursor := mygo.Screen.CursorScreenPoint()
	work := mygo.Screen.DisplayNearestPoint(cursor).WorkArea
	bounds := platform.PanelBounds(float64(cursor.X), float64(cursor.Y), panelWidth, panelHeight, 12, platform.Bounds{
		X: float64(work.X), Y: float64(work.Y), Width: float64(work.Width), Height: float64(work.Height),
	})
	a.win.SetBounds(mygo.Rectangle{X: int(bounds.X), Y: int(bounds.Y), Width: int(bounds.Width), Height: int(bounds.Height)})
}
