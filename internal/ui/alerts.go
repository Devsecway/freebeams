package ui

import (
	"fmt"
	"strings"

	"freebeams/internal/models"

	"graphics.gd/classdb/Button"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/GridContainer"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/MenuButton"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/OptionButton"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/SpinBox"
	"graphics.gd/classdb/TextEdit"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Tree"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// Alerts: the toolbar's Alerts ▾ menu and the dialogs behind it, after
// EasyWorship's Form_AlertMgr ("Live Alert Editor"), Form_AlertEdit ("Edit
// Alert"), Form_AlertTagFill ("Fill Alert Text Tokens") and the nursery half
// of Frame_AlertsList, plus a Countdown dialog for the Service Interval. All
// are dumb views: setters project the app's state, On… callbacks report
// what the operator asked for.

// ---- Alerts ▾ menu -----------------------------------------------------------------

// AlertsMenuKind is an entry of the Alerts ▾ menu.
type AlertsMenuKind int

const (
	AlertsMenuAlert     AlertsMenuKind = iota // a saved alert (Index)
	AlertsMenuNew                             // New Alert…
	AlertsMenuManager                         // Alert Manager…
	AlertsMenuNursery                         // Nursery…
	AlertsMenuCountdown                       // Countdown…
	AlertsMenuClock                           // Clock (check item)
	AlertsMenuClear                           // Clear All Alerts
)

// ParseAlertsMenu reads a menu entry name for the control API.
func ParseAlertsMenu(name string) (AlertsMenuKind, bool) {
	k, ok := map[string]AlertsMenuKind{
		"alert": AlertsMenuAlert, "new": AlertsMenuNew, "manager": AlertsMenuManager, "nursery": AlertsMenuNursery,
		"countdown": AlertsMenuCountdown, "clock": AlertsMenuClock, "clear": AlertsMenuClear,
	}[strings.ToLower(strings.TrimSpace(name))]
	return k, ok
}

// alertsMenuSaved is the first id of the saved alerts (id = alertsMenuSaved + index).
const alertsMenuSaved = 100

// SetAlertsMenu lists the saved alerts in the Alerts ▾ menu and checks the
// Clock entry.
func (t *MainToolbar) SetAlertsMenu(names []string, clockOn bool) {
	if t.alertsMenu == (PopupMenu.Instance{}) {
		t.alertsMenu = PopupMenu.New()
		t.alertsMenu.AsNode().SetName("AlertsMenu")
		t.alertsMenu.OnIdPressed(func(id int) {
			if t.OnAlertsMenu == nil {
				return
			}
			if id >= alertsMenuSaved {
				t.OnAlertsMenu(AlertsMenuAlert, id-alertsMenuSaved)
				return
			}
			t.OnAlertsMenu(AlertsMenuKind(id), -1)
		})
		t.root.AsNode().AddChild(t.alertsMenu.AsNode())
	}
	m := t.alertsMenu
	m.Clear()
	if len(names) == 0 {
		m.MoreArgs().AddItem("No saved alerts", alertsMenuSaved-1, 0)
		m.SetItemDisabled(0, true)
	}
	for i, n := range names {
		m.MoreArgs().AddItem(n, alertsMenuSaved+i, 0)
	}
	m.AddSeparator()
	m.MoreArgs().AddItem("New Alert…", int(AlertsMenuNew), 0)
	m.MoreArgs().AddItem("Alert Manager…", int(AlertsMenuManager), 0)
	m.AddSeparator()
	m.MoreArgs().AddItem("Nursery…", int(AlertsMenuNursery), 0)
	m.MoreArgs().AddItem("Countdown…", int(AlertsMenuCountdown), 0)
	m.MoreArgs().AddCheckItem("Clock", int(AlertsMenuClock), 0)
	m.SetItemChecked(m.GetItemIndex(int(AlertsMenuClock)), clockOn)
	m.AddSeparator()
	m.MoreArgs().AddItem("Clear All Alerts", int(AlertsMenuClear), 0)
}

// popupAlerts opens the Alerts ▾ menu under its button (false before the
// app filled it).
func (t *MainToolbar) popupAlerts() bool {
	if t.alertsMenu == (PopupMenu.Instance{}) {
		return false
	}
	popupBelow(t.alertsMenu, t.buttons[ActionAlerts].AsControl())
	return true
}

// AlertsMenuWindow is the Alerts ▾ menu's popup window (screenshots).
func (t *MainToolbar) AlertsMenuWindow() Window.Instance {
	if t.alertsMenu == (PopupMenu.Instance{}) {
		return Window.Nil
	}
	return t.alertsMenu.AsWindow()
}

// PopupAlerts opens the Alerts ▾ menu as a click on the button does.
func (t *MainToolbar) PopupAlerts() bool { return t.popupAlerts() }

// HideAlertsMenu closes the Alerts ▾ menu.
func (t *MainToolbar) HideAlertsMenu() {
	if t.alertsMenu != (PopupMenu.Instance{}) {
		t.alertsMenu.AsWindow().Hide()
	}
}

// AlertsMenuEntries lists the Alerts ▾ menu's entries ("" for a separator)
// and the Clock entry's check.
func (t *MainToolbar) AlertsMenuEntries() (entries []string, clock bool) {
	m := t.alertsMenu
	if m == (PopupMenu.Instance{}) {
		return nil, false
	}
	for i := range m.ItemCount() {
		entries = append(entries, m.GetItemText(i))
	}
	if idx := m.GetItemIndex(int(AlertsMenuClock)); idx >= 0 {
		clock = m.IsItemChecked(idx)
	}
	return entries, clock
}

// AlertsMenuVisible reports whether the Alerts ▾ menu is open.
func (t *MainToolbar) AlertsMenuVisible() bool {
	return t.alertsMenu != (PopupMenu.Instance{}) && t.alertsMenu.AsWindow().Visible()
}

// ---- shared bits ------------------------------------------------------------------

func optionOf(items []string) OptionButton.Instance {
	o := OptionButton.New()
	for _, it := range items {
		o.AddItem(it)
	}
	o.AsControl().SetCustomMinimumSize(Vector2.New(145, 24))
	return o
}

func button(text string, w float32, fn func()) Button.Instance {
	b := Button.New()
	b.SetText(text)
	b.AsControl().SetCustomMinimumSize(Vector2.New(w, 25))
	b.AsBaseButton().OnPressed(fn)
	return b
}

// dialogHeader is EasyWorship's Layout_Title: a large icon, a title and a
// subtitle.
func dialogHeader(icon, title, subtitle string) Control.Instance {
	row := hbox(12)
	ic := newGlyph(icon, 32)
	text := vbox(0)
	text.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	t := label(title, vTitle)
	t.SetClipText(false)
	t.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	s := label(subtitle, vHint)
	s.SetClipText(false)
	s.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	add(text, t.AsControl(), s.AsControl())
	add(row, ic, text.AsControl())
	return row.AsControl()
}

// newGlyph is a size × size control showing a named icon in the glyph color.
func newGlyph(name string, size float32) Control.Instance {
	c := Control.New()
	c.SetCustomMinimumSize(Vector2.New(size, size))
	c.SetMouseFilter(Control.MouseFilterIgnore)
	ci := c.AsCanvasItem()
	ci.OnDraw(func() {
		drawGlyph(ci, c, name, Vector2.New(0, 0), size, fwColor(c, "glyph"))
		ci.Visible() // flush the buffered draw calls inside the draw pass
	})
	return c
}

// alertWrap is a multi-line label that wraps at its width.
func alertWrap(variation string) Label.Instance {
	l := label("", variation)
	l.SetAutowrapMode(TextServer.AutowrapWordSmart)
	l.SetClipText(false)
	l.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	l.SetVerticalAlignment(GUI.VerticalAlignmentTop)
	return l
}

// AlertSummary describes an alert's display options in one line
// ("Scrolling · 3 times · Center").
func AlertSummary(a models.Alert) string {
	a = a.Normalized()
	parts := []string{a.Display.Label()}
	switch a.End {
	case models.AlertEndCount:
		if a.Count == 1 {
			parts = append(parts, "once")
		} else {
			parts = append(parts, fmt.Sprintf("%d times", a.Count))
		}
	case models.AlertEndManual:
		parts = append(parts, "until removed")
	case models.AlertEndToken:
		parts = append(parts, "until the countdown ends")
	case models.AlertEndDuration:
		parts = append(parts, fmt.Sprintf("for %d s", a.Seconds))
	}
	if a.Display != models.AlertScroll {
		parts = append(parts, strings.ToUpper(string(a.Align[:1]))+string(a.Align[1:]))
	}
	if toks := models.AlertTokens(a.Message); len(toks) > 0 {
		parts = append(parts, "asks for "+strings.Join(toks, ", "))
	}
	return strings.Join(parts, " · ")
}

// ---- Alert Manager ("Live Alert Editor") -----------------------------------------------

// AlertManager lists the saved alerts with Add / Delete under the list and
// Use / Edit… under the selected alert's message (Form_AlertMgr).
type AlertManager struct {
	OnSelect func(i int)
	OnAdd    func()
	OnDelete func(i int)
	OnUse    func(i int)
	OnEdit   func(i int)
	OnClose  func()

	dlg     *dialogWindow
	list    Tree.Instance
	name    Label.Instance
	message Label.Instance
	info    Label.Instance
	use     Button.Instance
	edit    Button.Instance
	del     Button.Instance

	alerts []models.Alert
	sel    int
}

// NewAlertManager builds the dialog.
func NewAlertManager() *AlertManager {
	m := &AlertManager{sel: -1}
	m.dlg = newDialogWindow("Live Alert Editor", 660, 400)
	m.dlg.onOK = m.dlg.close
	m.dlg.onClose = func() {
		if m.OnClose != nil {
			m.OnClose()
		}
	}
	m.dlg.ok.SetText("Close")

	m.list = Tree.New()
	m.list.SetHideRoot(true)
	m.list.AsControl().SetCustomMinimumSize(Vector2.New(200, 0))
	m.list.AsControl().SetSizeFlagsVertical(Control.SizeExpandFill)
	m.list.OnItemSelected(func() {
		if i := treeIndex(m.list.GetSelected()); i >= 0 && m.OnSelect != nil {
			m.OnSelect(i)
		}
	})
	m.list.OnItemActivated(func() {
		if i := treeIndex(m.list.GetSelected()); i >= 0 && m.OnUse != nil {
			m.OnUse(i)
		}
	})
	addBtn := button("Add", 64, func() {
		if m.OnAdd != nil {
			m.OnAdd()
		}
	})
	m.del = button("Delete", 64, func() {
		if m.sel >= 0 && m.OnDelete != nil {
			m.OnDelete(m.sel)
		}
	})
	listFoot := hbox(6)
	add(listFoot, addBtn.AsControl(), m.del.AsControl())
	left := vbox(6)
	left.AsControl().SetSizeFlagsVertical(Control.SizeExpandFill)
	add(left, m.list.AsControl(), listFoot.AsControl())

	m.name = label("", vTitle)
	m.name.AsControl().SetCustomMinimumSize(Vector2.New(0, 31))
	m.message = alertWrap("")
	expand(m.message.AsControl())
	box := panel(vBk1)
	expand(box.AsControl())
	add(box, padded(m.message.AsControl(), 10, 8, 10, 8))
	m.info = alertWrap(vHint)
	m.use = button("Use", 75, func() {
		if m.sel >= 0 && m.OnUse != nil {
			m.OnUse(m.sel)
		}
	})
	m.edit = button("Edit...", 75, func() {
		if m.sel >= 0 && m.OnEdit != nil {
			m.OnEdit(m.sel)
		}
	})
	bottom := hbox(10)
	add(bottom, m.use.AsControl(), spacer(), m.edit.AsControl())
	right := vbox(8)
	expand(right.AsControl())
	add(right, m.name.AsControl(), box.AsControl(), m.info.AsControl(), bottom.AsControl())

	row := hbox(20)
	expand(row.AsControl())
	add(row, left.AsControl(), right.AsControl())
	add(m.dlg.body, dialogHeader("alerts", "Live Alert Editor", "Manage existing or create new alerts"), fixed(0, 4), row.AsControl())
	add(m.dlg.footer, spacer(), m.dlg.ok.AsControl())
	Node.Instance(m.dlg.cancel.AsNode()).QueueFree()
	m.dlg.cancel = Button.Nil
	m.dlg.win.AsNode().SetName("AlertManager")
	m.render()
	return m
}

// SetAlerts lists the saved alerts with sel selected (-1: none).
func (m *AlertManager) SetAlerts(alerts []models.Alert, sel int) {
	m.alerts, m.sel = alerts, sel
	m.render()
}

func (m *AlertManager) render() {
	quietly(m.list.AsControl(), func() {
		m.list.Clear()
		root := m.list.CreateItem()
		for _, a := range m.alerts {
			it := m.list.MoreArgs().CreateItem(root, -1)
			it.SetText(0, a.Name)
			it.SetCustomMinimumHeight(26)
		}
		selectTreeRow(m.list, m.sel)
	})
	m.list.AsCanvasItem().QueueRedraw()
	has := m.sel >= 0 && m.sel < len(m.alerts)
	if has {
		a := m.alerts[m.sel]
		m.name.SetText(a.Name)
		m.message.SetText(a.Message)
		m.info.SetText(AlertSummary(a))
	} else {
		m.name.SetText("No alert selected")
		m.message.SetText("Choose an alert on the left, or Add one.")
		m.info.SetText("")
	}
	for _, b := range []Button.Instance{m.use, m.edit, m.del} {
		b.AsBaseButton().SetDisabled(!has)
	}
	flushCalls(m.name.AsControl())
}

// Open shows the dialog.
func (m *AlertManager) Open() { m.dlg.popup() }

// Hide closes the dialog without OnClose.
func (m *AlertManager) Hide() { m.dlg.hide() }

// Visible reports whether the dialog is open.
func (m *AlertManager) Visible() bool { return m.dlg.win.Visible() }

// AsWindow returns the dialog window.
func (m *AlertManager) AsWindow() Window.Instance { return m.dlg.win }

// ---- Edit Alert ---------------------------------------------------------------------

// Display Count entries after 1..9 (Form_AlertEdit's ComboBox_DisplayCount,
// plus a duration).
var alertCountItems = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "Until I Remove", "Until Token Expiration", "For a Duration"}

