package media

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // register the PNG decoder
	"os"
	"path/filepath"
	"strings"
)

// Still images that Go decodes itself (no ffmpeg needed). WebP and BMP are
// left to ffmpeg; Godot still displays them.
var goImageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true}

// goDecodes reports whether Go's image package can read path.
func goDecodes(path string) bool {
	return goImageExts[strings.ToLower(filepath.Ext(path))]
}

// imageInfo reads an image's dimensions with Go's decoders.
func imageInfo(path string) (w, h int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return cfg.Width, cfg.Height, nil
}

// imageThumbnail decodes src with Go's decoders, scales it down to width
// pixels wide (never up; the aspect ratio is kept) and writes it to dst as a
// JPEG, atomically.
func imageThumbnail(src, dst string, width int) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(src), err)
	}
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return fmt.Errorf("decode %s: empty image", filepath.Base(src))
	}
	if width <= 0 || width > sw {
		width = sw
	}
	height := max(1, (sh*width+sw/2)/sw)
	// Flatten onto black (JPEG has no alpha) and box-filter down.
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(rgba, b, img, b.Min, draw.Over)
	thumb := downscale(rgba, width, height)
	return writeJPEG(dst, thumb)
}

// downscale box-filters src (RGBA) to w×h.
func downscale(src *image.RGBA, w, h int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := 0; x < w; x++ {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			var r, g, b, n uint64
			for sy := y0; sy < y1; sy++ {
				i := src.PixOffset(src.Rect.Min.X+x0, src.Rect.Min.Y+sy)
				for sx := x0; sx < x1; sx++ {
					r += uint64(src.Pix[i])
					g += uint64(src.Pix[i+1])
					b += uint64(src.Pix[i+2])
					n++
					i += 4
				}
			}
			o := dst.PixOffset(x, y)
			dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = uint8(r/n), uint8(g/n), uint8(b/n), 255
		}
	}
	return dst
}

// writeJPEG encodes img to path through a temp file and a rename, so a
// reader never sees a partial file.
func writeJPEG(path string, img image.Image) (err error) {
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
	if err = jpeg.Encode(f, img, &jpeg.Options{Quality: 85}); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
