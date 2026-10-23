//go:build darwin

package media

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -Wno-deprecated-declarations
#cgo LDFLAGS: -framework AVFoundation -framework CoreMedia -framework CoreGraphics -framework Foundation
#import <AVFoundation/AVFoundation.h>
#import <CoreGraphics/CoreGraphics.h>
#include <stdlib.h>

// fw_asset_info reads a file's duration and its first video track's display
// size (the preferred transform applied). Returns 0 on success, 1 when
// AVFoundation cannot read the file. Synchronous: call it off the main thread.
static int fw_asset_info(const char *path, double *dur, int *w, int *h, int *hasVideo, int *hasAudio) {
	@autoreleasepool {
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
		AVURLAsset *asset = [AVURLAsset URLAssetWithURL:url options:@{AVURLAssetPreferPreciseDurationAndTimingKey: @YES}];
		if (asset == nil || !asset.playable && !asset.readable) return 1;
		NSArray *vt = [asset tracksWithMediaType:AVMediaTypeVideo];
		NSArray *at = [asset tracksWithMediaType:AVMediaTypeAudio];
		*hasVideo = vt.count > 0;
		*hasAudio = at.count > 0;
		if (vt.count == 0 && at.count == 0) return 1;
		CMTime d = asset.duration;
		*dur = CMTIME_IS_NUMERIC(d) ? CMTimeGetSeconds(d) : 0;
		*w = 0; *h = 0;
		if (vt.count > 0) {
			AVAssetTrack *t = vt[0];
			CGSize s = CGSizeApplyAffineTransform(t.naturalSize, t.preferredTransform);
			*w = (int)llround(fabs(s.width));
			*h = (int)llround(fabs(s.height));
		}
		return 0;
	}
}

// fw_asset_poster decodes the frame at secs (falling back to the first
// frame), at most maxW pixels wide, as RGBA rows top-down in a malloc'd
// buffer the caller frees. Returns NULL on failure.
static unsigned char *fw_asset_poster(const char *path, double secs, int maxW, int *outW, int *outH) {
	@autoreleasepool {
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
		AVURLAsset *asset = [AVURLAsset URLAssetWithURL:url options:nil];
		if (asset == nil || [asset tracksWithMediaType:AVMediaTypeVideo].count == 0) return NULL;
		AVAssetImageGenerator *gen = [AVAssetImageGenerator assetImageGeneratorWithAsset:asset];
		gen.appliesPreferredTrackTransform = YES;
		gen.maximumSize = CGSizeMake(maxW, 0);
		gen.requestedTimeToleranceBefore = CMTimeMakeWithSeconds(0.5, 600);
		gen.requestedTimeToleranceAfter = CMTimeMakeWithSeconds(0.5, 600);
		CGImageRef img = NULL;
		double d = CMTIME_IS_NUMERIC(asset.duration) ? CMTimeGetSeconds(asset.duration) : 0;
		if (d > secs) {
			img = [gen copyCGImageAtTime:CMTimeMakeWithSeconds(secs, 600) actualTime:NULL error:NULL];
		}
		if (img == NULL) {
			img = [gen copyCGImageAtTime:kCMTimeZero actualTime:NULL error:NULL];
		}
		if (img == NULL) return NULL;
		size_t w = CGImageGetWidth(img), h = CGImageGetHeight(img);
		if (w == 0 || h == 0) { CGImageRelease(img); return NULL; }
		unsigned char *buf = calloc(w * h, 4);
		if (buf == NULL) { CGImageRelease(img); return NULL; }
		// Draw in the frame's own colour space: no colour matching, so the
		// pixels are the decoder's (what the player shows), not an sRGB
		// conversion of them.
		CGColorSpaceRef cs = CGImageGetColorSpace(img);
		if (cs != NULL && CGColorSpaceGetModel(cs) == kCGColorSpaceModelRGB) {
			CGColorSpaceRetain(cs);
		} else {
			cs = CGColorSpaceCreateWithName(kCGColorSpaceSRGB);
		}
		CGContextRef ctx = CGBitmapContextCreate(buf, w, h, 8, w * 4, cs,
			kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
		CGColorSpaceRelease(cs);
		if (ctx == NULL) { free(buf); CGImageRelease(img); return NULL; }
		CGContextSetRGBFillColor(ctx, 0, 0, 0, 1);
		CGContextFillRect(ctx, CGRectMake(0, 0, w, h)); // opaque: premultiplied == straight
		CGContextDrawImage(ctx, CGRectMake(0, 0, w, h), img);
		CGContextRelease(ctx);
		CGImageRelease(img);
		*outW = (int)w; *outH = (int)h;
		return buf;
	}
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"time"
	"unsafe"

	"freebeams/internal/models"
)

// AVFoundation probes videos and audio files and takes video poster frames
// through macOS's AVFoundation (AVURLAsset, AVAssetImageGenerator), so the
// library has thumbnails without ffmpeg. It is safe off the main thread (no
// AppKit). Stills are not its business: it answers ErrUnavailable for them.
type AVFoundation struct{}

// AVFoundationAvailable reports whether this platform has it (macOS).
func AVFoundationAvailable() bool { return true }

// Probe reads a video's display size and duration, or an audio file's duration.
func (AVFoundation) Probe(ctx context.Context, path string) (Info, error) {
	kind, _ := models.MediaKindOf(path)
	if !kind.Plays() {
		return Info{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))
	var dur C.double
	var w, h, hv, ha C.int
	if C.fw_asset_info(cs, &dur, &w, &h, &hv, &ha) != 0 {
		return Info{}, fmt.Errorf("probe %s: AVFoundation cannot read the file", filepath.Base(path))
	}
	return Info{Width: int(w), Height: int(h), Duration: time.Duration(float64(dur) * float64(time.Second))}, nil
}

// Thumbnail writes a video's poster frame (one second in, else the first
// frame) to dst as a JPEG, width pixels wide at most.
func (AVFoundation) Thumbnail(ctx context.Context, src, dst string, width int) error {
	if kind, _ := models.MediaKindOf(src); kind != models.MediaVideo {
		return ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if width <= 0 {
		width = 320
	}
	cs := C.CString(src)
	defer C.free(unsafe.Pointer(cs))
	var w, h C.int
	buf := C.fw_asset_poster(cs, C.double(posterAt.Seconds()), C.int(width), &w, &h)
	if buf == nil {
		return fmt.Errorf("thumbnail %s: %w", filepath.Base(src), errors.New("AVFoundation decoded no frame"))
	}
	defer C.free(unsafe.Pointer(buf))
	img := &image.RGBA{Pix: C.GoBytes(unsafe.Pointer(buf), w*h*4), Stride: int(w) * 4, Rect: image.Rect(0, 0, int(w), int(h))}
	if err := writeJPEG(dst, img); err != nil {
		return fmt.Errorf("thumbnail %s: %w", filepath.Base(src), err)
	}
	return nil
}
