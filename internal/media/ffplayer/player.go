// Package ffplayer plays a video or audio file through ffmpeg processes: the
// decoded RGBA frames and f32le stereo samples come over pipes, and the
// player keeps them in sync with a clock (the audio clock when the file has
// sound, the wall clock otherwise).
//
// It is pure Go (no graphics.gd): the renderer's adapter
// (internal/render/ffplayer.go) uploads the frames into a texture and pushes
// the samples into an AudioStreamGenerator, calling Update once per frame.
//
// Video and audio come from two ffmpeg processes started at the same -ss,
// one pipe each (stdout): exec.Cmd.ExtraFiles does not exist on Windows, and
// one process writing two pipes would stall the audio whenever the video
// queue is full. Seeking restarts both at the new -ss. Looping starts the
// next pass at 0 when a pass ends; the video's next process is started
// ahead of time so the first frame of the next pass is already decoded.
//
// Every pass has the length of the container's duration D (the first one
// D - ss): the audio is padded with silence or cut to exactly that many
// samples, and video frames past it are dropped, so the audio and the video
// timelines agree on every loop and nothing drifts.
//
// Methods are safe for concurrent use, but are meant to be called from one
// thread (the Godot main thread).
package ffplayer

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Status is where the player is with its file.
type Status int

const (
	Loading Status = iota // probing, or waiting for the first decoded data
	Ready                 // playable (the first frame or samples arrived)
	Failed                // the file cannot be played (Err says why)
)

// Config says which tools to run and how to decode.
type Config struct {
	FFmpeg  string // path of ffmpeg (required)
	FFprobe string // path of ffprobe (required)

	MaxWidth, MaxHeight int // frames are scaled down to fit; 0: 1920 × 1080
	SampleRate          int // audio output rate; 0: 48000

	// AudioAhead is how much decoded audio is kept ready; 0: one second.
	AudioAhead time.Duration

	// Now is the wall clock (tests); nil: time.Now.
	Now func() time.Time
}

// Frame is a decoded video frame.
type Frame struct {
	Pix           []byte // RGBA8, Width*Height*4 bytes, rows top-down; valid until the next Update
	Width, Height int
	Position      float64 // media time of the frame (seconds)
}

const (
	defaultMaxW = 1920
	defaultMaxH = 1080
	defaultRate = 48000

	// frameBuffers bounds the decoded frames in memory per player: two
	// queued, one shown, one waiting for its time.
	frameBuffers = 4
	queuedFrames = 2

	// An audio clock that does not move for stallTimeout and stallUpdates
	// Updates while playing (no audio device, a stuck decoder) hands over to
	// the wall clock until the sound moves again.
	stallTimeout = 2 * time.Second
	stallUpdates = 30
)

// Player plays one file.
type Player struct {
	cfg  Config
	path string
	rate int

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	done   chan struct{}

	mu       sync.Mutex
	closed   bool
	probed   bool
	status   Status
	err      error
	info     Info
	loop     bool
	loopWake chan struct{} // closed and replaced whenever loop changes
	seekTo   float64       // a seek asked before the probe finished

	run         *run
	playing     bool
	atEnd       bool // stopped at the end with looping off
	ended       bool // ... and not reported yet (TakeEnded)
	clockT      float64
	clockAt     time.Time
	lastT       float64 // the timeline position of the last Update
	lastAdvance time.Time
	audioOut    int64 // frames handed out by ReadAudio this run
	backlog     int   // of those, frames not heard yet (SetAudioBacklog)
	audioWall   bool  // the sound ran out: the wall clock runs for the rest of this run
	epoch       int

	lastHeard float64 // the audio clock at its last move
	stalls    int     // Updates since then
	stalled   bool    // the audio clock stalled: the wall clock runs meanwhile

	cur, pending *frame
	fresh        bool
}

// frame is a decoded frame in flight.
type frame struct {
	pix []byte
	t   float64 // run timeline (seconds since the run's -ss)
	pos float64 // media time
}

// run is one start of the decoders at a position (Open, Seek, a restart).
type run struct {
	ctx    context.Context
	cancel context.CancelFunc
	ss     float64
	first  float64 // length of the first pass (D - ss); 0 when D is unknown

	frames chan *frame
	free   chan []byte
	audio  *ring

	vIdle, aIdle       atomic.Bool // the reader is at its end, waiting for looping (or gone)
	gotVideo, gotAudio atomic.Bool
	showFirst          bool // main thread: show the next frame at once

	fmu  sync.Mutex
	fail error
}

