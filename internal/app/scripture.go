package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
)

// The Scriptures tab: the reference field with its book suggestions, the
// verse list (whole chapters, multi-select) and the selected passage, which
// is what Preview, Go Live and Add to Schedule use.

// clearPassage forgets the looked-up passage (after a failed or empty
// lookup), so the verse list empties and nothing stale can be added to the
// schedule or sent live from it.
func (a *App) clearPassage() {
	if a.passage == nil && a.scriptureDeck == nil && a.verseRows == nil {
		return
	}
	a.passage, a.passageBible, a.scriptureDeck = nil, models.Bible{}, nil
	a.listVerses, a.verseRows = nil, nil
	a.versesVer++
	a.verseSel = core.NewRowSelection(nil)
}

// lookupReference parses a reference (models.ParseLocator: a book alone is
// its first chapter) and loads the whole chapters it names in the background.
// The verse list shows them with the referenced verses selected, and Preview
// shows the selected verses. A failed or empty lookup clears the previous
// passage; an ambiguous book ("jo") is not an error: the field's suggestions
// offer the candidates.
func (a *App) lookupReference(text string, cmd *control.Command) {
	a.reference = text
	text = strings.TrimSpace(text)
	ticket := a.verseGen.Next()
	if text == "" {
		a.clearPassage()
		a.setScriptStatus("", false)
		a.answer(cmd, nil)
		return
	}
	p, err := models.ParseLocator(text)
	if err != nil {
		a.clearPassage()
		var amb *models.AmbiguousBookError
		if errors.As(err, &amb) {
			a.setScriptStatus(core.ChooseBookText(models.BookCompletions(text)), false)
			a.answer(cmd, nil)
			return
		}
		a.setScriptStatus(err.Error(), true)
		a.answer(cmd, err)
		return
	}
	if a.bibleSel < 0 || a.bibleSel >= len(a.bibles) {
		a.clearPassage()
		a.setScriptStatus("No Bibles installed", true)
		a.answer(cmd, errors.New("no Bible installed"))
		return
	}
	if a.st == nil {
		a.clearPassage()
		a.answer(cmd, errNoLibrary)
		return
	}
	bible := a.bibles[a.bibleSel]
	st := a.st
	listed := p.Chapters()
	a.async(cmd, func(ctx context.Context) func() error {
		verses, err := st.Verses(ctx, bible.ID, listed)
		return func() error {
			if !a.verseGen.Current(ticket) {
				return nil
			}
			if err != nil {
				a.fail("look up "+p.String(), err)
				a.clearPassage()
				a.setScriptStatus("Could not load "+p.String(), true)
				return err
			}
			list := core.ListedVerses(verses)
			sel := core.VersesIn(list, p)
			if len(sel) == 0 { // no verses, or only empty ones
				a.clearPassage()
				a.setScriptStatus("No verses found for "+p.String()+" ("+bible.Abbrev+")", true)
				return fmt.Errorf("no verses found for %s (%s)", p.String(), bible.Abbrev)
			}
			a.passageBible = bible
			a.listVerses = list
			a.verseRows = core.VerseRows(list)
			a.versesVer++
			a.verseSel = core.NewRowSelection(sel)
			a.showSelection(&p)
			return nil
		}
	})
}

// showSelection makes the selected verses the passage: its deck goes to
// Preview (from its first verse) and the footer names it. p is the passage to
// call it (the typed reference after a lookup); nil derives it from the
// selected rows (models.SelectionPassage).
func (a *App) showSelection(p *models.Passage) {
	rows := a.verseSel.Rows
	if len(a.listVerses) == 0 || len(rows) == 0 {
		return
	}
	if p == nil {
		sp := models.SelectionPassage(a.listVerses[0].Book, a.listVerses, rows)
		p = &sp
	}
	verses := make([]models.Verse, 0, len(rows))
	for _, i := range rows {
		if i >= 0 && i < len(a.listVerses) {
			verses = append(verses, a.listVerses[i])
		}
	}
	deck := a.newScriptureDeck(*p, a.passageBible.Abbrev, verses)
	a.passage, a.scriptureDeck = p, &deck
	a.setScriptStatus(core.PassageStatus(p.String(), len(deck.Slides), a.passageBible.Abbrev), false)
	a.previewDeck(&deck)
}

// selectVerses applies a new verse selection from the list: Preview shows the
// selected verses, and the reference field names them ("John 3:16-18, 20").
func (a *App) selectVerses(sel core.RowSelection) {
	if len(sel.Rows) == 0 {
		return
	}
	if sel.Equal(a.verseSel) && a.scriptureDeck != nil {
		a.previewDeck(a.scriptureDeck)
		return
	}
	changed := !slices.Equal(sel.Rows, a.verseSel.Rows)
	a.verseSel = sel
	if !changed && a.scriptureDeck != nil {
		a.previewDeck(a.scriptureDeck) // only the cursor moved
		return
	}
	a.showSelection(nil)
	if a.passage != nil {
		a.setReferenceField(a.passage.String())
	}
}