// alertTokenItems is the Tokens ▾ menu: operator tokens, then the dynamic ones.
var alertTokenItems = []struct{ label, token string }{
	{"Name", "{name}"}, {"Number", "{number}"}, {"Plate", "{plate}"}, {"Room", "{room}"},
	{"", ""},
	{"Countdown (remaining time)", "{countdown}"}, {"Clock (time of day)", "{clock}"},
}

// AlertEditor is the New / Edit Alert dialog: name, message with a Tokens ▾
// menu, Display Method, Display Count, Text Alignment and a duration. OK
// saves the alert; Show puts it on the output without saving it.
type AlertEditor struct {
	OnSave func(models.Alert)
	OnShow func(models.Alert)

	dlg      *dialogWindow
	name     LineEdit.Instance
	message  TextEdit.Instance
	display  OptionButton.Instance
	count    OptionButton.Instance
	align    OptionButton.Instance
	seconds  SpinBox.Instance
	alignLbl Label.Instance
	secsLbl  Label.Instance
	errLbl   Label.Instance

	alert   models.Alert
	errText string
	aligns  []models.HAlign
}

// NewAlertEditor builds the dialog.
func NewAlertEditor() *AlertEditor {
	e := &AlertEditor{aligns: []models.HAlign{models.AlignLeft, models.AlignCenter, models.AlignRight}}
	e.dlg = newDialogWindow("Edit Alert", 578, 400)
	e.dlg.onOK = e.save

	e.name = LineEdit.New()
	e.name.SetPlaceholderText("Required to save")
	e.name.OnTextChanged(func(string) {
		if e.errText != "" {
			e.errText = "" // typing answers the validation message
			e.render()
		}
	})
	e.name.AsControl().SetCustomMinimumSize(Vector2.New(233, 24))
	nameRow := hbox(8)
	nl := inlineLabel("Alert Name")
	nl.AsControl().SetCustomMinimumSize(Vector2.New(68, 0))
	add(nameRow, nl.AsControl(), e.name.AsControl())

	tokens := MenuButton.New()
	tokens.AsButton().SetText("Tokens ▾")
	tokens.AsButton().SetFlat(false)
	pm := tokens.GetPopup()
	for i, t := range alertTokenItems {
		if t.label == "" {
			pm.AddSeparator()
			continue
		}
		pm.MoreArgs().AddItem(t.label+"   "+t.token, i, 0)
	}
	pm.OnIdPressed(func(id int) {
		if id >= 0 && id < len(alertTokenItems) {
			e.message.InsertTextAtCaret(alertTokenItems[id].token)
			e.message.AsControl().GrabFocus()
		}
	})
	msgRow := hbox(8)
	add(msgRow, inlineLabel("Message").AsControl(), spacer(), tokens.AsControl())

	e.message = TextEdit.New()
	e.message.SetPlaceholderText("The owner of the vehicle with plate {plate}, please move it.")
	e.message.SetWrapMode(TextEdit.LineWrappingBoundary)
	expand(e.message.AsControl())

	e.display = optionOf([]string{"Static", "Scrolling", "Scroll when needed"})
	e.display.OnItemSelected(func(int) { e.render() })
	e.count = optionOf(alertCountItems)
	e.count.OnItemSelected(func(int) { e.render() })
	e.align = optionOf([]string{"Left", "Center", "Right"})
	e.seconds = SpinBox.New()
	e.seconds.AsRange().SetMinValue(1)
	e.seconds.AsRange().SetMaxValue(3600)
	e.seconds.SetSuffix("s")
	e.seconds.AsControl().SetCustomMinimumSize(Vector2.New(145, 24))
	grid := GridContainer.New()
	grid.SetColumns(4)
	grid.AsControl().AddThemeConstantOverride("h_separation", 12)
	grid.AsControl().AddThemeConstantOverride("v_separation", 6)
	e.alignLbl = inlineLabel("Text Alignment")
	e.secsLbl = inlineLabel("Duration")
	add(grid, inlineLabel("Display Method").AsControl(), e.display.AsControl(), inlineLabel("Display Count").AsControl(), e.count.AsControl(),
		e.alignLbl.AsControl(), e.align.AsControl(), e.secsLbl.AsControl(), e.seconds.AsControl())

	add(e.dlg.body, nameRow.AsControl(), msgRow.AsControl(), e.message.AsControl(), fixed(0, 2), grid.AsControl())
	e.errLbl = label("", vError)
	e.errLbl.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	show := button("Show", 80, e.PressShow)
	for _, b := range []Button.Instance{e.dlg.ok, e.dlg.cancel} {
		b.AsControl().SetCustomMinimumSize(Vector2.New(80, 25))
	}
	add(e.dlg.footer, show.AsControl(), e.errLbl.AsControl(), e.dlg.ok.AsControl(), e.dlg.cancel.AsControl())
	e.dlg.win.AsNode().SetName("AlertEditor")
	return e
}

