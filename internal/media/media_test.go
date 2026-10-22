package media

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"freebeams/internal/models"
)

// writePNG writes a w×h PNG whose left half is left and right half is right.
func writePNG(t *testing.T, path string, w, h int, left, right color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := left
			if x >= w/2 {
				c = right
			}
			img.SetRGBA(x, y, c)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func paths(items []models.Media) []string {
	out := make([]string, len(items))
	for i, m := range items {
		out[i] = filepath.Base(m.Path)
	}
	return out
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"b.MP4", "a.png", "notes.txt", ".hidden.png", "sub/e.jpg", ".git/x.png", "sub/deep/f.webm", "sub/song.mp3"} {
		touch(t, filepath.Join(dir, f))
	}
	items, warnings := Scan(dir)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %q", warnings)
	}
	// Ordered by full path: sub/deep/f.webm sorts before sub/e.jpg.
	want := []string{"a.png", "b.MP4", "f.webm", "e.jpg", "song.mp3"}
	if got := paths(items); !slices.Equal(got, want) {
		t.Fatalf("Scan = %v, want %v", got, want)
	}
	kinds := map[string]models.MediaKind{"a.png": models.MediaImage, "b.MP4": models.MediaVideo, "e.jpg": models.MediaImage,
		"f.webm": models.MediaVideo, "song.mp3": models.MediaAudio}
	for _, m := range items {
		if m.Kind != kinds[filepath.Base(m.Path)] || m.Title == "" || !filepath.IsAbs(m.Path) || m.ID != 0 {
			t.Errorf("item = %+v", m)
		}
	}

	// A single file, a duplicate folder and a missing folder.
	items, warnings = Scan(filepath.Join(dir, "a.png"), dir, filepath.Join(dir, "nope"), filepath.Join(dir, "notes.txt"))
	if got := paths(items); !slices.Equal(got, want) {
		t.Fatalf("Scan with duplicates = %v, want %v", got, want)
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0], "nope") || !strings.Contains(warnings[1], "not an image, video or audio") {
		t.Fatalf("warnings = %q", warnings)
	}
	if items, warnings := Scan(); items != nil || warnings != nil {
		t.Fatalf("Scan() = %v, %v", items, warnings)
	}
}

func TestCachePaths(t *testing.T) {
	cache := filepath.FromSlash("/cache") // filepath.Join gives \cache on Windows
	a := ThumbPath(cache, "/media/My Clip #1.mp4")
	if filepath.Dir(a) != cache || filepath.Ext(a) != ".jpg" || !strings.Contains(a, "My_Clip_1") {
		t.Fatalf("ThumbPath = %q", a)
	}
	if again := ThumbPath(cache, "/media/My Clip #1.mp4"); again != a {
		t.Fatalf("ThumbPath is not stable: %q vs %q", a, again)
	}
	if b := ThumbPath(cache, "/media/My Clip #2.mp4"); b == a {
		t.Fatalf("different sources share a thumbnail: %q", a)
	}
	if c := ThumbPath(cache, "/other/My Clip #1.mp4"); c == a {
		t.Fatalf("same name in another folder shares a thumbnail: %q", a)
	}
	v := TranscodePath(cache, "/media/My Clip #1.mp4")
	if filepath.Ext(v) != ".ogv" || strings.TrimSuffix(v, ".ogv") != strings.TrimSuffix(a, ".jpg") {
		t.Fatalf("TranscodePath = %q (thumb %q)", v, a)
	}
	long := ThumbPath(cache, "/x/"+strings.Repeat("n", 200)+".png")
	if len(filepath.Base(long)) > 80 {
		t.Fatalf("long name not truncated: %d chars", len(filepath.Base(long)))
	}
	if got := sanitize("Ünïcode name/with:odd*chars.v2"); got != "ncode_namewithoddchars.v2" {
		t.Fatalf("sanitize = %q", got)
	}
	if p := partFile("/c/x.ogv", ".ogv"); p != "/c/x.part.ogv" {
		t.Fatalf("partFile = %q", p)
	}
}

