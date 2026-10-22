package ui

import (
	"freebeams/internal/models"

	"graphics.gd/classdb/CheckBox"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/GridContainer"
	"graphics.gd/classdb/OptionButton"
	"graphics.gd/classdb/ScrollContainer"
	"graphics.gd/classdb/SpinBox"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// Options ▸ Alerts: the look and place of the overlays, after EasyWorship's
// Message Alert, Nursery Alert, Clock and Service Interval cards. Every
// change is reported at once (OnAlertStyles) and projected back by
// SetValues, like the other sections.

// alertControls are the Alerts page's controls.
type alertControls struct {
	msgTop      OptionButton.Instance
	msgSize     SpinBox.Instance
	speed       SpinBox.Instance
	staticSecs  SpinBox.Instance
	nurseryPos  OptionButton.Instance
	nurseryAuto SpinBox.Instance
	cdFormat    OptionButton.Instance
	cdPos       OptionButton.Instance
	cdEnd       OptionButton.Instance
	clkFormat   OptionButton.Instance
	clkPos      OptionButton.Instance
	hideBlack   CheckBox.Instance
}

func positionLabels(ps []models.AlertPosition) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Label()
	}
	return out
}

// grid2 lays out caption / control pairs, one pair to a row.
func grid2(cells ...Control.Instance) Control.Instance {
	g := GridContainer.New()
	g.SetColumns(2)
	g.AsControl().AddThemeConstantOverride("h_separation", 8)
	g.AsControl().AddThemeConstantOverride("v_separation", 6)
	for i, c := range cells {
		if i%2 == 0 {
			continue
		}
		c.SetSizeFlagsHorizontal(Control.SizeExpandFill)
	}
	add(g, cells...)
	return g.AsControl()
}

func caption(text string) Control.Instance {
	l := label(text, "")
	l.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
	l.SetClipText(false)
	l.AsControl().SetCustomMinimumSize(Vector2.New(110, 0))
	return l.AsControl()
}

