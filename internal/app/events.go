package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"

	"freeworship/internal/app/core"
	"freeworship/internal/control"
	"freeworship/internal/models"
	"freeworship/internal/ui"
)

// Event handlers. They mutate state only; Process renders afterwards.

// ---- presenting ------------------------------------------------------------------

// liveChange runs fn (which changes what is live) and remembers the previous
// live item for Go Back.
func (a *App) liveChange(fn func()) {
	prev, prevIdx := a.pres.State.Live, a.pres.State.LiveIndex
	fn()
	cur := a.pres.State.Live
	if prev != nil && cur != prev && (cur == nil || cur.ItemID != prev.ItemID || cur.Title != prev.Title) {
		a.prevLive, a.prevLiveIndex = prev, prevIdx
	}
}

// goLive sends the preview live (Go Live button, Enter).
func (a *App) goLive() { a.liveChange(a.pres.GoLive) }

// goBack puts the previously live item back on air.
func (a *App) goBack() {
	if a.prevLive == nil {
		return
	}
	d, i := a.prevLive, a.prevLiveIndex
	a.pres.SetPreview(d)
	a.liveChange(func() { a.pres.GoLiveSlide(i) })
}

// setOutput turns the projector output on or off (the Live toggle).
func (a *App) setOutput(on bool) { a.pres.SetOutputEnabled(on) }

func (a *App) onToolbar(act ui.ToolbarAction) {
	switch act {
	case ui.ActionNewSchedule:
		a.confirmDiscard("New Schedule", a.newSchedule)
	case ui.ActionNewSong:
		a.newSong()
	case ui.ActionOpen:
		a.confirmDiscard("Open Schedule", a.openScheduleDialog)
	case ui.ActionSave:
		a.saveScheduleInteractive()
	case ui.ActionGoLive:
		a.goLive()
	case ui.ActionLogo:
		a.pres.ToggleLogo()
	case ui.ActionBlack:
		a.pres.ToggleBlack()
	case ui.ActionClear:
		a.pres.ToggleClear()
	case ui.ActionLive:
		a.setOutput(!a.pres.State.OutputEnabled)
	default:
		log.Printf("[freeworship] toolbar %s: not available in Phase 1", act)
	}
}

// execHotkey runs a hotkey command.
func (a *App) execHotkey(c core.Command) {
	p := a.pres
	switch c.Action {
	case core.ActNext:
		p.Next()
	case core.ActPrev:
		p.Prev()
	case core.ActFirst:
		p.First()
	case core.ActLast:
		p.Last()
	case core.ActScheduleNext:
		a.stepSchedule(+1, false)
	case core.ActSchedulePrev:
		a.stepSchedule(-1, false)
	case core.ActGoLiveNext:
		a.stepSchedule(+1, true)
	case core.ActGoLivePrev:
		a.stepSchedule(-1, true)
	case core.ActBlack:
		p.ToggleBlack()
	case core.ActClear:
		p.ToggleClear()
	case core.ActLogo:
		p.ToggleLogo()
	case core.ActGoLive:
		a.goLive()
	case core.ActJumpSection:
		p.JumpToSection(c.Section, c.N)
	case core.ActJumpSlide:
		if c.N > 0 {
			p.SelectLiveSlide(c.N - 1)
		}
	}
}

// ---- schedule ---------------------------------------------------------------------

// stepSchedule selects the next/previous schedule item into Preview, and with
// live also sends it live. Go-live stepping starts from the live item.
func (a *App) stepSchedule(delta int, live bool) {
	base := a.schedSel
	if live {
		if li := core.LiveItemIndex(a.sched, a.pres.State.Live); li >= 0 {
			base = li
		}
	}
	next, ok := core.StepIndex(base, len(a.sched.Items), delta)
	if !ok {
		return
	}
	a.selectScheduleItem(next)
	if live {
		a.goLive()
	}
}

// selectScheduleItem selects row i and loads the item into Preview.
func (a *App) selectScheduleItem(i int) {
	if i < 0 || i >= len(a.sched.Items) {
		return
	}
	a.schedSel = i
	a.pres.SetPreview(core.ItemDeck(a.sched.Items[i]))
}

func (a *App) removeScheduleItem(i int) bool {
	if !a.sched.Remove(i) {
		return false
	}
	a.schedVer++
	switch {
	case a.schedSel == i:
		a.schedSel = min(i, len(a.sched.Items)-1)
		if a.schedSel >= 0 {
			a.selectScheduleItem(a.schedSel)
		}
	case a.schedSel > i:
		a.schedSel--
	}
	return true
}

