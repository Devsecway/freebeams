package core

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"freebeams/internal/models"
)

func TestThemeRestyler(t *testing.T) {
	th := models.DefaultSongTheme()
	th.ID = 7
	edited := th
	edited.Font.Size = 50
	tests := []struct {
		name  string
		deck  models.Deck
		apply bool
		want  bool
	}{
		{"library deck, apply on", models.Deck{Theme: th}, true, true},
		{"library deck, apply off", models.Deck{Theme: th}, false, true},
		{"schedule deck, apply on", models.Deck{ItemID: "a", Theme: th}, true, true},
		{"schedule deck, apply off", models.Deck{ItemID: "a", Theme: th}, false, false},
		{"other theme", models.Deck{Theme: models.Theme{ID: 8}}, true, false},
		{"custom copy", models.Deck{Theme: func() models.Theme { c := th; c.Custom = true; return c }()}, true, false},
	}
	for _, tt := range tests {
		got, ok := ThemeRestyler(edited, tt.apply)(&tt.deck)
		if ok != tt.want || (ok && got != edited) {
			t.Errorf("%s: restyled %v, want %v", tt.name, ok, tt.want)
		}
	}
}

func TestMediaTheme(t *testing.T) {
	base := models.DefaultSongTheme()
	base.ID = 3
	video := models.Media{Kind: models.MediaVideo, Path: "/m/sky.mp4", Title: "Sky", ThumbPath: "/c/sky.jpg", Aspect: models.AspectFit, Repeat: models.RepeatOnce}
	tests := []struct {
		name    string
		m       models.Media
		taken   []string
		want    models.Background
		wantNm  string
		wantErr bool
	}{
		{"video loops behind the text", video, nil,
			models.Background{Kind: models.BackgroundVideo, Color: models.RGBA{A: 1}, Path: "/m/sky.mp4", Poster: "/c/sky.jpg", Fit: models.AspectFit, Loop: true}, "Sky", false},
		{"image", models.Media{Kind: models.MediaImage, Path: "/m/a.png", Title: "A"}, []string{"a"},
			models.Background{Kind: models.BackgroundImage, Color: models.RGBA{A: 1}, Path: "/m/a.png", Fit: models.AspectFill}, "A 2", false},
		{"audio refused", models.Media{Kind: models.MediaAudio, Path: "/m/a.mp3", Title: "Tone"}, nil, models.Background{}, "", true},
	}
	for _, tt := range tests {
		got, err := MediaTheme(base, tt.m, tt.taken)
		if (err != nil) != tt.wantErr {
			t.Fatalf("%s: err %v", tt.name, err)
		}
		if tt.wantErr {
			continue
		}
		if got.Background != tt.want || got.Name != tt.wantNm || got.ID != 0 || got.Font != base.Font {
			t.Errorf("%s: got %+v %q", tt.name, got.Background, got.Name)
		}
	}
}

func TestUniqueName(t *testing.T) {
	tests := []struct {
		name  string
		taken []string
		want  string
	}{
		{"Clean Dark", nil, "Clean Dark"},
		{"Clean Dark", []string{"clean dark"}, "Clean Dark 2"},
		{"Clean Dark", []string{"Clean Dark", "Clean Dark 2"}, "Clean Dark 3"},
		{"  ", nil, "Theme"},
	}
	for _, tt := range tests {
		if got := UniqueName(tt.name, tt.taken); got != tt.want {
			t.Errorf("UniqueName(%q, %v) = %q, want %q", tt.name, tt.taken, got, tt.want)
		}
	}
}

func TestThemeFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	video := write("sky.mp4", "VIDEO")
	poster := write("sky.jpg", "POSTER")
	image := write("hills.png", "IMAGE")

	tests := []struct {
		name        string
		bg          models.Background
		wantBody    map[string]string // file base -> content after import
		wantMissing string
	}{
		{"color", models.Background{Kind: models.BackgroundGradient, Color: models.Hex(0x102030), Color2: models.Hex(0x405060)}, nil, ""},
		{"image", models.Background{Kind: models.BackgroundImage, Path: image, Fit: models.AspectFit}, map[string]string{"hills.png": "IMAGE"}, ""},
		{"video and poster", models.Background{Kind: models.BackgroundVideo, Path: video, Poster: poster, Loop: true},
			map[string]string{"sky.mp4": "VIDEO", "sky.jpg": "POSTER"}, ""},
		{"missing file", models.Background{Kind: models.BackgroundImage, Path: filepath.Join(dir, "gone.png")}, nil, filepath.Join(dir, "gone.png")},
	}
	for _, tt := range tests {
		th := models.CleanDarkTheme()
		th.ID, th.Custom, th.Name = 42, true, "Share/Me"
		th.Background = tt.bg
		dst := filepath.Join(dir, ThemeFileName(th))
		rep, err := ExportThemeFile(dst, th)
		if err != nil {
			t.Fatalf("%s: export: %v", tt.name, err)
		}
		if rep.Missing != tt.wantMissing {
			t.Errorf("%s: export missing %q, want %q", tt.name, rep.Missing, tt.wantMissing)
		}
		got, irep, err := ImportThemeFile(dst, filepath.Join(dir, "imported"))
		if err != nil {
			t.Fatalf("%s: import: %v", tt.name, err)
		}
		if got.ID != 0 || got.Custom || got.Name != "Share/Me" {
			t.Errorf("%s: imported id %d custom %v name %q", tt.name, got.ID, got.Custom, got.Name)
		}
		if irep.Missing != tt.wantMissing {
			t.Errorf("%s: import missing %q", tt.name, irep.Missing)
		}
		want := th
		want.ID, want.Custom = 0, false
		want.Background.Path, want.Background.Poster = got.Background.Path, got.Background.Poster
		if !got.SameLook(want) {
			t.Errorf("%s: look changed:\n got %+v\nwant %+v", tt.name, got, want)
		}
		for _, p := range []string{got.Background.Path, got.Background.Poster} {
			if p == "" || p == tt.wantMissing {
				continue
			}
			body, err := os.ReadFile(p)
			if err != nil || string(body) != tt.wantBody[filepath.Base(p)] {
				t.Errorf("%s: %s = %q, %v", tt.name, p, body, err)
			}
			if !strings.HasPrefix(p, filepath.Join(dir, "imported")) {
				t.Errorf("%s: %s is outside the media dir", tt.name, p)
			}
		}
		if tt.wantMissing != "" && got.Background.Path != tt.wantMissing {
			t.Errorf("%s: a missing file keeps its path, got %q", tt.name, got.Background.Path)
		}
	}
}

func TestImportThemeFileRefuses(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string, files map[string]string) string {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for n, body := range files {
			w, _ := zw.Create(n)
			w.Write([]byte(body))
		}
		zw.Close()
		p := filepath.Join(dir, name)
		os.WriteFile(p, buf.Bytes(), 0o644)
		return p
	}
	notZip := filepath.Join(dir, "x.fbtheme")
	os.WriteFile(notZip, []byte("hello"), 0o644)
	tests := []struct {
		name, path, want string
	}{
		{"not a zip", notZip, "not a theme file"},
		{"no manifest", mk("a.fbtheme", map[string]string{"x": "y"}), "theme.json is missing"},
		{"other format", mk("b.fbtheme", map[string]string{"theme.json": `{"format":"other"}`}), "not a FreeBeams theme"},
		{"newer", mk("c.fbtheme", map[string]string{"theme.json": `{"format":"freebeams-theme","version":9}`}), "newer FreeBeams"},
		{"escaping path", mk("d.fbtheme", map[string]string{"theme.json": `{"format":"freebeams-theme","version":1,"theme":{"name":"x"},"background":"../evil"}`}), "not in the file"},
	}
	for _, tt := range tests {
		_, _, err := ImportThemeFile(tt.path, filepath.Join(dir, "media"))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestFrameMediaThemeBackground(t *testing.T) {
	motion := models.DefaultSongTheme()
	motion.Background = models.Background{Kind: models.BackgroundVideo, Path: "/sky.mp4", Loop: true}
	text := &models.Slide{Kind: models.SlideSong, Text: "Amazing grace"}
	still := &models.Background{Kind: models.BackgroundImage, Path: "/i.jpg"}
	tests := []struct {
		name string
		f    models.Frame
		want string
	}{
		{"a theme video plays behind the text", models.Frame{Mode: models.ModeNormal, Slide: text, Theme: motion}, "/sky.mp4"},
		{"clear keeps it", models.Frame{Mode: models.ModeClear, Slide: text, Theme: motion}, "/sky.mp4"},
		{"black stops it", models.Frame{Mode: models.ModeBlack, Slide: text, Theme: motion}, ""},
		{"a slide's own still wins", models.Frame{Mode: models.ModeNormal, Slide: &models.Slide{Text: "x", Background: still}, Theme: motion}, ""},
		{"a gradient theme plays nothing", models.Frame{Mode: models.ModeNormal, Slide: text, Theme: models.DefaultSongTheme()}, ""},
	}
	for _, tt := range tests {
		bg, ok := FrameMedia(tt.f)
		if bg.Path != tt.want || ok != (tt.want != "") {
			t.Errorf("%s: FrameMedia = %q, %v", tt.name, bg.Path, ok)
		}
		if ok && !bg.Loop {
			t.Errorf("%s: the theme's loop was lost", tt.name)
		}
	}
}
