package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"freebeams/internal/models"
	"freebeams/internal/sysproc"
)

// Tools runs ffprobe and ffmpeg. The zero value has no tools; Detect fills it
// (the bundled copy next to the executable, else PATH). It implements Prober, Thumbnailer and Transcoder; each method
// returns ErrUnavailable (or ErrNoTheora) when the needed tool is missing,
// so a Tools value can always be passed to a Processor.
type Tools struct {
	FFmpeg  string // path of the ffmpeg binary, "" when not found
	FFprobe string // path of the ffprobe binary, "" when not found
	Theora  bool   // ffmpeg has the libtheora encoder (needed for transcodes)
}

// Detect finds ffmpeg and ffprobe (Locate: the bundled copy, then PATH) and
// checks ffmpeg's encoders. It never fails: missing tools leave the fields
// empty.
func Detect(ctx context.Context) Tools {
	t := Locate()
	if t.FFmpeg != "" {
		out, err := t.run(ctx, t.FFmpeg, "-hide_banner", "-encoders")
		if err == nil {
			t.Theora = hasEncoder(out, "libtheora")
		}
	}
	return t
}

// hasEncoder reports whether `ffmpeg -encoders` output lists name. Lines look
// like " V....D libtheora            libtheora Theora".
func hasEncoder(out []byte, name string) bool {
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 2 && f[1] == name {
			return true
		}
	}
	return false
}

// Available reports whether both tools were found.
func (t Tools) Available() bool { return t.FFmpeg != "" && t.FFprobe != "" }

// CanTranscode reports whether videos can be converted for playback.
func (t Tools) CanTranscode() bool { return t.FFmpeg != "" && t.Theora }

// Status is a short description for the operator ("ffmpeg 7: ready",
// "ffmpeg without Theora", "ffmpeg not found").
func (t Tools) Status() string {
	switch {
	case t.FFmpeg == "" && t.FFprobe == "":
		return "ffmpeg not found"
	case t.FFmpeg == "":
		return "ffmpeg not found (ffprobe only)"
	case !t.Theora:
		return "ffmpeg without the Theora encoder"
	case t.FFprobe == "":
		return "ffmpeg ready (ffprobe not found)"
	}
	return "ffmpeg ready"
}

// run executes a tool and returns its stdout; stderr goes into the error.
func (t Tools) run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := sysproc.CommandContext(ctx, bin, args...)
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
		return nil, fmt.Errorf("%s: %s", filepath.Base(bin), lastLine(msg))
	}
	return stdout.Bytes(), nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Probe reads width, height and duration with ffprobe.
func (t Tools) Probe(ctx context.Context, path string) (Info, error) {
	if t.FFprobe == "" {
		return Info{}, ErrUnavailable
	}
	kind, _ := models.MediaKindOf(path)
	audio := kind == models.MediaAudio
	streams := "v:0"
	if audio {
		streams = "a:0"
	}
	out, err := t.run(ctx, t.FFprobe, "-v", "error", "-select_streams", streams,
		"-show_entries", "stream=width,height:format=duration", "-of", "json", path)
	if err != nil {
		return Info{}, fmt.Errorf("probe %s: %w", filepath.Base(path), err)
	}
	info, err := parseProbe(out, audio)
	if err != nil {
		return Info{}, fmt.Errorf("probe %s: %w", filepath.Base(path), err)
	}
	return info, nil
}

// parseProbe decodes ffprobe's JSON (stream width/height, format duration).
// A picture needs a video stream; an audio file needs a duration.
func parseProbe(out []byte, audio bool) (Info, error) {
	var doc struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return Info{}, fmt.Errorf("bad ffprobe output: %w", err)
	}
	var info Info
	if len(doc.Streams) > 0 {
		info.Width, info.Height = doc.Streams[0].Width, doc.Streams[0].Height
	}
	if d := strings.TrimSpace(doc.Format.Duration); d != "" && d != "N/A" {
		secs, err := strconv.ParseFloat(d, 64)
		if err == nil && secs > 0 {
			info.Duration = time.Duration(secs * float64(time.Second))
		}
	}
	if audio {
		if info.Duration <= 0 {
			return info, errors.New("no audio duration")
		}
		return info, nil
	}
	if info.Width <= 0 || info.Height <= 0 {
		return info, errors.New("no video stream")
	}
	return info, nil
}

