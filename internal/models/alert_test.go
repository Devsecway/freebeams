package models

import (
	"reflect"
	"testing"
	"time"
)

func TestAlertTokens(t *testing.T) {
	tests := []struct {
		msg  string
		want []string
	}{
		{"no tokens", nil},
		{"Car {plate}, please move it", []string{"plate"}},
		{"{Name} and {name} and {room}", []string{"Name", "room"}},
		{"Starts in {countdown} at {clock}", nil},
		{"{ bad} {1bad} {} {ok_1}", []string{"ok_1"}},
		{"Parent of {child name} to {room}", []string{"child name", "room"}},
	}
	for _, tt := range tests {
		if got := AlertTokens(tt.msg); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("AlertTokens(%q) = %#v, want %#v", tt.msg, got, tt.want)
		}
	}
}

func TestFillAlertTokens(t *testing.T) {
	tests := []struct {
		name    string
		msg     string
		values  map[string]string
		want    string
		missing []string
	}{
		{"filled", "Car {plate}, please move it", map[string]string{"plate": " ABC123 "}, "Car ABC123, please move it", nil},
		{"case-insensitive", "{Name} / {NAME}", map[string]string{"name": "Ann"}, "Ann / Ann", nil},
		{"missing once", "{a} {b} {a}", map[string]string{"a": "1"}, "1 {b} 1", []string{"b"}},
		{"empty value is missing", "{a}", map[string]string{"a": "  "}, "{a}", []string{"a"}},
		{"dynamic kept", "In {countdown} ({room})", map[string]string{"room": "B"}, "In {countdown} (B)", nil},
	}
	for _, tt := range tests {
		got, missing := FillAlertTokens(tt.msg, tt.values)
		if got != tt.want || !reflect.DeepEqual(missing, tt.missing) {
			t.Errorf("%s: FillAlertTokens = %q, %v; want %q, %v", tt.name, got, missing, tt.want, tt.missing)
		}
	}
}

func TestResolveDynamicTokens(t *testing.T) {
	got := ResolveDynamicTokens("Service in {countdown} — {Clock} {time} {other}", "04:59", "10:25 AM")
	if want := "Service in 04:59 — 10:25 AM 10:25 AM {other}"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !HasDynamicTokens("x {countdown}") || HasDynamicTokens("x {plate}") {
		t.Error("HasDynamicTokens")
	}
}

func TestMarquee(t *testing.T) {
	m := Marquee{Width: 1920, TextWidth: 480, Speed: 240} // a pass is 2400 units = 10 s
	if d := m.PassDuration(); d != 10*time.Second {
		t.Fatalf("PassDuration = %v", d)
	}
	tests := []struct {
		at   time.Duration
		x    float64
		pass int
	}{
		{0, 1920, 0},
		{time.Second, 1680, 0},
		{8 * time.Second, 0, 0},
		{9500 * time.Millisecond, -360, 0},
		{10 * time.Second, 1920, 1},
		{25 * time.Second, 720, 2},
		{-time.Second, 1920, 0},
	}
	for _, tt := range tests {
		x, pass := m.At(tt.at)
		if pass != tt.pass || x < tt.x-1e-6 || x > tt.x+1e-6 {
			t.Errorf("At(%v) = %v, %d; want %v, %d", tt.at, x, pass, tt.x, tt.pass)
		}
	}
	if x, p := (Marquee{Width: 1920, TextWidth: 100}).At(time.Second); x != 1920 || p != 0 {
		t.Errorf("zero speed moves: %v %d", x, p)
	}
}

