package render

import (
	"freebeams/internal/keepalive"
	"freebeams/internal/media/ffplayer"

	"graphics.gd/classdb/AudioServer"
	"graphics.gd/classdb/AudioStreamGenerator"
	"graphics.gd/classdb/AudioStreamGeneratorPlayback"
	"graphics.gd/classdb/AudioStreamPlayer"
	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// The ffmpeg player behind nativePlayer (docs/MEDIA.md "ffmpeg player"):
// internal/media/ffplayer runs the decoders and the clock; this adapter
// copies its RGBA frames into the source's texture and pushes its samples
// into an AudioStreamGenerator played by an AudioStreamPlayer under the
// scene root. Everything here runs on the main thread, from pumpSources.

// audioBufferSecs is the generator's buffer: what Godot holds ahead of the
// sound card. The player's clock subtracts what is still in it.
const audioBufferSecs = 0.25

type ffPlayer struct {
	// pl is the decoder: its goroutines write its state, so it is hidden
	// from graphics.gd's keepalive walk (internal/keepalive). Read it with p().
	pl keepalive.Hidden[ffplayer.Player]

	// Audio (files with sound, created once the player is ready).
	audio    AudioStreamPlayer.Instance
	hasNode  bool
	started  bool // Play was called on the AudioStreamPlayer
	paused   bool // stream_paused as last set
	epoch    int  // the player's audio epoch the generator holds
	capacity int  // the generator's buffer in frames (its free space when empty)
	volume   float64
	pcm      []float32
	xy       []Vector2.XY

	frame     ffplayer.Frame // a frame CopyFrame could not take yet (buffer too small)
	haveFrame bool
}

// p is the decoder.
func (f *ffPlayer) p() *ffplayer.Player { return f.pl.Get() }

// openFFPlayer starts the ffmpeg player on path.
func openFFPlayer(path string) (nativePlayer, error) {
	cfg := ffConfig
	if rate := int(AudioServer.GetMixRate()); rate > 0 {
		cfg.SampleRate = rate
	}
	return &ffPlayer{pl: keepalive.Hide(ffplayer.Open(cfg, path)), volume: 1, epoch: -1}, nil
}

// sceneRoot is the root window, where the AudioStreamPlayers go: sources
// outlive the views that opened them, and the playlist's (RoleAudio) has
// no view at all.
func sceneRoot() (Node.Instance, bool) {
	tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop())
	if !ok {
		return Node.Nil, false
	}
	return tree.Root().AsNode(), true
}

func (f *ffPlayer) Play()           { f.p().Play() }
func (f *ffPlayer) Pause()          { f.p().Pause() }
func (f *ffPlayer) Seek(s float64)  { f.p().Seek(s) }
func (f *ffPlayer) SetLoop(on bool) { f.p().SetLoop(on) }

func (f *ffPlayer) SetVolume(v float64) {
	if v == f.volume {
		return
	}
	f.volume = v
	if f.hasNode {
		f.audio.SetVolumeLinear(Float.X(v))
	}
}

func (f *ffPlayer) Status() playerStatus {
	switch f.p().Status() {
	case ffplayer.Ready:
		return statusReady
	case ffplayer.Failed:
		return statusFailed
	}
	return statusLoading
}

func (f *ffPlayer) Position() float64 { return f.p().Position() }
func (f *ffPlayer) Duration() float64 { return f.p().Duration() }
func (f *ffPlayer) Playing() bool     { return f.p().Playing() }
func (f *ffPlayer) TakeEnded() bool   { return f.p().TakeEnded() }

// Err is why the player failed (for the log).
func (f *ffPlayer) Err() error { return f.p().Err() }

func (f *ffPlayer) Tracks() (hasVideo, hasAudio bool, w, h int) {
	i := f.p().Info()
	return i.HasVideo, i.HasAudio, i.Width, i.Height
}

// Tick advances the player's clock, picks the due frame and feeds the
// sound (pumpSources, once per frame, before TakeEnded and CopyFrame).
func (f *ffPlayer) Tick() {
	f.p().Update()
	f.feedAudio()
}

// CopyFrame copies the frame Tick picked into dst (see nativePlayer).
func (f *ffPlayer) CopyFrame(dst []byte) (w, h int, result frameResult) {
	if !f.haveFrame {
		fr, ok := f.p().Frame()
		if !ok {
			return 0, 0, frameNone
		}
		f.frame, f.haveFrame = fr, true
	}
	w, h = f.frame.Width, f.frame.Height
	if len(dst) < w*h*4 {
		return w, h, frameTooLarge // kept for the retry in the same pump
	}
	copy(dst, f.frame.Pix)
	f.haveFrame = false
	return w, h, frameCopied
}

// feedAudio keeps the generator filled while playing and tells the player
// how much of what it handed out is still queued (its audio clock).
func (f *ffPlayer) feedAudio() {
	if !f.p().Info().HasAudio {
		return
	}
	if !f.hasNode {
		root, ok := sceneRoot()
		if !ok || f.p().Status() != ffplayer.Ready {
			return
		}
		gen := AudioStreamGenerator.New()
		gen.SetMixRate(Float.X(f.p().SampleRate()))
		gen.SetBufferLength(audioBufferSecs)
		f.audio = AudioStreamPlayer.New()
		f.audio.SetStream(gen.AsAudioStream())
		f.audio.SetVolumeLinear(Float.X(f.volume))
		root.AddChild(f.audio.AsNode())
		f.hasNode = true
	}
	playing := f.p().Playing()
	epoch := f.p().AudioEpoch()
	if f.started && epoch != f.epoch {
		// A seek or a restart: drop what the generator still holds. Its
		// clear_buffer refuses while the playback is active, so stop it; the
		// next Play makes a new, empty one.
		f.audio.Stop()
		f.started = false
	}
	if !f.started {
		if !playing || !f.audio.AsNode().IsInsideTree() {
			return
		}
		f.audio.Play() // one playback per epoch; pauses are stream_paused
		f.started, f.paused, f.epoch, f.capacity = true, false, epoch, 0
	}
	if f.paused == playing {
		f.paused = !playing
		f.audio.SetStreamPaused(f.paused)
	}
	if !f.audio.HasStreamPlayback() {
		return
	}
	pb, ok := Object.As[AudioStreamGeneratorPlayback.Instance](f.audio.GetStreamPlayback())
	if !ok {
		return
	}
	avail := pb.GetFramesAvailable()
	f.capacity = max(f.capacity, avail)
	if playing && avail > 0 {
		if len(f.pcm) < 2*avail {
			f.pcm = make([]float32, 2*avail)
			f.xy = make([]Vector2.XY, avail)
		}
		if n := f.p().ReadAudio(f.pcm[:2*avail]); n > 0 {
			for i := range n {
				f.xy[i] = Vector2.XY{X: Float.X(f.pcm[2*i]), Y: Float.X(f.pcm[2*i+1])}
			}
			pb.PushBuffer(f.xy[:n])
		}
		avail = pb.GetFramesAvailable()
	}
	f.p().SetAudioBacklog(epoch, f.capacity-avail)
}

func (f *ffPlayer) Close() {
	f.p().Close()
	if f.hasNode {
		f.audio.Stop()
		f.audio.AsNode().QueueFree()
		f.hasNode = false
	}
}