func TestDefaultCacheDir(t *testing.T) {
	override := filepath.Join(t.TempDir(), "sub", "cache")
	dir, err := DefaultCacheDir(override)
	if err != nil || dir != override {
		t.Fatalf("DefaultCacheDir(override) = %q, %v", dir, err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("cache dir not created: %v", err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "xdg"))
	t.Setenv("LocalAppData", filepath.Join(t.TempDir(), "lad"))
	dir, err = DefaultCacheDir("")
	if err != nil || filepath.Base(dir) != "media" || filepath.Base(filepath.Dir(dir)) != "FreeBeams" {
		t.Fatalf("DefaultCacheDir() = %q, %v", dir, err)
	}
}

func TestImageThumbnail(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.png")
	writePNG(t, src, 64, 36, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255})
	w, h, err := imageInfo(src)
	if err != nil || w != 64 || h != 36 {
		t.Fatalf("imageInfo = %d×%d, %v", w, h, err)
	}
	tests := []struct {
		name  string
		width int
		wantW int
		wantH int
	}{
		{"downscale", 32, 32, 18},
		{"odd width rounds the height", 30, 30, 17},
		{"never upscale", 500, 64, 36},
		{"zero means source size", 0, 64, 36},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dst := filepath.Join(dir, "out", tc.name+".jpg")
			if err := imageThumbnail(src, dst, tc.width); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(dst)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			img, err := jpeg.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			if b := img.Bounds(); b.Dx() != tc.wantW || b.Dy() != tc.wantH {
				t.Fatalf("thumbnail is %d×%d, want %d×%d", b.Dx(), b.Dy(), tc.wantW, tc.wantH)
			}
			// The colors survive (left red, right blue), within JPEG tolerance.
			r, _, b1, _ := img.At(2, img.Bounds().Dy()/2).RGBA()
			r2, _, b2, _ := img.At(img.Bounds().Dx()-3, img.Bounds().Dy()/2).RGBA()
			if r>>8 < 200 || b1>>8 > 60 || b2>>8 < 200 || r2>>8 > 60 {
				t.Fatalf("colors lost: left (%d,%d) right (%d,%d)", r>>8, b1>>8, r2>>8, b2>>8)
			}
		})
	}
	if err := imageThumbnail(filepath.Join(dir, "missing.png"), filepath.Join(dir, "x.jpg"), 32); err == nil {
		t.Fatal("missing source: no error")
	}
	bad := filepath.Join(dir, "bad.png")
	os.WriteFile(bad, []byte("not a png"), 0o644)
	if err := imageThumbnail(bad, filepath.Join(dir, "y.jpg"), 32); err == nil {
		t.Fatal("corrupt source: no error")
	}
	if entries, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(entries) != 0 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

// fakeTools records calls and writes placeholder outputs.
type fakeTools struct {
	probes, thumbs, transcodes []string
	info                       Info
	fail                       error
}

func (f *fakeTools) Probe(_ context.Context, path string) (Info, error) {
	f.probes = append(f.probes, path)
	if f.fail != nil {
		return Info{}, f.fail
	}
	return f.info, nil
}

func (f *fakeTools) Thumbnail(_ context.Context, src, dst string, width int) error {
	f.thumbs = append(f.thumbs, src)
	if f.fail != nil {
		return f.fail
	}
	return os.WriteFile(dst, []byte("jpeg"), 0o644)
}

func (f *fakeTools) Transcode(_ context.Context, src, dst string, progress func(float64)) error {
	f.transcodes = append(f.transcodes, src)
	if f.fail != nil {
		return f.fail
	}
	if progress != nil {
		progress(0.5)
		progress(1)
	}
	return os.WriteFile(dst, []byte("ogv"), 0o644)
}

