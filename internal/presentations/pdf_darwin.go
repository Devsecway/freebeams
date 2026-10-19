//go:build darwin

package presentations

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <stdlib.h>
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>

// fw_pdf_open opens a PDF file. NULL when it cannot be read as a PDF.
static CGPDFDocumentRef fw_pdf_open(const char *path) {
	CFStringRef s = CFStringCreateWithCString(NULL, path, kCFStringEncodingUTF8);
	if (s == NULL) {
		return NULL;
	}
	CFURLRef url = CFURLCreateWithFileSystemPath(NULL, s, kCFURLPOSIXPathStyle, false);
	CFRelease(s);
	if (url == NULL) {
		return NULL;
	}
	CGPDFDocumentRef doc = CGPDFDocumentCreateWithURL(url);
	CFRelease(url);
	return doc;
}

static void fw_pdf_close(CGPDFDocumentRef doc) {
	if (doc != NULL) {
		CGPDFDocumentRelease(doc);
	}
}

static size_t fw_pdf_pages(CGPDFDocumentRef doc) { return CGPDFDocumentGetNumberOfPages(doc); }

// fw_pdf_locked reports an encrypted document that cannot be opened without
// a password.
static int fw_pdf_ok(CGPDFDocumentRef doc) { return doc != NULL; }

static int fw_pdf_locked(CGPDFDocumentRef doc) {
	return CGPDFDocumentIsEncrypted(doc) && !CGPDFDocumentIsUnlocked(doc);
}

// fw_pdf_render draws page (1-based) into buf, a w*h*4 RGBA buffer (rows top
// to bottom), scaled to fit and centered on black. Returns 0 on success, 1
// when the page does not exist, 2 when no bitmap context could be made.
static int fw_pdf_render(CGPDFDocumentRef doc, size_t page, int w, int h, unsigned char *buf) {
	CGPDFPageRef p = CGPDFDocumentGetPage(doc, page);
	if (p == NULL) {
		return 1;
	}
	CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
	CGContextRef ctx = CGBitmapContextCreate(buf, (size_t)w, (size_t)h, 8, (size_t)w * 4, cs,
		kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
	CGColorSpaceRelease(cs);
	if (ctx == NULL) {
		return 2;
	}
	CGContextSetRGBFillColor(ctx, 0, 0, 0, 1);
	CGContextFillRect(ctx, CGRectMake(0, 0, w, h));

	// The page box as drawn (its /Rotate may swap width and height).
	CGRect box = CGPDFPageGetBoxRect(p, kCGPDFCropBox);
	int rot = CGPDFPageGetRotationAngle(p);
	CGFloat bw = box.size.width, bh = box.size.height;
	if (rot == 90 || rot == 270 || rot == -90) {
		CGFloat t = bw;
		bw = bh;
		bh = t;
	}
	if (bw <= 0 || bh <= 0) {
		CGContextRelease(ctx);
		return 0;
	}
	// Fit (down or up) into the canvas and center. CGPDFPageGetDrawingTransform
	// alone never scales a page up, so the scale is applied here and the
	// transform only handles rotation and the box origin.
	CGFloat scale = (CGFloat)w / bw;
	if ((CGFloat)h / bh < scale) {
		scale = (CGFloat)h / bh;
	}
	CGFloat fw = bw * scale, fh = bh * scale;
	CGContextSaveGState(ctx);
	CGContextTranslateCTM(ctx, ((CGFloat)w - fw) / 2, ((CGFloat)h - fh) / 2);
	CGContextScaleCTM(ctx, scale, scale);
	CGAffineTransform t = CGPDFPageGetDrawingTransform(p, kCGPDFCropBox, CGRectMake(0, 0, bw, bh), 0, true);
	CGContextConcatCTM(ctx, t);
	CGContextClipToRect(ctx, box);
	// Slides often have a white page under their content; draw it, as a
	// viewer does, so a page with no explicit background is not black.
	CGContextSetRGBFillColor(ctx, 1, 1, 1, 1);
	CGContextFillRect(ctx, box);
	CGContextSetInterpolationQuality(ctx, kCGInterpolationHigh);
	CGContextDrawPDFPage(ctx, p);
	CGContextRestoreGState(ctx);
	CGContextRelease(ctx);
	return 0;
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"unsafe"
)

// coreGraphicsSupported reports that PDFs render in-process on this platform.
const coreGraphicsSupported = true

// coreGraphicsRenderer draws PDF pages with Core Graphics (no external tool).
type coreGraphicsRenderer struct{}

func (coreGraphicsRenderer) Render(ctx context.Context, src, dir string, progress Progress) (Result, error) {
	name := filepath.Base(src)
	abs, err := filepath.Abs(src)
	if err != nil {
		return Result{}, fmt.Errorf("render %s: %w", name, err)
	}
	cpath := C.CString(abs)
	defer C.free(unsafe.Pointer(cpath))
	doc := C.fw_pdf_open(cpath)
	if C.fw_pdf_ok(doc) == 0 {
		return Result{}, fmt.Errorf("render %s: not a readable PDF", name)
	}
	defer C.fw_pdf_close(doc)
	if C.fw_pdf_locked(doc) != 0 {
		return Result{}, fmt.Errorf("render %s: the PDF is password protected", name)
	}
	n := int(C.fw_pdf_pages(doc))
	if n <= 0 {
		return Result{}, fmt.Errorf("render %s: %w", name, ErrNoPages)
	}
	buf := make([]byte, PageWidth*PageHeight*4)
	for i := 1; i <= n; i++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		rc := C.fw_pdf_render(doc, C.size_t(i), C.int(PageWidth), C.int(PageHeight), (*C.uchar)(unsafe.Pointer(&buf[0])))
		if rc != 0 {
			return Result{}, fmt.Errorf("render %s: page %d: %w", name, i, errors.New("Core Graphics could not draw the page"))
		}
		img := &image.RGBA{Pix: buf, Stride: PageWidth * 4, Rect: image.Rect(0, 0, PageWidth, PageHeight)}
		if err := writePNG(filepath.Join(dir, pageName(i)), img); err != nil {
			return Result{}, fmt.Errorf("render %s: page %d: %w", name, i, err)
		}
		if progress != nil {
			progress(i, n)
		}
	}
	return Result{Pages: n, Renderer: RendererCoreGraphics}, nil
}
