package clipboard

import "strings"

// Filter selects entries by tab and a case-insensitive content, path, or source query.
// Tabs are 0 for all, 1 for text, 2 for images/files, and 3 for favorites.
// Results retain input order and share immutable file lists with the input.
func Filter(entries []Entry, tab int, query string) []Entry {
	query = strings.ToLower(strings.TrimSpace(query))
	var result []Entry
	for _, entry := range entries {
		if tab == 1 && entry.Kind != KindText || tab == 2 && entry.Kind == KindText || tab == 3 && !entry.Favorite {
			continue
		}
		searchable := entry.Text + "\n" + strings.Join(entry.Files, "\n") + "\n" + entry.Source
		if entry.Kind == KindImage {
			searchable += "\n图片 PNG"
		}
		if query == "" || strings.Contains(strings.ToLower(searchable), query) {
			result = append(result, entry)
		}
	}
	return result
}