func TestProcessorStillsWithoutTools(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	src := filepath.Join(dir, "still.png")
	writePNG(t, src, 48, 27, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 255, 0, 255})
	p := &Processor{CacheDir: filepath.Join(dir, "cache"), ThumbWidth: 24}
	m := models.Media{Path: src, Kind: models.MediaImage}

	if !NeedsDescribe(m) || !NeedsThumbnail(m) || NeedsTranscode(m) {
		t.Fatalf("needs = %v %v %v", NeedsDescribe(m), NeedsThumbnail(m), NeedsTranscode(m))
	}
	changed, err := p.Describe(ctx, &m)
	if err != nil || !changed || m.Width != 48 || m.Height != 27 {
		t.Fatalf("Describe = %v, %v; %+v", changed, err, m)
	}
	if changed, err := p.Describe(ctx, &m); err != nil || changed {
		t.Fatalf("second Describe = %v, %v", changed, err)
	}
	changed, err = p.Thumbnail(ctx, &m)
	if err != nil || !changed || m.ThumbPath != ThumbPath(p.CacheDir, src) || !exists(m.ThumbPath) {
		t.Fatalf("Thumbnail = %v, %v; %+v", changed, err, m)
	}
	if changed, err := p.Thumbnail(ctx, &m); err != nil || changed {
		t.Fatalf("second Thumbnail = %v, %v", changed, err)
	}
	if changed, err := p.Transcode(ctx, &m, nil); err != nil || changed {
		t.Fatalf("Transcode of a still = %v, %v", changed, err)
	}

	// A still Go cannot decode needs the tools.
	webp := models.Media{Path: filepath.Join(dir, "x.webp"), Kind: models.MediaImage}
	touch(t, webp.Path)
	if _, err := p.Describe(ctx, &webp); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Describe webp without tools: %v", err)
	}
	if _, err := p.Thumbnail(ctx, &webp); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Thumbnail webp without tools: %v", err)
	}
	// A video needs the tools too.
	v := models.Media{Path: filepath.Join(dir, "v.mp4"), Kind: models.MediaVideo}
	touch(t, v.Path)
	if _, err := p.Describe(ctx, &v); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Describe video without tools: %v", err)
	}
	if _, err := p.Thumbnail(ctx, &v); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Thumbnail video without tools: %v", err)
	}
	if _, err := p.Transcode(ctx, &v, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Transcode without tools: %v", err)
	}
	// A zero Tools value behaves the same (ErrUnavailable everywhere).
	zero := NewProcessor(Tools{}, p.CacheDir)
	if _, err := zero.Transcode(ctx, &v, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Transcode with zero Tools: %v", err)
	}
	if _, err := zero.Thumbnail(ctx, &v); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Thumbnail with zero Tools: %v", err)
	}
	// Missing files and nil items are errors.
	gone := models.Media{Path: filepath.Join(dir, "gone.png"), Kind: models.MediaImage}
	if _, err := p.Describe(ctx, &gone); err == nil {
		t.Fatal("Describe of a missing file: no error")
	}
	if _, err := p.Thumbnail(ctx, &gone); err == nil {
		t.Fatal("Thumbnail of a missing file: no error")
	}
	if _, err := p.Describe(ctx, nil); err == nil {
		t.Fatal("nil item: no error")
	}
}

