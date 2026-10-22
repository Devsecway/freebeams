package ui

import (
	"math"
	"strings"

	"freebeams/internal/spell"

	"graphics.gd/classdb/Button"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventMouseButton"
	"graphics.gd/classdb/ItemList"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/TextEdit"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Timer"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Rect2i"
	"graphics.gd/variant/Transform2D"
	"graphics.gd/variant/Vector2"
	"graphics.gd/variant/Vector2i"
)

// Spell checking in the song editor (EasyWorship's Presentation Editor:
// the SpellCheck mini button with its dropdown, live underlines refreshed by
// a 250 ms timer, and Form_SpellCheckDialog "Spelling").
//
// The editor is a dumb view. It reports the lyrics to check (SongSpell.OnCheck,
// debounced while typing) and the operator's Ignore / Add to Dictionary /
// language choices; the app answers with SetSpelling (the flagged words of
// the text it checked) and SetSpellOptions. The right-click suggestions and
// the Spelling dialog's walk need answers at once: SongSpell.Suggest and
// SongSpell.CheckNow are injected funcs, like the thumbnail factories.
//
// The red wavy underline is drawn over the TextEdit from its draw signal
// (after TextEdit's own drawing): get_rect_at_line_column gives each flagged
// word's glyph boxes in the TextEdit's coordinates for the lines on screen.
// A flag is drawn only while the text under it still is its word, so an edit
// before the next check never shows an underline on the wrong letters.

// SongSpell connects the song editor's spelling checks to the app.
type SongSpell struct {
	OnCheck    func(text string)                     // check the lyrics (debounced while typing, at once on open)
	CheckNow   func(text string) []spell.Misspelling // the Spelling dialog's synchronous check
	Suggest    func(word string) []string            // right-click and dialog suggestions
	OnLearn    func(word string)                     // Add to Dictionary
	OnIgnore   func(word string)                     // Ignore / Ignore All (this session)
	OnLive     func(on bool)                         // Check Spelling as I Type
	OnLanguage func(lang string)                     // the dropdown's Language ▸ choice
}

// spellDebounce is EasyWorship's TimerOutlineLiveSpellUpdate interval.
const spellDebounce = 0.25

// Context menu ids: TextEdit's own MenuItems for the edit entries, these for
// ours.
const (
	spellMenuIgnore  = 90
	spellMenuLearn   = 91
	spellMenuSuggest = 100 // + the suggestion's index
)

// Dropdown ids.
const (
	spellDropCheck = 0
	spellDropLive  = 1
	spellDropLang  = 2
)

// editorSpell is the editor's spelling state (main thread only).
type editorSpell struct {
	timer    Timer.Instance
	flags    []spell.Misspelling // the app's last answer
	live     bool
	langs    []string
	lang     string
	menu     PopupMenu.Instance // the lyrics' context menu
	ctx      spell.Misspelling  // the flagged word the menu was opened on
	ctxOK    bool
	ctxSugg  []string
	drop     PopupMenu.Instance // the SpellCheck ▾ dropdown
	langMenu PopupMenu.Instance
	dlg      *SpellingDialog
}

