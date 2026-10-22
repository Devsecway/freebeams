package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"reflect"
	"strings"
	"time"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/media"
	"freebeams/internal/models"
	"freebeams/internal/remote"
	"freebeams/internal/render"
	"freebeams/internal/store"
	"freebeams/internal/ui"

	"graphics.gd/classdb/DisplayServer"
)

// Background audio: the schedule's audio playlist (docs/MEDIA.md "Audio
// playlist"). Its tracks and settings (repeat, shuffle, fade-out) are part
// of the schedule (models.Schedule.Playlist, saved in the .fbs file); what
// plays now is a.au.player (core.AudioPlayer). It runs independently of the
// Live slide: going live, Black, Clear and Logo never touch it.
//
// State machine (CLAUDE.md): the panel's callbacks, the control actions and
// the player's reports (a track ended or failed, audioEvents) mutate the
// state; renderAudio opens / closes the audio-only native player
// (render.OpenAudio) for the track the state wants and projects play,
// pause, seek and the volume (× the fade-out) onto it, then the state onto
// the panel. While the player is open it runs every frame without marking
// the app dirty (position, fade).

// audioState is the background audio's app state.
type audioState struct {
	player core.AudioPlayer
	sched  *models.Schedule // the schedule the player follows (another one resets it)
}

// audioFileFilters are the native dialog's filters for Add Audio Files….
var audioFileFilters = []string{"*.mp3, *.m4a, *.aac, *.wav, *.aiff, *.aif, *.flac, *.ogg ; Audio Files"}

// initAudio starts the player stopped at full volume (Ready).
func (a *App) initAudio() {
	a.au.player = core.NewAudioPlayer(core.AudioEnv{Exists: fileExists})
	a.au.sched = a.sched
	a.au.player.Reset(&a.sched.Playlist)
	a.view.audioSeek = a.au.player.SeekSeq
}

// fileExists reports whether a file is there (a missing track is skipped).
func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// audio returns the player and the playlist it plays, following a schedule
// that was replaced (New, Open): its playlist starts stopped, first track cued.
func (a *App) audio() (*core.AudioPlayer, *models.Playlist) {
	if a.au.sched != a.sched {
		a.au.sched = a.sched
		a.au.player.Reset(&a.sched.Playlist)
	}
	return &a.au.player, &a.sched.Playlist
}

// audioView is the player and the playlist for render and the state
// documents: like audio, but it never writes state. Right after the schedule
// was replaced (before the next event) the player has no track of the new
// playlist, so it shows stopped.
func (a *App) audioView() (*core.AudioPlayer, *models.Playlist) {
	return &a.au.player, &a.sched.Playlist
}

// editPlaylist changes the schedule's playlist (marking the schedule dirty)
// and lets the player follow; hint is the index of a removed track (-1).
func (a *App) editPlaylist(hint int, edit func(pl *models.Playlist) bool) bool {
	p, pl := a.audio()
	if !a.sched.EditPlaylist(edit) {
		return false
	}
	p.Sync(pl, hint)
	return true
}

// wireAudio connects the playlist panel (buildUI).
func (a *App) wireAudio() {
	w := a.win.AudioPlaylist
	w.OnPlayPause = func() { a.event(func() { a.fail("audio playlist", a.audioToggle()) }) }
	w.OnStop = func() { a.event(func() { a.audioStop(true) }) }
	w.OnNext = func() { a.event(func() { p, pl := a.audio(); p.Next(pl) }) }
	w.OnPrev = func() { a.event(a.audioPrev) }
	w.OnSeek = func(frac float64) {
		a.event(func() { p, _ := a.audio(); p.Seek(core.SeekFraction(frac, a.audioDuration())) })
	}
	w.OnVolume = func(v float64) { a.event(func() { p, _ := a.audio(); p.SetVolume(v) }) }
	w.OnRepeat = func() {
		a.event(func() {
			a.editPlaylist(-1, func(pl *models.Playlist) bool { pl.Repeat = pl.Repeat.Next(); return true })
		})
	}
	w.OnShuffle = func(on bool) { a.event(func() { a.setShuffle(on) }) }
	w.OnPlayTrack = func(i int) { a.event(func() { a.fail("audio playlist", a.playTrack(i)) }) }
	w.OnRemove = func(i int) { a.event(func() { a.removeTrack(i) }) }
	w.OnMove = func(from, to int) {
		a.event(func() { a.editPlaylist(-1, func(pl *models.Playlist) bool { return from != to && pl.Move(from, to) }) })
	}
	w.OnAddSelected = func() { a.event(func() { a.fail("audio playlist", a.addSelectedToPlaylist()) }) }
	w.OnAddFiles = func() { a.event(a.addAudioFilesDialog) }
	w.OnClear = func() { a.event(func() { a.clearPlaylist(false, nil) }) }
	w.OnFadeOut = func(secs float64) { a.event(func() { a.setFadeOut(secs) }) }
	a.win.Media.OnAddToPlaylist = func(i int) {
		a.event(func() {
			if i >= 0 && i < len(a.md.list) {
				a.fail("audio playlist", a.addMediaToPlaylist(a.md.list[i]))
			}
		})
	}
}

