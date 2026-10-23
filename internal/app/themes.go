package app

import (
	"context"
	"errors"
	"fmt"
	"log"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/render"
	"freebeams/internal/store"
	"freebeams/internal/ui"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/DisplayServer"
)

// The Themes tab and the theme editor: their state, event handlers and
// control actions. Authoritative state lives on the App (themeState); render
// projects it onto ui.ThemePanel, the editor's live preview and the
// schedule's Theme submenu. A saved theme is pushed into every deck that
// uses it (schedule items, Preview / Live, the library decks), so the
// monitors, thumbnails and the output follow without reloading anything.

// themeState is the Themes tab's authoritative state.
type themeState struct {
	list     []models.Theme
	ver      int // bumped when list or defaults change (rebuilds the tiles)
	sel      int // selected tile, -1 none
	selID    int64
	deck     *models.Deck // the selected theme's sample deck (resource preview)
	defaults map[models.ItemKind]int64
	loaded   bool
	editing  *models.Theme // the theme in the editor (nil when closed)
	gen      core.Generation

	editView *render.SlideView // the editor's live preview
}

// themeImageFilters are the native dialog filters for a background image.
var themeImageFilters = []string{"*.png, *.jpg, *.jpeg, *.webp, *.bmp ; Images"}

// initThemes seeds the built-in themes on a new library and loads the list.
// Called from Ready.
func (a *App) initThemes() {
	a.th.sel = -1
	a.th.defaults = map[models.ItemKind]int64{}
	if a.st() == nil {
		return
	}
	st := a.st()
	ticket := a.th.gen.Next()
	a.async(nil, func(ctx context.Context) func() error {
		wrote, err := st.EnsureDefaultThemes(ctx)
		var (
			list     []models.Theme
			defaults map[models.ItemKind]int64
			added    []string
			keep     bool
		)
		if err == nil {
			added, keep, err = seedThemePolish(ctx, st) // themepolish.go
		}
		if err == nil {
			list, defaults, err = fetchThemes(ctx, st)
		}
		return func() error {
			if err != nil {
				a.fail("load themes", err)
				return err
			}
			if wrote {
				log.Printf("[freebeams] seeded the default themes")
			}
			if len(added) > 0 {
				log.Printf("[freebeams] added the built-in themes %q", added)
			}
			a.thp.keepSchedule = keep
			if a.th.gen.Current(ticket) {
				a.applyThemes(list, defaults)
			}
			return nil
		}
	})
}

// fetchThemes lists the themes and the default-theme settings (goroutine).
func fetchThemes(ctx context.Context, st *store.Store) ([]models.Theme, map[models.ItemKind]int64, error) {
	list, err := st.ListThemes(ctx)
	if err != nil {
		return nil, nil, err
	}
	defaults := map[models.ItemKind]int64{}
	for _, kind := range store.DefaultThemeKinds {
		id, err := st.DefaultThemeID(ctx, kind)
		if err != nil {
			return nil, nil, err
		}
		defaults[kind] = id
	}
	return list, defaults, nil
}

// applyThemes takes a fresh list and defaults into the state. The selection
// follows the selected theme's id.
func (a *App) applyThemes(list []models.Theme, defaults map[models.ItemKind]int64) {
	a.th.list, a.th.defaults, a.th.loaded = list, defaults, true
	a.th.ver++
	a.th.sel = core.ThemeIndexOf(list, a.th.selID)
	if a.th.sel >= 0 {
		d := core.ThemeDeck(list[a.th.sel])
		a.th.deck = &d
	} else {
		a.th.selID, a.th.deck = 0, nil
	}
}

