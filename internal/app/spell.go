package app

import (
	"context"
	"errors"
	"log"
	"strings"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/keepalive"
	"freebeams/internal/render"
	"freebeams/internal/spell"
)

// Spell checking in the song editor. The engine (internal/spell) checks on a
// goroutine: the editor reports its lyrics (at once when it opens, then 250 ms
// after typing stops), the newest check's answer becomes spellState.flags,
// and render projects them as underlines. Personal words, Check Spelling as
// I Type and the language are settings (core.SettingSpell…).

// spellState is the app's spelling state (main thread). The engine is used
// from goroutines (its maps behind a mutex), so it stays out of graphics.gd's
// keepalive walk.
type spellState struct {
	engine keepalive.Hidden[spell.Engine]
	live   bool
	lang   string
	langs  []string
	gen    core.Generation
	flags  []spell.Misspelling // the newest check's answer
	ver    int                 // bumped when flags or the options change
	shown  int                 // the ver render last projected
	checks int                 // checks in flight
}

func (a *App) speller() *spell.Engine { return a.sp.engine.Get() }

// initSpell loads the settings, starts the engine and wires the editor.
func (a *App) initSpell() {
	var personal []string
	a.sp.live = true
	if st := a.st(); st != nil {
		ctx := a.ctx()
		if _, err := st.GetSetting(ctx, core.SettingSpellPersonal, &personal); err != nil {
			a.fail("read setting "+core.SettingSpellPersonal, err)
		}
		if _, err := st.GetSetting(ctx, core.SettingSpellLive, &a.sp.live); err != nil {
			a.fail("read setting "+core.SettingSpellLive, err)
		}
		if _, err := st.GetSetting(ctx, core.SettingSpellLanguage, &a.sp.lang); err != nil {
			a.fail("read setting "+core.SettingSpellLanguage, err)
		}
	}
	e := spell.Default(personal)
	a.sp.engine = keepalive.Hide(e)
	a.sp.langs = e.Languages()
	a.sp.ver++
	log.Printf("[freebeams] spelling: %s, %d language(s), %d personal word(s)", e.BackendName(), len(a.sp.langs), len(personal))

	ed := a.win.Editor
	ed.Spell.OnCheck = func(text string) { a.event(func() { a.checkSpelling(text, nil) }) }
	ed.Spell.CheckNow = func(text string) []spell.Misspelling { return a.speller().Check(text, a.sp.lang) }
	ed.Spell.Suggest = func(word string) []string { return a.speller().Suggest(word, a.sp.lang) }
	ed.Spell.OnLearn = func(word string) { a.event(func() { a.learnWord(word) }) }
	ed.Spell.OnIgnore = func(word string) { a.event(func() { a.ignoreWord(word) }) }
	ed.Spell.OnLive = func(on bool) { a.event(func() { a.setSpellLive(on) }) }
	ed.Spell.OnLanguage = func(lang string) { a.event(func() { a.setSpellLanguage(lang) }) }
}

// checkSpelling checks text on a goroutine; the newest check's answer
// becomes the editor's underlines. cmd, when not nil, is answered with the
// state once it is applied.
func (a *App) checkSpelling(text string, cmd *control.Command) {
	e := a.speller()
	if e == nil || !a.sp.live {
		a.answer(cmd, nil)
		return
	}
	ticket := a.sp.gen.Next()
	lang := a.sp.lang
	a.sp.checks++
	a.async(cmd, func(context.Context) func() error {
		flags := e.Check(text, lang)
		return func() error {
			a.sp.checks--
			if a.sp.gen.Current(ticket) {
				a.sp.flags = flags
				a.sp.ver++
			}
			return nil
		}
	})
}

// recheckSpelling checks the editor's lyrics again (the dictionary changed).
func (a *App) recheckSpelling(cmd *control.Command) {
	if !a.win.Editor.Visible() {
		a.answer(cmd, nil)
		return
	}
	a.checkSpelling(a.win.Editor.Lyrics(), cmd)
}

