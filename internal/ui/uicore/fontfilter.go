package uicore

import "strings"

// FilterFonts returns the indexes of the families in names that match the
// font picker's search query: every word of query appears in the name
// (case-insensitively), name-prefix matches first, then the rest in list
// order. The entry at keep (the theme's own font, -1 for none) is always
// listed, first when it does not match. An empty query lists everything.
func FilterFonts(names []string, query string, keep int) []int {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		out := make([]int, len(names))
		for i := range names {
			out[i] = i
		}
		return out
	}
	var prefix, other []int
	for i, n := range names {
		l := strings.ToLower(n)
		ok := true
		for _, w := range words {
			if !strings.Contains(l, w) {
				ok = false
				break
			}
		}
		switch {
		case !ok:
		case strings.HasPrefix(l, words[0]):
			prefix = append(prefix, i)
		default:
			other = append(other, i)
		}
	}
	out := append(prefix, other...)
	if keep >= 0 && keep < len(names) {
		for _, i := range out {
			if i == keep {
				return out
			}
		}
		out = append([]int{keep}, out...)
	}
	return out
}
