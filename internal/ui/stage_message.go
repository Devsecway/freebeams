package ui

import (
	"graphics.gd/classdb/Button"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Theme"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Vector2"
)

// StageMessageMax is the longest stage message the dialog takes.
const StageMessageMax = 200

// StageMessageDialog is Live ▸ Stage Message…: a one-line message to the
// musicians and the speaker (EasyWorship's FoldBack alert) that only the
// stage display shows. Show (Enter) puts the typed text up, Clear takes the
// message down, Cancel / Esc changes nothing.
type StageMessageDialog struct {
	// OnShow receives the typed message ("" when the box is empty).
	OnShow func(text string)
	// OnClear takes the message down.
	OnClear func()

	dlg   *dialogWindow
	edit  LineEdit.Instance
	clear Button.Instance
}

// NewStageMessageDialog builds the dialog. MainWindow adds AsWindow() to the tree.
func NewStageMessageDialog() *StageMessageDialog {
	m := &StageMessageDialog{}
	m.dlg = newDialogWindow("Stage Message", 460, 170)
	m.dlg.win.AsNode().SetName("StageMessageDialog")
	caption := label("Message for the stage display", "")
	caption.SetClipText(false)
	caption.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	m.edit = LineEdit.New()
	m.edit.SetMaxLength(StageMessageMax)
	m.edit.SetPlaceholderText("Repeat the chorus")
	m.edit.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	m.edit.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
	m.edit.OnTextSubmitted(func(string) { m.dlg.onOK() })
	note := label("Only the stage display shows it; the room does not see it.", vHint)
	note.SetAutowrapMode(TextServer.AutowrapWordSmart)
	note.SetClipText(false)
	note.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	add(m.dlg.body, caption.AsControl(), m.edit.AsControl(), note.AsControl())

	m.clear = Button.New()
	m.clear.SetText("Clear")
	m.clear.AsControl().SetCustomMinimumSize(Vector2.New(75, 25))
	m.clear.AsBaseButton().OnPressed(m.pressClear)
	m.dlg.ok.SetText("Show")
	add(m.dlg.footer, m.clear.AsControl(), spacer(), m.dlg.ok.AsControl(), m.dlg.cancel.AsControl())
	m.dlg.onOK = func() {
		text := m.edit.Text()
		m.dlg.hide()
		if m.OnShow != nil {
			m.OnShow(text)
		}
	}
	return m
}

// AsWindow returns the dialog window node.
func (m *StageMessageDialog) AsWindow() Window.Instance { return m.dlg.win }

// SetTheme sets the dialog's theme.
func (m *StageMessageDialog) SetTheme(th Theme.Instance) { m.dlg.setTheme(th) }

// Open shows the dialog with text in the box (selected, so typing replaces
// it). Clear is enabled while a message is up.
func (m *StageMessageDialog) Open(text string, showing bool) {
	m.edit.SetText(text)
	m.clear.AsBaseButton().SetDisabled(!showing)
	m.dlg.popup()
	m.edit.AsControl().GrabFocus()
	m.edit.SelectAll()
}

// Visible reports whether the dialog is on screen.
func (m *StageMessageDialog) Visible() bool { return m.dlg.win.Visible() }

// Text returns the text in the box.
func (m *StageMessageDialog) Text() string { return m.edit.Text() }

// Clear presses the Clear button (the control API's way to it). It
// reports false when the dialog is closed or Clear is disabled.
func (m *StageMessageDialog) Clear() bool {
	if !m.Visible() || m.clear.AsBaseButton().Disabled() {
		return false
	}
	m.pressClear()
	return true
}

func (m *StageMessageDialog) pressClear() {
	m.dlg.hide()
	if m.OnClear != nil {
		m.OnClear()
	}
}