// wireThemes connects the Themes panel, the theme editor and the schedule's
// Theme submenu (called from buildUI).
func (a *App) wireThemes() {
	p := a.win.Themes
	p.SetThumbnailFactory(func(i int) Control.Instance {
		if i < 0 || i >= len(a.th.list) {
			return Control.Instance{}
		}
		d := core.ThemeDeck(a.th.list[i])
		return thumbnail(&d, 0)
	})
	p.OnSelect = func(i int) { a.event(func() { a.selectTheme(i, nil) }) }
	p.OnActivate = func(i int) { a.event(func() { a.editTheme(i) }) }
	p.OnNew = func() { a.event(a.newTheme) }
	p.OnEdit = func(i int) { a.event(func() { a.editTheme(i) }) }
	p.OnDuplicate = func(i int) { a.event(func() { a.duplicateTheme(i, nil) }) }
	p.OnDelete = func(i int) { a.event(func() { a.deleteTheme(i) }) }
	p.OnSetDefault = func(i int, kind models.ItemKind) { a.event(func() { a.setDefaultTheme(i, kind, nil) }) }
	p.OnApply = func(i int) { a.event(func() { a.applyThemeToItem(i, a.schedSel, nil) }) }

	a.win.Schedule.OnSetTheme = func(row, theme int) { a.event(func() { a.applyThemeToItem(theme, row, nil) }) }
	a.wireSongThemes()

	e := a.win.ThemeEditor
	a.th.editView = render.NewSlideView()
	a.th.editView.SetTransitionDuration(0) // every edit shows at once
	e.Preview.SetContent(a.th.editView.AsControl())
	e.OnChange = func(t models.Theme) { a.event(func() { a.th.editing = &t }) }
	e.OnSave = func(t models.Theme) {
		a.event(func() {
			a.th.editing = nil
			a.saveTheme(t, nil)
		})
	}
	e.OnApply = func(t models.Theme) { a.event(func() { a.saveTheme(t, nil) }) }
	e.OnCancel = func() { a.event(func() { a.th.editing = nil }) }
	e.OnBrowseImage = func() { a.event(a.browseThemeImage) }
}

// defaultTheme is the configured default theme for an item type, or the
// built-in one until the library has answered (or without a library).
func (a *App) defaultTheme(kind models.ItemKind) models.Theme {
	if i := core.ThemeIndexOf(a.th.list, a.th.defaults[kind]); i >= 0 {
		return a.th.list[i]
	}
	return store.BuiltinTheme(kind)
}

// newSongDeck builds a song's deck with the song's own theme, or the default
// song theme when it has none (or its theme is gone): core.SongTheme.
func (a *App) newSongDeck(song models.Song) models.Deck {
	return core.SongDeckWith(song, core.SongTheme(song, a.th.list, a.defaultTheme(models.ItemSong)))
}

// newScriptureDeck builds a passage's deck with the default scripture theme.
func (a *App) newScriptureDeck(p models.Passage, bibleAbbrev string, verses []models.Verse) models.Deck {
	return core.ScriptureDeckWith(p, bibleAbbrev, verses, a.defaultTheme(models.ItemScripture))
}

// themeRows builds the grid rows (name + default badge).
func (a *App) themeRows() []ui.ThemeRow {
	rows := make([]ui.ThemeRow, len(a.th.list))
	for i, t := range a.th.list {
		rows[i] = ui.ThemeRow{Name: t.Name, Badge: core.ThemeBadge(t, a.th.defaults)}
	}
	return rows
}

// themeNames lists the theme names (the schedule's Theme submenu).
func (a *App) themeNames() []string {
	names := make([]string, len(a.th.list))
	for i, t := range a.th.list {
		names[i] = t.Name
	}
	return names
}

// themesStateDoc summarises the tab for GET /state.
func (a *App) themesStateDoc() core.ThemesState {
	editing := ""
	if a.th.editing != nil {
		editing = a.th.editing.Name
	}
	st := core.BuildThemesState(a.th.list, a.th.sel, a.th.defaults, a.win.ThemeEditor.Visible(), editing)
	a.themePolishStateDoc(&st) // themepolish.go
	return st
}

// editorFrame is what the editor's live preview shows for the theme being
// edited (Seq 0: compared by content).
func editorFrame(t models.Theme) models.Frame {
	s := core.SampleSlide()
	return models.Frame{Mode: models.ModeNormal, Slide: &s, Theme: t}
}

// ---- events -------------------------------------------------------------------------

// loadThemes reloads the list and the defaults in the background.
func (a *App) loadThemes(cmd *control.Command) {
	st := a.st()
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	ticket := a.th.gen.Next()
	a.async(cmd, func(ctx context.Context) func() error {
		list, defaults, err := fetchThemes(ctx, st)
		return func() error {
			if err != nil {
				a.fail("load themes", err)
				return err
			}
			if a.th.gen.Current(ticket) {
				a.applyThemes(list, defaults)
			}
			return nil
		}
	})
}

