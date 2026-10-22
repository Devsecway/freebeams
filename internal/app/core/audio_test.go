package core

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"freebeams/internal/models"
)

// testPlaylist has tracks a..e (ids "a".."e", paths "/m/a.mp3"…).
func testPlaylist(names ...string) *models.Playlist {
	pl := &models.Playlist{}
	for _, n := range names {
		pl.Tracks = append(pl.Tracks, models.PlaylistTrack{ID: n, Path: "/m/" + n + ".mp3", Title: n, Seconds: 10})
	}
	return pl
}

func newTestPlayer(missing ...string) AudioPlayer {
	r := rand.New(rand.NewPCG(1, 2))
	return NewAudioPlayer(AudioEnv{
		Exists: func(path string) bool {
			for _, m := range missing {
				if path == "/m/"+m+".mp3" {
					return false
				}
			}
			return true
		},
		Rand: func(n int) int { return r.IntN(n) },
	})
}

// playThrough plays from stopped and lets every track end until the player
// stops (or n tracks played), returning the tracks in play order.
func playThrough(t *testing.T, p *AudioPlayer, pl *models.Playlist, n int) []string {
	t.Helper()
	if err := p.Play(pl); err != nil {
		t.Fatalf("Play: %v", err)
	}
	var got []string
	for p.Status == AudioPlaying && len(got) < n {
		got = append(got, p.TrackID)
		p.TrackEnded(pl)
	}
	return got
}

func TestAudioOrderAndEnd(t *testing.T) {
	pl := testPlaylist("a", "b", "c")
	p := newTestPlayer()
	p.Reset(pl)
	if p.Status != AudioStopped || p.TrackID != "a" || p.Wants(pl) != "" {
		t.Fatalf("reset: %+v", p)
	}
	got := playThrough(t, &p, pl, 10)
	if !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("order: %v", got)
	}
	if p.Status != AudioStopped || p.TrackID != "a" {
		t.Fatalf("after the last track: %s %q (want stopped, a cued)", p.Status, p.TrackID)
	}

	// RepeatAll wraps around.
	pl.Repeat = models.RepeatAll
	got = playThrough(t, &p, pl, 7)
	if !slices.Equal(got, []string{"a", "b", "c", "a", "b", "c", "a"}) {
		t.Fatalf("repeat all: %v", got)
	}

	// RepeatOne plays the same track again from 0.
	pl.Repeat = models.RepeatOne
	seq := p.SeekSeq
	cur := p.TrackID
	p.TrackEnded(pl)
	if p.TrackID != cur || p.Status != AudioPlaying || p.SeekSeq != seq+1 || p.SeekTo != 0 {
		t.Fatalf("repeat one: %+v", p)
	}
	// ... but the operator's Next still moves on.
	p.Next(pl)
	if p.TrackID == cur || p.Status != AudioPlaying {
		t.Fatalf("Next under repeat one: %+v", p)
	}
}

func TestAudioSkipsMissingFiles(t *testing.T) {
	pl := testPlaylist("a", "b", "c", "d")
	p := newTestPlayer("b", "d")
	p.Reset(pl)
	if got := playThrough(t, &p, pl, 10); !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("with b and d missing: %v", got)
	}
	// The player failing on a file skips it from then on.
	p.Play(pl)
	if p.TrackID != "a" {
		t.Fatalf("play: %q", p.TrackID)
	}
	p.TrackFailed(pl, "a")
	if p.TrackID != "c" || p.Status != AudioPlaying || !p.Failed["a"] {
		t.Fatalf("after a failed: %+v", p)
	}
	p.TrackFailed(pl, "c")
	if p.Status != AudioStopped {
		t.Fatalf("nothing left to play: %+v", p)
	}
	if err := p.Play(pl); !errors.Is(err, ErrNoPlayableTrack) {
		t.Fatalf("Play with nothing playable: %v", err)
	}
	if err := p.PlayTrack(pl, "b"); err == nil {
		t.Fatal("PlayTrack played a missing file")
	}
	st := p.State(pl, time.Now(), AudioReport{}, true)
	for i, want := range []bool{true, true, true, true} {
		if st.Tracks[i].Missing != want {
			t.Fatalf("missing flags: %+v", st.Tracks)
		}
	}
	// An empty list cannot play either.
	empty := &models.Playlist{}
	q := newTestPlayer()
	q.Reset(empty)
	if err := q.Play(empty); !errors.Is(err, ErrNoPlayableTrack) || q.TrackID != "" {
		t.Fatalf("empty: %v %+v", err, q)
	}
}

