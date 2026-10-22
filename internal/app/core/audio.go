package core

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"freebeams/internal/models"
)

// Background audio: the player of the schedule's playlist (models.Playlist).
//
// It plays independently of the Live slide: going live with anything, Black,
// Clear and Logo leave it alone. AudioPlayer is the operator's state (which
// track, playing / paused / stopped, the volume, a fade-out in progress, the
// shuffle order of this pass); the app projects it onto an audio-only native
// player (render.OpenAudio) and turns what the player reports (a track
// ended, a file failed) back into events here. Every method that changes
// the state is an event; Gain, PeekNext and State only read.

// AudioStatus is what the playlist player is doing.
type AudioStatus string

const (
	AudioStopped AudioStatus = "stopped" // no player open; TrackID is the cued track (it starts from 0)
	AudioPlaying AudioStatus = "playing"
	AudioPaused  AudioStatus = "paused"
)

// PrevRestart is how far into a track Prev restarts it instead of going to
// the previous track (seconds), as music players do.
const PrevRestart = 3.0

// ErrNoPlayableTrack is returned when no track of the playlist can be played
// (the list is empty, or every file is missing).
var ErrNoPlayableTrack = errors.New("no track of the audio playlist can be played")

// AudioEnv is what the player needs from outside, injected for tests.
type AudioEnv struct {
	Exists func(path string) bool // nil: every file exists
	Rand   func(n int) int        // a random int in [0, n); nil: math/rand/v2
}

// AudioPlayer is the playlist player's state.
type AudioPlayer struct {
	Status  AudioStatus
	TrackID string  // the current track (playing / paused) or the cued one (stopped); "" none
	Volume  float64 // 0..1, the transport's volume
	SeekSeq int     // bumped by every seek and track start; render applies SeekTo once per bump
	SeekTo  float64 // seconds

	// The fade-out of a Stop: the volume ramps from 1 to 0 over FadeLen
	// seconds from FadeStart, then the player stops.
	Fading    bool
	FadeStart time.Time
	FadeLen   float64

	// Failed holds the tracks the player could not open this session
	// (skipped like missing files).
	Failed map[string]bool

	Env AudioEnv

	// shuffle is this pass's order (track IDs) while the playlist shuffles.
	shuffle []string
}

// NewAudioPlayer is a stopped player at full volume.
func NewAudioPlayer(env AudioEnv) AudioPlayer {
	return AudioPlayer{Status: AudioStopped, Volume: 1, Env: env}
}

func (p *AudioPlayer) exists(path string) bool {
	return p.Env.Exists == nil || p.Env.Exists(path)
}

func (p *AudioPlayer) randN(n int) int {
	if p.Env.Rand != nil {
		return p.Env.Rand(n)
	}
	return rand.IntN(n)
}

// Playable reports whether a track can be played: its file exists and the
// player has not failed on it.
func (p *AudioPlayer) Playable(t models.PlaylistTrack) bool {
	return !p.Failed[t.ID] && p.exists(t.Path)
}

// ---- the play order ------------------------------------------------------------

// listOrder is the playlist's own order (track IDs).
func listOrder(pl *models.Playlist) []string {
	ids := make([]string, len(pl.Tracks))
	for i, t := range pl.Tracks {
		ids[i] = t.ID
	}
	return ids
}

// reconcile brings the shuffle order in line with the playlist: none while
// the playlist does not shuffle; a new pass (the current track first) when
// shuffle was just turned on; after an edit, removed tracks are dropped and
// new ones put at random places among the tracks still to come, so a pass
// still plays every track once.
func (p *AudioPlayer) reconcile(pl *models.Playlist) {
	if !pl.Shuffle {
		p.shuffle = nil
		return
	}
	if p.shuffle == nil {
		p.newPass(pl, p.TrackID, "")
		return
	}
	present := make(map[string]bool, len(pl.Tracks))
	for _, t := range pl.Tracks {
		present[t.ID] = true
	}
	keep := p.shuffle[:0]
	inOrder := make(map[string]bool, len(p.shuffle))
	for _, id := range p.shuffle {
		if present[id] && !inOrder[id] {
			keep = append(keep, id)
			inOrder[id] = true
		}
	}
	p.shuffle = keep
	cur := p.pos(p.shuffle)
	for _, t := range pl.Tracks {
		if inOrder[t.ID] {
			continue
		}
		// A random place after the current track (cur+1 .. len).
		at := cur + 1 + p.randN(len(p.shuffle)-cur)
		p.shuffle = append(p.shuffle, "")
		copy(p.shuffle[at+1:], p.shuffle[at:])
		p.shuffle[at] = t.ID
	}
}

