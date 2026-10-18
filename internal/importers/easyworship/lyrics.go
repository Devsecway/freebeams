package easyworship

import (
	"regexp"
	"strconv"
	"strings"

	"freeworship/internal/models"
)

// EasyWorship lyrics (after RTF -> text) are plain lines. A line that is just
// a section label ("Verse 1", "Chorus 1", "Bridge", ...) starts a section, and
// a blank line is a slide break. EW does not mark sections in the RTF itself:
// labels are ordinary paragraphs.

// labelRE matches a whole line that is a section label, optionally wrapped in
// brackets and/or followed by a colon: "Verse 1", "[Chorus]", "Pre-Chorus 2:",
// "V1", "C", "Bridge 2".
var labelRE = regexp.MustCompile(`(?i)^\[?\s*(verse|chorus|refrain|pre-?\s?chorus|bridge|tag|intro|ending|outro|coda|v|c|b|pc)\s*(\d{1,2})?\s*\]?\s*:?$`)

type label struct {
	kind   models.SectionKind
	number int
	text   string
}

// parseLabel reports whether line is a section label.
func parseLabel(line string) (label, bool) {
	m := labelRE.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return label{}, false
	}
	var kind models.SectionKind
	switch w := strings.ToLower(strings.Join(strings.Fields(m[1]), "")); w {
	case "verse", "v":
		kind = models.SectionVerse
	case "chorus", "refrain", "c":
		kind = models.SectionChorus
	case "prechorus", "pre-chorus", "pc":
		kind = models.SectionPreChorus
	case "bridge", "b":
		kind = models.SectionBridge
	case "tag":
		kind = models.SectionTag
	case "intro":
		kind = models.SectionIntro
	default: // ending, outro, coda
		kind = models.SectionEnding
	}
	n := 0
	if m[2] != "" {
		n, _ = strconv.Atoi(m[2])
	}
	return label{kind: kind, number: n, text: sectionLabel(kind, n)}, true
}

// sectionLabel mirrors models.SectionLabel ("Verse 2", "Chorus", "Pre-Chorus").
// It is duplicated here so the importer does not depend on the models package
// implementation being present.
func sectionLabel(kind models.SectionKind, n int) string {
	var name string
	switch kind {
	case models.SectionVerse:
		name = "Verse"
	case models.SectionChorus:
		name = "Chorus"
	case models.SectionPreChorus:
		name = "Pre-Chorus"
	case models.SectionBridge:
		name = "Bridge"
	case models.SectionTag:
		name = "Tag"
	case models.SectionIntro:
		name = "Intro"
	case models.SectionEnding:
		name = "Ending"
	default:
		name = "Other"
	}
	if n > 0 {
		return name + " " + strconv.Itoa(n)
	}
	return name
}

// parseLyrics splits EasyWorship lyric text into sections.
//
//   - A label line starts a new section; following lines belong to it.
//   - A blank line ends the current slide. Lines after it (before the next
//     label) form another Section with the same Kind/Number/Label, so a
//     labelled section spanning two slides becomes two consecutive Sections.
//   - When the text has no labels at all, each blank-line separated stanza
//     becomes Verse 1, Verse 2, ...
//   - Stanzas before the first label in labelled text become unnumbered "Verse".
//
// Lines are trimmed; empty sections are dropped. Labels never appear in Lines.
func parseLyrics(text string) []models.Section {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")

	hasLabels := false
	for _, l := range lines {
		if _, ok := parseLabel(l); ok {
			hasLabels = true
			break
		}
	}

	var (
		out     []models.Section
		cur     *label // nil until the first label
		stanza  []string
		autoNum int
	)
	flush := func() {
		if len(stanza) == 0 {
			return
		}
		sec := models.Section{Lines: stanza}
		switch {
		case cur != nil:
			sec.Kind, sec.Number, sec.Label = cur.kind, cur.number, cur.text
		case hasLabels:
			sec.Kind, sec.Label = models.SectionVerse, sectionLabel(models.SectionVerse, 0)
		default:
			autoNum++
			sec.Kind, sec.Number, sec.Label = models.SectionVerse, autoNum, sectionLabel(models.SectionVerse, autoNum)
		}
		out = append(out, sec)
		stanza = nil
	}
	for _, raw := range lines {
		line := strings.TrimSpace(strings.ReplaceAll(raw, "\u00A0", " "))
		if line == "" {
			flush()
			continue
		}
		if l, ok := parseLabel(line); ok {
			flush()
			cur = &l
			continue
		}
		stanza = append(stanza, line)
	}
	flush()
	return out
}
