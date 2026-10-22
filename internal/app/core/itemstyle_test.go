package core

import (
	"errors"
	"testing"

	"freebeams/internal/models"
)

func ptr[T any](v T) *T { return &v }

func TestRestyleTheme(t *testing.T) {
	base := namedTheme(4, "Default Song") // gradient, white bold text, outline and shadow on, centered
	blue := models.Hex(0x1F3F9F)
	yellow := models.Hex(0xFFD400)
	solid := SolidBackground(blue)
	image := models.Background{Kind: models.BackgroundImage, Color: models.RGBA{A: 1}, Path: "/m/bg.png"}
	custom := RestyleTheme(base, base, StyleEdit{Background: &solid, FontColor: &yellow})

	cases := []struct {
		name   string
		from   models.Theme
		edit   StyleEdit
		check  func(t models.Theme) bool
		custom bool
	}{
		{"no change keeps the saved look", base, StyleEdit{}, func(t models.Theme) bool { return t == base }, false},
		{"solid background", base, StyleEdit{Background: &solid},
			func(t models.Theme) bool { return t.Background == solid && t.Font == base.Font }, true},
		{"library image", base, StyleEdit{Background: &image},
			func(t models.Theme) bool {
				return t.Background.Kind == models.BackgroundImage && t.Background.Path == "/m/bg.png"
			}, true},
		{"text color", base, StyleEdit{FontColor: &yellow},
			func(t models.Theme) bool { return t.Font.Color == yellow && t.Background == base.Background }, true},
		{"reset the background keeps the text change", custom, StyleEdit{ResetBackground: true},
			func(t models.Theme) bool { return t.Background == base.Background && t.Font.Color == yellow }, true},
		{"reset both is the saved look again", custom, StyleEdit{ResetBackground: true, ResetText: true},
			func(t models.Theme) bool { return t.SameLook(base) && t.ID == 4 && t.Name == "Default Song" }, false},
		{"reset the text keeps the background", custom, StyleEdit{ResetText: true},
			func(t models.Theme) bool { return t.Background == solid && t.Font == base.Font }, true},
		{"increase font scales the auto-fit floor", base, StyleEdit{FontStep: +1},
			func(t models.Theme) bool { return t.Font.Size == 104 && t.Font.MinSize == 39 }, true},
		{"decrease font", base, StyleEdit{FontStep: -1},
			func(t models.Theme) bool { return t.Font.Size == 88 && t.Font.MinSize == 33 }, true},
		{"a size value", base, StyleEdit{FontSize: ptr[float32](140)},
			func(t models.Theme) bool { return t.Font.Size == 140 && t.Font.MinSize == 53 }, true},
		{"the size is clamped", base, StyleEdit{FontSize: ptr[float32](4)},
			func(t models.Theme) bool { return t.Font.Size == MinFontSize && t.Font.MinSize <= t.Font.Size }, true},
		{"bold off, italic on", base, StyleEdit{Bold: ptr(false), Italic: ptr(true)},
			func(t models.Theme) bool { return !t.Font.Bold && t.Font.Italic }, true},
		{"outline and shadow off", base, StyleEdit{Outline: ptr(false), Shadow: ptr(false)},
			func(t models.Theme) bool { return !t.Outline.Enabled && !t.Shadow.Enabled }, true},
		{"align left", base, StyleEdit{Align: ptr(models.AlignLeft)},
			func(t models.Theme) bool { return t.HAlign == models.AlignLeft }, true},
		{"font family", base, StyleEdit{FontFamily: ptr(" Georgia ")},
			func(t models.Theme) bool { return t.Font.Family == "Georgia" }, true},
	}
	for _, tc := range cases {
		got := RestyleTheme(tc.from, base, tc.edit)
		if !tc.check(got) {
			t.Errorf("%s: got %+v", tc.name, got)
		}
		if got.Custom != tc.custom {
			t.Errorf("%s: Custom = %v, want %v", tc.name, got.Custom, tc.custom)
		}
		if got.ID != tc.from.ID || got.Name != tc.from.Name {
			t.Errorf("%s: identity changed to %d %q", tc.name, got.ID, got.Name)
		}
	}

	// An outline / shadow switched on without a size of its own gets the defaults.
	bare := base
	bare.Outline, bare.Shadow = models.Outline{}, models.Shadow{}
	on := RestyleTheme(bare, bare, StyleEdit{Outline: ptr(true), Shadow: ptr(true)})
	def := models.DefaultSongTheme()
	if !on.Outline.Enabled || on.Outline.Size != def.Outline.Size || on.Shadow != def.Shadow {
		t.Errorf("outline/shadow on from nothing = %+v / %+v", on.Outline, on.Shadow)
	}
}

