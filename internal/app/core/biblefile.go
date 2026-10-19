package core

import (
	"fmt"
	"path/filepath"
	"strings"

	"freeworship/internal/models"
)

// BibleFileQuestion is the confirmation shown before a Bible file is
// imported: it names the translation, its size and copyright status, and
// states that the user answers for the licence of a copyrighted text.
func BibleFileQuestion(b models.Bible, verses int, path string) string {
	name := b.Abbrev
	if b.Name != "" {
		name = b.Name + " (" + b.Abbrev + ")"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Import %s from %s: %s.\n\n", name, filepath.Base(path), VerseCountText(verses))
	switch {
	case models.IsPublicDomainBible(b.Abbrev, b.Name):
		sb.WriteString("This translation is in the public domain.")
	case b.Copyright != "":
		sb.WriteString("Copyright notice: " + b.Copyright + "\n\n")
		sb.WriteString("Import it only if your church is licensed to use it. You are responsible for holding a licence for any copyrighted text you import.")
	default:
		sb.WriteString("The file states no copyright. If this translation is copyrighted, import it only if your church is licensed to use it. You are responsible for holding a licence for any copyrighted text you import.")
	}
	sb.WriteString(" It stays in this computer's library.")
	return sb.String()
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
