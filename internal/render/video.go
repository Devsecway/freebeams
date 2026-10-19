package render

import (
	"os"

	"freebeams/internal/models"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/VideoStream"
	"graphics.gd/classdb/VideoStreamPlayer"
	"graphics.gd/classdb/VideoStreamTheora"
)

// Video backgrounds (models.BackgroundVideo) play through a VideoStreamPlayer
// in each layer, between the background texture and the text: a looping,
// muted Ogg Theora motion background. Godot decodes Theora only, so a video
// whose Path is not a playable .ogv file shows its Poster still (the
// thumbnail) instead, and without a poster the background color.
//
// While a video plays, low-processor mode is held off (AcquireAnimating) so
// the output renders at full frame rate; it is released when the video stops:
// the layer changed to another background, the layer was hidden (the outgoing
// side of a crossfade) or the view left the tree.

// mutedDb is the player volume for motion backgrounds (effectively silent).
const mutedDb = -80

// videoLayer is the video part of a layer. It is plain Go around an engine
// node (no extension class): the player's signals keep playback in step with
// visibility and the node's life.
type videoLayer struct {
	player  VideoStreamPlayer.Instance
	path    string // file loaded into the player ("" when none)
	playing bool   // holds one AcquireAnimating
}

// newVideoLayer adds a hidden, idle player under parent.
func newVideoLayer(parent Node.Instance) *videoLayer {
	v := &videoLayer{}
	v.player = VideoStreamPlayer.New()
	c := v.player.AsControl()
	ignoreMouse(c)
	c.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	v.player.SetExpand(true) // fill the letterboxed layer (backgrounds are 16:9)
	v.player.SetLoop(true)
	v.player.SetAutoplay(false)
	v.player.SetVolumeDb(mutedDb)
	c.AsCanvasItem().SetVisible(false)
	// Hidden (the layer went behind after a crossfade, or the view is off
	// screen): stop decoding and give low-processor mode back. Shown again
	// with a video loaded: play.
	c.AsCanvasItem().OnVisibilityChanged(func() {
		if c.AsCanvasItem().IsVisibleInTree() {
			v.play()
		} else {
			v.stop()
		}
	})
	c.AsNode().OnTreeExiting(v.stop)
	parent.AddChild(c.AsNode())
	return v
}

// playable reports whether the renderer can play path: an existing Ogg
// Theora file.
func playable(path string) bool {
	if !models.IsPlayableVideo(path) {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Size() > 0
}

// show loads path (when it changed) and makes the player visible, which
// starts playback once it is visible in the tree. It reports false, and hides
// the player, when path cannot be played.
func (v *videoLayer) show(path string) bool {
	if !playable(path) {
		v.hide()
		return false
	}
	if v.path != path {
		v.stop()
		stream := VideoStreamTheora.New()
		stream.AsVideoStream().SetFile(path)
		v.player.SetStream(stream.AsVideoStream())
		v.path = path
	}
	ci := v.player.AsCanvasItem()
	if !ci.Visible() {
		ci.SetVisible(true) // visibility_changed starts playback when in tree
	} else if ci.IsVisibleInTree() {
		v.play()
	}
	return true
}

// hide stops playback and hides the player (the layer shows something else).
func (v *videoLayer) hide() {
	v.stop()
	ci := v.player.AsCanvasItem()
	if ci.Visible() {
		ci.SetVisible(false)
	}
	if v.path != "" {
		v.player.SetStream(VideoStream.Nil)
		v.path = ""
	}
}

// play (re)starts the loaded video and holds low-processor mode off.
func (v *videoLayer) play() {
	if v.path == "" || !v.player.AsCanvasItem().IsVisibleInTree() {
		return
	}
	if !v.playing {
		v.playing = true
		AcquireAnimating()
	}
	if !v.player.IsPlaying() {
		v.player.Play()
	}
}

// stop halts playback and releases low-processor mode. Safe to call repeatedly.
func (v *videoLayer) stop() {
	if v.playing {
		v.playing = false
		ReleaseAnimating()
	}
	if v.path != "" && v.player.IsPlaying() {
		v.player.Stop()
	}
}

// Playing reports whether the layer's video is running.
func (v *videoLayer) Playing() bool { return v.playing }

// applyVideo projects a video or audio background onto the layer: the poster
// still on the texture rect (shown under the video while its first frame
// decodes, and alone when the video cannot play; audio has none, so its
// color shows) and the video on top. On a platform with a native player
// (macOS) a layer with a role plays the file through it (videosource.go);
// elsewhere Godot's Theora player takes a playable .ogv.
func (l *layer) applyVideo(bg models.Background) {
	if t, ok := imageTexture(bg.Poster); ok {
		l.tex.SetTexture(t)
		l.tex.SetStretchMode(stretchFor(bg.Fit)) // the item's Aspect Ratio, like the frames
		l.tex.AsCanvasItem().SetVisible(true)
	} else {
		l.tex.AsCanvasItem().SetVisible(false)
	}
	if nativeVideoSupported {
		l.video.hide()
		if l.native.role == RoleNone {
			l.native.hide()
		} else {
			l.native.rect.SetStretchMode(stretchFor(bg.Fit))
			l.native.set(bg.Path)
		}
		return
	}
	l.native.hide()
	if bg.Kind == models.BackgroundVideo {
		l.video.show(bg.Path)
	} else {
		l.video.hide()
	}
}

// VideoPlaying reports whether the view's current layer is playing a video
// or audio file.
func (v *SlideView) VideoPlaying() bool {
	if !v.built || v.cur == nil {
		return false
	}
	return (v.cur.video != nil && v.cur.video.Playing()) || (v.cur.native != nil && v.cur.native.playing())
}
