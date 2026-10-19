package uicore

import "freebeams/internal/models"

// SongMatchHint is the song list's Match column text and row tooltip for a
// search result: where the song matched when that was not its title
// ("author", "lyrics"), and for a lyrics match the excerpt. Both are "" for
// a title match and for a plain listing, so the column stays quiet unless a
// row is there for a reason the operator cannot see.
func SongMatchHint(matched, snippet string) (cell, tooltip string) {
	switch matched {
	case models.MatchedAuthor:
		return "author", "Matched in the author"
	case models.MatchedLyrics:
		if snippet == "" {
			return "lyrics", "Matched in the lyrics"
		}
		return "lyrics", "Matched in the lyrics:\n" + snippet
	}
	return "", ""
}
