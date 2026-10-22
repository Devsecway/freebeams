package ui

import (
	"runtime"
	"slices"

	"freebeams/internal/models"
	"freebeams/internal/ui/uicore"

	"graphics.gd/classdb/ColorPicker"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventMouseButton"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Rect2i"
	"graphics.gd/variant/Transform2D"
	"graphics.gd/variant/Vector2"
	"graphics.gd/variant/Vector2i"
)

// The Preview Output monitor's context menu (right-click, or Control-click
// on a Mac; UI-SPEC §3.4) restyles the item in Preview on its own: Change
// Theme ▸, Background ▸, Text ▸, Edit Theme for This Item… and Revert to
// Global Theme (EasyWorship's names from the item inspector and the
// presentation properties). It is a dumb view: the app projects a
// PreviewMenuState onto it in render, and a click only reports the entry
// (OnAction / OnTheme). Its dialogs, ColorDialog and MediaPicker, are below.

// Preview menu entries, as OnAction reports them (and the preview-menu
// control action names them).
const (
	PreviewBackgroundMedia = "background-media" // Background ▸ Select Media…
	PreviewBackgroundColor = "background-color" // Background ▸ Solid Color…
	PreviewBackgroundReset = "background-reset" // Background ▸ Reset to Theme Background
	PreviewFontBigger      = "font-bigger"      // Text ▸ Increase Font
	PreviewFontSmaller     = "font-smaller"     // Text ▸ Decrease Font
	PreviewTextColor       = "text-color"       // Text ▸ Font Color…
	PreviewBold            = "bold"
	PreviewItalic          = "italic"
	PreviewOutline         = "outline"
	PreviewShadow          = "shadow"
	PreviewAlignLeft       = "align-left"
	PreviewAlignCenter     = "align-center"
	PreviewAlignRight      = "align-right"
	PreviewTextReset       = "text-reset" // Text ▸ Reset to Theme Text
	PreviewEditTheme       = "edit-theme" // Edit Theme for This Item…
	PreviewRevert          = "revert"     // Revert to Global Theme
)

// PreviewMenuState is what the menu shows, projected from the app's state.
type PreviewMenuState struct {
	// Enabled is false when Preview holds nothing the theme styles (empty,
	// media, a presentation): every entry is disabled then.
	Enabled bool
	Themes  []string // Change Theme ▸ entries
	Theme   int      // the checked entry (-1 none)
	// Custom enables Revert to Global Theme; BackgroundCustom and
	// TextCustom enable the submenus' Reset entries.
	Custom, BackgroundCustom, TextCustom bool
	Bold, Italic, Outline, Shadow        bool
	Align                                models.HAlign
}

// Menu ids: the top menu's entries, then the submenus' (unique across the
// menus, so Press can find any entry by item).
const (
	pmTheme = iota + 1
	pmBackground
	pmText
	pmEditTheme
	pmRevert
	pmSubBase = 100 // submenu entries: pmSubBase + index into previewSubEntries
)

// previewEntry is one submenu entry.
type previewEntry struct {
	item, label string
	menu        int  // pmBackground or pmText
	check       bool // a check box (Bold, Outline…)
	radio       bool // a radio item (the alignment)
	sepBefore   bool
}

var previewSubEntries = []previewEntry{
	{item: PreviewBackgroundMedia, label: "Select Media…", menu: pmBackground},
	{item: PreviewBackgroundColor, label: "Solid Color…", menu: pmBackground},
	{item: PreviewBackgroundReset, label: "Reset to Theme Background", menu: pmBackground, sepBefore: true},
	{item: PreviewFontBigger, label: "Increase Font", menu: pmText},
	{item: PreviewFontSmaller, label: "Decrease Font", menu: pmText},
	{item: PreviewTextColor, label: "Font Color…", menu: pmText, sepBefore: true},
	{item: PreviewBold, label: "Bold", menu: pmText, check: true, sepBefore: true},
	{item: PreviewItalic, label: "Italic", menu: pmText, check: true},
	{item: PreviewOutline, label: "Outline", menu: pmText, check: true},
	{item: PreviewShadow, label: "Shadow", menu: pmText, check: true},
	{item: PreviewAlignLeft, label: "Align Left", menu: pmText, radio: true, sepBefore: true},
	{item: PreviewAlignCenter, label: "Align Center", menu: pmText, radio: true},
	{item: PreviewAlignRight, label: "Align Right", menu: pmText, radio: true},
	{item: PreviewTextReset, label: "Reset to Theme Text", menu: pmText, sepBefore: true},
}