// Open shows the dialog for an alert (ID 0: a new one).
func (e *AlertEditor) Open(a models.Alert) {
	e.alert = a.Normalized()
	e.errText = ""
	title := "Edit Alert"
	if a.ID == 0 {
		title = "New Alert"
	}
	e.dlg.win.SetTitle(title)
	e.name.SetText(e.alert.Name)
	e.message.SetText(a.Message)
	e.display.Select(indexOf(models.AlertDisplays(), e.alert.Display, 1))
	e.count.Select(countIndex(e.alert))
	e.align.Select(indexOf(e.aligns, e.alert.Align, 1))
	e.seconds.AsRange().SetValueNoSignal(Float.X(max(e.alert.Seconds, 30)))
	e.render()
	e.dlg.popup()
	e.message.AsControl().GrabFocus()
}

func countIndex(a models.Alert) int {
	switch a.End {
	case models.AlertEndManual:
		return 9
	case models.AlertEndToken:
		return 10
	case models.AlertEndDuration:
		return 11
	}
	return max(0, min(a.Count, models.MaxAlertCount)-1)
}

// SetFields fills the dialog's fields (the control API types into it).
func (e *AlertEditor) SetFields(a models.Alert) {
	e.errText = ""
	e.name.SetText(a.Name)
	e.message.SetText(a.Message)
	a = a.Normalized()
	e.display.Select(indexOf(models.AlertDisplays(), a.Display, 1))
	e.count.Select(countIndex(a))
	e.align.Select(indexOf(e.aligns, a.Align, 1))
	e.seconds.AsRange().SetValueNoSignal(Float.X(max(a.Seconds, 1)))
	e.render()
}