// selectTheme selects tile i and shows its sample in the resource preview.
func (a *App) selectTheme(i int, cmd *control.Command) {
	if i < 0 || i >= len(a.th.list) {
		a.answer(cmd, fmt.Errorf("no theme at index %d", i))
		return
	}
	a.th.sel, a.th.selID = i, a.th.list[i].ID
	d := core.ThemeDeck(a.th.list[i])
	a.th.deck = &d
	a.answer(cmd, nil)
}

// openThemeEditor shows the editor for t (ID 0 = new theme).
func (a *App) openThemeEditor(t models.Theme) {
	cp := t
	a.th.editing = &cp
	a.win.ThemeEditor.Open(t)
}

// newTheme opens the editor on a copy of the default song theme.
func (a *App) newTheme() {
	t := a.defaultTheme(models.ItemSong)
	t.ID, t.Name = 0, "New Theme"
	a.openThemeEditor(t)
}

// editTheme opens the editor on theme i.
func (a *App) editTheme(i int) {
	if i < 0 || i >= len(a.th.list) {
		return
	}
	a.th.sel, a.th.selID = i, a.th.list[i].ID
	a.openThemeEditor(a.th.list[i])
}

// mutateThemes runs a library change in the background, then takes the
// fresh list and defaults into the state and runs then (main thread). cmd is
// answered after that, so the reply reflects the new list.
func (a *App) mutateThemes(cmd *control.Command, title string, change func(ctx context.Context, st *store.Store) error, then func()) {
	st := a.st()
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	ticket := a.th.gen.Next()
	a.async(cmd, func(ctx context.Context) func() error {
		err := change(ctx, st)
		var (
			list     []models.Theme
			defaults map[models.ItemKind]int64
		)
		if err == nil {
			list, defaults, err = fetchThemes(ctx, st)
		}
		return func() error {
			if err != nil {
				a.alert(title, err)
				return err
			}
			if then != nil {
				then()
			}
			if a.th.gen.Current(ticket) {
				a.applyThemes(list, defaults)
			}
			return nil
		}
	})
}

// saveTheme persists a new or edited theme (the editor's OK / Apply and the
// save-theme action), then restyles everything drawn with it.
func (a *App) saveTheme(t models.Theme, cmd *control.Command) {
	isNew := t.ID == 0
	saved := t
	a.mutateThemes(cmd, "Save Theme", func(ctx context.Context, st *store.Store) error {
		if err := st.SaveTheme(ctx, &saved); err != nil {
			return fmt.Errorf("Could not save the theme %q.\n\n%w", t.Name, err)
		}
		return nil
	}, func() {
		log.Printf("[freebeams] saved theme %d %q", saved.ID, saved.Name)
		a.th.selID = saved.ID
		a.applyThemeChange(saved)
		// Apply on a new theme: the editor's copy (with any edits made since)
		// learns its id, so OK updates it instead of inserting again.
		if e := a.win.ThemeEditor; isNew && e.Visible() && a.th.editing != nil && a.th.editing.ID == 0 {
			cp := *a.th.editing
			cp.ID = saved.ID
			a.th.editing = &cp
			e.Update(cp)
		}
	})
}

// applyThemeChange pushes a saved theme into everything that uses it: the
// schedule items (the schedule becomes dirty), the Preview / Live decks (the
// monitors, grids and output re-render through the presenter's Seq) and the
// library decks behind the resource previews.
func (a *App) applyThemeChange(t models.Theme) {
	apply := !a.thp.keepSchedule // the editor's "Apply changes to items in schedule"
	if apply && core.RestyleSchedule(a.sched, t) > 0 {
		a.schedVer++
	}
	a.pres.RestyleDecks(core.ThemeRestyler(t, apply))
	for _, d := range []*models.Deck{a.songRes.deck, a.scriptureDeck, a.th.deck} {
		if d != nil && d.Theme.ID == t.ID && !d.Theme.Custom {
			d.Theme = t
		}
	}
	if a.th.deck != nil && a.th.deck.Theme.ID == t.ID {
		a.th.deck.Title = t.Name
	}
}

