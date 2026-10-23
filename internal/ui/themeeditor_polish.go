package ui

import (
	"fmt"

	"freebeams/internal/models"

	"graphics.gd/classdb/Control"

	"graphics.gd/classdb/TextServer"
	"graphics.gd/variant/Float"
)

// The theme editor's text box band, media backgrounds, the footer's
// "Apply changes to items in schedule" choice and the font search
// (themeeditor.go builds the rest).

func (e *ThemeEditor) buildBox(col interface{ AsControl() Control.Instance }) {
	add(col, sectionLabel("Text Box"))
	e.boxOn = e.check("Band behind the text", func(on bool) {
		e.change(func(t *models.Theme) {
			t.Box.Enabled = on
			if on && t.Box.Color.A == 0 {
				t.Box.Color = models.RGBA{A: 0.6}
			}
		})
	})
	e.boxColor = e.colorButton(true, func(c models.RGBA) { e.change(func(t *models.Theme) { t.Box.Color = c }) })
	e.boxPad = e.spin(0, 300, 2, func(v float64) { e.change(func(t *models.Theme) { t.Box.Padding = float32(v) }) })
	e.boxFull = e.check("Full width", func(on bool) { e.change(func(t *models.Theme) { t.Box.FullWidth = on }) })
	add(col, fieldRow("", e.boxOn.AsControl(), inlineLabel("Color").AsControl(), e.boxColor.AsControl(),
		inlineLabel("Padding").AsControl(), e.boxPad.AsControl(), e.boxFull.AsControl()))
	e.boxCaption = label("", vHint)
	e.boxCaption.SetClipText(false)
	e.boxCaption.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	add(col, fieldRow("", e.boxCaption.AsControl()))
}

// TextBoxCaption describes the text box the margins leave, in canvas units.
func TextBoxCaption(m models.Margins) string {
	w := max(0, models.CanvasWidth-m.Left-m.Right)
	h := max(0, models.CanvasHeight-m.Top-m.Bottom)
	return fmt.Sprintf("The text box (set by the margins) is %g × %g at %g, %g. Bottom alignment and a band make a lower third.", w, h, m.Left, m.Top)
}

func (e *ThemeEditor) renderBox() {
	b := e.theme.Box
	e.boxOn.AsBaseButton().SetPressedNoSignal(b.Enabled)
	e.boxColor.SetColor(toGodotColor(b.Color))
	e.boxPad.AsRange().SetValueNoSignal(Float.X(b.Padding))
	e.boxFull.AsBaseButton().SetPressedNoSignal(b.FullWidth)
	e.boxColor.AsBaseButton().SetDisabled(!b.Enabled)
	e.boxPad.SetEditable(b.Enabled)
	e.boxFull.AsBaseButton().SetDisabled(!b.Enabled)
	e.boxCaption.SetText(TextBoxCaption(e.theme.Margins))
}

// SetBackgroundMedia makes bg (an image or a video from the library or the
// Browse… dialog) the background being edited.
func (e *ThemeEditor) SetBackgroundMedia(bg models.Background) {
	e.change(func(t *models.Theme) { t.Background = bg })
}

// SetApplyToSchedule projects the "Apply changes to items in schedule" choice.
func (e *ThemeEditor) SetApplyToSchedule(on bool) {
	if e.applyScheduleOn == on {
		return
	}
	e.applyScheduleOn = on
	e.render()
}

// ApplyToSchedule is the footer checkbox's state as projected.
func (e *ThemeEditor) ApplyToSchedule() bool { return e.applyScheduleOn }

// SearchFonts types q into the font search (opening the list), as the
// operator does. FontMatches lists what the list then shows.
func (e *ThemeEditor) SearchFonts(q string) {
	if !e.font.open {
		e.font.setOpen(true)
	}
	e.font.setQuery(q)
}

// FontMatches lists the font list's entries for the current search.
func (e *ThemeEditor) FontMatches() []string { return e.font.matches() }

// FontListOpen reports whether the font list is open.
func (e *ThemeEditor) FontListOpen() bool { return e.font.open }

// FontCount is the number of families offered.
func (e *ThemeEditor) FontCount() int { return len(e.fonts) }

// CloseFontList closes the font list (Esc in the search field does too).
func (e *ThemeEditor) CloseFontList() { e.font.setOpen(false) }

// PickFont picks a family as a click in the list does.
func (e *ThemeEditor) PickFont(family string) bool {
	for i, n := range e.font.names {
		if n == family || e.font.labels[i] == family {
			e.font.pick(i)
			return true
		}
	}
	return false
}