// read collects the fields; a saved alert needs a name.
func (e *AlertEditor) read(needName bool) (models.Alert, bool) {
	a := e.alert
	a.Name = strings.TrimSpace(e.name.Text())
	a.Message = strings.TrimSpace(e.message.Text())
	a.Display = models.AlertDisplays()[max(0, e.display.Selected())]
	a.Align = e.aligns[max(0, e.align.Selected())]
	switch c := e.count.Selected(); {
	case c >= 0 && c < 9:
		a.End, a.Count = models.AlertEndCount, c+1
	case c == 9:
		a.End = models.AlertEndManual
	case c == 10:
		a.End, a.EndToken = models.AlertEndToken, models.TokenCountdown
	default:
		a.End, a.Seconds = models.AlertEndDuration, int(e.seconds.AsRange().Value())
	}
	switch {
	case a.Message == "":
		e.errText = "Type a message."
	case needName && a.Name == "":
		e.errText = "Name the alert to save it."
	default:
		e.errText = ""
	}
	e.render()
	return a.Normalized(), e.errText == ""
}

// PressShow is the Show button: the alert goes on screen without being saved.
func (e *AlertEditor) PressShow() {
	a, ok := e.read(false)
	if !ok {
		return
	}
	e.dlg.hide()
	if e.OnShow != nil {
		e.OnShow(a)
	}
}