func TestFormatCountdown(t *testing.T) {
	tests := []struct {
		d      time.Duration
		format string
		want   string
	}{
		{5 * time.Minute, CountdownMMSS, "05:00"},
		{4*time.Minute + 59*time.Second + 100*time.Millisecond, CountdownMMSS, "05:00"},
		{4*time.Minute + 59*time.Second, CountdownMMSS, "04:59"},
		{300 * time.Millisecond, CountdownMMSS, "00:01"},
		{0, CountdownMMSS, "00:00"},
		{-400 * time.Millisecond, CountdownMMSS, "00:00"},
		{-5 * time.Second, CountdownMMSS, "-00:05"},
		{75 * time.Minute, CountdownMMSS, "75:00"},
		{75*time.Minute + 5*time.Second, CountdownHMMSS, "01:15:05"},
		{4*time.Minute + 1*time.Second, CountdownHHMM, "00:05"},
		{61 * time.Minute, CountdownHHMM, "01:01"},
		{4*time.Minute + 59*time.Second, CountdownAuto, "4:59"},
		{time.Hour + 4*time.Minute, CountdownAuto, "1:04:00"},
		{-65 * time.Second, CountdownAuto, "-1:05"},
		{90 * time.Second, "bogus", "01:30"},
	}
	for _, tt := range tests {
		if got := FormatCountdown(tt.d, tt.format); got != tt.want {
			t.Errorf("FormatCountdown(%v, %q) = %q, want %q", tt.d, tt.format, got, tt.want)
		}
	}
}

func TestFormatClock(t *testing.T) {
	at := time.Date(2026, 10, 22, 18, 5, 9, 0, time.UTC)
	tests := map[string]string{Clock12: "6:05 PM", Clock12Secs: "6:05:09 PM", Clock24: "18:05", Clock24Secs: "18:05:09", "": "6:05 PM"}
	for f, want := range tests {
		if got := FormatClock(at, f); got != want {
			t.Errorf("FormatClock(%q) = %q, want %q", f, got, want)
		}
	}
}

func TestParseClockTime(t *testing.T) {
	now := time.Date(2026, 10, 22, 9, 30, 0, 0, time.UTC)
	tests := []struct {
		in   string
		want time.Time
		err  bool
	}{
		{"10:30", time.Date(2026, 10, 22, 10, 30, 0, 0, time.UTC), false},
		{"10:30 am", time.Date(2026, 10, 22, 10, 30, 0, 0, time.UTC), false},
		{"6:45pm", time.Date(2026, 10, 22, 18, 45, 0, 0, time.UTC), false},
		{"7 PM", time.Date(2026, 10, 22, 19, 0, 0, 0, time.UTC), false},
		{"18:45:30", time.Date(2026, 10, 22, 18, 45, 30, 0, time.UTC), false},
		{"9:00", time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC), false}, // passed today: tomorrow
		{"9:30", now, false},
		{"soon", time.Time{}, true},
		{"25:00", time.Time{}, true},
	}
	for _, tt := range tests {
		got, err := ParseClockTime(tt.in, now)
		if (err != nil) != tt.err || (!tt.err && !got.Equal(tt.want)) {
			t.Errorf("ParseClockTime(%q) = %v, %v; want %v (err %v)", tt.in, got, err, tt.want, tt.err)
		}
	}
}

