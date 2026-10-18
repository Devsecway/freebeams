package ui

import (
	"math"

	"freeworship/internal/ui/uicore"

	"graphics.gd/classdb/Container"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/PanelContainer"
	"graphics.gd/variant/Callable"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// MonitorHost keeps a 16:9 letterboxed area (EasyWorship's Frame_LivePreview /
// Panel_PreviewOutput) and fits every child control into it. The app adds its
// render.SlideView with SetContent or AsNode().AddChild. The window.bk1
// surround and the black screen with its 1px frame are internal child panels
// (themed, so Light/Dark restyles them), not custom drawing.
type MonitorHost struct {
	Container.Extension[MonitorHost] `gd:"FwMonitorHost"`

	Aspect float64 `gd:"-"` // width / height, default 16/9
	Margin float64 `gd:"-"` // margin around the screen rect, default 4
	// FitWidth makes the host's minimum height follow its width (a 16:9 screen
	// spanning the full width), for use in vertical boxes.
	FitWidth bool `gd:"-"`

	bg     PanelContainer.Instance // surround (FwBk1), hidden for thumbnails
	screen PanelContainer.Instance // black screen + frame (FwScreen)
}

func (m *MonitorHost) GetMinimumSize() Vector2.XY {
	if !m.FitWidth {
		return Vector2.Zero
	}
	w := float64(m.AsControl().Size().X) - 2*m.Margin
	if w <= 0 {
		return Vector2.Zero
	}
	return Vector2.New(0, float32(math.Round(w/m.Aspect+2*m.Margin)))
}

// NewMonitorHost creates a host with EasyWorship's output-monitor defaults:
// 4px margin, window.bk1 surround and a framed black screen.
func NewMonitorHost() *MonitorHost {
	m := &MonitorHost{Aspect: 16.0 / 9.0, Margin: 4}
	c := m.AsControl()
	c.SetClipContents(true)
	c.SetMouseFilter(Control.MouseFilterPass)
	m.bg = panel(vBk1)
	m.screen = panel(vScreen)
	for _, p := range []PanelContainer.Instance{m.bg, m.screen} {
		p.AsControl().SetMouseFilter(Control.MouseFilterIgnore)
		m.AsNode().MoreArgs().AddChild(p.AsNode(), false, Node.InternalModeFront)
	}
	return m
}

// newThumbHost is a bare 16:9 host for thumbnails: no margin, no surround.
func newThumbHost() *MonitorHost {
	m := NewMonitorHost()
	m.Margin = 0
	m.bg.AsCanvasItem().SetVisible(false)
	m.screen.AsControl().SetThemeTypeVariation(vThumb)
	return m
}

// SetContent replaces the hosted content with c (pass a zero Instance to
// clear). The content goes inside the internal screen panel, which sizes it to
// the letterboxed rect. Controls added directly with AsNode().AddChild are
// moved there on the next layout pass.
func (m *MonitorHost) SetContent(c Control.Instance) {
	holder := m.screen.AsNode()
	for _, child := range holder.GetChildren() {
		if c != (Control.Instance{}) && child.ID() == c.AsNode().ID() {
			continue
		}
		holder.RemoveChild(child)
		child.QueueFree()
	}
	if c != (Control.Instance{}) {
		if parent := c.AsNode().GetParent(); parent == (Node.Instance{}) {
			holder.AddChild(c.AsNode())
		} else if parent.ID() != holder.ID() {
			c.AsNode().Reparent(holder)
		}
	}
	m.AsContainer().QueueSort()
}

// Content returns the hosted content (zero Instance when empty).
func (m *MonitorHost) Content() Control.Instance {
	for _, child := range m.screen.AsNode().GetChildren() {
		if c, ok := Object.As[Control.Instance](child); ok {
			return c
		}
	}
	return Control.Instance{}
}

// ScreenRect is the letterboxed screen rectangle in local coordinates.
func (m *MonitorHost) ScreenRect() uicore.Rect {
	size := m.AsControl().Size()
	return uicore.Letterbox(float64(size.X), float64(size.Y), m.Aspect, m.Margin)
}

func (m *MonitorHost) Notification(what Object.Notification) {
	if what != Container.NotificationSortChildren {
		return
	}
	size := m.AsControl().Size()
	r := m.ScreenRect()
	m.AsContainer().FitChildInRect(m.bg.AsControl(), rect(0, 0, size.X, size.Y))
	m.AsContainer().FitChildInRect(m.screen.AsControl(), rect(float32(r.X), float32(r.Y), float32(r.W), float32(r.H)))
	// Content must live inside the screen panel: a Go-drawn extension control
	// that a container moves AND resizes in one step was observed to keep
	// rendering at the old offset (graphics.gd + Godot 4.6). Inside the panel the
	// content only ever changes size, never position.
	for _, child := range m.AsNode().GetChildren() {
		if c, ok := Object.As[Control.Instance](child); ok {
			Callable.Defer(Callable.New(func() { m.SetContent(c) }))
		}
	}
	if m.FitWidth {
		m.AsControl().UpdateMinimumSize()
	}
}