func (e *AlertEditor) save() {
	a, ok := e.read(true)
	if !ok {
		return
	}
	e.dlg.hide()
	if e.OnSave != nil {
		e.OnSave(a)
	}
}

func (e *AlertEditor) render() {
	static := e.display.Selected() != 1
	e.align.AsBaseButton().SetDisabled(!static)
	e.alignLbl.AsCanvasItem().SetModulate(dimIf(!static))
	dur := e.count.Selected() == 11
	e.seconds.SetEditable(dur)
	e.secsLbl.AsCanvasItem().SetModulate(dimIf(!dur))
	e.errLbl.SetText(e.errText)
	flushCalls(e.errLbl.AsControl())
}

// Visible reports whether the dialog is open.
func (e *AlertEditor) Visible() bool { return e.dlg.win.Visible() }

// Hide closes the dialog.
func (e *AlertEditor) Hide() { e.dlg.hide() }

// Error is the dialog's validation message.
func (e *AlertEditor) Error() string { return e.errText }

// AsWindow returns the dialog window.
func (e *AlertEditor) AsWindow() Window.Instance { return e.dlg.win }

// ---- Fill Alert Text Tokens --------------------------------------------------------------

// AlertTagFill asks for an alert's token values with a live preview
// (Form_AlertTagFill); USE shows the filled alert.
type AlertTagFill struct {
	OnUse func(values map[string]string)

	dlg     *dialogWindow
	grid    GridContainer.Instance
	preview Label.Instance
	edits   []LineEdit.Instance
	tokens  []string
	message string
	errLbl  Label.Instance
	errText string
}

// NewAlertTagFill builds the dialog.
func NewAlertTagFill() *AlertTagFill {
	f := &AlertTagFill{}
	f.dlg = newDialogWindow("Fill Alert Text Tokens", 509, 330)
	f.dlg.ok.SetText("USE")
	f.dlg.onOK = func() {
		values := f.Values()
		if _, missing := models.FillAlertTokens(f.message, values); len(missing) > 0 {
			f.errText = "Fill in " + strings.Join(missing, ", ") + "."
			f.renderPreview()
			return
		}
		f.dlg.hide()
		if f.OnUse != nil {
			f.OnUse(values)
		}
	}
	f.grid = GridContainer.New()
	f.grid.SetColumns(2)
	f.grid.AsControl().AddThemeConstantOverride("h_separation", 10)
	f.grid.AsControl().AddThemeConstantOverride("v_separation", 6)
	f.grid.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	list := panel(vBk1)
	expand(list.AsControl())
	add(list, padded(f.grid.AsControl(), 10, 10, 10, 10))
	f.preview = alertWrap("")
	f.preview.AsControl().SetCustomMinimumSize(Vector2.New(0, 80))
	box := panel(vBk0)
	box.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	add(box, padded(f.preview.AsControl(), 10, 8, 10, 8))
	add(f.dlg.body, list.AsControl(), inlineLabel("Preview").AsControl(), box.AsControl())
	for _, b := range []Button.Instance{f.dlg.ok, f.dlg.cancel} {
		b.AsControl().SetCustomMinimumSize(Vector2.New(80, 25))
	}
	f.errLbl = label("", vError)
	f.errLbl.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	add(f.dlg.footer, f.errLbl.AsControl(), f.dlg.ok.AsControl(), f.dlg.cancel.AsControl())
	f.dlg.win.AsNode().SetName("AlertTagFill")
	return f
}