func TestNurseryQueue(t *testing.T) {
	t0 := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	var q NurseryQueue
	steps := []struct {
		name  string
		do    func() bool
		ok    bool
		codes []string
		text  string
	}{
		{"add", func() bool { return q.Add(" 1234 ", t0) }, true, []string{"1234"}, "1234"},
		{"add second", func() bool { return q.Add("a  7", t0.Add(time.Minute)) }, true, []string{"1234", "a 7"}, "1234   a 7"},
		{"duplicate", func() bool { return q.Add("A 7", t0) }, false, []string{"1234", "a 7"}, "1234   a 7"},
		{"empty", func() bool { return q.Add("   ", t0) }, false, []string{"1234", "a 7"}, "1234   a 7"},
		{"long code is cut", func() bool { return q.Add("ABCDEFGHIJKLMNOPQRS", t0.Add(2*time.Minute)) }, true, []string{"1234", "a 7", "ABCDEFGHIJKLMNOP"}, "1234   a 7   ABCDEFGHIJKLMNOP"},
		{"remove", func() bool { return q.Remove("1234") }, true, []string{"a 7", "ABCDEFGHIJKLMNOP"}, "a 7   ABCDEFGHIJKLMNOP"},
		{"remove missing", func() bool { return q.Remove("1234") }, false, []string{"a 7", "ABCDEFGHIJKLMNOP"}, "a 7   ABCDEFGHIJKLMNOP"},
		{"expire off", func() bool { return q.Expire(t0.Add(time.Hour), 0) }, false, []string{"a 7", "ABCDEFGHIJKLMNOP"}, "a 7   ABCDEFGHIJKLMNOP"},
		{"expire the oldest", func() bool { return q.Expire(t0.Add(150*time.Second), 90*time.Second) }, true, []string{"ABCDEFGHIJKLMNOP"}, "ABCDEFGHIJKLMNOP"},
		{"clear", func() bool { return q.Clear() }, true, []string{}, ""},
		{"clear empty", func() bool { return q.Clear() }, false, []string{}, ""},
	}
	for _, s := range steps {
		if got := s.do(); got != s.ok {
			t.Errorf("%s: returned %v, want %v", s.name, got, s.ok)
		}
		if got := q.Codes(); !reflect.DeepEqual(got, s.codes) {
			t.Errorf("%s: codes %#v, want %#v", s.name, got, s.codes)
		}
		if q.Text() != s.text {
			t.Errorf("%s: text %q, want %q", s.name, q.Text(), s.text)
		}
	}
}

func TestAlertNormalized(t *testing.T) {
	a := Alert{Name: "  Car ", Message: " Move {plate}\r\n", Count: 42}.Normalized()
	want := Alert{Name: "Car", Message: "Move {plate}", Display: AlertScroll, Align: AlignCenter, End: AlertEndCount, Count: MaxAlertCount}
	if a != want {
		t.Errorf("Normalized = %+v, want %+v", a, want)
	}
	d := Alert{End: AlertEndDuration}.Normalized()
	if d.Seconds != 30 || d.Count != 3 {
		t.Errorf("duration defaults: %+v", d)
	}
	if tk := (Alert{End: AlertEndToken}).Normalized(); tk.EndToken != TokenCountdown {
		t.Errorf("token default: %+v", tk)
	}
}

func TestParseAlertDisplayAndPosition(t *testing.T) {
	for in, want := range map[string]AlertDisplay{"ticker": AlertScroll, "Marquee": AlertScroll, "banner": AlertStatic, "static": AlertStatic, "Scroll when needed": AlertAuto} {
		if got, err := ParseAlertDisplay(in); err != nil || got != want {
			t.Errorf("ParseAlertDisplay(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseAlertDisplay("wobble"); err == nil {
		t.Error("ParseAlertDisplay accepted wobble")
	}
	for in, want := range map[string]AlertPosition{"top-left": PosTopLeft, "Top Right": PosTopRight, "bl": PosBottomLeft, "bottom_right": PosBottomRight, "Centre": PosCenter} {
		if got, err := ParseAlertPosition(in); err != nil || got != want {
			t.Errorf("ParseAlertPosition(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseAlertPosition("upstairs"); err == nil {
		t.Error("ParseAlertPosition accepted upstairs")
	}
}

func TestAlertStylesNormalized(t *testing.T) {
	if got := (AlertStyles{}).Normalized(); got != DefaultAlertStyles() {
		t.Errorf("zero styles normalize to %+v, want the defaults", got)
	}
	s := DefaultAlertStyles()
	s.Message.Size = 5000
	s.ScrollSpeed = 1
	s.NurseryPosition = PosCenter // only the countdown may be centred
	s.CountdownFormat = "x"
	s.Message.Background.A = 3
	s.Message.Color = RGBA{R: 1, A: 0.2}
	n := s.Normalized()
	if n.Message.Size != MaxAlertSize || n.ScrollSpeed != MinScrollSpeed || n.NurseryPosition != PosTopRight ||
		n.CountdownFormat != DefaultCountdown || n.Message.Background.A != 1 || n.Message.Color.A != 1 {
		t.Errorf("Normalized = %+v", n)
	}
}
