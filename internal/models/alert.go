package models

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Alerts are messages shown over whatever is live, without being slides:
// EasyWorship's message alerts (Alerts.db `alert`: name, rtf, display_type,
// text_alignment, end_type, end_token, repeat_count), its nursery alert, a
// countdown (its Service Interval) and a clock. This file holds the pure
// parts: the saved alert, tag (token) fill, the marquee timeline, countdown
// and clock formatting, the nursery queue, the styles from Options ▸ Alerts
// and the overlay frame the renderer draws.

// AlertDisplay is how a message alert is drawn (EasyWorship's Display Method).
type AlertDisplay string

const (
	AlertStatic AlertDisplay = "static" // a banner, aligned per Alert.Align
	AlertScroll AlertDisplay = "scroll" // a ticker (marquee) right to left
	AlertAuto   AlertDisplay = "auto"   // "Scroll when needed": a banner when it fits, else a ticker
)

// AlertDisplays lists the display methods in EasyWorship's menu order.
func AlertDisplays() []AlertDisplay { return []AlertDisplay{AlertStatic, AlertScroll, AlertAuto} }

// Label is the display method's name in the Edit Alert dialog.
func (d AlertDisplay) Label() string {
	switch d {
	case AlertStatic:
		return "Static"
	case AlertAuto:
		return "Scroll when needed"
	}
	return "Scrolling"
}

// ParseAlertDisplay reads a display method leniently: static / banner,
// scroll / scrolling / ticker / marquee, auto / scroll-when-needed.
func ParseAlertDisplay(s string) (AlertDisplay, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "static", "banner":
		return AlertStatic, nil
	case "scroll", "scrolling", "ticker", "marquee":
		return AlertScroll, nil
	case "auto", "scroll when needed", "scroll-when-needed":
		return AlertAuto, nil
	}
	return "", fmt.Errorf("unknown alert style %q (ticker|static|auto)", s)
}

// AlertEnd is when a message alert stops (EasyWorship's Display Count).
type AlertEnd string

const (
	AlertEndCount    AlertEnd = "count"    // after Count passes (a ticker) or Count × the static time (a banner)
	AlertEndManual   AlertEnd = "manual"   // "Until I Remove"
	AlertEndToken    AlertEnd = "token"    // "Until Token Expiration": when the countdown ends
	AlertEndDuration AlertEnd = "duration" // after Seconds
)

// MaxAlertCount is the largest Display Count (EasyWorship offers 1–9).
const MaxAlertCount = 9

// Alert is a saved message alert (a template; tokens are filled when shown).
type Alert struct {
	ID      int64        `json:"id"`
	Name    string       `json:"name"`
	Message string       `json:"message"` // may contain {token} placeholders
	Display AlertDisplay `json:"display"`
	Align   HAlign       `json:"align"` // a banner's text alignment
	End     AlertEnd     `json:"end"`
	Count   int          `json:"count"`   // passes, for AlertEndCount (1..MaxAlertCount)
	Seconds int          `json:"seconds"` // for AlertEndDuration
	// EndToken names the token whose expiration ends the alert
	// (AlertEndToken); only {countdown} expires.
	EndToken string `json:"endToken,omitempty"`
}

// Normalized fills defaults and clamps values: a scrolling alert that runs
// for 3 passes, centred.
func (a Alert) Normalized() Alert {
	a.Name = strings.TrimSpace(a.Name)
	a.Message = strings.TrimSpace(strings.ReplaceAll(a.Message, "\r\n", "\n"))
	switch a.Display {
	case AlertStatic, AlertScroll, AlertAuto:
	default:
		a.Display = AlertScroll
	}
	switch a.Align {
	case AlignLeft, AlignCenter, AlignRight:
	default:
		a.Align = AlignCenter
	}
	switch a.End {
	case AlertEndCount, AlertEndManual, AlertEndToken, AlertEndDuration:
	default:
		a.End = AlertEndCount
	}
	if a.Count < 1 {
		a.Count = 3
	}
	a.Count = min(a.Count, MaxAlertCount)
	if a.End == AlertEndDuration && a.Seconds <= 0 {
		a.Seconds = 30
	}
	a.Seconds = max(a.Seconds, 0)
	if a.End == AlertEndToken && a.EndToken == "" {
		a.EndToken = TokenCountdown
	}
	return a
}

