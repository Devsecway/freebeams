package models

import (
	"reflect"
	"testing"
)

// testDeck builds a deck whose slides are labelled by section, e.g.
// testDeck("song", "v1", "c", "v2", "c", "b").
func testDeck(title string, groups ...string) *Deck {
	d := &Deck{ItemID: title + "-id", Title: title, Theme: DefaultSongTheme()}
	d.Theme.Name = title
	for _, g := range groups {
		s := Slide{Kind: SlideSong, Label: g, Text: title + " " + g}
		switch g {
		case "v1":
			s.Group, s.GroupNumber = SectionVerse, 1
		case "v2":
			s.Group, s.GroupNumber = SectionVerse, 2
		case "c":
			s.Group = SectionChorus
		case "b":
			s.Group = SectionBridge
		}
		d.Slides = append(d.Slides, s)
	}
	return d
}

// step is one presenter action plus the expected state afterwards.
type step struct {
	name    string
	do      func(p *Presenter) bool // returned value is checked against ret when check is set
	ret     bool
	check   bool
	bump    bool // Seq must increase by exactly one (otherwise must not change)
	preview int
	live    int
	mode    OutputMode
}

func runSteps(t *testing.T, p *Presenter, steps []step) {
	t.Helper()
	for _, s := range steps {
		before := p.State.Seq
		got := s.do(p)
		if s.check && got != s.ret {
			t.Errorf("%s: returned %v, want %v", s.name, got, s.ret)
		}
		wantSeq := before
		if s.bump {
			wantSeq++
		}
		if p.State.Seq != wantSeq {
			t.Errorf("%s: Seq %d -> %d, want %d", s.name, before, p.State.Seq, wantSeq)
		}
		if p.State.PreviewIndex != s.preview || p.State.LiveIndex != s.live || p.State.Mode != s.mode {
			t.Errorf("%s: preview %d live %d mode %s; want %d %d %s",
				s.name, p.State.PreviewIndex, p.State.LiveIndex, p.State.Mode, s.preview, s.live, s.mode)
		}
	}
}

func act(f func(p *Presenter)) func(p *Presenter) bool {
	return func(p *Presenter) bool { f(p); return false }
}

