package ffplayer

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os/exec"
	"strconv"
	"time"

	"freebeams/internal/sysproc"
)

// proc is one running ffmpeg with its stdout pipe.
type proc struct {
	cmd    *exec.Cmd
	out    io.Reader
	stderr *tail
}

// startFFmpeg runs ffmpeg with args, its stdout piped to the caller.
func (p *Player) startFFmpeg(ctx context.Context, args []string) (*proc, error) {
	cmd := sysproc.CommandContext(ctx, p.cfg.FFmpeg, args...)
	cmd.WaitDelay = 2 * time.Second // never hang on a pipe a grandchild still holds
	st := &tail{}
	cmd.Stderr = st
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	return &proc{cmd: cmd, out: bufio.NewReaderSize(out, 64<<10), stderr: st}, nil
}

// stop kills the process (when kill) and waits for it. It returns ffmpeg's
// error when it exited on its own with one.
func (pr *proc) stop(kill bool) error {
	if pr == nil {
		return nil
	}
	if kill {
		_ = pr.cmd.Process.Kill()
	}
	err := pr.cmd.Wait()
	if kill || err == nil {
		return nil
	}
	return fmt.Errorf("ffmpeg: %s", pr.stderr.lastLine(err))
}

func secs(s float64) string { return strconv.FormatFloat(s, 'f', 3, 64) }

// inputArgs are the common leading arguments: quiet, no stdin, -ss, -i.
func (p *Player) inputArgs(ss float64) []string {
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error"}
	if ss > 0 {
		args = append(args, "-ss", secs(ss)) // input seeking; accurate (frames before ss are decoded and dropped)
	}
	return append(args, "-i", p.path)
}

// videoArgs decode the video stream at a constant rate (the fps filter,
// start_time 0: frame n is at n / fps), scaled to the frame size, as raw
// RGBA on stdout. The scale filter reads the stream's colour matrix and
// range from the frame tags.
func (p *Player) videoArgs(info Info, ss float64) []string {
	return append(p.inputArgs(ss),
		"-map", "0:"+strconv.Itoa(info.videoIndex), "-an", "-sn", "-dn",
		"-vf", fmt.Sprintf("fps=fps=%s:start_time=0,scale=%d:%d:flags=bicubic+accurate_rnd+full_chroma_int,setsar=1", info.fpsArg, info.FrameWidth, info.FrameHeight),
		"-pix_fmt", "rgba", "-f", "rawvideo", "pipe:1")
}

// audioArgs decode the audio stream as f32le stereo at the output rate;
// aresample pads a late start (first_pts=0) and gaps with silence, so
// sample n is at n / rate like the video's frame n.
func (p *Player) audioArgs(info Info, ss float64) []string {
	rate := strconv.Itoa(p.rate)
	return append(p.inputArgs(ss),
		"-map", "0:"+strconv.Itoa(info.audioIndex), "-vn", "-sn", "-dn",
		"-af", "aresample="+rate+":async=1:first_pts=0",
		"-ac", "2", "-ar", rate, "-f", "f32le", "pipe:1")
}