func TestBaseThemeAndCustomize(t *testing.T) {
	list := []models.Theme{namedTheme(1, "Default Song"), namedTheme(2, "Sunday")}
	def := namedTheme(1, "Default Song")
	item := list[1]
	item.Background = SolidBackground(models.Hex(0x102030))
	item.Custom = true
	if got := BaseTheme(item, list, def); got != list[1] {
		t.Errorf("BaseTheme = %q, want the saved Sunday", got.Name)
	}
	gone := item
	gone.ID = 99
	if got := BaseTheme(gone, list, def); got != def {
		t.Errorf("BaseTheme of a deleted theme = %q, want the default", got.Name)
	}
	if c := Customize(list[1], list[1]); c.Custom {
		t.Error("Customize marked an unchanged copy")
	}
	if c := Customize(item, list[1]); !c.Custom || c.Name != "Sunday" {
		t.Errorf("Customize = %+v", c)
	}
}

func TestRestyleSkipsCustomCopies(t *testing.T) {
	saved := namedTheme(5, "Sunday")
	custom := RestyleTheme(saved, saved, StyleEdit{FontColor: ptr(models.Hex(0xFFD400))})
	s := models.NewSchedule()
	s.Add(models.ScheduleItem{Title: "plain", Theme: saved})
	s.Add(models.ScheduleItem{Title: "custom", Theme: custom})
	s.Dirty = false
	edited := saved
	edited.Background = SolidBackground(models.Hex(0x00FF00))
	if n := RestyleSchedule(s, edited); n != 1 || s.Items[0].Theme != edited || s.Items[1].Theme != custom {
		t.Errorf("RestyleSchedule = %d: %+v", n, s.Items)
	}
	if _, ok := RestyleByThemeID(edited)(&models.Deck{Theme: custom}); ok {
		t.Error("RestyleByThemeID restyled a custom deck")
	}
	if _, ok := RestyleByThemeID(edited)(&models.Deck{Theme: saved}); !ok {
		t.Error("RestyleByThemeID skipped a plain deck")
	}
}

func TestRestylableAndDeckKind(t *testing.T) {
	song := SongDeck(amazingGrace())
	media := MediaDeck(models.Media{Kind: models.MediaImage, Path: "/a.png", Title: "a"}, nil)
	pres := models.Deck{Slides: PresentationSlides([]string{"/p1.png"})}
	cases := []struct {
		name string
		d    *models.Deck
		ok   bool
		kind models.ItemKind
	}{
		{"nil", nil, false, ""},
		{"song", &song, true, models.ItemSong},
		{"scripture", &models.Deck{Slides: []models.Slide{{Kind: models.SlideScripture, Text: "For God"}}}, true, models.ItemScripture},
		{"blank", &models.Deck{Slides: []models.Slide{{Kind: models.SlideBlank}}}, true, ""},
		{"media", &media, false, ""},
		{"presentation", &pres, false, ""},
	}
	for _, tc := range cases {
		if got := Restylable(tc.d); got != tc.ok {
			t.Errorf("%s: Restylable = %v", tc.name, got)
		}
		if got := DeckKind(tc.d); got != tc.kind {
			t.Errorf("%s: DeckKind = %q", tc.name, got)
		}
	}
}

