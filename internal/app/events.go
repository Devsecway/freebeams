package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/ui"
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
	a.setPreview(d)
	a.liveChange(func() { a.pres.GoLiveSlide(i) })
}

// setPreview loads d into Preview. Every Preview load goes through here, so
// the schedule selection always follows Preview: the row d came from, or none
// for library content. (Otherwise a row stays highlighted after a song is
// previewed, and clicking it again does nothing.)
func (a *App) setPreview(d *models.Deck) {
	a.pres.SetPreview(d)
	a.schedSel = core.ScheduleSelection(a.sched, d)
}

// syncScheduleSelection re-derives the selected row after the schedule
// itself changed (items moved, removed, another schedule opened).
func (a *App) syncScheduleSelection() {
	a.schedSel = core.ScheduleSelection(a.sched, a.pres.State.Preview)
}

// setMode switches Black / Clear / Logo: toggles it, or with on sets it (a
// phone sends the state it wants, so two phones pressing Black agree).
func (a *App) setMode(mode models.OutputMode, on *bool, toggle func()) {
	if on == nil || *on != (a.pres.State.Mode == mode) {
		toggle()
	}
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
	case ui.ActionRemotePair:
		a.openRemoteDialog()
	case ui.ActionRemoteToggle:
		a.setRemoteEnabled(!a.rm.enabled)
	default:
		log.Printf("[freebeams] toolbar %s: not available in Phase 1", act)
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

// selectScheduleItem selects row i and loads the item into Preview. A
// video background plays from its transcode when the library has one; a
// presentation whose snapshot pages are stale or gone shows the library's
// current render of the document (the schedule itself is untouched).
func (a *App) selectScheduleItem(i int) {
	if i < 0 || i >= len(a.sched.Items) {
		return
	}
	item := core.ResolvePresentationItem(a.sched.Items[i], a.lookupPages)
	a.setPreview(core.ResolveDeck(core.ItemDeck(item), a.lookupPlayable))
}

// removeScheduleItem removes item i. When it was the selected item, the item
// that takes its place is loaded into Preview.
func (a *App) removeScheduleItem(i int) bool {
	selected := a.schedSel == i
	if !a.sched.Remove(i) {
		return false
	}
	a.schedVer++
	if selected && len(a.sched.Items) > 0 {
		a.selectScheduleItem(min(i, len(a.sched.Items)-1))
		return true
	}
	a.syncScheduleSelection()
	return true
}

func (a *App) moveScheduleItem(from, to int) bool {
	if !a.sched.Move(from, to) {
		return false
	}
	a.schedVer++
	a.syncScheduleSelection()
	return true
}

func (a *App) addToSchedule(item models.ScheduleItem) int {
	if a.drop.inserting { // a drop between rows (dragdrop.go)
		i := a.sched.Insert(a.drop.at, item)
		a.schedVer++
		a.syncScheduleSelection()
		return i
	}
	i := a.sched.Add(item)
	a.schedVer++
	return i
}

// addCurrentToSchedule adds the current resource (by tab) to the schedule.
func (a *App) addCurrentToSchedule(cmd *control.Command) {
	switch a.win.Resources.Tab() {
	case ui.TabScriptures:
		a.addScriptureToSchedule(cmd)
		return
	case ui.TabMedia:
		a.addMediaToSchedule(cmd)
		return
	case ui.TabPresentations:
		a.addPresentationToSchedule(cmd)
	case ui.TabThemes:
		a.answer(cmd, errors.New("themes are applied to schedule items, not added"))
		return
	}
	if a.songSel < 0 {
		a.answer(cmd, errors.New("select a song first"))
		return
	}
	a.selectSong(a.songSel, songSchedule, cmd)
}

// addSongToSchedule adds song; a restyled preview of it (the Preview Output
// menu) goes into the schedule with its look (core.CarryStyle).
func (a *App) addSongToSchedule(song models.Song) {
	a.addToSchedule(core.SongItem(song, core.CarryStyle(a.songRes.deck, a.newSongDeck(song))))
}

// addScriptureToSchedule adds the looked-up passage. After a failed or empty
// lookup there is none (lookupReference clears it), so nothing is added.
func (a *App) addScriptureToSchedule(cmd *control.Command) {
	if a.passage == nil || a.scriptureDeck == nil {
		status := "Look up a scripture reference first"
		if a.scriptErr {
			status = "Nothing to add: the reference did not find any verses"
		}
		a.setScriptStatus(status, true)
		a.answer(cmd, errors.New(strings.ToLower(status[:1])+status[1:]))
		return
	}
	a.addToSchedule(core.ScriptureItem(*a.passage, a.passageBible, *a.scriptureDeck))
	a.answer(cmd, nil)
}

func (a *App) newSchedule() {
	a.sched = models.NewSchedule()
	a.schedVer++
	a.syncScheduleSelection()
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

// saveSchedule writes the schedule to path. Saving to another file than the
// current one is a Save As: the schedule is renamed after the new file.
// Failures are shown to the operator.
func (a *App) saveSchedule(path string) error {
	path = core.WithScheduleExt(path)
	if path == "" {
		return errors.New("save schedule: no path")
	}
	save := a.sched.Save
	if core.IsSaveAs(a.sched.Path, path) {
		save = a.sched.SaveAs
	}
	if err := save(path); err != nil {
		a.alert("Save Schedule", fmt.Errorf("Could not save the schedule as %q.\n\n%w", filepath.Base(path), err))
		return err
	}
	log.Printf("[freebeams] saved schedule %s", path)
	a.rememberRecent(path)
	return nil
}

// openSchedule replaces the schedule with the file at path. Failures are
// shown to the operator.
func (a *App) openSchedule(path string) error {
	s, err := models.LoadSchedule(path)
	if err != nil {
		a.alert("Open Schedule", fmt.Errorf("Could not open %q.\n\n%w", filepath.Base(path), err))
		return err
	}
	a.sched = s
	a.schedVer++
	a.syncScheduleSelection()
	log.Printf("[freebeams] opened schedule %s (%d items)", path, len(s.Items))
	a.rememberRecent(path)
	return nil
}

// rememberRecent records path in the recent schedules setting (background).
func (a *App) rememberRecent(path string) {
	st := a.st()
	if st == nil {
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		var list []string
		_, err := st.GetSetting(ctx, core.SettingRecent, &list)
		if err == nil {
			list = core.AddRecent(list, path, 10)
			err = st.SetSetting(ctx, core.SettingRecent, list)
		}
		return func() error {
			if err != nil {
				a.fail("remember recent schedule", err)
				return nil
			}
			a.recent = list // File ▸ Open Recent
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
	if a.st() == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	ticket := a.searchGen.Next()
	st := a.st()
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
			a.songsStatus = core.SongsFooterMatches(len(rows), core.LyricsMatches(rows), a.importState)
			return nil
		}
	})
}

// selectSong selects row i and loads the song, then does purpose with it.
//
// Loads run concurrently (the store has a connection pool) and can finish in
// any order, so their results go through a.songSeq and apply in request
// order: an older request never overrides a newer one. A preview that a newer
// request overtook is dropped; the other purposes (go live, add to schedule,
// edit) are user actions and still happen, in order. cmd is answered once
// this request's result has been applied.
func (a *App) selectSong(i int, purpose songPurpose, cmd *control.Command) {
	a.selectSongAt(i, purpose, -1, cmd)
}

// selectSongAt is selectSong; a songSchedule purpose inserts the song at
// schedule index at (-1 appends: a drop between rows, dragdrop.go).
func (a *App) selectSongAt(i int, purpose songPurpose, at int, cmd *control.Command) {
	if i < 0 || i >= len(a.songs) {
		a.answer(cmd, fmt.Errorf("no song at index %d", i))
		return
	}
	if a.st() == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.songSel = i
	id := a.songs[i].ID
	ticket := a.songSeq.Next()
	st := a.st()
	a.async(nil, func(ctx context.Context) (apply func() error) {
		song, err := models.Song{}, error(nil)
		// Every ticket must reach the sequencer, even if the load panics,
		// or later results would wait forever.
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("load song %d: panic: %v", id, r)
			}
			apply = func() error {
				a.songSeq.Done(ticket, func() { a.applySong(ticket, song, err, purpose, at, cmd) })
				return nil
			}
		}()
		song, err = st.GetSong(ctx, id)
		return nil
	})
}

