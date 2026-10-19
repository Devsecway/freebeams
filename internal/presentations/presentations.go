// Package presentations turns presentation documents (PDF, PowerPoint,
// Keynote) into page images the renderer shows as full-screen slides. It
// must not import graphics.gd.
//
// Rendering sits behind the Renderer interface so the app and the tests do
// not care which backend did the work:
//
//   - PDF: Core Graphics on macOS (cgo, pdf_darwin.go), else pdftoppm
//     (poppler) when it is on PATH.
//   - PowerPoint / Keynote / OpenDocument: LibreOffice headless converts the
//     document to PDF, which then goes through the PDF renderer.
//   - Without LibreOffice, a .pptx or .key is read as the zip it is and its
//     embedded slide images become the pages (Result.Partial: text and
//     shapes are missing). A binary .ppt has no fallback.
//
// Every page is a PNG of the logical canvas size (1920×1080), the page
// letterboxed on black, written to a folder of the cache directory named by
// the document's cache key (Key), plus a small JPEG thumbnail of page 1. The
// folder is renamed into place when complete, so a reader never sees a
// half-rendered deck.
package presentations

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"freeworship/internal/models"
)

// ErrUnavailable is returned when no installed tool can render the document.
var ErrUnavailable = errors.New("no renderer is available for this document")

// ErrNoPages is returned when a document rendered to nothing.
var ErrNoPages = errors.New("the document has no pages")

// Canvas size of a page image (the logical slide canvas) and the thumbnail width.
const (
	PageWidth  = models.CanvasWidth
	PageHeight = models.CanvasHeight
	ThumbWidth = 640
)

// Renderer names for models.Presentation.Renderer.
const (
	RendererCoreGraphics = "coregraphics"
	RendererPdftoppm     = "pdftoppm"
	RendererLibreOffice  = "libreoffice"
	RendererEmbedded     = "embedded"
)

// Result is what a render produced.
type Result struct {
	Pages    int    // page images written
	Renderer string // which backend made them
	// Partial is set when the pages are the document's embedded images only.
	Partial bool
}

// Progress reports rendering progress: done pages of total (total may be 0
// while it is unknown).
type Progress func(done, total int)

// Renderer writes the pages of src as page-NNN.png into dir (which exists
// and is empty). It returns how many pages it wrote.
type Renderer interface {
	Render(ctx context.Context, src, dir string, progress Progress) (Result, error)
}

// DefaultCacheDir is where rendered pages go:
// <os.UserCacheDir()>/FreeWorship/presentations (directory created).
// override, when not empty, is the app's cache root instead (the
// FREEWORSHIP_CACHE override the media library uses too), and the pages go
// into its "presentations" subfolder.
func DefaultCacheDir(override string) (string, error) {
	base := override
	if base == "" {
		b, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("presentation cache dir: %w", err)
		}
		base = filepath.Join(b, "FreeWorship")
	}
	dir := filepath.Join(base, "presentations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("presentation cache dir: %w", err)
	}
	return dir, nil
}

// Key is the cache key of a document: a hash of its path, size and
// modification time (so an edited document renders again) with its base name
// (so the cache is readable). A missing file is an error.
func Key(src string) (string, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("presentation %s: %w", filepath.Base(src), err)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("presentation %s: not a regular file", filepath.Base(src))
	}
	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", filepath.Clean(src), fi.Size(), fi.ModTime().UnixNano())))
	base := sanitize(strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)))
	if len(base) > 40 {
		base = base[:40]
	}
	if base == "" {
		return hex.EncodeToString(sum[:8]), nil
	}
	return hex.EncodeToString(sum[:8]) + "-" + base, nil
}

// sanitize keeps a folder name portable: letters, digits, '-', '_' and '.'.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		}
	}
	return b.String()
}

// Dir is the cache folder of a rendered document.
func Dir(cacheDir, key string) string { return filepath.Join(cacheDir, key) }

// PagePath is the image of page i (1-based) of a rendered document.
func PagePath(cacheDir, key string, i int) string {
	return filepath.Join(cacheDir, key, pageName(i))
}

