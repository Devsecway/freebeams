package core

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"freebeams/internal/models"
)

func slides(labels ...string) []models.Slide {
	out := make([]models.Slide, len(labels))
	for i, l := range labels {
		out[i] = models.Slide{Kind: models.SlideSong, Label: l, Text: l + " text"}
	}
	return out
}

// stageSchedule: Amazing Grace (V1, C, V2), an empty item, John 3:16 (one
// verse), How Great (V1).
func stageSchedule() *models.Schedule {
	s := models.NewSchedule()
	s.Add(models.ScheduleItem{ID: "a", Title: "Amazing Grace", Slides: slides("Verse 1", "Chorus", "Verse 2")})
	s.Add(models.ScheduleItem{ID: "e", Title: "Empty"})
	s.Add(models.ScheduleItem{ID: "j", Title: "John 3:16", Slides: slides("John 3:16")})
	s.Add(models.ScheduleItem{ID: "h", Title: "How Great", Slides: slides("Verse 1")})
	return s
}

func TestNextStageSlide(t *testing.T) {
	sched := stageSchedule()
	deck := func(id string) *models.Deck { return ItemDeck(sched.Items[sched.IndexOf(id)]) }
	library := &models.Deck{Title: "Library song", Slides: slides("Verse 1", "Verse 2")}
	tests := []struct {
		name      string
		sched     *models.Schedule
		live      *models.Deck
		index     int
		wantLabel string // "" = no next slide
		wantTitle string
		wantItem  int
	}{
		{"nothing live", sched, nil, -1, "", "", -1},
		{"within the deck", sched, deck("a"), 0, "Chorus", "", -1},
		{"second to last", sched, deck("a"), 1, "Verse 2", "", -1},
		{"no slide selected starts at the first", sched, deck("a"), -1, "Verse 1", "", -1},
		{"last slide: the next item, skipping an empty one", sched, deck("a"), 2, "John 3:16", "John 3:16", 2},
		{"one-slide item: the next item", sched, deck("j"), 0, "Verse 1", "How Great", 3},
		{"last item, last slide", sched, deck("h"), 0, "", "", -1},
		{"library content: no item follows", sched, library, 1, "", "", -1},
		{"library content within its deck", sched, library, 0, "Verse 2", "", -1},
		{"nil schedule", nil, deck("a"), 2, "", "", -1},
		{"index past the end", sched, deck("a"), 7, "John 3:16", "John 3:16", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextStageSlide(tt.sched, tt.live, tt.index)
			label := ""
			if got.Slide != nil {
				label = got.Slide.Label
				if got.Slide.Text != label+" text" {
					t.Errorf("text = %q", got.Slide.Text)
				}
			}
			if label != tt.wantLabel || got.Title != tt.wantTitle || got.Item != tt.wantItem {
				t.Fatalf("got label %q title %q item %d; want %q %q %d", label, got.Title, got.Item, tt.wantLabel, tt.wantTitle, tt.wantItem)
			}
		})
	}
	// The next slide is a copy: the stage cannot change the schedule.
	n := NextStageSlide(sched, deck("a"), 2)
	n.Slide.Text = "changed"
	if sched.Items[2].Slides[0].Text != "John 3:16 text" {
		t.Error("NextStageSlide returned a slide aliasing the schedule")
	}
}