// duplicateTheme saves a copy of theme i ("<name> Copy") and selects it.
func (a *App) duplicateTheme(i int, cmd *control.Command) {
	if i < 0 || i >= len(a.th.list) {
		a.answer(cmd, fmt.Errorf("no theme at index %d", i))
		return
	}
	dup := a.th.list[i]
	dup.ID = 0
	dup.Name = core.CopyName(dup.Name, a.themeNames())
	a.mutateThemes(cmd, "Duplicate Theme", func(ctx context.Context, st *store.Store) error {
		if err := st.SaveTheme(ctx, &dup); err != nil {
			return fmt.Errorf("Could not duplicate the theme.\n\n%w", err)
		}
		return nil
	}, func() {
		log.Printf("[freebeams] duplicated theme as %d %q", dup.ID, dup.Name)
		a.th.selID = dup.ID
	})
}

// deleteTheme asks for confirmation, then deletes theme i.
func (a *App) deleteTheme(i int) {
	if i < 0 || i >= len(a.th.list) || a.st() == nil {
		return
	}
	t := a.th.list[i]
	if err := a.themeDeletable(t); err != nil {
		a.alert("Delete Theme", err)
		return
	}
	a.win.Confirm.Ask("Delete Theme", "Delete the theme \""+t.Name+"\"? Schedule items keep their copy of it.", func() {
		a.event(func() { a.doDeleteTheme(t, nil) })
	})
}

// themeDeletable reports why a theme cannot be deleted (it is a default).
func (a *App) themeDeletable(t models.Theme) error {
	if core.IsDefaultTheme(t, a.th.defaults) {
		return fmt.Errorf("%q is the default theme for %s. Choose another default theme first.", t.Name, themeDefaultUses(t, a.th.defaults))
	}
	return nil
}

// themeDefaultUses names what a theme is the default for ("songs and scripture").
func themeDefaultUses(t models.Theme, defaults map[models.ItemKind]int64) string {
	song, scripture := defaults[models.ItemSong] == t.ID, defaults[models.ItemScripture] == t.ID
	switch {
	case song && scripture:
		return "songs and scripture"
	case scripture:
		return "scripture"
	}
	return "songs"
}

// doDeleteTheme deletes a theme (after confirmation).
func (a *App) doDeleteTheme(t models.Theme, cmd *control.Command) {
	if err := a.themeDeletable(t); err != nil {
		a.alert("Delete Theme", err)
		a.answer(cmd, err)
		return
	}
	a.mutateThemes(cmd, "Delete Theme", func(ctx context.Context, st *store.Store) error {
		if err := st.DeleteTheme(ctx, t.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("Could not delete the theme %q.\n\n%w", t.Name, err)
		}
		return nil
	}, func() {
		log.Printf("[freebeams] deleted theme %d %q", t.ID, t.Name)
		a.forgetSongTheme(t.ID)
		if a.th.selID == t.ID {
			a.th.selID = 0
		}
	})
}

// setDefaultTheme makes theme i the default for new song or scripture decks.
func (a *App) setDefaultTheme(i int, kind models.ItemKind, cmd *control.Command) {
	if i < 0 || i >= len(a.th.list) {
		a.answer(cmd, fmt.Errorf("no theme at index %d", i))
		return
	}
	t := a.th.list[i]
	a.mutateThemes(cmd, "Set Default Theme", func(ctx context.Context, st *store.Store) error {
		if err := st.SetDefaultTheme(ctx, kind, t.ID); err != nil {
			return fmt.Errorf("Could not make %q the default theme.\n\n%w", t.Name, err)
		}
		return nil
	}, func() {
		log.Printf("[freebeams] default %s theme: %d %q", kind, t.ID, t.Name)
		a.followDefaultTheme(kind, t) // hotkeys.go: Preview follows the new default
	})
}

// applyThemeToItem gives schedule item row the theme at index theme (the
// per-item override), restyling its deck when it is in Preview or Live.
func (a *App) applyThemeToItem(theme, row int, cmd *control.Command) {
	if theme < 0 || theme >= len(a.th.list) {
		a.answer(cmd, fmt.Errorf("no theme at index %d", theme))
		return
	}
	if row < 0 || row >= len(a.sched.Items) {
		a.answer(cmd, errors.New("select a schedule item first"))
		return
	}
	t := a.th.list[theme]
	item := a.sched.Items[row]
	if core.ApplyItemTheme(a.sched, row, t) {
		a.schedVer++
		a.pres.RestyleDecks(core.RestyleByItemID(item.ID, t))
		log.Printf("[freebeams] schedule item %q uses theme %q", item.Title, t.Name)
	}
	a.answer(cmd, nil)
}