// newPass makes a new shuffle order: first leads it when given, and avoid
// (the track that just ended a pass) does not start it when there is a
// choice, so no track plays twice in a row.
func (p *AudioPlayer) newPass(pl *models.Playlist, first, avoid string) {
	ids := listOrder(pl)
	for i := len(ids) - 1; i > 0; i-- {
		j := p.randN(i + 1)
		ids[i], ids[j] = ids[j], ids[i]
	}
	lead := -1
	for i, id := range ids {
		if id == first {
			lead = i
		}
	}
	switch {
	case lead > 0:
		ids[0], ids[lead] = ids[lead], ids[0]
	case lead < 0 && avoid != "" && len(ids) > 1 && ids[0] == avoid:
		k := 1 + p.randN(len(ids)-1)
		ids[0], ids[k] = ids[k], ids[0]
	}
	p.shuffle = ids
}

// order is the play order of this pass (track IDs).
func (p *AudioPlayer) order(pl *models.Playlist) []string {
	if pl.Shuffle && p.shuffle != nil {
		return p.shuffle
	}
	return listOrder(pl)
}

// pos is the current track's place in order, -1 when it is not there.
func (p *AudioPlayer) pos(order []string) int {
	for i, id := range order {
		if id == p.TrackID {
			return i
		}
	}
	return -1
}

// scan finds the first playable track from order[from] in direction dir,
// stopping at the list's end (no wrap). -1 when there is none.
func (p *AudioPlayer) scan(pl *models.Playlist, order []string, from, dir int) int {
	for i := from; i >= 0 && i < len(order); i += dir {
		if t, ok := pl.Track(order[i]); ok && p.Playable(t) {
			return i
		}
	}
	return -1
}

// ---- events --------------------------------------------------------------------

// start makes id the current track, playing from 0 (status as given).
func (p *AudioPlayer) start(id string, status AudioStatus) {
	p.TrackID, p.Status = id, status
	p.SeekTo = 0
	p.SeekSeq++
	p.Fading = false
}

// cue makes id the stopped player's next track.
func (p *AudioPlayer) cue(id string) {
	p.TrackID, p.Status, p.Fading = id, AudioStopped, false
}

// Sync follows an edit of the playlist: the shuffle order takes the change,
// and when the current track was removed the track now at its index (hint,
// -1 unknown) takes over (playing on if the removed one played), or the
// first one. An empty playlist stops.
func (p *AudioPlayer) Sync(pl *models.Playlist, hint int) {
	p.reconcile(pl)
	if pl.IndexOf(p.TrackID) >= 0 {
		return
	}
	if len(pl.Tracks) == 0 {
		p.cue("")
		return
	}
	if hint < 0 || hint >= len(pl.Tracks) {
		hint = 0
	}
	next := pl.Tracks[hint].ID
	if p.Status == AudioPlaying && !p.Fading && p.Playable(pl.Tracks[hint]) {
		p.start(next, AudioPlaying)
		p.reconcile(pl)
		return
	}
	p.cue(next)
	p.reconcile(pl)
}

// Reset is a new playlist (another schedule): stopped, the first track cued.
// The volume and the failed files are kept.
func (p *AudioPlayer) Reset(pl *models.Playlist) {
	p.shuffle = nil
	p.cue("")
	if len(pl.Tracks) > 0 {
		p.TrackID = pl.Tracks[0].ID
	}
	p.reconcile(pl)
}