func TestProcessorVideoWithFakeTools(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ft := &fakeTools{info: Info{Width: 1280, Height: 720, Duration: 30 * time.Second}}
	p := &Processor{Prober: ft, Thumbnailer: ft, Transcoder: ft, CacheDir: filepath.Join(dir, "cache")}
	v := models.Media{Path: filepath.Join(dir, "clip.mp4"), Kind: models.MediaVideo}
	touch(t, v.Path)

	if changed, err := p.Describe(ctx, &v); err != nil || !changed || v.Width != 1280 || v.Height != 720 || v.Duration != 30*time.Second {
		t.Fatalf("Describe = %v, %v; %+v", changed, err, v)
	}
	if changed, err := p.Thumbnail(ctx, &v); err != nil || !changed || !exists(v.ThumbPath) {
		t.Fatalf("Thumbnail = %v, %v; %+v", changed, err, v)
	}
	if !NeedsTranscode(v) || v.PlayablePath() != "" {
		t.Fatalf("video without a transcode: needs %v, playable %q", NeedsTranscode(v), v.PlayablePath())
	}
	var progress []float64
	if changed, err := p.Transcode(ctx, &v, func(f float64) { progress = append(progress, f) }); err != nil || !changed {
		t.Fatalf("Transcode = %v, %v", changed, err)
	}
	if v.TranscodedPath != TranscodePath(p.CacheDir, v.Path) || !exists(v.TranscodedPath) || v.PlayablePath() != v.TranscodedPath {
		t.Fatalf("after transcode: %+v", v)
	}
	if !slices.Equal(progress, []float64{0.5, 1}) {
		t.Fatalf("progress = %v", progress)
	}
	if NeedsTranscode(v) || NeedsThumbnail(v) || NeedsDescribe(v) {
		t.Fatalf("still needs work: %+v", v)
	}
	// Every step is a no-op now, and the tools are not called again.
	calls := len(ft.probes) + len(ft.thumbs) + len(ft.transcodes)
	for _, step := range []func() (bool, error){
		func() (bool, error) { return p.Describe(ctx, &v) },
		func() (bool, error) { return p.Thumbnail(ctx, &v) },
		func() (bool, error) { return p.Transcode(ctx, &v, nil) },
	} {
		if changed, err := step(); err != nil || changed {
			t.Fatalf("repeated step = %v, %v", changed, err)
		}
	}
	if n := len(ft.probes) + len(ft.thumbs) + len(ft.transcodes); n != calls {
		t.Fatalf("tools called again: %d -> %d", calls, n)
	}

	// An existing cache file is adopted without running the tool.
	w := models.Media{Path: filepath.Join(dir, "other.mp4"), Kind: models.MediaVideo}
	touch(t, w.Path)
	touch(t, TranscodePath(p.CacheDir, w.Path))
	if changed, err := p.Transcode(ctx, &w, nil); err != nil || !changed || len(ft.transcodes) != 1 {
		t.Fatalf("adopting a cached transcode = %v, %v (%d transcodes)", changed, err, len(ft.transcodes))
	}
	// A playable source needs no transcode.
	ogv := models.Media{Path: filepath.Join(dir, "ready.ogv"), Kind: models.MediaVideo}
	touch(t, ogv.Path)
	if NeedsTranscode(ogv) {
		t.Fatal(".ogv source needs a transcode")
	}
	// Tool failures are reported and leave the item unchanged.
	ft.fail = errors.New("boom")
	x := models.Media{Path: filepath.Join(dir, "x.mp4"), Kind: models.MediaVideo}
	touch(t, x.Path)
	if _, err := p.Describe(ctx, &x); err == nil || x.Width != 0 {
		t.Fatalf("failed Describe = %v; %+v", err, x)
	}
	if _, err := p.Thumbnail(ctx, &x); err == nil || x.ThumbPath != "" {
		t.Fatalf("failed Thumbnail = %v; %+v", err, x)
	}
	if _, err := p.Transcode(ctx, &x, nil); err == nil || x.TranscodedPath != "" {
		t.Fatalf("failed Transcode = %v; %+v", err, x)
	}
	// A WebP still goes through the Thumbnailer.
	ft.fail = nil
	webp := models.Media{Path: filepath.Join(dir, "pic.webp"), Kind: models.MediaImage}
	touch(t, webp.Path)
	if changed, err := p.Thumbnail(ctx, &webp); err != nil || !changed || ft.thumbs[len(ft.thumbs)-1] != webp.Path {
		t.Fatalf("webp Thumbnail = %v, %v; thumbs %v", changed, err, ft.thumbs)
	}
}

