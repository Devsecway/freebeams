package presentations

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"freebeams/internal/sysproc"
)

// Tools are the external renderers found on this machine. The zero value has
// none; Detect fills it.
type Tools struct {
	CoreGraphics bool   // macOS: PDFs render in-process through Core Graphics
	Pdftoppm     string // poppler's pdftoppm, "" when not found
	Soffice      string // LibreOffice's soffice, "" when not found
}

// sofficeCandidates are where LibreOffice lives when it is not on PATH.
var sofficeCandidates = []string{
	"/Applications/LibreOffice.app/Contents/MacOS/soffice",
	"/opt/homebrew/bin/soffice",
	"/usr/local/bin/soffice",
	"/usr/bin/soffice",
	"/usr/lib/libreoffice/program/soffice",
	`C:\Program Files\LibreOffice\program\soffice.exe`,
}

// Detect looks for the renderers. It never fails and installs nothing.
func Detect(ctx context.Context) Tools {
	t := Tools{CoreGraphics: coreGraphicsSupported}
	if p, err := exec.LookPath("pdftoppm"); err == nil {
		t.Pdftoppm = p
	}
	for _, name := range []string{"soffice", "libreoffice"} {
		if p, err := exec.LookPath(name); err == nil {
			t.Soffice = p
			break
		}
	}
	if t.Soffice == "" {
		for _, c := range sofficeCandidates {
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				t.Soffice = c
				break
			}
		}
	}
	_ = ctx
	return t
}

// CanRenderPDF reports whether PDFs (and office documents through
// LibreOffice) can be rendered.
func (t Tools) CanRenderPDF() bool { return t.CoreGraphics || t.Pdftoppm != "" }

// CanConvertOffice reports whether PowerPoint / Keynote / OpenDocument files
// can be drawn faithfully (LibreOffice plus a PDF renderer).
func (t Tools) CanConvertOffice() bool { return t.Soffice != "" && t.CanRenderPDF() }

// Status is a short description for the operator and the log.
func (t Tools) Status() string {
	var parts []string
	switch {
	case t.CoreGraphics:
		parts = append(parts, "PDF: Core Graphics")
	case t.Pdftoppm != "":
		parts = append(parts, "PDF: pdftoppm")
	default:
		parts = append(parts, "PDF: no renderer (install poppler)")
	}
	switch {
	case t.CanConvertOffice():
		parts = append(parts, "PowerPoint: LibreOffice")
	case t.Soffice != "":
		parts = append(parts, "PowerPoint: LibreOffice found but no PDF renderer")
	default:
		parts = append(parts, "PowerPoint: embedded images only (LibreOffice not found)")
	}
	return strings.Join(parts, "; ")
}

// run executes a tool and returns its stdout; stderr goes into the error.
func run(ctx context.Context, dir, bin string, args ...string) ([]byte, error) {
	cmd := sysproc.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		lines := strings.Split(msg, "\n")
		return nil, fmt.Errorf("%s: %s", filepath.Base(bin), strings.TrimSpace(lines[len(lines)-1]))
	}
	return stdout.Bytes(), nil
}

// pdftoppmRenderer renders PDFs with poppler's pdftoppm: every page to a PNG
// whose longer side is pdftoppmScale pixels, then letterboxed on the canvas.
type pdftoppmRenderer struct{ bin string }

// pdftoppmScale is the longer side of pdftoppm's output. It is above the
// canvas so pages are only ever scaled down.
const pdftoppmScale = 2160

func (r pdftoppmRenderer) Render(ctx context.Context, src, dir string, progress Progress) (Result, error) {
	name := filepath.Base(src)
	work, err := os.MkdirTemp(dir, ".pdftoppm-*")
	if err != nil {
		return Result{}, fmt.Errorf("render %s: %w", name, err)
	}
	defer os.RemoveAll(work)
	if _, err := run(ctx, work, r.bin, "-png", "-scale-to", fmt.Sprint(pdftoppmScale), src, filepath.Join(work, "p")); err != nil {
		return Result{}, fmt.Errorf("render %s: %w", name, err)
	}
	files := numberedFiles(work, "p", ".png")
	if len(files) == 0 {
		return Result{}, fmt.Errorf("render %s: %w", name, ErrNoPages)
	}
	for i, f := range files {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		img, err := decodeFile(f)
		if err != nil {
			return Result{}, fmt.Errorf("render %s: page %d: %w", name, i+1, err)
		}
		if err := writePNG(filepath.Join(dir, pageName(i+1)), letterbox(img)); err != nil {
			return Result{}, fmt.Errorf("render %s: page %d: %w", name, i+1, err)
		}
		if progress != nil {
			progress(i+1, len(files))
		}
	}
	return Result{Pages: len(files), Renderer: RendererPdftoppm}, nil
}

// Converter turns an office document into a PDF file.
type Converter interface {
	// ToPDF writes src as a PDF into dir and returns the PDF's path.
	ToPDF(ctx context.Context, src, dir string) (string, error)
}

// sofficeConverter runs LibreOffice headless. Each conversion uses its own
// user profile under dir, so it never fights a running LibreOffice over the
// profile lock.
type sofficeConverter struct{ bin string }

func (c sofficeConverter) ToPDF(ctx context.Context, src, dir string) (string, error) {
	name := filepath.Base(src)
	profile := filepath.Join(dir, "profile")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		return "", fmt.Errorf("convert %s: %w", name, err)
	}
	profileURL := "file://" + filepath.ToSlash(profile)
	if runtime.GOOS == "windows" {
		profileURL = "file:///" + filepath.ToSlash(profile)
	}
	_, err := run(ctx, dir, c.bin, "-env:UserInstallation="+profileURL, "--headless", "--norestore", "--nolockcheck",
		"--convert-to", "pdf", "--outdir", dir, src)
	if err != nil {
		return "", fmt.Errorf("convert %s: %w", name, err)
	}
	pdf := filepath.Join(dir, strings.TrimSuffix(name, filepath.Ext(name))+".pdf")
	if !exists(pdf) {
		// LibreOffice names the output after the input; look for any PDF it wrote.
		matches, _ := filepath.Glob(filepath.Join(dir, "*.pdf"))
		if len(matches) == 0 {
			return "", fmt.Errorf("convert %s: %w", name, errors.New("LibreOffice wrote no PDF"))
		}
		pdf = matches[0]
	}
	return pdf, nil
}