// PreviewMenu is the Preview Output monitor's context menu.
type PreviewMenu struct {
	OnAction func(item string) // an entry of PreviewMenu… constants
	OnTheme  func(i int)       // Change Theme ▸ entry i
	// OnOpened reports the menu opening (the app refreshes the projection).
	OnOpened func()

	host                              Control.Instance
	menu, themeMenu, bgMenu, textMenu PopupMenu.Instance
	state                             PreviewMenuState
	themes                            []string // the Change Theme ▸ entries built
}

// NewPreviewMenu builds the menu and attaches it to host's right-click (and
// Control-click on macOS). The host's left clicks are left alone.
func NewPreviewMenu(host Control.Instance) *PreviewMenu {
	m := &PreviewMenu{host: host, state: PreviewMenuState{Theme: -1}}
	m.menu = PopupMenu.New()
	m.menu.AsNode().SetName("PreviewMenu")
	m.themeMenu = m.submenu("PreviewThemeMenu")
	m.bgMenu = m.submenu("PreviewBackgroundMenu")
	m.textMenu = m.submenu("PreviewTextMenu")
	// Submenus are named children referenced by name, with explicit ids
	// (CLAUDE.md: add_submenu_node_item panics; id -1 becomes the index).
	m.menu.MoreArgs().AddSubmenuItem("Change Theme", "PreviewThemeMenu", pmTheme)
	m.menu.MoreArgs().AddSubmenuItem("Background", "PreviewBackgroundMenu", pmBackground)
	m.menu.MoreArgs().AddSubmenuItem("Text", "PreviewTextMenu", pmText)
	m.menu.AddSeparator()
	m.menu.MoreArgs().AddItem("Edit Theme for This Item…", pmEditTheme, 0)
	m.menu.AddSeparator()
	m.menu.MoreArgs().AddItem("Revert to Global Theme", pmRevert, 0)
	m.menu.OnIdPressed(m.pressedID)
	for i, e := range previewSubEntries {
		pm := m.bgMenu
		if e.menu == pmText {
			pm = m.textMenu
		}
		if e.sepBefore && pm.ItemCount() > 0 {
			pm.AddSeparator()
		}
		switch {
		case e.check:
			pm.MoreArgs().AddCheckItem(e.label, pmSubBase+i, 0)
		case e.radio:
			pm.MoreArgs().AddRadioCheckItem(e.label, pmSubBase+i, 0)
		default:
			pm.MoreArgs().AddItem(e.label, pmSubBase+i, 0)
		}
	}
	m.bgMenu.OnIdPressed(m.pressedID)
	m.textMenu.OnIdPressed(m.pressedID)
	m.themeMenu.OnIdPressed(func(id int) {
		if id >= 0 && id < len(m.themes) && m.state.Enabled && m.OnTheme != nil {
			m.OnTheme(id)
		}
	})
	// An internal child: MonitorHost moves its regular Control children into
	// the screen, and a menu must stay out of that.
	host.AsNode().MoreArgs().AddChild(m.menu.AsNode(), false, Node.InternalModeBack)
	host.OnGuiInput(m.guiInput)
	m.project()
	return m
}

func (m *PreviewMenu) submenu(name string) PopupMenu.Instance {
	pm := PopupMenu.New()
	pm.AsNode().SetName(name)
	m.menu.AsNode().AddChild(pm.AsNode())
	return pm
}

// guiInput opens the menu on a context click inside the monitor.
func (m *PreviewMenu) guiInput(ev InputEvent.Instance) {
	mb, ok := Object.As[InputEventMouseButton.Instance](ev)
	if !ok || !mb.AsInputEvent().IsPressed() || mb.DoubleClick() {
		return
	}
	ctrl := mb.AsInputEventWithModifiers().CtrlPressed()
	if !uicore.IsContextClick(int(mb.ButtonIndex()), ctrl, runtime.GOOS == "darwin") {
		return
	}
	m.popupAt(mb.AsInputEventMouse().Position())
	acceptEvent(m.host)
}