// ---- events ---------------------------------------------------------------------

// audioEvents turns what the player reported since the last frame into
// state: the end of a track, a file it could not play, a finished fade-out;
// and it learns a track's real duration. It reports a change (Process).
func (a *App) audioEvents() bool {
	p, pl := a.audio()
	changed := false
	if src := render.Current(render.RoleAudio); src != nil {
		render.PumpMedia() // no monitor may be pumping the players (render/audiosource.go)
		if want := p.Wants(pl); want != "" && src.Path() == want {
			st := src.State()
			switch {
			case src.TakeEnded():
				p.TrackEnded(pl)
				changed = true
			case st.Failed:
				if t, ok := p.Current(pl); ok {
					log.Printf("[freebeams] audio playlist: skipping %q (the player could not play it)", t.Title)
				}
				p.TrackFailed(pl, p.TrackID)
				changed = true
			case st.Ready && st.Duration > 0:
				// The list shows the player's duration (the library may not know it yet).
				if i := pl.IndexOf(p.TrackID); i >= 0 && math.Abs(pl.Tracks[i].Seconds-st.Duration) > 0.5 {
					pl.Tracks[i].Seconds = math.Round(st.Duration*10) / 10
					changed = true
				}
			}
		}
	}
	if p.Tick(time.Now()) {
		changed = true
	}
	return changed
}

// audioToggle is the play / pause button.
func (a *App) audioToggle() error {
	p, pl := a.audio()
	return p.Toggle(pl)
}

// audioStop stops, with the playlist's fade-out when fade is set.
func (a *App) audioStop(fade bool) {
	p, pl := a.audio()
	if fade {
		p.Stop(pl, time.Now())
		return
	}
	p.StopNow()
}

// audioPrev restarts the track or goes to the previous one (core.PrevRestart).
func (a *App) audioPrev() {
	p, pl := a.audio()
	p.Prev(pl, a.audioPosition())
}

// playTrack starts track i.
func (a *App) playTrack(i int) error {
	p, pl := a.audio()
	if i < 0 || i >= len(pl.Tracks) {
		return fmt.Errorf("no track %d in the audio playlist", i)
	}
	return p.PlayTrack(pl, pl.Tracks[i].ID)
}

// removeTrack removes track i (the next one plays on if it was playing).
func (a *App) removeTrack(i int) bool {
	return a.editPlaylist(i, func(pl *models.Playlist) bool { return pl.Remove(i) })
}

// setShuffle turns shuffle on or off (a new pass starts with the current track).
func (a *App) setShuffle(on bool) {
	a.editPlaylist(-1, func(pl *models.Playlist) bool {
		changed := pl.Shuffle != on
		pl.Shuffle = on
		return changed
	})
}

// setFadeOut sets the fade-out on stop (seconds, 0 off).
func (a *App) setFadeOut(secs float64) {
	secs = models.ClampFadeOut(secs)
	a.editPlaylist(-1, func(pl *models.Playlist) bool {
		changed := pl.FadeOut != secs
		pl.FadeOut = secs
		return changed
	})
}

// clearPlaylist empties the playlist, asking first unless confirm is set.
func (a *App) clearPlaylist(confirm bool, cmd *control.Command) {
	_, pl := a.audio()
	if len(pl.Tracks) == 0 {
		a.answer(cmd, nil)
		return
	}
	if !confirm {
		n := len(pl.Tracks)
		a.win.Confirm.Ask("Clear Playlist", fmt.Sprintf("Remove all %d tracks from the audio playlist? The files stay in the media library.", n), func() {
			a.event(func() { a.clearPlaylist(true, nil) })
		})
		a.answer(cmd, nil)
		return
	}
	a.editPlaylist(0, func(pl *models.Playlist) bool { pl.Tracks = nil; return true })
	a.answer(cmd, nil)
}