// buildSpell adds the Lyrics header row (title, SpellCheck split button) and
// wires the lyrics TextEdit. It returns the header row.
func (e *SongEditor) buildSpell(title Label.Instance) Control.Instance {
	sp := &e.sp
	sp.live = true
	row := hbox(0)
	title.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	check := newMiniButton(miniSplitLeft, "spellcheck", 24, 24, "Check Spelling")
	check.AsNode().SetName("SpellCheckButton")
	check.AsBaseButton().OnPressed(func() { e.OpenSpelling() })
	dd := newMiniButton(miniSplitRight, "", 16, 24, "Spelling Options")
	dd.AsNode().SetName("SpellDropdownButton")
	add(row, title.AsControl(), check.AsControl(), dd.AsControl())

	sp.drop = PopupMenu.New()
	sp.drop.AsNode().SetName("SpellDropdown")
	sp.langMenu = PopupMenu.New()
	sp.langMenu.AsNode().SetName("SpellLanguageMenu")
	sp.drop.AsNode().AddChild(sp.langMenu.AsNode())
	dd.AsNode().AddChild(sp.drop.AsNode())
	sp.drop.OnIdPressed(func(id int) {
		switch id {
		case spellDropCheck:
			e.OpenSpelling()
		case spellDropLive:
			if f := e.Spell.OnLive; f != nil {
				f(!sp.live)
			}
		}
	})
	sp.langMenu.OnIdPressed(func(id int) {
		if f := e.Spell.OnLanguage; f != nil && id >= 0 && id < len(sp.langs) {
			f(sp.langs[id])
		}
	})
	dd.AsBaseButton().OnPressed(func() {
		e.projectSpellDropdown()
		popupBelow(sp.drop, dd.AsControl())
	})

	sp.timer = Timer.New()
	sp.timer.SetOneShot(true)
	sp.timer.SetWaitTime(spellDebounce)
	sp.timer.OnTimeout(func() { e.requestSpell() })
	e.dlg.win.AsNode().AddChild(sp.timer.AsNode())

	ly := e.lyrics
	ly.SetContextMenuEnabled(false) // ours, with the suggestions (showSpellMenu)
	ly.OnTextChanged(func() {
		if sp.live {
			sp.timer.Start()
		}
	})
	ly.AsCanvasItem().OnDraw(func() {
		e.drawSpelling()
		_ = ly.AsCanvasItem().Visible() // flush the buffered draw calls inside the draw pass
	})
	ly.AsControl().OnGuiInput(e.lyricsInput)
	sp.menu = PopupMenu.New()
	sp.menu.AsNode().SetName("SpellMenu")
	ly.AsNode().AddChild(sp.menu.AsNode())
	sp.menu.OnIdPressed(e.spellMenuPicked)

	sp.dlg = newSpellingDialog(e)
	return row.AsControl()
}

// requestSpell asks the app to check the lyrics as they are now.
func (e *SongEditor) requestSpell() {
	e.sp.timer.Stop()
	if f := e.Spell.OnCheck; f != nil && e.sp.live {
		f(e.lyrics.Text())
	}
}

// SetSpelling projects the app's answer: the flagged words of the lyrics it
// checked (text is that text; flags no longer under their word are not drawn).
func (e *SongEditor) SetSpelling(flags []spell.Misspelling) {
	e.sp.flags = append(e.sp.flags[:0], flags...)
	e.lyrics.AsCanvasItem().QueueRedraw()
}

// SetSpellOptions projects Check Spelling as I Type and the languages (the
// first of langs is the default; lang "" picks it).
func (e *SongEditor) SetSpellOptions(live bool, langs []string, lang string) {
	sp := &e.sp
	if live && !sp.live && e.Visible() {
		sp.timer.Start()
	}
	sp.live, sp.langs, sp.lang = live, append([]string(nil), langs...), lang
	if !live {
		sp.flags = nil
		e.lyrics.AsCanvasItem().QueueRedraw()
	}
}

// SpellPending reports a debounced check waiting for its timer.
func (e *SongEditor) SpellPending() bool {
	return e.sp.timer != Timer.Nil && e.Visible() && !e.sp.timer.IsStopped()
}

// SetLyrics replaces the lyrics as typing would (automation): the check runs
// at once instead of after the debounce.
func (e *SongEditor) SetLyrics(text string) {
	e.lyrics.SetText(text)
	e.requestSpell()
}

// Lyrics returns the lyrics text.
func (e *SongEditor) Lyrics() string { return e.lyrics.Text() }

// visibleFlags are the flags whose word is still in the lyrics where the
// check found it.
func (e *SongEditor) visibleFlags() []spell.Misspelling {
	var out []spell.Misspelling
	n := e.lyrics.GetLineCount()
	for _, f := range e.sp.flags {
		if f.Line < n && flagMatches(e.lyrics.GetLine(f.Line), f) {
			out = append(out, f)
		}
	}
	return out
}