// ---- tokens (EasyWorship's AlertTagFill) ---------------------------------------

// Tokens filled by the app rather than the operator: they change while the
// alert shows.
const (
	TokenCountdown = "countdown" // the running countdown's remaining time
	TokenClock     = "clock"     // the time of day
)

var tokenRE = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9 _-]{0,31})\}`)

// IsDynamicToken reports whether a token is filled by the app (countdown,
// clock) instead of being asked for.
func IsDynamicToken(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case TokenCountdown, TokenClock, "time":
		return true
	}
	return false
}

// AlertTokens lists the tokens of a message the operator must fill, in order
// of first appearance, without duplicates (case-insensitive) and without the
// dynamic ones.
func AlertTokens(msg string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range tokenRE.FindAllStringSubmatch(msg, -1) {
		name := strings.TrimSpace(m[1])
		key := strings.ToLower(name)
		if name == "" || seen[key] || IsDynamicToken(name) {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

// FillAlertTokens replaces every operator token with its value (looked up
// case-insensitively, trimmed). Dynamic tokens stay for render time. missing
// lists the tokens without a (non-empty) value; they are left in the text.
func FillAlertTokens(msg string, values map[string]string) (text string, missing []string) {
	lower := make(map[string]string, len(values))
	for k, v := range values {
		lower[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	seen := map[string]bool{}
	text = tokenRE.ReplaceAllStringFunc(msg, func(tok string) string {
		name := strings.TrimSpace(tok[1 : len(tok)-1])
		if IsDynamicToken(name) {
			return tok
		}
		if v := lower[strings.ToLower(name)]; v != "" {
			return v
		}
		if key := strings.ToLower(name); !seen[key] {
			seen[key] = true
			missing = append(missing, name)
		}
		return tok
	})
	return text, missing
}

// HasDynamicTokens reports whether text still contains a dynamic token.
func HasDynamicTokens(text string) bool {
	for _, m := range tokenRE.FindAllStringSubmatch(text, -1) {
		if IsDynamicToken(m[1]) {
			return true
		}
	}
	return false
}

// ResolveDynamicTokens fills {countdown} and {clock} / {time}.
func ResolveDynamicTokens(text, countdown, clock string) string {
	return tokenRE.ReplaceAllStringFunc(text, func(tok string) string {
		switch strings.ToLower(strings.TrimSpace(tok[1 : len(tok)-1])) {
		case TokenCountdown:
			return countdown
		case TokenClock, "time":
			return clock
		}
		return tok
	})
}

// ---- marquee timeline ------------------------------------------------------------

// Marquee is a ticker's motion in canvas units: the text enters at the right
// edge (X = Width) and moves left at Speed until it has fully left at the
// left edge (X = -TextWidth). One pass is that whole trip.
type Marquee struct {
	Width     float64 // the band's width (CanvasWidth)
	TextWidth float64
	Speed     float64 // canvas units per second
}

// PassLength is the distance of one pass.
func (m Marquee) PassLength() float64 { return m.Width + max(m.TextWidth, 0) }

// PassDuration is how long one pass takes (0 when the speed is not positive).
func (m Marquee) PassDuration() time.Duration {
	if m.Speed <= 0 {
		return 0
	}
	return time.Duration(m.PassLength() / m.Speed * float64(time.Second))
}

// At returns the text's left edge (canvas units, relative to the band) and
// the 0-based pass after elapsed time.
func (m Marquee) At(elapsed time.Duration) (x float64, pass int) {
	l := m.PassLength()
	if m.Speed <= 0 || l <= 0 || elapsed < 0 {
		return m.Width, 0
	}
	d := elapsed.Seconds() * m.Speed
	pass = int(math.Floor(d / l))
	return m.Width - (d - float64(pass)*l), pass
}

// ---- countdown and clock ---------------------------------------------------------------

// Countdown display formats (EasyWorship's Service Interval formats).
const (
	CountdownMMSS    = "mm:ss"    // 04:59 (minutes keep counting past 59: 75:00)
	CountdownHMMSS   = "hh:mm:ss" // 00:04:59
	CountdownHHMM    = "hh:mm"    // 00:05 (whole minutes, rounded up)
	CountdownAuto    = "auto"     // 4:59, 1:04:59
	DefaultCountdown = CountdownMMSS
)

// CountdownFormats lists the countdown formats for the Options dialog.
func CountdownFormats() []string {
	return []string{CountdownMMSS, CountdownHMMSS, CountdownHHMM, CountdownAuto}
}

// FormatCountdown formats a remaining time. A positive remainder rounds up to
// the second (a five-minute countdown starts at 05:00 and reaches 00:00 at its
// end); a negative one (an overrun) counts up with a leading minus.
func FormatCountdown(d time.Duration, format string) string {
	var secs int64
	neg := false
	if d > 0 {
		secs = int64((d + time.Second - 1) / time.Second)
	} else {
		secs = int64(-d / time.Second)
		neg = secs > 0
	}
	sign := ""
	if neg {
		sign = "-"
	}
	h, m, s := secs/3600, secs/60%60, secs%60
	switch format {
	case CountdownHMMSS:
		return fmt.Sprintf("%s%02d:%02d:%02d", sign, h, m, s)
	case CountdownHHMM:
		mins := (secs + 59) / 60
		if neg {
			mins = secs / 60
		}
		return fmt.Sprintf("%s%02d:%02d", sign, mins/60, mins%60)
	case CountdownAuto:
		if h > 0 {
			return fmt.Sprintf("%s%d:%02d:%02d", sign, h, m, s)
		}
		return fmt.Sprintf("%s%d:%02d", sign, m, s)
	}
	return fmt.Sprintf("%s%02d:%02d", sign, secs/60, s)
}

// Clock formats (EasyWorship's Clock Options ▸ Format).
const (
	Clock12      = "h:mm am/pm"
	Clock12Secs  = "h:mm:ss am/pm"
	Clock24      = "HH:mm"
	Clock24Secs  = "HH:mm:ss"
	DefaultClock = Clock12
)

// ClockFormats lists the clock formats for the Options dialog.
func ClockFormats() []string { return []string{Clock12, Clock12Secs, Clock24, Clock24Secs} }

// FormatClock formats the time of day.
func FormatClock(t time.Time, format string) string {
	switch format {
	case Clock12Secs:
		return t.Format("3:04:05 PM")
	case Clock24:
		return t.Format("15:04")
	case Clock24Secs:
		return t.Format("15:04:05")
	}
	return t.Format("3:04 PM")
}

// ParseClockTime reads a time of day ("10:30", "10:30:15", "10:30 am",
// "7pm") and returns the next such moment at or after now (tomorrow when it
// has passed today).
func ParseClockTime(s string, now time.Time) (time.Time, error) {
	raw := strings.ToUpper(strings.Join(strings.Fields(s), ""))
	layouts := []string{"15:04:05", "15:04", "3:04:05PM", "3:04PM", "3PM", "15"}
	for _, l := range layouts {
		t, err := time.ParseInLocation(l, raw, now.Location())
		if err != nil {
			continue
		}
		at := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
		if at.Before(now) {
			at = at.AddDate(0, 0, 1)
		}
		return at, nil
	}
	return time.Time{}, fmt.Errorf("unknown time %q (e.g. 10:30, 10:30 am, 18:45:00)", s)
}

// CountdownEnd is what a countdown does when it reaches zero.
type CountdownEnd string

const (
	CountdownHide    CountdownEnd = "hide"    // it disappears
	CountdownHold    CountdownEnd = "hold"    // it stays at 00:00 until cleared
	CountdownOverrun CountdownEnd = "overrun" // it keeps counting up (-00:05; EasyWorship's Allow Overrun)
)

// ---- nursery queue -------------------------------------------------------------------

// MaxNurseryCode is the longest nursery code, in characters.
const MaxNurseryCode = 16

// NurseryEntry is one code on the nursery badge.
type NurseryEntry struct {
	Code  string    `json:"code"`
	Added time.Time `json:"added"`
}

// NurseryQueue is the codes shown in the nursery badge, oldest first. Several
// can be shown at once; each stays until removed (or until auto-remove).
type NurseryQueue struct {
	entries []NurseryEntry
	text    string // the badge text, kept in step (no per-frame join)
}

// NormalizeNurseryCode trims and collapses spaces and limits the length; ""
// when nothing is left.
func NormalizeNurseryCode(code string) string {
	code = strings.Join(strings.Fields(code), " ")
	if utf8.RuneCountInString(code) > MaxNurseryCode {
		code = string([]rune(code)[:MaxNurseryCode])
	}
	return code
}

// Add queues a code. It reports false for an empty code or one already shown
// (compared case-insensitively).
func (q *NurseryQueue) Add(code string, now time.Time) bool {
	code = NormalizeNurseryCode(code)
	if code == "" || q.index(code) >= 0 {
		return false
	}
	q.entries = append(q.entries, NurseryEntry{Code: code, Added: now})
	q.update()
	return true
}

func (q *NurseryQueue) index(code string) int {
	code = NormalizeNurseryCode(code)
	for i, e := range q.entries {
		if strings.EqualFold(e.Code, code) {
			return i
		}
	}
	return -1
}

// Remove takes a code off the badge.
func (q *NurseryQueue) Remove(code string) bool {
	i := q.index(code)
	if i < 0 {
		return false
	}
	q.entries = append(q.entries[:i], q.entries[i+1:]...)
	q.update()
	return true
}

// Clear removes every code.
func (q *NurseryQueue) Clear() bool {
	if len(q.entries) == 0 {
		return false
	}
	q.entries = nil
	q.update()
	return true
}

// Expire removes the codes shown for at least after (after <= 0: never).
func (q *NurseryQueue) Expire(now time.Time, after time.Duration) bool {
	if after <= 0 || len(q.entries) == 0 {
		return false
	}
	keep := q.entries[:0]
	for _, e := range q.entries {
		if now.Sub(e.Added) < after {
			keep = append(keep, e)
		}
	}
	if len(keep) == len(q.entries) {
		return false
	}
	q.entries = keep
	q.update()
	return true
}

// Len is the number of codes.
func (q *NurseryQueue) Len() int { return len(q.entries) }

// Codes lists the codes, oldest first.
func (q *NurseryQueue) Codes() []string {
	out := make([]string, len(q.entries))
	for i, e := range q.entries {
		out[i] = e.Code
	}
	return out
}

// Entries returns a copy of the entries.
func (q *NurseryQueue) Entries() []NurseryEntry { return append([]NurseryEntry(nil), q.entries...) }

// Text is the badge text: the codes separated by wide gaps.
func (q *NurseryQueue) Text() string { return q.text }

func (q *NurseryQueue) update() { q.text = strings.Join(q.Codes(), "   ") }

// ---- styles (Options ▸ Alerts) --------------------------------------------------------------

// AlertPosition places a badge: a corner, or the centre of the screen.
type AlertPosition string

const (
	PosTopLeft     AlertPosition = "top-left"
	PosTopRight    AlertPosition = "top-right"
	PosBottomLeft  AlertPosition = "bottom-left"
	PosBottomRight AlertPosition = "bottom-right"
	PosCenter      AlertPosition = "center" // the countdown only: centred, twice the size
)

// AlertCorners lists the corner positions in EasyWorship's order.
func AlertCorners() []AlertPosition {
	return []AlertPosition{PosTopLeft, PosTopRight, PosBottomLeft, PosBottomRight}
}

// Label names a position for the Options dialog.
func (p AlertPosition) Label() string {
	switch p {
	case PosTopLeft:
		return "Top Left"
	case PosTopRight:
		return "Top Right"
	case PosBottomLeft:
		return "Bottom Left"
	case PosBottomRight:
		return "Bottom Right"
	case PosCenter:
		return "Center"
	}
	return string(p)
}

// ParseAlertPosition reads a position ("top-left", "Top Left", "tl", "center").
func ParseAlertPosition(s string) (AlertPosition, error) {
	k := strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.TrimSpace(s)))
	switch k {
	case "topleft", "tl":
		return PosTopLeft, nil
	case "topright", "tr":
		return PosTopRight, nil
	case "bottomleft", "bl":
		return PosBottomLeft, nil
	case "bottomright", "br":
		return PosBottomRight, nil
	case "center", "centre", "middle":
		return PosCenter, nil
	}
	return "", fmt.Errorf("unknown position %q (top-left|top-right|bottom-left|bottom-right|center)", s)
}

// Top reports a top corner.
func (p AlertPosition) Top() bool { return p == PosTopLeft || p == PosTopRight }

// Left reports a left corner.
func (p AlertPosition) Left() bool { return p == PosTopLeft || p == PosBottomLeft }

// AlertBoxStyle is the look of one overlay: its font and its background box.
type AlertBoxStyle struct {
	Family     string  `json:"family"` // "" = the slide default
	Size       float32 `json:"size"`   // canvas units
	Bold       bool    `json:"bold"`
	Color      RGBA    `json:"color"`
	Background RGBA    `json:"background"` // A is the Background Opacity
}

// AlertStyles are the Options ▸ Alerts settings (EasyWorship's Message
// Alert, Nursery Alert, Clock and Service Interval cards).
type AlertStyles struct {
	Message       AlertBoxStyle `json:"message"`
	MessageTop    bool          `json:"messageTop"`    // Screen Location: top (else bottom)
	ScrollSpeed   float32       `json:"scrollSpeed"`   // canvas units per second
	StaticSeconds int           `json:"staticSeconds"` // how long one Display Count of a banner lasts

	Nursery           AlertBoxStyle `json:"nursery"`
	NurseryPosition   AlertPosition `json:"nurseryPosition"`
	NurseryAutoRemove int           `json:"nurseryAutoRemove"` // seconds; 0 = until removed

	Countdown         AlertBoxStyle `json:"countdown"`
	CountdownPosition AlertPosition `json:"countdownPosition"`
	CountdownFormat   string        `json:"countdownFormat"`
	CountdownEnd      CountdownEnd  `json:"countdownEnd"`

	Clock         AlertBoxStyle `json:"clock"`
	ClockPosition AlertPosition `json:"clockPosition"`
	ClockFormat   string        `json:"clockFormat"`

	// HideOnBlack hides every overlay while the output is black. Off by
	// default: an alert (a nursery call above all) must reach the room
	// whatever is on screen, so overlays show over Black, Clear and Logo.
	HideOnBlack bool `json:"hideOnBlack"`
}

// Limits of the style values.
const (
	MinAlertSize    = 16
	MaxAlertSize    = 200
	MinScrollSpeed  = 40
	MaxScrollSpeed  = 1200
	MaxStaticSecs   = 120
	MaxAutoRemove   = 3600
	DefaultSpeed    = 240
	DefaultStaticSc = 10
)

// DefaultAlertStyles is a readable broadcast look: a dark translucent band
// with white semibold text at the bottom, a red nursery badge top right, the
// clock top left and the countdown centred.
func DefaultAlertStyles() AlertStyles {
	white := RGBA{R: 1, G: 1, B: 1, A: 1}
	return AlertStyles{
		Message:       AlertBoxStyle{Size: 54, Bold: true, Color: white, Background: RGBA{R: 0.04, G: 0.05, B: 0.07, A: 0.85}},
		ScrollSpeed:   DefaultSpeed,
		StaticSeconds: DefaultStaticSc,

		Nursery:         AlertBoxStyle{Size: 64, Bold: true, Color: white, Background: RGBA{R: 0.78, G: 0.12, B: 0.12, A: 0.95}},
		NurseryPosition: PosTopRight,

		Countdown:         AlertBoxStyle{Size: 72, Bold: true, Color: white, Background: RGBA{R: 0, G: 0, B: 0, A: 0.6}},
		CountdownPosition: PosCenter,
		CountdownFormat:   DefaultCountdown,
		CountdownEnd:      CountdownHide,

		Clock:         AlertBoxStyle{Size: 44, Bold: true, Color: white, Background: RGBA{R: 0, G: 0, B: 0, A: 0.55}},
		ClockPosition: PosTopLeft,
		ClockFormat:   DefaultClock,
	}
}

func clampF(v, lo, hi float32) float32 { return max(lo, min(v, hi)) }

func (b AlertBoxStyle) normalized(def AlertBoxStyle) AlertBoxStyle {
	if b.Size <= 0 {
		return def // a missing (zero) style: the default look
	}
	b.Size = clampF(b.Size, MinAlertSize, MaxAlertSize)
	b.Family = strings.TrimSpace(b.Family)
	if b.Color == (RGBA{}) {
		b.Color = def.Color
	}
	b.Color.A = 1
	b.Background.A = clampF(b.Background.A, 0, 1)
	return b
}

// Normalized fills missing values from the defaults and clamps the rest.
func (s AlertStyles) Normalized() AlertStyles {
	d := DefaultAlertStyles()
	s.Message = s.Message.normalized(d.Message)
	s.Nursery = s.Nursery.normalized(d.Nursery)
	s.Countdown = s.Countdown.normalized(d.Countdown)
	s.Clock = s.Clock.normalized(d.Clock)
	if s.ScrollSpeed <= 0 {
		s.ScrollSpeed = d.ScrollSpeed
	}
	s.ScrollSpeed = clampF(s.ScrollSpeed, MinScrollSpeed, MaxScrollSpeed)
	if s.StaticSeconds <= 0 {
		s.StaticSeconds = d.StaticSeconds
	}
	s.StaticSeconds = min(s.StaticSeconds, MaxStaticSecs)
	s.NurseryAutoRemove = max(0, min(s.NurseryAutoRemove, MaxAutoRemove))
	corner := func(p, def AlertPosition, center bool) AlertPosition {
		switch p {
		case PosTopLeft, PosTopRight, PosBottomLeft, PosBottomRight:
			return p
		case PosCenter:
			if center {
				return p
			}
		}
		return def
	}
	s.NurseryPosition = corner(s.NurseryPosition, d.NurseryPosition, false)
	s.ClockPosition = corner(s.ClockPosition, d.ClockPosition, false)
	s.CountdownPosition = corner(s.CountdownPosition, d.CountdownPosition, true)
	switch s.CountdownFormat {
	case CountdownMMSS, CountdownHMMSS, CountdownHHMM, CountdownAuto:
	default:
		s.CountdownFormat = d.CountdownFormat
	}
	switch s.CountdownEnd {
	case CountdownHide, CountdownHold, CountdownOverrun:
	default:
		s.CountdownEnd = d.CountdownEnd
	}
	switch s.ClockFormat {
	case Clock12, Clock12Secs, Clock24, Clock24Secs:
	default:
		s.ClockFormat = d.ClockFormat
	}
	return s
}

// ---- overlay frame (what the renderer draws) ---------------------------------------------------

// MessagePadX is the horizontal padding of a banner's text, in canvas units.
func MessagePadX(size float32) float64 { return float64(size) * 0.8 }

// BannerFits reports whether a message of textWidth (canvas units) fits the
// band as a banner ("Scroll when needed" scrolls it otherwise).
func BannerFits(textWidth float64, size float32) bool {
	return textWidth <= CanvasWidth-2*MessagePadX(size)
}

// MessageOverlay is the message band.
type MessageOverlay struct {
	Visible bool
	Text    string
	Scroll  bool    // a ticker: X moves; else a banner aligned per Align
	X       float64 // a ticker's text left edge in canvas units (Marquee.At)
	Align   HAlign
	Top     bool
}

// BoxOverlay is a badge: the nursery codes, the countdown or the clock.
type BoxOverlay struct {
	Visible  bool
	Text     string
	Caption  string // a smaller line above the text (the countdown's "Service starts in")
	Position AlertPosition
}

// OverlayFrame is everything the alert layer shows at one moment. It is
// comparable, so a view applies only what changed.
type OverlayFrame struct {
	Message   MessageOverlay
	Nursery   BoxOverlay
	Countdown BoxOverlay
	Clock     BoxOverlay
}

// Any reports whether anything is shown.
func (f OverlayFrame) Any() bool {
	return f.Message.Visible || f.Nursery.Visible || f.Countdown.Visible || f.Clock.Visible
}