func TestPresenterFlow(t *testing.T) {
	song := testDeck("song", "v1", "c", "v2", "c", "b")
	other := testDeck("other", "v1", "v2")
	empty := &Deck{Title: "empty"}

	p := NewPresenter()
	runSteps(t, p, []step{
		{name: "next with nothing live", do: (*Presenter).Next, check: true, ret: false, preview: -1, live: -1, mode: ModeNormal},
		{name: "prev with nothing live", do: (*Presenter).Prev, check: true, ret: false, preview: -1, live: -1, mode: ModeNormal},
		{name: "go live with empty preview", do: act((*Presenter).GoLive), preview: -1, live: -1, mode: ModeNormal},
		{name: "go live slide with empty preview", do: act(func(p *Presenter) { p.GoLiveSlide(2) }), preview: -1, live: -1, mode: ModeNormal},
		{name: "select preview without deck", do: act(func(p *Presenter) { p.SelectPreviewSlide(1) }), preview: -1, live: -1, mode: ModeNormal},
		{name: "select live without deck", do: act(func(p *Presenter) { p.SelectLiveSlide(1) }), preview: -1, live: -1, mode: ModeNormal},
		{name: "first/last without deck", do: act(func(p *Presenter) { p.First(); p.Last() }), preview: -1, live: -1, mode: ModeNormal},
		{name: "jump without deck", do: func(p *Presenter) bool { return p.JumpToSection(SectionChorus, 0) }, check: true, ret: false, preview: -1, live: -1, mode: ModeNormal},
		{name: "clear live when none", do: act((*Presenter).ClearLive), preview: -1, live: -1, mode: ModeNormal},
		{name: "set preview nil when none", do: act(func(p *Presenter) { p.SetPreview(nil) }), preview: -1, live: -1, mode: ModeNormal},

		{name: "set empty preview", do: act(func(p *Presenter) { p.SetPreview(empty) }), bump: true, preview: -1, live: -1, mode: ModeNormal},
		{name: "go live with empty deck", do: act((*Presenter).GoLive), preview: -1, live: -1, mode: ModeNormal},
		{name: "set preview", do: act(func(p *Presenter) { p.SetPreview(song) }), bump: true, preview: 0, live: -1, mode: ModeNormal},
		{name: "set same preview again", do: act(func(p *Presenter) { p.SetPreview(song) }), preview: 0, live: -1, mode: ModeNormal},
		{name: "select preview 2", do: act(func(p *Presenter) { p.SelectPreviewSlide(2) }), bump: true, preview: 2, live: -1, mode: ModeNormal},
		{name: "select preview 2 again", do: act(func(p *Presenter) { p.SelectPreviewSlide(2) }), preview: 2, live: -1, mode: ModeNormal},
		{name: "select preview clamps high", do: act(func(p *Presenter) { p.SelectPreviewSlide(99) }), bump: true, preview: 4, live: -1, mode: ModeNormal},
		{name: "select preview clamps low", do: act(func(p *Presenter) { p.SelectPreviewSlide(-3) }), bump: true, preview: 0, live: -1, mode: ModeNormal},
		{name: "select preview 1", do: act(func(p *Presenter) { p.SelectPreviewSlide(1) }), bump: true, preview: 1, live: -1, mode: ModeNormal},
		{name: "go live shows selected preview slide", do: act((*Presenter).GoLive), bump: true, preview: 1, live: 1, mode: ModeNormal},
		{name: "go live again is a no-op", do: act((*Presenter).GoLive), preview: 1, live: 1, mode: ModeNormal},
		{name: "next", do: (*Presenter).Next, check: true, ret: true, bump: true, preview: 1, live: 2, mode: ModeNormal},
		{name: "last", do: act((*Presenter).Last), bump: true, preview: 1, live: 4, mode: ModeNormal},
		{name: "next at end", do: (*Presenter).Next, check: true, ret: false, preview: 1, live: 4, mode: ModeNormal},
		{name: "last at end", do: act((*Presenter).Last), preview: 1, live: 4, mode: ModeNormal},
		{name: "prev", do: (*Presenter).Prev, check: true, ret: true, bump: true, preview: 1, live: 3, mode: ModeNormal},
		{name: "first", do: act((*Presenter).First), bump: true, preview: 1, live: 0, mode: ModeNormal},
		{name: "prev at start", do: (*Presenter).Prev, check: true, ret: false, preview: 1, live: 0, mode: ModeNormal},
		{name: "first at start", do: act((*Presenter).First), preview: 1, live: 0, mode: ModeNormal},
		{name: "select live clamps", do: act(func(p *Presenter) { p.SelectLiveSlide(50) }), bump: true, preview: 1, live: 4, mode: ModeNormal},
		{name: "select live same", do: act(func(p *Presenter) { p.SelectLiveSlide(4) }), preview: 1, live: 4, mode: ModeNormal},
		{name: "select live negative", do: act(func(p *Presenter) { p.SelectLiveSlide(-1) }), bump: true, preview: 1, live: 0, mode: ModeNormal},

		{name: "jump to chorus", do: func(p *Presenter) bool { return p.JumpToSection(SectionChorus, 0) }, check: true, ret: true, bump: true, preview: 1, live: 1, mode: ModeNormal},
		{name: "jump to chorus again", do: func(p *Presenter) bool { return p.JumpToSection(SectionChorus, 0) }, check: true, ret: true, preview: 1, live: 1, mode: ModeNormal},
		{name: "jump to verse 2", do: func(p *Presenter) bool { return p.JumpToSection(SectionVerse, 2) }, check: true, ret: true, bump: true, preview: 1, live: 2, mode: ModeNormal},
		{name: "jump to verse (any)", do: func(p *Presenter) bool { return p.JumpToSection(SectionVerse, 0) }, check: true, ret: true, bump: true, preview: 1, live: 0, mode: ModeNormal},
		{name: "jump to verse 3 missing", do: func(p *Presenter) bool { return p.JumpToSection(SectionVerse, 3) }, check: true, ret: false, preview: 1, live: 0, mode: ModeNormal},
		{name: "jump to tag missing", do: func(p *Presenter) bool { return p.JumpToSection(SectionTag, 0) }, check: true, ret: false, preview: 1, live: 0, mode: ModeNormal},
		{name: "jump to bridge", do: func(p *Presenter) bool { return p.JumpToSection(SectionBridge, 0) }, check: true, ret: true, bump: true, preview: 1, live: 4, mode: ModeNormal},

		{name: "black", do: act((*Presenter).ToggleBlack), bump: true, preview: 1, live: 4, mode: ModeBlack},
		{name: "next under black", do: act(func(p *Presenter) { p.SelectLiveSlide(3) }), bump: true, preview: 1, live: 3, mode: ModeBlack},
		{name: "new preview keeps live", do: act(func(p *Presenter) { p.SetPreview(other) }), bump: true, preview: 0, live: 3, mode: ModeBlack},
		{name: "go live keeps black", do: act((*Presenter).GoLive), bump: true, preview: 0, live: 0, mode: ModeBlack},
		{name: "black -> logo directly", do: act((*Presenter).ToggleLogo), bump: true, preview: 0, live: 0, mode: ModeLogo},
		{name: "go live slide keeps logo", do: act(func(p *Presenter) { p.GoLiveSlide(1) }), bump: true, preview: 1, live: 1, mode: ModeLogo},
		{name: "logo off", do: act((*Presenter).ToggleLogo), bump: true, preview: 1, live: 1, mode: ModeNormal},
		{name: "clear", do: act((*Presenter).ToggleClear), bump: true, preview: 1, live: 1, mode: ModeClear},
		{name: "clear persists on slide change", do: (*Presenter).Prev, check: true, ret: true, bump: true, preview: 1, live: 0, mode: ModeClear},
		{name: "go live leaves clear", do: act((*Presenter).GoLive), bump: true, preview: 1, live: 1, mode: ModeNormal},
		{name: "clear on", do: act((*Presenter).ToggleClear), bump: true, preview: 1, live: 1, mode: ModeClear},
		{name: "clear -> black directly", do: act((*Presenter).ToggleBlack), bump: true, preview: 1, live: 1, mode: ModeBlack},
		{name: "black off", do: act((*Presenter).ToggleBlack), bump: true, preview: 1, live: 1, mode: ModeNormal},
		{name: "clear on/off", do: act(func(p *Presenter) { p.ToggleClear() }), bump: true, preview: 1, live: 1, mode: ModeClear},
		{name: "clear off", do: act((*Presenter).ToggleClear), bump: true, preview: 1, live: 1, mode: ModeNormal},

		{name: "go live slide clamps", do: act(func(p *Presenter) { p.GoLiveSlide(-7) }), bump: true, preview: 0, live: 0, mode: ModeNormal},
		{name: "clear live", do: act((*Presenter).ClearLive), bump: true, preview: 0, live: -1, mode: ModeNormal},
		{name: "next after clear live", do: (*Presenter).Next, check: true, ret: false, preview: 0, live: -1, mode: ModeNormal},
		{name: "jump after clear live", do: func(p *Presenter) bool { return p.JumpToSection(SectionVerse, 0) }, check: true, ret: false, preview: 0, live: -1, mode: ModeNormal},
		{name: "clear preview", do: act(func(p *Presenter) { p.SetPreview(nil) }), bump: true, preview: -1, live: -1, mode: ModeNormal},
		{name: "go live after preview cleared", do: act((*Presenter).GoLive), preview: -1, live: -1, mode: ModeNormal},
	})
	if p.State.Preview != nil || p.State.Live != nil {
		t.Errorf("decks should be cleared: %+v", p.State)
	}
}