// flagMatches reports whether line still holds f's word at f.Col.
func flagMatches(line string, f spell.Misspelling) bool {
	rs := []rune(line)
	return f.Col >= 0 && f.Col+f.Len <= len(rs) && string(rs[f.Col:f.Col+f.Len]) == f.Word
}

// FlaggedWords lists the underlined words, in text order.
func (e *SongEditor) FlaggedWords() []string {
	out := []string{}
	for _, f := range e.visibleFlags() {
		out = append(out, f.Word)
	}
	return out
}

// spellRed is the underline colour (macOS's misspelling red).
var spellRed = Color.RGBA{R: 1, G: 0.23, B: 0.19, A: 1}

// drawSpelling draws a red wavy underline under each flagged word on screen.
func (e *SongEditor) drawSpelling() {
	ly := e.lyrics
	ci := ly.AsCanvasItem()
	for _, f := range e.visibleFlags() {
		// The rect at column c is the glyph before the caret position c
		// (measured: column c's rect covers character c-1).
		r0 := ly.GetRectAtLineColumn(f.Line, f.Col+1)
		r1 := ly.GetRectAtLineColumn(f.Line, f.Col+f.Len)
		if r0.Position.X < 0 || r1.Position.X < 0 || r0.Size.Y <= 0 {
			continue // not on screen
		}
		x0 := float32(r0.Position.X)
		x1 := float32(r1.Position.X + r1.Size.X)
		y := float32(r0.Position.Y+r0.Size.Y) - 2.5
		ci.MoreArgs().DrawPolyline(wavePoints(x0, x1, y), spellRed, 1.2, true)
	}
}

// wavePoints is a zigzag from x0 to x1 around y (2 px period, 1.5 px high).
func wavePoints(x0, x1, y float32) []Vector2.XY {
	const step, amp = 2, 0.75
	n := int(math.Max(1, math.Ceil(float64((x1-x0)/step))))
	pts := make([]Vector2.XY, 0, n+1)
	for i := 0; i <= n; i++ {
		x := min(x0+float32(i)*step, x1)
		dy := float32(amp)
		if i%2 == 1 {
			dy = -amp
		}
		pts = append(pts, Vector2.New(x, y+dy))
	}
	return pts
}

// lyricsInput opens the spelling context menu on a right click (the
// TextEdit's own menu is off; it still moves the caret).
func (e *SongEditor) lyricsInput(ev InputEvent.Instance) {
	mb, ok := Object.As[InputEventMouseButton.Instance](ev)
	if !ok || !mb.AsInputEvent().IsPressed() || mb.ButtonIndex() != Input.MouseButtonRight {
		return
	}
	pos := mb.AsInputEventMouse().Position()
	lc := e.lyrics.GetLineColumnAtPos(Vector2i.New(int(pos.X), int(pos.Y)))
	e.sp.ctxOK = false
	for _, f := range e.visibleFlags() {
		if f.Line == int(lc.Y) && int(lc.X) >= f.Col && int(lc.X) <= f.Col+f.Len {
			e.sp.ctx, e.sp.ctxOK = f, true
			break
		}
	}
	e.projectSpellMenu()
	popupAtMouse(e.sp.menu)
}

// OpenSpellMenu opens the context menu on flagged word i as a right click on
// it does (automation and screenshots). It reports whether there is such a
// word on screen.
func (e *SongEditor) OpenSpellMenu(i int) bool {
	flags := e.visibleFlags()
	if i < 0 || i >= len(flags) {
		return false
	}
	f := flags[i]
	r := e.lyrics.GetRectAtLineColumn(f.Line, f.Col)
	if r.Position.X < 0 {
		return false
	}
	e.sp.ctx, e.sp.ctxOK = f, true
	e.projectSpellMenu()
	st := e.lyrics.AsCanvasItem().GetScreenTransform()
	p := Transform2D.Vector(Vector2.New(float32(r.Position.X), float32(r.Position.Y+r.Size.Y)), st)
	e.sp.menu.AsWindow().ResetSize()
	e.sp.menu.AsWindow().MoreArgs().Popup(Rect2i.PositionSize{Position: Vector2i.New(int(p.X), int(p.Y))})
	return true
}

