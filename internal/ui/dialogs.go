package ui

import (
	"strings"

	"freebeams/internal/models"

	"graphics.gd/classdb/Button"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/GridContainer"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventKey"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/MarginContainer"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/OptionButton"
	"graphics.gd/classdb/TextEdit"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Theme"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
	"graphics.gd/variant/Vector2i"
)

// dialogWindow is a modal OS window (embed_subwindows=false) with a bk2
// background, content margins and an OK / Cancel footer (75 x 25 buttons).
type dialogWindow struct {
	win     Window.Instance
	body    Control.Instance // content column
	footer  Control.Instance // footer row (left part is free for status)
	ok      Button.Instance
	cancel  Button.Instance
	size    Vector2.XY // logical size
	onOK    func()
	onClose func()
	// host returns the node the window must live under when it opens (see
	// MainWindow.dialogHost); nil keeps the current parent.
	host func(self Node.ID) Node.Instance

	// keyHook sees every key press first (Options ▸ Hotkeys capturing a
	// key); true marks it handled, so the dialog's controls never get it.
	keyHook func(ev InputEvent.Instance) bool
}

func newDialogWindow(title string, w, h float32) *dialogWindow {
	d := &dialogWindow{size: Vector2.New(w, h)}
	d.win = Window.New()
	d.win.SetTitle(title)
	d.win.SetTransient(true)
	d.win.SetExclusive(true)
	d.win.SetWrapControls(false)
	d.win.SetVisible(false)
	d.win.SetInitialPosition(Window.WindowInitialPositionCenterMainWindowScreen)
	d.win.OnCloseRequested(func() { d.close() })
	d.win.OnWindowInput(d.input)

	bg := panel(vBk2)
	bg.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	d.win.AsNode().AddChild(bg.AsNode())
	m := MarginContainer.New()
	for _, side := range []string{"margin_left", "margin_right"} {
		m.AsControl().AddThemeConstantOverride(side, 14)
	}
	m.AsControl().AddThemeConstantOverride("margin_top", 12)
	m.AsControl().AddThemeConstantOverride("margin_bottom", 12)
	add(bg, m.AsControl())
	col := vbox(8)
	add(m, col.AsControl())
	body := vbox(6)
	expand(body.AsControl())
	d.body = body.AsControl()

	foot := hbox(8)
	d.footer = foot.AsControl()
	d.ok = Button.New()
	d.ok.SetText("OK")
	d.cancel = Button.New()
	d.cancel.SetText("Cancel")
	for _, b := range []Button.Instance{d.ok, d.cancel} {
		b.AsControl().SetCustomMinimumSize(Vector2.New(75, 25))
	}
	d.ok.AsBaseButton().OnPressed(func() {
		if d.onOK != nil {
			d.onOK()
		}
	})
	d.cancel.AsBaseButton().OnPressed(func() { d.close() })
	add(col, d.body, d.footer)
	return d
}

func (d *dialogWindow) setTheme(th Theme.Instance) { d.win.SetTheme(th) }

// popup shows the window centered over its parent window, scaled like it.
// A dialog opened while another one is open becomes that dialog's child:
// Godot allows a window only one exclusive child.
func (d *dialogWindow) popup() {
	if d.host != nil && !d.win.Visible() {
		self := d.win.AsNode()
		if h := d.host(self.ID()); h != (Node.Instance{}) {
			if p := self.GetParent(); p == (Node.Instance{}) || p.ID() != h.ID() {
				self.MoreArgs().Reparent(h, false)
			}
		}
	}
	scale := float32(1)
	if parent := d.win.AsNode().GetParent(); parent != (Node.Instance{}) {
		if pw := Window.Get(parent); pw != Window.Nil {
			scale = pw.ContentScaleFactor()
		}
	}
	d.win.SetContentScaleFactor(scale)
	d.win.SetMinSize(Vector2i.New(int(d.size.X*scale*0.8), int(d.size.Y*scale*0.8)))
	d.win.MoreArgs().PopupCentered(Vector2i.New(int(d.size.X*scale), int(d.size.Y*scale)))
}

func (d *dialogWindow) hide() { d.win.Hide() }

func (d *dialogWindow) close() {
	d.win.Hide()
	if d.onClose != nil {
		d.onClose()
	}
}

func (d *dialogWindow) input(ev InputEvent.Instance) {
	k, ok := Object.As[InputEventKey.Instance](ev)
	if !ok || !k.AsInputEvent().IsPressed() || k.AsInputEvent().IsEcho() {
		return
	}
	if d.keyHook != nil && d.keyHook(ev) {
		setInputHandled(d.win.AsViewport())
		return
	}
	switch k.Keycode() {
	case Input.KeyEscape:
		d.close()
		setInputHandled(d.win.AsViewport())
	case Input.KeyEnter, Input.KeyKpEnter:
		if k.AsInputEventWithModifiers().IsCommandOrControlPressed() && d.onOK != nil {
			d.onOK()
			setInputHandled(d.win.AsViewport())
		}
	}
}

// ---- Song editor ------------------------------------------------------------