func pageName(i int) string { return fmt.Sprintf("page-%03d.png", i) }

// ThumbPath is the thumbnail (page 1, ThumbWidth pixels wide, JPEG).
func ThumbPath(cacheDir, key string) string { return filepath.Join(cacheDir, key, "thumb.jpg") }

// Pages lists the page images of a rendered document, in order.
func Pages(cacheDir, key string, n int) []string {
	out := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, PagePath(cacheDir, key, i))
	}
	return out
}

// Ready reports whether every page image (and the thumbnail) of a rendered
// document exists in the cache.
func Ready(cacheDir, key string, n int) bool {
	if key == "" || n <= 0 {
		return false
	}
	for _, p := range Pages(cacheDir, key, n) {
		if !exists(p) {
			return false
		}
	}
	return exists(ThumbPath(cacheDir, key))
}

// exists reports whether path is an existing regular file with content.
func exists(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Size() > 0
}

// Service renders documents into the cache: it picks the backend for each
// document type from the Tools, writes into a temporary folder and renames
// it into place when done.
type Service struct {
	Tools    Tools
	CacheDir string
	// PDF renders PDF files; nil when no PDF backend exists (then PDFs, and
	// office documents that would go through PDF, cannot be rendered).
	PDF Renderer
	// Office converts office documents to PDF (LibreOffice); nil without it.
	Office Converter
}

// NewService returns a service using the detected tools and the Core
// Graphics / pdftoppm / LibreOffice backends they allow.
func NewService(t Tools, cacheDir string) *Service {
	s := &Service{Tools: t, CacheDir: cacheDir}
	switch {
	case t.CoreGraphics:
		s.PDF = coreGraphicsRenderer{}
	case t.Pdftoppm != "":
		s.PDF = pdftoppmRenderer{bin: t.Pdftoppm}
	}
	if t.Soffice != "" {
		s.Office = sofficeConverter{bin: t.Soffice}
	}
	return s
}

// CanRender reports whether the service can make pages for path, and how
// ("" when it cannot). A .pptx / .key without LibreOffice falls back to its
// embedded images.
func (s *Service) CanRender(path string) (renderer string, ok bool) {
	if !models.IsPresentationFile(path) {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".pdf" {
		if s.PDF == nil {
			return "", false
		}
		return s.pdfName(), true
	}
	if s.Office != nil && s.PDF != nil {
		return RendererLibreOffice, true
	}
	if ext == ".pptx" || ext == ".key" {
		return RendererEmbedded, true
	}
	return "", false
}

func (s *Service) pdfName() string {
	if _, ok := s.PDF.(pdftoppmRenderer); ok {
		return RendererPdftoppm
	}
	return RendererCoreGraphics
}

// Render makes the pages of src under CacheDir/key (an existing complete
// render is reused) and returns the key with what was made. progress may be
// nil. Cancellation leaves no partial folder behind.
func (s *Service) Render(ctx context.Context, src string, progress Progress) (key string, res Result, err error) {
	if s.CacheDir == "" {
		return "", Result{}, errors.New("render presentation: no cache dir")
	}
	key, err = Key(src)
	if err != nil {
		return "", Result{}, err
	}
	final := Dir(s.CacheDir, key)
	if n := countPages(final); n > 0 && Ready(s.CacheDir, key, n) {
		return key, Result{Pages: n, Renderer: readRenderer(final), Partial: readPartial(final)}, nil
	}
	if err := os.MkdirAll(s.CacheDir, 0o755); err != nil {
		return "", Result{}, fmt.Errorf("render %s: %w", filepath.Base(src), err)
	}
	tmp := final + ".part"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return "", Result{}, fmt.Errorf("render %s: %w", filepath.Base(src), err)
	}
	defer os.RemoveAll(tmp)
	res, err = s.renderInto(ctx, src, tmp, progress)
	if err != nil {
		return "", Result{}, err
	}
	if res.Pages <= 0 {
		return "", Result{}, fmt.Errorf("render %s: %w", filepath.Base(src), ErrNoPages)
	}
	if err := writeThumb(filepath.Join(tmp, pageName(1)), filepath.Join(tmp, "thumb.jpg")); err != nil {
		return "", Result{}, fmt.Errorf("render %s: thumbnail: %w", filepath.Base(src), err)
	}
	writeMeta(tmp, res)
	os.RemoveAll(final)
	if err := os.Rename(tmp, final); err != nil {
		return "", Result{}, fmt.Errorf("render %s: %w", filepath.Base(src), err)
	}
	return key, res, nil
}