// Play starts the cued track (or the next playable one after it), or
// resumes a paused one. A Play during a fade-out cancels the fade.
func (p *AudioPlayer) Play(pl *models.Playlist) error {
	p.reconcile(pl)
	switch p.Status {
	case AudioPlaying:
		p.Fading = false
		return nil
	case AudioPaused:
		if t, ok := pl.Track(p.TrackID); ok && p.Playable(t) {
			p.Status = AudioPlaying
			return nil
		}
	}
	order := p.order(pl)
	from := max(p.pos(order), 0)
	i := p.scan(pl, order, from, 1)
	if i < 0 {
		i = p.scan(pl, order, 0, 1) // nothing playable from the cued track on: from the top
	}
	if i < 0 {
		p.cue(p.TrackID)
		return ErrNoPlayableTrack
	}
	p.start(order[i], AudioPlaying)
	return nil
}

// PlayTrack starts the track with id from 0. Under shuffle it is moved to
// the current place of the pass, so the pass still plays every track once.
func (p *AudioPlayer) PlayTrack(pl *models.Playlist, id string) error {
	p.reconcile(pl)
	t, ok := pl.Track(id)
	if !ok {
		return fmt.Errorf("no track %q in the playlist", id)
	}
	if !p.Playable(t) {
		return fmt.Errorf("%q cannot be played (the file is missing or could not be opened)", t.Title)
	}
	if pl.Shuffle && p.shuffle != nil {
		cur := p.pos(p.shuffle)
		from := -1
		for i, s := range p.shuffle {
			if s == id {
				from = i
			}
		}
		switch {
		case from > cur: // still to come: it plays next, the rest keeps its order
			copy(p.shuffle[cur+2:from+1], p.shuffle[cur+1:from])
			p.shuffle[cur+1] = id
		case from >= 0 && from < cur: // played already: it takes the current place, the rest still comes
			copy(p.shuffle[from:cur], p.shuffle[from+1:cur+1])
			p.shuffle[cur] = id
		}
	}
	p.start(id, AudioPlaying)
	return nil
}

// Pause pauses a playing track (and cancels a fade-out).
func (p *AudioPlayer) Pause() {
	if p.Status == AudioPlaying {
		p.Status, p.Fading = AudioPaused, false
	}
}

// Toggle pauses a playing track, else plays.
func (p *AudioPlayer) Toggle(pl *models.Playlist) error {
	if p.Status == AudioPlaying && !p.Fading {
		p.Pause()
		return nil
	}
	return p.Play(pl)
}

// Stop stops the music: playing with a fade-out set, it fades out over
// pl.FadeOut seconds from now first (a second Stop during the fade stops at
// once). The track stays cued and starts from 0 next time.
func (p *AudioPlayer) Stop(pl *models.Playlist, now time.Time) {
	if p.Status == AudioPlaying && !p.Fading && pl.FadeOut > 0 {
		p.Fading, p.FadeStart, p.FadeLen = true, now, pl.FadeOut
		return
	}
	p.cue(p.TrackID)
}

// StopNow stops at once, without a fade.
func (p *AudioPlayer) StopNow() { p.cue(p.TrackID) }

// Next goes to the next playable track (the operator's Next): playing or
// paused stays so on the new track, stopped cues it. After the last track it
// wraps under RepeatAll (a new pass when shuffling), else the player stops
// with the first track cued. RepeatOne does not hold the operator back.
func (p *AudioPlayer) Next(pl *models.Playlist) {
	p.advance(pl)
}

// TrackEnded is the player reaching the end of the current track: RepeatOne
// plays it again, otherwise the next playable track starts (as Next).
func (p *AudioPlayer) TrackEnded(pl *models.Playlist) {
	if p.Status != AudioPlaying {
		return
	}
	if pl.Repeat.Normalized() == models.RepeatOne {
		if t, ok := pl.Track(p.TrackID); ok && p.Playable(t) {
			p.start(p.TrackID, AudioPlaying)
			return
		}
	}
	p.advance(pl)
}

