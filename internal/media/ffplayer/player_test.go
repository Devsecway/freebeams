package ffplayer

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// The tests decode small clips that ffmpeg generates into a temp dir (once
// per run). Without ffmpeg / ffprobe on PATH they skip, unless
// FREEBEAMS_REQUIRE_FFMPEG=1 (CI) makes that a failure.

const testRate = 8000

var (
	clipsOnce sync.Once
	clipsDir  string
	clipsErr  error
	ffmpegBin string // makes the clips (PATH)
	probeBin  string

	// The player runs these: PATH, or the ffmpeg / ffprobe in
	// $FREEBEAMS_FFMPEG_DIR (to test the bundled LGPL build, bin/build-ffmpeg).
	playFFmpeg  string
	playFFprobe string
)

// clip returns the path of a generated test clip.
func clip(t *testing.T, name string) string {
	t.Helper()
	clipsOnce.Do(makeClips)
	if clipsErr != nil {
		if os.Getenv("FREEBEAMS_REQUIRE_FFMPEG") == "1" {
			t.Fatalf("test clips: %v", clipsErr)
		}
		t.Skipf("test clips: %v", clipsErr)
	}
	return filepath.Join(clipsDir, name)
}

func makeClips() {
	var err error
	if ffmpegBin, err = exec.LookPath("ffmpeg"); err != nil {
		clipsErr = err
		return
	}
	if probeBin, err = exec.LookPath("ffprobe"); err != nil {
		clipsErr = err
		return
	}
	playFFmpeg, playFFprobe = ffmpegBin, probeBin
	if dir := os.Getenv("FREEBEAMS_FFMPEG_DIR"); dir != "" {
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}
		playFFmpeg, playFFprobe = filepath.Join(dir, "ffmpeg"+ext), filepath.Join(dir, "ffprobe"+ext)
	}
	clipsDir, err = os.MkdirTemp("", "ffplayer-test")
	if err != nil {
		clipsErr = err
		return
	}
	// Native encoders only (mpeg4, aac, pcm), so any ffmpeg build works.
	red := "color=c=red:s=64x36:r=10:d=1"
	blue := "color=c=blue:s=64x36:r=10:d=1"
	jobs := map[string][]string{
		// 2 s, red then blue, 10 fps, with a stereo 440 Hz tone.
		"av.mp4": {"-f", "lavfi", "-i", red, "-f", "lavfi", "-i", blue, "-f", "lavfi", "-i", "sine=frequency=440:duration=2:sample_rate=44100",
			"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[v]", "-map", "[v]", "-map", "2:a", "-ac", "2",
			"-c:v", "mpeg4", "-q:v", "2", "-c:a", "aac", "-shortest", "av.mp4"},
		// 1 s, red then blue, no sound.
		"video.mp4": {"-f", "lavfi", "-i", "color=c=red:s=64x36:r=10:d=0.5", "-f", "lavfi", "-i", "color=c=blue:s=64x36:r=10:d=0.5",
			"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[v]", "-map", "[v]", "-c:v", "mpeg4", "-q:v", "2", "video.mp4"},
		// The smoke test's colour: #2a7f62 tagged BT.709 limited range.
		"green709.mkv": {"-f", "lavfi", "-i", "color=c=0x2a7f62:s=64x36:r=10:d=0.5",
			"-vf", "scale=out_color_matrix=bt709:out_range=tv,format=yuv420p", "-colorspace", "bt709", "-color_primaries", "bt709",
			"-color_trc", "bt709", "-color_range", "tv", "-c:v", "ffv1", "green709.mkv"},
		// 1 s mono tone.
		"tone.wav": {"-f", "lavfi", "-i", "sine=frequency=440:duration=1:sample_rate=22050", "-c:a", "pcm_s16le", "tone.wav"},
		// 2 s, 1920×1080 scaled to fit 640×360 in the tests that set it.
		"big.mp4": {"-f", "lavfi", "-i", "color=c=white:s=1920x1080:r=5:d=0.4", "-c:v", "mpeg4", "big.mp4"},
	}
	for name, args := range jobs {
		cmd := exec.Command(ffmpegBin, append([]string{"-v", "error", "-y"}, args...)...)
		cmd.Dir = clipsDir
		if out, err := cmd.CombinedOutput(); err != nil {
			clipsErr = fmt.Errorf("make %s: %v: %s", name, err, out)
			return
		}
	}
	if err := os.WriteFile(filepath.Join(clipsDir, "text.mp4"), []byte("not a video\n"), 0o644); err != nil {
		clipsErr = err
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	if clipsDir != "" {
		os.RemoveAll(clipsDir)
	}
	os.Exit(code)
}

