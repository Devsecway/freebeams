package core

import (
	"fmt"
	"path/filepath"
	"strings"

	"freebeams/internal/models"
)

// Licence sentences of BibleFileQuestion. The confirm dialog is a fixed
// 420 × 150 window (ui.ConfirmDialog), so the whole question must stay
// within about four lines.
const (
	bibleFileLicensed   = "Import it only if your church holds a licence for it; you are responsible for that. It stays in this computer's library."
	bibleFileUnknown    = "If it is copyrighted, import it only if your church holds a licence for it; you are responsible for that. It stays in this computer's library."
	bibleFilePublic     = "This translation is in the public domain. It stays in this computer's library."
	bibleFileNoticeRune = 60 // the copyright notice is cut to this many characters
)

// BibleFileQuestion is the confirmation shown before a Bible file is
// imported: it names the translation, its size and copyright status, and
// states that the user answers for the licence of a copyrighted text.
func BibleFileQuestion(b models.Bible, verses int, path string) string {
	name := b.Abbrev
	if b.Name != "" {
		name = b.Name + " (" + b.Abbrev + ")"
	}
	head := fmt.Sprintf("Import %s from %s: %s", name, filepath.Base(path), VerseCountText(verses))
	notice := firstLine(b.Copyright, bibleFileNoticeRune)
	switch {
	case models.IsPublicDomainBible(b.Abbrev, b.Name) || strings.EqualFold(notice, "public domain"):
		return head + ".\n\n" + bibleFilePublic
	case notice != "":
		return head + " (" + notice + ").\n\n" + bibleFileLicensed
	default:
		return head + "; the file states no copyright.\n\n" + bibleFileUnknown
	}
}

// firstLine is the first non-empty line of s with its whitespace collapsed,
// cut to max characters with an ellipsis.
func firstLine(s string, max int) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > max {
			return strings.TrimSpace(string(r[:max-1])) + "…"
		}
		return line
	}
	return ""
}

// BibleFileSummary is the Scriptures footer text after a Bible file import.
func BibleFileSummary(b models.Bible, verses int) string {
	return "Imported " + b.Abbrev + ": " + VerseCountText(verses)
}

// VerseCountText is a verse count ("1 verse", "31,102 verses").
func VerseCountText(n int) string {
	if n == 1 {
		return "1 verse"
	}
	return groupThousands(n) + " verses"
}

// groupThousands formats n with thousands separators (31102 -> "31,102").
func groupThousands(n int) string {
	if n < 0 {
		return "-" + groupThousands(-n)
	}
	s := fmt.Sprint(n)
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}
