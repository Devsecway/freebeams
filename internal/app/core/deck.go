package core

import (
	"fmt"
	"reflect"
	"strings"

	"freebeams/internal/models"
)

// SongDeck builds the displayable deck for a library song (not in the
// schedule: ItemID is empty). Phase 1 always uses the default song theme.
func SongDeck(song models.Song) models.Deck {
	return models.Deck{
		Title:  song.Title,
		Theme:  models.DefaultSongTheme(),
		Slides: song.Slides(models.DefaultSlideOptions()),
	}
}

// ScriptureDeck builds the deck for a looked-up passage: one verse per slide
// with a "John 3:16 (KJV)" footer, titled with the passage reference.
func ScriptureDeck(p models.Passage, bibleAbbrev string, verses []models.Verse) models.Deck {
	return models.Deck{
		Title:  p.String(),
		Theme:  models.DefaultScriptureTheme(),
		Slides: models.ScriptureSlides(p, verses, bibleAbbrev, models.OneVersePerSlide),
	}
}

// SongItem makes a schedule item for a song from its deck. The slides and
// theme are a snapshot, so the schedule still opens when the library changes.
func SongItem(song models.Song, deck models.Deck) models.ScheduleItem {
	return models.ScheduleItem{
		Kind:     models.ItemSong,
		Title:    song.Title,
		Subtitle: song.Author,
		SongID:   song.ID,
		Theme:    deck.Theme,
		Slides:   cloneSlides(deck.Slides),
	}
}

// ScriptureItem makes a schedule item for a passage.
func ScriptureItem(p models.Passage, bible models.Bible, deck models.Deck) models.ScheduleItem {
	pp := p
	pp.Ranges = append([]models.VerseRange(nil), p.Ranges...)
	sub := "Scripture"
	if bible.Abbrev != "" {
		sub = bible.Abbrev + " · Scripture"
	}
	return models.ScheduleItem{
		Kind:     models.ItemScripture,
		Title:    deck.Title,
		Subtitle: sub,
		Passage:  &pp,
		BibleID:  bible.ID,
		Theme:    deck.Theme,
		Slides:   cloneSlides(deck.Slides),
	}
}

// ItemDeck converts a schedule item to the deck shown in Preview/Live. The
// deck's ItemID is the item's ID, which is how the schedule marks the live item.
func ItemDeck(item models.ScheduleItem) *models.Deck {
	return &models.Deck{
		ItemID: item.ID,
		Title:  item.Title,
		Theme:  item.Theme,
		Slides: cloneSlides(item.Slides),
	}
}

func cloneSlides(in []models.Slide) []models.Slide {
	if in == nil {
		return nil
	}
	out := make([]models.Slide, len(in))
	for i, s := range in {
		if s.Background != nil {
			bg := *s.Background
			s.Background = &bg
		}
		out[i] = s
	}
	return out
}

// LiveItemIndex returns the schedule index of the live deck (matched by
// Deck.ItemID), or -1 when nothing from the schedule is live.
func LiveItemIndex(s *models.Schedule, live *models.Deck) int {
	if s == nil || live == nil || live.ItemID == "" {
		return -1
	}
	return s.IndexOf(live.ItemID)
}

// StepIndex moves a selection by delta within n items. From no selection
// (cur < 0) a forward step selects the first item and a backward step the
// last. ok is false when there is nowhere to go (empty list, or already at
// the end in that direction).
func StepIndex(cur, n, delta int) (int, bool) {
	if n <= 0 || delta == 0 {
		return cur, false
	}
	if cur < 0 || cur >= n {
		if delta > 0 {
			return 0, true
		}
		return n - 1, true
	}
	next := cur + delta
	if next < 0 || next >= n {
		return cur, false
	}
	return next, true
}

// SameDeck reports whether two decks show the same thing: the same schedule
// item (or both library content), title, theme and slides.
func SameDeck(a, b *models.Deck) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a == b || (a.ItemID == b.ItemID && a.Title == b.Title && a.Theme == b.Theme && reflect.DeepEqual(a.Slides, b.Slides))
}

// SelectedVerse is the selected verse of the looked-up passage: the one the
// verse list highlights, the resource preview shows and Go Live sends. While
// Preview shows the passage itself it is Preview's selected slide (moving in
// the Preview grid moves the verse selection); otherwise it is lastRow, the
// verse last chosen in the verse list. -1 when there is no passage.
func SelectedVerse(passage, preview *models.Deck, previewIndex, lastRow int) int {
	if passage == nil || len(passage.Slides) == 0 {
		return -1
	}
	if preview != nil && preview.ItemID == "" && SameDeck(passage, preview) {
		return previewIndex
	}
	if lastRow >= len(passage.Slides) {
		return -1
	}
	return lastRow
}

