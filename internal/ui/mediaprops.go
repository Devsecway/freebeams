package ui

import (
	"freebeams/internal/models"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/OptionButton"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Theme"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Vector2"
)

// MediaPropertiesDialog is the media tile menu's Edit Properties…: a small
// version of EasyWorship's Form_VideoProperties with what FreeBeams keeps
// per item: Title, Aspect Ratio (Fill / Fit / Stretch), Repeating (Loop /
// Play Once; videos and audio) and Tags (the Info tab's Tags memo). OK
// reports the edited properties through OnOK; Cancel / Esc changes nothing.
type MediaPropertiesDialog struct {
	OnOK func(p models.MediaProperties)

	dlg     *dialogWindow
	heading Label.Instance
	title   LineEdit.Instance
	aspect  OptionButton.Instance
	repeat  OptionButton.Instance
	repRow  Control.Instance
	tags    LineEdit.Instance
}

// mediaAspectLabels and mediaRepeatLabels follow models.MediaAspects /
// models.MediaRepeats (the tile menu's wording).
var (
	mediaAspectLabels = []string{"Fill", "Fit", "Stretch"}
	mediaRepeatLabels = []string{"Loop", "Play Once"}
)

// NewMediaPropertiesDialog builds the dialog. MainWindow adds AsWindow() to the tree.
func NewMediaPropertiesDialog() *MediaPropertiesDialog {
	d := &MediaPropertiesDialog{}
	d.dlg = newDialogWindow("Media Properties", 460, 250)
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
	add(d.dlg.body, d.heading.AsControl(), row("Title", d.title.AsControl()), row("Aspect Ratio", d.aspect.AsControl()),
		d.repRow, row("Tags", d.tags.AsControl()))
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

// AsWindow returns the dialog window node.
func (d *MediaPropertiesDialog) AsWindow() Window.Instance { return d.dlg.win }

// SetTheme sets the dialog's theme.
func (d *MediaPropertiesDialog) SetTheme(th Theme.Instance) { d.dlg.setTheme(th) }

// Open shows the dialog for an item: kind decides the window title (Video /
// Audio / Image Properties) and whether Repeating is offered (not for stills).
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
	return p
}

// SetFields fills the fields (the control API's way of typing into them);
// an empty aspect or repeat leaves that choice.
func (d *MediaPropertiesDialog) SetFields(p models.MediaProperties) {
	d.title.SetText(p.Title)
	d.tags.SetText(p.Tags)
	if p.Aspect.Valid() {
		d.aspect.Select(indexOf(models.MediaAspects, p.Aspect, 0))
	}
	if p.Repeat.Valid() {
		d.repeat.Select(indexOf(models.MediaRepeats, p.Repeat, 0))
	}
	flushCalls(d.title.AsControl())
}

// Accept presses OK (the control API).
func (d *MediaPropertiesDialog) Accept() { d.dlg.onOK() }

// Cancel presses Cancel (the control API).
func (d *MediaPropertiesDialog) Cancel() { d.dlg.close() }
