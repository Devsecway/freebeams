package core

import (
	"testing"
	"time"

	"freebeams/internal/models"
)

// fixedWidth measures every message as 480 canvas units, so a ticker pass at
// the default 240 units/s is (1920 + 480) / 240 = 10 s.
func fixedWidth(string, models.AlertBoxStyle) float64 { return 480 }

var alertT0 = time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)

func alertAt(d time.Duration) time.Time { return alertT0.Add(d) }

func TestAlertTickerTimeline(t *testing.T) {
	st := models.DefaultAlertStyles()
	a := &Alerts{Measure: fixedWidth}
	run := a.Show(AlertRun{Text: "Car ABC123", Display: models.AlertScroll, End: models.AlertEndCount, Count: 2}, alertT0)
	if run != 1 {
		t.Fatalf("first run id = %d", run)
	}
	steps := []struct {
		at        time.Duration
		visible   bool
		x         float64
		pass      int // 1-based, in /state
		remPasses int
		remaining float64
	}{
		{0, true, 1920, 1, 2, 20},
		{5 * time.Second, true, 720, 1, 2, 15},
		{10 * time.Second, true, 1920, 2, 1, 10},
		{19 * time.Second, true, -240, 2, 1, 1},
		{20 * time.Second, false, 0, 0, 0, 0},
	}
	for _, s := range steps {
		now := alertAt(s.at)
		a.Tick(now, st)
		f := a.Frame(now, st, false)
		if f.Message.Visible != s.visible {
			t.Fatalf("at %v: visible %v, want %v", s.at, f.Message.Visible, s.visible)
		}
		if !s.visible {
			if a.Active() || len(a.State(now, st, false, nil).Active) != 0 {
				t.Errorf("at %v: the finished alert is still queued", s.at)
			}
			continue
		}
		if !f.Message.Scroll || f.Message.X != s.x || f.Message.Top {
			t.Errorf("at %v: frame %+v, want a bottom ticker at x %v", s.at, f.Message, s.x)
		}
		as := a.State(now, st, false, nil)
		r := as.Active[0]
		if r.Pass != s.pass || r.RemainingPasses != s.remPasses || r.Remaining != s.remaining || r.Style != "ticker" || !as.Moving {
			t.Errorf("at %v: state %+v, want pass %d, %d left, %vs", s.at, r, s.pass, s.remPasses, s.remaining)
		}
	}
}

func TestAlertQueueAndEnds(t *testing.T) {
	st := models.DefaultAlertStyles() // static seconds: 10
	a := &Alerts{Measure: fixedWidth}
	a.Show(AlertRun{Text: "first", Display: models.AlertStatic, End: models.AlertEndCount, Count: 1, Align: models.AlignLeft}, alertT0)
	a.Show(AlertRun{Text: "second", Display: models.AlertStatic, End: models.AlertEndDuration, Seconds: 4}, alertAt(time.Second))
	a.Show(AlertRun{Text: "third", Display: models.AlertAuto, End: models.AlertEndManual, Location: "top"}, alertAt(2*time.Second))

	check := func(d time.Duration, want string, scroll, top bool) {
		t.Helper()
		now := alertAt(d)
		a.Tick(now, st)
		f := a.Frame(now, st, false)
		if f.Message.Text != want || (want != "" && (f.Message.Scroll != scroll || f.Message.Top != top)) {
			t.Errorf("at %v: %+v, want %q scroll=%v top=%v", d, f.Message, want, scroll, top)
		}
	}
	check(9*time.Second, "first", false, false)
	check(10*time.Second, "second", false, false) // the banner's 10 s are up
	check(13*time.Second, "second", false, false)
	check(14*time.Second, "third", false, true) // 4 s later; it fits, so a banner at the top
	check(time.Hour, "third", false, true)      // until removed
	if !a.Next(alertAt(time.Hour)) {
		t.Fatal("Next")
	}
	check(time.Hour, "", false, false)
	if a.Next(alertAt(time.Hour)) {
		t.Error("Next on an empty queue")
	}
}

func TestAlertScrollWhenNeeded(t *testing.T) {
	st := models.DefaultAlertStyles()
	wide := func(string, models.AlertBoxStyle) float64 { return 1900 }
	a := &Alerts{Measure: wide}
	a.Show(AlertRun{Text: "a very long message", Display: models.AlertAuto, End: models.AlertEndManual}, alertT0)
	if f := a.Frame(alertT0, st, false); !f.Message.Scroll {
		t.Errorf("a message wider than the band does not scroll: %+v", f.Message)
	}
}

func TestAlertClearRemovesOneRun(t *testing.T) {
	st := models.DefaultAlertStyles()
	a := &Alerts{Measure: fixedWidth}
	r1 := a.Show(AlertRun{Text: "one", End: models.AlertEndManual}, alertT0)
	r2 := a.Show(AlertRun{Text: "two", End: models.AlertEndManual}, alertT0)
	if !a.ClearMessage(r1, alertAt(3*time.Second)) {
		t.Fatal("ClearMessage")
	}
	s := a.State(alertAt(3*time.Second), st, false, nil)
	if len(s.Active) != 1 || s.Active[0].Run != r2 || !s.Active[0].Showing || s.Active[0].Elapsed != 0 {
		t.Errorf("after clearing the first: %+v", s.Active)
	}
	if a.ClearMessage(99, alertT0) {
		t.Error("cleared a run that does not exist")
	}
	if !a.ClearMessages() || a.ClearMessages() {
		t.Error("ClearMessages")
	}
}

