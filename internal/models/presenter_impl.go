package models

import "reflect"

// This file implements the Presenter state machine documented in presenter.go.
// Every method that changes State increments State.Seq exactly once; calls that
// change nothing (selecting the current slide, Next at the end, toggling with no
// deck, ...) leave Seq alone so renderers can skip work.

// bump records a visible change.
func (p *Presenter) bump() { p.State.Seq++ }

// copySlide returns a deep copy of s (the Background override is copied too).
func copySlide(s Slide) Slide {
	if s.Background != nil {
		bg := *s.Background
		s.Background = &bg
	}
	return s
}

// copyDeck returns a deep copy of d, or nil.
func copyDeck(d *Deck) *Deck {
	if d == nil {
		return nil
	}
	cp := *d
	if d.Slides != nil {
		cp.Slides = make([]Slide, len(d.Slides))
		for i, s := range d.Slides {
			cp.Slides[i] = copySlide(s)
		}
	}
	return &cp
}

// deckLen returns the number of slides in d (0 for nil).
func deckLen(d *Deck) int {
	if d == nil {
		return 0
	}
	return len(d.Slides)
}

// clampIndex clamps i into [0, n-1]; n must be > 0.
func clampIndex(i, n int) int { return max(0, min(i, n-1)) }

// slideAt returns a copy of slide i of d, or nil when out of range.
func slideAt(d *Deck, i int) *Slide {
	if i < 0 || i >= deckLen(d) {
		return nil
	}
	s := copySlide(d.Slides[i])
	return &s
}

// SetPreview loads a deck into Preview, selecting slide 0 (or -1 if empty).
// The deck is copied, so later changes to d do not affect the presenter.
// SetPreview(nil) clears the preview.
func (p *Presenter) SetPreview(d *Deck) {
	idx := -1
	if deckLen(d) > 0 {
		idx = 0
	}
	if p.State.PreviewIndex == idx && reflect.DeepEqual(p.State.Preview, d) {
		return
	}
	p.State.Preview = copyDeck(d)
	p.State.PreviewIndex = idx
	p.bump()
}

// SelectPreviewSlide changes the preview selection (clamped); no-op without
// a preview deck.
func (p *Presenter) SelectPreviewSlide(i int) {
	n := deckLen(p.State.Preview)
	if n == 0 {
		return
	}
	i = clampIndex(i, n)
	if i == p.State.PreviewIndex {
		return
	}
	p.State.PreviewIndex = i
	p.bump()
}

// GoLive copies the preview deck to Live, showing the selected preview slide
// (or slide 0). It does nothing when the preview is empty.
func (p *Presenter) GoLive() {
	n := deckLen(p.State.Preview)
	if n == 0 {
		return
	}
	i := p.State.PreviewIndex
	if i < 0 || i >= n {
		i = 0
	}
	p.goLive(i)
}

// GoLiveSlide is GoLive with an explicit (clamped) slide index, as when a
// preview slide is double-clicked; the preview selection moves to that slide.
func (p *Presenter) GoLiveSlide(i int) {
	n := deckLen(p.State.Preview)
	if n == 0 {
		return
	}
	p.goLive(clampIndex(i, n))
}

// goLive puts preview slide i live. Black and Logo persist; Clear and any
// other mode return to Normal.
func (p *Presenter) goLive(i int) {
	mode := p.State.Mode
	if mode != ModeBlack && mode != ModeLogo {
		mode = ModeNormal
	}
	st := &p.State
	if st.LiveIndex == i && st.PreviewIndex == i && st.Mode == mode && reflect.DeepEqual(st.Live, st.Preview) {
		return
	}
	st.Live = copyDeck(st.Preview)
	st.LiveIndex = i
	st.PreviewIndex = i
	st.Mode = mode
	p.bump()
}

// SelectLiveSlide changes the live slide within the live deck (clamped); no-op
// without a live deck. The output mode is unchanged.
func (p *Presenter) SelectLiveSlide(i int) {
	n := deckLen(p.State.Live)
	if n == 0 {
		return
	}
	i = clampIndex(i, n)
	if i == p.State.LiveIndex {
		return
	}
	p.State.LiveIndex = i
	p.bump()
}

// Next moves to the next live slide; false at the last slide or without a live deck.
func (p *Presenter) Next() bool {
	n := deckLen(p.State.Live)
	if n == 0 || p.State.LiveIndex >= n-1 {
		return false
	}
	p.SelectLiveSlide(max(p.State.LiveIndex+1, 0))
	return true
}

// Prev moves to the previous live slide; false at the first slide or without a live deck.
func (p *Presenter) Prev() bool {
	n := deckLen(p.State.Live)
	if n == 0 || p.State.LiveIndex <= 0 {
		return false
	}
	p.SelectLiveSlide(min(p.State.LiveIndex-1, n-1))
	return true
}

