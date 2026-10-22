package ui

import (
	"slices"
	"time"

	"freebeams/internal/models"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventKey"
	"graphics.gd/classdb/InputEventMouseButton"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/Tree"
	"graphics.gd/classdb/TreeItem"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

const (
	scriptMenuAddToSchedule = iota
	scriptMenuGoLive
	scriptMenuImportCopyrighted
	scriptMenuImportFile
)

// verseCollapseDelay is how long a plain click on a row of a multi-verse
// selection waits before it narrows the selection to that row: a double-click
// in that time sends the whole selection live instead (as in Finder).
const verseCollapseDelay = 400 * time.Millisecond

// ScripturePanel is the Scriptures resource tab: translations and the
// reference locator on the left, the verses of the listed chapters on the
// right. The verse list is multi-select; the app owns the selection
// (OnVerseClick / OnVerseStep / OnSelectAll report the user's intent and
// SetVerseSelection projects the result). The locator offers book
// completions in a dropdown under the field (SetSuggestions).
type ScripturePanel struct {
	OnBibleChanged func(i int)
	OnReference    func(text string) // on Enter and debounced typing
	// OnReferenceEdited sees every change of the reference field's text at
	// once (the book suggestions follow it); OnReferenceFocus its focus.
	OnReferenceEdited func(text string)
	OnReferenceFocus  func(focused bool)
	// The suggestion dropdown: ↑/↓ move the highlight, Enter / Tab or a click
	// pick a book, Esc dismisses the list.
	OnSuggestionMove    func(delta int)
	OnSuggestionPick    func(i int)
	OnSuggestionDismiss func()
	// OnVerseClick is a left click on verse row i: extend is Shift, toggle
	// Cmd (Ctrl on Windows and Linux). OnVerseStep is ↑/↓ (±1) and Home/End
	// (a large ±delta), Shift extending; OnSelectAll is Cmd+A.
	OnVerseClick func(row int, extend, toggle bool)
	OnVerseStep  func(delta int, extend bool)
	OnSelectAll  func()
	// OnActivate is a double-click on row i, or Enter in the list (i = -1):
	// go live with the selected verses.
	OnActivate      func(i int)
	OnAddToSchedule func()
	OnGoLive        func()
	// OnImportCopyrighted is the gear menu's "Import Copyrighted Bibles from
	// EasyWorship…" (enabled by SetImportCopyrightedAvailable).
	OnImportCopyrighted func()
	// OnImportFile is the gear menu's "Import Bible File…" (a Bible JSON
	// file or folder, or an .ewb file, chosen in the native dialog).
	OnImportFile func()

	split    *Splitter
	locator  *searchLocator
	suggest  *suggestionList
	bibles   Tree.Instance
	verses   Tree.Instance
	status   Label.Instance
	menu     PopupMenu.Instance // verse context menu
	gearMenu PopupMenu.Instance // footer gear: the context menu plus the import
	collapse *ticker            // waits out verseCollapseDelay

	bibleList   []models.Bible
	bible       int
	verseRows   []string
	selRows     []int // projected selection (ascending)
	cursor      int   // projected cursor row
	suggestions []string
	suggestSel  int
	statusText  string
	statusError bool
	importAvail bool
	builtBibles bool
	builtVerses bool
	scrollTo    int // row to bring into view on the next render (-1 none)
	center      bool

	pendingRow int       // a plain click inside a multi-selection, not yet applied
	pendingAt  time.Time // when it applies
}

// NewScripturePanel builds the Scriptures tab.
func NewScripturePanel() *ScripturePanel {
	s := &ScripturePanel{bible: -1, cursor: -1, scrollTo: -1, pendingRow: -1}
	s.split = NewSplitter(SplitHorizontal, 0.35, 150, 150, 0, 0)

	s.locator = newSearchLocator("Enter reference e.g. John 3:16", func(q string) {
		if s.OnReference != nil {
			s.OnReference(q)
		}
	})
	s.locator.onEdit = func(text string) {
		if s.OnReferenceEdited != nil {
			s.OnReferenceEdited(text)
		}
	}
	edit := s.locator.edit.AsControl()
	edit.OnFocusEntered(func() {
		if s.OnReferenceFocus != nil {
			s.OnReferenceFocus(true)
		}
	})
	edit.OnFocusExited(func() {
		if s.OnReferenceFocus != nil {
			s.OnReferenceFocus(false)
		}
	})
	edit.OnGuiInput(func(ev InputEvent.Instance) { s.referenceKey(edit, ev) })

	s.bibles = Tree.New()
	s.bibles.AsControl().SetThemeTypeVariation(vLibraryTree)
	s.bibles.SetHideRoot(true)
	s.bibles.OnItemSelected(func() {
		if i := treeIndex(s.bibles.GetSelected()); i >= 0 {
			s.bible = i
			if s.OnBibleChanged != nil {
				s.OnBibleChanged(i)
			}
		}
	})
	onThemeReady(s.bibles.AsControl(), func() { s.builtBibles = false; s.render() })

	// The suggestions overlay the translations list, right under the field.
	s.suggest = newSuggestionList()
	s.suggest.OnPick = func(i int) {
		if s.OnSuggestionPick != nil {
			s.OnSuggestionPick(i)
		}
	}
	stack := Control.New()
	s.bibles.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	add(stack, s.bibles.AsControl(), s.suggest.AsControl())
	left := libraryPane(s.locator.root.AsControl(), stack, libraryFooter())

	s.verses = Tree.New()
	s.verses.SetHideRoot(true)
	s.verses.SetSelectMode(Tree.SelectMulti)
	s.verses.SetAllowSearch(false) // typed letters must not move the selection behind the app's back
	// Clicks and the navigation keys are the app's (verseInput); the Tree
	// only draws the projected selection and scrolls.
	s.verses.AsControl().OnGuiInput(s.verseInput)
	s.initDrag()
	s.collapse = newTicker(s.pollCollapse)
	s.verses.AsNode().AddChild(s.collapse.AsNode())
	s.menu = PopupMenu.New()
	s.menu.MoreArgs().AddItem("Add to Schedule", scriptMenuAddToSchedule, 0)
	s.menu.MoreArgs().AddItem("Go Live", scriptMenuGoLive, 0)
	s.menu.OnIdPressed(s.menuPressed)
	s.verses.AsNode().AddChild(s.menu.AsNode())
	s.gearMenu = PopupMenu.New()
	s.gearMenu.MoreArgs().AddItem("Add to Schedule", scriptMenuAddToSchedule, 0)
	s.gearMenu.MoreArgs().AddItem("Go Live", scriptMenuGoLive, 0)
	s.gearMenu.AddSeparator()
	s.gearMenu.MoreArgs().AddItem("Import Bible File…", scriptMenuImportFile, 0)
	s.gearMenu.MoreArgs().AddItem("Import Copyrighted Bibles from EasyWorship…", scriptMenuImportCopyrighted, 0)
	s.gearMenu.OnIdPressed(s.menuPressed)

	// Footer: [⚙▾]   status   [▤|▾]
	s.status = label("", vCaption)
	s.status.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	s.status.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	gear := newMiniButton(miniDropdown, "gear", 40, 24, "Scripture Actions")
	gear.AsNode().AddChild(s.gearMenu.AsNode())
	gear.AsBaseButton().OnPressed(func() { popupBelow(s.gearMenu, gear.AsControl()) })
	footer := panel(vMiniBarTL)
	footer.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	row := hbox(0)
	l, r := viewSizer("view_details")
	add(row, gear.AsControl(), s.status.AsControl(), l.AsControl(), r.AsControl())
	add(footer, row.AsControl())
	right := itemsPane(s.verses.AsControl(), footer.AsControl())

	s.split.SetPanes(left, right)
	s.render()
	return s
}

// referenceKey runs the suggestion keys typed into the reference field while
// the dropdown is open. The gui_input signal comes before the LineEdit's own
// handling, so an accepted Enter is not also a text_submitted and Tab does
// not move the focus.
func (s *ScripturePanel) referenceKey(edit Control.Instance, ev InputEvent.Instance) {
	k, ok := Object.As[InputEventKey.Instance](ev)
	if !ok || !k.AsInputEvent().IsPressed() || len(s.suggestions) == 0 {
		return
	}
	mods := k.AsInputEventWithModifiers()
	if mods.AltPressed() || mods.IsCommandOrControlPressed() {
		return
	}
	switch k.Keycode() {
	case Input.KeyDown:
		if s.OnSuggestionMove != nil {
			s.OnSuggestionMove(1)
		}
	case Input.KeyUp:
		if s.OnSuggestionMove != nil {
			s.OnSuggestionMove(-1)
		}
	case Input.KeyEnter, Input.KeyKpEnter, Input.KeyTab:
		if mods.ShiftPressed() && k.Keycode() == Input.KeyTab {
			return // Shift+Tab still moves the focus back
		}
		if s.OnSuggestionPick != nil && !k.AsInputEvent().IsEcho() {
			s.OnSuggestionPick(max(s.suggestSel, 0))
		}
	case Input.KeyEscape:
		if s.OnSuggestionDismiss != nil {
			s.OnSuggestionDismiss()
		}
	default:
		return
	}
	acceptEvent(edit)
}

// verseInput turns clicks and navigation keys on the verse list into intents.
// Everything it handles is accepted, so the Tree never changes its selection
// on its own.
func (s *ScripturePanel) verseInput(ev InputEvent.Instance) {
	c := s.verses.AsControl()
	if mb, ok := Object.As[InputEventMouseButton.Instance](ev); ok {
		button := mb.ButtonIndex()
		if button != Input.MouseButtonLeft && button != Input.MouseButtonRight {
			return // the wheel scrolls
		}
		if mb.AsInputEvent().IsPressed() {
			s.verseClick(mb, button)
		}
		acceptEvent(c)
		return
	}
	k, ok := Object.As[InputEventKey.Instance](ev)
	if !ok || !k.AsInputEvent().IsPressed() {
		return
	}
	mods := k.AsInputEventWithModifiers()
	shift, cmd := mods.ShiftPressed(), mods.IsCommandOrControlPressed()
	if mods.AltPressed() {
		return
	}
	const far = 1 << 20
	step := func(delta int) {
		s.pendingRow = -1
		if s.OnVerseStep != nil {
			s.OnVerseStep(delta, shift)
		}
	}
	switch code := k.Keycode(); {
	case cmd && code == Input.KeyA:
		if s.OnSelectAll != nil && !k.AsInputEvent().IsEcho() {
			s.OnSelectAll()
		}
	case cmd:
		return
	case code == Input.KeyDown:
		step(1)
	case code == Input.KeyUp:
		step(-1)
	case code == Input.KeyHome:
		step(-far)
	case code == Input.KeyEnd:
		step(far)
	case code == Input.KeyEnter || code == Input.KeyKpEnter:
		if s.OnActivate != nil && !k.AsInputEvent().IsEcho() {
			s.OnActivate(-1)
		}
	default:
		return
	}
	acceptEvent(c)
}

func (s *ScripturePanel) verseClick(mb InputEventMouseButton.Instance, button Input.MouseButton) {
	row := treeIndex(s.verses.GetItemAtPosition(mb.AsInputEventMouse().Position()))
	s.verses.AsControl().GrabFocus()
	if row < 0 {
		return
	}
	selected := slices.Contains(s.selRows, row)
	if button == Input.MouseButtonRight {
		s.pendingRow = -1
		if !selected && s.OnVerseClick != nil {
			s.OnVerseClick(row, false, false)
		}
		popupAtMouse(s.menu)
		return
	}
	if mb.DoubleClick() {
		s.pendingRow = -1
		if s.OnActivate != nil {
			s.OnActivate(row)
		}
		return
	}
	mods := mb.AsInputEventWithModifiers()
	extend, toggle := mods.ShiftPressed(), mods.IsCommandOrControlPressed()
	if !extend && !toggle && selected && len(s.selRows) > 1 {
		// Narrow the selection only if no double-click follows.
		s.pendingRow, s.pendingAt = row, time.Now().Add(verseCollapseDelay)
		s.collapse.AsNode().SetProcess(true)
		return
	}
	s.pendingRow = -1
	if s.OnVerseClick != nil {
		s.OnVerseClick(row, extend, toggle)
	}
}

func (s *ScripturePanel) pollCollapse() {
	if s.pendingRow < 0 {
		s.collapse.AsNode().SetProcess(false)
		return
	}
	if time.Now().Before(s.pendingAt) {
		return
	}
	row := s.pendingRow
	s.pendingRow = -1
	s.collapse.AsNode().SetProcess(false)
	if s.OnVerseClick != nil {
		s.OnVerseClick(row, false, false)
	}
}

// AsControl returns the panel's root control.
func (s *ScripturePanel) AsControl() Control.Instance { return s.split.AsControl() }

// Splitter returns the translations | verses splitter.
func (s *ScripturePanel) Splitter() *Splitter { return s.split }

// SetBibles sets the installed translations and the selected one.
func (s *ScripturePanel) SetBibles(bibles []models.Bible, selected int) {
	s.bibleList = append([]models.Bible(nil), bibles...)
	s.bible = selected
	s.builtBibles = false
	s.render()
}

// SetReferenceText sets the reference field without firing OnReference.
func (s *ScripturePanel) SetReferenceText(text string) { s.locator.setText(text) }

// SetVerses sets the verse rows ("16  For God so loved…"). The selection is
// projected separately (SetVerseSelection).
func (s *ScripturePanel) SetVerses(rows []string) {
	s.verseRows = append([]string(nil), rows...)
	s.builtVerses = false
	s.pendingRow = -1
	s.render()
}

// SetVerseSelection projects the selected verse rows (ascending) and the
// cursor row. scroll brings the cursor into view (center: in the middle of
// the list, as after a lookup).
func (s *ScripturePanel) SetVerseSelection(rows []int, cursor int, scroll, center bool) {
	s.selRows = append(s.selRows[:0], rows...)
	s.cursor = cursor
	if scroll {
		s.scrollTo, s.center = cursor, center
	}
	s.render()
}

// SetSuggestions projects the reference field's book suggestions and the
// highlighted one; none hides the dropdown.
func (s *ScripturePanel) SetSuggestions(names []string, highlighted int) {
	s.suggestions = append(s.suggestions[:0], names...)
	s.suggestSel = highlighted
	s.suggest.set(s.suggestions, highlighted)
}

// VerseRowCenter is verse row i's centre in canvas coordinates, when it is
// scrolled into the visible part of the list (for the click hooks).
func (s *ScripturePanel) VerseRowCenter(i int) (Vector2.XY, bool) {
	root := s.verses.GetRoot()
	if root == (TreeItem.Instance{}) || i < 0 || i >= root.GetChildCount() {
		return Vector2.XY{}, false
	}
	r := s.verses.GetItemAreaRect(root.GetChild(i))
	c := s.verses.AsControl()
	y := r.Position.Y + r.Size.Y/2
	if y < 0 || y > c.Size().Y {
		return Vector2.XY{}, false
	}
	g := c.GetGlobalRect().Position
	return Vector2.New(g.X+c.Size().X/2, g.Y+y), true
}

// ScrollVerseIntoView scrolls the verse list so row i is in the middle (the
// click hooks do this before clicking, as an operator scrolls first).
func (s *ScripturePanel) ScrollVerseIntoView(i int) {
	root := s.verses.GetRoot()
	if root == (TreeItem.Instance{}) || i < 0 || i >= root.GetChildCount() {
		return
	}
	// Centred: a row scrolled just into view can still have its middle under
	// the list's bottom edge. Twice: scroll_to_item refreshes the Tree's
	// cached scroll offset (what get_item_area_rect uses) before it scrolls,
	// and the cache otherwise waits for the next draw, which low-processor
	// mode may not run before the click lands.
	it := root.GetChild(i)
	s.verses.MoreArgs().ScrollToItem(it, true)
	s.verses.MoreArgs().ScrollToItem(it, true)
}

// SuggestionCenter is suggestion i's centre in canvas coordinates, while
// the dropdown shows it.
func (s *ScripturePanel) SuggestionCenter(i int) (Vector2.XY, bool) {
	c := s.suggest.AsControl()
	if !c.AsCanvasItem().IsVisibleInTree() {
		return Vector2.XY{}, false
	}
	p, ok := s.suggest.rowCenter(i)
	if !ok {
		return Vector2.XY{}, false
	}
	g := c.GetGlobalRect().Position
	return Vector2.New(g.X+p.X, g.Y+p.Y), true
}

// SetStatus shows a status or error message in the footer.
func (s *ScripturePanel) SetStatus(msg string, isError bool) {
	s.statusText, s.statusError = msg, isError
	s.render()
}

// SetImportCopyrightedAvailable enables the gear menu's import of
// EasyWorship's copyrighted Bibles (there are some that are not installed).
func (s *ScripturePanel) SetImportCopyrightedAvailable(ok bool) {
	s.importAvail = ok
	s.render()
}

func (s *ScripturePanel) menuPressed(id int) {
	switch {
	case id == scriptMenuAddToSchedule && s.OnAddToSchedule != nil:
		s.OnAddToSchedule()
	case id == scriptMenuGoLive && s.OnGoLive != nil:
		s.OnGoLive()
	case id == scriptMenuImportCopyrighted && s.OnImportCopyrighted != nil:
		s.OnImportCopyrighted()
	case id == scriptMenuImportFile && s.OnImportFile != nil:
		s.OnImportFile()
	}
}

// render projects the panel state. Its programmatic Tree changes run quietly
// (signals blocked), so they never come back as OnBibleChanged / OnSelect.
func (s *ScripturePanel) render() {
	quietly(s.bibles.AsControl(), func() {
		if !s.builtBibles {
			s.builtBibles = true
			s.bibles.Clear()
			root := s.bibles.CreateItem()
			icon := fwIcon(s.bibles.AsControl(), "bible")
			for _, b := range s.bibleList {
				it := root.CreateChild()
				name := b.Abbrev
				if b.Name != "" {
					name = b.Name + " (" + b.Abbrev + ")"
				}
				it.SetText(0, name)
				it.SetIcon(0, icon)
				it.SetIconMaxWidth(0, 16)
			}
		}
		selectTreeRow(s.bibles, s.bible)
	})
	quietly(s.verses.AsControl(), func() {
		if !s.builtVerses {
			s.builtVerses = true
			s.verses.Clear()
			root := s.verses.CreateItem()
			for _, v := range s.verseRows {
				root.CreateChild().SetText(0, v)
			}
		}
		s.projectVerseSelection()
	})
	s.gearMenu.SetItemDisabled(s.gearMenu.GetItemIndex(scriptMenuImportCopyrighted), !s.importAvail)
	s.status.SetText(s.statusText)
	if s.statusError {
		s.status.AsControl().SetThemeTypeVariation(vError)
	} else {
		s.status.AsControl().SetThemeTypeVariation(vCaption)
	}
}

// projectVerseSelection selects the projected rows in the Tree (multi
// mode: TreeItem.Select adds to the selection) with the cursor on the
// cursor row, and scrolls a requested row into view.
func (s *ScripturePanel) projectVerseSelection() {
	t := s.verses
	defer t.AsCanvasItem().QueueRedraw()
	t.DeselectAll()
	root := t.GetRoot()
	if root == (TreeItem.Instance{}) {
		return
	}
	n := root.GetChildCount()
	for _, i := range s.selRows {
		if i >= 0 && i < n && i != s.cursor {
			root.GetChild(i).MoreArgs().Select(0, false)
		}
	}
	if s.cursor >= 0 && s.cursor < n && slices.Contains(s.selRows, s.cursor) {
		root.GetChild(s.cursor).MoreArgs().Select(0, true)
	}
	if i := s.scrollTo; i >= 0 && i < n {
		t.MoreArgs().ScrollToItem(root.GetChild(i), s.center)
	}
	s.scrollTo = -1
}
