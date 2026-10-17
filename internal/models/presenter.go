package models

// OutputMode is what the live output shows on top of / instead of the slide.
type OutputMode string

const (
	// ModeNormal shows the live slide (background + text).
	ModeNormal OutputMode = "normal"
	// ModeBlack shows a black screen.
	ModeBlack OutputMode = "black"
	// ModeClear shows the live slide's background without text.
	ModeClear OutputMode = "clear"
	// ModeLogo shows the logo background.
	ModeLogo OutputMode = "logo"
)

// Deck is a resolved, displayable item: its slides and the theme to draw them with.
type Deck struct {
	ItemID string  `json:"itemId"` // ScheduleItem.ID, or "" for library content not in the schedule
	Title  string  `json:"title"`
	Theme  Theme   `json:"theme"`
	Slides []Slide `json:"slides"`
}

// Frame is everything the output renderer needs to draw one moment of output.
// Two Frames compare equal (==) only if nothing visible changed; Seq increases
// on every change so renderers can detect updates cheaply.
type Frame struct {
	Seq   uint64     `json:"seq"`
	Mode  OutputMode `json:"mode"`
	Slide *Slide     `json:"slide,omitempty"` // nil = nothing live (black background)
	Theme Theme      `json:"theme"`
	Logo  Background `json:"logo"`
}

// PresenterState is the single source of truth for what is in Preview and Live.
// All fields are read-only for views; mutate only through Presenter methods.
type PresenterState struct {
	Preview      *Deck      `json:"preview,omitempty"`
	PreviewIndex int        `json:"previewIndex"` // selected slide in preview; -1 when none
	Live         *Deck      `json:"live,omitempty"`
	LiveIndex    int        `json:"liveIndex"` // live slide; -1 when none
	Mode         OutputMode `json:"mode"`
	// OutputEnabled is the toolbar "Live" toggle (output window shown).
	OutputEnabled bool       `json:"outputEnabled"`
	Logo          Background `json:"logo"`
	// Seq increases on every mutation that changes what any view shows.
	Seq uint64 `json:"seq"`
}

// Presenter is the preview/live state machine. It is pure logic: no IO, no
// Godot. Every method that changes state increments State.Seq.
//
// Semantics (match EasyWorship):
//   - SetPreview loads a deck into Preview, selecting slide 0 (or -1 if empty).
//   - SelectPreviewSlide changes the preview selection only.
//   - GoLive copies the preview deck to Live, showing the selected preview slide
//     (or slide 0); Mode returns to ModeNormal unless it is ModeBlack/ModeLogo,
//     which persist until toggled off (EasyWorship keeps black/logo up while
//     you change slides underneath).
//   - GoLiveSlide(i) is GoLive with an explicit slide index (double-click).
//   - SelectLiveSlide(i) changes the live slide within the live deck.
//   - Next/Prev move the live slide; at the ends they do not wrap and return false.
//   - First/Last jump to the ends of the live deck.
//   - JumpToSection(kind, n) moves live to the first slide whose Group == kind
//     (and GroupNumber == n when n > 0); returns false if none.
//   - ToggleBlack/ToggleClear/ToggleLogo switch to that mode, or back to
//     ModeNormal if already in it. Switching between them is direct.
//   - ClearLive removes the live deck (LiveIndex -1).
//   - Out-of-range indexes are clamped; methods never panic.
type Presenter struct {
	State PresenterState
}

// NewPresenter returns a presenter with nothing in preview/live, ModeNormal.
func NewPresenter() *Presenter {
	return &Presenter{State: PresenterState{PreviewIndex: -1, LiveIndex: -1, Mode: ModeNormal}}
}
