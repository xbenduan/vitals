package platform

import "testing"

func TestPanelBounds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		work  Bounds
		x, y  float64
		scale float64
		want  Bounds
	}{
		{"beside cursor", Bounds{0, 24, 1920, 1016}, 300, 150, 1, Bounds{312, 162, 480, 588}},
		{"bottom right", Bounds{0, 24, 1920, 1016}, 1910, 1030, 1, Bounds{1418, 430, 480, 588}},
		{"left monitor", Bounds{-1920, 0, 1920, 1080}, -1800, 100, 1, Bounds{-1788, 112, 480, 588}},
		{"upper monitor", Bounds{0, -1080, 1920, 1080}, 100, -1000, 1, Bounds{112, -988, 480, 588}},
		{"tiny work area", Bounds{0, 40, 320, 400}, 160, 200, 1, Bounds{0, 40, 320, 400}},
		{"left taskbar", Bounds{80, 0, 1840, 1080}, 20, 20, 1, Bounds{80, 32, 480, 588}},
		{"150 percent secondary", Bounds{1920, 0, 2560, 1380}, 4400, 1300, 1.5, Bounds{3662, 400, 720, 882}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := PanelBounds(tc.x, tc.y, 480*tc.scale, 588*tc.scale, 12*tc.scale, tc.work)
			if got != tc.want {
				t.Fatalf("bounds = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPasteDestinationIdentity(t *testing.T) {
	original := Target{PID: 10, Window: 100, Name: "Editor"}
	if !original.Matches(Target{PID: 10, Window: 100, Name: "New display name"}) {
		t.Fatal("display names must not change destination identity")
	}
	for _, other := range []Target{{}, {PID: 10, Window: 101}, {PID: 11, Window: 100}} {
		if original.Matches(other) {
			t.Fatalf("different destination matched: %+v", other)
		}
	}
	if (Target{}).Matches(Target{}) {
		t.Fatal("two missing targets must not match")
	}
	if !(Target{PID: 10}).Matches(Target{PID: 10}) {
		t.Fatal("macOS process targets must match without window handles")
	}
}