// verseClick is a click on verse row (OnVerseClick).
func (a *App) verseClick(row int, extend, toggle bool) {
	a.selectVerses(a.verseSel.Click(row, len(a.listVerses), extend, toggle))
}

// verseStep is ↑/↓/Home/End in the verse list (OnVerseStep).
func (a *App) verseStep(delta int, extend bool) {
	a.selectVerses(a.verseSel.Step(delta, len(a.listVerses), extend))
}

// selectAllVerses is Cmd+A in the verse list.
func (a *App) selectAllVerses() { a.selectVerses(a.verseSel.All(len(a.listVerses))) }

// verseSelection is the slide of the selected passage that the resource
// preview shows and Go Live starts at (core.SelectedVerse): Preview's slide
// while Preview shows the passage, else the cursor's verse.
func (a *App) verseSelection() int {
	return core.SelectedVerse(a.scriptureDeck, a.pres.State.Preview, a.pres.State.PreviewIndex, a.verseSel.Index(a.verseSel.Cursor))
}

// currentVerseRow is the verse list row of verseSelection (-1 when none).
func (a *App) currentVerseRow() int {
	if i := a.verseSelection(); i >= 0 && i < len(a.verseSel.Rows) {
		return a.verseSel.Rows[i]
	}
	return -1
}

// goLiveVerse sends the selected verses live (the Go Live button and menu,
// Enter in the verse list), at the verse the resource preview shows.
func (a *App) goLiveVerse() { a.goLiveSelection(max(a.verseSelection(), 0)) }

// activateVerse is a double-click on verse row (or Enter, row -1): the
// selected verses go live, at the double-clicked one. A row outside the
// selection is selected first.
func (a *App) activateVerse(row int) {
	if row < 0 {
		a.goLiveVerse()
		return
	}
	if !a.verseSel.Has(row) {
		a.verseClick(row, false, false)
	}
	a.goLiveSelection(max(a.verseSel.Index(row), 0))
}

// goLiveSelection loads the selected passage into Preview and sends it live
// at slide i.
func (a *App) goLiveSelection(i int) {
	d := a.scriptureDeck
	if d == nil || i < 0 || i >= len(d.Slides) {
		return
	}
	a.previewDeck(d)
	a.liveChange(func() { a.pres.GoLiveSlide(i) })
}

// ---- the reference field and its book suggestions ------------------------------

// setReferenceField sets the reference field's text from the app (the
// selection, a picked suggestion, the reference action). render projects it.
func (a *App) setReferenceField(text string) {
	a.reference = text
	a.refEdited(text)
	a.refVer++
}

// refEdited follows the field's text (typing, or setReferenceField).
func (a *App) refEdited(text string) {
	if text != a.refText {
		a.suggestSel = 0
	}
	a.refText = text
	if text != a.suggestOff {
		a.suggestOff = ""
	}
}

// bookSuggestions are the books the reference field offers for its text.
func (a *App) bookSuggestions() []models.Book { return models.BookCompletions(a.refText) }

// suggestionsShown reports whether the dropdown is open: the field has the
// focus, there are suggestions and Esc did not dismiss them.
func (a *App) suggestionsShown() bool {
	return a.refFocused && (a.suggestOff == "" || a.suggestOff != a.refText) && len(a.bookSuggestions()) > 0
}

// moveSuggestion moves the highlighted suggestion (↑/↓), clamped.
func (a *App) moveSuggestion(delta int) {
	if n := len(a.bookSuggestions()); n > 0 {
		a.suggestSel = min(max(a.suggestSel+delta, 0), n-1)
	}
}

// pickSuggestion completes the field with suggestion i ("joh" → "John ") and
// looks the result up (a book alone shows its first chapter).
func (a *App) pickSuggestion(i int, cmd *control.Command) {
	books := a.bookSuggestions()
	if i < 0 || i >= len(books) {
		a.answer(cmd, fmt.Errorf("no suggestion %d (%d shown)", i, len(books)))
		return
	}
	text := models.CompleteBook(a.refText, books[i])
	a.setReferenceField(text)
	a.lookupReference(text, cmd)
}

// dismissSuggestions closes the dropdown until the text changes (Esc).
func (a *App) dismissSuggestions() { a.suggestOff = a.refText }

// listedText names the listed chapters ("John 3", "John 3-4").
func (a *App) listedText() string {
	if len(a.listVerses) == 0 {
		return ""
	}
	all := make([]int, len(a.listVerses))
	for i := range all {
		all[i] = i
	}
	return models.SelectionPassage(a.listVerses[0].Book, a.listVerses, all).String()
}

func bookNames(books []models.Book) []string {
	out := []string{}
	for _, b := range books {
		out = append(out, b.Name)
	}
	return out
}