// popupAt shows the menu at local position p of the host.
func (m *PreviewMenu) popupAt(p Vector2.XY) {
	if m.OnOpened != nil {
		m.OnOpened()
	}
	at := Transform2D.Vector(p, m.host.AsCanvasItem().GetScreenTransform())
	w := m.menu.AsWindow()
	w.ResetSize()
	w.MoreArgs().Popup(Rect2i.PositionSize{Position: Vector2i.New(int(at.X), int(at.Y))})
}

// pressedID maps a top menu or submenu id to its entry.
func (m *PreviewMenu) pressedID(id int) {
	if !m.state.Enabled || m.OnAction == nil {
		return
	}
	switch {
	case id == pmEditTheme:
		m.OnAction(PreviewEditTheme)
	case id == pmRevert:
		m.OnAction(PreviewRevert)
	case id >= pmSubBase && id < pmSubBase+len(previewSubEntries):
		m.OnAction(previewSubEntries[id-pmSubBase].item)
	}
}

// Press runs entry item as a click on it does (the preview-menu control
// action); false for an unknown item. A disabled menu ignores it.
func (m *PreviewMenu) Press(item string) bool {
	switch item {
	case PreviewEditTheme:
		m.pressedID(pmEditTheme)
		return true
	case PreviewRevert:
		m.pressedID(pmRevert)
		return true
	}
	for i, e := range previewSubEntries {
		if e.item == item {
			m.pressedID(pmSubBase + i)
			return true
		}
	}
	return false
}

// PressTheme runs Change Theme ▸ entry i as a click does.
func (m *PreviewMenu) PressTheme(i int) {
	if m.state.Enabled && i >= 0 && i < len(m.themes) && m.OnTheme != nil {
		m.OnTheme(i)
	}
}

// SetState projects s onto the menu.
func (m *PreviewMenu) SetState(s PreviewMenuState) {
	s.Themes = slices.Clone(s.Themes)
	m.state = s
	m.project()
}

// State is the projection last applied.
func (m *PreviewMenu) State() PreviewMenuState { return m.state }

func (m *PreviewMenu) project() {
	s := m.state
	if !slices.Equal(m.themes, s.Themes) {
		m.themes = slices.Clone(s.Themes)
		m.themeMenu.Clear()
		for k, name := range m.themes {
			m.themeMenu.MoreArgs().AddRadioCheckItem(name, k, 0)
		}
		if len(m.themes) == 0 {
			m.themeMenu.MoreArgs().AddItem("No themes", -1, 0)
			m.themeMenu.SetItemDisabled(0, true)
		}
	}
	for k := range m.themes {
		m.themeMenu.SetItemChecked(k, k == s.Theme)
		m.themeMenu.SetItemDisabled(k, !s.Enabled)
	}
	for _, id := range []int{pmTheme, pmBackground, pmText, pmEditTheme} {
		m.menu.SetItemDisabled(m.menu.GetItemIndex(id), !s.Enabled)
	}
	m.menu.SetItemDisabled(m.menu.GetItemIndex(pmRevert), !s.Enabled || !s.Custom)
	for i, e := range previewSubEntries {
		pm := m.bgMenu
		if e.menu == pmText {
			pm = m.textMenu
		}
		idx := pm.GetItemIndex(pmSubBase + i)
		disabled := !s.Enabled
		switch e.item {
		case PreviewBackgroundReset:
			disabled = disabled || !s.BackgroundCustom
		case PreviewTextReset:
			disabled = disabled || !s.TextCustom
		}
		pm.SetItemDisabled(idx, disabled)
		switch e.item {
		case PreviewBold:
			pm.SetItemChecked(idx, s.Bold)
		case PreviewItalic:
			pm.SetItemChecked(idx, s.Italic)
		case PreviewOutline:
			pm.SetItemChecked(idx, s.Outline)
		case PreviewShadow:
			pm.SetItemChecked(idx, s.Shadow)
		case PreviewAlignLeft:
			pm.SetItemChecked(idx, s.Align == models.AlignLeft)
		case PreviewAlignCenter:
			pm.SetItemChecked(idx, s.Align == models.AlignCenter || s.Align == "")
		case PreviewAlignRight:
			pm.SetItemChecked(idx, s.Align == models.AlignRight)
		}
	}
	flushCalls(m.host)
}