// ScheduleSelection is the schedule row that matches the Preview deck: the
// item Preview was loaded from, or -1 when Preview holds library content (a
// song, a passage) or nothing.
func ScheduleSelection(s *models.Schedule, preview *models.Deck) int {
	return LiveItemIndex(s, preview)
}

// GridLabel is one Preview/Live grid tile label.
type GridLabel struct {
	Label  string
	Number int
}

// GridLabels returns the tile labels of a deck (1-based numbers).
func GridLabels(d *models.Deck) []GridLabel {
	if d == nil {
		return nil
	}
	out := make([]GridLabel, len(d.Slides))
	for i, s := range d.Slides {
		out[i] = GridLabel{Label: s.Label, Number: i + 1}
	}
	return out
}

// VerseRows formats verse list rows: "16  For God so loved…". Verses from a
// second chapter (cross-chapter passages) are shown as "4:1  …".
func VerseRows(verses []models.Verse) []string {
	rows := make([]string, 0, len(verses))
	first := 0
	if len(verses) > 0 {
		first = verses[0].Chapter
	}
	for _, v := range verses {
		text := strings.Join(strings.Fields(v.Text), " ")
		if text == "" {
			continue // ScriptureSlides skips these too, keeping rows == slides
		}
		num := fmt.Sprint(v.Verse)
		if v.Chapter != first {
			num = fmt.Sprintf("%d:%d", v.Chapter, v.Verse)
		}
		rows = append(rows, num+"  "+text)
	}
	return rows
}

// SlideCaption is the resource preview caption: "Verse 1 · 1 / 6".
func SlideCaption(d *models.Deck, i int) string {
	if d == nil || i < 0 || i >= len(d.Slides) {
		return ""
	}
	label := d.Slides[i].Label
	pos := fmt.Sprintf("%d / %d", i+1, len(d.Slides))
	if label == "" {
		return pos
	}
	return label + " · " + pos
}

// FindSong returns the index of the song in rows whose title matches title:
// an exact (case-insensitive) match first, then a prefix, then a substring
// match. -1 when none.
func FindSong(rows []models.SongSummary, title string) int {
	t := strings.ToLower(strings.TrimSpace(title))
	if t == "" {
		return -1
	}
	for _, match := range []func(string) bool{
		func(s string) bool { return s == t },
		func(s string) bool { return strings.HasPrefix(s, t) },
		func(s string) bool { return strings.Contains(s, t) },
	} {
		for i, r := range rows {
			if match(strings.ToLower(r.Title)) {
				return i
			}
		}
	}
	return -1
}

// FindBible returns the index of the Bible with the given abbreviation (or
// name), case-insensitively; -1 when none.
func FindBible(bibles []models.Bible, abbrev string) int {
	a := strings.TrimSpace(abbrev)
	if a == "" {
		return -1
	}
	for i, b := range bibles {
		if strings.EqualFold(b.Abbrev, a) || strings.EqualFold(b.Name, a) {
			return i
		}
	}
	return -1
}

// DefaultBible picks the initial translation: KJV when installed, else the first.
func DefaultBible(bibles []models.Bible) int {
	if i := FindBible(bibles, "KJV"); i >= 0 {
		return i
	}
	if len(bibles) > 0 {
		return 0
	}
	return -1
}

// SongsFooter is the Songs footer text for n listed songs. A running import
// says so. A failed one stays visible next to the count, so a later search
// cannot hide it.
func SongsFooter(n int, imp ImportState) string {
	return SongsFooterMatches(n, 0, imp)
}

// SongsFooterMatches is SongsFooter for a search result: lyrics is the number
// of listed songs that needed their lyrics to match ("5 songs · 2 matched in
// lyrics"), which tells the operator why those rows are there.
func SongsFooterMatches(n, lyrics int, imp ImportState) string {
	if imp.Running {
		return "Importing from EasyWorship…"
	}
	text := SongCountText(n)
	if lyrics > 0 {
		text += fmt.Sprintf(" · %d matched in lyrics", lyrics)
	}
	if imp.Error != "" {
		text += " · EasyWorship import failed"
	}
	return text
}

// LyricsMatches counts the search results that needed their lyrics to match.
func LyricsMatches(rows []models.SongSummary) int {
	n := 0
	for _, r := range rows {
		if r.Matched == models.MatchedLyrics {
			n++
		}
	}
	return n
}

// FirstSongTitle is the title of the first listed song ("" for none): the
// row a search ranks first.
func FirstSongTitle(rows []models.SongSummary) string {
	if len(rows) == 0 {
		return ""
	}
	return rows[0].Title
}

// SongCountText is the song count ("1 song", "223 songs").
func SongCountText(n int) string {
	if n == 1 {
		return "1 song"
	}
	return fmt.Sprintf("%d songs", n)
}