// fakeClock is a settable wall clock.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1000, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(secs float64) {
	c.mu.Lock()
	c.t = c.t.Add(time.Duration(secs * float64(time.Second)))
	c.mu.Unlock()
}

// harness drives one player like the renderer does.
type harness struct {
	t     *testing.T
	p     *Player
	clock *fakeClock
	last  Frame // copy of the newest frame
	got   bool
}

func open(t *testing.T, name string, mod ...func(*Config)) *harness {
	t.Helper()
	path := clip(t, name)
	h := &harness{t: t, clock: newFakeClock()}
	cfg := Config{FFmpeg: playFFmpeg, FFprobe: playFFprobe, SampleRate: testRate, Now: h.clock.Now}
	for _, f := range mod {
		f(&cfg)
	}
	h.p = Open(cfg, path)
	t.Cleanup(func() {
		h.p.Close()
		select {
		case <-h.p.Done():
		case <-time.After(10 * time.Second):
			t.Error("player did not stop within 10 s of Close")
		}
	})
	return h
}

// update runs one Update and keeps the newest frame.
func (h *harness) update() {
	h.p.Update()
	if f, ok := h.p.Frame(); ok {
		h.last = Frame{Pix: append([]byte(nil), f.Pix...), Width: f.Width, Height: f.Height, Position: f.Position}
		h.got = true
	}
}

// until updates (in real time) until cond holds or 10 s passed.
func (h *harness) until(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		h.update()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s (status %v, err %v, position %.3f)", what, h.p.Status(), h.p.Err(), h.p.Position())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (h *harness) ready() {
	h.t.Helper()
	h.until("ready", func() bool { return h.p.Status() != Loading })
	if h.p.Status() != Ready {
		h.t.Fatalf("status %v: %v", h.p.Status(), h.p.Err())
	}
}

// hear plays secs of audio: reads it like the sound card and reports it
// heard (backlog 0). It returns the frames read.
func (h *harness) hear(secs float64) int {
	h.t.Helper()
	want := int(math.Round(secs * testRate))
	buf := make([]float32, 2*512)
	total := 0
	deadline := time.Now().Add(10 * time.Second)
	for total < want {
		n := h.p.ReadAudio(buf[:2*min(512, want-total)])
		if n == 0 {
			if !h.p.Playing() {
				return total
			}
			if time.Now().After(deadline) {
				h.t.Fatalf("no audio after %d of %d frames", total, want)
			}
			h.update()
			time.Sleep(time.Millisecond)
			continue
		}
		total += n
		h.p.SetAudioBacklog(h.p.AudioEpoch(), 0)
		h.update()
	}
	return total
}