// SongEditor is the New / Edit song dialog: a simplified EasyWorship
// Presentation Editor with the song's fields and a lyrics box using
// [Verse 1] / [Chorus] tags. Esc cancels; Title is required.
type SongEditor struct {
	OnSave   func(song models.Song, lyricsText string)
	OnCancel func()

	dlg     *dialogWindow
	fields  map[string]LineEdit.Instance
	lyrics  TextEdit.Instance
	errLbl  Label.Instance
	song    models.Song
	errText string

	// The Theme picker (songthemes.go): the choices after "(Default)" and the
	// song's theme id (0 = default), projected by renderTheme.
	themeOpt     OptionButton.Instance
	themeChoices []ThemeChoice
	themeID      int64
	themesDirty  bool
}

var songFields = []struct{ key, label string }{
	{"title", "Title"}, {"author", "Author"},
	{"copyright", "Copyright"}, {"administrator", "Administrator"},
	{"ccli", "CCLI #"}, {"tags", "Tags"},
}

// NewSongEditor builds the dialog. Add AsNode() to the scene tree (MainWindow
// does this for its own editor).
func NewSongEditor() *SongEditor {
	e := &SongEditor{fields: map[string]LineEdit.Instance{}}
	e.dlg = newDialogWindow("Song Editor", 760, 600)
	e.dlg.onOK = e.save
	e.dlg.onClose = func() {
		if e.OnCancel != nil {
			e.OnCancel()
		}
	}

	grid := GridContainer.New()
	grid.SetColumns(4)
	grid.AsControl().AddThemeConstantOverride("h_separation", 8)
	grid.AsControl().AddThemeConstantOverride("v_separation", 6)
	for _, f := range songFields {
		l := label(f.label, "")
		l.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
		l.AsControl().SetCustomMinimumSize(Vector2.New(84, 0))
		ed := LineEdit.New()
		ed.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
		ed.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
		e.fields[f.key] = ed
		add(grid, l.AsControl(), ed.AsControl())
	}
	e.fields["title"].SetPlaceholderText("Required")
	e.addThemePicker(grid)

	lyricsLbl := label("Lyrics", vTitle)
	e.lyrics = TextEdit.New()
	e.lyrics.SetPlaceholderText("[Verse 1]\nAmazing grace! How sweet the sound\nThat saved a wretch like me!\n\n[Chorus]\n…")
	e.lyrics.AsControl().AddThemeFontOverride("font", monoFont())
	e.lyrics.AsControl().AddThemeFontSizeOverride("font_size", fontBase)
	expand(e.lyrics.AsControl())
	hint := label("Start each section with a tag such as [Verse 1], [Chorus], [Bridge], [Pre-Chorus], [Tag], [Intro] or [Ending], or separate stanzas with a blank line.", vHint)
	hint.SetAutowrapMode(TextServer.AutowrapWordSmart)
	hint.SetClipText(false)
	hint.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	add(e.dlg.body, grid.AsControl(), fixed(0, 4), lyricsLbl.AsControl(), e.lyrics.AsControl(), hint.AsControl())

	e.errLbl = label("", vError)
	e.errLbl.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	add(e.dlg.footer, e.errLbl.AsControl(), e.dlg.ok.AsControl(), e.dlg.cancel.AsControl())
	e.dlg.win.AsNode().SetName("SongEditor")
	return e
}

// AsWindow returns the dialog window (add it under the main window).
func (e *SongEditor) AsWindow() Window.Instance { return e.dlg.win }

// SetTheme sets the dialog's theme (separate OS windows do not inherit it).
func (e *SongEditor) SetTheme(th Theme.Instance) { e.dlg.setTheme(th) }

// Open shows the editor for song (zero ID = new song). lyricsText is the
// tagged lyrics text (the app formats it with models.FormatLyrics).
func (e *SongEditor) Open(song models.Song, lyricsText string) {
	e.song = song
	e.errText = ""
	title := "Edit Song"
	if song.ID == 0 {
		title = "New Song"
	}
	e.dlg.win.SetTitle(title)
	e.fields["title"].SetText(song.Title)
	e.fields["author"].SetText(song.Author)
	e.fields["copyright"].SetText(song.Copyright)
	e.fields["administrator"].SetText(song.Administrator)
	e.fields["ccli"].SetText(song.CCLI)
	e.fields["tags"].SetText(song.Tags)
	e.lyrics.SetText(lyricsText)
	e.themeID = song.ThemeID
	e.render()
	e.dlg.popup()
	e.fields["title"].AsControl().GrabFocus()
}

// Close hides the editor without calling OnCancel.
func (e *SongEditor) Close() { e.dlg.hide() }

// Visible reports whether the editor is open.
func (e *SongEditor) Visible() bool { return e.dlg.win.Visible() }

func (e *SongEditor) render() {
	e.errLbl.SetText(e.errText)
	e.renderTheme()
}

