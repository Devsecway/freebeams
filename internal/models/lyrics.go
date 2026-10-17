package models

import (
	"strconv"
	"strings"
	"unicode"
)

// Lyric text format
//
// ParseLyrics understands three kinds of section header line:
//
//   - Bracket tags: "[Verse 1]", "[V1]", "[Chorus]", "[C2]", "[Pre-Chorus]", "[Bridge]",
//     "[Tag]", "[Intro]", "[Ending]", "[Outro]", ... Short forms (V, C, P/PC, B, T, I, E/O)
//     are only accepted inside brackets. A bracket tag that is not a known section
//     ("[Vamp]", "[Repeat x2]") starts a SectionOther section whose Label is the tag text.
//   - Colon headers: a line such as "Verse 1:" or "Chorus:" (long forms only).
//   - Bare headers: a line that is exactly a long-form label ("Verse 2", "Chorus",
//     "Pre-Chorus", "Bridge", ...) at the start of a stanza (first line of the text
//     or directly after a blank line). EasyWorship's own lyric text uses this form.
//
// Text before the first header is split on blank lines into Verse 1, Verse 2, ...
//
// Inside a tagged section, a blank line starts a new slide of the same section
// (EasyWorship behaviour). This is represented by two consecutive Sections with
// identical Kind, Number and Label. FormatLyrics writes such runs back as one
// header followed by blank-line separated stanzas.
//
// Known sections always get the canonical label from SectionLabel ("[Outro]" and
// "[Refrain]" become "Ending" and "Chorus"), so a Section's Kind/Number and its
// Label never disagree.

// sectionAliases maps a normalised tag word (lower case, without spaces, hyphens,
// underscores or dots) to a section kind. long reports whether the word is a
// long form, which is also accepted for colon and bare header lines.
var sectionAliases = map[string]struct {
	kind SectionKind
	long bool
}{
	"verse":     {SectionVerse, true},
	"v":         {SectionVerse, false},
	"vs":        {SectionVerse, false},
	"chorus":    {SectionChorus, true},
	"refrain":   {SectionChorus, true},
	"c":         {SectionChorus, false},
	"ch":        {SectionChorus, false},
	"prechorus": {SectionPreChorus, true},
	"pre":       {SectionPreChorus, false},
	"pc":        {SectionPreChorus, false},
	"prc":       {SectionPreChorus, false},
	"p":         {SectionPreChorus, false},
	"bridge":    {SectionBridge, true},
	"b":         {SectionBridge, false},
	"br":        {SectionBridge, false},
	"tag":       {SectionTag, true},
	"t":         {SectionTag, false},
	"intro":     {SectionIntro, true},
	"i":         {SectionIntro, false},
	"ending":    {SectionEnding, true},
	"outro":     {SectionEnding, true},
	"end":       {SectionEnding, false},
	"e":         {SectionEnding, false},
	"o":         {SectionEnding, false},
}

// maxSectionNumber bounds section numbers; larger numbers make the tag a custom
// (SectionOther) label instead.
const maxSectionNumber = 999

// sectionTag is a parsed header label.
type sectionTag struct {
	kind   SectionKind
	number int
	label  string
}

// parseSectionLabel classifies a label such as "Verse 2", "v2", "Pre-Chorus" or
// "Vamp". ok is false for an empty label. long reports whether the label used a
// long form (only those are accepted as colon or bare headers); it is false for
// SectionOther.
func parseSectionLabel(label string) (tag sectionTag, long, ok bool) {
	label = strings.TrimSpace(label)
	if label == "" {
		return sectionTag{}, false, false
	}
	other := sectionTag{kind: SectionOther, label: label}

	// Normalise: lower case, drop separators.
	var b strings.Builder
	for _, r := range strings.ToLower(label) {
		switch {
		case r == ' ' || r == '-' || r == '_' || r == '.' || unicode.IsSpace(r):
		default:
			b.WriteRune(r)
		}
	}
	norm := b.String()

	// Split the trailing number off ("verse12" -> "verse", 12).
	i := len(norm)
	for i > 0 && norm[i-1] >= '0' && norm[i-1] <= '9' {
		i--
	}
	word, digits := norm[:i], norm[i:]
	alias, known := sectionAliases[word]
	if !known {
		return other, false, true
	}
	number := 0
	if digits != "" {
		if len(digits) > 4 {
			return other, false, true
		}
		n, err := strconv.Atoi(digits)
		if err != nil || n > maxSectionNumber {
			return other, false, true
		}
		number = n
	}
	return sectionTag{kind: alias.kind, number: number, label: SectionLabel(alias.kind, number)}, alias.long, true
}