func (a *App) moveScheduleItem(from, to int) bool {
	if !a.sched.Move(from, to) {
		return false
	}
	a.schedVer++
	switch {
	case a.schedSel == from:
		a.schedSel = to
	case from < to && a.schedSel > from && a.schedSel <= to:
		a.schedSel--
	case from > to && a.schedSel >= to && a.schedSel < from:
		a.schedSel++
	}
	return true
}

func (a *App) addToSchedule(item models.ScheduleItem) int {
	i := a.sched.Add(item)
	a.schedVer++
	return i
}

// addCurrentToSchedule adds the current resource (by tab) to the schedule.
func (a *App) addCurrentToSchedule(cmd *control.Command) {
	if a.win.Resources.Tab() == ui.TabScriptures {
		a.addScriptureToSchedule(cmd)
		return
	}
	if a.songSel < 0 {
		a.answer(cmd, errors.New("select a song first"))
		return
	}
	a.selectSong(a.songSel, songSchedule, cmd)
}

func (a *App) addSongToSchedule(song models.Song) {
	a.addToSchedule(core.SongItem(song, core.SongDeck(song)))
}

func (a *App) addScriptureToSchedule(cmd *control.Command) {
	if a.passage == nil || a.scriptureDeck == nil {
		err := errors.New("look up a scripture reference first")
		a.setScriptStatus(err.Error(), true)
		a.answer(cmd, err)
		return
	}
	a.addToSchedule(core.ScriptureItem(*a.passage, a.passageBible, *a.scriptureDeck))
	a.answer(cmd, nil)
}

func (a *App) newSchedule() {
	a.sched = models.NewSchedule()
	a.schedSel = -1
	a.schedVer++
}

// confirmDiscard runs then, asking first when the schedule has unsaved changes.
func (a *App) confirmDiscard(title string, then func()) {
	if !a.sched.Dirty {
		then()
		return
	}
	a.win.Confirm.Ask(title, "The schedule \""+a.sched.Title+"\" has unsaved changes. Discard them?", func() {
		a.event(then)
	})
}

// saveSchedule writes the schedule to path.
func (a *App) saveSchedule(path string) error {
	path = core.WithScheduleExt(path)
	if path == "" {
		return errors.New("save schedule: no path")
	}
	if err := a.sched.Save(path); err != nil {
		a.fail("save schedule", err)
		return err
	}
	log.Printf("[freeworship] saved schedule %s", path)
	a.rememberRecent(path)
	return nil
}

// openSchedule replaces the schedule with the file at path.
func (a *App) openSchedule(path string) error {
	s, err := models.LoadSchedule(path)
	if err != nil {
		a.fail("open schedule", err)
		return err
	}
	a.sched = s
	a.schedSel = -1
	a.schedVer++
	log.Printf("[freeworship] opened schedule %s (%d items)", path, len(s.Items))
	a.rememberRecent(path)
	return nil
}

// rememberRecent records path in the recent schedules setting (background).
func (a *App) rememberRecent(path string) {
	st := a.st
	if st == nil {
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		var list []string
		_, err := st.GetSetting(ctx, core.SettingRecent, &list)
		if err == nil {
			err = st.SetSetting(ctx, core.SettingRecent, core.AddRecent(list, path, 10))
		}
		return func() error {
			if err != nil {
				a.fail("remember recent schedule", err)
			}
			return nil
		}
	})
}

// ---- songs ----------------------------------------------------------------------

// songPurpose is what to do once a selected song is loaded.
type songPurpose int

const (
	songPreview songPurpose = iota
	songGoLive
	songSchedule
	songEdit
)

// searchSongs runs a library search in the background. Stale results (a newer
// search was started) are dropped.
func (a *App) searchSongs(q string, cmd *control.Command) {
	a.songQuery = q
	if a.st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	ticket := a.searchGen.Next()
	st := a.st
	a.async(cmd, func(ctx context.Context) func() error {
		rows, err := st.SearchSongs(ctx, q, 0)
		n, cerr := st.CountSongs(ctx)
		return func() error {
			if !a.searchGen.Current(ticket) {
				return nil // superseded
			}
			if cerr == nil {
				a.songCount = n
			}
			if err != nil {
				a.fail("search songs", err)
				a.songsStatus = "Search failed"
				return err
			}
			a.songs = rows
			a.songsVer++
			a.songSel = -1
			if a.song != nil {
				for i, r := range rows {
					if r.ID == a.song.ID {
						a.songSel = i
					}
				}
			}
			if !a.importState.Running {
				a.songsStatus = core.SongCountText(len(rows))
			}
			return nil
		}
	})
}

