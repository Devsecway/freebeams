package ffplayer

import (
	"strings"
	"sync"
	"testing"
)

func TestParseProbe(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    Info
		wantErr string
	}{
		{
			name: "video and audio",
			json: `{"streams":[{"index":0,"codec_type":"video","width":1280,"height":720,"avg_frame_rate":"30000/1001","r_frame_rate":"30000/1001","sample_aspect_ratio":"1:1"},
				{"index":1,"codec_type":"audio"}],"format":{"duration":"12.5"}}`,
			want: Info{HasVideo: true, HasAudio: true, Width: 1280, Height: 720, FrameWidth: 1280, FrameHeight: 720, FPS: 30000.0 / 1001, Duration: 12.5,
				videoIndex: 0, audioIndex: 1, fpsArg: "30000/1001"},
		},
		{
			name: "cover art is not a video; 4K scales to 1080p; 120 fps is capped",
			json: `{"streams":[{"index":0,"codec_type":"audio"},{"index":1,"codec_type":"video","width":600,"height":600,"disposition":{"attached_pic":1}},
				{"index":2,"codec_type":"video","width":3840,"height":2160,"avg_frame_rate":"0/0","r_frame_rate":"120/1"}],"format":{"duration":"N/A"}}`,
			want: Info{HasVideo: true, HasAudio: true, Width: 3840, Height: 2160, FrameWidth: 1920, FrameHeight: 1080, FPS: 60,
				videoIndex: 2, audioIndex: 0, fpsArg: "60"},
		},
		{
			name: "audio with cover art only",
			json: `{"streams":[{"index":0,"codec_type":"video","width":500,"height":500,"disposition":{"attached_pic":1}},{"index":1,"codec_type":"audio"}],"format":{"duration":"3.0"}}`,
			want: Info{HasAudio: true, Duration: 3, videoIndex: -1, audioIndex: 1},
		},
		{
			name: "portrait phone video (rotation side data), anamorphic SAR, no frame rate",
			json: `{"streams":[{"index":0,"codec_type":"video","width":1920,"height":1080,"side_data_list":[{"rotation":-90}]},
				{"index":1,"codec_type":"video","width":720,"height":576,"sample_aspect_ratio":"16:15"}],"format":{}}`,
			want: Info{HasVideo: true, Width: 1080, Height: 1920, FrameWidth: 608, FrameHeight: 1080, FPS: 30, videoIndex: 0, audioIndex: -1, fpsArg: "30"},
		},
		{
			name: "rotate tag",
			json: `{"streams":[{"index":0,"codec_type":"video","width":720,"height":576,"sample_aspect_ratio":"16:15","tags":{"rotate":"180"},"avg_frame_rate":"25/1"}]}`,
			want: Info{HasVideo: true, Width: 768, Height: 576, FrameWidth: 768, FrameHeight: 576, FPS: 25, videoIndex: 0, audioIndex: -1, fpsArg: "25/1"},
		},
		{name: "nothing to play", json: `{"streams":[{"index":0,"codec_type":"subtitle"}]}`, wantErr: "no video or audio"},
		{name: "not JSON", json: `nope`, wantErr: "bad ffprobe output"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseProbe([]byte(tt.json), 1920, 1080)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestFitSize(t *testing.T) {
	tests := []struct{ w, h, mw, mh, ww, wh int }{
		{1920, 1080, 1920, 1080, 1920, 1080},
		{3840, 2160, 1920, 1080, 1920, 1080},
		{320, 180, 1920, 1080, 320, 180}, // never up
		{1080, 1920, 1920, 1080, 608, 1080},
		{2560, 1080, 1920, 1080, 1920, 810},
		{0, 0, 1920, 1080, 0, 0},
		{4000, 10, 1920, 1080, 1920, 5},
	}
	for _, tt := range tests {
		if w, h := fitSize(tt.w, tt.h, tt.mw, tt.mh); w != tt.ww || h != tt.wh {
			t.Errorf("fitSize(%d, %d) = %d×%d, want %d×%d", tt.w, tt.h, w, h, tt.ww, tt.wh)
		}
	}
}

func TestRing(t *testing.T) {
	r := newRing(4)
	if !r.write([]float32{1, 1, 2, 2, 3, 3}) || r.frames() != 3 {
		t.Fatal("write 3")
	}
	dst := make([]float32, 4)
	if n := r.read(dst); n != 2 || dst[0] != 1 || dst[2] != 2 {
		t.Fatalf("read %d %v", n, dst)
	}
	// Wraps around the end.
	if !r.write([]float32{4, 4, 5, 5, 6, 6}) || r.frames() != 4 {
		t.Fatal("write across the end")
	}
	dst = make([]float32, 10)
	if n := r.read(dst); n != 4 || dst[0] != 3 || dst[6] != 6 {
		t.Fatalf("read across the end: %d %v", n, dst)
	}
	// A full ring blocks the writer until a read makes room.
	r.write([]float32{1, 1, 1, 1, 1, 1, 1, 1})
	var wg sync.WaitGroup
	wg.Add(1)
	done := false
	go func() {
		defer wg.Done()
		done = r.write([]float32{9, 9})
	}()
	r.read(make([]float32, 2))
	wg.Wait()
	if !done || r.frames() != 4 {
		t.Fatal("the blocked write did not finish")
	}
	// Close wakes a blocked writer, which fails.
	wg.Add(1)
	go func() {
		defer wg.Done()
		done = r.write([]float32{7, 7})
	}()
	r.close()
	wg.Wait()
	if done {
		t.Fatal("write after close succeeded")
	}
}