// headerTag reports whether line (already trimmed) is a section header.
// stanzaStart is true when the line is the first of the text or follows a blank
// line; only then are bare headers ("Chorus") recognised.
func headerTag(line string, stanzaStart bool) (sectionTag, bool) {
	if len(line) >= 2 && line[0] == '[' && line[len(line)-1] == ']' {
		tag, _, ok := parseSectionLabel(line[1 : len(line)-1])
		return tag, ok
	}
	if strings.HasSuffix(line, ":") {
		if tag, long, ok := parseSectionLabel(strings.TrimSuffix(line, ":")); ok && long {
			return tag, true
		}
		return sectionTag{}, false
	}
	if stanzaStart {
		if tag, long, ok := parseSectionLabel(line); ok && long {
			return tag, true
		}
	}
	return sectionTag{}, false
}

// isHeaderLine reports whether line would be read as a header at a stanza start.
func isHeaderLine(line string) bool {
	_, ok := headerTag(strings.TrimSpace(line), true)
	return ok
}

// ParseLyrics parses lyric text into sections. Accepted forms:
//   - Tagged:   "[Verse 1]\nline\nline\n\n[Chorus]\nline" (tags case-insensitive,
//     also "Verse 1:" / "Chorus:" header lines, "V1", "C", "Pre-Chorus", "Bridge 2", "Tag", "Intro", "Ending"/"Outro")
//   - Untagged: blank-line separated stanzas become Verse 1, Verse 2, ...
//
// Leading/trailing whitespace is trimmed, CRLF is normalised, empty sections dropped.
// A blank line inside a tagged section starts a new slide of the same section,
// returned as a further Section with the same Kind, Number and Label. See the
// "Lyric text format" comment in lyrics.go for the full grammar.
func ParseLyrics(text string) []Section {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var (
		sections    []Section
		cur         *sectionTag // current tagged section; nil before the first header
		lines       []string    // lines of the stanza being collected
		autoVerse   int         // last auto-numbered verse for untagged stanzas
		stanzaStart = true
	)
	flush := func() {
		if len(lines) == 0 {
			return
		}
		var tag sectionTag
		if cur != nil {
			tag = *cur
		} else {
			autoVerse++
			tag = sectionTag{kind: SectionVerse, number: autoVerse, label: SectionLabel(SectionVerse, autoVerse)}
		}
		sections = append(sections, Section{Kind: tag.kind, Number: tag.number, Label: tag.label, Lines: lines})
		lines = nil
	}

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			flush()
			stanzaStart = true
			continue
		}
		if tag, ok := headerTag(line, stanzaStart); ok {
			flush()
			cur = &tag
			stanzaStart = false
			continue
		}
		lines = append(lines, line)
		stanzaStart = false
	}
	flush()
	return sections
}

// FormatLyrics is the inverse of ParseLyrics: "[Label]\nlines" blocks joined by a
// blank line. ParseLyrics(FormatLyrics(s)) must round-trip.
//
// Known kinds are written with their canonical SectionLabel; SectionOther uses
// Label. Consecutive sections with the same Kind/Number/Label are written as one
// header followed by blank-line separated stanzas (the slide-break form), unless a
// stanza's first line would itself read as a header. Blank lines inside
// Section.Lines are dropped and sections without text are skipped, so the
// round-trip holds for any sections without header-like lyric lines, and always
// for sections produced by ParseLyrics.
func FormatLyrics(sections []Section) string {
	var b strings.Builder
	var prev *Section
	for i := range sections {
		s := &sections[i]
		var lines []string
		for _, l := range s.Lines {
			for _, part := range strings.Split(strings.ReplaceAll(l, "\r", "\n"), "\n") {
				if t := strings.TrimSpace(part); t != "" {
					lines = append(lines, t)
				}
			}
		}
		if len(lines) == 0 {
			continue
		}
		label := formatLabel(*s)
		continuation := prev != nil && sameSection(*prev, *s) && !isHeaderLine(lines[0])
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		if !continuation {
			b.WriteString("[" + label + "]\n")
		}
		b.WriteString(strings.Join(lines, "\n"))
		prev = s
	}
	return b.String()
}

// formatLabel returns the header label FormatLyrics writes for s.
func formatLabel(s Section) string {
	if s.Kind == SectionOther || SectionLabel(s.Kind, 0) == "" {
		if l := strings.TrimSpace(s.Label); l != "" {
			return l
		}
	}
	if l := SectionLabel(s.Kind, s.Number); l != "" {
		return l
	}
	return SectionLabel(SectionOther, s.Number)
}

// sameSection reports whether b continues a (another slide of the same section).
func sameSection(a, b Section) bool {
	return a.Kind == b.Kind && a.Number == b.Number && formatLabel(a) == formatLabel(b)
}

