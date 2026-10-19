package presentations

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// writePDF writes a PDF with one solid-colour 1920×1080pt page per colour,
// with a correct xref table (Core Graphics and poppler both read it).
func writePDF(t *testing.T, path string, colors [][3]float64) {
	t.Helper()
	var objs []string
	kids := make([]string, 0, len(colors))
	for i := range colors {
		kids = append(kids, fmt.Sprintf("%d 0 R", 3+2*i))
	}
	objs = append(objs, "<< /Type /Catalog /Pages 2 0 R >>")
	objs = append(objs, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(colors)))
	for i, c := range colors {
		content := fmt.Sprintf("%.3f %.3f %.3f rg 0 0 1920 1080 re f", c[0], c[1], c[2])
		objs = append(objs, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 1920 1080] /Contents %d 0 R >>", 4+2*i))
		objs = append(objs, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// solidPNG / solidJPEG encode a w×h image of one colour.
func solidPNG(w, h int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func solidJPEG(w, h int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: 100})
	return b.Bytes()
}

// writePPTX writes a minimal .pptx: slides in order, each with the given
// embedded image (nil for a slide without pictures). thumb adds
// docProps/thumbnail.jpeg.
func writePPTX(t *testing.T, path string, slides [][]byte, thumb []byte) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name string, data []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	write("[Content_Types].xml", []byte(`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`))
	var ids, rels strings.Builder
	for i := range slides {
		fmt.Fprintf(&ids, `<p:sldId id="%d" r:id="rId%d"/>`, 256+i, i+1)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>`, i+1, i+1)
	}
	write("ppt/presentation.xml", []byte(`<?xml version="1.0"?><p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldIdLst>`+ids.String()+`</p:sldIdLst></p:presentation>`))
	write("ppt/_rels/presentation.xml.rels", []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+rels.String()+`</Relationships>`))
	for i, img := range slides {
		write(fmt.Sprintf("ppt/slides/slide%d.xml", i+1), []byte(`<?xml version="1.0"?><p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"/>`))
		slideRels := `<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>`
		if img != nil {
			ext := "png"
			if bytes.HasPrefix(img, []byte{0xFF, 0xD8}) {
				ext = "jpeg"
			}
			name := fmt.Sprintf("image%d.%s", i+1, ext)
			write("ppt/media/"+name, img)
			slideRels += fmt.Sprintf(`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/%s"/>`, name)
		}
		write(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", i+1), []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+slideRels+`</Relationships>`))
	}
	if thumb != nil {
		write("docProps/thumbnail.jpeg", thumb)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// pixelAt decodes a page image and returns the colour at (x, y).
func pixelAt(t *testing.T, path string, x, y int) color.RGBA {
	t.Helper()
	img, err := decodeFile(path)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if b := img.Bounds(); b.Dx() != PageWidth || b.Dy() != PageHeight {
		t.Fatalf("%s is %dx%d, want %dx%d", filepath.Base(path), b.Dx(), b.Dy(), PageWidth, PageHeight)
	}
	r, g, b, _ := img.At(x, y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
}

// near reports whether two colours are within tol per channel.
func near(a, b color.RGBA, tol int) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}

func TestKeyAndPaths(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "My Deck (final).pdf")
	if err := os.WriteFile(src, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	k1, err := Key(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(k1, "-My_Deck_final") || len(k1) != 16+1+len("My_Deck_final") {
		t.Fatalf("Key = %q", k1)
	}
	k2, err := Key(src)
	if err != nil || k2 != k1 {
		t.Fatalf("Key is not stable: %q vs %q (%v)", k1, k2, err)
	}
	// A changed file (size) gets another key.
	if err := os.WriteFile(src, []byte("one two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if k3, err := Key(src); err != nil || k3 == k1 {
		t.Fatalf("Key after edit = %q, %v (want a new key)", k3, err)
	}
	// Same size, later mtime.
	if err := os.WriteFile(src, []byte("one twp"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(src, time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	k4, _ := Key(src)
	if err := os.Chtimes(src, time.Now(), time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if k5, _ := Key(src); k5 == k4 {
		t.Fatalf("Key ignores the modification time: %q", k5)
	}
	if _, err := Key(filepath.Join(dir, "missing.pdf")); err == nil {
		t.Fatal("Key of a missing file must fail")
	}
	if _, err := Key(dir); err == nil {
		t.Fatal("Key of a directory must fail")
	}

	cache := filepath.Join(dir, "cache")
	if got := PagePath(cache, "k", 7); got != filepath.Join(cache, "k", "page-007.png") {
		t.Errorf("PagePath = %q", got)
	}
	if got := ThumbPath(cache, "k"); got != filepath.Join(cache, "k", "thumb.jpg") {
		t.Errorf("ThumbPath = %q", got)
	}
	if got := Pages(cache, "k", 2); len(got) != 2 || got[1] != PagePath(cache, "k", 2) {
		t.Errorf("Pages = %v", got)
	}
	if Ready(cache, "k", 2) || Ready(cache, "", 1) || Ready(cache, "k", 0) {
		t.Error("Ready must be false for a missing render")
	}
	for _, p := range append(Pages(cache, "k", 2), ThumbPath(cache, "k")) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !Ready(cache, "k", 2) || Ready(cache, "k", 3) {
		t.Error("Ready with the files present")
	}
	if got := countPages(Dir(cache, "k")); got != 2 {
		t.Errorf("countPages = %d", got)
	}
}

func TestDefaultCacheDir(t *testing.T) {
	root := t.TempDir()
	dir, err := DefaultCacheDir(root)
	if err != nil || dir != filepath.Join(root, "presentations") {
		t.Fatalf("DefaultCacheDir(override) = %q, %v", dir, err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatal("the cache dir was not created")
	}
	def, err := DefaultCacheDir("")
	if err != nil || !strings.HasSuffix(def, filepath.Join("FreeWorship", "presentations")) {
		t.Fatalf("DefaultCacheDir() = %q, %v", def, err)
	}
}

func TestLetterbox(t *testing.T) {
	red := color.RGBA{255, 0, 0, 255}
	tests := []struct {
		name   string
		w, h   int
		wantW  int // width of the drawn image on the canvas
		wantH  int
		center bool // the canvas centre shows the image
	}{
		{"16:9 at canvas size", 1920, 1080, 1920, 1080, true},
		{"16:9 larger (scaled down)", 3840, 2160, 1920, 1080, true},
		{"4:3 larger (fit height)", 1600, 1200, 1440, 1080, true},
		{"portrait (fit height)", 600, 1200, 540, 1080, true},
		{"small (scaled up at most 2x)", 160, 90, 320, 180, true},
		{"wide strip", 1920, 100, 1920, 100, true},
	}
	for _, tc := range tests {
		img := image.NewRGBA(image.Rect(0, 0, tc.w, tc.h))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+3] = 255, 255
		}
		got := letterbox(img)
		if b := got.Bounds(); b.Dx() != PageWidth || b.Dy() != PageHeight {
			t.Fatalf("%s: canvas %v", tc.name, b)
		}
		x0, y0 := (PageWidth-tc.wantW)/2, (PageHeight-tc.wantH)/2
		inside := got.RGBAAt(x0+2, y0+2)
		if !near(inside, red, 2) {
			t.Errorf("%s: pixel inside the image = %v, want red", tc.name, inside)
		}
		if tc.wantW < PageWidth {
			if left := got.RGBAAt(x0-2, PageHeight/2); left != (color.RGBA{0, 0, 0, 255}) {
				t.Errorf("%s: pixel left of the image = %v, want black", tc.name, left)
			}
		}
		if tc.wantH < PageHeight {
			if top := got.RGBAAt(PageWidth/2, y0-2); top != (color.RGBA{0, 0, 0, 255}) {
				t.Errorf("%s: pixel above the image = %v, want black", tc.name, top)
			}
		}
		if c := got.RGBAAt(PageWidth/2, PageHeight/2); tc.center && !near(c, red, 2) {
			t.Errorf("%s: centre = %v, want red", tc.name, c)
		}
	}
}

func TestNumberedFiles(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"p-1.png", "p-10.png", "p-2.png", "p-x.png", "q-3.png", "p-4.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := numberedFiles(dir, "p", ".png")
	want := []string{"p-1.png", "p-2.png", "p-10.png"}
	if len(got) != len(want) {
		t.Fatalf("numberedFiles = %v", got)
	}
	for i := range want {
		if filepath.Base(got[i]) != want[i] {
			t.Fatalf("numberedFiles = %v, want %v", got, want)
		}
	}
}

func TestCanRender(t *testing.T) {
	none := &Service{}
	pdfOnly := &Service{PDF: coreGraphicsRenderer{}}
	poppler := &Service{PDF: pdftoppmRenderer{bin: "pdftoppm"}}
	full := &Service{PDF: coreGraphicsRenderer{}, Office: sofficeConverter{bin: "soffice"}}
	officeNoPDF := &Service{Office: sofficeConverter{bin: "soffice"}}
	tests := []struct {
		name string
		s    *Service
		path string
		want string
		ok   bool
	}{
		{"pdf without renderer", none, "a.pdf", "", false},
		{"pdf core graphics", pdfOnly, "a.PDF", RendererCoreGraphics, true},
		{"pdf pdftoppm", poppler, "a.pdf", RendererPdftoppm, true},
		{"pptx without office falls back", none, "a.pptx", RendererEmbedded, true},
		{"key without office falls back", pdfOnly, "a.key", RendererEmbedded, true},
		{"ppt without office", pdfOnly, "a.ppt", "", false},
		{"odp without office", pdfOnly, "a.odp", "", false},
		{"pptx with office", full, "a.pptx", RendererLibreOffice, true},
		{"ppt with office", full, "a.ppt", RendererLibreOffice, true},
		{"office without pdf renderer falls back", officeNoPDF, "a.pptx", RendererEmbedded, true},
		{"not a presentation", full, "a.png", "", false},
	}
	for _, tc := range tests {
		got, ok := tc.s.CanRender(tc.path)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: CanRender(%q) = %q, %v; want %q, %v", tc.name, tc.path, got, ok, tc.want, tc.ok)
		}
	}
}

func TestDetect(t *testing.T) {
	tools := Detect(context.Background())
	if tools.Status() == "" {
		t.Fatal("empty status")
	}
	t.Logf("tools: %s (pdftoppm=%q soffice=%q coregraphics=%v)", tools.Status(), tools.Pdftoppm, tools.Soffice, tools.CoreGraphics)
	if tools.Pdftoppm != "" && !tools.CanRenderPDF() {
		t.Error("CanRenderPDF with pdftoppm")
	}
}

// pdfService returns a service with a PDF renderer, or skips the test.
func pdfService(t *testing.T, cache string) *Service {
	t.Helper()
	s := NewService(Detect(context.Background()), cache)
	if s.PDF == nil {
		t.Skip("no PDF renderer on this machine (no Core Graphics, no pdftoppm)")
	}
	return s
}

func TestRenderPDF(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	s := pdfService(t, cache)
	src := filepath.Join(dir, "Two Pages.pdf")
	writePDF(t, src, [][3]float64{{0.2, 0.4, 0.8}, {0.8, 0.4, 0.2}})

	var last [2]int
	key, res, err := s.Render(context.Background(), src, func(done, total int) { last = [2]int{done, total} })
	if err != nil {
		t.Fatal(err)
	}
	if res.Pages != 2 || res.Partial || (res.Renderer != RendererCoreGraphics && res.Renderer != RendererPdftoppm) {
		t.Fatalf("Render = %+v", res)
	}
	if last != [2]int{2, 2} {
		t.Errorf("last progress = %v, want [2 2]", last)
	}
	if !Ready(cache, key, 2) {
		t.Fatal("pages or thumbnail missing after Render")
	}
	if _, err := os.Stat(Dir(cache, key) + ".part"); !errors.Is(err, os.ErrNotExist) {
		t.Error("the .part folder was left behind")
	}
	pages := Pages(cache, key, 2)
	blue, orange := color.RGBA{51, 102, 204, 255}, color.RGBA{204, 102, 51, 255}
	if c := pixelAt(t, pages[0], PageWidth/2, PageHeight/2); !near(c, blue, 3) {
		t.Errorf("page 1 centre = %v, want %v", c, blue)
	}
	if c := pixelAt(t, pages[0], 3, 3); !near(c, blue, 3) {
		t.Errorf("page 1 corner = %v, want %v (a 16:9 page fills the canvas)", c, blue)
	}
	if c := pixelAt(t, pages[1], PageWidth/2, PageHeight/2); !near(c, orange, 3) {
		t.Errorf("page 2 centre = %v, want %v", c, orange)
	}
	thumb, err := decodeFile(ThumbPath(cache, key))
	if err != nil {
		t.Fatal(err)
	}
	if b := thumb.Bounds(); b.Dx() != ThumbWidth || b.Dy() != ThumbWidth*9/16 {
		t.Errorf("thumbnail is %dx%d", b.Dx(), b.Dy())
	}

	// A second render reuses the folder (same key, same result, no work).
	key2, res2, err := s.Render(context.Background(), src, nil)
	if err != nil || key2 != key || res2 != res {
		t.Fatalf("second Render = %q, %+v, %v; want %q, %+v", key2, res2, err, key, res)
	}

	// A cancelled context renders nothing and leaves nothing behind.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	other := filepath.Join(dir, "other.pdf")
	writePDF(t, other, [][3]float64{{0, 0, 0}})
	if _, _, err := s.Render(ctx, other, nil); err == nil {
		t.Fatal("Render with a cancelled context must fail")
	}
	entries, _ := os.ReadDir(cache)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Errorf("leftover %s", e.Name())
		}
	}

	// Not a PDF.
	bad := filepath.Join(dir, "bad.pdf")
	if err := os.WriteFile(bad, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Render(context.Background(), bad, nil); err == nil {
		t.Fatal("Render of a non-PDF must fail")
	}
}

func TestRenderPDFWithoutRenderer(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.pdf")
	writePDF(t, src, [][3]float64{{1, 1, 1}})
	s := &Service{CacheDir: filepath.Join(dir, "cache")}
	_, _, err := s.Render(context.Background(), src, nil)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if _, _, err := (&Service{}).Render(context.Background(), src, nil); err == nil {
		t.Fatal("no cache dir must fail")
	}
}

func TestRenderPPTXEmbedded(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	// No LibreOffice: the embedded images are the pages.
	s := &Service{CacheDir: cache}
	red, green := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 200, 0, 255}
	src := filepath.Join(dir, "Deck.pptx")
	writePPTX(t, src, [][]byte{solidPNG(960, 540, red), nil, solidJPEG(640, 480, green)}, solidJPEG(256, 144, color.RGBA{0, 0, 255, 255}))
	key, res, err := s.Render(context.Background(), src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pages != 2 || !res.Partial || res.Renderer != RendererEmbedded {
		t.Fatalf("Render = %+v", res)
	}
	pages := Pages(cache, key, 2)
	if c := pixelAt(t, pages[0], PageWidth/2, PageHeight/2); !near(c, red, 2) {
		t.Errorf("page 1 centre = %v, want red", c)
	}
	if c := pixelAt(t, pages[0], 3, 3); !near(c, red, 2) {
		t.Errorf("page 1 corner = %v, want red (960x540 is scaled 2x to the canvas)", c)
	}
	if c := pixelAt(t, pages[1], PageWidth/2, PageHeight/2); !near(c, green, 6) {
		t.Errorf("page 2 centre = %v, want green", c)
	}
	if c := pixelAt(t, pages[1], 3, 3); c != (color.RGBA{0, 0, 0, 255}) {
		t.Errorf("page 2 corner = %v, want black (4:3 is letterboxed)", c)
	}
	// The reused render reports the same partial result.
	if _, res2, err := s.Render(context.Background(), src, nil); err != nil || res2 != res {
		t.Fatalf("second Render = %+v, %v", res2, err)
	}

	// A deck without pictures falls back to its thumbnail.
	thumbOnly := filepath.Join(dir, "thumb-only.pptx")
	writePPTX(t, thumbOnly, [][]byte{nil, nil}, solidJPEG(256, 144, color.RGBA{0, 0, 255, 255}))
	if _, res, err := s.Render(context.Background(), thumbOnly, nil); err != nil || res.Pages != 1 || !res.Partial {
		t.Fatalf("thumbnail-only deck: %+v, %v", res, err)
	}

	// Nothing to show at all.
	empty := filepath.Join(dir, "empty.pptx")
	writePPTX(t, empty, [][]byte{nil}, nil)
	if _, _, err := s.Render(context.Background(), empty, nil); err == nil {
		t.Fatal("a deck without any image must fail")
	}

	// Binary PowerPoint has no fallback.
	ppt := filepath.Join(dir, "old.ppt")
	if err := os.WriteFile(ppt, []byte{0xD0, 0xCF, 0x11, 0xE0}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Render(context.Background(), ppt, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf(".ppt without LibreOffice: err = %v, want ErrUnavailable", err)
	}

	// Not a zip.
	notZip := filepath.Join(dir, "garbage.pptx")
	if err := os.WriteFile(notZip, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Render(context.Background(), notZip, nil); err == nil {
		t.Fatal("garbage .pptx must fail")
	}
}

func TestRenderKeynoteEmbedded(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "talk.key")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("preview.jpeg")
	_, _ = w.Write(solidJPEG(1024, 768, color.RGBA{250, 250, 0, 255}))
	w, _ = zw.Create("Index/Slide.iwa")
	_, _ = w.Write([]byte("binary"))
	_ = zw.Close()
	if err := os.WriteFile(src, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Service{CacheDir: filepath.Join(dir, "cache")}
	key, res, err := s.Render(context.Background(), src, nil)
	if err != nil || res.Pages != 1 || !res.Partial {
		t.Fatalf("Render = %+v, %v", res, err)
	}
	if c := pixelAt(t, PagePath(s.CacheDir, key, 1), PageWidth/2, PageHeight/2); !near(c, color.RGBA{250, 250, 0, 255}, 6) {
		t.Errorf("centre = %v", c)
	}
}

func TestRealLibreOffice(t *testing.T) {
	tools := Detect(context.Background())
	if tools.Soffice == "" {
		t.Skip("LibreOffice (soffice) is not installed")
	}
	if !tools.CanRenderPDF() {
		t.Skip("no PDF renderer to draw the converted PDF with")
	}
	dir := t.TempDir()
	s := NewService(tools, filepath.Join(dir, "cache"))
	src := filepath.Join(dir, "deck.pptx")
	writePPTX(t, src, [][]byte{solidPNG(960, 540, color.RGBA{255, 0, 0, 255}), nil}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_, res, err := s.Render(ctx, src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Renderer != RendererLibreOffice || res.Partial || res.Pages != 2 {
		t.Fatalf("Render through LibreOffice = %+v", res)
	}
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	mk := func(rel string) string {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := mk("decks/B Deck.pdf")
	b := mk("decks/sub/a.pptx")
	mk("decks/.hidden/secret.pdf")
	mk("decks/notes.txt")
	mk("decks/.DS_Store.pdf")
	single := mk("single.key")
	items, warnings := Scan(filepath.Join(dir, "decks"), single, single, filepath.Join(dir, "missing"), filepath.Join(dir, "decks/notes.txt"))
	got := make([]string, len(items))
	for i, it := range items {
		got[i] = it.Path
	}
	want := []string{a, b, single}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Scan = %v, want %v", got, want)
	}
	if items[0].Title != "B Deck" || items[2].Title != "single" {
		t.Errorf("titles = %q, %q", items[0].Title, items[2].Title)
	}
	if len(warnings) != 2 {
		t.Errorf("warnings = %v, want 2 (missing path, not a presentation)", warnings)
	}
}
