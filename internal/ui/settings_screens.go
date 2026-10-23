package ui

import (
	"freebeams/internal/models"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/OptionButton"
	"graphics.gd/classdb/SpinBox"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// The Black, Clear and Logo transitions on the Transition page
// (EasyWorship's Options ▸ Transitions tabs Black / Clear / Logo). Each row
// is a kind picker whose first entry, "Same as Slide", follows the slide
// transition, and a duration that is editable only for a transition of its
// own. Changes go out through SettingsDialog.OnScreenTransition.

// screenRows are the screens in EasyWorship's order.
var screenRows = []struct {
	mode    models.OutputMode
	caption string
}{{models.ModeBlack, "Black"}, {models.ModeClear, "Clear"}, {models.ModeLogo, "Logo"}}

// screenControls are one row's controls.
type screenControls struct {
	mode     models.OutputMode
	kind     OptionButton.Instance // 0 = Same as Slide, then d.kinds
	duration SpinBox.Instance
}

// buildScreenTransitionsCard is the card under the slide transition.
func (d *SettingsDialog) buildScreenTransitionsCard() Control.Instance {
	rows := []Control.Instance{}
	for _, r := range screenRows {
		c := &screenControls{mode: r.mode}
		c.kind = OptionButton.New()
		c.kind.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
		c.kind.MoreArgs().AddItem("Same as Slide", 0)
		for i, k := range d.kinds {
			c.kind.MoreArgs().AddItem(k.Label(), i+1)
		}
		c.kind.OnItemSelected(func(idx int) {
			if d.rendering || d.OnScreenTransition == nil {
				return
			}
			if idx <= 0 || idx > len(d.kinds) {
				d.OnScreenTransition(c.mode, models.SameTransition())
				return
			}
			t := d.screenTransition(c.mode)
			t.Kind = d.kinds[idx-1]
			d.OnScreenTransition(c.mode, models.OwnTransition(t))
		})
		c.duration = SpinBox.New()
		rg := c.duration.AsRange()
		rg.SetMinValue(0)
		rg.SetMaxValue(Float.X(models.MaxTransitionDuration.Milliseconds()))
		rg.SetStep(50)
		c.duration.SetSuffix("ms")
		c.duration.AsControl().SetCustomMinimumSize(Vector2.New(120, 0))
		rg.OnValueChanged(func(v Float.X) {
			if d.rendering || d.OnScreenTransition == nil {
				return
			}
			st, _ := d.values.Screens.Get(c.mode)
			if st = st.Normalized(); st.Same {
				return // follows the slide's duration
			}
			t := st.Transition
			t.Duration = msToDuration(float64(v))
			d.OnScreenTransition(c.mode, models.OwnTransition(t))
		})
		d.screenCtl = append(d.screenCtl, c)
		row := hbox(8)
		add(row, c.kind.AsControl(), c.duration.AsControl())
		row.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
		rows = append(rows, settingRow(r.caption, row.AsControl()))
	}
	rows = append(rows, hint("Going to and from Black, Clear or Logo uses its own transition; Same as Slide uses the "+
		"slide transition above."))
	return card("Black, Clear and Logo", rows...)
}

// screenTransition is what mode's row runs now (the slide's when Same).
func (d *SettingsDialog) screenTransition(mode models.OutputMode) models.Transition {
	st, _ := d.values.Screens.Get(mode)
	return st.Resolve(d.values.Transition)
}

// renderScreenTransitions projects the values onto the rows (part of
// render; the no-signal setters keep a projection from reporting a change).
func (d *SettingsDialog) renderScreenTransitions() {
	for _, c := range d.screenCtl {
		st, _ := d.values.Screens.Get(c.mode)
		st = st.Normalized()
		t := st.Resolve(d.values.Transition)
		sel := 0
		if !st.Same {
			for i, k := range d.kinds {
				if k == t.Kind {
					sel = i + 1
				}
			}
		}
		c.kind.Select(sel)
		c.duration.AsRange().SetValueNoSignal(Float.X(t.Duration.Milliseconds()))
		c.duration.SetEditable(!st.Same && t.Kind != models.TransitionCut)
	}
	if len(d.screenCtl) > 0 {
		flushCalls(d.screenCtl[0].kind.AsControl())
	}
}