// Open shows the menu over the middle of the monitor, as a right-click
// there does (automation and screenshots).
func (m *PreviewMenu) Open() {
	size := m.host.Size()
	m.popupAt(Vector2.New(size.X/2, size.Y/2))
}

// OpenSubmenu opens submenu name ("theme", "background" or "text") beside
// the open menu, where hovering its entry opens it; false for another name.
func (m *PreviewMenu) OpenSubmenu(name string) bool {
	id, sub := 0, PopupMenu.Nil
	switch name {
	case "theme":
		id, sub = pmTheme, m.themeMenu
	case "background":
		id, sub = pmBackground, m.bgMenu
	case "text":
		id, sub = pmText, m.textMenu
	default:
		return false
	}
	w := m.menu.AsWindow()
	n := m.menu.ItemCount()
	y := int(w.Position().Y)
	if idx := m.menu.GetItemIndex(id); idx >= 0 && n > 0 {
		y += int(w.Size().Y) * idx / n
	}
	sub.AsWindow().ResetSize()
	sub.AsWindow().MoreArgs().Popup(Rect2i.PositionSize{Position: Vector2i.New(int(w.Position().X+w.Size().X), y)})
	return true
}

// Close hides the menu and its submenus.
func (m *PreviewMenu) Close() {
	for _, pm := range []PopupMenu.Instance{m.themeMenu, m.bgMenu, m.textMenu, m.menu} {
		pm.AsWindow().Hide()
	}
}

// Visible reports whether the menu (or one of its submenus) is open.
func (m *PreviewMenu) Visible() bool {
	for _, pm := range []PopupMenu.Instance{m.menu, m.themeMenu, m.bgMenu, m.textMenu} {
		if pm.AsWindow().Visible() {
			return true
		}
	}
	return false
}

// Window is the popup window of the menu ("") or of a submenu ("theme",
// "background", "text"), for screenshots.
func (m *PreviewMenu) Window(sub string) Window.Instance {
	switch sub {
	case "theme":
		return m.themeMenu.AsWindow()
	case "background":
		return m.bgMenu.AsWindow()
	case "text":
		return m.textMenu.AsWindow()
	}
	return m.menu.AsWindow()
}

