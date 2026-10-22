package render

import (
	"log"

	"graphics.gd/classdb/CanvasItem"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/Image"
	"graphics.gd/classdb/ImageTexture"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/TextureRect"
)

// Media playback through a player behind nativePlayer (player.go: AVFoundation
// on macOS, avplayer_darwin.go; ffmpeg processes elsewhere, ffplayer.go).
//
// A SlideView that shows a video or audio background opens the file through
// a shared MediaSource keyed by (role, path). The live monitor and the
// projector output both have RoleLive, so one decoder feeds both views and
// the sound plays once; the preview monitor has RolePreview (its own
// decoder, paused); thumbnails have RoleNone and show the poster only.
//
// A source owns the native player, one RGBA8 Image and the ImageTexture the
// views draw. Every frame (pumpSources, driven from the Process of a view
// with a role) it pulls the newest decoded frame into the texture, reads the
// player's status and holds low-processor mode off while playing.
//
// What the operator wants (play, pause, seek, loop, volume) is app state;
// the app projects it onto Current(role) in its render pass. The source
// itself keeps only what the player must be told once it is ready.

// VideoRole says which monitor a SlideView is, and so which shared player
// feeds it.
type VideoRole int

const (
	RoleNone    VideoRole = iota // thumbnails and previews: poster still only
	RoleLive                     // the live monitor and the projector output (one decoder)
	RolePreview                  // the preview monitor (its own decoder, paused by default)
)

// playerStatus is where the native player is with its file.
type playerStatus int

const (
	statusLoading playerStatus = iota
	statusReady
	statusFailed
)

// frameResult is CopyFrame's outcome.
type frameResult int

const (
	frameNone     frameResult = iota // nothing new
	frameCopied                      // dst holds the frame
	frameTooLarge                    // dst is too small for w*h*4 bytes; call again with a larger one
)

// nativePlayer is one platform media player playing one file. All methods
// run on the main thread.
type nativePlayer interface {
	Play()
	Pause()
	Seek(secs float64)
	SetVolume(v float64)
	SetLoop(on bool)
	Status() playerStatus
	Position() float64
	Duration() float64
	Playing() bool
	TakeEnded() bool // a non-looping end since the last call
	Tracks() (hasVideo, hasAudio bool, w, h int)
	CopyFrame(dst []byte) (w, h int, result frameResult)
	Close()
}

// ticker is a nativePlayer that needs a call every frame once ready (the
// ffmpeg player: its clock, frame choice and audio feed), before TakeEnded
// and CopyFrame.
type ticker interface{ Tick() }

// Playback is what a source's player reports (read-only, every frame).
type Playback struct {
	Path     string
	Ready    bool
	Failed   bool
	Playing  bool // the player's rate is non-zero
	Position float64
	Duration float64
	HasVideo bool
	HasAudio bool
	Width    int
	Height   int
}

// MediaSource is one shared native player with its texture.
type MediaSource struct {
	role   VideoRole
	path   string
	player nativePlayer
	refs   int

	status     playerStatus
	hasVideo   bool
	hasAudio   bool
	natW, natH int
	ended      bool // the player stopped at the end (loop off); cleared by Play / Seek

	img      Image.Instance
	tex      ImageTexture.Instance
	buf      []byte
	w, h     int
	hasFrame bool

	playing   bool // Play was asked (the player may still be loading)
	loop      bool
	volume    float64
	seek      float64
	seekAsked bool
	animating bool

	layers []*nativeLayer
}

type sourceKey struct {
	role VideoRole
	path string
}

var (
	sources     = map[sourceKey]*MediaSource{}
	current     = map[VideoRole]*MediaSource{}
	pumpedFrame = -1
)

// Current returns the source most recently opened for a role: what the
// monitor shows now (an outgoing crossfade layer may still hold the previous
// one). nil when the role shows no video or audio.
func Current(role VideoRole) *MediaSource { return current[role] }

// acquireSource opens (or shares) the player for (role, path). host is a
// node in the scene tree (see openNativePlayer).
func acquireSource(role VideoRole, path string, host Node.Instance) *MediaSource {
	k := sourceKey{role, path}
	if s, ok := sources[k]; ok {
		s.refs++
		current[role] = s
		return s
	}
	s := &MediaSource{role: role, path: path, refs: 1, volume: 1}
	p, err := openNativePlayer(path, host)
	if err != nil {
		log.Printf("[render] open %s: %v", path, err)
		s.status = statusFailed
	} else {
		s.player = p
		p.SetVolume(1)
	}
	sources[k] = s
	current[role] = s
	return s
}

// release drops one reference; the last one closes the player.
func (s *MediaSource) release() {
	s.refs--
	if s.refs > 0 {
		return
	}
	s.close()
}

