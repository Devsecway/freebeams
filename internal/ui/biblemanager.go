package ui

import (
	"reflect"

	"graphics.gd/classdb/Button"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventKey"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/ProgressBar"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Theme"
	"graphics.gd/classdb/Tree"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// BibleManagerRow is one row of the Bible Manager's list, already formatted.
type BibleManagerRow struct {
	Name      string // "King James Version (KJV)"
	Language  string
	Source    string // Built-in / Bibles folder / Library only
	Status    string // Installed / Not installed
	Size      string
	Installed bool
}

// BibleManagerValues is everything the Bible Manager shows.
type BibleManagerValues struct {
	Rows     []BibleManagerRow
	Selected int    // -1: none
	Detail   string // the selected row's details (verses, copyright, file)
	Folder   string // the Bibles folder path
	Status   string
	Error    bool
	Busy     bool    // a task runs: the list stays, the action buttons wait
	Progress float64 // 0..1 while Busy
	CanAdd   bool    // for the selected row
	CanDel   bool
	CanSave  bool
}

// BibleManager lists every translation it can add (built in, in the Bibles
// folder) merged with the library, and adds, removes and packages them
// (Scriptures ▸ + / gear ▸ Manage Bibles…). It is a dumb view: the app owns
// the rows, the selection and the running task (SetValues).
type BibleManager struct {
	OnSelect     func(i int)
	OnActivate   func(i int) // double-click / Enter: Add a row that is not installed
	OnAdd        func(i int)
	OnRemove     func(i int)
	OnSave       func(i int) // Save as Package
	OnAddFile    func()      // Add from File…
	OnOpenFolder func()
	OnRefresh    func()
	OnClose      func()

	dlg      *dialogWindow
	list     Tree.Instance
	detail   Label.Instance
	folder   Label.Instance
	status   Label.Instance
	progress ProgressBar.Instance
	add      Button.Instance
	remove   Button.Instance
	save     Button.Instance
	addFile  Button.Instance
	refresh  Button.Instance

	values BibleManagerValues
	shown  []BibleManagerRow // what the list shows (rebuilt only on change)
	built  bool
}

// bibleManagerColumns are the list's columns and their expand ratios
// (0: fixed width).
var bibleManagerColumns = []struct {
	title string
	ratio int
	min   float32
}{
	{"Name", 4, 0}, {"Language", 0, 96}, {"Source", 0, 112}, {"Status", 0, 104}, {"Size", 0, 72},
}

// NewBibleManager builds the dialog. MainWindow adds AsWindow() to the tree.
func NewBibleManager() *BibleManager {
	m := &BibleManager{values: BibleManagerValues{Selected: -1}}
	m.dlg = newDialogWindow("Bible Manager", 780, 500)
	m.dlg.onOK = m.dlg.close
	m.dlg.onClose = func() {
		if m.OnClose != nil {
			m.OnClose()
		}
	}

	m.list = Tree.New()
	m.list.SetHideRoot(true)
	m.list.SetColumns(len(bibleManagerColumns))
	m.list.SetColumnTitlesVisible(true)
	m.list.SetSelectMode(Tree.SelectRow)
	m.list.SetAllowReselect(true)
	for i, c := range bibleManagerColumns {
		m.list.SetColumnTitle(i, c.title)
		m.list.SetColumnTitleAlignment(i, GUI.HorizontalAlignmentLeft)
		m.list.SetColumnClipContent(i, true)
		if c.ratio > 0 {
			m.list.SetColumnExpand(i, true)
			m.list.SetColumnExpandRatio(i, c.ratio)
		} else {
			m.list.SetColumnExpand(i, false)
			m.list.SetColumnCustomMinimumWidth(i, int(c.min))
		}
	}
	m.list.AsNode().SetName("BibleManagerList")
	expand(m.list.AsControl())
	m.list.OnItemSelected(func() {
		if i := treeIndex(m.list.GetSelected()); i >= 0 && m.OnSelect != nil {
			m.OnSelect(i)
		}
	})
	m.list.OnItemActivated(func() {
		if i := treeIndex(m.list.GetSelected()); i >= 0 && m.OnActivate != nil {
			m.OnActivate(i)
		}
	})
	// Delete / Backspace asks to remove the selected translation.
	m.list.AsControl().OnGuiInput(func(ev InputEvent.Instance) {
		k, ok := Object.As[InputEventKey.Instance](ev)
		if !ok || !k.AsInputEvent().IsPressed() || k.AsInputEvent().IsEcho() {
			return
		}
		if key := k.Keycode(); (key == Input.KeyDelete || key == Input.KeyBackspace) && m.values.CanDel && m.OnRemove != nil {
			m.OnRemove(m.values.Selected)
			acceptEvent(m.list.AsControl())
		}
	})

	m.detail = label("", vHint)
	m.detail.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)

	sel := func(f *func(int)) func() {
		return func() {
			if *f != nil && m.values.Selected >= 0 {
				(*f)(m.values.Selected)
			}
		}
	}
	m.add = button("Add", 75, sel(&m.OnAdd))
	m.remove = button("Remove…", 84, sel(&m.OnRemove))
	m.save = button("Save as Package", 124, sel(&m.OnSave))
	m.addFile = button("Add from File…", 118, func() {
		if m.OnAddFile != nil {
			m.OnAddFile()
		}
	})
	openFolder := button("Open Bibles Folder", 132, func() {
		if m.OnOpenFolder != nil {
			m.OnOpenFolder()
		}
	})
	m.refresh = button("Refresh", 75, func() {
		if m.OnRefresh != nil {
			m.OnRefresh()
		}
	})
	actions := hbox(8)
	add(actions, m.add.AsControl(), m.remove.AsControl(), m.save.AsControl(), spacer(),
		m.addFile.AsControl(), openFolder.AsControl(), m.refresh.AsControl())

	m.folder = label("", vHint)
	m.folder.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	m.folder.SetTextOverrunBehavior(TextServer.OverrunTrimEllipsis)
	note := wrapLabel("Drop .fbbible packages, Bible JSON files or EasyWorship .ewb files into this folder, then Refresh. "+
		"Copyrighted translations stay on this computer: add them only if your church holds a licence.", vHint)

	m.status = label("", vCaption)
	m.status.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	m.progress = ProgressBar.New()
	m.progress.SetShowPercentage(false)
	m.progress.AsRange().SetMaxValue(1)
	m.progress.AsRange().SetStep(0.001)
	m.progress.AsControl().SetCustomMinimumSize(Vector2.New(180, 10))
	m.progress.AsControl().SetSizeFlagsVertical(Control.SizeShrinkCenter)

	add(m.dlg.body,
		dialogHeader("bible", "Bible Manager", "Add translations to your library, remove them, or keep them as packages"),
		fixed(0, 4), m.list.AsControl(), m.detail.AsControl(), fixed(0, 2), actions.AsControl(), fixed(0, 6),
		m.folder.AsControl(), note.AsControl())
	m.dlg.ok.SetText("Close")
	add(m.dlg.footer, m.status.AsControl(), m.progress.AsControl(), fixed(8, 0), m.dlg.ok.AsControl())
	Object.Free(m.dlg.cancel) // Close only: the Cancel button never joins the tree
	m.dlg.cancel = Button.Nil
	m.dlg.win.AsNode().SetName("BibleManager")
	m.render()
	return m
}