// addMediaToPlaylist appends a library audio item.
func (a *App) addMediaToPlaylist(m models.Media) error {
	if m.Kind != models.MediaAudio {
		return fmt.Errorf("%q is not an audio file", m.Title)
	}
	t := models.TrackFromMedia(m)
	a.editPlaylist(-1, func(pl *models.Playlist) bool { pl.Insert(-1, t); return true })
	log.Printf("[freebeams] audio playlist: added %q", t.Title)
	return nil
}

// addSelectedToPlaylist appends the Media tab's selected item (+ ▸ Add Selected Audio).
func (a *App) addSelectedToPlaylist() error {
	if a.md.item == nil {
		return errors.New("select an audio file in the Media tab first")
	}
	return a.addMediaToPlaylist(*a.md.item)
}

// addAudioFilesDialog shows the native Open dialog for audio files.
func (a *App) addAudioFilesDialog() {
	err := DisplayServer.FileDialogShow("Add Audio Files", a.dialogDir(), "", false, DisplayServer.FileDialogModeOpenFiles,
		audioFileFilters, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() { a.addAudioPaths(paths, nil) })
		}, 0)
	if err != nil {
		a.alert("Add Audio", fmt.Errorf("Could not show the Open dialog.\n\n%w", err))
	}
}

// addAudioPaths adds the audio files among paths (files or folders, in
// name order) to the playlist. A playlist plays library media, so files the
// library does not have yet are registered first (like Add Media Files…).
func (a *App) addAudioPaths(paths []string, cmd *control.Command) {
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		found, warnings := media.Scan(paths...)
		var (
			items []models.Media
			added int // of which new to the library
			err   error
		)
		for _, m := range found {
			if m.Kind != models.MediaAudio {
				continue
			}
			known, lookupErr := st.MediaByPath(ctx, m.Path)
			switch {
			case lookupErr == nil:
				items = append(items, known)
			case errors.Is(lookupErr, store.ErrNotFound):
				items = append(items, m)
				added++
			default:
				err = lookupErr
			}
			if err != nil {
				break
			}
		}
		if err == nil && added > 0 {
			news := make([]*models.Media, 0, added)
			for i := range items {
				if items[i].ID == 0 {
					news = append(news, &items[i])
				}
			}
			err = st.SaveMediaItems(ctx, news) // assigns their ids
		}
		return func() error {
			for _, w := range warnings {
				log.Printf("[freebeams] add audio: %s", w)
			}
			if err == nil && len(items) == 0 {
				err = errors.New("no audio files found")
			}
			if err != nil {
				a.alert("Add Audio", fmt.Errorf("Could not add the audio files to the playlist.\n\n%w", err))
				a.answer(cmd, err)
				return err
			}
			for _, m := range items {
				if e := a.addMediaToPlaylist(m); e != nil {
					a.fail("add audio", e)
				}
			}
			if added > 0 {
				a.loadMedia(a.md.query, a.md.kind, cmd) // the new files in the Media tab
				return nil
			}
			a.answer(cmd, nil)
			return nil
		}
	})
}

// audioSource is the open player when it has the current track's file.
func (a *App) audioSource() *render.MediaSource {
	p, pl := a.audioView()
	src := render.Current(render.RoleAudio)
	if src == nil || src.Path() != p.Wants(pl) {
		return nil
	}
	return src
}

// audioPosition is the current track's position (0 when no player is open).
func (a *App) audioPosition() float64 { // also used by render: reads only
	if src := a.audioSource(); src != nil {
		return src.State().Position
	}
	return 0
}

// audioDuration is the current track's duration (player, else library).
func (a *App) audioDuration() float64 {
	if src := a.audioSource(); src != nil {
		if d := src.State().Duration; d > 0 {
			return d
		}
	}
	p, pl := a.audioView()
	if t, ok := p.Current(pl); ok {
		return t.Seconds
	}
	return 0
}

// ---- render --------------------------------------------------------------------