func TestStageTimer(t *testing.T) {
	t0 := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	at := func(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }
	var tm StageTimer
	steps := []struct {
		action string
		at     float64
		check  float64 // the time to read Elapsed at
		want   float64 // seconds
		run    bool
	}{
		{"", 0, 5, 0, false},          // never started
		{"start", 0, 10, 10, true},    // runs
		{"start", 4, 10, 10, true},    // a second start does not restart
		{"stop", 15, 100, 15, false},  // stopped keeps its time
		{"stop", 20, 100, 15, false},  // a second stop is harmless
		{"start", 100, 130, 45, true}, // resumes from 15 s
		{"reset", 130, 131, 1, true},  // reset while running: from 0, still running
		{"stop", 140, 200, 10, false},
		{"reset", 200, 300, 0, false}, // reset while stopped
		{"toggle", 300, 302, 2, true},
		{"toggle", 302, 400, 2, false},
	}
	for i, s := range steps {
		if s.action != "" {
			if err := tm.Apply(s.action, at(s.at)); err != nil {
				t.Fatalf("step %d: %v", i, err)
			}
		}
		if got := tm.Elapsed(at(s.check)).Seconds(); math.Abs(got-s.want) > 1e-9 || tm.Running != s.run {
			t.Fatalf("step %d (%s at %v): elapsed %v running %v; want %v %v", i, s.action, s.at, got, tm.Running, s.want, s.run)
		}
	}
	if err := tm.Apply("rewind", t0); err == nil {
		t.Error("an unknown action was accepted")
	}
	// A clock that goes backwards never makes the timer negative.
	var back StageTimer
	back.Start(at(10))
	if got := back.Elapsed(at(5)); got != 0 {
		t.Errorf("elapsed before the start = %v", got)
	}
}