func TestPresenterGoLiveReplacesDeck(t *testing.T) {
	p := NewPresenter()
	a := testDeck("a", "v1", "c", "v2")
	b := testDeck("b", "v1", "c", "v2")
	p.SetPreview(a)
	p.GoLiveSlide(2)
	p.SetPreview(b)
	p.SelectPreviewSlide(2)
	seq := p.State.Seq
	p.GoLive() // same index, different deck: must change
	if p.State.Seq != seq+1 || p.State.Live.Title != "b" || p.State.LiveIndex != 2 {
		t.Errorf("go live with new deck: seq %d->%d live %+v", seq, p.State.Seq, p.State.Live)
	}
	// Preview index out of range (e.g. zero-value state) falls back to slide 0.
	p.State.PreviewIndex = 99
	p.GoLive()
	if p.State.LiveIndex != 0 {
		t.Errorf("out-of-range preview index: live %d", p.State.LiveIndex)
	}
}

func TestPresenterOutputAndLogo(t *testing.T) {
	p := NewPresenter()
	steps := []struct {
		name string
		do   func()
		bump bool
	}{
		{"enable output", func() { p.SetOutputEnabled(true) }, true},
		{"enable output again", func() { p.SetOutputEnabled(true) }, false},
		{"disable output", func() { p.SetOutputEnabled(false) }, true},
		{"set logo", func() { p.SetLogo(Background{Kind: BackgroundImage, Path: "logo.png"}) }, true},
		{"same logo", func() { p.SetLogo(Background{Kind: BackgroundImage, Path: "logo.png"}) }, false},
		{"other logo", func() { p.SetLogo(Background{Kind: BackgroundColor, Color: Hex(0x112233)}) }, true},
	}
	for _, s := range steps {
		before := p.State.Seq
		s.do()
		if got := p.State.Seq - before; (got == 1) != s.bump || got > 1 {
			t.Errorf("%s: seq delta %d, want bump %v", s.name, got, s.bump)
		}
	}
	if p.State.OutputEnabled || p.State.Logo.Color != Hex(0x112233) {
		t.Errorf("state %+v", p.State)
	}
}

