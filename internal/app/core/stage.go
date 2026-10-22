package core

import (
	"fmt"
	"math"
	"strings"
	"time"

	"freebeams/internal/models"
)

// The stage display (EasyWorship's Foldback output): a confidence monitor for
// the musicians and the speaker that shows the live slide's text, the next
// slide, a clock and a service timer. This file is its pure state and
// projection; internal/app/stage.go wires it, render.Stage and
// render.StageView draw it.

// SettingStage is the stage display's settings key (one JSON object).
const SettingStage = "stage.settings"

// Font scale bounds of the stage display (Options ▸ Stage Display).
const (
	StageFontScaleMin = 0.5
	StageFontScaleMax = 2.0
)

// StageSettings is the Options dialog's Stage Display section.
type StageSettings struct {
	// Enabled shows the stage display (EasyWorship: "Enable support for
	// Foldback Output"). The Live menu and `stage {on}` change it too.
	Enabled bool `json:"enabled"`
	// Screen is AutoScreen (the first non-primary display the output does not
	// use, else a window on the primary) or a display index.
	Screen    int     `json:"screen"`
	ShowNext  bool    `json:"showNext"`
	ShowClock bool    `json:"showClock"`
	Clock24h  bool    `json:"clock24h"`
	ShowTimer bool    `json:"showTimer"`
	FontScale float64 `json:"fontScale"`
}

// DefaultStageSettings: off, automatic display, next slide, 12-hour clock and
// timer shown, normal text size.
func DefaultStageSettings() StageSettings {
	return StageSettings{Screen: AutoScreen, ShowNext: true, ShowClock: true, ShowTimer: true, FontScale: 1}
}

// Normalized folds a negative screen to AutoScreen and clamps the font scale
// (0 or an invalid value is 1).
func (s StageSettings) Normalized() StageSettings {
	if s.Screen < 0 {
		s.Screen = AutoScreen
	}
	switch {
	case !(s.FontScale > 0) || math.IsInf(s.FontScale, 0):
		s.FontScale = 1
	case s.FontScale < StageFontScaleMin:
		s.FontScale = StageFontScaleMin
	case s.FontScale > StageFontScaleMax:
		s.FontScale = StageFontScaleMax
	}
	s.FontScale = math.Round(s.FontScale*100) / 100
	return s
}

// ---- display ---------------------------------------------------------------------

// StageScreenFor resolves the stage display's screen against the connected
// displays (n of them, primary the primary's index) and the output's screen
// (-1 when the output is off). An explicit, connected display wins unless the
// output covers it (a non-primary display holds one fullscreen window);
// Automatic takes the first non-primary display the output does not use. The
// fallback is a window on the primary display. conflict reports that the
// configured display was given up because the output is on it.
func StageScreenFor(setting, n, primary, output int) (screen int, conflict bool) {
	primary = max(primary, 0)
	if setting >= 0 && setting < n {
		if setting == output && setting != primary {
			return primary, true
		}
		return setting, false
	}
	for i := 0; i < n; i++ {
		if i != primary && i != output {
			return i, false
		}
	}
	return primary, false
}

// StageConflict reports whether the stage display and the output would both
// cover the same display: the same screen, and not the primary one (on the
// primary both are ordinary windows). The Options dialog and `stage {screen}`
// refuse such a choice.
func StageConflict(stageScreen, outputScreen, primary int) bool {
	return stageScreen >= 0 && stageScreen == outputScreen && stageScreen != max(primary, 0)
}

// StageConflictMessage explains a refused display choice.
func StageConflictMessage(screen int) string {
	return fmt.Sprintf("The stage display and the output cannot share Display %d. Choose another display for one of them.", screen+1)
}

// ---- service timer ---------------------------------------------------------------

// StageTimer is the service timer: elapsed time since it was started, with
// stop (pause) and reset. It takes the time as an argument so it is testable.
type StageTimer struct {
	Running bool
	since   time.Time     // when the current run started
	acc     time.Duration // elapsed before the current run
}

