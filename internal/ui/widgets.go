package ui

import (
	"graphics.gd/classdb/BaseButton"
	"graphics.gd/classdb/BoxContainer"
	"graphics.gd/classdb/CanvasItem"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/DisplayServer"
	"graphics.gd/classdb/Font"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/HBoxContainer"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PanelContainer"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/StyleBox"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Texture2D"
	"graphics.gd/classdb/VBoxContainer"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Rect2"
	"graphics.gd/variant/Rect2i"
	"graphics.gd/variant/Transform2D"
	"graphics.gd/variant/Vector2"
	"graphics.gd/variant/Vector2i"
)

// ---- theme access for custom-drawn widgets --------------------------------

func fwColor(c Control.Instance, name string) Color.RGBA {
	return c.MoreArgs().GetThemeColor(name, tFw)
}

func fwFont(c Control.Instance, name string) Font.Instance {
	return c.MoreArgs().GetThemeFont(name, tFw)
}

func fwBox(c Control.Instance, name string) StyleBox.Instance {
	return c.MoreArgs().GetThemeStylebox(name, tFw)
}

func fwIcon(c Control.Instance, name string) Texture2D.Instance {
	return c.MoreArgs().GetThemeIcon(name, tIcons)
}

func alpha(c Color.RGBA, a float32) Color.RGBA {
	c.A *= a
	return c
}

func rect(x, y, w, h float32) Rect2.PositionSize {
	return Rect2.PositionSize{Position: Vector2.New(x, y), Size: Vector2.New(w, h)}
}

// drawText draws one line of text vertically centered in r, aligned
// horizontally within r's width and trimmed to fit it.
func drawText(ci CanvasItem.Instance, f Font.Instance, size int, r Rect2.PositionSize, text string, align GUI.HorizontalAlignment, c Color.RGBA) {
	if text == "" || r.Size.X <= 0 {
		return
	}
	asc := f.MoreArgs().GetAscent(size)
	h := f.MoreArgs().GetHeight(size)
	y := r.Position.Y + (r.Size.Y-h)/2 + asc
	text = fitText(f, size, text, r.Size.X)
	ci.MoreArgs().DrawString(f, Vector2.New(r.Position.X, Float.X(y)), text, align, r.Size.X, size, c,
		TextServer.JustificationKashida|TextServer.JustificationWordBound, TextServer.DirectionAuto, TextServer.OrientationHorizontal, 0)
}

// fitText trims text with an ellipsis so it fits width.
func fitText(f Font.Instance, size int, text string, width Float.X) string {
	measure := func(s string) Float.X {
		return f.MoreArgs().GetStringSize(s, GUI.HorizontalAlignmentLeft, -1, size, 0, TextServer.DirectionAuto, TextServer.OrientationHorizontal).X
	}
	if measure(text) <= width {
		return text
	}
	runes := []rune(text)
	for n := len(runes) - 1; n > 0; n-- {
		s := string(runes[:n]) + "…"
		if measure(s) <= width {
			return s
		}
	}
	return ""
}

// drawIcon draws a theme icon at its logical size (textures are rasterised at
// iconRasterScale) with its top-left at pos.
func drawIcon(ci CanvasItem.Instance, tex Texture2D.Instance, pos Vector2.XY, size float32, mod Color.RGBA) {
	if tex == (Texture2D.Instance{}) {
		return
	}
	ci.MoreArgs().DrawTextureRect(tex, rect(pos.X, pos.Y, size, size), false, mod, false)
}

// drawArrow draws the small ▾ dropdown triangle centered at (cx, cy).
func drawArrow(ci CanvasItem.Instance, cx, cy float32, c Color.RGBA) {
	ci.DrawColoredPolygon([]Vector2.XY{
		Vector2.New(cx-3, cy-1.5), Vector2.New(cx+3, cy-1.5), Vector2.New(cx, cy+1.75),
	}, c)
}

// ---- small node builders --------------------------------------------------

func hbox(sep int) HBoxContainer.Instance {
	b := HBoxContainer.New()
	b.AsControl().AddThemeConstantOverride("separation", sep)
	return b
}

func vbox(sep int) VBoxContainer.Instance {
	b := VBoxContainer.New()
	b.AsControl().AddThemeConstantOverride("separation", sep)
	return b
}

func panel(variation string) PanelContainer.Instance {
	p := PanelContainer.New()
	p.AsControl().SetThemeTypeVariation(variation)
	return p
}