// TrackFailed records that the player could not play the track id and, when
// it is the current one, moves on as at its end (never repeating it).
func (p *AudioPlayer) TrackFailed(pl *models.Playlist, id string) {
	if p.Failed == nil {
		p.Failed = map[string]bool{}
	}
	p.Failed[id] = true
	if id == p.TrackID && p.Status != AudioStopped {
		p.advance(pl)
	}
}

// advance moves to the next playable track in this pass's order.
// A Next during a fade-out plays the next track at full volume.
func (p *AudioPlayer) advance(pl *models.Playlist) {
	p.reconcile(pl)
	status := p.Status
	order := p.order(pl)
	if len(order) == 0 {
		p.cue("")
		return
	}
	cur := p.pos(order)
	if i := p.scan(pl, order, cur+1, 1); i >= 0 {
		p.move(order[i], status)
		return
	}
	// The end of the pass.
	last := p.TrackID
	if pl.Shuffle {
		p.newPass(pl, "", last)
		order = p.shuffle
	}
	first := p.scan(pl, order, 0, 1)
	if first < 0 {
		p.cue(p.TrackID)
		return
	}
	if pl.Repeat.Normalized() == models.RepeatAll {
		p.move(order[first], status)
		return
	}
	p.cue(order[first]) // the list is over: stopped, the first track cued
}

// move switches to id keeping the status (stopped: only cued).
func (p *AudioPlayer) move(id string, status AudioStatus) {
	if status == AudioStopped {
		p.cue(id)
		return
	}
	p.start(id, status)
}

// Prev restarts a track that has played for more than PrevRestart seconds
// (position), else goes to the previous playable track: before the first it
// wraps under RepeatAll, else the first track restarts.
func (p *AudioPlayer) Prev(pl *models.Playlist, position float64) {
	p.reconcile(pl)
	if p.Status != AudioStopped && position > PrevRestart {
		p.start(p.TrackID, p.Status)
		return
	}
	order := p.order(pl)
	if len(order) == 0 {
		p.cue("")
		return
	}
	cur := p.pos(order)
	i := -1
	if cur > 0 {
		i = p.scan(pl, order, cur-1, -1)
	}
	if i < 0 && pl.Repeat.Normalized() == models.RepeatAll {
		i = p.scan(pl, order, len(order)-1, -1)
	}
	if i < 0 {
		i = p.scan(pl, order, max(cur, 0), 1) // restart this one (or the first playable)
	}
	if i < 0 {
		p.cue(p.TrackID)
		return
	}
	p.move(order[i], p.Status)
}

// Seek asks for a seek in the current track (not while stopped).
func (p *AudioPlayer) Seek(secs float64) bool {
	if p.Status == AudioStopped || math.IsNaN(secs) {
		return false
	}
	p.SeekTo = max(secs, 0)
	p.SeekSeq++
	return true
}

// SetVolume sets the volume (0..1).
func (p *AudioPlayer) SetVolume(v float64) { p.Volume = ClampVolume(v) }

// Tick ends a fade-out whose time is up; it reports a change.
func (p *AudioPlayer) Tick(now time.Time) bool {
	if p.Fading && p.Gain(now) <= 0 {
		p.cue(p.TrackID)
		return true
	}
	return false
}

// Gain is the fade-out's volume factor now: 1, ramping to 0 while fading.
func (p *AudioPlayer) Gain(now time.Time) float64 {
	if !p.Fading {
		return 1
	}
	if p.FadeLen <= 0 {
		return 0
	}
	g := 1 - now.Sub(p.FadeStart).Seconds()/p.FadeLen
	return min(max(g, 0), 1)
}

// OutputVolume is the volume the player is given now (volume × fade).
func (p *AudioPlayer) OutputVolume(now time.Time) float64 { return p.Volume * p.Gain(now) }

// Current is the current (or cued) track.
func (p *AudioPlayer) Current(pl *models.Playlist) (models.PlaylistTrack, bool) {
	return pl.Track(p.TrackID)
}

// Wants reports the file the player should have open: the current track
// while playing or paused, "" when stopped.
func (p *AudioPlayer) Wants(pl *models.Playlist) string {
	if p.Status == AudioStopped {
		return ""
	}
	if t, ok := pl.Track(p.TrackID); ok {
		return t.Path
	}
	return ""
}

