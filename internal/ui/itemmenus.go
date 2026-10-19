package ui

import (
	"freebeams/internal/models"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/PopupMenu"
)

// The library tile menus' shared parts (the Media and Presentations tabs'
// right-click and gear menus, UI-SPEC §3.6): the View ▸ and Sort by ▸
// choices, the radio submenus whose checked entry is projected from state,
// and the footer view sizer that offers the same views. The view and sort
// values mirror core.ItemView / core.ItemSort by value; the app maps between
// them (ui must not import core).

// View ▸ choices of the library grids.
const (
	ViewLarge   = "large"
	ViewMedium  = "medium" // the default
	ViewSmall   = "small"
	ViewDetails = "details" // the list: one row per item (SlideGrid.SetDetails)
)

// Sort by ▸ choices of the library grids.
const (
	SortName = "name"
	SortDate = "date"
	SortKind = "kind"
)

// choice is one entry of a radio submenu: its value and its label.
type choice struct{ value, label string }

var (
	viewChoices   = []choice{{ViewLarge, "Large Thumbnails"}, {ViewMedium, "Medium Thumbnails"}, {ViewSmall, "Small Thumbnails"}, {ViewDetails, "Details"}}
	sortChoices   = []choice{{SortName, "Name"}, {SortDate, "Date Added"}, {SortKind, "Kind"}}
	aspectChoices = []choice{{string(models.AspectFill), "Fill"}, {string(models.AspectFit), "Fit"}, {string(models.AspectStretch), "Stretch"}}
	repeatChoices = []choice{{string(models.RepeatLoop), "Loop"}, {string(models.RepeatOnce), "Play Once"}}
)

// applyView projects a View ▸ choice onto a grid: the Details list, or the
// tile width of the thumbnail sizes (TileWidths: small, medium, large).
func applyView(g *SlideGrid, view string) {
	g.SetDetails(view == ViewDetails)
	switch view {
	case ViewLarge:
		g.SetTileWidth(TileWidths[2])
	case ViewSmall:
		g.SetTileWidth(TileWidths[0])
	default:
		g.SetTileWidth(TileWidths[1])
	}
}

// radioMenu is a popup of exclusive choices. Its checked entry is a
// projection (setChecked from the owner's render pass); a click reports the
// chosen value through onPick and changes nothing itself.
type radioMenu struct {
	menu    PopupMenu.Instance
	choices []choice
}

// newRadioPopup builds a standalone radio popup (a dropdown button's menu).
func newRadioPopup(choices []choice, onPick func(value string)) *radioMenu {
	r := &radioMenu{menu: PopupMenu.New(), choices: choices}
	for i, c := range choices {
		r.menu.MoreArgs().AddRadioCheckItem(c.label, i, 0)
	}
	r.menu.OnIdPressed(func(i int) {
		if i >= 0 && i < len(r.choices) && onPick != nil {
			onPick(r.choices[i].value)
		}
	})
	return r
}

// newRadioMenu adds a radio submenu to parent as the entry label with id.
// The submenu is a named child referenced by name: passing the node itself
// (add_submenu_node_item) trips graphics.gd's ownership check (see Menus).
func newRadioMenu(parent PopupMenu.Instance, name, label string, id int, choices []choice, onPick func(value string)) *radioMenu {
	r := newRadioPopup(choices, onPick)
	r.menu.AsNode().SetName(name)
	parent.AsNode().AddChild(r.menu.AsNode())
	parent.MoreArgs().AddSubmenuItem(label, name, id)
	return r
}

// setChecked marks the entry for value (none when no entry has it).
func (r *radioMenu) setChecked(value string) {
	for i, c := range r.choices {
		r.menu.SetItemChecked(i, c.value == value)
	}
}

// setDisabled greys every entry out (or back in).
func (r *radioMenu) setDisabled(on bool) {
	for i := range r.choices {
		r.menu.SetItemDisabled(i, on)
	}
}

// itemViewSizer is a library footer's [▤|▾] split button: the left half
// steps through the views, the right half drops the View menu. Both report
// through onView only; setView projects the current view back (the checked
// entry and the button's icon).
type itemViewSizer struct {
	controls []Control.Instance
	left     *miniButton
	menu     *radioMenu
	view     string
}

func newItemViewSizer(onView func(view string)) *itemViewSizer {
	s := &itemViewSizer{view: ViewMedium}
	l, r := viewSizer("view_large")
	s.left = l
	s.menu = newRadioPopup(viewChoices, onView)
	r.AsNode().AddChild(s.menu.menu.AsNode())
	l.AsBaseButton().OnPressed(func() {
		next := 0
		for i, c := range viewChoices {
			if c.value == s.view {
				next = (i + 1) % len(viewChoices)
			}
		}
		if onView != nil {
			onView(viewChoices[next].value)
		}
	})
	r.AsBaseButton().OnPressed(func() { popupBelow(s.menu.menu, r.AsControl()) })
	s.controls = []Control.Instance{l.AsControl(), r.AsControl()}
	s.menu.setChecked(s.view)
	return s
}

func (s *itemViewSizer) setView(view string) {
	s.view = view
	s.menu.setChecked(view)
	icon := "view_large"
	if view == ViewDetails {
		icon = "view_details"
	}
	if s.left.icon != icon {
		s.left.icon = icon
		s.left.AsCanvasItem().QueueRedraw()
	}
}
