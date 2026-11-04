package clipboard

import "testing"

func TestHistoryFilters(t *testing.T) {
	entries := []Entry{
		{ID: "a", Kind: KindText, Text: "Go", Source: "Safari"},
		{ID: "b", Kind: KindImage, Favorite: true},
		{ID: "c", Kind: KindFiles, Files: []string{"/tmp/report.pdf"}},
	}
	for _, tc := range []struct {
		tab   int
		query string
		count int
	}{
		{0, "", 3}, {1, "", 1}, {2, "", 2}, {3, "", 1},
		{0, "SAFARI", 1}, {2, "report", 1}, {2, "图片", 1}, {1, "report", 0},
	} {
		if got := Filter(entries, tc.tab, tc.query); len(got) != tc.count {
			t.Fatalf("filter (%d, %q) = %+v", tc.tab, tc.query, got)
		}
	}
}