// learnWord adds word to the personal dictionary and saves it.
func (a *App) learnWord(word string) error {
	word = strings.TrimSpace(word)
	if word == "" {
		return errors.New("spell-learn: no word")
	}
	a.speller().Learn(word)
	a.saveSpellSetting(core.SettingSpellPersonal, a.speller().Personal())
	a.recheckSpelling(nil)
	return nil
}

// forgetWord removes word from the personal dictionary.
func (a *App) forgetWord(word string) {
	a.speller().Forget(word)
	a.saveSpellSetting(core.SettingSpellPersonal, a.speller().Personal())
	a.recheckSpelling(nil)
}

// ignoreWord accepts word until the app quits.
func (a *App) ignoreWord(word string) {
	a.speller().Ignore(word)
	a.recheckSpelling(nil)
}

func (a *App) setSpellLive(on bool) {
	a.sp.live = on
	a.sp.ver++
	if !on {
		a.sp.gen.Next() // drop the checks in flight
		a.sp.flags = nil
	}
	a.saveSpellSetting(core.SettingSpellLive, on)
	if on {
		a.recheckSpelling(nil)
	}
}

func (a *App) setSpellLanguage(lang string) {
	if len(a.sp.langs) > 0 && lang == a.sp.langs[0] {
		lang = "" // the checker's default
	}
	a.sp.lang = lang
	a.sp.ver++
	a.saveSpellSetting(core.SettingSpellLanguage, lang)
	a.recheckSpelling(nil)
}

func (a *App) saveSpellSetting(key string, v any) {
	if st := a.st(); st != nil {
		if err := st.SetSetting(a.ctx(), key, v); err != nil {
			a.fail("save setting "+key, err)
		}
	}
}

// renderSpell projects the flags and options onto the editor.
func (a *App) renderSpell() {
	if a.sp.ver == a.sp.shown || a.speller() == nil {
		return
	}
	a.sp.shown = a.sp.ver
	a.win.Editor.SetSpellOptions(a.sp.live, a.sp.langs, a.sp.lang)
	a.win.Editor.SetSpelling(a.sp.flags)
}

// spellBusy reports a check in flight or waiting for the typing debounce.
func (a *App) spellBusy() bool { return a.sp.checks > 0 || a.win.Editor.SpellPending() }

// spellStateDoc is dialog.spell in GET /state (nil while the editor is closed).
func (a *App) spellStateDoc() *core.SpellState {
	e := a.speller()
	if e == nil || !a.win.Editor.Visible() {
		return nil
	}
	ed := a.win.Editor
	doc := &core.SpellState{Engine: e.BackendName(), Language: a.sp.lang, Languages: a.sp.langs,
		Live: a.sp.live, Personal: e.Personal(), Flagged: ed.FlaggedWords(), Checking: a.spellBusy()}
	if ed.SpellMenuOpen() {
		doc.Menu = ed.SpellMenuEntries()
	}
	if d := ed.SpellingDialog(); d.Visible() {
		doc.Dialog = &core.SpellDialogState{Word: d.Word(), Suggestions: d.Suggestions(),
			Replacement: d.Replacement(), Done: d.Done(), Changes: d.Changes()}
	}
	return doc
}

