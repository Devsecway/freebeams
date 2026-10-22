//go:build darwin

package render

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AVFoundation -framework CoreMedia -framework CoreVideo -framework Accelerate -framework QuartzCore -framework Foundation
#import <AVFoundation/AVFoundation.h>
#import <Accelerate/Accelerate.h>
#import <QuartzCore/QuartzCore.h>
#include <stdint.h>
#include <string.h>

// FwPlayer wraps one AVPlayer playing one file: the item, a video output
// that hands us BGRA frames, and the end-of-item observer. Every call comes
// from the Godot main thread; the observer block runs on the main queue,
// which Godot's NSApp run loop pumps between frames, so nothing races.
@interface FwPlayer : NSObject
@property(strong) AVPlayer *player;
@property(strong) AVPlayerItem *item;
@property(strong) AVPlayerItemVideoOutput *output;
@property(strong) id endObserver;
@property(assign) CVPixelBufferRef pending; // a frame too large for the last buffer
@property BOOL loop;
@property BOOL ended;
@end

@implementation FwPlayer
@end

static FwPlayer *fw_av(uintptr_t h) { return (__bridge FwPlayer *)(void *)h; }

// fw_av_open opens path. The player is paused at 0 until fw_av_play. 0 when
// the file cannot be opened at all (status failures surface through
// fw_av_status later).
static uintptr_t fw_av_open(const char *path) {
	NSString *s = [NSString stringWithUTF8String:path];
	if (s == nil) {
		return 0;
	}
	NSURL *url = [NSURL fileURLWithPath:s];
	AVURLAsset *asset = [AVURLAsset URLAssetWithURL:url options:nil];
	if (asset == nil) {
		return 0;
	}
	FwPlayer *p = [FwPlayer new];
	p.item = [AVPlayerItem playerItemWithAsset:asset];
	NSDictionary *attrs = @{
		(id)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_32BGRA),
		(id)kCVPixelBufferIOSurfacePropertiesKey : @{},
	};
	p.output = [[AVPlayerItemVideoOutput alloc] initWithPixelBufferAttributes:attrs];
	p.output.suppressesPlayerRendering = YES; // frames go to our texture only
	[p.item addOutput:p.output];
	p.player = [AVPlayer playerWithPlayerItem:p.item];
	// Local files: start at once, never wait to "minimise stalling".
	p.player.automaticallyWaitsToMinimizeStalling = NO;
	// Looping: at the end jump back to 0 without touching the rate; otherwise
	// pause and tell Go (fw_av_ended).
	p.player.actionAtItemEnd = AVPlayerActionAtItemEndNone;
	__weak FwPlayer *weak = p;
	p.endObserver = [[NSNotificationCenter defaultCenter]
	    addObserverForName:AVPlayerItemDidPlayToEndTimeNotification
	                object:p.item
	                 queue:[NSOperationQueue mainQueue]
	            usingBlock:^(NSNotification *note) {
		            FwPlayer *me = weak;
		            if (me == nil) {
			            return;
		            }
		            if (me.loop) {
			            [me.item seekToTime:kCMTimeZero toleranceBefore:kCMTimeZero toleranceAfter:kCMTimeZero completionHandler:nil];
		            } else {
			            [me.player pause];
			            me.ended = YES;
		            }
	            }];
	return (uintptr_t)(__bridge_retained void *)p;
}

static void fw_av_close(uintptr_t h) {
	if (h == 0) {
		return;
	}
	FwPlayer *p = (__bridge_transfer FwPlayer *)(void *)h;
	if (p.endObserver != nil) {
		[[NSNotificationCenter defaultCenter] removeObserver:p.endObserver];
		p.endObserver = nil;
	}
	[p.player pause];
	if (p.pending != NULL) {
		CVPixelBufferRelease(p.pending);
		p.pending = NULL;
	}
	[p.item removeOutput:p.output];
	[p.player replaceCurrentItemWithPlayerItem:nil];
	p.output = nil;
	p.item = nil;
	p.player = nil;
}