// applySong applies one song load (in request order, see selectSong).
func (a *App) applySong(ticket uint64, song models.Song, err error, purpose songPurpose, at int, cmd *control.Command) {
	if err != nil {
		a.alert("Load Song", fmt.Errorf("Could not load the song.\n\n%w", err))
		a.answer(cmd, err)
		return
	}
	if purpose == songPreview && a.songSeq.Superseded(ticket) {
		a.answer(cmd, nil) // the user selected another song meanwhile
		return
	}
	a.song = &song
	a.insertingAt(at, func() { a.useSong(song, purpose) })
	a.answer(cmd, nil)
}

// useSong previews the song and performs purpose.
func (a *App) useSong(song models.Song, purpose songPurpose) {
	if purpose == songEdit {
		a.win.Editor.Open(song, models.FormatLyrics(song.Sections))
		return
	}
	deck := core.CarryStyle(a.songRes.deck, a.newSongDeck(song)) // keeps a restyled preview's look
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
	if core.SameDeck(a.pres.State.Preview, d) {
		return
	}
	a.setPreview(d)
}

func (a *App) newSong() { a.win.Editor.Open(models.Song{}, "") }

// saveSong persists an edited or new song (the editor's OK button). cmd, when
// not nil, is answered once the song is saved.
func (a *App) saveSong(song models.Song, lyrics string, cmd *control.Command) {
	if a.st() == nil {
		a.fail("save song", errNoLibrary)
		a.answer(cmd, errNoLibrary)
		return
	}
	song.Sections = models.ParseLyrics(lyrics)
	isNew := song.ID == 0
	current := a.song != nil && a.song.ID == song.ID
	st := a.st()
	a.async(cmd, func(ctx context.Context) func() error {
		err := st.SaveSong(ctx, &song)
		return func() error {
			if err != nil {
				a.alert("Save Song", fmt.Errorf("Could not save %q.\n\n%w", song.Title, err))
				return err
			}
			log.Printf("[freebeams] saved song %d %q", song.ID, song.Title)
			if isNew || current {
				a.song = &song
				deck := a.newSongDeck(song)
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
	if i < 0 || i >= len(a.songs) || a.st() == nil {
		return
	}
	row := a.songs[i]
	a.win.Confirm.Ask("Delete Song", "Delete \""+row.Title+"\" from the library? This cannot be undone.", func() {
		a.event(func() { a.doDeleteSong(row, nil) })
	})
}

// doDeleteSong deletes a song from the library (after confirmation).
func (a *App) doDeleteSong(row models.SongSummary, cmd *control.Command) {
	st := a.st()
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(cmd, func(ctx context.Context) func() error {
		err := st.DeleteSong(ctx, row.ID)
		return func() error {
			if err != nil {
				a.alert("Delete Song", fmt.Errorf("Could not delete %q.\n\n%w", row.Title, err))
				return err
			}
			log.Printf("[freebeams] deleted song %d %q", row.ID, row.Title)
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
		a.goLiveVerse()
	case ui.TabMedia:
		a.mediaGoLive()
	case ui.TabPresentations:
		a.presentationGoLive()
	}
}

// ---- scriptures -------------------------------------------------------------------

// loadBibles refreshes the installed translations.
func (a *App) loadBibles(cmd *control.Command) {
	if a.st() == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	st := a.st()
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
