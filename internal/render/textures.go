package render

import (
	"os"

	"freeworship/internal/models"
	"freeworship/internal/render/layout"

	"graphics.gd/classdb/Gradient"
	"graphics.gd/classdb/GradientTexture2D"
	"graphics.gd/classdb/Image"
	"graphics.gd/classdb/ImageTexture"
	"graphics.gd/classdb/TextParagraph"
	"graphics.gd/classdb/Texture2D"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// Background textures are shared by every SlideView (main thread only).

func toColor(c models.RGBA) Color.RGBA {
	c = layout.ClampColor(c)
	return Color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}

var black = Color.RGBA{A: 1}

type gradientKey struct{ top, bottom models.RGBA }

var gradientCache = map[gradientKey]Texture2D.Instance{}

// gradientTexture returns a vertical top→bottom gradient texture. It is tall
// enough that linear filtering gives a smooth ramp at 4K.
func gradientTexture(top, bottom models.RGBA) Texture2D.Instance {
	k := gradientKey{layout.ClampColor(top), layout.ClampColor(bottom)}
	if t, ok := gradientCache[k]; ok {
		return t
	}
	g := Gradient.New()
	g.SetOffsets([]float32{0, 1})
	g.SetColors([]Color.RGBA{toColor(k.top), toColor(k.bottom)})
	gt := pin(GradientTexture2D.New())
	gt.SetGradient(g)
	gt.SetWidth(4)
	gt.SetHeight(512)
	gt.SetFill(GradientTexture2D.FillLinear)
	gt.SetFillFrom(Vector2.New(0, 0))
	gt.SetFillTo(Vector2.New(0, 1))
	t := gt.AsTexture2D()
	gradientCache[k] = t
	return t
}

const imageCacheMax = 64

var imageCache = map[string]Texture2D.Instance{}

// imageTexture loads (and caches) an image file as a mipmapped texture. It
// returns false when the file is missing or unreadable so callers can fall
// back to the background color. Failures are not cached: the file may appear
// later.
func imageTexture(path string) (Texture2D.Instance, bool) {
	if path == "" {
		return Texture2D.Nil, false
	}
	if t, ok := imageCache[path]; ok {
		return t, true
	}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return Texture2D.Nil, false
	}
	img := Image.LoadFromFile(path)
	if img == Image.Nil || img.IsEmpty() {
		return Texture2D.Nil, false
	}
	// Mipmaps keep thumbnails of large photos from aliasing.
	_ = img.GenerateMipmaps()
	t := pin(ImageTexture.CreateFromImage(img)).AsTexture2D()
	if len(imageCache) >= imageCacheMax {
		freeAll(imageCache)
	}
	imageCache[path] = t
	return t, true
}

// pin keeps a Go-held engine resource alive across frames. graphics.gd
// releases Go-owned references at the end of every frame unless they are
// reachable from a live extension instance; package-level caches are not, so
// they take an extra engine reference that unpin drops again.
func pin[T Object.Any](obj T) T { return Object.Leak(obj) }

// unpin drops the reference taken by pin. Object.Free is a no-op on a leaked
// reference in this graphics.gd version, so call RefCounted.unreference —
// through Object.call, because Object.Call panics when given no arguments.
func unpin(obj Object.Any) {
	Object.Call(obj, "call", "unreference")
}

func freeAll[K comparable](m map[K]Texture2D.Instance) {
	for k, t := range m {
		unpin(t)
		delete(m, k)
	}
}

// ReleaseCaches drops every cached font, texture and measurement object. Call
// it at shutdown (e.g. on NOTIFICATION_WM_CLOSE_REQUEST, before quitting) so
// Godot does not report them as leaked. SlideViews still showing a cached
// texture keep working (the engine holds its own reference); later use simply
// re-creates the caches.
func ReleaseCaches() {
	freeAll(gradientCache)
	freeAll(imageCache)
	for k, f := range fontCache {
		unpin(f)
		delete(fontCache, k)
	}
	if paragraph != TextParagraph.Nil {
		unpin(paragraph)
		paragraph = TextParagraph.Nil
	}
	clear(fitCache)
}