// browseThemeImage shows the native Open dialog for a background image and
// hands the choice to the editor.
func (a *App) browseThemeImage() {
	err := DisplayServer.FileDialogShow("Background Image", a.dialogDir(), "", false, DisplayServer.FileDialogModeOpenFile,
		themeImageFilters, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() { a.win.ThemeEditor.SetBackgroundImage(paths[0]) })
		}, 0)
	if err != nil {
		a.alert("Background Image", fmt.Errorf("Could not show the Open dialog.\n\n%w", err))
	}
}

// ---- control actions ------------------------------------------------------------------

// themeArg resolves a theme from an action's index or name; -1 when none.
func (a *App) themeArg(args actionArgs) int {
	switch {
	case args.Index != nil:
		return *args.Index
	case args.Name != "":
		return core.FindTheme(a.th.list, args.Name)
	}
	return -1
}

// handleThemeCommand runs the theme actions; false when the action is unknown.
func (a *App) handleThemeCommand(cmd *control.Command, args actionArgs) bool {
	switch cmd.Action {
	case "themes":
		a.loadThemes(cmd)
	case "select-theme":
		i := a.themeArg(args)
		if i < 0 || i >= len(a.th.list) {
			cmd.RespondErr(fmt.Errorf("select-theme: no theme matches %s", string(cmd.Data)))
			return true
		}
		a.win.Resources.SetTab(ui.TabThemes)
		a.selectTheme(i, cmd)
	case "get-theme":
		// The full theme JSON (for save-theme round trips).
		i := a.themeArg(args)
		if i < 0 || i >= len(a.th.list) {
			cmd.RespondErr(fmt.Errorf("get-theme: no theme matches %s", string(cmd.Data)))
			return true
		}
		cmd.RespondJSON(struct {
			Theme models.Theme `json:"theme"`
		}{a.th.list[i]})
	case "save-theme":
		// The same path as the editor's OK button (ThemeEditor.OnSave).
		if args.Theme == nil {
			cmd.RespondErr(errors.New("save-theme: a theme object is required"))
			return true
		}
		a.saveTheme(*args.Theme, cmd)
	case "duplicate-theme":
		i := a.themeArg(args)
		if i < 0 || i >= len(a.th.list) {
			cmd.RespondErr(fmt.Errorf("duplicate-theme: no theme matches %s", string(cmd.Data)))
			return true
		}
		a.duplicateTheme(i, cmd)
	case "delete-theme":
		i := a.themeArg(args)
		if i < 0 || i >= len(a.th.list) {
			cmd.RespondErr(fmt.Errorf("delete-theme: no theme matches %s", string(cmd.Data)))
			return true
		}
		if !args.Confirm {
			a.deleteTheme(i) // asks first, like the UI
			a.answer(cmd, nil)
			return true
		}
		a.doDeleteTheme(a.th.list[i], cmd)
	case "set-default-theme":
		// {kind, name} or {kind, themeName} (Options ▸ Themes).
		if args.Name == "" && args.Index == nil {
			args.Name = args.ThemeName
		}
		i := a.themeArg(args)
		if i < 0 || i >= len(a.th.list) {
			cmd.RespondErr(fmt.Errorf("set-default-theme: no theme matches %s", string(cmd.Data)))
			return true
		}
		kind, err := parseDefaultThemeKind(args.Kind) // hotkeys.go
		if err != nil {
			cmd.RespondErr(fmt.Errorf("set-default-theme: %w", err))
			return true
		}
		a.setDefaultTheme(i, kind, cmd)
	case "apply-theme":
		i := a.themeArg(args)
		if i < 0 || i >= len(a.th.list) {
			cmd.RespondErr(fmt.Errorf("apply-theme: no theme matches %s", string(cmd.Data)))
			return true
		}
		row := a.schedSel
		if args.Row != nil {
			row = *args.Row
		}
		a.applyThemeToItem(i, row, cmd)
	default:
		if a.handleThemePolishCommand(cmd) { // themepolish.go
			return true
		}
		return a.handleSongThemeCommand(cmd, args)
	}
	return true
}
