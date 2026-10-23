package ui

import (
	"strings"
	"time"

	"freebeams/internal/models"

	"graphics.gd/classdb/Button"
	"graphics.gd/classdb/CheckBox"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/MarginContainer"
	"graphics.gd/classdb/OptionButton"
	"graphics.gd/classdb/SpinBox"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Theme"
	"graphics.gd/classdb/Tree"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// ScreenAuto is the output display choice "first non-primary display".
const ScreenAuto = -1

// SettingsValues is what the Options dialog shows (a projection of the app's
// settings).
type SettingsValues struct {
	Transition   models.Transition
	OutputScreen int    // ScreenAuto or a display index
	Logo         string // description of the logo image ("logo.png", "Default (FreeBeams logo)", "Default gradient")
	LibraryPath  string

	// The Remote page: the remote control's setting, its port and a status
	// line ("On: http://192.168.1.20:8765/", "Off", an error).
	RemoteEnabled bool
	RemotePort    int
	RemoteStatus  string

	// Alerts is Options ▸ Alerts (alertsettings.go).
	Alerts models.AlertStyles

	// Screens are the Black, Clear and Logo transitions (settings_screens.go).
	Screens models.ScreenTransitions
}

// ScreenRow is one display in the output picker.
type ScreenRow struct {
	Index int
	Label string
}

// Settings dialog sections, in menu order.
const (
	SettingsPageOutput = iota
	SettingsPageTransition
	SettingsPageLogo
	SettingsPageLibrary
	SettingsPageHotkeys

	// SettingsPageStage is the Stage Display (Foldback) section (settings_stage.go).
	SettingsPageStage
	SettingsPageRemote
	SettingsPageAlerts

	// SettingsPageThemes is the default theme per item type (settings_themes.go).
	SettingsPageThemes
)

var settingsPageNames = []string{"Output", "Transition", "Logo", "Library", "Hotkeys", "Stage Display", "Remote", "Alerts",
	"Themes"}

// ParseSettingsPage finds a section by its (case-insensitive) name.
func ParseSettingsPage(name string) (int, bool) {
	if isStagePage(name) {
		return SettingsPageStage, true
	}
	for i, n := range settingsPageNames {
		if strings.EqualFold(n, name) {
			return i, true
		}
	}
	return 0, false
}

// SettingsDialog is the Options dialog (UI-SPEC §5: 712 × 532, a 188px menu
// of sections on the left and the section's page on the right, OK / Cancel in
// the footer). It is a dumb view: SetValues / SetScreens / SetHotkeys project
// the app's settings onto it and every change the operator makes is reported
// at once through the On… callbacks (the app applies it and renders it back).
// OK closes; Cancel asks the app (OnCancel) to restore what it had.
type SettingsDialog struct {
	OnTransition        func(models.Transition)
	OnOutputScreen      func(int) // ScreenAuto or a display index
	OnChooseLogo        func()    // "Choose Image…": the app shows the file dialog
	OnDefaultLogo       func()
	OnImportCopyrighted func() // "Import Copyrighted EasyWorship Bibles…"
	OnOK                func()
	OnCancel            func()

	// OnStage reports a change in the Stage Display section (all its values).
	OnStage func(StageValues)

	// The Remote page: the switch, the port and "Show Pairing Code…".
	OnRemoteEnabled func(bool)
	OnRemotePort    func(int)
	OnShowRemote    func()

	// OnAlertStyles reports a change on the Alerts page.
	OnAlertStyles func(models.AlertStyles)

	// OnScreenTransition reports a new Black, Clear or Logo transition
	// (settings_screens.go).
	OnScreenTransition func(models.OutputMode, models.ScreenTransition)

	// LogoPreview hosts the app's render of the logo screen (a SlideView).
	LogoPreview *MonitorHost

	// Hotkeys is the Hotkeys section (settings_hotkeys.go) and DefaultThemes
	// the Themes section (settings_themes.go); each has its own callbacks.
	Hotkeys       *HotkeysPage
	DefaultThemes *DefaultThemesPage

	dlg      *dialogWindow
	sections Tree.Instance
	pages    []Control.Instance
	page     int

	screen    OptionButton.Instance
	kind      OptionButton.Instance
	duration  SpinBox.Instance
	logoLbl   Label.Instance
	libLbl    Label.Instance
	importBtn Button.Instance

	remoteOn     CheckBox.Instance
	remotePort   SpinBox.Instance
	remoteStatus Label.Instance

	alertCtl alertControls // the Alerts page (alertsettings.go)

	values      SettingsValues
	screens     []ScreenRow
	importAvail bool
	kinds       []models.TransitionKind
	rendering   bool

	stage *stagePage // the Stage Display section (settings_stage.go)

	screenCtl []*screenControls // the Black / Clear / Logo rows (settings_screens.go)
}

// NewSettingsDialog builds the dialog. MainWindow adds AsWindow() to the tree.
func NewSettingsDialog() *SettingsDialog {
	d := &SettingsDialog{kinds: models.TransitionKinds()}
	d.dlg = newDialogWindow("Options", 712, 532)
	d.dlg.onOK = func() {
		d.dlg.hide()
		if d.OnOK != nil {
			d.OnOK()
		}
	}
	d.dlg.onClose = func() {
		if d.OnCancel != nil {
			d.OnCancel()
		}
	}

	// Left: the section menu (188 wide, 32px rows).
	d.sections = Tree.New()
	d.sections.AsControl().SetThemeTypeVariation(vLibraryTree)
	d.sections.SetHideRoot(true)
	d.sections.AsControl().SetCustomMinimumSize(Vector2.New(188, 0))
	d.sections.AsControl().SetSizeFlagsVertical(Control.SizeExpandFill)
	root := d.sections.CreateItem()
	for _, name := range settingsPageNames {
		it := d.sections.MoreArgs().CreateItem(root, -1)
		it.SetText(0, name)
		it.SetCustomMinimumHeight(32)
	}
	d.sections.OnItemSelected(func() {
		if i := treeIndex(d.sections.GetSelected()); i >= 0 && !d.rendering {
			d.page = i
			d.render()
		}
	})

	// Right: one page per section, stacked.
	host := Control.New()
	host.SetClipContents(true)
	expand(host)
	d.pages = []Control.Instance{d.buildOutputPage(), d.buildTransitionPage(), d.buildLogoPage(), d.buildLibraryPage(), d.buildHotkeysPage(), d.buildStagePage(), d.buildRemotePage(),
		d.buildAlertsPage(), d.buildThemesPage()}
	for _, p := range d.pages {
		p.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
		host.AsNode().AddChild(p.AsNode())
	}

	row := hbox(12)
	expand(row.AsControl())
	add(row, d.sections.AsControl(), host)
	add(d.dlg.body, row.AsControl())
	add(d.dlg.footer, spacer(), d.dlg.ok.AsControl(), d.dlg.cancel.AsControl())
	d.dlg.win.AsNode().SetName("SettingsDialog")
	d.dlg.keyHook = d.captureKey
	d.render()
	return d
}

// card is a titled group of settings (EasyWorship's Inspector Cards).
func card(title string, body ...Control.Instance) Control.Instance {
	p := panel(vBk1)
	col := vbox(8)
	t := label(title, vTitle)
	add(col, t.AsControl())
	add(col, body...)
	add(p, padded(col.AsControl(), 12, 10, 12, 12))
	return p.AsControl()
}

// padded wraps c in margins (left, top, right, bottom).
func padded(c Control.Instance, l, t, r, b int) Control.Instance {
	m := MarginContainer.New()
	for side, v := range map[string]int{"margin_left": l, "margin_top": t, "margin_right": r, "margin_bottom": b} {
		m.AsControl().AddThemeConstantOverride(side, v)
	}
	add(m, c)
	return m.AsControl()
}

// hint is a wrapping, dimmed explanation line.
func hint(text string) Control.Instance {
	l := label(text, vHint)
	l.SetAutowrapMode(TextServer.AutowrapWordSmart)
	l.SetClipText(false)
	l.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	l.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	return l.AsControl()
}

// settingRow puts a right-aligned caption before a control (the theme editor's
// fieldRow is the same idea with its own label width).
func settingRow(caption string, c Control.Instance) Control.Instance {
	row := hbox(8)
	l := label(caption, "")
	l.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
	l.AsControl().SetCustomMinimumSize(Vector2.New(96, 0))
	add(row, l.AsControl(), c)
	return row.AsControl()
}

// page stacks cards from the top of the right-hand area.
func page(cards ...Control.Instance) Control.Instance {
	col := vbox(10)
	add(col, cards...)
	return col.AsControl()
}

func (d *SettingsDialog) buildOutputPage() Control.Instance {
	d.screen = OptionButton.New()
	d.screen.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	d.screen.OnItemSelected(func(idx int) {
		if d.rendering || d.OnOutputScreen == nil {
			return
		}
		d.OnOutputScreen(d.screen.GetItemId(idx))
	})
	return page(card("Select Output Monitor",
		settingRow("Display", d.screen.AsControl()),
		hint("The output covers the chosen display. With only one display connected, the output opens as a resizable "+
			"window instead, and the Live button turns it on. Changes apply at once."),
	))
}

func (d *SettingsDialog) buildTransitionPage() Control.Instance {
	d.kind = OptionButton.New()
	d.kind.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	for i, k := range d.kinds {
		d.kind.MoreArgs().AddItem(k.Label(), i)
	}
	d.kind.OnItemSelected(func(idx int) {
		if d.rendering || d.OnTransition == nil || idx < 0 || idx >= len(d.kinds) {
			return
		}
		d.OnTransition(models.Transition{Kind: d.kinds[idx], Duration: d.values.Transition.Duration})
	})
	d.duration = SpinBox.New()
	rg := d.duration.AsRange()
	rg.SetMinValue(0)
	rg.SetMaxValue(Float.X(models.MaxTransitionDuration.Milliseconds()))
	rg.SetStep(50)
	d.duration.SetSuffix("ms")
	d.duration.AsControl().SetCustomMinimumSize(Vector2.New(120, 0))
	rg.OnValueChanged(func(v Float.X) {
		if d.rendering || d.OnTransition == nil {
			return
		}
		d.OnTransition(models.Transition{Kind: d.values.Transition.Kind, Duration: msToDuration(float64(v))})
	})
	return page(card("Slide Transition",
		settingRow("Transition", d.kind.AsControl()),
		settingRow("Duration", d.duration.AsControl()),
		hint("Used by the Live monitor and the output for slide changes. Directional transitions name the direction of "+
			"motion: Push Left pushes the old slide off to the left. Thumbnails and the Preview monitor cut."),
	), d.buildScreenTransitionsCard())
}

func (d *SettingsDialog) buildLogoPage() Control.Instance {
	d.LogoPreview = NewMonitorHost()
	d.LogoPreview.AsControl().SetCustomMinimumSize(Vector2.New(320, 180))
	d.logoLbl = label("", "")
	d.logoLbl.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	choose := Button.New()
	choose.SetText("Choose Image…")
	choose.AsBaseButton().OnPressed(func() {
		if d.OnChooseLogo != nil {
			d.OnChooseLogo()
		}
	})
	def := Button.New()
	def.SetText("Use Default")
	def.AsBaseButton().OnPressed(func() {
		if d.OnDefaultLogo != nil {
			d.OnDefaultLogo()
		}
	})
	buttons := hbox(8)
	add(buttons, choose.AsControl(), def.AsControl())
	preview := hbox(0)
	add(preview, d.LogoPreview.AsControl(), spacer())
	return page(card("Logo Screen",
		preview.AsControl(),
		settingRow("Image", d.logoLbl.AsControl()),
		buttons.AsControl(),
		hint("The Logo button (Ctrl+L) shows this image full screen instead of the live slide."),
	))
}

func (d *SettingsDialog) buildLibraryPage() Control.Instance {
	d.libLbl = label("", "")
	d.libLbl.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	d.importBtn = Button.New()
	d.importBtn.SetText("Import Copyrighted EasyWorship Bibles…")
	d.importBtn.AsBaseButton().OnPressed(func() {
		if d.OnImportCopyrighted != nil {
			d.OnImportCopyrighted()
		}
	})
	row := hbox(0)
	add(row, d.importBtn.AsControl(), spacer())
	return page(
		card("Library", settingRow("Database", d.libLbl.AsControl())),
		card("EasyWorship Bibles",
			row.AsControl(),
			hint("The first launch imports EasyWorship's public-domain Bibles. Copyrighted translations (such as the HCSB) "+
				"are imported only on request, if your church is licensed to use them."),
		),
	)
}

func (d *SettingsDialog) buildHotkeysPage() Control.Instance {
	d.Hotkeys = newHotkeysPage()
	return d.Hotkeys.build()
}

func (d *SettingsDialog) buildThemesPage() Control.Instance {
	d.DefaultThemes = newDefaultThemesPage()
	return d.DefaultThemes.build()
}

// captureKey hands a key press to the Hotkeys page while it captures one
// (the dialog's key hook); true keeps the key from the dialog's controls.
func (d *SettingsDialog) captureKey(ev InputEvent.Instance) bool {
	if !d.Visible() || !d.Hotkeys.Capturing() {
		return false
	}
	if d.Hotkeys.OnKey != nil {
		d.Hotkeys.OnKey(ev)
	}
	return true
}

func (d *SettingsDialog) buildRemotePage() Control.Instance {
	d.remoteOn = CheckBox.New()
	d.remoteOn.AsButton().SetText("Allow phones and tablets on this network to control FreeBeams")
	d.remoteOn.AsBaseButton().OnToggled(func(on bool) {
		if !d.rendering && d.OnRemoteEnabled != nil {
			d.OnRemoteEnabled(on)
		}
	})
	d.remotePort = SpinBox.New()
	rg := d.remotePort.AsRange()
	rg.SetMinValue(1024)
	rg.SetMaxValue(65535)
	rg.SetStep(1)
	d.remotePort.AsControl().SetCustomMinimumSize(Vector2.New(120, 0))
	rg.OnValueChanged(func(v Float.X) {
		if !d.rendering && d.OnRemotePort != nil {
			d.OnRemotePort(int(v))
		}
	})
	d.remoteStatus = label("", "")
	d.remoteStatus.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	show := Button.New()
	show.SetText("Show Pairing Code…")
	show.AsBaseButton().OnPressed(func() {
		if d.OnShowRemote != nil {
			d.OnShowRemote()
		}
	})
	row := hbox(0)
	add(row, show.AsControl(), spacer())
	return page(card("Remote Control",
		d.remoteOn.AsControl(),
		settingRow("Port", d.remotePort.AsControl()),
		settingRow("Status", d.remoteStatus.AsControl()),
		row.AsControl(),
		hint("A phone opens the address shown with the pairing code and asks to pair; you approve it by name. Paired "+
			"devices can move through the slides, go live with schedule items, and use Black, Clear and Logo."),
	))
}

// AsWindow returns the dialog window node.
func (d *SettingsDialog) AsWindow() Window.Instance { return d.dlg.win }

// SetTheme sets the dialog's theme (separate OS windows do not inherit it).
func (d *SettingsDialog) SetTheme(th Theme.Instance) { d.dlg.setTheme(th) }

// Open shows the dialog on its first section.
func (d *SettingsDialog) Open() { d.OpenAt(SettingsPageOutput) }

// OpenAt shows the dialog on a section (SettingsPage…; out of range = Output).
func (d *SettingsDialog) OpenAt(page int) {
	if page < 0 || page >= len(d.pages) {
		page = SettingsPageOutput
	}
	d.page = page
	d.render()
	d.dlg.popup()
}

// Hide closes the dialog without calling OnOK / OnCancel.
func (d *SettingsDialog) Hide() { d.dlg.hide() }

// Visible reports whether the dialog is on screen.
func (d *SettingsDialog) Visible() bool { return d.dlg.win.Visible() }

// Page returns the shown section (SettingsPage…).
func (d *SettingsDialog) Page() int { return d.page }

// SetPage shows a section.
func (d *SettingsDialog) SetPage(i int) {
	if i >= 0 && i < len(d.pages) {
		d.page = i
		d.render()
	}
}

// SetValues projects the app's settings onto the controls.
func (d *SettingsDialog) SetValues(v SettingsValues) {
	d.values = v
	d.render()
}

// SetScreens lists the connected displays in the output picker.
func (d *SettingsDialog) SetScreens(rows []ScreenRow) {
	d.screens = rows
	d.render()
}

// SetHotkeys fills the hotkey table.
func (d *SettingsDialog) SetHotkeys(rows []HotkeyRow) { d.Hotkeys.SetRows(rows) }

// SetImportCopyrightedAvailable enables the Bible import button.
func (d *SettingsDialog) SetImportCopyrightedAvailable(ok bool) {
	d.importAvail = ok
	d.render()
}

func (d *SettingsDialog) render() {
	d.rendering = true
	defer func() { d.rendering = false }()
	for i, p := range d.pages {
		p.AsCanvasItem().SetVisible(i == d.page)
	}
	quietly(d.sections.AsControl(), func() { selectTreeRow(d.sections, d.page) })

	// Output.
	d.screen.Clear()
	d.screen.MoreArgs().AddItem("Automatic (first non-primary display)", ScreenAuto)
	for _, s := range d.screens {
		d.screen.MoreArgs().AddItem(s.Label, s.Index)
	}
	if idx := d.screen.GetItemIndex(d.values.OutputScreen); idx >= 0 {
		d.screen.Select(idx)
	} else {
		d.screen.Select(0)
	}

	// Transition. SetValueNoSignal keeps value_changed from reporting a
	// render as a user change.
	for i, k := range d.kinds {
		if k == d.values.Transition.Kind {
			d.kind.Select(i)
		}
	}
	d.duration.AsRange().SetValueNoSignal(Float.X(d.values.Transition.Duration.Milliseconds()))
	d.duration.SetEditable(d.values.Transition.Kind != models.TransitionCut) // a cut has no duration
	d.renderScreenTransitions()

	// Logo and library.
	d.logoLbl.SetText(d.values.Logo)
	d.libLbl.SetText(d.values.LibraryPath)
	d.importBtn.AsBaseButton().SetDisabled(!d.importAvail)

	// Remote.
	d.remoteOn.AsBaseButton().SetPressedNoSignal(d.values.RemoteEnabled)
	d.remotePort.AsRange().SetValueNoSignal(Float.X(d.values.RemotePort))
	d.remoteStatus.SetText(d.values.RemoteStatus)
	flushCalls(d.remoteOn.AsControl())

	d.renderAlerts()

	d.renderStage()
}

// msToDuration converts a spin box value in milliseconds to a Duration.
func msToDuration(ms float64) time.Duration { return time.Duration(ms) * time.Millisecond }