func TestAudioShuffleWithoutRepeats(t *testing.T) {
	pl := testPlaylist("a", "b", "c", "d", "e", "f", "g")
	pl.Shuffle = true
	pl.Repeat = models.RepeatAll
	p := newTestPlayer()
	p.Reset(pl)
	got := playThrough(t, &p, pl, 7*30)
	if len(got) != 7*30 {
		t.Fatalf("played %d", len(got))
	}
	shuffled := false
	for pass := 0; pass < 30; pass++ {
		one := slices.Clone(got[pass*7 : pass*7+7])
		if !slices.Equal(one, []string{"a", "b", "c", "d", "e", "f", "g"}) {
			shuffled = true
		}
		slices.Sort(one)
		if !slices.Equal(one, []string{"a", "b", "c", "d", "e", "f", "g"}) {
			t.Fatalf("pass %d does not play every track once: %v", pass, got[pass*7:pass*7+7])
		}
	}
	if !shuffled {
		t.Fatal("30 passes in list order: not shuffled")
	}
	for i := 1; i < len(got); i++ {
		if got[i] == got[i-1] {
			t.Fatalf("track %s played twice in a row at %d", got[i], i)
		}
	}

	// Turning shuffle on mid-list keeps the current track and plays the rest once.
	pl2 := testPlaylist("a", "b", "c", "d", "e")
	q := newTestPlayer()
	q.Reset(pl2)
	q.Play(pl2)
	q.TrackEnded(pl2) // b
	pl2.Shuffle = true
	q.Sync(pl2, -1)
	if q.TrackID != "b" || q.Order(pl2)[0] != 1 {
		t.Fatalf("shuffle on: %q %v", q.TrackID, q.Order(pl2))
	}
	var rest []string
	for q.Status == AudioPlaying {
		rest = append(rest, q.TrackID)
		q.TrackEnded(pl2)
	}
	slices.Sort(rest)
	if !slices.Equal(rest, []string{"a", "b", "c", "d", "e"}) {
		t.Fatalf("after shuffle on: %v", rest)
	}

	// A track added mid-pass is still to come; picking one keeps the pass unique.
	pl3 := testPlaylist("a", "b", "c", "d")
	pl3.Shuffle = true
	r := newTestPlayer()
	r.Reset(pl3)
	r.Play(pl3)
	first := r.TrackID
	pl3.Insert(-1, models.PlaylistTrack{ID: "z", Path: "/m/z.mp3", Title: "z"})
	r.Sync(pl3, -1)
	order := r.Order(pl3)
	if pl3.Tracks[order[0]].ID != first || !slices.Contains(order, 4) {
		t.Fatalf("added mid-pass: %v", order)
	}
	pick := pl3.Tracks[order[3]].ID
	if err := r.PlayTrack(pl3, pick); err != nil {
		t.Fatal(err)
	}
	played := []string{first}
	for r.Status == AudioPlaying {
		played = append(played, r.TrackID)
		r.TrackEnded(pl3)
	}
	if played[1] != pick {
		t.Fatalf("the picked track did not play next: %v", played)
	}
	slices.Sort(played)
	if !slices.Equal(played, []string{"a", "b", "c", "d", "z"}) {
		t.Fatalf("pass with a pick: %v", played)
	}

	// Shuffle off goes back to the list order from the current track.
	pl3.Shuffle = false
	r.Sync(pl3, -1)
	if !slices.Equal(r.Order(pl3), []int{0, 1, 2, 3, 4}) {
		t.Fatalf("shuffle off: %v", r.Order(pl3))
	}
}

func TestAudioPrevNextStopped(t *testing.T) {
	pl := testPlaylist("a", "b", "c")
	p := newTestPlayer()
	p.Reset(pl)
	p.Next(pl)
	if p.Status != AudioStopped || p.TrackID != "b" {
		t.Fatalf("Next while stopped cues: %+v", p)
	}
	p.Play(pl)
	if p.TrackID != "b" {
		t.Fatalf("Play starts the cued track: %q", p.TrackID)
	}
	seq := p.SeekSeq
	p.Prev(pl, 5) // 5 s in: restart
	if p.TrackID != "b" || p.SeekSeq != seq+1 || p.SeekTo != 0 {
		t.Fatalf("Prev 5 s in: %+v", p)
	}
	p.Prev(pl, 1) // 1 s in: the previous track
	if p.TrackID != "a" || p.Status != AudioPlaying {
		t.Fatalf("Prev 1 s in: %+v", p)
	}
	p.Prev(pl, 0) // the first, repeat off: restart it
	if p.TrackID != "a" {
		t.Fatalf("Prev at the first: %q", p.TrackID)
	}
	pl.Repeat = models.RepeatAll
	p.Prev(pl, 0)
	if p.TrackID != "c" {
		t.Fatalf("Prev at the first under repeat all: %q", p.TrackID)
	}
	p.Next(pl)
	if p.TrackID != "a" || p.Status != AudioPlaying {
		t.Fatalf("Next at the last under repeat all: %+v", p)
	}
	pl.Repeat = models.RepeatOff
	p.Pause()
	p.Next(pl)
	if p.TrackID != "b" || p.Status != AudioPaused {
		t.Fatalf("Next while paused stays paused: %+v", p)
	}
	p.Next(pl)
	p.Next(pl) // past the last, repeat off: stopped, a cued
	if p.TrackID != "a" || p.Status != AudioStopped {
		t.Fatalf("Next past the last: %+v", p)
	}
	if p.Seek(3) {
		t.Fatal("a seek while stopped was accepted")
	}
	if err := p.Toggle(pl); err != nil || p.Status != AudioPlaying {
		t.Fatalf("Toggle from stopped: %v %+v", err, p)
	}
	if !p.Seek(4) || p.SeekTo != 4 {
		t.Fatal("seek")
	}
	p.Toggle(pl)
	if p.Status != AudioPaused || p.Wants(pl) != "/m/a.mp3" {
		t.Fatalf("Toggle pauses: %+v", p)
	}
	p.Toggle(pl)
	if p.Status != AudioPlaying {
		t.Fatal("Toggle resumes")
	}
	p.TrackEnded(pl)
	if p.TrackID != "b" {
		t.Fatal("ended")
	}
	p.Pause()
	p.TrackEnded(pl) // a paused player does not end
	if p.TrackID != "b" || p.Status != AudioPaused {
		t.Fatalf("ended while paused: %+v", p)
	}
}