// Open asks for the tokens of message (models.AlertTokens).
func (f *AlertTagFill) Open(title, message string, tokens []string) {
	f.message, f.tokens = message, tokens
	for _, c := range f.grid.AsNode().GetChildren() {
		f.grid.AsNode().RemoveChild(c)
		c.QueueFree()
	}
	f.edits = f.edits[:0]
	for _, t := range tokens {
		l := label(strings.ToUpper(t[:1])+t[1:], "")
		l.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
		l.AsControl().SetCustomMinimumSize(Vector2.New(110, 0))
		ed := LineEdit.New()
		ed.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
		ed.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
		ed.OnTextChanged(func(string) {
			f.errText = "" // typing answers the "Fill in …" message
			f.renderPreview()
		})
		ed.OnTextSubmitted(func(string) { f.dlg.onOK() })
		f.edits = append(f.edits, ed)
		add(f.grid, l.AsControl(), ed.AsControl())
	}
	f.dlg.win.SetTitle("Fill Alert Text Tokens — " + title)
	f.errText = ""
	f.renderPreview()
	f.dlg.popup()
	if len(f.edits) > 0 {
		f.edits[0].AsControl().GrabFocus()
	}
}

// Values returns the typed values by token.
func (f *AlertTagFill) Values() map[string]string {
	v := map[string]string{}
	for i, ed := range f.edits {
		if i < len(f.tokens) {
			v[f.tokens[i]] = ed.Text()
		}
	}
	return v
}

// SetValues types values into the fields (the control API).
func (f *AlertTagFill) SetValues(values map[string]string) {
	f.errText = ""
	for i, t := range f.tokens {
		for k, v := range values {
			if strings.EqualFold(k, t) && i < len(f.edits) {
				f.edits[i].SetText(v)
			}
		}
	}
	f.renderPreview()
}

func (f *AlertTagFill) renderPreview() {
	text, _ := models.FillAlertTokens(f.message, f.Values())
	f.preview.SetText(text)
	f.errLbl.SetText(f.errText)
	flushCalls(f.errLbl.AsControl())
}

// Error is the dialog's validation message ("Fill in plate.").
func (f *AlertTagFill) Error() string { return f.errText }

// Tokens lists the tokens the dialog asks for.
func (f *AlertTagFill) Tokens() []string { return append([]string(nil), f.tokens...) }

// Preview is the filled message as the dialog shows it.
func (f *AlertTagFill) Preview() string { return f.preview.Text() }

// Visible reports whether the dialog is open.
func (f *AlertTagFill) Visible() bool { return f.dlg.win.Visible() }

// Hide closes the dialog.
func (f *AlertTagFill) Hide() { f.dlg.hide() }

// AsWindow returns the dialog window.
func (f *AlertTagFill) AsWindow() Window.Instance { return f.dlg.win }

// ---- Nursery ------------------------------------------------------------------------

// NurseryDialog is EasyWorship's Nursery Alert panel: a code field with Add,
// and the codes on screen with Remove / Clear All.
type NurseryDialog struct {
	OnAdd    func(code string)
	OnRemove func(code string)
	OnClear  func()

	dlg    *dialogWindow
	code   LineEdit.Instance
	list   Tree.Instance
	remove Button.Instance
	clear  Button.Instance
	empty  Label.Instance
	codes  []string
}

// NewNurseryDialog builds the dialog.
func NewNurseryDialog() *NurseryDialog {
	n := &NurseryDialog{}
	n.dlg = newDialogWindow("Nursery Alert", 400, 352)
	n.dlg.ok.SetText("Close")
	n.dlg.onOK = n.dlg.close

	n.code = LineEdit.New()
	n.code.SetPlaceholderText("Number")
	n.code.SetMaxLength(models.MaxNurseryCode)
	n.code.AsControl().SetCustomMinimumSize(Vector2.New(120, 26))
	n.code.OnTextSubmitted(func(string) { n.PressAdd() })
	addBtn := button("Add", 56, n.PressAdd)
	head := hbox(6)
	title := label("Nursery Alert", vTitle)
	title.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	add(head, title.AsControl(), n.code.AsControl(), addBtn.AsControl())

	n.list = Tree.New()
	n.list.SetHideRoot(true)
	expand(n.list.AsControl())
	n.list.OnItemSelected(func() { n.render() })
	n.empty = label("There are no nursery numbers on screen.", vHint)
	n.empty.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	n.remove = button("Remove", 75, func() {
		if i := treeIndex(n.list.GetSelected()); i >= 0 && i < len(n.codes) && n.OnRemove != nil {
			n.OnRemove(n.codes[i])
		}
	})
	n.clear = button("Clear All", 75, func() {
		if n.OnClear != nil {
			n.OnClear()
		}
	})
	btns := hbox(6)
	add(btns, n.remove.AsControl(), n.clear.AsControl())
	add(n.dlg.body, head.AsControl(), n.list.AsControl(), n.empty.AsControl(), btns.AsControl(),
		hint("Each number shows in the nursery badge on the live output until it is removed. Options ▸ Alerts sets where, "+
			"how it looks and whether numbers are removed automatically."))
	add(n.dlg.footer, spacer(), n.dlg.ok.AsControl())
	Node.Instance(n.dlg.cancel.AsNode()).QueueFree()
	n.dlg.cancel = Button.Nil
	n.dlg.win.AsNode().SetName("NurseryDialog")
	n.render()
	return n
}

// SetCodes lists the codes on screen.
func (n *NurseryDialog) SetCodes(codes []string) {
	n.codes = codes
	quietly(n.list.AsControl(), func() {
		n.list.Clear()
		root := n.list.CreateItem()
		for _, c := range codes {
			it := n.list.MoreArgs().CreateItem(root, -1)
			it.SetText(0, c)
			it.SetCustomMinimumHeight(28)
		}
	})
	n.list.AsCanvasItem().QueueRedraw()
	n.render()
}

