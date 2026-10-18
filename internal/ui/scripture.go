package ui

import (
	"freeworship/internal/models"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/Tree"
	"graphics.gd/variant/Vector2"
)

const (
	scriptMenuAddToSchedule = iota
	scriptMenuGoLive
)

// ScripturePanel is the Scriptures resource tab: translations and the
// reference locator on the left, the verses of the passage on the right.
type ScripturePanel struct {
	OnBibleChanged  func(i int)
	OnReference     func(text string) // on Enter and debounced typing
	OnSelect        func(i int)
	OnActivate      func(i int)
	OnAddToSchedule func()
	OnGoLive        func()

	split   *Splitter
	locator *searchLocator
	bibles  Tree.Instance
	verses  Tree.Instance
	status  Label.Instance
	menu    PopupMenu.Instance

	bibleList   []models.Bible
	bible       int
	verseRows   []string
	verse       int
	statusText  string
	statusError bool
	rendering   bool
	builtBibles bool
	builtVerses bool
}

// NewScripturePanel builds the Scriptures tab.
func NewScripturePanel() *ScripturePanel {
	s := &ScripturePanel{bible: -1, verse: -1}
	s.split = NewSplitter(SplitHorizontal, 0.35, 150, 150, 0, 0)

	s.locator = newSearchLocator("Enter reference e.g. John 3:16", func(q string) {
		if s.OnReference != nil {
			s.OnReference(q)
		}
	})
	s.bibles = Tree.New()
	s.bibles.AsControl().SetThemeTypeVariation(vLibraryTree)
	s.bibles.SetHideRoot(true)
	s.bibles.OnItemSelected(func() {
		if s.rendering {
			return
		}
		if i := treeIndex(s.bibles.GetSelected()); i >= 0 {
			s.bible = i
			if s.OnBibleChanged != nil {
				s.OnBibleChanged(i)
			}
		}
	})
	s.bibles.AsControl().OnThemeChanged(func() { s.builtBibles = false; s.render() })
	left := libraryPane(s.locator.root.AsControl(), s.bibles.AsControl(), libraryFooter())

	s.verses = Tree.New()
	s.verses.SetHideRoot(true)
	s.verses.SetSelectMode(Tree.SelectRow)
	s.verses.SetAllowRmbSelect(true)
	s.verses.SetAllowReselect(true)
	s.verses.OnItemSelected(func() {
		if s.rendering {
			return
		}
		if i := treeIndex(s.verses.GetSelected()); i >= 0 {
			s.verse = i
			if s.OnSelect != nil {
				s.OnSelect(i)
			}
		}
	})
	s.verses.OnItemActivated(func() {
		if i := treeIndex(s.verses.GetSelected()); i >= 0 && s.OnActivate != nil {
			s.OnActivate(i)
		}
	})
	s.menu = PopupMenu.New()
	s.menu.MoreArgs().AddItem("Add to Schedule", scriptMenuAddToSchedule, 0)
	s.menu.MoreArgs().AddItem("Go Live", scriptMenuGoLive, 0)
	s.menu.OnIdPressed(func(id int) {
		switch {
		case id == scriptMenuAddToSchedule && s.OnAddToSchedule != nil:
			s.OnAddToSchedule()
		case id == scriptMenuGoLive && s.OnGoLive != nil:
			s.OnGoLive()
		}
	})
	s.verses.AsNode().AddChild(s.menu.AsNode())
	s.verses.OnItemMouseSelected(func(_ Vector2.XY, button int) {
		if Input.MouseButton(button) == Input.MouseButtonRight {
			popupAtMouse(s.menu)
		}
	})

	// Footer: [⚙▾]   status   [▤|▾]
	s.status = label("", vCaption)
	s.status.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	s.status.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	gear := newMiniButton(miniDropdown, "gear", 40, 24, "Scripture Actions")
	gear.AsBaseButton().OnPressed(func() { popupBelow(s.menu, gear.AsControl()) })
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

// SetVerses sets the verse rows ("16  For God so loved…") and the selection.
func (s *ScripturePanel) SetVerses(rows []string, selected int) {
	s.verseRows = append([]string(nil), rows...)
	s.verse = selected
	s.builtVerses = false
	s.render()
}

// SetSelected selects verse row i without calling OnSelect.
func (s *ScripturePanel) SetSelected(i int) {
	s.verse = i
	s.render()
}

// SetStatus shows a status or error message in the footer.
func (s *ScripturePanel) SetStatus(msg string, isError bool) {
	s.statusText, s.statusError = msg, isError
	s.render()
}

func (s *ScripturePanel) render() {
	s.rendering = true
	defer func() { s.rendering = false }()
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
	if !s.builtVerses {
		s.builtVerses = true
		s.verses.Clear()
		root := s.verses.CreateItem()
		for _, v := range s.verseRows {
			root.CreateChild().SetText(0, v)
		}
	}
	selectTreeRow(s.verses, s.verse)
	s.status.SetText(s.statusText)
	if s.statusError {
		s.status.AsControl().SetThemeTypeVariation(vError)
	} else {
		s.status.AsControl().SetThemeTypeVariation(vCaption)
	}
}