// projectSpellMenu rebuilds the context menu: the suggestions, Ignore and
// Add to Dictionary for a flagged word, then the edit entries (TextEdit's
// own ids, handled by its menu_option).
func (e *SongEditor) projectSpellMenu() {
	sp := &e.sp
	m := sp.menu
	m.Clear()
	sp.ctxSugg = nil
	if sp.ctxOK {
		if f := e.Spell.Suggest; f != nil {
			sp.ctxSugg = f(sp.ctx.Word)
		}
		if len(sp.ctxSugg) == 0 {
			m.MoreArgs().AddItem("No Guesses Found", spellMenuSuggest-1, 0)
			m.SetItemDisabled(0, true)
		}
		for k, s := range sp.ctxSugg {
			m.MoreArgs().AddItem(s, spellMenuSuggest+k, 0)
		}
		m.AddSeparator()
		m.MoreArgs().AddItem("Ignore", spellMenuIgnore, 0)
		m.MoreArgs().AddItem("Add to Dictionary", spellMenuLearn, 0)
		m.AddSeparator()
	}
	ly := e.lyrics
	items := []struct {
		label string
		id    TextEdit.MenuItems
		off   bool
	}{
		{"Undo", TextEdit.MenuUndo, !ly.HasUndo()},
		{"Redo", TextEdit.MenuRedo, !ly.HasRedo()},
		{"", -1, false},
		{"Cut", TextEdit.MenuCut, !ly.HasSelection()},
		{"Copy", TextEdit.MenuCopy, !ly.HasSelection()},
		{"Paste", TextEdit.MenuPaste, false},
		{"", -1, false},
		{"Select All", TextEdit.MenuSelectAll, false},
	}
	for _, it := range items {
		if it.id < 0 {
			m.AddSeparator()
			continue
		}
		m.MoreArgs().AddItem(it.label, int(it.id), 0)
		m.SetItemDisabled(m.ItemCount()-1, it.off)
	}
	_ = m.ItemCount() // flush
}

// SpellMenuEntries lists the context menu's entries as last projected
// (separators are "-").
func (e *SongEditor) SpellMenuEntries() []string {
	var out []string
	m := e.sp.menu
	for k := 0; k < m.ItemCount(); k++ {
		t := m.GetItemText(k)
		if t == "" {
			t = "-"
		}
		out = append(out, t)
	}
	return out
}

// SpellMenuOpen reports the context menu being on screen.
func (e *SongEditor) SpellMenuOpen() bool { return e.sp.menu.AsWindow().Visible() }

// SpellMenuWindow is the context menu's popup window (screenshots).
func (e *SongEditor) SpellMenuWindow() Window.Instance { return e.sp.menu.AsWindow() }

// PickSpellMenu chooses the context menu entry labelled label, as a click
// does, and closes the menu. It reports whether there was one.
func (e *SongEditor) PickSpellMenu(label string) bool {
	m := e.sp.menu
	for k := 0; k < m.ItemCount(); k++ {
		if m.GetItemText(k) == label && !m.IsItemDisabled(k) {
			id := m.GetItemId(k)
			m.AsWindow().Hide()
			e.spellMenuPicked(id)
			return true
		}
	}
	return false
}

