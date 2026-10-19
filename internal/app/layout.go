package app

import (
	"context"
	"maps"

	"freebeams/internal/app/core"
	"freebeams/internal/ui"
)

// layoutSaveFrames is how long after the last splitter change the layout is
// saved: a drag reports every motion, and one write per drag is enough.
const layoutSaveFrames = 30

// splitters names every splitter the operator can move, for the split control
// hooks and the persisted layout (core.SettingLayout).
func (a *App) splitters() map[string]*ui.Splitter {
	return map[string]*ui.Splitter{
		"main":          a.win.MainSplit,
		"top":           a.win.TopSplit,
		"preview-live":  a.win.PreviewLiveSplit,
		"preview":       a.win.PreviewSplit,
		"live":          a.win.LiveSplit,
		"resource":      a.win.ResourceSplit,
		"songs":         a.win.Songs.Splitter(),
		"scriptures":    a.win.Scriptures.Splitter(),
		"presentations": a.win.Presentations.Splitter(),
	}
}

// splitter returns the splitter called name, or nil.
func (a *App) splitter(name string) *ui.Splitter { return a.splitters()[name] }

// splitterPositions reports where every splitter is (GET /state).
func (a *App) splitterPositions() map[string]core.SplitPos {
	all := a.splitters()
	out := make(map[string]core.SplitPos, len(all))
	for name, sp := range all {
		out[name] = core.SplitPos{Percent: sp.Percent, Collapsed: sp.Collapsed()}
	}
	return out
}

// layoutChanged records a splitter the operator moved and saves the layout
// once the drag has settled.
func (a *App) layoutChanged(name string, sp *ui.Splitter) {
	if a.layout.Splitters == nil {
		a.layout.Splitters = map[string]core.SplitPos{}
	}
	a.layout.Splitters[name] = core.SplitPos{Percent: sp.Percent, Collapsed: sp.Collapsed()}
	if a.layoutSaveQueued || a.st == nil {
		return
	}
	a.layoutSaveQueued = true
	a.after(layoutSaveFrames, a.saveLayout)
}

// saveLayout writes the layout setting, off the main thread.
func (a *App) saveLayout() {
	a.layoutSaveQueued = false
	if a.st == nil {
		return
	}
	st, layout := a.st, core.Layout{Splitters: maps.Clone(a.layout.Splitters)}
	a.async(nil, func(ctx context.Context) func() error {
		err := st.SetSetting(ctx, core.SettingLayout, layout)
		return func() error {
			a.fail("save layout", err)
			return err
		}
	})
}

// restoreLayout puts the splitters where the operator left them (startup).
func (a *App) restoreLayout() {
	if a.st == nil {
		return
	}
	var layout core.Layout
	if _, err := a.st.GetSetting(a.ctx, core.SettingLayout, &layout); err != nil {
		a.fail("read layout setting", err)
		return
	}
	a.layout = layout
	for name, sp := range a.splitters() {
		if pos, ok := layout.Splitters[name]; ok {
			sp.SetCollapsed(pos.Collapsed)
			sp.SetPercent(pos.Percent)
		}
	}
}
