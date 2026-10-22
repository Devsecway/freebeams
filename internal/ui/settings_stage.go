package ui

import (
	"math"
	"strings"

	"graphics.gd/classdb/CheckBox"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/OptionButton"
	"graphics.gd/classdb/SpinBox"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// StageValues is the Options dialog's Stage Display section (EasyWorship's
// Foldback page): a projection of the app's stage settings. Warning is shown
// under the display picker (e.g. why a display was refused).
type StageValues struct {
	Enabled   bool
	Screen    int // ScreenAuto or a display index
	ShowNext  bool
	ShowClock bool
	Clock24h  bool
	ShowTimer bool
	FontScale float64 // 0.5 .. 2
	Warning   string
}

// stagePage holds the Stage Display section's controls.
type stagePage struct {
	enabled   CheckBox.Instance
	screen    OptionButton.Instance
	showNext  CheckBox.Instance
	showClock CheckBox.Instance
	clock24h  CheckBox.Instance
	showTimer CheckBox.Instance
	scale     SpinBox.Instance
	warning   Label.Instance
	values    StageValues
}

// stagePageAliases are the other names `options {page}` accepts for it.
var stagePageAliases = []string{"stage", "foldback", "stage-display"}

// isStagePage matches the Stage Display section's names.
func isStagePage(name string) bool {
	for _, a := range stagePageAliases {
		if strings.EqualFold(a, name) {
			return true
		}
	}
	return false
}

func (d *SettingsDialog) buildStagePage() Control.Instance {
	p := &stagePage{}
	d.stage = p
	report := func(change func(v *StageValues)) {
		if d.rendering || d.OnStage == nil {
			return
		}
		v := p.values
		change(&v)
		v.Warning = ""
		d.OnStage(v)
	}
	check := func(text string, change func(v *StageValues, on bool)) CheckBox.Instance {
		c := CheckBox.New()
		c.AsButton().SetText(text)
		c.AsBaseButton().OnToggled(func(on bool) { report(func(v *StageValues) { change(v, on) }) })
		return c
	}
	p.enabled = check("Enable support for Foldback Output", func(v *StageValues, on bool) { v.Enabled = on })
	p.screen = OptionButton.New()
	p.screen.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	p.screen.OnItemSelected(func(idx int) {
		report(func(v *StageValues) { v.Screen = p.screen.GetItemId(idx) })
	})
	p.warning = label("", vHint)
	p.warning.SetAutowrapMode(TextServer.AutowrapWordSmart)
	p.warning.SetClipText(false)
	p.warning.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	p.warning.AsControl().AddThemeColorOverride("font_color", Color.RGBA{R: 1, G: 0.36, B: 0.30, A: 1})
	p.warning.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)

	p.showNext = check("Show the next slide", func(v *StageValues, on bool) { v.ShowNext = on })
	p.showTimer = check("Show the service timer", func(v *StageValues, on bool) { v.ShowTimer = on })
	p.showClock = check("Show the clock", func(v *StageValues, on bool) { v.ShowClock = on })
	p.clock24h = check("24-hour clock", func(v *StageValues, on bool) { v.Clock24h = on })
	p.scale = SpinBox.New()
	rg := p.scale.AsRange()
	rg.SetMinValue(50)
	rg.SetMaxValue(200)
	rg.SetStep(10)
	p.scale.SetSuffix("%")
	p.scale.AsControl().SetCustomMinimumSize(Vector2.New(120, 0))
	rg.OnValueChanged(func(val Float.X) {
		report(func(v *StageValues) { v.FontScale = float64(val) / 100 })
	})

	pair := func(a, b Control.Instance) Control.Instance {
		row := hbox(16)
		a.SetCustomMinimumSize(Vector2.New(220, 0))
		add(row, a, b)
		return row.AsControl()
	}
	scaleRow := hbox(0)
	add(scaleRow, p.scale.AsControl(), spacer())
	return page(
		card("Enable Foldback Output",
			p.enabled.AsControl(),
			hint("A stage display for the musicians and the speaker: the live slide's text, the next slide, a clock and a "+
				"service timer, white on black. The Live menu turns it on and off too."),
		),
		card("Stage Display Monitor",
			settingRow("Display", p.screen.AsControl()),
			p.warning.AsControl(),
			hint("A second display is covered like the output; on the primary display it opens as a window. It cannot share a display with the output."),
		),
		card("Content",
			pair(p.showNext.AsControl(), p.showTimer.AsControl()),
			pair(p.showClock.AsControl(), p.clock24h.AsControl()),
			settingRow("Text size", scaleRow.AsControl()),
		),
	)
}

// SetStage projects the stage settings onto the Stage Display section.
func (d *SettingsDialog) SetStage(v StageValues) {
	d.stage.values = v
	d.render()
}

// renderStage is the Stage Display part of render (rendering is set).
func (d *SettingsDialog) renderStage() {
	p := d.stage
	if p == nil {
		return
	}
	v := p.values
	p.enabled.AsBaseButton().SetPressedNoSignal(v.Enabled)
	p.screen.Clear()
	p.screen.MoreArgs().AddItem("Automatic (a display the output does not use)", ScreenAuto)
	for _, s := range d.screens {
		p.screen.MoreArgs().AddItem(s.Label, s.Index)
	}
	if idx := p.screen.GetItemIndex(v.Screen); idx >= 0 {
		p.screen.Select(idx)
	} else {
		p.screen.Select(0)
	}
	p.warning.SetText(v.Warning)
	p.warning.AsCanvasItem().SetVisible(v.Warning != "")
	p.showNext.AsBaseButton().SetPressedNoSignal(v.ShowNext)
	p.showClock.AsBaseButton().SetPressedNoSignal(v.ShowClock)
	p.clock24h.AsBaseButton().SetPressedNoSignal(v.Clock24h)
	p.clock24h.AsBaseButton().SetDisabled(!v.ShowClock)
	p.showTimer.AsBaseButton().SetPressedNoSignal(v.ShowTimer)
	p.scale.AsRange().SetValueNoSignal(Float.X(math.Round(v.FontScale * 100)))
	flushCalls(p.scale.AsControl()) // the no-signal setters land inside the rendering guard
}