// selectSong selects row i and loads the song, then does purpose with it.
func (a *App) selectSong(i int, purpose songPurpose, cmd *control.Command) {
	if i < 0 || i >= len(a.songs) {
		a.answer(cmd, fmt.Errorf("no song at index %d", i))
		return
	}
	if a.st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.songSel = i
	id := a.songs[i].ID
	ticket := a.songGen.Next()
	st := a.st
	a.async(cmd, func(ctx context.Context) func() error {
		song, err := st.GetSong(ctx, id)
		return func() error {
			if err != nil {
				a.fail("load song", err)
				a.songsStatus = "Could not load the song"
				return err
			}
			if !a.songGen.Current(ticket) && purpose == songPreview {
				return nil // the user selected another song meanwhile
			}
			a.song = &song
			a.useSong(song, purpose)
			return nil
		}
	})
}

// useSong previews the song and performs purpose.
func (a *App) useSong(song models.Song, purpose songPurpose) {
	if purpose == songEdit {
		a.win.Editor.Open(song, models.FormatLyrics(song.Sections))
		return
	}
	deck := core.SongDeck(song)
	a.songRes = resourcePreview{deck: &deck, index: 0}
	switch purpose {
	case songSchedule:
		a.addSongToSchedule(song)
	case songGoLive:
		a.previewDeck(&deck)
		a.goLive()
	default:
		a.previewDeck(&deck)
	}
}

// previewDeck loads d into Preview unless it is already there (keeping the
// preview selection).
func (a *App) previewDeck(d *models.Deck) {
	if p := a.pres.State.Preview; p != nil && p.ItemID == d.ItemID && p.Title == d.Title &&
		reflect.DeepEqual(p.Slides, d.Slides) && p.Theme == d.Theme {
		return
	}
	a.pres.SetPreview(d)
}

func (a *App) newSong() { a.win.Editor.Open(models.Song{}, "") }

// saveSong persists an edited or new song (the editor's OK button). cmd, when
// not nil, is answered once the song is saved.
func (a *App) saveSong(song models.Song, lyrics string, cmd *control.Command) {
	if a.st == nil {
		a.fail("save song", errNoLibrary)
		a.answer(cmd, errNoLibrary)
		return
	}
	song.Sections = models.ParseLyrics(lyrics)
	isNew := song.ID == 0
	current := a.song != nil && a.song.ID == song.ID
	st := a.st
	a.async(cmd, func(ctx context.Context) func() error {
		err := st.SaveSong(ctx, &song)
		return func() error {
			if err != nil {
				a.fail("save song", err)
				a.songsStatus = "Could not save \"" + song.Title + "\""
				return err
			}
			log.Printf("[freeworship] saved song %d %q", song.ID, song.Title)
			if isNew || current {
				a.song = &song
				deck := core.SongDeck(song)
				a.songRes = resourcePreview{deck: &deck}
				a.previewDeck(&deck)
			}
			a.searchSongs(a.songQuery, nil)
			return nil
		}
	})
}

// deleteSong asks for confirmation, then deletes song row i.
func (a *App) deleteSong(i int) {
	if i < 0 || i >= len(a.songs) || a.st == nil {
		return
	}
	row := a.songs[i]
	a.win.Confirm.Ask("Delete Song", "Delete \""+row.Title+"\" from the library? This cannot be undone.", func() {
		a.event(func() { a.doDeleteSong(row, nil) })
	})
}

// doDeleteSong deletes a song from the library (after confirmation).
func (a *App) doDeleteSong(row models.SongSummary, cmd *control.Command) {
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(cmd, func(ctx context.Context) func() error {
		err := st.DeleteSong(ctx, row.ID)
		return func() error {
			if err != nil {
				a.fail("delete song", err)
				a.songsStatus = "Could not delete \"" + row.Title + "\""
				return err
			}
			log.Printf("[freeworship] deleted song %d %q", row.ID, row.Title)
			if a.song != nil && a.song.ID == row.ID {
				a.song = nil
				a.songRes = resourcePreview{}
			}
			a.searchSongs(a.songQuery, nil)
			return nil
		}
	})
}

