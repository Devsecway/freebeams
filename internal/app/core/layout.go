package core

// SplitPos is a splitter's position: the first pane's fraction of the shared
// space and whether a pane is snapped closed (ui.CollapseNone / First /
// Second).
type SplitPos struct {
	Percent   float64 `json:"percent"`
	Collapsed int     `json:"collapsed"`
}

// Layout is the part of the operator window the user arranges: the splitter
// positions by name. It is stored under SettingLayout, so the window comes
// back as it was left (UI-SPEC §3.2: the splitter percentages persist).
type Layout struct {
	Splitters map[string]SplitPos `json:"splitters"`
}