// renderInto picks the backend for src and renders into dir.
func (s *Service) renderInto(ctx context.Context, src, dir string, progress Progress) (Result, error) {
	name := filepath.Base(src)
	if !models.IsPresentationFile(src) {
		return Result{}, fmt.Errorf("render %s: not a presentation file", name)
	}
	ext := strings.ToLower(filepath.Ext(src))
	if ext == ".pdf" {
		if s.PDF == nil {
			return Result{}, fmt.Errorf("render %s: %w (install poppler's pdftoppm)", name, ErrUnavailable)
		}
		res, err := s.PDF.Render(ctx, src, dir, progress)
		if err != nil {
			return Result{}, err
		}
		res.Renderer = s.pdfName()
		return res, nil
	}
	// Office documents: LibreOffice → PDF → pages.
	var officeErr error
	if s.Office != nil && s.PDF != nil {
		res, err := s.renderOffice(ctx, src, dir, progress)
		if err == nil {
			return res, nil
		}
		if ctx.Err() != nil {
			return Result{}, err
		}
		officeErr = err
		clearDir(dir)
	}
	// Fallback: the embedded images of a zip-based document.
	if ext == ".pptx" || ext == ".key" {
		res, err := embeddedRenderer{}.Render(ctx, src, dir, progress)
		if err != nil {
			if officeErr != nil {
				return Result{}, fmt.Errorf("%w (and %v)", err, officeErr)
			}
			return Result{}, err
		}
		return res, nil
	}
	if officeErr != nil {
		return Result{}, officeErr
	}
	return Result{}, fmt.Errorf("render %s: %w (install LibreOffice)", name, ErrUnavailable)
}

// renderOffice converts src to PDF in a temp folder and renders that.
func (s *Service) renderOffice(ctx context.Context, src, dir string, progress Progress) (Result, error) {
	work, err := os.MkdirTemp(s.CacheDir, ".convert-*")
	if err != nil {
		return Result{}, fmt.Errorf("render %s: %w", filepath.Base(src), err)
	}
	defer os.RemoveAll(work)
	pdf, err := s.Office.ToPDF(ctx, src, work)
	if err != nil {
		return Result{}, err
	}
	res, err := s.PDF.Render(ctx, pdf, dir, progress)
	if err != nil {
		return Result{}, err
	}
	res.Renderer = RendererLibreOffice
	return res, nil
}

// clearDir removes everything inside dir (a failed backend's leftovers).
func clearDir(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}

// countPages counts the page-NNN.png files in dir, in sequence from 1.
func countPages(dir string) int {
	n := 0
	for exists(filepath.Join(dir, pageName(n+1))) {
		n++
	}
	return n
}

// The folder keeps a one-line note of which backend made it, so a reused
// render reports the same Result.
const metaFile = "renderer.txt"

func writeMeta(dir string, res Result) {
	line := res.Renderer
	if res.Partial {
		line += " partial"
	}
	_ = os.WriteFile(filepath.Join(dir, metaFile), []byte(line+"\n"), 0o644)
}

func readRenderer(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, metaFile))
	if err != nil {
		return ""
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func readPartial(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, metaFile))
	return err == nil && strings.Contains(string(b), "partial")
}

// ---- image helpers -----------------------------------------------------------------

// letterbox draws img scaled to fit the page canvas, centered on black
// (never scaled up beyond 2×, so tiny embedded images stay sharp rather
// than blurry).
func letterbox(img image.Image) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, PageWidth, PageHeight))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return canvas
	}
	scale := min(float64(PageWidth)/float64(sw), float64(PageHeight)/float64(sh), 2)
	w, h := max(1, int(float64(sw)*scale+0.5)), max(1, int(float64(sh)*scale+0.5))
	x, y := (PageWidth-w)/2, (PageHeight-h)/2
	src := toRGBA(img)
	var scaled *image.RGBA
	switch {
	case w == sw && h == sh:
		scaled = src
	case w < sw:
		scaled = downscale(src, w, h)
	default:
		scaled = upscale(src, w, h)
	}
	draw.Draw(canvas, image.Rect(x, y, x+w, y+h), scaled, image.Point{}, draw.Over)
	return canvas
}