func (d *SettingsDialog) buildAlertsPage() Control.Instance {
	c := &d.alertCtl
	changed := func(fn func(st *models.AlertStyles)) {
		if d.rendering || d.OnAlertStyles == nil {
			return
		}
		st := d.values.Alerts
		fn(&st)
		d.OnAlertStyles(st.Normalized())
	}
	opt := func(items []string, fn func(st *models.AlertStyles, i int)) OptionButton.Instance {
		o := OptionButton.New()
		for _, it := range items {
			o.AddItem(it)
		}
		o.OnItemSelected(func(i int) { changed(func(st *models.AlertStyles) { fn(st, i) }) })
		return o
	}
	spin := func(lo, hi, step float64, suffix string, fn func(st *models.AlertStyles, v float64)) SpinBox.Instance {
		s := SpinBox.New()
		r := s.AsRange()
		r.SetMinValue(Float.X(lo))
		r.SetMaxValue(Float.X(hi))
		r.SetStep(Float.X(step))
		s.SetSuffix(suffix)
		r.OnValueChanged(func(v Float.X) { changed(func(st *models.AlertStyles) { fn(st, float64(v)) }) })
		return s
	}

	c.msgTop = opt([]string{"Bottom", "Top"}, func(st *models.AlertStyles, i int) { st.MessageTop = i == 1 })
	c.msgSize = spin(models.MinAlertSize, models.MaxAlertSize, 2, "pt", func(st *models.AlertStyles, v float64) { st.Message.Size = float32(v) })
	c.speed = spin(models.MinScrollSpeed, models.MaxScrollSpeed, 20, "/s", func(st *models.AlertStyles, v float64) { st.ScrollSpeed = float32(v) })
	c.staticSecs = spin(1, models.MaxStaticSecs, 1, "s", func(st *models.AlertStyles, v float64) { st.StaticSeconds = int(v) })

	corners := models.AlertCorners()
	c.nurseryPos = opt(positionLabels(corners), func(st *models.AlertStyles, i int) { st.NurseryPosition = corners[i] })
	c.nurseryAuto = spin(0, models.MaxAutoRemove, 10, "s", func(st *models.AlertStyles, v float64) { st.NurseryAutoRemove = int(v) })

	formats := models.CountdownFormats()
	c.cdFormat = opt(formats, func(st *models.AlertStyles, i int) { st.CountdownFormat = formats[i] })
	c.cdPos = opt(positionLabels(countdownPositions), func(st *models.AlertStyles, i int) { st.CountdownPosition = countdownPositions[i] })
	c.cdEnd = opt([]string{"Hide it", "Show 00:00 until cleared", "Keep counting (overrun)"}, func(st *models.AlertStyles, i int) {
		st.CountdownEnd = countdownEnds[i]
	})
	clocks := models.ClockFormats()
	c.clkFormat = opt(clocks, func(st *models.AlertStyles, i int) { st.ClockFormat = clocks[i] })
	c.clkPos = opt(positionLabels(corners), func(st *models.AlertStyles, i int) { st.ClockPosition = corners[i] })

	c.hideBlack = CheckBox.New()
	c.hideBlack.AsButton().SetText("Hide alerts while the output is black")
	c.hideBlack.AsBaseButton().OnToggled(func(on bool) { changed(func(st *models.AlertStyles) { st.HideOnBlack = on }) })

	col := vbox(10)
	col.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	add(col,
		card("Message Alert", grid2(
			caption("Location"), c.msgTop.AsControl(), caption("Text size"), c.msgSize.AsControl(),
			caption("Scroll speed"), c.speed.AsControl(), caption("Banner time"), c.staticSecs.AsControl(),
		)),
		card("Nursery Alert", grid2(
			caption("Position"), c.nurseryPos.AsControl(), caption("Remove after"), c.nurseryAuto.AsControl(),
		), hint("Remove after 0 s keeps a number on screen until it is removed.")),
		card("Countdown and Clock", grid2(
			caption("Countdown"), c.cdFormat.AsControl(), caption("Position"), c.cdPos.AsControl(),
			caption("At zero"), c.cdEnd.AsControl(), caption("Clock"), c.clkFormat.AsControl(),
			caption("Clock position"), c.clkPos.AsControl(),
		)),
		c.hideBlack.AsControl(),
	)
	sc := ScrollContainer.New()
	sc.SetHorizontalScrollMode(ScrollContainer.ScrollModeDisabled)
	add(sc, col.AsControl())
	return sc.AsControl()
}

// renderAlerts projects the alert styles (part of render, rendering set).
func (d *SettingsDialog) renderAlerts() {
	c, st := &d.alertCtl, d.values.Alerts.Normalized()
	sel := func(o OptionButton.Instance, i int) { o.Select(max(i, 0)) }
	val := func(s SpinBox.Instance, v float64) { s.AsRange().SetValueNoSignal(Float.X(v)) }
	sel(c.msgTop, map[bool]int{false: 0, true: 1}[st.MessageTop])
	val(c.msgSize, float64(st.Message.Size))
	val(c.speed, float64(st.ScrollSpeed))
	val(c.staticSecs, float64(st.StaticSeconds))
	sel(c.nurseryPos, indexOf(models.AlertCorners(), st.NurseryPosition, 0))
	val(c.nurseryAuto, float64(st.NurseryAutoRemove))
	sel(c.cdFormat, indexOf(models.CountdownFormats(), st.CountdownFormat, 0))
	sel(c.cdPos, indexOf(countdownPositions, st.CountdownPosition, 0))
	sel(c.cdEnd, indexOf(countdownEnds, st.CountdownEnd, 0))
	sel(c.clkFormat, indexOf(models.ClockFormats(), st.ClockFormat, 0))
	sel(c.clkPos, indexOf(models.AlertCorners(), st.ClockPosition, 0))
	c.hideBlack.AsBaseButton().SetPressedNoSignal(st.HideOnBlack)
	flushCalls(c.hideBlack.AsControl())
}