static void fw_av_play(uintptr_t h) {
	FwPlayer *p = fw_av(h);
	p.ended = NO;
	[p.player play];
}

static void fw_av_pause(uintptr_t h) { [fw_av(h).player pause]; }

static void fw_av_seek(uintptr_t h, double secs) {
	FwPlayer *p = fw_av(h);
	p.ended = NO;
	CMTime t = CMTimeMakeWithSeconds(secs, 600);
	[p.item seekToTime:t toleranceBefore:kCMTimeZero toleranceAfter:kCMTimeZero completionHandler:nil];
}

static void fw_av_set_volume(uintptr_t h, float v) { fw_av(h).player.volume = v; }

static void fw_av_set_loop(uintptr_t h, int on) { fw_av(h).loop = on != 0; }

// fw_av_status: 0 loading, 1 ready to play, 2 failed.
static int fw_av_status(uintptr_t h) {
	FwPlayer *p = fw_av(h);
	if (p.player.status == AVPlayerStatusFailed) {
		return 2;
	}
	switch (p.item.status) {
	case AVPlayerItemStatusReadyToPlay:
		return 1;
	case AVPlayerItemStatusFailed:
		return 2;
	default:
		return 0;
	}
}

static double fw_av_seconds(CMTime t) {
	if (!CMTIME_IS_NUMERIC(t)) {
		return 0;
	}
	double s = CMTimeGetSeconds(t);
	return s < 0 ? 0 : s;
}

static double fw_av_position(uintptr_t h) { return fw_av_seconds([fw_av(h).item currentTime]); }

static double fw_av_duration(uintptr_t h) { return fw_av_seconds(fw_av(h).item.duration); }

// fw_av_playing reports whether the player's rate is non-zero.
static int fw_av_playing(uintptr_t h) { return fw_av(h).player.rate != 0 ? 1 : 0; }

// fw_av_ended reports (and clears) a non-looping end of the item.
static int fw_av_ended(uintptr_t h) {
	FwPlayer *p = fw_av(h);
	int e = p.ended ? 1 : 0;
	p.ended = NO;
	return e;
}

// fw_av_tracks reports the item's tracks (valid once ready) and the
// presentation size of its video.
static void fw_av_tracks(uintptr_t h, int *hasVideo, int *hasAudio, int *w, int *hgt) {
	FwPlayer *p = fw_av(h);
	*hasVideo = 0;
	*hasAudio = 0;
	for (AVPlayerItemTrack *t in p.item.tracks) {
		NSString *type = t.assetTrack.mediaType;
		if ([type isEqualToString:AVMediaTypeVideo]) {
			*hasVideo = 1;
		} else if ([type isEqualToString:AVMediaTypeAudio]) {
			*hasAudio = 1;
		}
	}
	CGSize sz = p.item.presentationSize;
	*w = (int)sz.width;
	*hgt = (int)sz.height;
}