func TestHasEncoderAndStatus(t *testing.T) {
	out := []byte(`Encoders:
 V..... = Video
 ------
 V....D libx264              libx264 H.264 / AVC / MPEG-4 AVC / MPEG-4 part 10 (codec h264)
 V....D libtheora            libtheora Theora
 A....D vorbis               Vorbis
`)
	if !hasEncoder(out, "libtheora") || !hasEncoder(out, "vorbis") || hasEncoder(out, "theora") || hasEncoder(out, "libvpx") {
		t.Fatal("hasEncoder misread the encoder list")
	}
	tests := []struct {
		t          Tools
		available  bool
		transcodes bool
		status     string
	}{
		{Tools{}, false, false, "ffmpeg not found"},
		{Tools{FFprobe: "/p"}, false, false, "ffmpeg not found (ffprobe only)"},
		{Tools{FFmpeg: "/f", FFprobe: "/p"}, true, false, "ffmpeg without the Theora encoder"},
		{Tools{FFmpeg: "/f"}, false, false, "ffmpeg without the Theora encoder"},
		{Tools{FFmpeg: "/f", Theora: true}, false, true, "ffmpeg ready (ffprobe not found)"},
		{Tools{FFmpeg: "/f", FFprobe: "/p", Theora: true}, true, true, "ffmpeg ready"},
	}
	for _, tc := range tests {
		if tc.t.Available() != tc.available || tc.t.CanTranscode() != tc.transcodes || tc.t.Status() != tc.status {
			t.Errorf("%+v: available %v, transcodes %v, status %q", tc.t, tc.t.Available(), tc.t.CanTranscode(), tc.t.Status())
		}
	}
	ctx := context.Background()
	var none Tools
	if _, err := none.Probe(ctx, "x"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("zero Probe: %v", err)
	}
	if err := none.Thumbnail(ctx, "x", "y", 10); !errors.Is(err, ErrUnavailable) {
		t.Errorf("zero Thumbnail: %v", err)
	}
	if err := none.Transcode(ctx, "x", "y", nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("zero Transcode: %v", err)
	}
	if err := (Tools{FFmpeg: "/f"}).Transcode(ctx, "x", "y", nil); !errors.Is(err, ErrNoTheora) {
		t.Errorf("Transcode without Theora: %v", err)
	}
}

func TestParseProbe(t *testing.T) {
	tests := []struct {
		name  string
		json  string
		audio bool
		want  Info
		err   bool
	}{
		{"video", `{"streams":[{"width":1280,"height":720}],"format":{"duration":"30.037000"}}`, false,
			Info{Width: 1280, Height: 720, Duration: 30037 * time.Millisecond}, false},
		{"image has no duration", `{"streams":[{"width":1920,"height":1080}],"format":{}}`, false, Info{Width: 1920, Height: 1080}, false},
		{"N/A duration", `{"streams":[{"width":10,"height":10}],"format":{"duration":"N/A"}}`, false, Info{Width: 10, Height: 10}, false},
		{"no stream", `{"streams":[],"format":{"duration":"1"}}`, false, Info{Duration: time.Second}, true},
		{"garbage", `nope`, false, Info{}, true},
		{"audio", `{"streams":[{}],"format":{"duration":"3.5"}}`, true, Info{Duration: 3500 * time.Millisecond}, false},
		{"audio without duration", `{"streams":[{}],"format":{}}`, true, Info{}, true},
	}
	for _, tc := range tests {
		got, err := parseProbe([]byte(tc.json), tc.audio)
		if (err != nil) != tc.err || got != tc.want {
			t.Errorf("%s: parseProbe = %+v, %v; want %+v, err %v", tc.name, got, err, tc.want, tc.err)
		}
	}
}