// PeekNext is the index of the track that follows the current one in this
// pass (-1: the list ends, or the next pass is not shuffled yet). It does not
// change the state.
func (p *AudioPlayer) PeekNext(pl *models.Playlist) int {
	order := p.order(pl)
	if pl.Shuffle && p.shuffle == nil {
		return -1
	}
	if i := p.scan(pl, order, p.pos(order)+1, 1); i >= 0 {
		return pl.IndexOf(order[i])
	}
	if pl.Repeat.Normalized() == models.RepeatAll && !pl.Shuffle {
		if i := p.scan(pl, order, 0, 1); i >= 0 {
			return pl.IndexOf(order[i])
		}
	}
	return -1
}

// Order is this pass's play order as playlist indices.
func (p *AudioPlayer) Order(pl *models.Playlist) []int {
	order := p.order(pl)
	out := make([]int, 0, len(order))
	for _, id := range order {
		if i := pl.IndexOf(id); i >= 0 {
			out = append(out, i)
		}
	}
	return out
}

// ---- /state ----------------------------------------------------------------------

// AudioTrackState is one playlist row in GET /state.
type AudioTrackState struct {
	Title   string  `json:"title"`
	Path    string  `json:"path"`
	Seconds float64 `json:"seconds"`
	Missing bool    `json:"missing,omitempty"` // the file is gone or could not be played
}

// AudioState is the /state document's "audio" object: the playlist, the
// player's state and what the native player reports.
type AudioState struct {
	Status   AudioStatus       `json:"status"` // stopped | playing | paused
	Playing  bool              `json:"playing"`
	Track    int               `json:"track"` // the current (or cued) track's index, -1
	Title    string            `json:"title"` // its title
	Next     int               `json:"next"`  // the track after it in this pass, -1
	Order    []int             `json:"order"` // this pass's play order (indices)
	Position float64           `json:"position"`
	Duration float64           `json:"duration"`
	Volume   float64           `json:"volume"`
	Gain     float64           `json:"gain"` // the fade-out's factor (1 unless fading)
	Fading   bool              `json:"fading"`
	Repeat   models.RepeatMode `json:"repeat"`
	Shuffle  bool              `json:"shuffle"`
	FadeOut  float64           `json:"fadeOut"`
	Tracks   []AudioTrackState `json:"tracks"`
	Native   bool              `json:"native"` // the platform player exists
	Loaded   bool              `json:"loaded"` // the player has the current file open and ready
	Failed   bool              `json:"failed,omitempty"`
}

// AudioReport is what the native player says about the current file.
type AudioReport struct {
	Open     bool // a player has the current track's file
	Ready    bool
	Failed   bool
	Position float64
	Duration float64
}

// State builds the /state document's "audio" object.
func (p *AudioPlayer) State(pl *models.Playlist, now time.Time, rep AudioReport, native bool) AudioState {
	st := AudioState{Status: p.Status, Playing: p.Status == AudioPlaying, Track: pl.IndexOf(p.TrackID),
		Next: p.PeekNext(pl), Order: p.Order(pl), Volume: p.Volume, Gain: p.Gain(now), Fading: p.Fading,
		Repeat: pl.Repeat.Normalized(), Shuffle: pl.Shuffle, FadeOut: pl.FadeOut, Native: native,
		Tracks: make([]AudioTrackState, len(pl.Tracks))}
	for i, t := range pl.Tracks {
		st.Tracks[i] = AudioTrackState{Title: t.Title, Path: t.Path, Seconds: t.Seconds, Missing: !p.Playable(t)}
	}
	if t, ok := pl.Track(p.TrackID); ok {
		st.Title = t.Title
		st.Duration = t.Seconds
	}
	if rep.Open {
		st.Loaded, st.Failed = rep.Ready, rep.Failed
		st.Position = rep.Position
		if rep.Duration > 0 {
			st.Duration = rep.Duration
		}
	}
	return st
}