func (e *SongEditor) spellMenuPicked(id int) {
	sp := &e.sp
	switch {
	case id >= spellMenuSuggest && sp.ctxOK && id-spellMenuSuggest < len(sp.ctxSugg):
		e.replaceWord(sp.ctx, sp.ctxSugg[id-spellMenuSuggest])
	case id == spellMenuIgnore && sp.ctxOK:
		if f := e.Spell.OnIgnore; f != nil {
			f(sp.ctx.Word)
		}
	case id == spellMenuLearn && sp.ctxOK:
		if f := e.Spell.OnLearn; f != nil {
			f(sp.ctx.Word)
		}
	case id >= 0 && id < spellMenuIgnore:
		e.lyrics.MenuOption(id) // Undo, Cut, Paste, …
	}
	sp.ctxOK = false
}

// replaceWord replaces flagged word f with repl as one undoable edit (only
// while the word is still there).
func (e *SongEditor) replaceWord(f spell.Misspelling, repl string) {
	ly := e.lyrics
	if f.Line >= ly.GetLineCount() || !flagMatches(ly.GetLine(f.Line), f) {
		return
	}
	ly.BeginComplexOperation()
	ly.RemoveText(f.Line, f.Col, f.Line, f.Col+f.Len)
	ly.InsertText(repl, f.Line, f.Col)
	ly.EndComplexOperation()
}

// projectSpellDropdown fills the SpellCheck ▾ menu: Check Spelling…, Check
// Spelling as I Type and the Language ▸ submenu.
func (e *SongEditor) projectSpellDropdown() {
	sp := &e.sp
	d := sp.drop
	d.Clear()
	d.MoreArgs().AddItem("Check Spelling…", spellDropCheck, 0)
	d.AddSeparator()
	d.MoreArgs().AddCheckItem("Check Spelling as I Type", spellDropLive, 0)
	d.SetItemChecked(d.ItemCount()-1, sp.live)
	sp.langMenu.Clear()
	current := sp.lang
	if current == "" && len(sp.langs) > 0 {
		current = sp.langs[0]
	}
	for k, l := range sp.langs {
		sp.langMenu.MoreArgs().AddRadioCheckItem(languageName(l), k, 0)
		sp.langMenu.SetItemChecked(k, l == current)
	}
	if len(sp.langs) > 1 {
		d.MoreArgs().AddSubmenuItem("Language", "SpellLanguageMenu", spellDropLang)
	}
	_ = d.ItemCount()
}

// languageName labels a spelling language code ("en_GB" -> "English (en_GB)").
func languageName(code string) string {
	names := map[string]string{"en": "English", "es": "Spanish", "fr": "French", "de": "German",
		"pt": "Portuguese", "it": "Italian", "nl": "Dutch", "sv": "Swedish", "da": "Danish",
		"nb": "Norwegian", "fi": "Finnish", "pl": "Polish", "ru": "Russian", "uk": "Ukrainian",
		"ko": "Korean", "tr": "Turkish", "ro": "Romanian", "cs": "Czech", "hu": "Hungarian"}
	base, _, _ := strings.Cut(code, "_")
	if n, ok := names[base]; ok {
		if base == code {
			return n
		}
		return n + " (" + code + ")"
	}
	return code
}

// ---- Spelling dialog --------------------------------------------------------

// SpellingDialog is EasyWorship's Spelling dialog (Form_SpellCheckDialog,
// 337 x 237): the suspect word in an editable field, the line it is on, the
// suggestions, and Change / Change All / Ignore / Ignore All / Add to
// Dictionary / Close. It walks the editor's lyrics with a spell.Session and
// edits them in place.
type SpellingDialog struct {
	e       *SongEditor
	dlg     *dialogWindow
	edit    LineEdit.Instance
	context Label.Instance
	list    ItemList.Instance
	buttons map[string]Button.Instance

	sess  *spell.Session
	cur   spell.Misspelling
	has   bool
	sugg  []string
	done  bool
	count int // changes made
}

// Spelling dialog buttons, top to bottom (Change All stands where
// EasyWorship has Find Next).
var spellButtons = []struct {
	key, label string
	y          float32
}{
	{"change", "Change", 8}, {"change-all", "Change All", 40}, {"ignore", "Ignore", 88},
	{"ignore-all", "Ignore All", 120}, {"add", "Add to Dictionary", 152}, {"close", "Close", 200},
}