// renderAudio opens the player the state wants (or closes it), projects the
// state onto it, then onto the panel. Idempotent; it writes only the view
// cache (the open player, the applied seek, the rows shown).
func (a *App) renderAudio() {
	p, pl := a.audioView()
	v := &a.view
	now := time.Now()
	want := p.Wants(pl)
	if v.audioSrc != nil && (v.audioSrc.Path() != want || render.Current(render.RoleAudio) != v.audioSrc) {
		render.CloseAudio(v.audioSrc)
		v.audioSrc = nil
	}
	if v.audioSrc == nil && want != "" {
		v.audioSrc = render.OpenAudio(want)
	}
	if src := v.audioSrc; src != nil {
		if p.SeekSeq != v.audioSeek {
			src.Seek(p.SeekTo) // before Play: a track that ended starts again from 0
		}
		src.SetLoop(false) // RepeatOne restarts the track itself (core.AudioPlayer.TrackEnded)
		src.SetVolume(p.OutputVolume(now))
		if playing := p.Status == core.AudioPlaying; playing != src.Playing() {
			if playing {
				src.Play()
			} else {
				src.Pause()
			}
		}
	}
	v.audioSeek = p.SeekSeq // a seek with no player open is dropped

	rows := a.playlistRows()
	if !reflect.DeepEqual(rows, v.audioRows) {
		v.audioRows = rows
		a.win.AudioPlaylist.SetRows(rows)
	}
	st := a.audioStateDoc()
	a.win.AudioPlaylist.SetState(ui.PlaylistState{Title: st.Title, Playing: st.Playing, Stopped: st.Status == core.AudioStopped,
		Position: st.Position, Duration: st.Duration, Volume: st.Volume, Repeat: string(st.Repeat), Shuffle: st.Shuffle,
		FadeOut: st.FadeOut, Enabled: len(pl.Tracks) > 0, CanAdd: a.md.item != nil && a.md.item.Kind == models.MediaAudio})
}

// playlistRows are the track list's rows.
func (a *App) playlistRows() []ui.PlaylistRow {
	p, pl := a.audioView()
	rows := make([]ui.PlaylistRow, len(pl.Tracks))
	for i, t := range pl.Tracks {
		r := ui.PlaylistRow{Title: t.Title, Missing: !p.Playable(t)}
		secs := t.Seconds
		if secs <= 0 && t.MediaID != 0 { // added before the library probed it: the library's duration
			if j := core.MediaIndexOf(a.md.list, t.MediaID); j >= 0 {
				secs = a.md.list[j].Duration.Seconds()
			}
		}
		if secs > 0 {
			r.Time = core.FormatDuration(time.Duration(secs * float64(time.Second)))
		}
		if t.ID == p.TrackID {
			switch p.Status {
			case core.AudioPlaying:
				r.Marker = "play"
			case core.AudioPaused:
				r.Marker = "pause"
			}
		}
		rows[i] = r
	}
	return rows
}

// audioStateDoc is /state.audio.
func (a *App) audioStateDoc() core.AudioState {
	p, pl := a.audioView()
	rep := core.AudioReport{}
	if src := a.audioSource(); src != nil {
		s := src.State()
		rep = core.AudioReport{Open: true, Ready: s.Ready, Failed: s.Failed, Position: s.Position, Duration: s.Duration}
	}
	return p.State(pl, time.Now(), rep, render.NativeVideoSupported())
}

// audioPhoneDoc is what the phones show of the playlist (no position: the
// phones' state is pushed on changes only).
func (a *App) audioPhoneDoc() remote.AudioDoc {
	p, pl := a.audioView()
	doc := remote.AudioDoc{Status: string(p.Status), Tracks: len(pl.Tracks)}
	if t, ok := p.Current(pl); ok {
		doc.Title = t.Title
	}
	return doc
}

// ---- control actions -------------------------------------------------------------