func (s *MediaSource) close() {
	delete(sources, sourceKey{s.role, s.path})
	if current[s.role] == s {
		delete(current, s.role)
	}
	for _, l := range s.layers {
		l.src = nil
	}
	s.layers = nil
	s.setAnimating(false)
	if s.player != nil {
		s.player.Close()
		s.player = nil
	}
	if s.tex != ImageTexture.Nil {
		unpin(s.tex)
		s.tex = ImageTexture.Nil
	}
	if s.img != Image.Nil {
		unpin(s.img)
		s.img = Image.Nil
	}
	s.buf = nil
	s.refs = 0
}

// CloseSources closes every open player (shutdown).
func CloseSources() {
	for _, s := range sources {
		s.close()
	}
}

// Path is the file the source plays.
func (s *MediaSource) Path() string { return s.path }

// Role is the monitor the source belongs to.
func (s *MediaSource) Role() VideoRole { return s.role }

// Play starts (or resumes) playback. Before the player is ready it is
// remembered and applied then.
func (s *MediaSource) Play() {
	s.playing = true
	s.ended = false
	if s.player != nil && s.status == statusReady {
		s.player.Play()
	}
	s.updateAnimating()
}

// Pause stops playback, keeping the position.
func (s *MediaSource) Pause() {
	s.playing = false
	if s.player != nil && s.status == statusReady {
		s.player.Pause()
	}
	s.updateAnimating()
}

// Seek moves to secs (clamped by the player); before the player is ready it
// is applied once it is.
func (s *MediaSource) Seek(secs float64) {
	if secs < 0 {
		secs = 0
	}
	s.ended = false
	if s.player != nil && s.status == statusReady {
		s.player.Seek(secs)
		s.seekAsked = false
		return
	}
	s.seek, s.seekAsked = secs, true
}

// SetLoop makes the file start again at its end.
func (s *MediaSource) SetLoop(on bool) {
	s.loop = on
	if s.player != nil {
		s.player.SetLoop(on)
	}
}

// SetVolume sets the player volume (0..1).
func (s *MediaSource) SetVolume(v float64) {
	v = min(max(v, 0), 1)
	s.volume = v
	if s.player != nil {
		s.player.SetVolume(v)
	}
}

// Playing reports whether playback was asked for and has not ended.
func (s *MediaSource) Playing() bool { return s.playing && !s.ended }

// Loop reports the looping flag applied to the player.
func (s *MediaSource) Loop() bool { return s.loop }

// Volume reports the volume applied to the player.
func (s *MediaSource) Volume() float64 { return s.volume }

// TakeEnded reports (once) that the file reached its end with looping off:
// the player paused itself. The app turns this into a state change.
func (s *MediaSource) TakeEnded() bool {
	if !s.ended {
		return false
	}
	s.ended = false
	s.playing = false
	s.updateAnimating()
	return true
}

// State is what the player reports now.
func (s *MediaSource) State() Playback {
	p := Playback{Path: s.path, Ready: s.status == statusReady, Failed: s.status == statusFailed,
		HasVideo: s.hasVideo, HasAudio: s.hasAudio, Width: s.natW, Height: s.natH}
	if s.player != nil && s.status == statusReady {
		p.Playing = s.player.Playing()
		p.Position = s.player.Position()
		p.Duration = s.player.Duration()
	}
	return p
}

// HasFrame reports whether at least one video frame reached the texture.
func (s *MediaSource) HasFrame() bool { return s.hasFrame }

func (s *MediaSource) updateAnimating() {
	s.setAnimating(s.playing && !s.ended && s.status == statusReady && s.player != nil)
}

func (s *MediaSource) setAnimating(on bool) {
	if on == s.animating {
		return
	}
	s.animating = on
	if on {
		AcquireAnimating()
	} else {
		ReleaseAnimating()
	}
}

// pump runs once per frame: status, tracks, pending commands, a new frame.
func (s *MediaSource) pump() {
	if s.player == nil {
		return
	}
	if s.status != statusReady {
		switch s.player.Status() {
		case statusReady:
			s.status = statusReady
			s.hasVideo, s.hasAudio, s.natW, s.natH = s.player.Tracks()
			s.player.SetLoop(s.loop)
			s.player.SetVolume(s.volume)
			if s.seekAsked {
				s.player.Seek(s.seek)
				s.seekAsked = false
			}
			if s.playing {
				s.player.Play()
			}
			s.updateAnimating()
		case statusFailed:
			s.status = statusFailed
			why := ""
			if e, ok := s.player.(interface{ Err() error }); ok && e.Err() != nil {
				why = " (" + e.Err().Error() + ")"
			}
			log.Printf("[render] %s: the media player could not play the file%s", s.path, why)
			s.player.Close()
			s.player = nil
			s.updateAnimating()
			return
		default:
			return
		}
	}
	if t, ok := s.player.(ticker); ok {
		t.Tick()
	}
	if s.player.TakeEnded() {
		s.ended = true
		s.updateAnimating()
	}
	if s.hasVideo {
		s.pullFrame()
	}
}