// Start runs the timer (no-op when running).
func (t *StageTimer) Start(now time.Time) {
	if t.Running {
		return
	}
	t.Running, t.since = true, now
}

// Stop pauses the timer, keeping the elapsed time (no-op when stopped).
func (t *StageTimer) Stop(now time.Time) {
	if !t.Running {
		return
	}
	t.acc += max(0, now.Sub(t.since))
	t.Running = false
}

// Reset sets the elapsed time to zero. A running timer keeps running from now.
func (t *StageTimer) Reset(now time.Time) {
	t.acc, t.since = 0, now
}

// Elapsed is the time on the timer at now.
func (t StageTimer) Elapsed(now time.Time) time.Duration {
	if !t.Running {
		return t.acc
	}
	return t.acc + max(0, now.Sub(t.since))
}

// Apply runs a timer action ("start", "stop", "reset"; "toggle" starts a
// stopped timer and stops a running one).
func (t *StageTimer) Apply(action string, now time.Time) error {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "start":
		t.Start(now)
	case "stop", "pause":
		t.Stop(now)
	case "reset":
		t.Reset(now)
	case "toggle":
		if t.Running {
			t.Stop(now)
		} else {
			t.Start(now)
		}
	default:
		return fmt.Errorf("unknown timer action %q (start|stop|reset|toggle)", action)
	}
	return nil
}

// FormatClock formats the wall clock: "14:05" (24-hour) or "2:05 PM".
func FormatClock(t time.Time, h24 bool) string {
	if h24 {
		return t.Format("15:04")
	}
	return t.Format("3:04 PM")
}

// FormatTimer formats an elapsed time: "MM:SS" under an hour, "H:MM:SS" from
// then on. Negative durations are 00:00; fractions of a second are dropped.
func FormatTimer(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int64(d / time.Second)
	h, m, sec := s/3600, (s/60)%60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%02d:%02d", m, sec)
}

// ---- projection ------------------------------------------------------------------

// StageNext is what follows the live slide on the stage display.
type StageNext struct {
	Slide *models.Slide // nil when nothing follows
	// Title is the next schedule item's title when Slide is that item's first
	// slide (the live slide is the last of its deck); "" within the deck.
	Title string
	// Item is the next item's schedule index (-1 within the deck or when
	// nothing follows).
	Item int
}

// NextStageSlide resolves the slide after the live one: the next slide of the
// live deck, else the first slide of the next schedule item (when the live
// deck is a schedule item that is not the last). Items without slides are
// skipped. A live deck with no slide selected starts at its first slide.
func NextStageSlide(sched *models.Schedule, live *models.Deck, index int) StageNext {
	none := StageNext{Item: -1}
	if live == nil {
		return none
	}
	if index < 0 {
		index = -1
	}
	if index+1 < len(live.Slides) {
		s := live.Slides[index+1]
		return StageNext{Slide: &s, Item: -1}
	}
	k := LiveItemIndex(sched, live)
	if k < 0 {
		return none
	}
	for j := k + 1; j < len(sched.Items); j++ {
		it := sched.Items[j]
		if len(it.Slides) == 0 {
			continue
		}
		s := it.Slides[0]
		return StageNext{Slide: &s, Title: it.Title, Item: j}
	}
	return none
}

// StageInput is what BuildStageFrame reads.
type StageInput struct {
	Presenter models.PresenterState
	Schedule  *models.Schedule
	Settings  StageSettings
	Timer     StageTimer
	Now       time.Time
}

// stageText is a slide's text as the stage shows it: the body, trimmed, with
// Windows line ends folded.
func stageText(s *models.Slide) string {
	if s == nil {
		return ""
	}
	t := strings.ReplaceAll(s.Text, "\r\n", "\n")
	return strings.TrimSpace(strings.ReplaceAll(t, "\r", "\n"))
}