// sectionNames are the display names of the known section kinds.
var sectionNames = map[SectionKind]string{
	SectionVerse:     "Verse",
	SectionChorus:    "Chorus",
	SectionPreChorus: "Pre-Chorus",
	SectionBridge:    "Bridge",
	SectionTag:       "Tag",
	SectionIntro:     "Intro",
	SectionEnding:    "Ending",
	SectionOther:     "Other",
}

// SectionLabel returns the display label: (verse,2) -> "Verse 2", (chorus,0) -> "Chorus",
// (prechorus,0) -> "Pre-Chorus". Unknown kinds are title-cased; an empty kind
// returns "".
func SectionLabel(kind SectionKind, number int) string {
	name, ok := sectionNames[kind]
	if !ok {
		k := strings.TrimSpace(string(kind))
		if k == "" {
			return ""
		}
		r := []rune(k)
		name = string(unicode.ToUpper(r[0])) + string(r[1:])
	}
	if number > 0 {
		return name + " " + strconv.Itoa(number)
	}
	return name
}

// Slides splits the song into slides (one per section, or more when a section
// exceeds opt.MaxLines), setting Kind=SlideSong, Label, Group, GroupNumber, Text,
// and the copyright Footer per opt.
//
// A section longer than MaxLines is split into ceil(n/MaxLines) slides whose line
// counts differ by at most one, longer slides first (7 lines, max 4 -> 4+3;
// 8 lines, max 3 -> 3+3+2). Every slide of a section carries the section's label.
// Sections without text are skipped. The footer (see CopyrightFooter) goes on
// the last slide, or on every slide with CopyrightEverySlide.
func (s Song) Slides(opt SlideOptions) []Slide {
	var slides []Slide
	for _, sec := range s.Sections {
		if !hasText(sec.Lines) {
			continue
		}
		label := sec.Label
		if label == "" {
			label = SectionLabel(sec.Kind, sec.Number)
		}
		for _, chunk := range splitEven(sec.Lines, opt.MaxLines) {
			slides = append(slides, Slide{
				Kind:        SlideSong,
				Label:       label,
				Group:       sec.Kind,
				GroupNumber: sec.Number,
				Text:        strings.Join(chunk, "\n"),
			})
		}
	}
	if opt.IncludeCopyright && len(slides) > 0 {
		if footer := s.CopyrightFooter(); footer != "" {
			if opt.CopyrightEverySlide {
				for i := range slides {
					slides[i].Footer = footer
				}
			} else {
				slides[len(slides)-1].Footer = footer
			}
		}
	}
	return slides
}

// hasText reports whether any line is non-blank.
func hasText(lines []string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			return true
		}
	}
	return false
}

// splitEven splits lines into chunks of at most max lines (max <= 0: one chunk),
// as evenly as possible with the longer chunks first.
func splitEven(lines []string, max int) [][]string {
	n := len(lines)
	if max <= 0 || n <= max {
		return [][]string{lines}
	}
	chunks := (n + max - 1) / max
	base, extra := n/chunks, n%chunks
	out := make([][]string, 0, chunks)
	start := 0
	for i := 0; i < chunks; i++ {
		size := base
		if i < extra {
			size++
		}
		out = append(out, lines[start:start+size])
		start += size
	}
	return out
}

// CopyrightFooter returns the one-line song footer shown on slides:
//
//	"Amazing Grace" by John Newton · © 1779 Public Domain · CCLI #22025
//
// Empty parts are omitted (no author: `"Amazing Grace" · © ...`; no title:
// `John Newton · © ...`). A leading "©", "(c)" or "Copyright" in the copyright
// text is replaced by a single "©", and a leading "#" in the CCLI number is
// dropped. It returns "" when the song has none of these fields.
func (s Song) CopyrightFooter() string {
	var parts []string
	title, author := strings.TrimSpace(s.Title), strings.TrimSpace(s.Author)
	switch {
	case title != "" && author != "":
		parts = append(parts, `"`+title+`" by `+author)
	case title != "":
		parts = append(parts, `"`+title+`"`)
	case author != "":
		parts = append(parts, author)
	}
	if c := stripCopyrightMark(s.Copyright); c != "" {
		parts = append(parts, "© "+c)
	}
	if n := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s.CCLI), "#")); n != "" {
		parts = append(parts, "CCLI #"+n)
	}
	return strings.Join(parts, " · ")
}

// stripCopyrightMark removes leading "©", "(c)" and "copyright" markers.
func stripCopyrightMark(c string) string {
	c = strings.TrimSpace(c)
	for {
		switch {
		case strings.HasPrefix(c, "©"):
			c = c[len("©"):]
		case hasPrefixFold(c, "(c)"):
			c = c[len("(c)"):]
		case hasPrefixFold(c, "copyright"):
			c = c[len("copyright"):]
		default:
			return c
		}
		c = strings.TrimSpace(c)
	}
}

// hasPrefixFold is a case-insensitive strings.HasPrefix for an ASCII prefix.
func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}