// fw_av_copy_frame copies the newest video frame into dst (RGBA8, w*h*4
// bytes, rows top-down, no padding) when there is a new one. It returns 1
// with the frame's size in *w / *h when it copied, 0 when no new frame is
// due, and -2 when the frame needs more than cap bytes: *w / *h then give
// its size and the frame is kept for the next call with a larger buffer.
static int fw_av_copy_frame(uintptr_t h, void *dst, size_t cap, int *w, int *hgt) {
	FwPlayer *p = fw_av(h);
	CVPixelBufferRef buf = p.pending;
	p.pending = NULL;
	if (buf == NULL) {
		CMTime t = [p.output itemTimeForHostTime:CACurrentMediaTime()];
		if (![p.output hasNewPixelBufferForItemTime:t]) {
			return 0;
		}
		buf = [p.output copyPixelBufferForItemTime:t itemTimeForDisplay:NULL];
		if (buf == NULL) {
			return 0;
		}
	}
	size_t bw = CVPixelBufferGetWidth(buf), bh = CVPixelBufferGetHeight(buf);
	*w = (int)bw;
	*hgt = (int)bh;
	if (bw == 0 || bh == 0) {
		CVPixelBufferRelease(buf);
		return 0;
	}
	if (cap < bw * bh * 4 || dst == NULL) {
		p.pending = buf;
		return -2;
	}
	CVPixelBufferLockBaseAddress(buf, kCVPixelBufferLock_ReadOnly);
	vImage_Buffer src = {CVPixelBufferGetBaseAddress(buf), bh, bw, CVPixelBufferGetBytesPerRow(buf)};
	vImage_Buffer out = {dst, bh, bw, bw * 4};
	const uint8_t map[4] = {2, 1, 0, 3}; // BGRA -> RGBA
	vImagePermuteChannels_ARGB8888(&src, &out, map, kvImageNoFlags);
	CVPixelBufferUnlockBaseAddress(buf, kCVPixelBufferLock_ReadOnly);
	CVPixelBufferRelease(buf);
	return 1;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// avFoundationAvailable reports that this platform plays videos and audio
// through its own media framework (AVFoundation): every format macOS plays
// (H.264 / HEVC MP4 and MOV, MP3, AAC, WAV, AIFF, …) works without ffmpeg.
const avFoundationAvailable = true

// avPlayer is the AVFoundation player behind nativePlayer.
type avPlayer struct {
	h C.uintptr_t
}

// openAVPlayer opens path with AVFoundation. The player starts paused at
// 0; its status becomes ready (or failed) a few frames later.
func openAVPlayer(path string) (nativePlayer, error) {
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))
	h := C.fw_av_open(cs)
	if h == 0 {
		return nil, errors.New("AVFoundation could not open the file")
	}
	return &avPlayer{h: h}, nil
}

func (p *avPlayer) Close() {
	if p.h != 0 {
		C.fw_av_close(p.h)
		p.h = 0
	}
}

func (p *avPlayer) Play()                { C.fw_av_play(p.h) }
func (p *avPlayer) Pause()               { C.fw_av_pause(p.h) }
func (p *avPlayer) Seek(secs float64)    { C.fw_av_seek(p.h, C.double(secs)) }
func (p *avPlayer) SetVolume(v float64)  { C.fw_av_set_volume(p.h, C.float(v)) }
func (p *avPlayer) SetLoop(on bool)      { C.fw_av_set_loop(p.h, C.int(b2i(on))) }
func (p *avPlayer) Position() float64    { return float64(C.fw_av_position(p.h)) }
func (p *avPlayer) Duration() float64    { return float64(C.fw_av_duration(p.h)) }
func (p *avPlayer) Playing() bool        { return C.fw_av_playing(p.h) != 0 }
func (p *avPlayer) TakeEnded() bool      { return C.fw_av_ended(p.h) != 0 }
func (p *avPlayer) Status() playerStatus { return playerStatus(C.fw_av_status(p.h)) }

func (p *avPlayer) Tracks() (hasVideo, hasAudio bool, w, h int) {
	var v, a, cw, ch C.int
	C.fw_av_tracks(p.h, &v, &a, &cw, &ch)
	return v != 0, a != 0, int(cw), int(ch)
}

// CopyFrame copies the newest frame into dst (see nativePlayer.CopyFrame).
func (p *avPlayer) CopyFrame(dst []byte) (w, h int, result frameResult) {
	var cw, ch C.int
	var ptr unsafe.Pointer
	if len(dst) > 0 {
		ptr = unsafe.Pointer(&dst[0])
	}
	switch C.fw_av_copy_frame(p.h, ptr, C.size_t(len(dst)), &cw, &ch) {
	case 1:
		return int(cw), int(ch), frameCopied
	case -2:
		return int(cw), int(ch), frameTooLarge
	}
	return 0, 0, frameNone
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