func TestCountdown(t *testing.T) {
	tests := []struct {
		name   string
		end    models.CountdownEnd
		at     time.Duration
		active bool
		text   string
	}{
		{"start", models.CountdownHide, 0, true, "05:00"},
		{"running", models.CountdownHide, 1500 * time.Millisecond, true, "04:59"},
		{"last second", models.CountdownHide, 299500 * time.Millisecond, true, "00:01"},
		{"hide at zero", models.CountdownHide, 5 * time.Minute, false, ""},
		{"hold at zero", models.CountdownHold, 6 * time.Minute, true, "00:00"},
		{"overrun", models.CountdownOverrun, 5*time.Minute + 5*time.Second, true, "-00:05"},
	}
	for _, tt := range tests {
		st := models.DefaultAlertStyles()
		st.CountdownEnd = tt.end
		a := &Alerts{}
		a.StartCountdown(CountdownRequest{Seconds: 300, Caption: "Service starts in"}, alertT0)
		now := alertAt(tt.at)
		a.Tick(now, st)
		f := a.Frame(now, st, false)
		if f.Countdown.Visible != tt.active || (tt.active && (f.Countdown.Text != tt.text || f.Countdown.Caption != "Service starts in" || f.Countdown.Position != models.PosCenter)) {
			t.Errorf("%s: %+v, want visible=%v %q", tt.name, f.Countdown, tt.active, tt.text)
		}
		if s := a.State(now, st, false, nil).Countdown; s.Active != tt.active {
			t.Errorf("%s: state %+v", tt.name, s)
		}
	}
}

func TestCountdownToTimeAndTokenEnd(t *testing.T) {
	st := models.DefaultAlertStyles()
	a := &Alerts{Measure: fixedWidth}
	a.StartCountdown(CountdownRequest{Target: alertAt(90 * time.Second), Position: models.PosBottomRight}, alertT0)
	a.Show(AlertRun{Text: "Service starts in {countdown}", Display: models.AlertStatic, End: models.AlertEndToken}, alertT0)
	f := a.Frame(alertAt(30*time.Second), st, false)
	if f.Message.Text != "Service starts in 01:00" || f.Countdown.Position != models.PosBottomRight {
		t.Errorf("frame %+v", f)
	}
	a.Tick(alertAt(90*time.Second), st)
	if a.Frame(alertAt(90*time.Second), st, false).Message.Visible || a.CountdownActive() {
		t.Error("the token alert and the countdown outlive the countdown")
	}
}

func TestNurseryAutoRemoveAndHide(t *testing.T) {
	st := models.DefaultAlertStyles()
	st.NurseryAutoRemove = 60
	a := &Alerts{}
	a.AddNursery("1234", alertT0)
	a.AddNursery("5678", alertAt(30*time.Second))
	if a.AddNursery("1234", alertT0) {
		t.Error("a duplicate code was added")
	}
	f := a.Frame(alertT0, st, false)
	if f.Nursery.Text != "1234   5678" || f.Nursery.Position != models.PosTopRight {
		t.Errorf("badge %+v", f.Nursery)
	}
	if !a.Tick(alertAt(61*time.Second), st) {
		t.Error("auto-remove did not change anything")
	}
	if got := a.NurseryCodes(); len(got) != 1 || got[0] != "5678" {
		t.Errorf("after auto-remove: %v", got)
	}
	if !Hidden(models.AlertStyles{HideOnBlack: true}, models.ModeBlack) || Hidden(st, models.ModeBlack) || Hidden(models.AlertStyles{HideOnBlack: true}, models.ModeLogo) {
		t.Error("Hidden")
	}
	if a.Frame(alertT0, st, true).Any() {
		t.Error("a hidden frame shows something")
	}
}

func TestClockAndClearAll(t *testing.T) {
	st := models.DefaultAlertStyles()
	a := &Alerts{}
	if a.Active() {
		t.Fatal("active at start")
	}
	v := a.Version()
	a.SetClock(true)
	if a.Version() == v || !a.Active() {
		t.Error("SetClock did not change the version")
	}
	if f := a.Frame(alertAt(5*time.Hour+7*time.Minute), st, false); f.Clock.Text != "3:07 PM" || f.Clock.Position != models.PosTopLeft {
		t.Errorf("clock %+v", f.Clock)
	}
	a.AddNursery("9", alertT0)
	a.StartCountdown(CountdownRequest{Seconds: 10}, alertT0)
	a.Show(AlertRun{Text: "x"}, alertT0)
	if !a.ClearAll() || a.Active() || a.ClearAll() {
		t.Error("ClearAll")
	}
}

func TestBannerFits(t *testing.T) {
	if !models.BannerFits(1000, 54) || models.BannerFits(1900, 54) {
		t.Error("BannerFits")
	}
}