// MenuEntry is one entry as the menu shows it (for tests and the control API).
type MenuEntry struct {
	Label    string `json:"label"`
	Checked  bool   `json:"checked,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

// Entries lists a menu's entries as projected ("" the top menu, else a
// submenu name), separators left out.
func (m *PreviewMenu) Entries(sub string) []MenuEntry {
	pm := m.menu
	switch sub {
	case "theme":
		pm = m.themeMenu
	case "background":
		pm = m.bgMenu
	case "text":
		pm = m.textMenu
	}
	var out []MenuEntry
	for k := 0; k < pm.ItemCount(); k++ {
		if pm.IsItemSeparator(k) {
			continue
		}
		out = append(out, MenuEntry{Label: pm.GetItemText(k), Checked: pm.IsItemChecked(k), Disabled: pm.IsItemDisabled(k)})
	}
	return out
}

// ---- Color dialog -------------------------------------------------------------

// ColorDialog picks one color (Background ▸ Solid Color…, Text ▸ Font
// Color…). OK reports the color through OnPick; Cancel and Esc close it.
type ColorDialog struct {
	OnPick func(c models.RGBA)

	dlg    *dialogWindow
	picker ColorPicker.Instance
	color  models.RGBA
}

// NewColorDialog builds the dialog (MainWindow adds it with its dialogs).
func NewColorDialog() *ColorDialog {
	d := &ColorDialog{}
	d.dlg = newDialogWindow("Color", 340, 540)
	d.picker = ColorPicker.New()
	d.picker.SetEditAlpha(false)
	d.picker.SetCanAddSwatches(false)
	d.picker.SetPresetsVisible(false)
	d.picker.SetSamplerVisible(false) // the screen sampler needs Screen Recording permission
	d.picker.AsControl().SetSizeFlagsHorizontal(Control.SizeShrinkCenter)
	d.picker.OnColorChanged(func(c Color.RGBA) { d.color = fromGodotColor(c) })
	add(d.dlg.body, expand(d.picker.AsControl()))
	add(d.dlg.footer, spacer(), d.dlg.ok.AsControl(), d.dlg.cancel.AsControl())
	d.dlg.onOK = func() {
		d.dlg.hide()
		if d.OnPick != nil {
			c := d.color
			c.A = 1
			d.OnPick(c)
		}
	}
	d.dlg.win.AsNode().SetName("ColorDialog")
	return d
}

// Open shows the dialog titled title with c picked.
func (d *ColorDialog) Open(title string, c models.RGBA) {
	d.dlg.win.SetTitle(title)
	d.SetColor(c)
	d.dlg.popup()
}

// SetColor picks c as the operator would (the picker does not report it).
func (d *ColorDialog) SetColor(c models.RGBA) {
	d.color = c
	d.picker.SetColor(toGodotColor(c))
	flushCalls(d.picker.AsControl())
}

// Color is the picked color.
func (d *ColorDialog) Color() models.RGBA { return d.color }

// Visible reports whether the dialog is open.
func (d *ColorDialog) Visible() bool { return d.dlg.win.Visible() }

// AsWindow returns the dialog window.
func (d *ColorDialog) AsWindow() Window.Instance { return d.dlg.win }

// ---- Media picker -------------------------------------------------------------

// MediaPicker is Background ▸ Select Media…: the library's images and videos
// as thumbnails. OK (or a double-click) reports the chosen item through
// OnPick; thumbnails come from an injected factory.
type MediaPicker struct {
	OnPick func(i int)

	dlg   *dialogWindow
	grid  *SlideGrid
	empty Control.Instance
	sel   int
}

// NewMediaPicker builds the dialog (MainWindow adds it with its dialogs).
func NewMediaPicker() *MediaPicker {
	p := &MediaPicker{sel: -1}
	p.dlg = newDialogWindow("Select Media", 640, 460)
	p.grid = NewSlideGrid()
	p.grid.SetTileWidth(TileWidths[0])
	p.grid.OnSelect = func(i int) { p.sel = i }
	p.grid.OnActivate = func(i int) {
		p.sel = i
		p.dlg.onOK()
	}
	hint := label("No images or videos yet. Add them in the Media tab.", vDim)
	hint.SetClipText(false)
	p.empty = hint.AsControl()
	add(p.dlg.body, expand(surface(vGridBk, p.grid.AsControl())), p.empty)
	add(p.dlg.footer, spacer(), p.dlg.ok.AsControl(), p.dlg.cancel.AsControl())
	p.dlg.onOK = func() {
		if p.sel < 0 {
			return // nothing chosen: OK does nothing, like an empty selection
		}
		p.dlg.hide()
		if p.OnPick != nil {
			p.OnPick(p.sel)
		}
	}
	p.dlg.win.AsNode().SetName("MediaPicker")
	return p
}

// SetItems sets the tiles (their labels and badges).
func (p *MediaPicker) SetItems(items []SlideGridItem) {
	p.grid.SetItems(items)
	p.sel = -1
	p.empty.AsCanvasItem().SetVisible(len(items) == 0)
}

// SetThumbnailFactory sets the thumbnail factory.
func (p *MediaPicker) SetThumbnailFactory(f func(i int) Control.Instance) {
	p.grid.SetThumbnailFactory(f)
}

// Open shows the picker with nothing chosen.
func (p *MediaPicker) Open() {
	p.sel = -1
	p.grid.SetSelected(-1)
	p.dlg.popup()
	p.grid.AsControl().GrabFocus()
}

// Select chooses tile i as a click does (automation).
func (p *MediaPicker) Select(i int) {
	p.sel = i
	p.grid.SetSelected(i)
}

// Selected is the chosen tile (-1 none).
func (p *MediaPicker) Selected() int { return p.sel }

// Visible reports whether the picker is open.
func (p *MediaPicker) Visible() bool { return p.dlg.win.Visible() }

// AsWindow returns the dialog window.
func (p *MediaPicker) AsWindow() Window.Instance { return p.dlg.win }