func TestProgressFraction(t *testing.T) {
	total := 10 * time.Second
	tests := []struct {
		line string
		want float64
		ok   bool
	}{
		{"out_time_us=5000000", 0.5, true},
		{"out_time_ms=2500000", 0.25, true},
		{"out_time_us=99000000", 1, true},
		{"out_time_us=-1", 0, false},
		{"frame=12", 0, false},
		{"progress=end", 0, false},
		{"garbage", 0, false},
	}
	for _, tc := range tests {
		got, ok := progressFraction(tc.line, total)
		if got != tc.want || ok != tc.ok {
			t.Errorf("progressFraction(%q) = %v, %v; want %v, %v", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}

// TestRealFFmpeg exercises the real tools when they are installed: a tiny
// synthetic video is generated, probed and given a poster frame, and
// transcoded when the Theora encoder is present.
func TestRealFFmpeg(t *testing.T) {
	ctx := context.Background()
	tools := Detect(ctx)
	if !tools.Available() {
		t.Skipf("ffmpeg/ffprobe not on PATH (%s)", tools.Status())
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "test clip.mp4")
	gen := exec.CommandContext(ctx, tools.FFmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=size=64x36:rate=10",
		"-t", "2", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("cannot generate a test video: %v %s", err, out)
	}
	info, err := tools.Probe(ctx, src)
	if err != nil || info.Width != 64 || info.Height != 36 || info.Duration < 1500*time.Millisecond || info.Duration > 2500*time.Millisecond {
		t.Fatalf("Probe = %+v, %v", info, err)
	}
	if _, err := tools.Probe(ctx, filepath.Join(dir, "missing.mp4")); err == nil {
		t.Fatal("Probe of a missing file: no error")
	}

	p := NewProcessor(tools, filepath.Join(dir, "cache"))
	p.ThumbWidth = 32
	m := models.Media{Path: src, Kind: models.MediaVideo}
	if changed, err := p.Describe(ctx, &m); err != nil || !changed || m.Duration <= 0 {
		t.Fatalf("Describe = %v, %v; %+v", changed, err, m)
	}
	if changed, err := p.Thumbnail(ctx, &m); err != nil || !changed || !exists(m.ThumbPath) {
		t.Fatalf("Thumbnail = %v, %v; %+v", changed, err, m)
	}
	f, err := os.Open(m.ThumbPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != 32 || cfg.Height != 18 {
		t.Fatalf("poster = %+v, %v", cfg, err)
	}
	// A still through ffmpeg (the WebP/BMP path) also works with a PNG.
	still := filepath.Join(dir, "still.png")
	writePNG(t, still, 40, 20, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255})
	if err := tools.Thumbnail(ctx, still, filepath.Join(dir, "still.jpg"), 20); err != nil || !exists(filepath.Join(dir, "still.jpg")) {
		t.Fatalf("Thumbnail of a still via ffmpeg: %v", err)
	}

	var progress []float64
	changed, err := p.Transcode(ctx, &m, func(f float64) { progress = append(progress, f) })
	switch {
	case !tools.Theora:
		if !errors.Is(err, ErrNoTheora) || changed || m.TranscodedPath != "" {
			t.Fatalf("Transcode without Theora = %v, %v; %+v", changed, err, m)
		}
		t.Logf("ffmpeg has no libtheora encoder: transcode skipped (%s)", tools.Status())
	default:
		if err != nil || !changed || !exists(m.TranscodedPath) || !models.IsPlayableVideo(m.TranscodedPath) {
			t.Fatalf("Transcode = %v, %v; %+v", changed, err, m)
		}
		if len(progress) == 0 || progress[len(progress)-1] != 1 {
			t.Fatalf("progress = %v", progress)
		}
		if info, err := tools.Probe(ctx, m.TranscodedPath); err != nil || info.Width != 64 {
			t.Fatalf("probe of the transcode = %+v, %v", info, err)
		}
	}
	if parts, _ := filepath.Glob(filepath.Join(dir, "cache", "*.part.*")); len(parts) != 0 {
		t.Fatalf("part files left behind: %v", parts)
	}

	// Cancellation stops a tool.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := tools.Probe(cctx, src); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Probe = %v", err)
	}
}