func (r *run) setFail(err error) {
	r.fmu.Lock()
	if r.fail == nil {
		r.fail = err
	}
	r.fmu.Unlock()
}

func (r *run) failure() error {
	r.fmu.Lock()
	defer r.fmu.Unlock()
	return r.fail
}

// Open starts probing path; the player is Loading until the first frame (or,
// for audio, the first samples) is decoded, paused at 0.
func Open(cfg Config, path string) *Player {
	if cfg.MaxWidth <= 0 {
		cfg.MaxWidth = defaultMaxW
	}
	if cfg.MaxHeight <= 0 {
		cfg.MaxHeight = defaultMaxH
	}
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = defaultRate
	}
	if cfg.AudioAhead <= 0 {
		cfg.AudioAhead = time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	p := &Player{cfg: cfg, path: path, rate: cfg.SampleRate, done: make(chan struct{}), loopWake: make(chan struct{})}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	if cfg.FFmpeg == "" || cfg.FFprobe == "" {
		p.status, p.err, p.probed = Failed, errors.New("ffmpeg / ffprobe not found"), true
		p.cancel()
		close(p.done)
		return p
	}
	p.wg.Add(1)
	go p.probe()
	go func() {
		<-p.ctx.Done()
		p.wg.Wait()
		close(p.done)
	}()
	return p
}

func (p *Player) probe() {
	defer p.wg.Done()
	info, err := probe(p.ctx, p.cfg.FFprobe, p.path, p.cfg.MaxWidth, p.cfg.MaxHeight)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.probed = true
	if err != nil {
		p.status, p.err = Failed, err
		return
	}
	p.info = info
	p.startRun(p.clampSeek(p.seekTo))
}

// Close kills the decoders. Done is closed once every goroutine and process
// is gone.
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.playing = false
	p.cancel()
	p.run = nil
	p.cur, p.pending = nil, nil
}

// Done is closed when the player has stopped every goroutine and process
// after Close.
func (p *Player) Done() <-chan struct{} { return p.done }

// Status reports where the player is.
func (p *Player) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.status == Loading && p.run != nil {
		r := p.run
		switch {
		case r.failure() != nil:
			p.status, p.err = Failed, r.failure()
		case p.info.HasVideo && r.gotVideo.Load(), !p.info.HasVideo && r.gotAudio.Load():
			p.status = Ready
		}
	}
	return p.status
}

// Err is why the player failed.
func (p *Player) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// Info is what the probe found (zero until then).
func (p *Player) Info() Info {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.info
}

// Duration is the file's duration in seconds (0 until probed or unknown).
func (p *Player) Duration() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.info.Duration
}

// SampleRate is the rate of the samples ReadAudio hands out.
func (p *Player) SampleRate() int { return p.rate }

// Play starts or resumes playback. A player that stopped at its end starts
// again from 0.
func (p *Player) Play() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.playing {
		return
	}
	if p.atEnd {
		p.startRun(0)
	}
	now := p.cfg.Now()
	p.playing = true
	p.clockAt, p.lastAdvance, p.stalls = now, now, 0
}

// Pause stops the clock; the position holds.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.playing {
		return
	}
	p.clockT = p.timeline(p.cfg.Now())
	p.lastT = p.clockT
	p.playing = false
}

// Playing reports whether the clock runs.
func (p *Player) Playing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

// Seek restarts the decoders at secs (clamped to the duration). The
// position is secs at once; the first frame there replaces the shown one
// as soon as it is decoded, playing or not.
func (p *Player) Seek(secs float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if !p.probed || p.status == Failed {
		p.seekTo = max(secs, 0)
		return
	}
	p.startRun(p.clampSeek(secs))
}

func (p *Player) clampSeek(secs float64) float64 {
	secs = max(secs, 0)
	if d := p.info.Duration; d > 0 {
		secs = min(secs, d)
	}
	return secs
}

// SetLoop makes the file start again at 0 when it ends.
func (p *Player) SetLoop(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if on == p.loop {
		return
	}
	p.loop = on
	close(p.loopWake)
	p.loopWake = make(chan struct{})
}

