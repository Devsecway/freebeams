package ui

import (
	"slices"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/OptionButton"
)

// DefaultThemeRow is one item type of Options ▸ Themes: its name ("Songs"),
// the theme names to choose from and the index of the current default (-1
// when none).
type DefaultThemeRow struct {
	Kind     string
	Choices  []string
	Selected int
}

// DefaultThemesPage is Options ▸ Themes: one dropdown per item type that has
// a default theme. The choice is the same state as the Themes tab's "Set as
// Default"; OnChange reports it (kind and theme are row and choice indexes).
type DefaultThemesPage struct {
	OnChange func(kind, theme int)

	col   Control.Instance
	opts  []OptionButton.Instance
	rows  []DefaultThemeRow
	shown []DefaultThemeRow

	rendering bool
}

func newDefaultThemesPage() *DefaultThemesPage {
	p := &DefaultThemesPage{}
	c := vbox(8)
	p.col = c.AsControl()
	return p
}

func (p *DefaultThemesPage) build() Control.Instance {
	return page(card("Default Themes",
		p.col,
		hint("New songs and scriptures shown in Preview or added to the schedule use these themes, unless a song has a "+
			"theme of its own. Items already in the schedule keep their look. Same as Set as Default on the Themes tab."),
	))
}

// SetRows projects the item types and their defaults.
func (p *DefaultThemesPage) SetRows(rows []DefaultThemeRow) {
	p.rows = rows
	p.render()
}

func sameThemeRows(a, b []DefaultThemeRow) bool {
	return slices.EqualFunc(a, b, func(x, y DefaultThemeRow) bool {
		return x.Kind == y.Kind && x.Selected == y.Selected && slices.Equal(x.Choices, y.Choices)
	})
}

func (p *DefaultThemesPage) render() {
	if sameThemeRows(p.rows, p.shown) {
		return
	}
	p.rendering = true
	defer func() { p.rendering = false }()
	// One dropdown per kind (the kinds never change at runtime; build once).
	for len(p.opts) < len(p.rows) {
		kind := len(p.opts)
		o := OptionButton.New()
		o.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
		o.AsNode().SetName("DefaultTheme" + p.rows[kind].Kind)
		o.OnItemSelected(func(idx int) {
			if !p.rendering && p.OnChange != nil {
				p.OnChange(kind, o.GetItemId(idx))
			}
		})
		p.opts = append(p.opts, o)
		add(p.col, settingRow(p.rows[kind].Kind, o.AsControl()))
	}
	for i, r := range p.rows {
		o := p.opts[i]
		if i >= len(p.shown) || !slices.Equal(r.Choices, p.shown[i].Choices) {
			o.Clear()
			for j, name := range r.Choices {
				o.MoreArgs().AddItem(name, j)
			}
		}
		o.Select(r.Selected)
		flushCalls(o.AsControl())
	}
	p.shown = slices.Clone(p.rows)
}