// handleAudioCommand runs the playlist actions; false when the action is not
// one of them:
//
//	audio-play {index}        play (or resume); index: that playlist track
//	audio-pause, audio-toggle
//	audio-stop {fade}         stop (with the playlist's fade-out unless fade is false)
//	audio-next, audio-prev
//	audio-seek {seconds}, audio-volume {value}
//	audio-repeat {mode}       off | all | one ("" cycles)
//	audio-shuffle {on}        (no on: toggle)
//	audio-fade-out {seconds}  the fade-out on stop (0 off, at most 30)
//	playlist-add {index | title | path | paths}  a Media tab item (index / title in the list,
//	                          default the selection) or audio files (added to the library too)
//	playlist-remove {index}, playlist-move {from, to}, playlist-clear {confirm}
func (a *App) handleAudioCommand(cmd *control.Command, args actionArgs) bool {
	if !strings.HasPrefix(cmd.Action, "audio-") && !strings.HasPrefix(cmd.Action, "playlist-") {
		return false
	}
	p, pl := a.audio()
	ok := func(err error) {
		if err != nil {
			cmd.RespondErr(fmt.Errorf("%s: %w", cmd.Action, err))
			return
		}
		a.answer(cmd, nil)
	}
	switch cmd.Action {
	case "audio-play":
		if args.Index != nil {
			ok(a.playTrack(*args.Index))
			return true
		}
		ok(p.Play(pl))
	case "audio-pause":
		p.Pause()
		ok(nil)
	case "audio-toggle":
		ok(p.Toggle(pl))
	case "audio-stop":
		a.audioStop(args.Fade == nil || *args.Fade)
		ok(nil)
	case "audio-next":
		p.Next(pl)
		ok(nil)
	case "audio-prev":
		a.audioPrev()
		ok(nil)
	case "audio-seek":
		if args.Seconds == nil {
			ok(errors.New("\"seconds\" is required"))
			return true
		}
		if !p.Seek(*args.Seconds) {
			ok(errors.New("no track is playing"))
			return true
		}
		ok(nil)
	case "audio-volume":
		var v float64
		if len(args.Value) == 0 || json.Unmarshal(args.Value, &v) != nil {
			ok(errors.New("\"value\" must be a number from 0 to 1"))
			return true
		}
		p.SetVolume(v)
		ok(nil)
	case "audio-repeat":
		mode, err := models.ParseRepeatMode(args.Mode)
		if err != nil {
			ok(err)
			return true
		}
		if strings.TrimSpace(args.Mode) == "" {
			mode = pl.Repeat.Next()
		}
		a.editPlaylist(-1, func(pl *models.Playlist) bool {
			changed := pl.Repeat.Normalized() != mode
			pl.Repeat = mode
			return changed
		})
		ok(nil)
	case "audio-shuffle":
		on := !pl.Shuffle
		if args.On != nil {
			on = *args.On
		}
		a.setShuffle(on)
		ok(nil)
	case "audio-fade-out":
		if args.Seconds == nil || math.IsNaN(*args.Seconds) || *args.Seconds < 0 || *args.Seconds > models.MaxFadeOut {
			ok(fmt.Errorf("\"seconds\" must be from 0 to %d", models.MaxFadeOut))
			return true
		}
		a.setFadeOut(*args.Seconds)
		ok(nil)
	case "playlist-add":
		paths := args.Paths
		if args.Path != "" {
			paths = append(paths, args.Path)
		}
		if len(paths) > 0 {
			a.addAudioPaths(paths, cmd)
			return true
		}
		i := -1
		switch {
		case args.Index != nil:
			i = *args.Index
		case args.Title != "":
			i = core.FindMedia(a.md.list, args.Title)
		default:
			ok(a.addSelectedToPlaylist())
			return true
		}
		if i < 0 || i >= len(a.md.list) {
			ok(fmt.Errorf("no media item matches %s", string(cmd.Data)))
			return true
		}
		ok(a.addMediaToPlaylist(a.md.list[i]))
	case "playlist-remove":
		if args.Index == nil || !a.removeTrack(*args.Index) {
			ok(fmt.Errorf("no track %s", string(cmd.Data)))
			return true
		}
		ok(nil)
	case "playlist-move":
		if args.From == nil || args.To == nil {
			ok(errArgs)
			return true
		}
		from, to := *args.From, *args.To
		if from < 0 || from >= len(pl.Tracks) || to < 0 || to >= len(pl.Tracks) {
			ok(fmt.Errorf("no track %s", string(cmd.Data)))
			return true
		}
		a.editPlaylist(-1, func(pl *models.Playlist) bool { return from != to && pl.Move(from, to) })
		ok(nil)
	case "playlist-clear":
		a.clearPlaylist(args.Confirm, cmd)
	default:
		cmd.RespondErr(fmt.Errorf("unknown action %q (audio-play|audio-pause|audio-toggle|audio-stop|audio-next|audio-prev|"+
			"audio-seek|audio-volume|audio-repeat|audio-shuffle|audio-fade-out|playlist-add|playlist-remove|playlist-move|playlist-clear)", cmd.Action))
	}
	return true
}