// Loop reports the looping flag.
func (p *Player) Loop() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loop
}

// TakeEnded reports (once) that playback stopped at the end with looping off.
func (p *Player) TakeEnded() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.ended
	p.ended = false
	return e
}

// Position is the media time of the last Update (seconds).
func (p *Player) Position() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.run == nil {
		return p.seekTo
	}
	return p.mediaPos(p.lastT)
}

// Update advances the clock, stops at the end when looping is off and picks
// the video frame that is due. Call it once per displayed frame.
func (p *Player) Update() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.run == nil {
		return
	}
	t := p.timeline(p.cfg.Now())
	if p.playing && !p.loop {
		if b, ok := p.nextBoundary(p.lastT); ok {
			if t >= b {
				t = b
				p.stopAtEnd(t)
			}
		} else if p.drained() {
			p.stopAtEnd(t)
		}
	}
	p.lastT = t
	p.pickFrame(t)
}

// Frame returns the newest due frame when it changed since the last call.
func (p *Player) Frame() (Frame, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.fresh || p.cur == nil {
		return Frame{}, false
	}
	p.fresh = false
	return Frame{Pix: p.cur.pix, Width: p.info.FrameWidth, Height: p.info.FrameHeight, Position: p.cur.pos}, true
}

// AudioEpoch changes whenever audio handed out earlier must be dropped
// (a seek or a restart): the caller clears its output buffer then.
func (p *Player) AudioEpoch() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.epoch
}

// ReadAudio moves decoded samples (interleaved stereo float32) into dst
// while playing and returns the number of frames (sample pairs). With
// looping off nothing past the end is handed out.
func (p *Player) ReadAudio(dst []float32) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	r := p.run
	if r == nil || r.audio == nil || !p.playing || p.atEnd {
		return 0
	}
	n := len(dst) / 2
	if !p.loop {
		if b, ok := p.nextBoundary(p.lastT); ok {
			n = int(min(int64(n), int64(math.Round(b*float64(p.rate)))-p.audioOut))
		}
	}
	if n <= 0 {
		return 0
	}
	got := r.audio.read(dst[:2*n])
	p.audioOut += int64(got)
	return got
}

// SetAudioBacklog tells the player how many of the frames ReadAudio handed
// out in epoch are still queued for the sound card (not heard yet): the
// audio clock is what has been heard.
func (p *Player) SetAudioBacklog(epoch, frames int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if epoch == p.epoch {
		p.backlog = max(frames, 0)
	}
}

// ---- clock ----------------------------------------------------------------------------

// timeline is the run's clock (seconds since its -ss) at now.
func (p *Player) timeline(now time.Time) float64 {
	if !p.playing {
		return p.clockT
	}
	if !p.audioClock() {
		return p.clockT + now.Sub(p.clockAt).Seconds()
	}
	heard := float64(p.audioOut-int64(p.backlog)) / float64(p.rate)
	if heard != p.lastHeard {
		p.lastHeard, p.lastAdvance, p.stalls = heard, now, 0
		if p.stalled { // the sound moves again: back to the audio clock, never backwards
			p.stalled = false
			p.clockT = max(p.clockT+now.Sub(p.clockAt).Seconds(), heard)
			p.clockAt = now
			return p.clockT
		}
	} else if !p.stalled {
		p.stalls++
		// Many frames and a long time without a sample heard: no audio
		// device, or a stuck decoder. One long hitch (a modal dialog) is not
		// a stall: the frame count stays low.
		if p.stalls >= stallUpdates && now.Sub(p.lastAdvance) > stallTimeout {
			p.stalled = true
		}
	}
	if p.stalled {
		return p.clockT + now.Sub(p.clockAt).Seconds()
	}
	p.clockT = max(p.clockT, heard)
	p.clockAt = now
	return p.clockT
}

// audioClock reports whether the audio drives the clock: the file has sound
// that has not run out or stalled.
func (p *Player) audioClock() bool {
	r := p.run
	if r.audio == nil || p.audioWall {
		return false
	}
	if r.aIdle.Load() && r.audio.frames() == 0 && p.backlog == 0 {
		// The sound ran out: the video carries on by the wall clock.
		p.audioWall = true
		if !p.stalled {
			p.clockT = max(p.clockT, float64(p.audioOut)/float64(p.rate))
			p.clockAt = p.cfg.Now()
		}
		return false
	}
	return true
}

