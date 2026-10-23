//go:build darwin

package media

import (
	"context"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"freebeams/internal/models"
)

// TestAVFoundationPoster makes a clip with ffmpeg (skipped without it), then
// probes it and takes its poster frame with no ffmpeg at all: the processor
// gets AVFoundation chained to a zero Tools.
func TestAVFoundationPoster(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg (to make the test clip) is not on PATH")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "clip.mp4")
	gen := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "color=c=0x2a7f62:s=320x240:r=30:d=2",
		"-vf", "scale=out_color_matrix=bt709:out_range=tv,format=yuv420p", "-colorspace", "bt709", "-color_primaries", "bt709",
		"-color_trc", "bt709", "-color_range", "tv", "-c:v", "libx264", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("cannot make the test clip: %v %s", err, out)
	}
	t.Setenv("PATH", "") // nothing below may reach ffmpeg
	ctx := context.Background()
	p := &Processor{Prober: Chain{First: AVFoundation{}, Then: Tools{}}, Thumbnailer: Chain{First: AVFoundation{}, Then: Tools{}},
		CacheDir: filepath.Join(dir, "cache"), ThumbWidth: 160}
	m := models.Media{Path: src, Kind: models.MediaVideo}
	if changed, err := p.Describe(ctx, &m); err != nil || !changed || m.Width != 320 || m.Height != 240 ||
		m.Duration < 1900*time.Millisecond || m.Duration > 2100*time.Millisecond {
		t.Fatalf("Describe = %v, %v; %+v", changed, err, m)
	}
	if changed, err := p.Thumbnail(ctx, &m); err != nil || !changed {
		t.Fatalf("Thumbnail = %v, %v", changed, err)
	}
	f, err := os.Open(m.ThumbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 160 || b.Dy() != 120 {
		t.Fatalf("poster is %v, want 160x120", b)
	}
	r, g, b, _ := img.At(80, 60).RGBA()
	if d := func(a uint32, want int) int { x := int(a>>8) - want; return max(x, -x) }; d(r, 0x2a) > 8 || d(g, 0x7f) > 8 || d(b, 0x62) > 8 {
		t.Fatalf("poster colour %02x%02x%02x, want ~2a7f62", r>>8, g>>8, b>>8)
	}
	// Audio: duration only; a still is not AVFoundation's business.
	wav := filepath.Join(dir, "tone.wav")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "sine=duration=1", wav).CombinedOutput(); err != nil {
		t.Fatalf("tone: %v %s", err, out)
	}
	if info, err := (AVFoundation{}).Probe(ctx, wav); err != nil || info.Duration < 900*time.Millisecond || info.Width != 0 {
		t.Fatalf("Probe(wav) = %+v, %v", info, err)
	}
	if err := (AVFoundation{}).Thumbnail(ctx, filepath.Join(dir, "x.png"), filepath.Join(dir, "x.jpg"), 64); err != ErrUnavailable {
		t.Fatalf("Thumbnail(png) = %v, want ErrUnavailable", err)
	}
	if _, err := (AVFoundation{}).Probe(ctx, filepath.Join(dir, "missing.mp4")); err == nil {
		t.Fatal("Probe of a missing file: no error")
	}
}