func TestFormatClockAndTimer(t *testing.T) {
	clock := []struct {
		h, m int
		h24  bool
		want string
	}{
		{14, 5, true, "14:05"},
		{14, 5, false, "2:05 PM"},
		{0, 0, true, "00:00"},
		{0, 0, false, "12:00 AM"},
		{12, 30, false, "12:30 PM"},
		{9, 7, true, "09:07"},
		{9, 7, false, "9:07 AM"},
		{23, 59, false, "11:59 PM"},
	}
	for _, c := range clock {
		tm := time.Date(2026, 10, 22, c.h, c.m, 42, 0, time.Local)
		if got := FormatClock(tm, c.h24); got != c.want {
			t.Errorf("FormatClock(%02d:%02d, 24h=%v) = %q, want %q", c.h, c.m, c.h24, got, c.want)
		}
	}
	timer := map[time.Duration]string{
		0:                                 "00:00",
		-3 * time.Second:                  "00:00",
		999 * time.Millisecond:            "00:00",
		61 * time.Second:                  "01:01",
		59*time.Minute + 59*time.Second:   "59:59",
		time.Hour:                         "1:00:00",
		2*time.Hour + 3*time.Minute + 4e9: "2:03:04",
		12 * time.Hour:                    "12:00:00",
	}
	for d, want := range timer {
		if got := FormatTimer(d); got != want {
			t.Errorf("FormatTimer(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestStageSettingsNormalized(t *testing.T) {
	def := DefaultStageSettings()
	if def.Enabled || def.Screen != AutoScreen || !def.ShowNext || !def.ShowClock || def.Clock24h || !def.ShowTimer || def.FontScale != 1 {
		t.Fatalf("defaults = %+v", def)
	}
	tests := []struct {
		in        StageSettings
		screen    int
		fontScale float64
	}{
		{StageSettings{Screen: -5}, AutoScreen, 1},
		{StageSettings{Screen: 1, FontScale: 0.1}, 1, StageFontScaleMin},
		{StageSettings{FontScale: 9}, 0, StageFontScaleMax},
		{StageSettings{FontScale: 1.234}, 0, 1.23},
		{StageSettings{FontScale: math.NaN()}, 0, 1},
		{StageSettings{FontScale: math.Inf(1)}, 0, 1},
	}
	for _, tt := range tests {
		got := tt.in.Normalized()
		if got.Screen != tt.screen || got.FontScale != tt.fontScale {
			t.Errorf("Normalized(%+v) = %+v", tt.in, got)
		}
	}
	// A stored object without some fields keeps their defaults when decoded
	// over the defaults (how the app loads it).
	st := DefaultStageSettings()
	if err := json.Unmarshal([]byte(`{"enabled":true,"clock24h":true}`), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || !st.Clock24h || !st.ShowNext || st.FontScale != 1 || st.Screen != AutoScreen {
		t.Errorf("partial decode = %+v", st)
	}
}

func TestStageScreenFor(t *testing.T) {
	tests := []struct {
		name                        string
		setting, n, primary, output int
		want                        int
		conflict                    bool
	}{
		{"one display: a window on it", AutoScreen, 1, 0, 0, 0, false},
		{"two displays, output on the TV: a window on the primary", AutoScreen, 2, 0, 1, 0, false},
		{"two displays, output off: the TV", AutoScreen, 2, 0, -1, 1, false},
		{"three displays: the one the output leaves", AutoScreen, 3, 0, 1, 2, false},
		{"explicit TV with the output on the primary", 1, 2, 0, 0, 1, false},
		{"explicit display the output covers: conflict, primary window", 1, 2, 0, 1, 0, true},
		{"explicit primary shared with a windowed output", 0, 1, 0, 0, 0, false},
		{"unplugged display: automatic", 4, 2, 0, -1, 1, false},
		{"the primary is the second display", AutoScreen, 2, 1, 0, 1, false},
		{"no displays reported", AutoScreen, 0, 0, -1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conflict := StageScreenFor(tt.setting, tt.n, tt.primary, tt.output)
			if got != tt.want || conflict != tt.conflict {
				t.Fatalf("StageScreenFor(%d, %d, %d, %d) = %d, %v; want %d, %v", tt.setting, tt.n, tt.primary, tt.output, got, conflict, tt.want, tt.conflict)
			}
		})
	}
	for _, c := range []struct {
		stage, output, primary int
		want                   bool
	}{
		{1, 1, 0, true}, {0, 0, 0, false}, {AutoScreen, 1, 0, false}, {1, 0, 0, false}, {2, 2, 0, true},
	} {
		if got := StageConflict(c.stage, c.output, c.primary); got != c.want {
			t.Errorf("StageConflict(%d, %d, %d) = %v", c.stage, c.output, c.primary, got)
		}
	}
	if msg := StageConflictMessage(1); msg == "" || !strings.Contains(msg, "Display 2") {
		t.Errorf("message = %q", msg)
	}
}

func TestBuildStageFrame(t *testing.T) {
	sched := stageSchedule()
	now := time.Date(2026, 10, 22, 19, 5, 0, 0, time.Local)
	running := StageTimer{}
	running.Start(now.Add(-75 * time.Second))
	live := func(id string, i int, mode models.OutputMode) models.PresenterState {
		return models.PresenterState{Live: ItemDeck(sched.Items[sched.IndexOf(id)]), LiveIndex: i, Mode: mode, PreviewIndex: -1}
	}
	all := DefaultStageSettings()
	tests := []struct {
		name string
		p    models.PresenterState
		st   StageSettings
		want models.StageFrame
	}{
		{"nothing live", models.PresenterState{LiveIndex: -1, Mode: models.ModeNormal}, all,
			models.StageFrame{ShowNext: true, Clock: "7:05 PM", Timer: "01:15", TimerRunning: true, FontScale: 1}},
		{"first verse", live("a", 0, models.ModeNormal), all, models.StageFrame{
			Title: "Amazing Grace", Label: "Verse 1", Current: "Verse 1 text", ShowNext: true, Next: "Chorus text", NextLabel: "Chorus",
			Clock: "7:05 PM", Timer: "01:15", TimerRunning: true, FontScale: 1}},
		{"last verse: next item", live("a", 2, models.ModeNormal), all, models.StageFrame{
			Title: "Amazing Grace", Label: "Verse 2", Current: "Verse 2 text", ShowNext: true, Next: "John 3:16 text", NextLabel: "John 3:16",
			NextTitle: "John 3:16", Clock: "7:05 PM", Timer: "01:15", TimerRunning: true, FontScale: 1}},
		{"black keeps the text and shows the badge", live("a", 0, models.ModeBlack), all, models.StageFrame{
			Title: "Amazing Grace", Label: "Verse 1", Current: "Verse 1 text", ShowNext: true, Next: "Chorus text", NextLabel: "Chorus",
			Badge: "BLACK", Clock: "7:05 PM", Timer: "01:15", TimerRunning: true, FontScale: 1}},
		{"clear and logo badges", live("h", 0, models.ModeClear), StageSettings{FontScale: 1.5}, models.StageFrame{
			Title: "How Great", Label: "Verse 1", Current: "Verse 1 text", Badge: "CLEAR", FontScale: 1.5}},
		{"24-hour clock, no timer, no next", live("j", 0, models.ModeLogo), StageSettings{ShowClock: true, Clock24h: true}, models.StageFrame{
			Title: "John 3:16", Label: "John 3:16", Current: "John 3:16 text", Badge: "LOGO", Clock: "19:05", FontScale: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildStageFrame(StageInput{Presenter: tt.p, Schedule: sched, Settings: tt.st, Timer: running, Now: now})
			if got != tt.want {
				t.Fatalf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
	// Windows line ends and surrounding space are folded.
	p := models.PresenterState{Live: &models.Deck{Title: "T", Slides: []models.Slide{{Label: "V", Text: " a\r\nb\rc \n"}}}, LiveIndex: 0}
	if got := BuildStageFrame(StageInput{Presenter: p, Settings: all, Now: now}).Current; got != "a\nb\nc" {
		t.Errorf("current = %q", got)
	}
}

func TestStageHeaders(t *testing.T) {
	tests := []struct {
		f          models.StageFrame
		live       bool
		next, head string
	}{
		{models.StageFrame{Title: "Amazing Grace", Label: "Verse 2", Next: "x", NextLabel: "Chorus"}, true, "NEXT · Chorus", "Amazing Grace · Verse 2"},
		{models.StageFrame{Title: "Amazing Grace", Label: "Verse 3", Next: "x", NextLabel: "Verse 1", NextTitle: "How Great"}, true, "NEXT ITEM · How Great · Verse 1", "Amazing Grace · Verse 3"},
		{models.StageFrame{Title: "John 3:16", Label: "John 3:16"}, true, "LAST SLIDE", "John 3:16"},
		{models.StageFrame{Title: "Media", Next: "text only"}, true, "NEXT", "Media"},
		{models.StageFrame{Label: "Only a label"}, false, "", "Only a label"},
		{models.StageFrame{}, false, "", ""},
	}
	for _, tt := range tests {
		if got := StageNextHeader(tt.f, tt.live); got != tt.next {
			t.Errorf("StageNextHeader(%+v) = %q, want %q", tt.f, got, tt.next)
		}
		if got := StageHeaderTitle(tt.f); got != tt.head {
			t.Errorf("StageHeaderTitle(%+v) = %q, want %q", tt.f, got, tt.head)
		}
	}
}

func TestStageStateDoc(t *testing.T) {
	now := time.Date(2026, 10, 22, 19, 5, 0, 0, time.Local)
	var tm StageTimer
	tm.Start(now.Add(-90500 * time.Millisecond))
	f := models.StageFrame{Title: "A", Label: "Verse 1", Current: "c", Next: "n", NextLabel: "Chorus", Badge: "BLACK", Timer: "01:30"}
	doc := StageStateDoc(f, DefaultStageSettings(), tm, now, StageWindow{Open: true, Fullscreen: true, Screen: 1, Monitor: true})
	if !doc.Open || !doc.Fullscreen || doc.Screen != 1 || doc.Monitor != "foldback" || doc.NextHeader != "NEXT · Chorus" ||
		doc.Badge != "BLACK" || doc.TimerSeconds != 90 || !doc.TimerRunning || doc.Current != "c" || doc.Next != "n" {
		t.Fatalf("doc = %+v", doc)
	}
	closed := StageStateDoc(f, DefaultStageSettings(), StageTimer{}, now, StageWindow{Screen: 1})
	if closed.Open || closed.Screen != -1 || closed.Monitor != "live" || closed.TimerRunning || closed.TimerSeconds != 0 {
		t.Fatalf("closed = %+v", closed)
	}
}