// toRGBA converts img to RGBA (a copy unless it already is one at origin 0,0).
func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok && r.Rect.Min == (image.Point{}) {
		return r
	}
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

// downscale box-filters src to w×h (w <= src width, h <= src height).
func downscale(src *image.RGBA, w, h int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := 0; x < w; x++ {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			var r, g, b, a, n uint64
			for sy := y0; sy < y1; sy++ {
				i := src.PixOffset(x0, sy)
				for sx := x0; sx < x1; sx++ {
					r += uint64(src.Pix[i])
					g += uint64(src.Pix[i+1])
					b += uint64(src.Pix[i+2])
					a += uint64(src.Pix[i+3])
					n++
					i += 4
				}
			}
			o := dst.PixOffset(x, y)
			dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = uint8(r/n), uint8(g/n), uint8(b/n), uint8(a/n)
		}
	}
	return dst
}

// upscale enlarges src to w×h with bilinear interpolation.
func upscale(src *image.RGBA, w, h int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(h) - 0.5
		y0 := int(fy)
		if fy < 0 {
			fy, y0 = 0, 0
		}
		y1 := min(y0+1, sh-1)
		wy := fy - float64(y0)
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(w) - 0.5
			x0 := int(fx)
			if fx < 0 {
				fx, x0 = 0, 0
			}
			x1 := min(x0+1, sw-1)
			wx := fx - float64(x0)
			o := dst.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				p00 := float64(src.Pix[src.PixOffset(x0, y0)+c])
				p10 := float64(src.Pix[src.PixOffset(x1, y0)+c])
				p01 := float64(src.Pix[src.PixOffset(x0, y1)+c])
				p11 := float64(src.Pix[src.PixOffset(x1, y1)+c])
				v := (p00*(1-wx)+p10*wx)*(1-wy) + (p01*(1-wx)+p11*wx)*wy
				dst.Pix[o+c] = uint8(v + 0.5)
			}
		}
	}
	return dst
}

// writePNG encodes img to path through a temp file and a rename.
func writePNG(path string, img image.Image) (err error) {
	return writeAtomic(path, func(f *os.File) error {
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		return enc.Encode(f, img)
	})
}

// writeThumb makes the JPEG thumbnail of page image src at dst.
func writeThumb(src, dst string) error {
	img, err := decodeFile(src)
	if err != nil {
		return err
	}
	rgba := toRGBA(img)
	w := min(ThumbWidth, rgba.Bounds().Dx())
	h := max(1, (rgba.Bounds().Dy()*w+rgba.Bounds().Dx()/2)/rgba.Bounds().Dx())
	thumb := downscale(rgba, w, h)
	return writeAtomic(dst, func(f *os.File) error {
		return jpeg.Encode(f, thumb, &jpeg.Options{Quality: 85})
	})
}

// decodeFile decodes a PNG / JPEG / GIF file.
func decodeFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return img, nil
}

// writeAtomic writes path through a temp file in the same folder.
func writeAtomic(path string, write func(*os.File) error) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if err = write(f); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// numberedFiles lists the files in dir matching prefix*ext sorted by the
// number between them (pdftoppm's prefix-1.png, prefix-2.png, …).
func numberedFiles(dir, prefix, ext string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type file struct {
		n    int
		path string
	}
	var files []file
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		mid := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
		mid = strings.TrimLeft(mid, "-_")
		n := 0
		for _, r := range mid {
			if r < '0' || r > '9' {
				n = -1
				break
			}
			n = n*10 + int(r-'0')
		}
		if n < 0 {
			continue
		}
		files = append(files, file{n, filepath.Join(dir, name)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].n < files[j].n })
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.path
	}
	return out
}