// handleSpellCommand runs the spelling actions; false when the action is
// not one of them.
//
//	spell-check {text}          the misspellings of text (no editor needed)
//	spell-suggest {text}        suggestions for the word text
//	spell-learn {text}          Add to Dictionary (persisted)
//	spell-forget {text}         remove a word from the personal dictionary
//	spell-ignore {text}         Ignore All for this session
//	spell-live {on}             Check Spelling as I Type
//	spell-language {name}       the checker's language ("" is its default)
//	editor-lyrics {lyrics}      type lyrics into the open song editor
//	spell-menu {index}          right-click flagged word index (0-based)
//	spell-menu-pick {item}      choose a context menu entry by its label
//	spell-dialog {mode, text}   the Spelling dialog: open, change, change-all,
//	                            ignore, ignore-all, add, close; text sets the
//	                            change-to field first
func (a *App) handleSpellCommand(cmd *control.Command, args actionArgs) bool {
	if !strings.HasPrefix(cmd.Action, "spell-") && cmd.Action != "editor-lyrics" {
		return false
	}
	e := a.speller()
	if e == nil {
		cmd.RespondErr(errors.New(cmd.Action + ": the spelling checker is not available"))
		return true
	}
	ed := a.win.Editor
	needEditor := func() bool {
		if !ed.Visible() {
			cmd.RespondErr(errors.New(cmd.Action + ": the song editor is not open"))
			return false
		}
		return true
	}
	switch cmd.Action {
	case "spell-check":
		text, lang := args.Text, a.sp.lang
		a.async(nil, func(context.Context) func() error {
			res := e.Check(text, lang)
			return func() error {
				if res == nil {
					res = []spell.Misspelling{}
				}
				cmd.RespondJSON(map[string]any{"engine": e.BackendName(), "misspellings": res})
				return nil
			}
		})
	case "spell-suggest":
		cmd.RespondJSON(map[string]any{"suggestions": e.Suggest(args.Text, a.sp.lang)})
	case "spell-learn":
		if err := a.learnWord(args.Text); err != nil {
			cmd.RespondErr(err)
			return true
		}
		a.answerAfterSpell(cmd)
	case "spell-forget":
		a.forgetWord(args.Text)
		a.answerAfterSpell(cmd)
	case "spell-ignore":
		a.ignoreWord(args.Text)
		a.answerAfterSpell(cmd)
	case "spell-live":
		if args.On == nil {
			cmd.RespondErr(errArgs)
			return true
		}
		a.setSpellLive(*args.On)
		a.answerAfterSpell(cmd)
	case "spell-language":
		a.setSpellLanguage(args.Name)
		a.answerAfterSpell(cmd)
	case "editor-lyrics":
		if !needEditor() {
			return true
		}
		ed.SetLyrics(args.Lyrics) // reports the text: checkSpelling queues the check
		a.answerAfterSpell(cmd)
	case "spell-menu":
		if !needEditor() {
			return true
		}
		if args.Index == nil || !ed.OpenSpellMenu(*args.Index) {
			cmd.RespondErr(errors.New("spell-menu: no such flagged word on screen"))
			return true
		}
		a.answer(cmd, nil)
	case "spell-menu-pick":
		if !needEditor() {
			return true
		}
		if !ed.PickSpellMenu(args.Item) {
			cmd.RespondErr(errors.New("spell-menu-pick: no enabled entry " + args.Item))
			return true
		}
		a.answerAfterSpell(cmd)
	case "spell-dialog":
		if !needEditor() {
			return true
		}
		d := ed.SpellingDialog()
		if args.Mode == "open" {
			ed.OpenSpelling()
			a.answer(cmd, nil)
			return true
		}
		if !d.Visible() {
			cmd.RespondErr(errors.New("spell-dialog: the Spelling dialog is not open"))
			return true
		}
		if args.Text != "" {
			d.SetReplacement(args.Text)
		}
		if !d.Do(args.Mode) {
			cmd.RespondErr(errors.New("spell-dialog: " + args.Mode + " is not available now"))
			return true
		}
		a.answerAfterSpell(cmd)
	default:
		cmd.RespondErr(errors.New("unknown action " + cmd.Action))
	}
	return true
}

// answerAfterSpell answers cmd once the spelling checks queued so far are
// applied (wait-idle does the same for everything).
func (a *App) answerAfterSpell(cmd *control.Command) {
	a.waiters = append(a.waiters, cmd)
}

// spellScreenshot captures the Spelling dialog or the lyrics' context menu.
func (a *App) spellScreenshot(window string) ([]byte, bool, error) {
	switch window {
	case "spelling":
		png, err := render.WindowScreenshot(a.win.Editor.SpellingDialog().AsWindow())
		return png, true, err
	case "spell-menu":
		png, err := render.WindowScreenshot(a.win.Editor.SpellMenuWindow())
		return png, true, err
	}
	return nil, false, nil
}