func label(text, variation string) Label.Instance {
	l := Label.New()
	l.SetText(text)
	if variation != "" {
		l.AsControl().SetThemeTypeVariation(variation)
	}
	l.SetVerticalAlignment(GUI.VerticalAlignmentCenter)
	l.SetTextOverrunBehavior(TextServer.OverrunTrimEllipsis)
	l.SetClipText(true)
	return l
}

func spacer() Control.Instance {
	c := Control.New()
	c.SetSizeFlagsHorizontal(Control.SizeExpandFill)
	c.SetMouseFilter(Control.MouseFilterPass)
	return c
}

// hexpand makes a control fill the remaining horizontal space.
func hexpand(c Control.Instance) Control.Instance {
	c.SetSizeFlagsHorizontal(Control.SizeExpandFill)
	return c
}

func expand(c Control.Instance) Control.Instance {
	c.SetSizeFlagsHorizontal(Control.SizeExpandFill)
	c.SetSizeFlagsVertical(Control.SizeExpandFill)
	return c
}

func add(parent interface{ AsControl() Control.Instance }, children ...Control.Instance) {
	for _, c := range children {
		parent.AsControl().AsNode().AddChild(c.AsNode())
	}
}

func center(b BoxContainer.Instance) BoxContainer.Instance {
	b.SetAlignment(BoxContainer.AlignmentCenter)
	return b
}

// ---- main toolbar button (sd.toolbutton.style) ------------------------------

// toolButton is a 56 x 54 icon-over-label button of the main toolbar.
type toolButton struct {
	BaseButton.Extension[toolButton] `gd:"FwToolButton"`

	text     string
	icon     string
	dropdown bool
}

func newToolButton(text, icon string, dropdown, toggle bool) *toolButton {
	b := &toolButton{text: text, icon: icon, dropdown: dropdown}
	c := b.AsControl()
	c.SetCustomMinimumSize(Vector2.New(56, 54))
	c.SetSizeFlagsVertical(Control.SizeShrinkCenter)
	c.SetFocusMode(Control.FocusNone)
	c.SetTooltipText(text)
	b.AsBaseButton().SetToggleMode(toggle)
	return b
}

func (b *toolButton) Draw() {
	c := b.AsControl()
	ci := b.AsCanvasItem()
	size := c.Size()
	bb := b.AsBaseButton()
	switch bb.GetDrawMode() {
	case BaseButton.DrawHover:
		fwBox(c, "toolbutton_hot").Draw(ci, rect(0, 0, size.X, size.Y))
	case BaseButton.DrawPressed, BaseButton.DrawHoverPressed:
		fwBox(c, "toolbutton_sel").Draw(ci, rect(0, 0, size.X, size.Y))
	}
	mod := Color.RGBA{R: 1, G: 1, B: 1, A: 1}
	if bb.Disabled() {
		mod.A = 0.35
	}
	iconX := (size.X - 32) / 2
	drawIcon(ci, fwIcon(c, b.icon), Vector2.New(iconX, 4), 32, mod)
	if b.dropdown {
		drawArrow(ci, iconX+32+4, 24, alpha(fwColor(c, "glyph"), mod.A*0.85))
	}
	drawText(ci, fwFont(c, "regular"), 12, rect(2, 36, size.X-4, 16), b.text, GUI.HorizontalAlignmentCenter,
		alpha(fwColor(c, "toolbutton_text"), mod.A))
}

// ---- mini buttons (sd.button.mini.*) ----------------------------------------

type miniKind int

const (
	miniPlain      miniKind = iota // glyph only (28 x 24/27)
	miniDropdown                   // glyph + ▾ (40 x 24)
	miniSplitLeft                  // left half of a split view-sizer button (24 x 24)
	miniSplitRight                 // ▾ right half (16 x 24)
	miniText                       // text label (e.g. Go Back)
)

// miniButton is a small toolbar button. It has no background at rest and a
// translucent highlight when hovered or pressed (EasyWorship's mini style).
type miniButton struct {
	BaseButton.Extension[miniButton] `gd:"FwMiniButton"`

	kind miniKind
	icon string
	text string
}

func newMiniButton(kind miniKind, icon string, w, h float32, tooltip string) *miniButton {
	b := &miniButton{kind: kind, icon: icon}
	c := b.AsControl()
	c.SetCustomMinimumSize(Vector2.New(w, h))
	c.SetSizeFlagsVertical(Control.SizeShrinkCenter)
	c.SetFocusMode(Control.FocusNone)
	c.SetTooltipText(tooltip)
	return b
}

