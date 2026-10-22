package ffplayer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"freebeams/internal/sysproc"
)

// Info is what the player knows about its file once it is probed.
type Info struct {
	HasVideo bool
	HasAudio bool

	// Width and Height are the presentation size: the display aspect
	// (sample aspect ratio and rotation applied), not scaled down.
	Width, Height int

	// FrameWidth and FrameHeight are the size of the RGBA frames the player
	// hands out: the presentation size scaled to fit MaxWidth × MaxHeight.
	FrameWidth, FrameHeight int

	FPS      float64 // frames per second the video is decoded at (capped)
	Duration float64 // seconds (0 when the container does not say)

	videoIndex int    // absolute stream index of the video (-map 0:N)
	audioIndex int    // absolute stream index of the audio
	fpsArg     string // the fps filter's rate, e.g. "30000/1001"
}

// maxFPS caps the decode rate: the output refreshes at 60 Hz at most.
const maxFPS = 60

// defaultFPS is used when the container reports no usable frame rate.
const defaultFPS = 30

// probeDoc is the part of `ffprobe -show_streams -show_format -of json`
// the player reads.
type probeDoc struct {
	Streams []struct {
		Index        int               `json:"index"`
		CodecType    string            `json:"codec_type"`
		Width        int               `json:"width"`
		Height       int               `json:"height"`
		AvgFrameRate string            `json:"avg_frame_rate"`
		RFrameRate   string            `json:"r_frame_rate"`
		SAR          string            `json:"sample_aspect_ratio"`
		Tags         map[string]string `json:"tags"`
		Disposition  struct {
			AttachedPic int `json:"attached_pic"`
		} `json:"disposition"`
		SideData []struct {
			Rotation *float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// probe runs ffprobe on path.
func probe(ctx context.Context, ffprobe, path string, maxW, maxH int) (Info, error) {
	cmd := sysproc.CommandContext(ctx, ffprobe, "-v", "error", "-show_streams", "-show_format", "-of", "json", path)
	var stderr tail
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return Info{}, ctx.Err()
		}
		return Info{}, fmt.Errorf("ffprobe: %s", stderr.lastLine(err))
	}
	return parseProbe(out, maxW, maxH)
}

// parseProbe reads ffprobe's JSON: the first video stream that is not cover
// art, the first audio stream, the duration, and the frame size to decode at.
func parseProbe(out []byte, maxW, maxH int) (Info, error) {
	var doc probeDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		return Info{}, fmt.Errorf("bad ffprobe output: %w", err)
	}
	info := Info{videoIndex: -1, audioIndex: -1}
	for _, s := range doc.Streams {
		switch {
		case s.CodecType == "video" && info.videoIndex < 0 && s.Disposition.AttachedPic == 0 && s.Width > 0 && s.Height > 0:
			info.videoIndex = s.Index
			info.HasVideo = true
			rot := 0
			if r, ok := s.Tags["rotate"]; ok {
				if v, err := strconv.Atoi(strings.TrimSpace(r)); err == nil {
					rot = v
				}
			}
			for _, sd := range s.SideData {
				if sd.Rotation != nil {
					rot = int(math.Round(*sd.Rotation))
				}
			}
			info.Width, info.Height = displaySize(s.Width, s.Height, parseRatio(s.SAR), rot)
			info.FrameWidth, info.FrameHeight = fitSize(info.Width, info.Height, maxW, maxH)
			info.FPS, info.fpsArg = frameRate(s.AvgFrameRate, s.RFrameRate)
		case s.CodecType == "audio" && info.audioIndex < 0:
			info.audioIndex = s.Index
			info.HasAudio = true
		}
	}
	if d := strings.TrimSpace(doc.Format.Duration); d != "" && d != "N/A" {
		if secs, err := strconv.ParseFloat(d, 64); err == nil && secs > 0 && !math.IsInf(secs, 0) {
			info.Duration = secs
		}
	}
	if !info.HasVideo && !info.HasAudio {
		return info, errors.New("no video or audio stream")
	}
	return info, nil
}

// parseRatio reads "num:den" or "num/den" (0 when it is not a ratio).
func parseRatio(s string) float64 {
	s = strings.TrimSpace(s)
	sep := strings.IndexAny(s, ":/")
	if sep < 0 {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return v
	}
	n, err1 := strconv.ParseFloat(s[:sep], 64)
	d, err2 := strconv.ParseFloat(s[sep+1:], 64)
	if err1 != nil || err2 != nil || n <= 0 || d <= 0 {
		return 0
	}
	return n / d
}

// displaySize applies the sample aspect ratio (sar <= 0: square pixels) and a
// rotation of ±90° / 270° (ffmpeg autorotates the frames) to a coded size.
func displaySize(w, h int, sar float64, rot int) (int, int) {
	if sar > 0 && math.Abs(sar-1) > 1e-6 {
		w = int(math.Round(float64(w) * sar))
	}
	rot = ((rot % 360) + 360) % 360
	if rot == 90 || rot == 270 {
		w, h = h, w
	}
	return max(w, 1), max(h, 1)
}

// fitSize scales w × h down (never up) to fit maxW × maxH, keeping the aspect.
func fitSize(w, h, maxW, maxH int) (int, int) {
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	scale := 1.0
	if maxW > 0 && w > maxW {
		scale = float64(maxW) / float64(w)
	}
	if maxH > 0 && float64(h)*scale > float64(maxH) {
		scale = float64(maxH) / float64(h)
	}
	if scale >= 1 {
		return w, h
	}
	return max(int(math.Round(float64(w)*scale)), 1), max(int(math.Round(float64(h)*scale)), 1)
}

// frameRate picks the decode rate from avg_frame_rate, else r_frame_rate,
// capped at maxFPS; it returns the rate and the fps filter's argument.
func frameRate(avg, r string) (float64, string) {
	for _, s := range []string{avg, r} {
		v := parseRatio(s)
		if v <= 0 || v > 1000 {
			continue
		}
		if v > maxFPS {
			return maxFPS, strconv.Itoa(maxFPS)
		}
		return v, strings.ReplaceAll(strings.TrimSpace(s), ":", "/")
	}
	return defaultFPS, strconv.Itoa(defaultFPS)
}

// tail keeps the last bytes a process wrote to stderr, for error messages.
type tail struct{ b []byte }

const tailMax = 2048

func (t *tail) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > tailMax {
		t.b = append(t.b[:0], t.b[len(t.b)-tailMax:]...)
	}
	return len(p), nil
}

// lastLine is the last non-empty line written, else err's text.
func (t *tail) lastLine(err error) string {
	lines := strings.Split(strings.TrimSpace(string(t.b)), "\n")
	if l := strings.TrimSpace(lines[len(lines)-1]); l != "" {
		return l
	}
	if err != nil {
		return err.Error()
	}
	return "failed"
}
