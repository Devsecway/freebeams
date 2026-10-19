package ui

import (
	"path/filepath"
	"sort"
	"strings"

	"freeworship/internal/models"

	"graphics.gd/classdb/Button"
	"graphics.gd/classdb/CheckBox"
	"graphics.gd/classdb/ColorPickerButton"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/OS"
	"graphics.gd/classdb/OptionButton"
	"graphics.gd/classdb/ScrollContainer"
	"graphics.gd/classdb/SpinBox"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Theme"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// ThemeEditor is the New / Edit theme dialog: the theme's fields on the left
// and a live preview on the right. The theme being edited lives in plain Go
// (e.theme); every widget change mutates it and one render pass projects it
// back onto the widgets, so the widgets never hold state of their own. The
// preview is a slot (Preview): the app puts a render.SlideView in it and
// feeds it the theme it receives through OnChange.
//
// OK saves and closes (OnSave), Apply saves and stays open (OnApply), Cancel
// and Esc close without saving (OnCancel). The name is required.
type ThemeEditor struct {
	OnSave   func(t models.Theme)
	OnApply  func(t models.Theme)
	OnCancel func()
	// OnChange reports every edit (and Open) with the current theme, for the
	// live preview.
	OnChange func(t models.Theme)
	// OnBrowseImage is the background image "Browse…" button; the app shows
	// the native file dialog and calls SetBackgroundImage.
	OnBrowseImage func()

	// Preview is the 16:9 slot for the live preview.
	Preview *MonitorHost

	dlg     *dialogWindow
	theme   models.Theme
	errText string
	fonts   []string // font families offered: "" (default) first

	name              LineEdit.Instance
	bgKind            OptionButton.Instance
	bgColor, bgColor2 ColorPickerButton.Instance
	bgColor2Lbl       Control.Instance
	bgImageRow        Control.Instance
	bgPath            LineEdit.Instance
	font              OptionButton.Instance
	size, minSize     SpinBox.Instance
	lineSpacing       SpinBox.Instance
	bold, italic      CheckBox.Instance
	color             ColorPickerButton.Instance
	outlineOn         CheckBox.Instance
	outlineSize       SpinBox.Instance
	outlineColor      ColorPickerButton.Instance
	shadowOn          CheckBox.Instance
	shadowX, shadowY  SpinBox.Instance
	shadowBlur        SpinBox.Instance
	shadowColor       ColorPickerButton.Instance
	hAlign, vAlign    OptionButton.Instance
	mL, mT, mR, mB    SpinBox.Instance
	footerSize        SpinBox.Instance
	footerColor       ColorPickerButton.Instance
	previewCaption    Label.Instance
	errLbl            Label.Instance
	apply             Button.Instance
	bgKinds           []models.BackgroundKind
	hAligns           []models.HAlign
	vAligns           []models.VAlign
}

// NewThemeEditor builds the dialog. Add AsWindow() to the scene tree
// (MainWindow does this for its own editor).
func NewThemeEditor() *ThemeEditor {
	e := &ThemeEditor{
		bgKinds: []models.BackgroundKind{models.BackgroundColor, models.BackgroundGradient, models.BackgroundImage},
		hAligns: []models.HAlign{models.AlignLeft, models.AlignCenter, models.AlignRight},
		vAligns: []models.VAlign{models.AlignTop, models.AlignMiddle, models.AlignBottom},
	}
	e.dlg = newDialogWindow("Theme Editor", 1020, 620)
	e.dlg.onOK = e.save
	e.dlg.onClose = func() {
		if e.OnCancel != nil {
			e.OnCancel()
		}
	}

	// Name row across the top.
	nameLbl := label("Name", "")
	nameLbl.AsControl().SetCustomMinimumSize(Vector2.New(84, 0))
	nameLbl.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
	e.name = LineEdit.New()
	e.name.SetPlaceholderText("Required")
	e.name.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
	e.name.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	e.name.OnTextChanged(func(text string) { e.change(func(t *models.Theme) { t.Name = text }) })
	nameRow := hbox(8)
	add(nameRow, nameLbl.AsControl(), e.name.AsControl())

	// Fields (left) | preview (right).
	fields := vbox(6)
	e.buildBackground(fields)
	e.buildText(fields)
	e.buildOutlineShadow(fields)
	e.buildLayout(fields)
	scroll := ScrollContainer.New()
	scroll.SetHorizontalScrollMode(ScrollContainer.ScrollModeDisabled)
	fields.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	add(scroll, fields.AsControl())
	scroll.AsControl().SetCustomMinimumSize(Vector2.New(500, 0))
	expand(scroll.AsControl())

	e.Preview = NewMonitorHost()
	e.Preview.FitWidth = true
	e.Preview.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	e.previewCaption = label("Preview", vCaption)
	e.previewCaption.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	right := vbox(4)
	right.AsControl().SetCustomMinimumSize(Vector2.New(420, 0))
	right.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	add(right, e.Preview.AsControl(), e.previewCaption.AsControl())

	body := hbox(12)
	expand(body.AsControl())
	add(body, scroll.AsControl(), right.AsControl())
	add(e.dlg.body, nameRow.AsControl(), body.AsControl())

	// Footer: error text, Apply / OK / Cancel.
	e.errLbl = label("", vError)
	e.errLbl.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	e.apply = Button.New()
	e.apply.SetText("Apply")
	e.apply.AsControl().SetCustomMinimumSize(Vector2.New(75, 25))
	e.apply.AsBaseButton().OnPressed(e.applyPressed)
	add(e.dlg.footer, e.errLbl.AsControl(), e.apply.AsControl(), e.dlg.ok.AsControl(), e.dlg.cancel.AsControl())
	e.dlg.win.AsNode().SetName("ThemeEditor")
	e.render()
	return e
}

// ---- field groups ------------------------------------------------------------

func (e *ThemeEditor) buildBackground(col interface{ AsControl() Control.Instance }) {
	add(col, sectionLabel("Background"))
	e.bgKind = e.option([]string{"Solid color", "Gradient", "Image"}, func(i int) {
		e.change(func(t *models.Theme) {
			if i >= 0 && i < len(e.bgKinds) {
				t.Background.Kind = e.bgKinds[i]
			}
		})
	})
	e.bgColor = e.colorButton(false, func(c models.RGBA) { e.change(func(t *models.Theme) { t.Background.Color = c }) })
	e.bgColor2 = e.colorButton(false, func(c models.RGBA) { e.change(func(t *models.Theme) { t.Background.Color2 = c }) })
	e.bgColor2Lbl = inlineLabel("Bottom").AsControl()
	add(col, fieldRow("Style", e.bgKind.AsControl(), inlineLabel("Color").AsControl(), e.bgColor.AsControl(), e.bgColor2Lbl, e.bgColor2.AsControl()))

	e.bgPath = LineEdit.New()
	e.bgPath.SetEditable(false)
	e.bgPath.SetPlaceholderText("No image chosen")
	e.bgPath.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	e.bgPath.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
	browse := Button.New()
	browse.SetText("Browse…")
	browse.AsBaseButton().OnPressed(func() {
		if e.OnBrowseImage != nil {
			e.OnBrowseImage()
		}
	})
	clear := Button.New()
	clear.SetText("Clear")
	clear.AsBaseButton().OnPressed(func() { e.SetBackgroundImage("") })
	e.bgImageRow = fieldRow("Image", e.bgPath.AsControl(), browse.AsControl(), clear.AsControl())
	add(col, e.bgImageRow)
}

func (e *ThemeEditor) buildText(col interface{ AsControl() Control.Instance }) {
	add(col, sectionLabel("Text"))
	e.fonts = fontFamilies()
	names := make([]string, len(e.fonts))
	for i, f := range e.fonts {
		names[i] = f
	}
	names[0] = "Default (Helvetica Neue)"
	e.font = e.option(names, func(i int) {
		e.change(func(t *models.Theme) {
			if i >= 0 && i < len(e.fonts) {
				t.Font.Family = e.fonts[i]
			}
		})
	})
	e.font.SetFitToLongestItem(false)
	e.font.AsControl().SetCustomMinimumSize(Vector2.New(220, 26))
	e.font.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	e.bold = e.check("Bold", func(on bool) { e.change(func(t *models.Theme) { t.Font.Bold = on }) })
	e.italic = e.check("Italic", func(on bool) { e.change(func(t *models.Theme) { t.Font.Italic = on }) })
	add(col, fieldRow("Font", e.font.AsControl(), e.bold.AsControl(), e.italic.AsControl()))

	e.size = e.spin(8, 400, 1, func(v float64) { e.change(func(t *models.Theme) { t.Font.Size = float32(v) }) })
	e.minSize = e.spin(8, 400, 1, func(v float64) { e.change(func(t *models.Theme) { t.Font.MinSize = float32(v) }) })
	e.lineSpacing = e.spin(-0.5, 2, 0.05, func(v float64) { e.change(func(t *models.Theme) { t.Font.LineSpacing = float32(v) }) })
	e.color = e.colorButton(true, func(c models.RGBA) { e.change(func(t *models.Theme) { t.Font.Color = c }) })
	add(col, fieldRow("Size", e.size.AsControl(), inlineLabel("Shrink to").AsControl(), e.minSize.AsControl(),
		inlineLabel("Line spacing").AsControl(), e.lineSpacing.AsControl(), inlineLabel("Color").AsControl(), e.color.AsControl()))
}

func (e *ThemeEditor) buildOutlineShadow(col interface{ AsControl() Control.Instance }) {
	add(col, sectionLabel("Outline and Shadow"))
	e.outlineOn = e.check("Outline", func(on bool) { e.change(func(t *models.Theme) { t.Outline.Enabled = on }) })
	e.outlineSize = e.spin(0, 40, 0.5, func(v float64) { e.change(func(t *models.Theme) { t.Outline.Size = float32(v) }) })
	e.outlineColor = e.colorButton(true, func(c models.RGBA) { e.change(func(t *models.Theme) { t.Outline.Color = c }) })
	add(col, fieldRow("", e.outlineOn.AsControl(), inlineLabel("Size").AsControl(), e.outlineSize.AsControl(),
		inlineLabel("Color").AsControl(), e.outlineColor.AsControl()))

	e.shadowOn = e.check("Shadow", func(on bool) { e.change(func(t *models.Theme) { t.Shadow.Enabled = on }) })
	e.shadowX = e.spin(-40, 40, 1, func(v float64) { e.change(func(t *models.Theme) { t.Shadow.OffsetX = float32(v) }) })
	e.shadowY = e.spin(-40, 40, 1, func(v float64) { e.change(func(t *models.Theme) { t.Shadow.OffsetY = float32(v) }) })
	e.shadowBlur = e.spin(0, 40, 1, func(v float64) { e.change(func(t *models.Theme) { t.Shadow.Blur = float32(v) }) })
	e.shadowColor = e.colorButton(true, func(c models.RGBA) { e.change(func(t *models.Theme) { t.Shadow.Color = c }) })
	add(col, fieldRow("", e.shadowOn.AsControl(), inlineLabel("Offset").AsControl(), e.shadowX.AsControl(), e.shadowY.AsControl(),
		inlineLabel("Blur").AsControl(), e.shadowBlur.AsControl(), inlineLabel("Color").AsControl(), e.shadowColor.AsControl()))
}

func (e *ThemeEditor) buildLayout(col interface{ AsControl() Control.Instance }) {
	add(col, sectionLabel("Layout"))
	e.hAlign = e.option([]string{"Left", "Center", "Right"}, func(i int) {
		e.change(func(t *models.Theme) {
			if i >= 0 && i < len(e.hAligns) {
				t.HAlign = e.hAligns[i]
			}
		})
	})
	e.vAlign = e.option([]string{"Top", "Middle", "Bottom"}, func(i int) {
		e.change(func(t *models.Theme) {
			if i >= 0 && i < len(e.vAligns) {
				t.VAlign = e.vAligns[i]
			}
		})
	})
	add(col, fieldRow("Align", e.hAlign.AsControl(), e.vAlign.AsControl()))

	margin := func(set func(t *models.Theme, v float32)) SpinBox.Instance {
		return e.spin(0, 900, 4, func(v float64) { e.change(func(t *models.Theme) { set(t, float32(v)) }) })
	}
	e.mL = margin(func(t *models.Theme, v float32) { t.Margins.Left = v })
	e.mT = margin(func(t *models.Theme, v float32) { t.Margins.Top = v })
	e.mR = margin(func(t *models.Theme, v float32) { t.Margins.Right = v })
	e.mB = margin(func(t *models.Theme, v float32) { t.Margins.Bottom = v })
	add(col, fieldRow("Margins", inlineLabel("L").AsControl(), e.mL.AsControl(), inlineLabel("T").AsControl(), e.mT.AsControl(),
		inlineLabel("R").AsControl(), e.mR.AsControl(), inlineLabel("B").AsControl(), e.mB.AsControl()))

	e.footerSize = e.spin(8, 120, 1, func(v float64) { e.change(func(t *models.Theme) { t.FooterSize = float32(v) }) })
	e.footerColor = e.colorButton(true, func(c models.RGBA) { e.change(func(t *models.Theme) { t.FooterColor = c }) })
	add(col, fieldRow("Footer", inlineLabel("Size").AsControl(), e.footerSize.AsControl(), inlineLabel("Color").AsControl(), e.footerColor.AsControl()))
	hint := label("Sizes and margins are in canvas units (the slide is 1920 × 1080). Text shrinks from Size down to Shrink to until it fits.", vHint)
	hint.SetAutowrapMode(TextServer.AutowrapWordSmart)
	hint.SetClipText(false)
	hint.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	add(col, hint.AsControl())
}

// ---- widget builders -----------------------------------------------------------

func sectionLabel(text string) Control.Instance {
	l := label(text, vTitle)
	l.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
	return l.AsControl()
}

// inlineLabel is a small caption between the controls of a row. label()
// clips its text, which gives a non-expanding label a zero minimum width in
// a box container (it vanishes), so clipping is turned off here.
func inlineLabel(text string) Label.Instance {
	l := label(text, vCaption)
	l.SetClipText(false)
	l.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	l.AsControl().SetSizeFlagsVertical(Control.SizeShrinkCenter)
	return l
}

// fieldRow is a labelled row: an 84px right-aligned label, then the controls.
func fieldRow(labelText string, controls ...Control.Instance) Control.Instance {
	row := hbox(6)
	l := label(labelText, "")
	l.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
	l.AsControl().SetCustomMinimumSize(Vector2.New(84, 0))
	add(row, l.AsControl())
	add(row, controls...)
	return row.AsControl()
}

func (e *ThemeEditor) spin(min, max, step float64, fn func(v float64)) SpinBox.Instance {
	sb := SpinBox.New()
	r := sb.AsRange()
	r.SetMinValue(Float.X(min))
	r.SetMaxValue(Float.X(max))
	r.SetStep(Float.X(step))
	sb.SetUpdateOnTextChanged(true)
	sb.SetSelectAllOnFocus(true)
	sb.AsControl().SetCustomMinimumSize(Vector2.New(78, 26))
	r.OnValueChanged(func(v Float.X) { fn(float64(v)) })
	return sb
}

func (e *ThemeEditor) colorButton(alpha bool, fn func(c models.RGBA)) ColorPickerButton.Instance {
	b := ColorPickerButton.New()
	b.SetEditAlpha(alpha)
	b.AsControl().SetCustomMinimumSize(Vector2.New(44, 26))
	b.OnColorChanged(func(c Color.RGBA) { fn(fromGodotColor(c)) })
	return b
}

func (e *ThemeEditor) option(items []string, fn func(i int)) OptionButton.Instance {
	o := OptionButton.New()
	for _, it := range items {
		o.AddItem(it)
	}
	o.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
	o.OnItemSelected(fn)
	return o
}

func (e *ThemeEditor) check(text string, fn func(on bool)) CheckBox.Instance {
	c := CheckBox.New()
	c.AsButton().SetText(text)
	c.AsBaseButton().OnToggled(fn)
	return c
}

// fontFamilies lists the font families the editor offers: "" (the theme
// default) first, then the installed system fonts, sorted.
func fontFamilies() []string {
	seen := map[string]bool{}
	var names []string
	for _, n := range OS.GetSystemFonts() {
		n = strings.TrimSpace(n)
		if n == "" || strings.HasPrefix(n, ".") || seen[strings.ToLower(n)] {
			continue
		}
		seen[strings.ToLower(n)] = true
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return append([]string{""}, names...)
}

func toGodotColor(c models.RGBA) Color.RGBA { return Color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A} }

func fromGodotColor(c Color.RGBA) models.RGBA { return models.RGBA{R: c.R, G: c.G, B: c.B, A: c.A} }

// ---- dialog API ------------------------------------------------------------------

// AsWindow returns the dialog window (add it under the main window).
func (e *ThemeEditor) AsWindow() Window.Instance { return e.dlg.win }

// SetTheme sets the dialog's UI theme (separate OS windows do not inherit it).
func (e *ThemeEditor) SetTheme(th Theme.Instance) { e.dlg.setTheme(th) }

// Open shows the editor for t (zero ID = new theme) and reports it through
// OnChange for the preview.
func (e *ThemeEditor) Open(t models.Theme) {
	e.theme = t
	e.errText = ""
	title := "Edit Theme"
	if t.ID == 0 {
		title = "New Theme"
	}
	e.dlg.win.SetTitle(title)
	e.render()
	e.notify()
	e.dlg.popup()
	e.name.AsControl().GrabFocus()
}

// Update replaces the theme being edited without closing (after Apply saved
// it and assigned an ID).
func (e *ThemeEditor) Update(t models.Theme) {
	e.theme = t
	e.render()
}

// SetBackgroundImage sets the background image path (from the Browse…
// dialog) and switches the background to an image when path is not empty.
func (e *ThemeEditor) SetBackgroundImage(path string) {
	e.change(func(t *models.Theme) {
		t.Background.Path = path
		if path != "" {
			t.Background.Kind = models.BackgroundImage
		} else if t.Background.Kind == models.BackgroundImage {
			t.Background.Kind = models.BackgroundColor
		}
	})
}

// Theme returns the theme as currently edited.
func (e *ThemeEditor) Theme() models.Theme { return e.theme }

// Close hides the editor without calling OnCancel.
func (e *ThemeEditor) Close() { e.dlg.hide() }

// Visible reports whether the editor is open.
func (e *ThemeEditor) Visible() bool { return e.dlg.win.Visible() }

// change mutates the theme, re-renders and reports the change.
func (e *ThemeEditor) change(fn func(t *models.Theme)) {
	fn(&e.theme)
	e.render()
	e.notify()
}

func (e *ThemeEditor) notify() {
	if e.OnChange != nil {
		e.OnChange(e.theme)
	}
}

// render projects e.theme onto the widgets. Every setter used here is a
// no-signal one (set_value_no_signal, set_pressed_no_signal, select,
// LineEdit.set_text, ColorPickerButton.set_pick_color), so the projection
// never comes back as an edit. graphics.gd buffers these void calls, so
// render ends with a getter to apply them now.
func (e *ThemeEditor) render() {
	t := &e.theme
	if e.name.Text() != t.Name {
		e.name.SetText(t.Name)
	}
	e.bgKind.Select(indexOf(e.bgKinds, t.Background.Kind, 0))
	e.bgColor.SetColor(toGodotColor(t.Background.Color))
	e.bgColor2.SetColor(toGodotColor(t.Background.Color2))
	gradient := t.Background.Kind == models.BackgroundGradient
	image := t.Background.Kind == models.BackgroundImage
	e.bgColor2Lbl.AsCanvasItem().SetVisible(gradient)
	e.bgColor2.AsCanvasItem().SetVisible(gradient)
	e.bgImageRow.AsCanvasItem().SetVisible(image)
	if e.bgPath.Text() != t.Background.Path {
		e.bgPath.SetText(t.Background.Path)
	}
	e.bgPath.AsControl().SetTooltipText(t.Background.Path)
	if t.Background.Path != "" {
		e.bgPath.SetPlaceholderText(filepath.Base(t.Background.Path))
	}

	e.font.Select(e.fontIndex(t.Font.Family))
	e.size.AsRange().SetValueNoSignal(Float.X(t.Font.Size))
	e.minSize.AsRange().SetValueNoSignal(Float.X(t.Font.MinSize))
	e.lineSpacing.AsRange().SetValueNoSignal(Float.X(t.Font.LineSpacing))
	e.bold.AsBaseButton().SetPressedNoSignal(t.Font.Bold)
	e.italic.AsBaseButton().SetPressedNoSignal(t.Font.Italic)
	e.color.SetColor(toGodotColor(t.Font.Color))

	e.outlineOn.AsBaseButton().SetPressedNoSignal(t.Outline.Enabled)
	e.outlineSize.AsRange().SetValueNoSignal(Float.X(t.Outline.Size))
	e.outlineColor.SetColor(toGodotColor(t.Outline.Color))
	e.outlineSize.SetEditable(t.Outline.Enabled)
	e.outlineColor.AsBaseButton().SetDisabled(!t.Outline.Enabled)

	e.shadowOn.AsBaseButton().SetPressedNoSignal(t.Shadow.Enabled)
	e.shadowX.AsRange().SetValueNoSignal(Float.X(t.Shadow.OffsetX))
	e.shadowY.AsRange().SetValueNoSignal(Float.X(t.Shadow.OffsetY))
	e.shadowBlur.AsRange().SetValueNoSignal(Float.X(t.Shadow.Blur))
	e.shadowColor.SetColor(toGodotColor(t.Shadow.Color))
	for _, sb := range []SpinBox.Instance{e.shadowX, e.shadowY, e.shadowBlur} {
		sb.SetEditable(t.Shadow.Enabled)
	}
	e.shadowColor.AsBaseButton().SetDisabled(!t.Shadow.Enabled)

	e.hAlign.Select(indexOf(e.hAligns, t.HAlign, 1))
	e.vAlign.Select(indexOf(e.vAligns, t.VAlign, 1))
	e.mL.AsRange().SetValueNoSignal(Float.X(t.Margins.Left))
	e.mT.AsRange().SetValueNoSignal(Float.X(t.Margins.Top))
	e.mR.AsRange().SetValueNoSignal(Float.X(t.Margins.Right))
	e.mB.AsRange().SetValueNoSignal(Float.X(t.Margins.Bottom))
	e.footerSize.AsRange().SetValueNoSignal(Float.X(t.FooterSize))
	e.footerColor.SetColor(toGodotColor(t.FooterColor))

	e.previewCaption.SetText("Preview · " + strings.TrimSpace(t.Name))
	e.errLbl.SetText(e.errText)
	flushCalls(e.errLbl.AsControl())
}

// fontIndex is the option index of a family; an installed-but-unlisted or
// missing family is appended so the selection shows it.
func (e *ThemeEditor) fontIndex(family string) int {
	for i, f := range e.fonts {
		if strings.EqualFold(f, family) {
			return i
		}
	}
	if family == "" {
		return 0
	}
	e.fonts = append(e.fonts, family)
	e.font.AddItem(family)
	return len(e.fonts) - 1
}

// indexOf is the index of v in list, or fallback (what the renderer does
// with an unknown value: a solid color, centered text).
func indexOf[T comparable](list []T, v T, fallback int) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return fallback
}

// validate reports whether the theme can be saved, setting the error text.
func (e *ThemeEditor) validate() bool {
	if strings.TrimSpace(e.theme.Name) == "" {
		e.errText = "Name is required."
		e.render()
		e.name.AsControl().GrabFocus()
		return false
	}
	e.errText = ""
	e.render()
	return true
}

func (e *ThemeEditor) save() {
	if !e.validate() {
		return
	}
	t := e.theme
	t.Name = strings.TrimSpace(t.Name)
	e.dlg.hide()
	if e.OnSave != nil {
		e.OnSave(t)
	}
}

func (e *ThemeEditor) applyPressed() {
	if !e.validate() {
		return
	}
	t := e.theme
	t.Name = strings.TrimSpace(t.Name)
	if e.OnApply != nil {
		e.OnApply(t)
	}
}
