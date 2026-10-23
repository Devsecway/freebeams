package ui

import (
	"freebeams/internal/ui/uicore"

	"graphics.gd/classdb/Button"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Font"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/SystemFont"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Tree"
	"graphics.gd/classdb/TreeItem"
	"graphics.gd/variant/Vector2"
)

// fontPicker is the theme editor's font chooser: a button that shows the
// theme's font in its own face and, below the row, a search field and a
// list of the installed families, each drawn in its own face. Typing
// filters the list (uicore.FilterFonts); Enter or a click picks a family.
// It is a projection: the family comes from the editor's state (setFamily)
// and a pick is reported through onPick only.
type fontPicker struct {
	onPick func(family string)

	button Button.Instance
	panel  Control.Instance // the search field and the list (shown while open)
	search LineEdit.Instance
	list   Tree.Instance

	names  []string // families, "" (the default) first
	labels []string // what the list shows for each family
	items  []TreeItem.Instance
	shown  []int // indexes of names that match the query, in list order
	family int   // index of the theme's font
	query  string
	open   bool
	built  bool
}

const defaultFontLabel = "Default (Helvetica Neue)"

func newFontPicker(onPick func(family string)) *fontPicker {
	p := &fontPicker{onPick: onPick}
	p.button = Button.New()
	p.button.SetTextOverrunBehavior(TextServer.OverrunTrimEllipsis)
	p.button.SetClipText(true)
	p.button.AsControl().SetCustomMinimumSize(Vector2.New(220, 26))
	p.button.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	p.button.AsControl().SetTooltipText("Choose the font: type to search")
	p.button.AsBaseButton().OnPressed(func() { p.setOpen(!p.open) })

	p.search = LineEdit.New()
	p.search.SetPlaceholderText("Search fonts")
	p.search.SetClearButtonEnabled(true)
	p.search.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
	p.search.OnTextChanged(func(q string) { p.setQuery(q) })
	p.search.OnTextSubmitted(func(string) {
		if len(p.shown) > 0 {
			p.pick(p.shown[0])
		}
	})

	p.list = Tree.New()
	p.list.SetHideRoot(true)
	p.list.SetColumns(1)
	p.list.AsControl().SetCustomMinimumSize(Vector2.New(0, 170))
	p.list.OnItemActivated(func() {
		if it := p.list.GetSelected(); it != (TreeItem.Instance{}) {
			p.pick(it.GetIndex())
		}
	})
	p.list.OnItemSelected(func() {
		if it := p.list.GetSelected(); it != (TreeItem.Instance{}) {
			p.pick(it.GetIndex())
		}
	})

	box := vbox(4)
	add(box, p.search.AsControl(), p.list.AsControl())
	p.panel = fieldRow("", box.AsControl())
	box.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	p.panel.AsCanvasItem().SetVisible(false)
	return p
}

// setNames sets the families offered ("" first).
func (p *fontPicker) setNames(names []string) {
	p.names = append([]string(nil), names...)
	p.labels = make([]string, len(names))
	for i, n := range names {
		p.labels[i] = n
		if n == "" {
			p.labels[i] = defaultFontLabel
		}
	}
	p.built = false
	p.items = nil
	if p.open {
		p.render()
	}
}

// appendName adds a family that is not installed (a theme's own font).
func (p *fontPicker) appendName(name string) int {
	p.setNames(append(p.names, name))
	return len(p.names) - 1
}

// faceFor is a family's own face (the default UI face for "").
func faceFor(family string) Font.Instance {
	if family == "" {
		return Font.Instance{}
	}
	f := SystemFont.New()
	f.SetFontNames([]string{family})
	return f.AsFont()
}

// setFamily projects the theme's font (an index into names).
func (p *fontPicker) setFamily(i int) {
	if i == p.family && p.button.Text() == p.labels[i] {
		return
	}
	p.family = i
	p.button.SetText(p.labels[i])
	c := p.button.AsControl()
	if f := faceFor(p.names[i]); f != (Font.Instance{}) {
		c.AddThemeFontOverride("font", f)
	} else {
		c.RemoveThemeFontOverride("font")
	}
	if p.open {
		p.render()
	}
}

func (p *fontPicker) setOpen(on bool) {
	p.open = on
	if !on {
		p.query = ""
	}
	p.render()
	if on {
		p.search.AsControl().GrabFocus()
	}
}

func (p *fontPicker) setQuery(q string) {
	p.query = q
	p.render()
}

func (p *fontPicker) pick(i int) {
	if i < 0 || i >= len(p.names) {
		return
	}
	p.setOpen(false)
	if p.onPick != nil {
		p.onPick(p.names[i])
	}
}

// matches lists the families the list shows for the current query.
func (p *fontPicker) matches() []string {
	out := make([]string, len(p.shown))
	for i, k := range p.shown {
		out[i] = p.labels[k]
	}
	return out
}

// render projects open / query / family onto the panel. The list is built
// the first time it opens (one SystemFont per family, owned by its row).
func (p *fontPicker) render() {
	p.panel.AsCanvasItem().SetVisible(p.open)
	if p.search.Text() != p.query {
		p.search.SetText(p.query)
	}
	p.shown = uicore.FilterFonts(p.labels, p.query, p.family)
	if !p.open {
		flushCalls(p.panel)
		return
	}
	quietly(p.list.AsControl(), func() {
		if !p.built {
			p.built = true
			p.list.Clear()
			root := p.list.CreateItem()
			p.items = make([]TreeItem.Instance, len(p.names))
			for i, n := range p.names {
				it := root.CreateChild()
				it.SetText(0, p.labels[i])
				if f := faceFor(n); f != (Font.Instance{}) {
					it.SetCustomFont(0, f)
				}
				p.items[i] = it
			}
		}
		visible := make([]bool, len(p.items))
		for _, k := range p.shown {
			visible[k] = true
		}
		for i, it := range p.items {
			it.SetVisible(visible[i])
		}
		if p.family >= 0 && p.family < len(p.items) {
			p.list.SetSelected(p.items[p.family], 0)
			p.list.ScrollToItem(p.items[p.family])
		}
	})
	p.list.AsCanvasItem().QueueRedraw()
}