func (e *SongEditor) save() {
	song := e.song
	get := func(k string) string { return strings.TrimSpace(e.fields[k].Text()) }
	song.Title = get("title")
	song.Author = get("author")
	song.Copyright = get("copyright")
	song.Administrator = get("administrator")
	song.CCLI = get("ccli")
	song.Tags = get("tags")
	song.ThemeID = e.savedThemeID()
	if song.Title == "" {
		e.errText = "Title is required."
		e.render()
		e.fields["title"].AsControl().GrabFocus()
		return
	}
	e.errText = ""
	e.render()
	e.dlg.hide()
	if e.OnSave != nil {
		e.OnSave(song, e.lyrics.Text())
	}
}

// ---- Confirm dialog ---------------------------------------------------------

// ConfirmDialog is a simple OK / Cancel question.
type ConfirmDialog struct {
	dlg   *dialogWindow
	msg   Label.Instance
	title string
	onOK  func()
}

// NewConfirmDialog builds the dialog. Add AsWindow() to the scene tree.
func NewConfirmDialog() *ConfirmDialog {
	c := &ConfirmDialog{}
	c.dlg = newDialogWindow("FreeBeams", 420, 150)
	c.msg = label("", "")
	c.msg.SetAutowrapMode(TextServer.AutowrapWordSmart)
	c.msg.SetClipText(false)
	c.msg.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	expand(c.msg.AsControl())
	add(c.dlg.body, c.msg.AsControl())
	add(c.dlg.footer, spacer(), c.dlg.ok.AsControl(), c.dlg.cancel.AsControl())
	c.dlg.onOK = func() {
		c.dlg.hide()
		if f := c.onOK; f != nil {
			c.onOK = nil
			f()
		}
	}
	c.dlg.onClose = func() { c.onOK = nil }
	c.dlg.win.AsNode().SetName("ConfirmDialog")
	return c
}

// AsWindow returns the dialog window node.
func (c *ConfirmDialog) AsWindow() Window.Instance { return c.dlg.win }

// SetTheme sets the dialog's theme.
func (c *ConfirmDialog) SetTheme(th Theme.Instance) { c.dlg.setTheme(th) }

// Ask shows the question; onOK runs only if the user confirms.
func (c *ConfirmDialog) Ask(title, message string, onOK func()) {
	c.onOK = onOK
	c.title = title
	c.dlg.win.SetTitle(title)
	c.msg.SetText(message)
	c.dlg.popup()
	c.dlg.ok.AsControl().GrabFocus()
}

// Visible reports whether the question is on screen.
func (c *ConfirmDialog) Visible() bool { return c.dlg.win.Visible() }

// Title returns the title of the last question asked.
func (c *ConfirmDialog) Title() string { return c.title }

// ---- Message dialog ---------------------------------------------------------

// MessageDialog tells the operator about an error: a heading, the message
// and a single OK button. OnClose runs when the user dismisses it (OK, Esc or
// the window's close button), not when Hide is called.
type MessageDialog struct {
	OnClose func()

	dlg     *dialogWindow
	heading Label.Instance
	msg     Label.Instance
	title   string
	text    string
}

// NewMessageDialog builds the dialog. Add AsWindow() to the scene tree.
func NewMessageDialog() *MessageDialog {
	m := &MessageDialog{}
	m.dlg = newDialogWindow("FreeBeams", 480, 210)
	m.heading = label("", vTitle)
	m.msg = label("", "")
	m.msg.SetAutowrapMode(TextServer.AutowrapWordSmart)
	m.msg.SetClipText(false)
	m.msg.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	expand(m.msg.AsControl())
	add(m.dlg.body, m.heading.AsControl(), m.msg.AsControl())
	add(m.dlg.footer, spacer(), m.dlg.ok.AsControl())
	Object.Free(m.dlg.cancel) // OK only: the Cancel button never joins the tree
	m.dlg.cancel = Button.Nil
	m.dlg.onOK = m.dlg.close
	m.dlg.onClose = func() {
		if m.OnClose != nil {
			m.OnClose()
		}
	}
	m.dlg.win.AsNode().SetName("MessageDialog")
	return m
}

// AsWindow returns the dialog window node.
func (m *MessageDialog) AsWindow() Window.Instance { return m.dlg.win }

// SetTheme sets the dialog's theme.
func (m *MessageDialog) SetTheme(th Theme.Instance) { m.dlg.setTheme(th) }

// Show displays message under a title ("Open Schedule") until dismissed.
func (m *MessageDialog) Show(title, message string) {
	m.title, m.text = title, message
	m.dlg.win.SetTitle(title)
	m.heading.SetText(title)
	m.msg.SetText(message)
	m.dlg.popup()
	m.dlg.ok.AsControl().GrabFocus()
}

// Hide closes the dialog without calling OnClose.
func (m *MessageDialog) Hide() { m.dlg.hide() }

// Visible reports whether the dialog is on screen.
func (m *MessageDialog) Visible() bool { return m.dlg.win.Visible() }

// Title and Text return what the dialog shows.
func (m *MessageDialog) Title() string { return m.title }

// Text returns the message text.
func (m *MessageDialog) Text() string { return m.text }