func TestAudioFadeOut(t *testing.T) {
	pl := testPlaylist("a", "b")
	pl.FadeOut = 2
	p := newTestPlayer()
	p.Reset(pl)
	p.Play(pl)
	p.SetVolume(0.8)
	t0 := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	p.Stop(pl, t0)
	if p.Status != AudioPlaying || !p.Fading {
		t.Fatalf("Stop with a fade: %+v", p)
	}
	if g := p.Gain(t0.Add(500 * time.Millisecond)); g != 0.75 {
		t.Fatalf("gain at 0.5 s: %v", g)
	}
	if v := p.OutputVolume(t0.Add(time.Second)); v != 0.4 {
		t.Fatalf("volume at 1 s: %v", v)
	}
	if p.Tick(t0.Add(1900 * time.Millisecond)) {
		t.Fatal("the fade ended early")
	}
	if !p.Tick(t0.Add(2*time.Second)) || p.Status != AudioStopped || p.Fading || p.TrackID != "a" {
		t.Fatalf("after the fade: %+v", p)
	}
	if p.Gain(t0.Add(3*time.Second)) != 1 {
		t.Fatal("the gain stays down after the fade")
	}
	// Play during a fade cancels it; a second Stop stops at once.
	p.Play(pl)
	p.Stop(pl, t0)
	p.Play(pl)
	if p.Fading || p.Status != AudioPlaying {
		t.Fatalf("Play during the fade: %+v", p)
	}
	p.Stop(pl, t0)
	p.Stop(pl, t0)
	if p.Status != AudioStopped {
		t.Fatalf("second Stop: %+v", p)
	}
	// No fade set: Stop is immediate.
	pl.FadeOut = 0
	p.Play(pl)
	p.Stop(pl, t0)
	if p.Status != AudioStopped {
		t.Fatal("Stop without a fade")
	}
}

func TestAudioSyncAfterEdits(t *testing.T) {
	pl := testPlaylist("a", "b", "c")
	p := newTestPlayer()
	p.Reset(pl)
	p.Play(pl)
	p.Next(pl) // b plays
	pl.Remove(1)
	p.Sync(pl, 1)
	if p.TrackID != "c" || p.Status != AudioPlaying {
		t.Fatalf("removing the playing track plays the one at its place: %+v", p)
	}
	pl.Remove(1)
	p.Sync(pl, 1)
	if p.TrackID != "a" || p.Status != AudioPlaying {
		t.Fatalf("removing the last track: %+v", p)
	}
	p.Pause()
	pl.Insert(0, models.PlaylistTrack{ID: "x", Path: "/m/x.mp3", Title: "x"})
	pl.Remove(1)
	p.Sync(pl, 1)
	if p.TrackID != "x" || p.Status != AudioStopped {
		t.Fatalf("removing the paused track cues: %+v", p)
	}
	pl.Remove(0)
	p.Sync(pl, 0)
	if p.TrackID != "" || p.Status != AudioStopped {
		t.Fatalf("empty: %+v", p)
	}
	// A move keeps the current track.
	pl2 := testPlaylist("a", "b", "c")
	q := newTestPlayer()
	q.Reset(pl2)
	q.Play(pl2)
	pl2.Move(0, 2)
	q.Sync(pl2, -1)
	if q.TrackID != "a" || q.PeekNext(pl2) != -1 {
		t.Fatalf("after a move: %+v next %d", q, q.PeekNext(pl2))
	}
}

func TestAudioState(t *testing.T) {
	pl := testPlaylist("a", "b")
	pl.Repeat = models.RepeatAll
	p := newTestPlayer()
	p.Reset(pl)
	st := p.State(pl, time.Now(), AudioReport{}, true)
	if st.Status != AudioStopped || st.Track != 0 || st.Title != "a" || st.Next != 1 || st.Duration != 10 ||
		st.Repeat != models.RepeatAll || st.Gain != 1 || len(st.Tracks) != 2 || !slices.Equal(st.Order, []int{0, 1}) {
		t.Fatalf("stopped: %+v", st)
	}
	p.Play(pl)
	p.Next(pl)
	st = p.State(pl, time.Now(), AudioReport{Open: true, Ready: true, Position: 1.5, Duration: 9.5}, true)
	if !st.Playing || st.Track != 1 || st.Next != 0 || st.Position != 1.5 || st.Duration != 9.5 || !st.Loaded {
		t.Fatalf("playing: %+v", st)
	}
}