// First jumps to the first slide of the live deck.
func (p *Presenter) First() { p.SelectLiveSlide(0) }

// Last jumps to the last slide of the live deck.
func (p *Presenter) Last() { p.SelectLiveSlide(deckLen(p.State.Live) - 1) }

// JumpToSection moves live to the first slide whose Group == kind (and
// GroupNumber == n when n > 0); returns false if none. Returns true without a
// change when that slide is already live.
func (p *Presenter) JumpToSection(kind SectionKind, n int) bool {
	if p.State.Live == nil {
		return false
	}
	for i, s := range p.State.Live.Slides {
		if s.Group == kind && (n <= 0 || s.GroupNumber == n) {
			p.SelectLiveSlide(i)
			return true
		}
	}
	return false
}

// toggle switches to mode, or back to ModeNormal when already in it.
func (p *Presenter) toggle(mode OutputMode) {
	if p.State.Mode == mode {
		p.State.Mode = ModeNormal
	} else {
		p.State.Mode = mode
	}
	p.bump()
}

// ToggleBlack switches to ModeBlack, or back to ModeNormal if already black.
func (p *Presenter) ToggleBlack() { p.toggle(ModeBlack) }

// ToggleClear switches to ModeClear, or back to ModeNormal if already clear.
func (p *Presenter) ToggleClear() { p.toggle(ModeClear) }

// ToggleLogo switches to ModeLogo, or back to ModeNormal if already showing the logo.
func (p *Presenter) ToggleLogo() { p.toggle(ModeLogo) }

// ClearLive removes the live deck (LiveIndex -1). The output mode is unchanged.
func (p *Presenter) ClearLive() {
	if p.State.Live == nil && p.State.LiveIndex == -1 {
		return
	}
	p.State.Live = nil
	p.State.LiveIndex = -1
	p.bump()
}

// SetOutputEnabled sets the toolbar "Live" toggle (output window shown).
func (p *Presenter) SetOutputEnabled(on bool) {
	if p.State.OutputEnabled == on {
		return
	}
	p.State.OutputEnabled = on
	p.bump()
}

// SetLogo sets the background shown in ModeLogo.
func (p *Presenter) SetLogo(bg Background) {
	if p.State.Logo == bg {
		return
	}
	p.State.Logo = bg
	p.bump()
}

// RestyleDecks gives the Preview and Live decks a new theme where restyle
// says so: it is called with each deck (nil decks are skipped) and returns
// the theme to apply and true, or false to leave that deck alone. The deck's
// slides, title and item id are untouched, so it is still the same deck to
// the schedule. It reports whether any deck changed; Seq is bumped once then.
func (p *Presenter) RestyleDecks(restyle func(d *Deck) (Theme, bool)) bool {
	if restyle == nil {
		return false
	}
	changed := false
	for _, d := range []*Deck{p.State.Preview, p.State.Live} {
		if d == nil {
			continue
		}
		if th, ok := restyle(d); ok && th != d.Theme {
			d.Theme = th
			changed = true
		}
	}
	if changed {
		p.bump()
	}
	return changed
}

// RestylePreview gives the Preview deck theme t (an item restyled on its own,
// the Preview Output menu). Live is left alone, even when it shows the same
// item: the change reaches the output when the item is sent live again. The
// slides and the selection are kept. It reports whether anything changed;
// Seq is bumped then.
func (p *Presenter) RestylePreview(t Theme) bool {
	d := p.State.Preview
	if d == nil || d.Theme == t {
		return false
	}
	d.Theme = t
	p.bump()
	return true
}

// LiveSlide returns the current live slide, or nil.
// The result is a copy; changing it does not affect the presenter.
func (p *Presenter) LiveSlide() *Slide {
	return slideAt(p.State.Live, p.State.LiveIndex)
}

// LiveFrame returns what the output window should show now (Seq = State.Seq).
//
// Slide is a copy of the live slide (nil when nothing is live) and is included
// in every mode; the renderer decides what ModeBlack/ModeClear/ModeLogo draw.
// Theme is the live deck's theme (zero without a live deck).
func (p *Presenter) LiveFrame() Frame {
	f := Frame{Seq: p.State.Seq, Mode: p.State.Mode, Logo: p.State.Logo, Slide: p.LiveSlide()}
	if f.Mode == "" {
		f.Mode = ModeNormal
	}
	if p.State.Live != nil {
		f.Theme = p.State.Live.Theme
	}
	return f
}

// PreviewFrame returns the frame for the Preview Output monitor (always ModeNormal).
// Slide is a copy of the selected preview slide, or nil.
func (p *Presenter) PreviewFrame() Frame {
	f := Frame{Seq: p.State.Seq, Mode: ModeNormal, Logo: p.State.Logo,
		Slide: slideAt(p.State.Preview, p.State.PreviewIndex)}
	if p.State.Preview != nil {
		f.Theme = p.State.Preview.Theme
	}
	return f
}