func (n *NurseryDialog) render() {
	n.empty.AsCanvasItem().SetVisible(len(n.codes) == 0)
	n.remove.AsBaseButton().SetDisabled(treeIndex(n.list.GetSelected()) < 0)
	n.clear.AsBaseButton().SetDisabled(len(n.codes) == 0)
}

// Open shows the dialog with the code field focused.
func (n *NurseryDialog) Open() {
	n.dlg.popup()
	n.code.AsControl().GrabFocus()
}

// PressAdd is the Add button: the typed code goes on the badge.
func (n *NurseryDialog) PressAdd() {
	c := strings.TrimSpace(n.code.Text())
	if c == "" {
		return
	}
	n.code.Clear()
	if n.OnAdd != nil {
		n.OnAdd(c)
	}
	n.code.AsControl().GrabFocus()
}

// SetCode types a code (the control API).
func (n *NurseryDialog) SetCode(code string) { n.code.SetText(code) }

// Visible reports whether the dialog is open.
func (n *NurseryDialog) Visible() bool { return n.dlg.win.Visible() }

// Hide closes the dialog.
func (n *NurseryDialog) Hide() { n.dlg.hide() }

// AsWindow returns the dialog window.
func (n *NurseryDialog) AsWindow() Window.Instance { return n.dlg.win }

// ---- Countdown --------------------------------------------------------------------------

// CountdownForm is what the Countdown dialog asks to start.
type CountdownForm struct {
	ToTime   bool   // to a time of day (Time), else for Minutes:Seconds
	Time     string // "10:30 AM"
	Minutes  int
	Seconds  int
	Caption  string
	Position models.AlertPosition
	End      models.CountdownEnd
}

var (
	countdownPositions = []models.AlertPosition{models.PosCenter, models.PosTopLeft, models.PosTopRight, models.PosBottomLeft, models.PosBottomRight}
	countdownEnds      = []models.CountdownEnd{models.CountdownHide, models.CountdownHold, models.CountdownOverrun}
)

// CountdownDialog starts a countdown to a time or for a length of time
// (EasyWorship's Service Interval), with a caption, a position and what
// happens at zero.
type CountdownDialog struct {
	OnStart func(CountdownForm)
	OnStop  func()

	dlg      *dialogWindow
	mode     OptionButton.Instance
	at       LineEdit.Instance
	mins     SpinBox.Instance
	secs     SpinBox.Instance
	caption  LineEdit.Instance
	position OptionButton.Instance
	end      OptionButton.Instance
	atRow    Control.Instance
	forRow   Control.Instance
	status   Label.Instance
	stop     Button.Instance
	errLbl   Label.Instance

	running bool
	statusT string
	errText string
}

// NewCountdownDialog builds the dialog.
func NewCountdownDialog() *CountdownDialog {
	c := &CountdownDialog{}
	c.dlg = newDialogWindow("Countdown", 460, 340)
	c.dlg.ok.SetText("Start")
	c.dlg.onOK = func() {
		form := c.Form()
		if c.OnStart != nil {
			c.OnStart(form)
		}
	}
	row := func(caption string, ctl ...Control.Instance) Control.Instance {
		r := hbox(8)
		l := label(caption, "")
		l.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
		l.AsControl().SetCustomMinimumSize(Vector2.New(96, 0))
		add(r, l.AsControl())
		add(r, ctl...)
		return r.AsControl()
	}
	c.mode = optionOf([]string{"To a time of day", "For a length of time"})
	c.mode.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	c.mode.OnItemSelected(func(int) { c.render() })
	c.at = LineEdit.New()
	c.at.SetPlaceholderText("10:30 AM")
	c.at.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	c.at.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
	c.mins = SpinBox.New()
	c.mins.AsRange().SetMaxValue(600)
	c.mins.SetSuffix("min")
	c.mins.AsRange().SetValueNoSignal(5)
	c.secs = SpinBox.New()
	c.secs.AsRange().SetMaxValue(59)
	c.secs.SetSuffix("s")
	c.caption = LineEdit.New()
	c.caption.SetText("Service starts in")
	c.caption.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	c.caption.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
	c.position = optionOf([]string{"Center", "Top Left", "Top Right", "Bottom Left", "Bottom Right"})
	c.position.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	c.end = optionOf([]string{"Hide it", "Show 00:00 until cleared", "Keep counting (overrun)"})
	c.end.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	c.atRow = row("Time", c.at.AsControl())
	c.forRow = row("Length", c.mins.AsControl(), c.secs.AsControl())
	c.status = label("", vHint)
	c.status.SetClipText(false)
	add(c.dlg.body, row("Count down", c.mode.AsControl()), c.atRow, c.forRow, row("Caption", c.caption.AsControl()),
		row("Position", c.position.AsControl()), row("When it ends", c.end.AsControl()), fixed(0, 4), c.status.AsControl())
	c.stop = button("Stop Countdown", 120, func() {
		if c.OnStop != nil {
			c.OnStop()
		}
	})
	c.errLbl = label("", vError)
	c.errLbl.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	add(c.dlg.footer, c.stop.AsControl(), c.errLbl.AsControl(), c.dlg.ok.AsControl(), c.dlg.cancel.AsControl())
	c.dlg.win.AsNode().SetName("CountdownDialog")
	c.render()
	return c
}