// posterAt is where a video's poster frame is taken from (motion
// backgrounds often fade in from black).
const posterAt = 1 * time.Second

// Thumbnail writes a JPEG still of src to dst, width pixels wide: a scaled
// copy of an image, or a frame of a video (one second in, else its first
// frame). Written atomically.
func (t Tools) Thumbnail(ctx context.Context, src, dst string, width int) error {
	if t.FFmpeg == "" {
		return ErrUnavailable
	}
	if width <= 0 {
		width = 320
	}
	kind, _ := models.MediaKindOf(src)
	scale := fmt.Sprintf("scale=%d:-2", width)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("thumbnail %s: %w", filepath.Base(src), err)
	}
	tmp := partFile(dst, ".jpg")
	defer os.Remove(tmp)
	seeks := [][]string{nil}
	if kind == models.MediaVideo {
		seeks = [][]string{{"-ss", strconv.FormatFloat(posterAt.Seconds(), 'f', 2, 64)}, nil}
	}
	var err error
	for _, seek := range seeks {
		args := append([]string{"-v", "error", "-y"}, seek...)
		args = append(args, "-i", src, "-frames:v", "1", "-vf", scale, "-f", "image2", "-c:v", "mjpeg", "-q:v", "3", tmp)
		_, err = t.run(ctx, t.FFmpeg, args...)
		if err == nil && exists(tmp) {
			if err = os.Rename(tmp, dst); err != nil {
				return fmt.Errorf("thumbnail %s: %w", filepath.Base(src), err)
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			err = errors.New("ffmpeg wrote no frame")
		}
	}
	return fmt.Errorf("thumbnail %s: %w", filepath.Base(src), err)
}

// maxTranscodeWidth bounds the transcode: motion backgrounds need no more
// than 1080p, and Theora decoding is done in software.
const maxTranscodeWidth = 1920

// Transcode converts src to Ogg Theora (video only, no audio: motion
// backgrounds are silent) at dst, atomically. progress gets the fraction
// done, taken from ffmpeg's -progress output against the probed duration.
func (t Tools) Transcode(ctx context.Context, src, dst string, progress func(float64)) error {
	if t.FFmpeg == "" {
		return ErrUnavailable
	}
	if !t.Theora {
		return ErrNoTheora
	}
	var total time.Duration
	if info, err := t.Probe(ctx, src); err == nil {
		total = info.Duration
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("transcode %s: %w", filepath.Base(src), err)
	}
	tmp := partFile(dst, ".ogv")
	defer os.Remove(tmp)
	cmd := sysproc.CommandContext(ctx, t.FFmpeg, "-v", "error", "-y", "-i", src, "-an",
		"-vf", fmt.Sprintf("scale='min(%d,iw)':-2", maxTranscodeWidth),
		"-c:v", "libtheora", "-q:v", "7", "-f", "ogg",
		"-nostats", "-progress", "pipe:1", tmp)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("transcode %s: %w", filepath.Base(src), err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("transcode %s: %w", filepath.Base(src), err)
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if progress != nil && total > 0 {
			if frac, ok := progressFraction(sc.Text(), total); ok {
				progress(frac)
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := lastLine(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("transcode %s: ffmpeg: %s", filepath.Base(src), msg)
	}
	if !exists(tmp) {
		return fmt.Errorf("transcode %s: ffmpeg wrote nothing", filepath.Base(src))
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("transcode %s: %w", filepath.Base(src), err)
	}
	if progress != nil {
		progress(1)
	}
	return nil
}

// progressFraction reads one `-progress` line ("out_time_us=1234567" or
// "out_time_ms=...") into a fraction of total.
func progressFraction(line string, total time.Duration) (float64, bool) {
	key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
	if !ok {
		return 0, false
	}
	var unit time.Duration
	switch key {
	case "out_time_us", "out_time_ms": // both are microseconds in practice
		unit = time.Microsecond
	default:
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	frac := float64(time.Duration(n)*unit) / float64(total)
	return min(max(frac, 0), 1), true
}

// partFile names the temp file a tool writes before the rename into place.
// It keeps the extension so ffmpeg picks the right muxer.
func partFile(dst, ext string) string {
	return strings.TrimSuffix(dst, filepath.Ext(dst)) + ".part" + ext
}