// nextBoundary is the first end of a pass at or after t (ok false when the
// duration is unknown).
func (p *Player) nextBoundary(t float64) (float64, bool) {
	d := p.info.Duration
	if d <= 0 {
		return 0, false
	}
	first := p.run.first
	if t <= first {
		return first, true
	}
	return first + math.Ceil((t-first)/d-1e-9)*d, true
}

// mediaPos maps the run timeline to the file's time.
func (p *Player) mediaPos(t float64) float64 {
	d := p.info.Duration
	if d <= 0 {
		return p.run.ss + t
	}
	if p.atEnd {
		return d
	}
	if t <= p.run.first {
		return min(p.run.ss+t, d)
	}
	return math.Mod(t-p.run.first, d)
}

// drained reports, for a file without a known duration, that every stream
// reached its end and was played out.
func (p *Player) drained() bool {
	r := p.run
	if p.info.HasVideo && !(r.vIdle.Load() && len(r.frames) == 0 && p.pending == nil) {
		return false
	}
	if r.audio != nil && !(r.aIdle.Load() && r.audio.frames() == 0 && p.backlog == 0) {
		return false
	}
	return true
}

func (p *Player) stopAtEnd(t float64) {
	p.clockT = t
	p.playing = false
	p.atEnd = true
	p.ended = true
}

// pickFrame takes the frames whose time has come; the newest one is shown.
func (p *Player) pickFrame(t float64) {
	r := p.run
	if !p.info.HasVideo {
		return
	}
	tol := 0.5 / p.info.FPS
	for {
		if p.pending == nil {
			select {
			case f := <-r.frames:
				p.pending = f
			default:
				return
			}
		}
		if !r.showFirst && p.pending.t > t+tol {
			return
		}
		if p.cur != nil {
			p.recycle(p.cur.pix)
		}
		p.cur, p.pending = p.pending, nil
		p.fresh = true
		r.showFirst = false
	}
}

func (p *Player) recycle(buf []byte) { p.run.giveBack(buf) }

// giveBack returns a frame buffer to the free list (dropped when it is full).
func (r *run) giveBack(buf []byte) {
	select {
	case r.free <- buf:
	default:
	}
}

// startRun (re)starts the decoders at ss and resets the clock there.
func (p *Player) startRun(ss float64) {
	if p.run != nil {
		p.run.cancel()
	}
	r := &run{ss: ss, frames: make(chan *frame, queuedFrames), free: make(chan []byte, frameBuffers), showFirst: true}
	r.ctx, r.cancel = context.WithCancel(p.ctx)
	for range frameBuffers {
		r.free <- nil
	}
	if d := p.info.Duration; d > 0 {
		r.first = max(d-ss, 0)
	}
	p.run = r
	p.pending = nil
	p.atEnd, p.ended = false, false
	p.clockT, p.lastT = 0, 0
	now := p.cfg.Now()
	p.clockAt, p.lastAdvance = now, now
	p.audioOut, p.backlog, p.audioWall = 0, 0, false
	p.lastHeard, p.stalls, p.stalled = 0, 0, false
	p.epoch++
	info := p.info
	if info.HasAudio {
		r.audio = newRing(int(p.cfg.AudioAhead.Seconds() * float64(p.rate)))
		context.AfterFunc(r.ctx, r.audio.close)
		p.wg.Add(1)
		go p.audioReader(r, info)
	}
	if info.HasVideo {
		p.wg.Add(1)
		go p.videoReader(r, info)
	}
}

// waitLoop parks a reader at the end of a pass until looping is on (true)
// or the run is over (false). idle is set while it waits.
func (p *Player) waitLoop(r *run, idle *atomic.Bool) bool {
	for {
		p.mu.Lock()
		on, wake := p.loop, p.loopWake
		p.mu.Unlock()
		if on {
			idle.Store(false)
			return true
		}
		idle.Store(true)
		select {
		case <-r.ctx.Done():
			return false
		case <-wake:
		}
	}
}

func (p *Player) looping() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loop
}