// Open shows the dialog with the styles' position and end choices.
func (c *CountdownDialog) Open(pos models.AlertPosition, end models.CountdownEnd) {
	c.errText = ""
	c.position.Select(indexOf(countdownPositions, pos, 0))
	c.end.Select(indexOf(countdownEnds, end, 0))
	c.render()
	c.dlg.popup()
}

// Form reads the fields.
func (c *CountdownDialog) Form() CountdownForm {
	return CountdownForm{
		ToTime:   c.mode.Selected() == 0,
		Time:     strings.TrimSpace(c.at.Text()),
		Minutes:  int(c.mins.AsRange().Value()),
		Seconds:  int(c.secs.AsRange().Value()),
		Caption:  strings.TrimSpace(c.caption.Text()),
		Position: countdownPositions[max(0, c.position.Selected())],
		End:      countdownEnds[max(0, c.end.Selected())],
	}
}

// SetForm fills the fields (the control API).
func (c *CountdownDialog) SetForm(f CountdownForm) {
	if f.ToTime {
		c.mode.Select(0)
	} else {
		c.mode.Select(1)
	}
	c.at.SetText(f.Time)
	c.mins.AsRange().SetValueNoSignal(Float.X(f.Minutes))
	c.secs.AsRange().SetValueNoSignal(Float.X(f.Seconds))
	c.caption.SetText(f.Caption)
	if f.Position != "" {
		c.position.Select(indexOf(countdownPositions, f.Position, 0))
	}
	if f.End != "" {
		c.end.Select(indexOf(countdownEnds, f.End, 0))
	}
	c.render()
}

// SetStatus shows the running countdown ("" when none) and enables Stop.
func (c *CountdownDialog) SetStatus(running bool, status string) {
	c.running, c.statusT = running, status
	c.render()
}

// Error is why Start did not start ("" when it did).
func (c *CountdownDialog) Error() string { return c.errText }

// SetError shows why Start did not start.
func (c *CountdownDialog) SetError(err string) {
	c.errText = err
	c.render()
}

func (c *CountdownDialog) render() {
	to := c.mode.Selected() == 0
	c.atRow.AsCanvasItem().SetVisible(to)
	c.forRow.AsCanvasItem().SetVisible(!to)
	c.stop.AsBaseButton().SetDisabled(!c.running)
	c.status.SetText(c.statusT)
	c.errLbl.SetText(c.errText)
	flushCalls(c.errLbl.AsControl())
}

// Hide closes the dialog.
func (c *CountdownDialog) Hide() { c.dlg.hide() }

// Visible reports whether the dialog is open.
func (c *CountdownDialog) Visible() bool { return c.dlg.win.Visible() }

// AsWindow returns the dialog window.
func (c *CountdownDialog) AsWindow() Window.Instance { return c.dlg.win }

// ---- MainWindow plumbing ------------------------------------------------------------------

// buildAlertDialogs creates the alert dialogs (NewMainWindow).
func (w *MainWindow) buildAlertDialogs() {
	w.AlertManager = NewAlertManager()
	w.AlertEditor = NewAlertEditor()
	w.AlertFill = NewAlertTagFill()
	w.Nursery = NewNurseryDialog()
	w.Countdown = NewCountdownDialog()
}

// alertDialogs lists the alert dialog windows (MainWindow.dialogs).
func (w *MainWindow) alertDialogs() []*dialogWindow {
	if w.AlertManager == nil {
		return nil
	}
	return []*dialogWindow{w.AlertManager.dlg, w.AlertEditor.dlg, w.AlertFill.dlg, w.Nursery.dlg, w.Countdown.dlg}
}

// alertDialogName names an alert dialog window for TopDialogHost ("" when n
// is not one).
func (w *MainWindow) alertDialogName(n Node.Instance) string {
	if w.AlertManager == nil {
		return ""
	}
	switch n.ID() {
	case w.AlertManager.dlg.win.AsNode().ID():
		return "alert-manager"
	case w.AlertEditor.dlg.win.AsNode().ID():
		return "alert-editor"
	case w.AlertFill.dlg.win.AsNode().ID():
		return "alert-fill"
	case w.Nursery.dlg.win.AsNode().ID():
		return "nursery"
	case w.Countdown.dlg.win.AsNode().ID():
		return "countdown"
	}
	return ""
}

// AlertDialog names the topmost open alert dialog ("" when none).
func (w *MainWindow) AlertDialog() string {
	top := w.dialogHost(0)
	return w.alertDialogName(top)
}

// AlertWindow returns an alert dialog's window by its AlertDialog name.
func (w *MainWindow) AlertWindow(name string) (Window.Instance, bool) {
	for _, d := range w.alertDialogs() {
		if w.alertDialogName(d.win.AsNode()) == name {
			return d.win, true
		}
	}
	return Window.Nil, false
}

func dimIf(dim bool) Color.RGBA {
	if dim {
		return Color.RGBA{R: 1, G: 1, B: 1, A: 0.45}
	}
	return Color.RGBA{R: 1, G: 1, B: 1, A: 1}
}