// color names the first pixel of the newest frame.
func (h *harness) color() string {
	if !h.got {
		return "none"
	}
	r, g, b := int(h.last.Pix[0]), int(h.last.Pix[1]), int(h.last.Pix[2])
	switch {
	case r > 200 && g < 60 && b < 60:
		return "red"
	case b > 200 && r < 60 && g < 60:
		return "blue"
	case r > 200 && g > 200 && b > 200:
		return "white"
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestOpenPausedShowsFirstFrame(t *testing.T) {
	h := open(t, "av.mp4")
	if s := h.p.Status(); s != Loading {
		t.Fatalf("status right after Open = %v, want Loading", s)
	}
	h.ready()
	info := h.p.Info()
	if !info.HasVideo || !info.HasAudio || info.Width != 64 || info.Height != 36 || info.FrameWidth != 64 || info.FPS != 10 || !near(info.Duration, 2, 0.05) {
		t.Fatalf("info = %+v", info)
	}
	h.until("the first frame", func() bool { return h.got })
	if h.color() != "red" || h.last.Position != 0 || h.last.Width != 64 || h.last.Height != 36 || len(h.last.Pix) != 64*36*4 {
		t.Fatalf("first frame: %s at %v, %dx%d, %d bytes", h.color(), h.last.Position, h.last.Width, h.last.Height, len(h.last.Pix))
	}
	h.clock.Advance(1.5)
	for range 20 {
		h.update()
	}
	if pos := h.p.Position(); pos != 0 || h.p.Playing() || h.p.ReadAudio(make([]float32, 64)) != 0 {
		t.Fatalf("paused player moved: position %v, playing %v", pos, h.p.Playing())
	}
}

func TestExactColor(t *testing.T) {
	h := open(t, "green709.mkv")
	h.ready()
	h.until("the first frame", func() bool { return h.got })
	want := [3]int{0x2a, 0x7f, 0x62}
	for i, w := range want {
		if got := int(h.last.Pix[i]); got < w-3 || got > w+3 {
			t.Fatalf("pixel = #%02x%02x%02x, want #2a7f62 ±3", h.last.Pix[0], h.last.Pix[1], h.last.Pix[2])
		}
	}
	if h.last.Pix[3] != 255 {
		t.Fatalf("alpha = %d", h.last.Pix[3])
	}
}

func TestScaledToFit(t *testing.T) {
	h := open(t, "big.mp4", func(c *Config) { c.MaxWidth, c.MaxHeight = 640, 360 })
	h.ready()
	info := h.p.Info()
	if info.Width != 1920 || info.Height != 1080 || info.FrameWidth != 640 || info.FrameHeight != 360 {
		t.Fatalf("info = %+v", info)
	}
	h.until("a frame", func() bool { return h.got })
	if h.last.Width != 640 || len(h.last.Pix) != 640*360*4 || h.color() != "white" {
		t.Fatalf("frame %dx%d %s", h.last.Width, h.last.Height, h.color())
	}
}

func TestWallClockPlayback(t *testing.T) {
	h := open(t, "video.mp4")
	h.ready()
	if h.p.Info().HasAudio {
		t.Fatal("video.mp4 has no sound")
	}
	h.p.Play()
	h.clock.Advance(0.25)
	h.update()
	if pos := h.p.Position(); !near(pos, 0.25, 1e-6) {
		t.Fatalf("position %v after 0.25 s", pos)
	}
	h.clock.Advance(0.4) // 0.65: the blue half
	h.until("the blue half", func() bool { return h.color() == "blue" })
	if !near(h.last.Position, 0.6, 0.11) {
		t.Fatalf("blue frame at %v", h.last.Position)
	}
	h.p.Pause()
	held := h.p.Position()
	h.clock.Advance(1)
	h.update()
	if h.p.Position() != held || h.p.Playing() {
		t.Fatalf("paused: position %v → %v", held, h.p.Position())
	}
	// Looping off (the default): the end stops it once.
	h.p.Play()
	h.clock.Advance(2)
	h.update()
	if h.p.Playing() || !h.p.TakeEnded() || h.p.TakeEnded() {
		t.Fatal("the end was not reported exactly once")
	}
	if pos, d := h.p.Position(), h.p.Duration(); pos != d || !near(d, 1, 0.05) {
		t.Fatalf("at the end: position %v, duration %v", pos, d)
	}
	// Play after the end starts again from 0 with the first frame.
	h.p.Play()
	h.update()
	if pos := h.p.Position(); pos != 0 || !h.p.Playing() {
		t.Fatalf("restart: position %v, playing %v", pos, h.p.Playing())
	}
	h.until("red again", func() bool { return h.color() == "red" })
}

func TestAudioClockAndGaplessLoop(t *testing.T) {
	h := open(t, "av.mp4")
	h.ready()
	h.p.SetLoop(true)
	h.p.Play()
	h.clock.Advance(5) // the wall clock does not drive a file with sound
	h.update()
	if pos := h.p.Position(); pos != 0 {
		t.Fatalf("position %v before any audio was heard", pos)
	}
	h.hear(0.5)
	if pos := h.p.Position(); !near(pos, 0.5, 1e-6) {
		t.Fatalf("position %v after hearing 0.5 s", pos)
	}
	h.hear(1.0) // 1.5 s: blue
	h.until("blue at 1.5 s", func() bool { return h.color() == "blue" })
	// Across the loop point: the audio is continuous and the position wraps.
	h.hear(0.8) // 2.3 s → 0.3 s into the second pass
	if pos := h.p.Position(); !near(pos, 0.3, 1e-6) {
		t.Fatalf("position %v after the loop, want 0.3", pos)
	}
	h.until("red after the loop", func() bool { return h.color() == "red" && h.last.Position < 0.5 })
	if !h.p.Playing() || h.p.TakeEnded() {
		t.Fatal("a looping file ended")
	}
	// Looping off in the second pass: it ends at the end of that pass, the
	// audio stops at exactly 2 × duration.
	h.p.SetLoop(false)
	rest := h.hear(10)
	d := h.p.Duration()
	if want := int(math.Round((2*d - 2.3) * testRate)); rest != want {
		t.Fatalf("heard %d frames to the end, want %d", rest, want)
	}
	h.update()
	if h.p.Playing() || !h.p.TakeEnded() || h.p.Position() != d {
		t.Fatalf("end: playing %v, position %v", h.p.Playing(), h.p.Position())
	}
}

func TestOnePassIsExactlyTheDuration(t *testing.T) {
	h := open(t, "av.mp4")
	h.ready()
	h.p.Play()
	n := h.hear(10)
	if want := int(math.Round(h.p.Duration() * testRate)); n != want {
		t.Fatalf("one pass = %d frames, want %d", n, want)
	}
	h.update()
	if !h.p.TakeEnded() {
		t.Fatal("no end")
	}
}

func TestSeek(t *testing.T) {
	h := open(t, "av.mp4")
	h.ready()
	h.until("red", func() bool { return h.color() == "red" })
	h.p.Seek(1.5)
	if pos := h.p.Position(); pos != 1.5 {
		t.Fatalf("position %v right after the seek", pos)
	}
	h.until("the frame at 1.5 s while paused", func() bool { return h.color() == "blue" })
	if !near(h.last.Position, 1.5, 1e-6) || h.p.Playing() {
		t.Fatalf("frame at %v, playing %v", h.last.Position, h.p.Playing())
	}
	epoch := h.p.AudioEpoch()
	h.p.Play()
	h.hear(0.2)
	if pos := h.p.Position(); !near(pos, 1.7, 1e-6) {
		t.Fatalf("position %v after 0.2 s from 1.5", pos)
	}
	h.p.Seek(0.25)
	if h.p.AudioEpoch() == epoch || !h.p.Playing() {
		t.Fatal("a seek must start a new audio epoch and keep playing")
	}
	h.until("red after seeking back", func() bool { return h.color() == "red" })
	h.p.Seek(99)
	if pos := h.p.Position(); pos != h.p.Duration() {
		t.Fatalf("seek past the end: position %v", pos)
	}
}

func TestSeekBeforeReady(t *testing.T) {
	h := open(t, "av.mp4")
	h.p.Seek(1.2)
	h.ready()
	h.until("blue", func() bool { return h.color() == "blue" })
	if pos := h.p.Position(); pos != 1.2 {
		t.Fatalf("position %v", pos)
	}
}

func TestAudioOnly(t *testing.T) {
	h := open(t, "tone.wav")
	h.ready()
	info := h.p.Info()
	if info.HasVideo || !info.HasAudio || !near(info.Duration, 1, 0.01) {
		t.Fatalf("info = %+v", info)
	}
	h.p.Play()
	h.hear(0.4)
	if pos := h.p.Position(); !near(pos, 0.4, 1e-6) {
		t.Fatalf("position %v", pos)
	}
	buf := make([]float32, 2*256)
	if n := h.p.ReadAudio(buf); n == 0 {
		t.Fatal("no samples")
	}
	peak := float32(0)
	for i := 0; i < len(buf); i += 2 {
		if buf[i] != buf[i+1] {
			t.Fatal("mono is not upmixed to both channels")
		}
		peak = max(peak, buf[i])
	}
	if peak < 0.05 {
		t.Fatalf("silent tone (peak %v)", peak)
	}
}

func TestWallClockWhenAudioIsNotConsumed(t *testing.T) {
	h := open(t, "av.mp4")
	h.ready()
	h.p.Play()
	h.update()
	// One long hitch is not a stall: the clock still waits for the sound.
	h.clock.Advance(stallTimeout.Seconds() + 0.1)
	h.update()
	h.clock.Advance(0.5)
	h.update()
	if pos := h.p.Position(); pos != 0 {
		t.Fatalf("position %v after one hitch, want 0", pos)
	}
	for range stallUpdates {
		h.update() // the audio clock stalled: the wall clock takes over from here
	}
	h.clock.Advance(0.5)
	h.update()
	if pos := h.p.Position(); !near(pos, 0.5, 1e-6) {
		t.Fatalf("position %v, want 0.5 on the wall clock", pos)
	}
	// The sound moves again: the audio clock is back, never going backwards.
	h.hear(0.2)
	if pos := h.p.Position(); !near(pos, 0.5, 1e-6) {
		t.Fatalf("position %v, want 0.5 (held until the sound catches up)", pos)
	}
	h.hear(0.5)
	if pos := h.p.Position(); !near(pos, 0.7, 1e-6) {
		t.Fatalf("position %v, want 0.7 on the audio clock", pos)
	}
}

func TestFailures(t *testing.T) {
	for _, name := range []string{"text.mp4", "missing.mp4"} {
		t.Run(name, func(t *testing.T) {
			h := open(t, name)
			h.until("failure", func() bool { return h.p.Status() != Loading })
			if h.p.Status() != Failed || h.p.Err() == nil {
				t.Fatalf("status %v, err %v", h.p.Status(), h.p.Err())
			}
			h.p.Play()
			h.p.Seek(1)
			h.update()
			if h.p.Duration() != 0 {
				t.Fatal("a failed file has a duration")
			}
		})
	}
	p := Open(Config{}, "x.mp4")
	if p.Status() != Failed {
		t.Fatal("no ffmpeg must fail")
	}
	<-p.Done()
}

func TestCloseStopsEverything(t *testing.T) {
	path := clip(t, "av.mp4")
	before := runtime.NumGoroutine()
	for range 3 {
		p := Open(Config{FFmpeg: playFFmpeg, FFprobe: playFFprobe, SampleRate: testRate}, path)
		deadline := time.Now().Add(10 * time.Second)
		for p.Status() == Loading && time.Now().Before(deadline) {
			p.Update()
			time.Sleep(2 * time.Millisecond)
		}
		p.SetLoop(true)
		p.Play()
		p.Seek(0.5) // a second run, the first one cancelled
		time.Sleep(50 * time.Millisecond)
		p.Close()
		p.Close() // idempotent
		select {
		case <-p.Done():
		case <-time.After(10 * time.Second):
			t.Fatal("not stopped 10 s after Close")
		}
		p.Update() // harmless after Close
		p.Play()
	}
	var now int
	for range 100 {
		if now = runtime.NumGoroutine(); now <= before {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutines: %d before, %d after Close", before, now)
}