// AsWindow returns the dialog window node.
func (m *BibleManager) AsWindow() Window.Instance { return m.dlg.win }

// SetTheme sets the dialog's theme.
func (m *BibleManager) SetTheme(th Theme.Instance) { m.dlg.setTheme(th) }

// Open shows the dialog.
func (m *BibleManager) Open() {
	m.render()
	m.dlg.popup()
	m.list.AsControl().GrabFocus()
}

// Hide closes the dialog without calling OnClose.
func (m *BibleManager) Hide() { m.dlg.hide() }

// Visible reports whether the dialog is on screen.
func (m *BibleManager) Visible() bool { return m.dlg.win.Visible() }

// ListControl is the translations list (focus and key hooks).
func (m *BibleManager) ListControl() Control.Instance { return m.list.AsControl() }

// SetValues projects the app's Bible Manager state.
func (m *BibleManager) SetValues(v BibleManagerValues) {
	m.values = v
	m.render()
}

func (m *BibleManager) render() {
	v := m.values
	quietly(m.list.AsControl(), func() {
		if !m.built || !reflect.DeepEqual(v.Rows, m.shown) {
			m.built = true
			m.shown = append([]BibleManagerRow(nil), v.Rows...)
			m.list.Clear()
			root := m.list.CreateItem()
			for _, r := range v.Rows {
				it := root.CreateChild()
				for col, text := range []string{r.Name, r.Language, r.Source, r.Status, r.Size} {
					it.SetText(col, text)
				}
				it.SetTextAlignment(4, GUI.HorizontalAlignmentRight)
				it.SetCustomMinimumHeight(24)
			}
		}
		selectTreeRow(m.list, v.Selected)
	})
	m.detail.SetText(v.Detail)
	m.folder.SetText("Bibles folder: " + v.Folder)
	m.folder.AsControl().SetTooltipText(v.Folder)
	m.status.SetText(v.Status)
	if v.Error {
		m.status.AsControl().SetThemeTypeVariation(vError)
	} else {
		m.status.AsControl().SetThemeTypeVariation(vCaption)
	}
	m.progress.AsCanvasItem().SetVisible(v.Busy)
	m.progress.AsRange().SetValueNoSignal(Float.X(v.Progress))
	m.add.AsBaseButton().SetDisabled(v.Busy || !v.CanAdd)
	m.remove.AsBaseButton().SetDisabled(v.Busy || !v.CanDel)
	m.save.AsBaseButton().SetDisabled(v.Busy || !v.CanSave)
	m.addFile.AsBaseButton().SetDisabled(v.Busy)
	m.refresh.AsBaseButton().SetDisabled(v.Busy)
	flushCalls(m.list.AsControl())
}
