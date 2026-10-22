package ui

import (
	"slices"

	"graphics.gd/classdb/Button"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/MenuButton"
	"graphics.gd/classdb/Tree"
)

// HotkeyRow is one row of the hotkey table: an action and its keys.
type HotkeyRow struct {
	Action string
	Keys   string   // the labels joined for the table ("→ / Space / N")
	Labels []string // one label per key (the Remove menu)
}

// HotkeysPage is Options ▸ Hotkeys: a table of actions and their keys. Select
// an action, then "Add Key…" (the next key press, with its modifiers, is
// captured and reported through OnKey) or Remove ▾ (one of its keys). "Reset
// to Defaults" restores EasyWorship's keys; "Import from EasyWorship" reads
// its HotKeys.2.ini. It is a dumb view: the app owns the keymap and whether a
// capture is running (SetCapturing), and reports refusals through SetStatus.
type HotkeysPage struct {
	OnAdd    func(row int)      // start capturing a key for the row's action
	OnRemove func(row, key int) // remove key (index into the row's Labels)
	OnReset  func()
	OnImport func()
	// OnKey receives each key press while a capture runs (Escape included:
	// the app cancels the capture). The dialog does not act on these keys.
	OnKey func(ev InputEvent.Instance)

	tree   Tree.Instance
	add    Button.Instance
	remove MenuButton.Instance
	reset  Button.Instance
	imp    Button.Instance
	status Label.Instance

	rows        []HotkeyRow
	shownRows   []HotkeyRow // what the tree shows (rebuilt only on change)
	sel         int
	capturing   bool
	statusText  string
	statusErr   bool
	importAvail bool
	rendering   bool
}

func newHotkeysPage() *HotkeysPage {
	p := &HotkeysPage{sel: -1}
	p.tree = Tree.New()
	p.tree.SetHideRoot(true)
	p.tree.SetColumns(2)
	p.tree.SetColumnTitlesVisible(true)
	p.tree.SetColumnTitle(0, "Action")
	p.tree.SetColumnTitle(1, "Keys")
	p.tree.SetColumnExpand(0, true)
	p.tree.SetColumnExpand(1, true)
	p.tree.SetColumnExpandRatio(0, 3)
	p.tree.SetColumnExpandRatio(1, 2)
	p.tree.SetSelectMode(Tree.SelectRow)
	p.tree.AsNode().SetName("HotkeyTable")
	expand(p.tree.AsControl())
	p.tree.OnItemSelected(func() {
		if p.rendering {
			return
		}
		p.sel = treeIndex(p.tree.GetSelected())
		p.render()
	})

	p.add = Button.New()
	p.add.SetText("Add Key…")
	p.add.AsBaseButton().OnPressed(func() {
		if p.OnAdd != nil && p.sel >= 0 {
			p.OnAdd(p.sel)
		}
	})
	p.remove = MenuButton.New()
	p.remove.AsButton().SetText("Remove ▾")
	p.remove.AsButton().SetFlat(false)
	pm := p.remove.GetPopup()
	p.remove.OnAboutToPopup(func() {
		pm.Clear()
		if p.sel >= 0 && p.sel < len(p.rows) {
			for i, l := range p.rows[p.sel].Labels {
				pm.MoreArgs().AddItem(l, i, 0)
			}
		}
	})
	pm.OnIdPressed(func(id int) {
		if p.OnRemove != nil && p.sel >= 0 {
			p.OnRemove(p.sel, id)
		}
	})
	p.reset = Button.New()
	p.reset.SetText("Reset to Defaults")
	p.reset.AsBaseButton().OnPressed(func() {
		if p.OnReset != nil {
			p.OnReset()
		}
	})
	p.imp = Button.New()
	p.imp.SetText("Import from EasyWorship")
	p.imp.AsBaseButton().OnPressed(func() {
		if p.OnImport != nil {
			p.OnImport()
		}
	})
	p.status = label("", vHint)
	p.status.SetClipText(false)
	p.status.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	return p
}

// build lays the page out (the dialog stacks it with the other sections).
func (p *HotkeysPage) build() Control.Instance {
	buttons := hbox(8)
	add(buttons, p.add.AsControl(), p.remove.AsControl(), spacer(), p.imp.AsControl(), p.reset.AsControl())
	col := vbox(6)
	expand(col.AsControl())
	add(col, p.tree.AsControl(), buttons.AsControl(), p.status.AsControl(),
		hint("Select an action, then Add Key… and press the key, with Ctrl (⌘), Alt or Shift if you like. An action can have "+
			"several keys. Hotkeys are off while a text field or a dialog has focus, so ⌘C still copies text."))
	return col.AsControl()
}

// SetRows fills the table.
func (p *HotkeysPage) SetRows(rows []HotkeyRow) {
	p.rows = rows
	if p.sel >= len(rows) {
		p.sel = -1
	}
	p.render()
}

// Select selects a row (-1 for none), as a click would but without reporting.
func (p *HotkeysPage) Select(row int) {
	p.sel = row
	p.render()
}

// Selected returns the selected row (-1 for none).
func (p *HotkeysPage) Selected() int { return p.sel }

// SetCapturing says whether the next key press is being captured.
func (p *HotkeysPage) SetCapturing(on bool) {
	p.capturing = on
	p.render()
}

// Capturing reports whether a capture runs (the dialog routes keys to OnKey).
func (p *HotkeysPage) Capturing() bool { return p.capturing }

// SetStatus shows a line under the table: a prompt, a result or (isErr) a
// refusal in red.
func (p *HotkeysPage) SetStatus(text string, isErr bool) {
	p.statusText, p.statusErr = text, isErr
	p.render()
}

// SetImportAvailable enables "Import from EasyWorship" (its file exists).
func (p *HotkeysPage) SetImportAvailable(ok bool) {
	p.importAvail = ok
	p.render()
}

func (p *HotkeysPage) render() {
	p.rendering = true
	defer func() { p.rendering = false }()
	if !slices.EqualFunc(p.rows, p.shownRows, func(a, b HotkeyRow) bool { return a.Action == b.Action && a.Keys == b.Keys }) {
		p.shownRows = slices.Clone(p.rows)
		p.tree.Clear()
		root := p.tree.CreateItem()
		for _, r := range p.rows {
			it := p.tree.MoreArgs().CreateItem(root, -1)
			it.SetText(0, r.Action)
			it.SetText(1, r.Keys)
		}
	}
	quietly(p.tree.AsControl(), func() { selectTreeRow(p.tree, p.sel) })
	none := p.sel < 0 || p.sel >= len(p.rows)
	p.add.AsBaseButton().SetDisabled(none || p.capturing)
	p.remove.AsBaseButton().SetDisabled(none || p.capturing || len(p.rows[max(p.sel, 0)].Labels) == 0)
	p.reset.AsBaseButton().SetDisabled(p.capturing)
	p.imp.AsBaseButton().SetDisabled(p.capturing || !p.importAvail)
	p.status.SetText(p.statusText)
	v := vHint
	if p.statusErr {
		v = vError
	}
	p.status.AsControl().SetThemeTypeVariation(v)
	flushCalls(p.tree.AsControl())
}