func (b *miniButton) Draw() {
	c := b.AsControl()
	ci := b.AsCanvasItem()
	size := c.Size()
	bb := b.AsBaseButton()
	prefix := "mini_"
	switch b.kind {
	case miniSplitLeft:
		prefix = "mini_l_"
	case miniSplitRight:
		prefix = "mini_r_"
	}
	switch bb.GetDrawMode() {
	case BaseButton.DrawHover:
		fwBox(c, prefix+"hot").Draw(ci, rect(0, 0, size.X, size.Y))
	case BaseButton.DrawPressed, BaseButton.DrawHoverPressed:
		fwBox(c, prefix+"sel").Draw(ci, rect(0, 0, size.X, size.Y))
	}
	mod := Color.RGBA{R: 1, G: 1, B: 1, A: 1}
	if bb.Disabled() {
		mod.A = 0.35
	}
	glyph := alpha(fwColor(c, "glyph"), mod.A)
	switch b.kind {
	case miniDropdown:
		x := (size.X - 16 - 9) / 2
		drawIcon(ci, fwIcon(c, b.icon), Vector2.New(x, (size.Y-16)/2), 16, mod)
		drawArrow(ci, x+16+5, size.Y/2+0.5, glyph)
	case miniSplitRight:
		drawArrow(ci, size.X/2, size.Y/2+0.5, glyph)
	case miniText:
		drawText(ci, fwFont(c, "regular"), fontCaption+1, rect(4, 0, size.X-8, size.Y), b.text, GUI.HorizontalAlignmentCenter, alpha(fwColor(c, "text"), mod.A))
	default:
		drawIcon(ci, fwIcon(c, b.icon), Vector2.New((size.X-16)/2, (size.Y-16)/2), 16, mod)
	}
}

// ---- etched dividers (sd.divider.button[.mini].style) ----------------------

type divider struct {
	Control.Extension[divider] `gd:"FwDivider"`

	mini bool
}

// newDivider returns a 2px etched divider: 2 x 50 on the main toolbar
// (opacity 0.4, margins 4) or 2 x 22 between mini buttons (margins 2).
func newDivider(mini bool) *divider {
	d := &divider{mini: mini}
	c := d.AsControl()
	if mini {
		c.SetCustomMinimumSize(Vector2.New(2, 22))
	} else {
		c.SetCustomMinimumSize(Vector2.New(2, 50))
	}
	c.SetSizeFlagsVertical(Control.SizeShrinkCenter)
	c.SetMouseFilter(Control.MouseFilterIgnore)
	return d
}

func (d *divider) Draw() {
	c := d.AsControl()
	ci := d.AsCanvasItem()
	h := c.Size().Y
	lead, trail, a := "div_lead", "div_trail", float32(0.4)
	if d.mini {
		lead, trail, a = "divmini_lead", "divmini_trail", 1
	}
	ci.DrawRect(rect(0, 0, 1, h), alpha(fwColor(c, lead), a))
	ci.DrawRect(rect(1, 0, 1, h), alpha(fwColor(c, trail), a))
}

// withMargins wraps a control in horizontal margins (the dividers' margins).
func withMargins(c Control.Instance, left, right int) Control.Instance {
	box := hbox(0)
	box.AsControl().SetMouseFilter(Control.MouseFilterIgnore)
	if left > 0 {
		l := Control.New()
		l.SetCustomMinimumSize(Vector2.New(float32(left), 0))
		l.SetMouseFilter(Control.MouseFilterIgnore)
		box.AsNode().AddChild(l.AsNode())
	}
	box.AsNode().AddChild(c.AsNode())
	if right > 0 {
		r := Control.New()
		r.SetCustomMinimumSize(Vector2.New(float32(right), 0))
		r.SetMouseFilter(Control.MouseFilterIgnore)
		box.AsNode().AddChild(r.AsNode())
	}
	return box.AsControl()
}

// popupBelow opens a popup menu under a control (dropdown buttons).
func popupBelow(m PopupMenu.Instance, c Control.Instance) {
	st := c.AsCanvasItem().GetScreenTransform()
	p := Transform2D.Vector(Vector2.New(0, c.Size().Y), st)
	m.AsWindow().ResetSize()
	m.AsWindow().MoreArgs().Popup(Rect2i.PositionSize{Position: Vector2i.New(int(p.X), int(p.Y))})
}

// popupAtMouse opens a context menu at the mouse position.
func popupAtMouse(m PopupMenu.Instance) {
	m.AsWindow().ResetSize()
	m.AsWindow().MoreArgs().Popup(Rect2i.PositionSize{Position: DisplayServer.MouseGetPosition()})
}