func TestMediaBackground(t *testing.T) {
	img := models.Media{Kind: models.MediaImage, Path: "/m/a.png", Aspect: models.AspectFit}
	bg, err := MediaBackground(img)
	if err != nil || bg.Kind != models.BackgroundImage || bg.Path != "/m/a.png" || bg.Fit != models.AspectFit {
		t.Errorf("image = %+v, %v", bg, err)
	}
	vid := models.Media{Kind: models.MediaVideo, Path: "/m/a.mp4", ThumbPath: "/c/a.jpg", Repeat: models.RepeatOnce}
	bg, err = MediaBackground(vid)
	if err != nil || bg.Kind != models.BackgroundVideo || !bg.Loop || bg.Poster != "/c/a.jpg" {
		t.Errorf("video = %+v, %v", bg, err)
	}
	if _, err := MediaBackground(models.Media{Kind: models.MediaAudio, Path: "/m/a.mp3"}); !errors.Is(err, ErrAudioBackground) {
		t.Errorf("audio: %v", err)
	}
	list := []models.Media{{Title: "a", Kind: models.MediaAudio}, {Title: "b", Kind: models.MediaImage}, {Title: "c", Kind: models.MediaVideo}}
	if got := BackgroundMedia(list); len(got) != 2 || got[0].Title != "b" || got[1].Title != "c" {
		t.Errorf("BackgroundMedia = %+v", got)
	}
}

func TestCarryStyle(t *testing.T) {
	song := SongDeck(amazingGrace())
	restyled := song
	restyled.Theme = RestyleTheme(song.Theme, song.Theme, StyleEdit{FontColor: ptr(models.Hex(0xFFD400))})
	if got := CarryStyle(&restyled, song); got.Theme != restyled.Theme {
		t.Error("the override did not carry to a fresh deck of the same song")
	}
	other := song
	other.Title = "Other"
	if got := CarryStyle(&restyled, other); got.Theme != other.Theme {
		t.Error("the override carried to another item")
	}
	if got := CarryStyle(&song, song); got.Theme != song.Theme {
		t.Error("a plain deck changed the fresh one")
	}
	if got := CarryStyle(nil, song); got.Theme != song.Theme {
		t.Error("nil old deck changed the fresh one")
	}
	if !SameContent(&restyled, &song) || SameContent(&restyled, &other) || !SameContent(nil, nil) || SameContent(&song, nil) {
		t.Error("SameContent")
	}
}

func TestHexColors(t *testing.T) {
	cases := []struct {
		in   string
		want models.RGBA
		hex  string
		bad  bool
	}{
		{in: "#1f3f9f", want: models.Hex(0x1F3F9F), hex: "#1f3f9f"},
		{in: "FFD400", want: models.Hex(0xFFD400), hex: "#ffd400"},
		{in: "#00000080", want: models.RGBA{A: 128.0 / 255}, hex: "#00000080"},
		{in: "#12345", bad: true},
		{in: "red", bad: true},
		{in: "#zzzzzz", bad: true},
	}
	for _, tc := range cases {
		c, err := ParseHexColor(tc.in)
		if tc.bad {
			if err == nil {
				t.Errorf("%q: no error", tc.in)
			}
			continue
		}
		if err != nil || c != tc.want {
			t.Errorf("%q = %+v, %v", tc.in, c, err)
		}
		if got := HexColor(c); got != tc.hex {
			t.Errorf("HexColor(%q) = %q, want %q", tc.in, got, tc.hex)
		}
	}
}

func TestStylePatchAndState(t *testing.T) {
	base := namedTheme(4, "Default Song")
	e, err := StylePatch{BackgroundColor: "#1f3f9f", Color: "#ffd400", Size: ptr[float32](120), Align: "Left", Bold: ptr(false)}.Edit()
	if err != nil {
		t.Fatal(err)
	}
	got := RestyleTheme(base, base, e)
	st := StyleOf(got)
	want := DeckStyle{Custom: true, Background: "color", BackgroundColor: "#1f3f9f", Size: 120, MinSize: 45, Color: "#ffd400",
		Bold: false, Italic: false, Outline: true, Shadow: true, Align: "left"}
	if st != want {
		t.Errorf("StyleOf = %+v\nwant %+v", st, want)
	}
	for _, p := range []StylePatch{{BackgroundColor: "blue"}, {Color: "#1234"}, {Align: "justify"}} {
		if _, err := p.Edit(); err == nil {
			t.Errorf("%+v: no error", p)
		}
	}
	d := deckState(&models.Deck{Title: "x", Theme: got, Slides: []models.Slide{{Text: "a"}}}, 0)
	if d.Style == nil || !d.Style.Custom || d.Theme != "Default Song" {
		t.Errorf("deckState = %+v", d)
	}
	if deckState(nil, -1).Style != nil {
		t.Error("deckState(nil) has a style")
	}
}