func newSpellingDialog(e *SongEditor) *SpellingDialog {
	s := &SpellingDialog{e: e, buttons: map[string]Button.Instance{}}
	s.dlg = newDialogWindow("Spelling", 337, 237)
	s.dlg.win.AsNode().SetName("SpellingDialog")
	s.dlg.onOK = func() { s.Do("change") }
	s.dlg.onClose = func() { s.sess = nil }
	for _, b := range []Button.Instance{s.dlg.ok, s.dlg.cancel} {
		Object.Free(b) // the dialog has its own buttons, no footer
	}
	s.dlg.ok, s.dlg.cancel = Button.Nil, Button.Nil
	s.dlg.footer.AsCanvasItem().SetVisible(false)

	row := hbox(15)
	expand(row.AsControl())
	left := vbox(6)
	left.AsControl().SetCustomMinimumSize(Vector2.New(193, 0))
	s.edit = LineEdit.New()
	s.edit.AsControl().SetCustomMinimumSize(Vector2.New(193, 22))
	s.edit.OnTextSubmitted(func(string) { s.Do("change") })
	s.context = label("", vHint)
	s.context.SetAutowrapMode(TextServer.AutowrapWordSmart)
	s.context.SetClipText(false)
	s.context.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	s.context.AsControl().SetCustomMinimumSize(Vector2.New(193, 48))
	s.list = ItemList.New()
	expand(s.list.AsControl())
	s.list.OnItemSelected(func(i int) {
		if i >= 0 && i < len(s.sugg) {
			s.edit.SetText(s.sugg[i])
		}
	})
	s.list.OnItemActivated(func(i int) {
		if i >= 0 && i < len(s.sugg) {
			s.edit.SetText(s.sugg[i])
			s.Do("change")
		}
	})
	add(left, s.edit.AsControl(), s.context.AsControl(), s.list.AsControl())

	right := vbox(0)
	right.AsControl().SetCustomMinimumSize(Vector2.New(113, 0))
	y := float32(0)
	for _, b := range spellButtons {
		if gap := b.y - 8 - y; gap > 0 {
			add(right, fixed(0, gap))
		}
		btn := Button.New()
		btn.SetText(b.label)
		btn.AsControl().SetCustomMinimumSize(Vector2.New(113, 22))
		key := b.key
		btn.AsBaseButton().OnPressed(func() { s.Do(key) })
		s.buttons[key] = btn
		add(right, btn.AsControl())
		y = b.y - 8 + 22
	}
	add(row, left.AsControl(), right.AsControl())
	add(s.dlg.body, row.AsControl())
	return s
}

// checker adapts the editor's injected funcs to spell.Checker for the walk.
type editorChecker struct{ e *SongEditor }

func (c editorChecker) Check(text, _ string) []spell.Misspelling {
	if f := c.e.Spell.CheckNow; f != nil {
		return f(text)
	}
	return nil
}

func (c editorChecker) Suggest(word, _ string) []string {
	if f := c.e.Spell.Suggest; f != nil {
		return f(word)
	}
	return nil
}

func (c editorChecker) Learn(word string) {
	if f := c.e.Spell.OnLearn; f != nil {
		f(word)
	}
}

func (c editorChecker) Ignore(word string) {
	if f := c.e.Spell.OnIgnore; f != nil {
		f(word)
	}
}

// OpenSpelling starts the Spelling dialog at the top of the lyrics (the
// SpellCheck button and the dropdown's Check Spelling…).
func (e *SongEditor) OpenSpelling() {
	s := e.sp.dlg
	s.sess = spell.NewSession(e.lyrics.Text())
	s.count = 0
	s.next()
	s.dlg.popup()
	s.edit.AsControl().GrabFocus()
}

// SpellingDialog returns the editor's Spelling dialog.
func (e *SongEditor) SpellingDialog() *SpellingDialog { return e.sp.dlg }

