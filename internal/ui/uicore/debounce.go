package uicore

import "time"

// Debouncer delays a text value until input has been quiet for Delay. It is a
// plain state machine driven by the caller's clock (the UI calls Poll from
// Process), so it is deterministic and unit-testable.
type Debouncer struct {
	Delay time.Duration

	pending  bool
	value    string
	deadline time.Time
	last     string
	hasLast  bool
}

// Trigger records a new value at time now and restarts the quiet period.
func (d *Debouncer) Trigger(now time.Time, value string) {
	d.pending = true
	d.value = value
	d.deadline = now.Add(d.Delay)
}

// Pending reports whether a value is waiting to fire.
func (d *Debouncer) Pending() bool { return d.pending }

// Poll returns the value and true once the quiet period has elapsed. A value
// equal to the last one fired is swallowed (typing "a" then deleting it does
// not re-run the same search).
func (d *Debouncer) Poll(now time.Time) (string, bool) {
	if !d.pending || now.Before(d.deadline) {
		return "", false
	}
	d.pending = false
	if d.hasLast && d.last == d.value {
		return "", false
	}
	d.last, d.hasLast = d.value, true
	return d.value, true
}

// Flush fires the pending value immediately (e.g. on Enter). It returns false
// when nothing is pending. Unlike Poll it fires even if the value repeats.
func (d *Debouncer) Flush() (string, bool) {
	if !d.pending {
		return "", false
	}
	d.pending = false
	d.last, d.hasLast = d.value, true
	return d.value, true
}

// Reset forgets the pending and last-fired values (e.g. when the field is
// cleared programmatically).
func (d *Debouncer) Reset(value string) {
	d.pending = false
	d.last, d.hasLast = value, true
}