// videoReader decodes the run's video pass after pass into r.frames.
func (p *Player) videoReader(r *run, info Info) {
	defer p.wg.Done()
	size := info.FrameWidth * info.FrameHeight * 4
	fps := info.FPS
	d := info.Duration
	var next *proc // the next pass, started ahead while looping
	defer func() { _ = next.stop(true) }()
	t0 := 0.0
	for pass := 0; ; pass++ {
		ss := 0.0
		if pass == 0 {
			ss = r.ss
		}
		limit := math.Inf(1)
		if d > 0 {
			limit = d - ss
		}
		pr := next
		next = nil
		if pr == nil {
			var err error
			if pr, err = p.startFFmpeg(r.ctx, p.videoArgs(info, ss)); err != nil {
				r.setFail(err)
				r.vIdle.Store(true)
				return
			}
		}
		n, cut := 0, false
		for {
			if next == nil && n == 1 && d > 0 && p.looping() {
				next, _ = p.startFFmpeg(r.ctx, p.videoArgs(info, 0))
			}
			var buf []byte
			select {
			case buf = <-r.free:
			case <-r.ctx.Done():
				_ = pr.stop(true)
				return
			}
			if cap(buf) < size {
				buf = make([]byte, size)
			}
			buf = buf[:size]
			if _, err := io.ReadFull(pr.out, buf); err != nil {
				r.giveBack(buf)
				break
			}
			rel := float64(n) / fps
			if rel >= limit-1e-6 {
				r.giveBack(buf)
				cut = true
				break
			}
			select {
			case r.frames <- &frame{pix: buf, t: t0 + rel, pos: ss + rel}:
			case <-r.ctx.Done():
				_ = pr.stop(true)
				return
			}
			r.gotVideo.Store(true)
			n++
		}
		perr := pr.stop(cut)
		if r.ctx.Err() != nil {
			return
		}
		if n == 0 && pass == 0 && (d <= 0 || ss < d-0.5) {
			if perr == nil {
				perr = errors.New("ffmpeg decoded no video frames")
			}
			r.setFail(perr)
			r.vIdle.Store(true)
			return
		}
		if d > 0 {
			t0 += limit
		} else {
			t0 += float64(n) / fps
		}
		if !p.looping() {
			_ = next.stop(true)
			next = nil
		}
		if !p.waitLoop(r, &r.vIdle) {
			return
		}
	}
}

// audioChunk is how many frames the audio reader moves at a time.
const audioChunk = 2048

// audioReader decodes the run's audio pass after pass into r.audio, each
// pass padded or cut to its exact length.
func (p *Player) audioReader(r *run, info Info) {
	defer p.wg.Done()
	defer r.aIdle.Store(true)
	d := info.Duration
	raw := make([]byte, audioChunk*8)
	pcm := make([]float32, audioChunk*2)
	for pass := 0; ; pass++ {
		ss := 0.0
		if pass == 0 {
			ss = r.ss
		}
		limit := int64(-1)
		if d > 0 {
			limit = int64(math.Round((d - ss) * float64(p.rate)))
		}
		pr, err := p.startFFmpeg(r.ctx, p.audioArgs(info, ss))
		if err != nil {
			r.setFail(err)
			return
		}
		var got int64
		cut := false
		for limit < 0 || got < limit {
			k, rerr := io.ReadFull(pr.out, raw)
			frames := int64(k / 8)
			if limit >= 0 && got+frames > limit {
				frames, cut = limit-got, true
			}
			for i := range 2 * int(frames) {
				pcm[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:]))
			}
			if frames > 0 {
				if !r.audio.write(pcm[:2*frames]) {
					_ = pr.stop(true)
					return
				}
				got += frames
				r.gotAudio.Store(true)
			}
			if rerr != nil {
				break
			}
		}
		if limit >= 0 && got >= limit {
			cut = true
		}
		perr := pr.stop(cut)
		if r.ctx.Err() != nil {
			return
		}
		if got == 0 && pass == 0 && (perr != nil || !info.HasVideo) && (d <= 0 || ss < d-0.5) {
			if perr == nil {
				perr = errors.New("ffmpeg decoded no audio")
			}
			if !info.HasVideo {
				r.setFail(perr)
				return
			}
			// A video whose sound cannot be decoded plays silent, on the
			// wall clock (the reader's end makes the audio clock give up).
			log.Printf("[ffplayer] %s: the sound cannot play: %v", p.path, perr)
			return
		}
		if limit > got { // the sound is shorter than the pass: silence to its end
			clear(pcm)
			for rest := limit - got; rest > 0; {
				k := min(rest, audioChunk)
				if !r.audio.write(pcm[:2*k]) {
					return
				}
				rest -= k
			}
			r.gotAudio.Store(true)
		}
		if !p.waitLoop(r, &r.aIdle) {
			return
		}
	}
}