// pullFrame copies the newest decoded frame into the texture.
func (s *MediaSource) pullFrame() {
	for range 2 { // once, or twice when the buffer must grow
		w, h, res := s.player.CopyFrame(s.buf)
		switch res {
		case frameNone:
			return
		case frameTooLarge:
			s.buf = make([]byte, w*h*4)
			continue
		}
		s.applyFrame(w, h)
		return
	}
}

func (s *MediaSource) applyFrame(w, h int) {
	data := s.buf[:w*h*4]
	switch {
	case s.img == Image.Nil:
		s.img = pin(Image.CreateFromData(w, h, false, Image.FormatRgba8, data))
		s.tex = pin(ImageTexture.CreateFromImage(s.img))
	case w != s.w || h != s.h:
		s.img.SetData(w, h, false, Image.FormatRgba8, data)
		s.tex.SetImage(s.img)
	default:
		s.img.SetData(w, h, false, Image.FormatRgba8, data)
		s.tex.Update(s.img)
	}
	s.w, s.h = w, h
	if !s.hasFrame {
		s.hasFrame = true
		for _, l := range s.layers {
			l.showTexture(s.tex)
		}
	}
}

// pumpSources advances every source once per engine frame. Views with a role
// call it from Process.
func pumpSources() {
	frame := Engine.GetProcessFrames()
	if frame == pumpedFrame {
		return
	}
	pumpedFrame = frame
	for _, s := range sources {
		s.pump()
	}
}

// AnyMediaActive reports whether any source is open (the app re-projects
// the transport every frame while one is).
func AnyMediaActive() bool { return len(sources) > 0 }

// ── Layer side ───────────────────────────────────────────────────────────

// nativeLayer is a layer's native video: a TextureRect fed by the source's
// texture, shown over the poster once the first frame arrived.
type nativeLayer struct {
	rect    TextureRect.Instance
	role    VideoRole
	path    string // the file this layer shows ("" none)
	visible bool   // the layer root is visible in the tree
	src     *MediaSource
}

func newNativeLayer(parent Node.Instance) *nativeLayer {
	n := &nativeLayer{}
	n.rect = TextureRect.New()
	ignoreMouse(n.rect.AsControl())
	n.rect.SetExpandMode(TextureRect.ExpandIgnoreSize)
	n.rect.SetStretchMode(TextureRect.StretchKeepAspectCovered)
	n.rect.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	n.rect.AsCanvasItem().SetTextureFilter(CanvasItem.TextureFilterLinear)
	n.rect.AsCanvasItem().SetVisible(false)
	parent.AddChild(n.rect.AsNode())
	return n
}

// set shows path (a video or audio file) through the layer's role.
func (n *nativeLayer) set(path string) {
	if n.path != path {
		n.detach()
		n.path = path
		n.rect.AsCanvasItem().SetVisible(false)
	}
	n.sync()
}

// hide drops the file. The last frame stays on the (hidden) texture rect.
func (n *nativeLayer) hide() {
	n.detach()
	n.path = ""
	n.rect.AsCanvasItem().SetVisible(false)
}

// setVisible records the layer root's visibility in the tree: a hidden
// layer (the outgoing side of a finished crossfade, a view off screen)
// releases its player and keeps its last frame.
func (n *nativeLayer) setVisible(v bool) {
	n.visible = v
	n.sync()
}

// setRole re-opens the file under another role.
func (n *nativeLayer) setRole(role VideoRole) {
	if role == n.role {
		return
	}
	n.detach()
	n.role = role
	n.sync()
}

// sync acquires or releases the source to match: a path, a role and a
// visible layer.
func (n *nativeLayer) sync() {
	want := n.path != "" && n.role != RoleNone && n.visible
	switch {
	case want && n.src == nil:
		n.src = acquireSource(n.role, n.path, n.rect.AsNode())
		n.src.layers = append(n.src.layers, n)
		if n.src.hasFrame {
			n.showTexture(n.src.tex)
		}
	case !want && n.src != nil:
		n.detach()
	}
}

func (n *nativeLayer) detach() {
	s := n.src
	if s == nil {
		return
	}
	n.src = nil
	for i, l := range s.layers {
		if l == n {
			s.layers = append(s.layers[:i], s.layers[i+1:]...)
			break
		}
	}
	s.release()
}

func (n *nativeLayer) showTexture(t ImageTexture.Instance) {
	n.rect.SetTexture(t.AsTexture2D())
	n.rect.AsCanvasItem().SetVisible(true)
}

// playing reports whether this layer's source is playing.
func (n *nativeLayer) playing() bool { return n.src != nil && n.src.Playing() }
