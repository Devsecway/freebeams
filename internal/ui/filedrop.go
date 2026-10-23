package ui

import (
	"freebeams/internal/ui/uicore"

	"graphics.gd/classdb/Control"
	"graphics.gd/variant/Rect2"
	"graphics.gd/variant/Vector2"
)

// Files dropped from the Finder (or Explorer) onto the main window. Godot
// reports them through the window's files_dropped signal with the paths
// only, and no hover events before it: the app asks the window what is under
// the pointer when the drop arrives. The window stays a dumb view: it names
// the spot, the app decides what the files become.

// FileDropSpot is what an OS file drop at a point lands on.
type FileDropSpot struct {
	Schedule bool // the schedule column (its header and list, not the audio playlist)
	Gap      int  // Schedule: the insertion gap under the pointer (uicore.DropGap)
	Tab      int  // a resource tab the drop is for (its button, or its page while shown); -1 none
}

// FileDropAt names the spot under canvas point p.
func (w *MainWindow) FileDropAt(p Vector2.XY) FileDropSpot {
	spot := FileDropSpot{Gap: -1, Tab: -1}
	if Rect2.HasPoint(w.scheduleDropRect(), p) {
		_, _, mids := w.Schedule.rowSpans()
		spot.Schedule, spot.Gap = true, uicore.DropGap(float64(p.Y), mids)
		return spot
	}
	if i := w.Resources.TabAt(p); i >= 0 {
		spot.Tab = i
		return spot
	}
	if Rect2.HasPoint(w.ResourceSplit.AsControl().GetGlobalRect(), p) {
		spot.Tab = w.Resources.Tab()
	}
	return spot
}

// scheduleDropRect is the schedule column without the audio playlist under
// it: the header and the list.
func (w *MainWindow) scheduleDropRect() Rect2.PositionSize {
	list := w.Schedule.AsControl().GetGlobalRect()
	col := w.Schedule.AsControl().GetParentControl()
	if col == (Control.Instance{}) {
		return list
	}
	r := col.GetGlobalRect()
	bottom := list.Position.Y + list.Size.Y
	return Rect2.New(r.Position.X, r.Position.Y, r.Size.X, bottom-r.Position.Y)
}

// FileDropPoint is a canvas point where a file drop lands on the schedule's
// gap k (schedule true), or on resource tab button tab (for the drop-files
// hook, which warps the pointer there).
func (w *MainWindow) FileDropPoint(schedule bool, k, tab int) (Vector2.XY, bool) {
	if schedule {
		return w.Schedule.DropGapPoint(k)
	}
	return w.Resources.TabCenter(tab)
}

// TabAt is the tab whose button is under canvas point p, or -1.
func (r *ResourceTabs) TabAt(p Vector2.XY) int {
	for i, t := range r.tabs {
		if Rect2.HasPoint(t.AsControl().GetGlobalRect(), p) {
			return i
		}
	}
	return -1
}

// TabCenter is the centre of tab i's button (canvas coordinates).
func (r *ResourceTabs) TabCenter(i int) (Vector2.XY, bool) {
	if i < 0 || i >= len(r.tabs) {
		return Vector2.Zero, false
	}
	g := r.tabs[i].AsControl().GetGlobalRect()
	return Vector2.New(g.Position.X+g.Size.X/2, g.Position.Y+g.Size.Y/2), true
}
