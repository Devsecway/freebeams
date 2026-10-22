package models

// StageFrame is everything the stage display (EasyWorship's Foldback output,
// the confidence monitor for musicians and the speaker) draws: text only,
// white on black, never the slide's background or video. The app builds it
// from the presenter state, the schedule, the clock and the service timer
// (app/core.BuildStageFrame); render.StageView draws it. It is comparable
// with ==, which is how the view skips unchanged frames.
type StageFrame struct {
	// Title and Label name the live item and slide ("Amazing Grace",
	// "Verse 2").
	Title string `json:"title"`
	Label string `json:"label"`
	// Current is the live slide's text (empty for media and blank slides).
	Current string `json:"current"`

	// ShowNext shows the next slide (NextLabel, Next) below the current one.
	// NextTitle is set when that slide is the first slide of the next
	// schedule item (the live slide is the last of its deck).
	ShowNext  bool   `json:"showNext"`
	Next      string `json:"next"`
	NextLabel string `json:"nextLabel"`
	NextTitle string `json:"nextTitle"`

	// Badge is "BLACK", "CLEAR" or "LOGO" while the main output shows that
	// instead of the slide ("" in normal mode): the musicians keep the text
	// and are told what the room sees.
	Badge string `json:"badge"`

	// Clock and Timer are the formatted wall clock and service timer; ""
	// hides them. TimerRunning colours the timer.
	Clock        string `json:"clock"`
	Timer        string `json:"timer"`
	TimerRunning bool   `json:"timerRunning"`

	// FontScale multiplies every text size (1 = the default sizes).
	FontScale float64 `json:"fontScale"`
}

// Stage badges (StageFrame.Badge).
const (
	StageBadgeBlack = "BLACK"
	StageBadgeClear = "CLEAR"
	StageBadgeLogo  = "LOGO"
)