// StageBadge is the badge for an output mode ("" in normal mode).
func StageBadge(mode models.OutputMode) string {
	switch mode {
	case models.ModeBlack:
		return models.StageBadgeBlack
	case models.ModeClear:
		return models.StageBadgeClear
	case models.ModeLogo:
		return models.StageBadgeLogo
	}
	return ""
}

// BuildStageFrame projects the state onto the stage display. The text stays
// up whatever the output mode (the musicians keep playing during Black); the
// badge says what the room sees.
func BuildStageFrame(in StageInput) models.StageFrame {
	st := in.Settings.Normalized()
	p := in.Presenter
	f := models.StageFrame{FontScale: st.FontScale, Badge: StageBadge(p.Mode), ShowNext: st.ShowNext}
	if d := p.Live; d != nil {
		f.Title = d.Title
		if i := p.LiveIndex; i >= 0 && i < len(d.Slides) {
			s := d.Slides[i]
			f.Label, f.Current = s.Label, stageText(&s)
		}
		if st.ShowNext {
			n := NextStageSlide(in.Schedule, d, p.LiveIndex)
			if n.Slide != nil {
				f.Next, f.NextLabel, f.NextTitle = stageText(n.Slide), n.Slide.Label, n.Title
			}
		}
	}
	if st.ShowClock {
		f.Clock = FormatClock(in.Now, st.Clock24h)
	}
	if st.ShowTimer {
		f.Timer = FormatTimer(in.Timer.Elapsed(in.Now))
		f.TimerRunning = in.Timer.Running
	}
	return f
}

// StageState is /state.stage.
type StageState struct {
	Settings   StageSettings `json:"settings"`
	Open       bool          `json:"open"`       // the stage window exists
	Fullscreen bool          `json:"fullscreen"` // it covers a secondary display
	Screen     int           `json:"screen"`     // its display (-1 when closed)
	Focused    bool          `json:"focused"`    // it has keyboard focus
	Conflict   bool          `json:"conflict"`   // the configured display is the output's (it fell back to a window)
	Monitor    string        `json:"monitor"`    // what the Live Output monitor shows: "live" or "foldback"

	Title        string  `json:"title"`
	Label        string  `json:"label"`
	Current      string  `json:"current"`
	Next         string  `json:"next"`
	NextLabel    string  `json:"nextLabel"`
	NextTitle    string  `json:"nextTitle"`
	NextHeader   string  `json:"nextHeader"`
	Badge        string  `json:"badge"`
	Clock        string  `json:"clock"`
	Timer        string  `json:"timer"`
	TimerRunning bool    `json:"timerRunning"`
	TimerSeconds float64 `json:"timerSeconds"`
}

// StageWindow is the stage window's state for StageStateDoc.
type StageWindow struct {
	Open, Fullscreen, Focused, Conflict, Monitor bool
	Screen                                       int
}

// StageStateDoc builds /state.stage from the frame and the window.
func StageStateDoc(f models.StageFrame, st StageSettings, timer StageTimer, now time.Time, w StageWindow) StageState {
	doc := StageState{
		Settings: st, Open: w.Open, Fullscreen: w.Fullscreen, Screen: w.Screen, Focused: w.Focused, Conflict: w.Conflict,
		Monitor: "live",
		Title:   f.Title, Label: f.Label, Current: f.Current,
		Next: f.Next, NextLabel: f.NextLabel, NextTitle: f.NextTitle, NextHeader: f.NextHeader(),
		Badge: f.Badge, Clock: f.Clock, Timer: f.Timer, TimerRunning: timer.Running,
		TimerSeconds: math.Floor(timer.Elapsed(now).Seconds()),
	}
	if w.Monitor {
		doc.Monitor = "foldback"
	}
	if !w.Open {
		doc.Screen = -1
	}
	return doc
}