func TestPresenterDefensiveCopies(t *testing.T) {
	p := NewPresenter()
	bg := Background{Kind: BackgroundColor, Color: Hex(0xFF0000)}
	d := testDeck("song", "v1", "c")
	d.Slides[0].Background = &bg
	p.SetPreview(d)

	// Mutating the caller's deck after SetPreview does not leak into state.
	d.Slides[0].Text = "changed"
	d.Slides = append(d.Slides[:1], Slide{Text: "inserted"})
	bg.Color = Hex(0x00FF00)
	d.Title = "renamed"
	if s := p.State.Preview.Slides; s[0].Text != "song v1" || s[1].Text != "song c" || s[0].Background.Color != Hex(0xFF0000) || p.State.Preview.Title != "song" {
		t.Fatalf("preview state mutated through caller: %+v", p.State.Preview)
	}

	p.GoLive()
	// Live is independent of Preview.
	p.State.Preview.Slides[0].Text = "preview edit"
	if p.State.Live.Slides[0].Text != "song v1" {
		t.Errorf("live shares preview storage")
	}

	// LiveSlide / LiveFrame return copies.
	ls := p.LiveSlide()
	ls.Text = "mutated"
	ls.Background.Color = Hex(0x0000FF)
	f := p.LiveFrame()
	f.Slide.Text = "mutated too"
	if s := p.State.Live.Slides[0]; s.Text != "song v1" || s.Background.Color != Hex(0xFF0000) {
		t.Errorf("live state mutated through returned slide: %+v", s)
	}
	pf := p.PreviewFrame()
	pf.Slide.Text = "x"
	if p.State.Preview.Slides[0].Text != "preview edit" {
		t.Errorf("preview state mutated through PreviewFrame")
	}
}

func TestPresenterFrames(t *testing.T) {
	p := NewPresenter()
	logo := Background{Kind: BackgroundImage, Path: "logo.png"}
	p.SetLogo(logo)

	f := p.LiveFrame()
	if f.Slide != nil || f.Mode != ModeNormal || f.Seq != p.State.Seq || f.Logo != logo || f.Theme != (Theme{}) {
		t.Errorf("empty live frame %+v", f)
	}
	if pf := p.PreviewFrame(); pf.Slide != nil || pf.Mode != ModeNormal || pf.Seq != p.State.Seq {
		t.Errorf("empty preview frame %+v", pf)
	}
	if p.LiveSlide() != nil {
		t.Error("LiveSlide should be nil")
	}

	song := testDeck("song", "v1", "c", "v2")
	scripture := testDeck("scripture", "a", "b")
	scripture.Theme = DefaultScriptureTheme()
	p.SetPreview(song)
	p.GoLiveSlide(1)
	p.SetPreview(scripture)
	p.SelectPreviewSlide(1)

	for _, mode := range []OutputMode{ModeNormal, ModeBlack, ModeClear, ModeLogo} {
		switch mode {
		case ModeBlack:
			p.ToggleBlack()
		case ModeClear:
			p.ToggleClear()
		case ModeLogo:
			p.ToggleLogo()
		}
		f := p.LiveFrame()
		if f.Mode != mode || f.Seq != p.State.Seq || f.Logo != logo || f.Theme != song.Theme {
			t.Errorf("%s: live frame %+v", mode, f)
		}
		if f.Slide == nil || !reflect.DeepEqual(*f.Slide, song.Slides[1]) {
			t.Errorf("%s: live frame slide %+v, want %+v", mode, f.Slide, song.Slides[1])
		}
		pf := p.PreviewFrame()
		if pf.Mode != ModeNormal || pf.Theme != scripture.Theme || pf.Slide == nil || !reflect.DeepEqual(*pf.Slide, scripture.Slides[1]) {
			t.Errorf("%s: preview frame %+v", mode, pf)
		}
	}
	if ls := p.LiveSlide(); ls == nil || ls.Text != "song c" {
		t.Errorf("LiveSlide = %+v", ls)
	}

	// Frames are comparable values; with no slide, equal state gives equal frames.
	p.ClearLive()
	if p.LiveFrame() != p.LiveFrame() {
		t.Error("identical state should give equal frames")
	}
}

