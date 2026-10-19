package presentations

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/gif"  // register the decoders the embedded images may use
	_ "image/jpeg" // (PNG is registered by presentations.go's import)
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// embeddedRenderer is the fallback for zip-based documents when no office
// converter exists: a .pptx yields one page per slide that has a picture
// (the slide's largest embedded image, in slide order); a .key yields its
// preview image. Text and shapes are not drawn (Result.Partial).
type embeddedRenderer struct{}

// maxEmbeddedImage bounds how much of one embedded image is read (a decoy
// zip entry must not exhaust memory).
const maxEmbeddedImage = 64 << 20

func (embeddedRenderer) Render(ctx context.Context, src, dir string, progress Progress) (Result, error) {
	name := filepath.Base(src)
	zr, err := zip.OpenReader(src)
	if err != nil {
		return Result{}, fmt.Errorf("render %s: not a zip-based document: %w", name, err)
	}
	defer zr.Close()
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[path.Clean(f.Name)] = f
	}
	var images []string
	switch strings.ToLower(filepath.Ext(src)) {
	case ".key":
		images = keynoteImages(files)
	default:
		images = pptxImages(files)
	}
	if len(images) == 0 {
		return Result{}, fmt.Errorf("render %s: no embedded slide images (install LibreOffice to render it)", name)
	}
	n := 0
	for _, entry := range images {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		img, err := readZipImage(files[entry])
		if err != nil {
			continue // not a decodable image (EMF, SVG, …)
		}
		n++
		if err := writePNG(filepath.Join(dir, pageName(n)), letterbox(img)); err != nil {
			return Result{}, fmt.Errorf("render %s: page %d: %w", name, n, err)
		}
		if progress != nil {
			progress(n, len(images))
		}
	}
	if n == 0 {
		return Result{}, fmt.Errorf("render %s: no decodable embedded images (install LibreOffice to render it)", name)
	}
	return Result{Pages: n, Renderer: RendererEmbedded, Partial: true}, nil
}

// readZipImage decodes one zip entry as an image.
func readZipImage(f *zip.File) (image.Image, error) {
	if f == nil {
		return nil, fmt.Errorf("missing entry")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxEmbeddedImage))
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// imageEntry reports whether a zip entry name is a decodable picture.
func imageEntry(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif":
		return true
	}
	return false
}

// relationships is an OOXML .rels file.
type relationships struct {
	Rels []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
		Type   string `xml:"Type,attr"`
	} `xml:"Relationship"`
}

// readRels parses a .rels entry into id → target (resolved against base,
// the folder of the part the rels belong to).
func readRels(files map[string]*zip.File, relsPath, base string) map[string]string {
	out := map[string]string{}
	f := files[relsPath]
	if f == nil {
		return out
	}
	rc, err := f.Open()
	if err != nil {
		return out
	}
	defer rc.Close()
	var rels relationships
	if err := xml.NewDecoder(io.LimitReader(rc, 4<<20)).Decode(&rels); err != nil {
		return out
	}
	for _, r := range rels.Rels {
		t := r.Target
		if strings.HasPrefix(t, "/") {
			t = path.Clean(strings.TrimPrefix(t, "/"))
		} else {
			t = path.Clean(path.Join(base, t))
		}
		out[r.ID] = t
	}
	return out
}

// pptxImages lists, in slide order, the largest picture of every slide that
// has one (by compressed size, which tracks pixel size well enough and needs
// no decoding). Slides without pictures are skipped. A deck whose slide
// list cannot be read falls back to the document thumbnail.
func pptxImages(files map[string]*zip.File) []string {
	type sldID struct {
		RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
	}
	var pres struct {
		Slides []sldID `xml:"sldIdLst>sldId"`
	}
	if f := files["ppt/presentation.xml"]; f != nil {
		if rc, err := f.Open(); err == nil {
			_ = xml.NewDecoder(io.LimitReader(rc, 4<<20)).Decode(&pres)
			rc.Close()
		}
	}
	presRels := readRels(files, "ppt/_rels/presentation.xml.rels", "ppt")
	var slides []string
	for _, s := range pres.Slides {
		if t, ok := presRels[s.RID]; ok {
			slides = append(slides, t)
		}
	}
	if len(slides) == 0 {
		// No slide list: every slide part, in number order.
		for name := range files {
			if strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml") {
				slides = append(slides, name)
			}
		}
		sort.Slice(slides, func(i, j int) bool { return slideNumber(slides[i]) < slideNumber(slides[j]) })
	}
	var out []string
	for _, slide := range slides {
		rels := readRels(files, path.Join(path.Dir(slide), "_rels", path.Base(slide)+".rels"), path.Dir(slide))
		best, bestSize := "", uint64(0)
		for _, target := range rels {
			f := files[target]
			if f == nil || !imageEntry(target) {
				continue
			}
			if f.UncompressedSize64 > bestSize {
				best, bestSize = target, f.UncompressedSize64
			}
		}
		if best != "" {
			out = append(out, best)
		}
	}
	if len(out) == 0 {
		for _, thumb := range []string{"docProps/thumbnail.jpeg", "docProps/thumbnail.jpg", "docProps/thumbnail.png"} {
			if files[thumb] != nil {
				return []string{thumb}
			}
		}
	}
	return out
}

// slideNumber extracts N from ppt/slides/slideN.xml (0 when absent).
func slideNumber(name string) int {
	base := strings.TrimSuffix(path.Base(name), ".xml")
	n := 0
	for _, r := range strings.TrimPrefix(base, "slide") {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// keynoteImages returns a Keynote package's preview image (the largest of
// the previews it stores at its root).
func keynoteImages(files map[string]*zip.File) []string {
	for _, name := range []string{"preview.jpeg", "preview.jpg", "preview.png", "preview-web.jpeg", "preview-micro.jpeg"} {
		if files[name] != nil {
			return []string{name}
		}
	}
	return nil
}