// next finds the next misspelling and projects it.
func (s *SpellingDialog) next() {
	s.cur, s.has = spell.Misspelling{}, false
	s.sugg = nil
	if s.sess != nil {
		s.cur, s.has = s.sess.Next(editorChecker{s.e}, "")
	}
	s.done = !s.has
	if s.has {
		s.sugg = editorChecker{s.e}.Suggest(s.cur.Word, "")
		ly := s.e.lyrics
		ly.Select(s.cur.Line, s.cur.Col, s.cur.Line, s.cur.Col+s.cur.Len)
		ly.SetCaretLine(s.cur.Line)
		ly.CenterViewportToCaret()
	}
	s.render()
}

// render projects the walk onto the dialog.
func (s *SpellingDialog) render() {
	s.list.Clear()
	for _, w := range s.sugg {
		s.list.AddItem(w)
	}
	switch {
	case s.has:
		s.dlg.win.SetTitle("Spelling: " + s.cur.Word)
		repl := s.cur.Word
		if len(s.sugg) > 0 {
			repl = s.sugg[0]
			s.list.Select(0)
		}
		s.edit.SetText(repl)
		s.context.SetText("Not in dictionary: “" + s.cur.Word + "”\n" + strings.TrimSpace(lineOf(s.sess.Text, s.cur.Line)))
	default:
		s.dlg.win.SetTitle("Spelling")
		s.edit.SetText("")
		s.context.SetText("The spelling check is complete.")
	}
	for key, b := range s.buttons {
		b.AsBaseButton().SetDisabled(!s.has && key != "close")
	}
	_ = s.list.ItemCount()
}

func lineOf(text string, line int) string {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	return lines[line]
}

// Do presses a button: change, change-all, ignore, ignore-all, add or close.
// It reports whether the button exists and is enabled.
func (s *SpellingDialog) Do(key string) bool {
	if key == "close" {
		s.dlg.close()
		return true
	}
	if !s.has || s.sess == nil {
		return false
	}
	m := s.cur
	repl := strings.TrimSpace(s.edit.Text())
	switch key {
	case "change":
		if repl == "" {
			return false
		}
		s.sess.Change(m, repl)
		s.e.replaceWord(m, repl)
		s.count++
	case "change-all":
		if repl == "" {
			return false
		}
		var occ []spell.Token
		for _, t := range spell.Tokenize(s.sess.Text) {
			if t.Word == m.Word {
				occ = append(occ, t)
			}
		}
		s.sess.ChangeAll(m, repl)
		for i := len(occ) - 1; i >= 0; i-- {
			s.e.replaceWord(spell.Misspelling(occ[i]), repl)
		}
		s.count += len(occ)
	case "ignore":
		s.sess.Skip(m)
	case "ignore-all":
		editorChecker{s.e}.Ignore(m.Word)
	case "add":
		editorChecker{s.e}.Learn(m.Word)
	default:
		return false
	}
	s.next()
	return true
}

// SetReplacement types text into the change-to field (automation).
func (s *SpellingDialog) SetReplacement(text string) { s.edit.SetText(text) }

// Visible reports the dialog being open.
func (s *SpellingDialog) Visible() bool { return s.dlg.win.Visible() }

// Word is the misspelling on show ("" when the check is complete).
func (s *SpellingDialog) Word() string {
	if !s.has {
		return ""
	}
	return s.cur.Word
}

// Suggestions are the suggestions on show.
func (s *SpellingDialog) Suggestions() []string { return append([]string{}, s.sugg...) }

// Done reports the check having reached the end of the lyrics.
func (s *SpellingDialog) Done() bool { return s.done }

// Changes counts the words changed since the dialog opened.
func (s *SpellingDialog) Changes() int { return s.count }

// Replacement is the change-to field's text.
func (s *SpellingDialog) Replacement() string { return s.edit.Text() }

// AsWindow returns the dialog window.
func (s *SpellingDialog) AsWindow() Window.Instance { return s.dlg.win }
