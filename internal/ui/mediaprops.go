package ui

import (
	"freebeams/internal/models"

	"graphics.gd/classdb/CheckBox"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/HSlider"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/OptionButton"
	"graphics.gd/classdb/Range"
	"graphics.gd/classdb/SpinBox"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Theme"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// MediaPropertiesDialog is the media tile menu's Edit Properties…: a small
// version of EasyWorship's Form_VideoProperties with what FreeBeams keeps
// per item: Title, Aspect Ratio (Fill / Fit / Stretch), Repeating (Loop /
// Play Once; videos and audio), Volume (Carry Over, or Fixed at 1..100 %)
// and Mute (videos and audio: the Audio Properties' Volume / Mute) and Tags
// (the Info tab's Tags memo). OK reports the edited properties through OnOK;
// Cancel / Esc changes nothing.
type MediaPropertiesDialog struct {
	OnOK func(p models.MediaProperties)

	dlg     *dialogWindow
	heading Label.Instance
	title   LineEdit.Instance
	aspect  OptionButton.Instance
	repeat  OptionButton.Instance
	repRow  Control.Instance
	tags    LineEdit.Instance

	// Volume / Mute (videos and audio).
	volMode   OptionButton.Instance // mediaVolumeLabels
	volSlider HSlider.Instance
	volSpin   SpinBox.Instance
	mute      CheckBox.Instance
	volRow    Control.Instance
	muteRow   Control.Instance
}

// mediaVolumeLabels are the Volume choices: no volume of its own (the
// monitor's volume carries over from the previous item) or a fixed one.
var mediaVolumeLabels = []string{"Carry Over", "Fixed"}

// volumeFixed is the "Fixed" index of mediaVolumeLabels.
const volumeFixed = 1

// mediaAspectLabels and mediaRepeatLabels follow models.MediaAspects /
// models.MediaRepeats (the tile menu's wording).
var (
	mediaAspectLabels = []string{"Fill", "Fit", "Stretch"}
	mediaRepeatLabels = []string{"Loop", "Play Once"}
)

// NewMediaPropertiesDialog builds the dialog. MainWindow adds AsWindow() to the tree.
func NewMediaPropertiesDialog() *MediaPropertiesDialog {
	d := &MediaPropertiesDialog{}
	d.dlg = newDialogWindow("Media Properties", 500, 320)
	d.dlg.win.AsNode().SetName("MediaPropertiesDialog")
	d.heading = label("", vHint)
	d.heading.SetClipText(false)
	d.heading.SetTextOverrunBehavior(TextServer.OverrunTrimEllipsis)
	row := func(caption string, c Control.Instance) Control.Instance {
		r := hbox(8)
		l := label(caption, "")
		l.SetClipText(false)
		l.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
		l.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
		l.AsControl().SetCustomMinimumSize(Vector2.New(90, 0))
		c.SetSizeFlagsHorizontal(Control.SizeExpandFill)
		add(r, l.AsControl(), c)
		return r.AsControl()
	}
	d.title = LineEdit.New()
	d.title.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
	d.title.OnTextSubmitted(func(string) { d.dlg.onOK() })
	d.aspect = optionOf(mediaAspectLabels)
	d.repeat = optionOf(mediaRepeatLabels)
	d.tags = LineEdit.New()
	d.tags.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
	d.tags.SetPlaceholderText("worship, intro, calm")
	d.tags.OnTextSubmitted(func(string) { d.dlg.onOK() })
	d.repRow = row("Repeating", d.repeat.AsControl())
	d.buildVolume()
	d.volRow = row("Volume", d.volRow)
	d.muteRow = row("", d.mute.AsControl())
	add(d.dlg.body, d.heading.AsControl(), row("Title", d.title.AsControl()), row("Aspect Ratio", d.aspect.AsControl()),
		d.repRow, d.volRow, d.muteRow, row("Tags", d.tags.AsControl()))
	add(d.dlg.footer, spacer(), d.dlg.ok.AsControl(), d.dlg.cancel.AsControl())
	d.dlg.onOK = func() {
		p := d.Properties()
		d.dlg.hide()
		if d.OnOK != nil {
			d.OnOK(p)
		}
	}
	return d
}

// buildVolume makes the Volume row's parts (Carry Over / Fixed, a slider
// and a percent box that follow each other; EasyWorship's TrackBar_Volume +
// SpinEdit_Volume) and the Mute check box. d.volRow is the row's box.
func (d *MediaPropertiesDialog) buildVolume() {
	d.volMode = optionOf(mediaVolumeLabels)
	d.volMode.AsControl().SetCustomMinimumSize(Vector2.New(120, 24))
	d.volMode.OnItemSelected(func(idx int) { d.showVolumeMode(idx == volumeFixed) })
	d.volSlider = HSlider.New()
	d.volSlider.AsControl().SetCustomMinimumSize(Vector2.New(80, 20))
	d.volSlider.AsControl().SetSizeFlagsVertical(Control.SizeShrinkCenter)
	d.volSlider.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	d.volSpin = SpinBox.New()
	d.volSpin.SetSuffix("%")
	d.volSpin.AsControl().SetCustomMinimumSize(Vector2.New(84, 0))
	for _, rg := range []Range.Instance{d.volSlider.AsRange(), d.volSpin.AsRange()} {
		rg.SetMinValue(1)
		rg.SetMaxValue(Float.X(models.MaxMediaVolume))
		rg.SetStep(1)
	}
	d.volSlider.AsRange().OnValueChanged(func(v Float.X) { d.volSpin.AsRange().SetValueNoSignal(v) })
	d.volSpin.AsRange().OnValueChanged(func(v Float.X) { d.volSlider.AsRange().SetValueNoSignal(v) })
	d.mute = CheckBox.New()
	d.mute.AsButton().SetText("Mute (starts silent; the transport can still turn it up)")
	r := hbox(8)
	add(r, d.volMode.AsControl(), d.volSlider.AsControl(), d.volSpin.AsControl())
	d.volRow = r.AsControl()
}

// showVolumeMode enables the slider and the percent box for a fixed
// volume and dims them for Carry Over.
func (d *MediaPropertiesDialog) showVolumeMode(fixed bool) {
	d.volSlider.AsSlider().SetEditable(fixed)
	d.volSpin.SetEditable(fixed)
	tint := Color.RGBA{R: 1, G: 1, B: 1, A: 1}
	if !fixed {
		tint.A = 0.4
	}
	d.volSlider.AsCanvasItem().SetModulate(tint)
	d.volSpin.AsCanvasItem().SetModulate(tint)
}

// setVolume shows an item volume (percent; 0 = Carry Over) and Mute.
func (d *MediaPropertiesDialog) setVolume(volume int, mute bool) {
	fixed := volume > 0
	v := models.MaxMediaVolume
	if fixed {
		v = models.ClampMediaVolume(volume)
	}
	if fixed {
		d.volMode.Select(volumeFixed)
	} else {
		d.volMode.Select(0)
	}
	d.volSlider.AsRange().SetValueNoSignal(Float.X(v))
	d.volSpin.AsRange().SetValueNoSignal(Float.X(v))
	d.mute.AsBaseButton().SetPressedNoSignal(mute)
	d.showVolumeMode(fixed)
}

// AsWindow returns the dialog window node.
func (d *MediaPropertiesDialog) AsWindow() Window.Instance { return d.dlg.win }

// SetTheme sets the dialog's theme.
func (d *MediaPropertiesDialog) SetTheme(th Theme.Instance) { d.dlg.setTheme(th) }

// Open shows the dialog for an item: kind decides the window title (Video /
// Audio / Image Properties) and whether Repeating, Volume and Mute are
// offered (not for stills).
func (d *MediaPropertiesDialog) Open(kind models.MediaKind, path string, p models.MediaProperties) {
	name := map[models.MediaKind]string{models.MediaVideo: "Video", models.MediaAudio: "Audio"}[kind]
	if name == "" {
		name = "Image"
	}
	d.dlg.win.SetTitle(name + " Properties")
	d.heading.SetText("Edit properties for " + path)
	d.title.SetText(p.Title)
	d.aspect.Select(indexOf(models.MediaAspects, p.Aspect.Normalized(), 0))
	d.repeat.Select(indexOf(models.MediaRepeats, p.Repeat.Normalized(), 0))
	d.repRow.AsCanvasItem().SetVisible(kind.Plays())
	d.volRow.AsCanvasItem().SetVisible(kind.Plays())
	d.muteRow.AsCanvasItem().SetVisible(kind.Plays())
	d.setVolume(p.Volume, p.Mute)
	d.tags.SetText(p.Tags)
	d.dlg.popup()
	d.title.AsControl().GrabFocus()
	d.title.SelectAll()
}

// Visible reports whether the dialog is on screen.
func (d *MediaPropertiesDialog) Visible() bool { return d.dlg.win.Visible() }

// Properties returns what the dialog's fields say now.
func (d *MediaPropertiesDialog) Properties() models.MediaProperties {
	p := models.MediaProperties{Title: d.title.Text(), Tags: d.tags.Text()}
	if i := d.aspect.Selected(); i >= 0 && i < len(models.MediaAspects) {
		p.Aspect = models.MediaAspects[i]
	}
	if i := d.repeat.Selected(); i >= 0 && i < len(models.MediaRepeats) {
		p.Repeat = models.MediaRepeats[i]
	}
	if d.volMode.Selected() == volumeFixed {
		p.Volume = models.ClampMediaVolume(int(d.volSpin.AsRange().Value() + 0.5))
	}
	p.Mute = d.mute.AsBaseButton().ButtonPressed()
	return p
}

// SetFields fills the fields (the control API's way of typing into them);
// an empty aspect or repeat leaves that choice; the volume (0 = Carry
// Over) and Mute are always set.
func (d *MediaPropertiesDialog) SetFields(p models.MediaProperties) {
	d.title.SetText(p.Title)
	d.tags.SetText(p.Tags)
	if p.Aspect.Valid() {
		d.aspect.Select(indexOf(models.MediaAspects, p.Aspect, 0))
	}
	if p.Repeat.Valid() {
		d.repeat.Select(indexOf(models.MediaRepeats, p.Repeat, 0))
	}
	d.setVolume(p.Volume, p.Mute)
	flushCalls(d.title.AsControl())
}

// Accept presses OK (the control API).
func (d *MediaPropertiesDialog) Accept() { d.dlg.onOK() }

// Cancel presses Cancel (the control API).
func (d *MediaPropertiesDialog) Cancel() { d.dlg.close() }