func TestPresenterRestyleDecks(t *testing.T) {
	p := NewPresenter()
	song := testDeck("song", "v1", "v2")
	song.Theme.ID = 1
	scripture := testDeck("scripture", "a")
	scripture.Theme.ID = 2
	p.SetPreview(song)
	p.GoLive()
	p.SetPreview(scripture)

	byID := func(id int64, th Theme) func(*Deck) (Theme, bool) {
		return func(d *Deck) (Theme, bool) {
			if d.Theme.ID == id {
				return th, true
			}
			return Theme{}, false
		}
	}
	red := DefaultSongTheme()
	red.ID, red.Name = 1, "red"
	red.Font.Color = Hex(0xFF0000)

	tests := []struct {
		name          string
		restyle       func(*Deck) (Theme, bool)
		changed       bool
		preview, live string // theme names afterwards
	}{
		{"nil restyle", nil, false, "scripture", "song"},
		{"no deck matches", byID(99, red), false, "scripture", "song"},
		{"live deck matches", byID(1, red), true, "scripture", "red"},
		{"same theme again", byID(1, red), false, "scripture", "red"},
		{"every deck", func(d *Deck) (Theme, bool) { return red, true }, true, "red", "red"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seq := p.State.Seq
			if got := p.RestyleDecks(tc.restyle); got != tc.changed {
				t.Fatalf("RestyleDecks = %v, want %v", got, tc.changed)
			}
			if bumped := p.State.Seq != seq; bumped != tc.changed {
				t.Errorf("Seq bumped = %v, want %v", bumped, tc.changed)
			}
			if p.State.Preview.Theme.Name != tc.preview || p.State.Live.Theme.Name != tc.live {
				t.Errorf("themes = %q / %q, want %q / %q", p.State.Preview.Theme.Name, p.State.Live.Theme.Name, tc.preview, tc.live)
			}
		})
	}
	// Slides, titles and ids are untouched; the live frame carries the new theme.
	if p.State.Live.Title != "song" || len(p.State.Live.Slides) != 2 || p.State.Live.ItemID != "song-id" {
		t.Errorf("live deck changed beyond its theme: %+v", p.State.Live)
	}
	if f := p.LiveFrame(); f.Theme.Font.Color != Hex(0xFF0000) {
		t.Errorf("live frame theme = %+v", f.Theme)
	}
	// Empty decks are skipped without panicking.
	p.ClearLive()
	p.SetPreview(nil)
	if p.RestyleDecks(func(*Deck) (Theme, bool) { return red, true }) {
		t.Error("RestyleDecks changed something with no decks")
	}
}

func TestPresenterZeroValue(t *testing.T) {
	// A zero Presenter (not built with NewPresenter) must not panic.
	var p Presenter
	p.Next()
	p.Prev()
	p.First()
	p.Last()
	p.GoLive()
	p.SelectLiveSlide(3)
	if f := p.LiveFrame(); f.Mode != ModeNormal || f.Slide != nil {
		t.Errorf("zero live frame %+v", f)
	}
	p.SetPreview(testDeck("x", "v1", "v2"))
	p.GoLive()
	if p.State.LiveIndex != 0 || p.State.Mode != ModeNormal {
		t.Errorf("zero presenter go live: %+v", p.State)
	}
	if !p.Next() || p.Next() {
		t.Error("next semantics on zero presenter")
	}
}