// resourceGoLive is the resource tab bar's Go Live button.
func (a *App) resourceGoLive() {
	switch a.win.Resources.Tab() {
	case ui.TabSongs:
		if a.songSel >= 0 {
			a.selectSong(a.songSel, songGoLive, nil)
		}
	case ui.TabScriptures:
		a.selectVerse(max(a.verseSel, 0), true)
	}
}

// ---- scriptures -------------------------------------------------------------------

// loadBibles refreshes the installed translations.
func (a *App) loadBibles(cmd *control.Command) {
	if a.st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	st := a.st
	a.async(cmd, func(ctx context.Context) func() error {
		bibles, err := st.ListBibles(ctx)
		return func() error {
			if err != nil {
				a.fail("list bibles", err)
				a.setScriptStatus("Could not list Bibles: "+err.Error(), true)
				return err
			}
			cur := ""
			if a.bibleSel >= 0 && a.bibleSel < len(a.bibles) {
				cur = a.bibles[a.bibleSel].Abbrev
			}
			a.bibles = bibles
			a.biblesVer++
			a.bibleSel = core.FindBible(bibles, cur)
			if a.bibleSel < 0 {
				a.bibleSel = core.DefaultBible(bibles)
			}
			if len(bibles) == 0 && a.scriptStatus == "" {
				a.setScriptStatus("No Bibles installed", false)
			} else if a.scriptStatus == "No Bibles installed" {
				a.setScriptStatus("", false)
			}
			return nil
		}
	})
}

func (a *App) setScriptStatus(msg string, isErr bool) {
	a.scriptStatus, a.scriptErr = msg, isErr
}

// lookupReference parses a reference and loads its verses in the background,
// then shows them in the verse list and Preview.
func (a *App) lookupReference(text string, cmd *control.Command) {
	a.reference = text
	text = strings.TrimSpace(text)
	ticket := a.verseGen.Next()
	if text == "" {
		a.setScriptStatus("", false)
		a.answer(cmd, nil)
		return
	}
	p, err := models.ParseReference(text)
	if err != nil {
		a.setScriptStatus(err.Error(), true)
		a.answer(cmd, err)
		return
	}
	if a.bibleSel < 0 || a.bibleSel >= len(a.bibles) {
		err := errors.New("no Bible installed")
		a.setScriptStatus("No Bibles installed", true)
		a.answer(cmd, err)
		return
	}
	if a.st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	bible := a.bibles[a.bibleSel]
	st := a.st
	a.async(cmd, func(ctx context.Context) func() error {
		verses, err := st.Verses(ctx, bible.ID, p)
		return func() error {
			if !a.verseGen.Current(ticket) {
				return nil
			}
			if err != nil {
				a.fail("look up "+p.String(), err)
				a.setScriptStatus("Could not load "+p.String(), true)
				return err
			}
			deck := core.ScriptureDeck(p, bible.Abbrev, verses)
			if len(deck.Slides) == 0 {
				err := fmt.Errorf("no verses found for %s (%s)", p.String(), bible.Abbrev)
				a.setScriptStatus("No verses found for "+p.String()+" ("+bible.Abbrev+")", true)
				return err
			}
			a.passage, a.passageBible, a.scriptureDeck = &p, bible, &deck
			a.verseRows = core.VerseRows(verses)
			a.versesVer++
			a.verseSel = 0
			n := len(deck.Slides)
			a.setScriptStatus(fmt.Sprintf("%s · %d verse%s (%s)", p.String(), n, map[bool]string{true: "", false: "s"}[n == 1], bible.Abbrev), false)
			a.scriptRes = resourcePreview{deck: &deck, index: 0}
			a.pres.SetPreview(&deck)
			return nil
		}
	})
}

// verseSelection is the highlighted verse row: the preview selection while
// the preview shows the looked-up passage, else the last selected row.
func (a *App) verseSelection() int {
	d, p := a.scriptureDeck, a.pres.State.Preview
	if d != nil && p != nil && p.ItemID == "" && p.Title == d.Title && reflect.DeepEqual(p.Slides, d.Slides) {
		return a.pres.State.PreviewIndex
	}
	return a.verseSel
}

// selectVerse selects verse row i into Preview; live also sends it live.
func (a *App) selectVerse(i int, live bool) {
	d := a.scriptureDeck
	if d == nil || i < 0 || i >= len(d.Slides) {
		return
	}
	a.verseSel = i
	a.scriptRes.index = i
	a.previewDeck(d)
	if live {
		a.liveChange(func() { a.pres.GoLiveSlide(i) })
		return
	}
	a.pres.SelectPreviewSlide(i)
}
