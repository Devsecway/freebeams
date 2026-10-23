package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"freebeams/internal/app/core"
	"freebeams/internal/appdirs"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/store"

	"graphics.gd/classdb/DisplayServer"
)

// Theme polish: video (and library image) backgrounds in the theme editor
// and "Use As Theme Background" on a media tile, EasyWorship's "Apply
// changes to items in schedule" choice, the font search, and sharing themes
// as files (Export Theme… / Import Theme…). The built-in themes are seeded
// with the defaults (initThemes → seedThemePolish).

// themePolishState is the polish's own app state.
type themePolishState struct {
	// keepSchedule is "Apply changes to items in schedule" unchecked: a
	// saved theme leaves the schedule's snapshots alone. The zero value is
	// EasyWorship's applying behaviour, until the setting has loaded.
	keepSchedule bool
	// pickForEditor: the media picker was opened by the theme editor's
	// Library… button, so its OK sets the editor's background.
	pickForEditor bool
}

var themeVideoFilters = []string{"*.mp4, *.mov, *.m4v, *.ogv, *.webm ; Videos"}

// themeFileFilters are the native dialog filters for a theme file.
var themeFileFilters = []string{"*" + core.ThemeFileExt + " ; FreeBeams Themes"}

// seedThemePolish adds the built-in themes once and reads the apply choice
// (goroutine, from initThemes).
func seedThemePolish(ctx context.Context, st *store.Store) (added []string, keep bool, err error) {
	added, err = st.EnsureBuiltinThemes(ctx)
	if err != nil {
		return added, false, err
	}
	apply := true
	if _, err := st.GetSetting(ctx, core.ApplyToScheduleKey, &apply); err != nil {
		return added, false, err
	}
	return added, !apply, nil
}

// wireThemePolish connects the editor's media buttons and apply choice, the
// Themes tab's Export / Import and the media tile's Use As Theme Background
// (from buildUI, after wirePreviewStyle: it wraps the media picker's OK).
func (a *App) wireThemePolish() {
	e := a.win.ThemeEditor
	e.OnPickMedia = func() { a.event(func() { a.openEditorMediaPicker(nil) }) }
	e.OnBrowseVideo = func() { a.event(a.browseThemeVideo) }
	e.OnApplyToSchedule = func(on bool) { a.event(func() { a.setApplyToSchedule(on, nil) }) }
	save, apply, cancel := e.OnSave, e.OnApply, e.OnCancel
	e.OnSave = func(t models.Theme) { a.thp.pickForEditor = false; save(t) }
	e.OnApply = apply
	e.OnCancel = func() { a.thp.pickForEditor = false; cancel() }

	mp := a.win.MediaPicker
	pick := mp.OnPick
	mp.OnPick = func(i int) {
		if !a.thp.pickForEditor || !e.Visible() {
			pick(i)
			return
		}
		a.event(func() {
			a.thp.pickForEditor = false
			a.fail("theme background", a.editorMediaPicked(i))
		})
	}

	p := a.win.Themes
	p.OnExport = func(i int) { a.event(func() { a.browseExportTheme(i) }) }
	p.OnImport = func() { a.event(a.browseImportTheme) }
	a.win.Media.OnThemeBackground = func(i int) { a.event(func() { a.fail("theme background", a.useMediaAsThemeBackground(i)) }) }
}

// renderThemePolish projects the apply choice onto the editor (from render).
func (a *App) renderThemePolish() {
	a.win.ThemeEditor.SetApplyToSchedule(!a.thp.keepSchedule)
}

// ---- events -------------------------------------------------------------------------

// setApplyToSchedule is the editor's "Apply changes to items in schedule"
// checkbox: remembered in the library's settings.
func (a *App) setApplyToSchedule(on bool, cmd *control.Command) {
	a.thp.keepSchedule = !on
	st := a.st()
	if st == nil {
		a.answer(cmd, nil)
		return
	}
	a.async(cmd, func(ctx context.Context) func() error {
		err := st.SetSetting(ctx, core.ApplyToScheduleKey, on)
		return func() error {
			a.fail("save the apply-to-schedule choice", err)
			return err
		}
	})
}

// openEditorMediaPicker is the editor's Library… button: the media picker,
// whose OK sets the editor's background.
func (a *App) openEditorMediaPicker(cmd *control.Command) {
	a.thp.pickForEditor = true
	a.openMediaPicker(cmd)
}

// editorMediaPicked takes media picker tile i into the editor.
func (a *App) editorMediaPicked(i int) error {
	if i < 0 || i >= len(a.ps.media) {
		return fmt.Errorf("no media tile %d", i)
	}
	return a.setEditorMedia(a.ps.media[i], nil, nil)
}

// setEditorMedia makes library item m the edited theme's background; fit
// and loop override the item's own choices when given.
func (a *App) setEditorMedia(m models.Media, fit *models.MediaAspect, loop *bool) error {
	e := a.win.ThemeEditor
	if !e.Visible() {
		return errors.New("the theme editor is not open")
	}
	t, err := core.WithMediaBackground(e.Theme(), m)
	if err != nil {
		return err
	}
	if fit != nil {
		t.Background.Fit = *fit
	}
	if loop != nil {
		t.Background.Loop = *loop
	}
	log.Printf("[freebeams] theme editor background: media %q (%s)", m.Title, t.Background.Kind)
	e.SetBackgroundMedia(t.Background)
	return nil
}

// browseThemeVideo is the video Browse… button: a file outside the library.
func (a *App) browseThemeVideo() {
	err := DisplayServer.FileDialogShow("Background Video", a.dialogDir(), "", false, DisplayServer.FileDialogModeOpenFile,
		themeVideoFilters, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() {
				a.win.ThemeEditor.SetBackgroundMedia(models.Background{Kind: models.BackgroundVideo, Color: models.RGBA{A: 1}, Path: paths[0], Loop: true})
			})
		}, 0)
	if err != nil {
		a.alert("Background Video", fmt.Errorf("Could not show the Open dialog.\n\n%w", err))
	}
}

// useMediaAsThemeBackground is a media tile's Use As Theme Background: the
// open editor takes the item as its background; otherwise a new theme
// opens on it (the default song theme's text, named after the item).
func (a *App) useMediaAsThemeBackground(i int) error {
	if i < 0 || i >= len(a.md.list) {
		return fmt.Errorf("no media item at index %d", i)
	}
	m := a.md.list[i]
	if a.win.ThemeEditor.Visible() {
		return a.setEditorMedia(m, nil, nil)
	}
	t, err := core.MediaTheme(a.defaultTheme(models.ItemSong), m, a.themeNames())
	if err != nil {
		return err
	}
	log.Printf("[freebeams] new theme on media %q", m.Title)
	a.openThemeEditor(t)
	return nil
}

// ---- sharing ------------------------------------------------------------------------

// browseExportTheme asks where to save theme i as a theme file.
func (a *App) browseExportTheme(i int) {
	if i < 0 || i >= len(a.th.list) {
		return
	}
	t := a.th.list[i]
	err := DisplayServer.FileDialogShow("Export Theme", a.dialogDir(), core.ThemeFileName(t), false, DisplayServer.FileDialogModeSaveFile,
		themeFileFilters, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() { a.exportTheme(t, core.WithThemeExt(paths[0]), nil) })
		}, 0)
	if err != nil {
		a.alert("Export Theme", fmt.Errorf("Could not show the Save dialog.\n\n%w", err))
	}
}

// exportTheme writes t (and its background files) to path.
func (a *App) exportTheme(t models.Theme, path string, cmd *control.Command) {
	a.async(nil, func(context.Context) func() error {
		rep, err := core.ExportThemeFile(path, t)
		return func() error {
			if err != nil {
				a.alert("Export Theme", fmt.Errorf("Could not export the theme %q.\n\n%w", t.Name, err))
				a.answer(cmd, err)
				return err
			}
			log.Printf("[freebeams] exported theme %q to %s", t.Name, path)
			if rep.Missing != "" {
				a.alert("Export Theme", fmt.Errorf("The theme was exported without its background, which could not be found:\n%s", rep.Missing))
			}
			if cmd != nil {
				cmd.RespondJSON(struct {
					Path   string               `json:"path"`
					Report core.ThemeFileReport `json:"report"`
				}{path, rep})
			}
			return nil
		}
	})
}

// browseImportTheme asks for a theme file to add.
func (a *App) browseImportTheme() {
	err := DisplayServer.FileDialogShow("Import Theme", a.dialogDir(), "", false, DisplayServer.FileDialogModeOpenFile,
		themeFileFilters, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() { a.importTheme(paths[0], "", nil) })
		}, 0)
	if err != nil {
		a.alert("Import Theme", fmt.Errorf("Could not show the Open dialog.\n\n%w", err))
	}
}

// themeMediaDir is where imported backgrounds are kept: <config>/Theme Media.
func themeMediaDir() (string, error) {
	base, err := appdirs.Config()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Theme Media"), nil
}

// importTheme adds the theme in the file at path (its backgrounds go to
// mediaDir, default themeMediaDir), named uniquely, and selects it.
func (a *App) importTheme(path, mediaDir string, cmd *control.Command) {
	if mediaDir == "" {
		d, err := themeMediaDir()
		if err != nil {
			a.alert("Import Theme", err)
			a.answer(cmd, err)
			return
		}
		mediaDir = d
	}
	taken := a.themeNames()
	var (
		t   models.Theme
		rep core.ThemeFileReport
	)
	a.mutateThemes(cmd, "Import Theme", func(ctx context.Context, st *store.Store) error {
		var err error
		t, rep, err = core.ImportThemeFile(path, mediaDir)
		if err != nil {
			return fmt.Errorf("Could not import %s.\n\n%w", filepath.Base(path), err)
		}
		t.Name = core.UniqueName(t.Name, taken)
		if err := st.SaveTheme(ctx, &t); err != nil {
			return fmt.Errorf("Could not save the imported theme %q.\n\n%w", t.Name, err)
		}
		return nil
	}, func() {
		log.Printf("[freebeams] imported theme %d %q from %s", t.ID, t.Name, path)
		a.th.selID = t.ID
		if rep.Missing != "" {
			if _, err := os.Stat(rep.Missing); err != nil {
				a.alert("Import Theme", fmt.Errorf("%q was shared without its background:\n%s\n\nChoose another background in the theme editor.", t.Name, rep.Missing))
			}
		}
	})
}

// ---- control actions ----------------------------------------------------------------

type themePolishArgs struct {
	Name     string  `json:"name"`
	Index    *int    `json:"index"`
	Title    string  `json:"title"`
	Path     string  `json:"path"`
	MediaDir string  `json:"mediaDir"`
	Q        *string `json:"q"`
	On       *bool   `json:"on"`
	Fit      string  `json:"fit"`
	Loop     *bool   `json:"loop"`
	Family   *string `json:"family"`
}

// fontReply is the reply of the font actions: the list as shown.
func (a *App) fontReply(cmd *control.Command) {
	e := a.win.ThemeEditor
	cmd.RespondJSON(struct {
		Open    bool     `json:"open"`
		Matches []string `json:"matches"`
		Total   int      `json:"total"`
		Family  string   `json:"family"`
	}{e.FontListOpen(), e.FontMatches(), e.FontCount(), e.Theme().Font.Family})
}

// handleThemePolishCommand runs the polish's actions; false when the action
// is not one of them.
//
//	theme-apply-to-schedule {on}        the editor's checkbox
//	theme-font-search {q}               types into the font search (reply: the matches)
//	theme-font {family}                 picks a font from the list ("" = default)
//	theme-editor-media {title|index, fit, loop}   Library… with that item
//	use-as-theme-background {title|index}         the media tile's entry
//	export-theme {name|index, path}     Export Theme… to path
//	import-theme {path, mediaDir}       Import Theme…
func (a *App) handleThemePolishCommand(cmd *control.Command) bool {
	var args themePolishArgs
	switch cmd.Action {
	case "theme-apply-to-schedule", "theme-font-search", "theme-font", "theme-editor-media",
		"use-as-theme-background", "export-theme", "import-theme":
	default:
		return false
	}
	if err := decodeArgs(cmd, &args); err != nil {
		cmd.RespondErr(err)
		return true
	}
	e := a.win.ThemeEditor
	media := func() (int, bool) {
		i := -1
		switch {
		case args.Index != nil:
			i = *args.Index
		case args.Title != "":
			i = core.FindMedia(a.md.list, args.Title)
		}
		if i < 0 || i >= len(a.md.list) {
			cmd.RespondErr(fmt.Errorf("%s: no media item matches %s", cmd.Action, string(cmd.Data)))
			return 0, false
		}
		return i, true
	}
	switch cmd.Action {
	case "theme-apply-to-schedule":
		if args.On == nil {
			cmd.RespondErr(errors.New("theme-apply-to-schedule: on is required"))
			return true
		}
		a.setApplyToSchedule(*args.On, cmd)
	case "theme-font-search":
		if !e.Visible() || args.Q == nil {
			cmd.RespondErr(errors.New("theme-font-search: open the theme editor and give q"))
			return true
		}
		e.SearchFonts(*args.Q)
		a.fontReply(cmd)
	case "theme-font":
		if !e.Visible() || args.Family == nil || !e.PickFont(*args.Family) {
			cmd.RespondErr(fmt.Errorf("theme-font: no font %s in the open editor", string(cmd.Data)))
			return true
		}
		a.fontReply(cmd)
	case "theme-editor-media":
		i, ok := media()
		if !ok {
			return true
		}
		m := a.md.list[i]
		var fit *models.MediaAspect
		if args.Fit != "" {
			f, err := models.ParseMediaAspect(args.Fit)
			if err != nil {
				cmd.RespondErr(fmt.Errorf("theme-editor-media: %w", err))
				return true
			}
			fit = &f
		}
		a.answer(cmd, a.setEditorMedia(m, fit, args.Loop))
	case "use-as-theme-background":
		i, ok := media()
		if !ok {
			return true
		}
		a.answer(cmd, a.useMediaAsThemeBackground(i))
	case "export-theme":
		i := a.themeArg(actionArgs{Index: args.Index, Name: args.Name})
		if i < 0 || i >= len(a.th.list) || args.Path == "" {
			cmd.RespondErr(fmt.Errorf("export-theme: give a theme (name|index) and a path: %s", string(cmd.Data)))
			return true
		}
		a.exportTheme(a.th.list[i], core.WithThemeExt(args.Path), cmd)
	case "import-theme":
		if args.Path == "" {
			cmd.RespondErr(errors.New("import-theme: path is required"))
			return true
		}
		a.importTheme(args.Path, args.MediaDir, cmd)
	}
	return true
}

// themePolishStateDoc adds the polish's part of /state.themes.
func (a *App) themePolishStateDoc(st *core.ThemesState) {
	st.ApplyToSchedule = !a.thp.keepSchedule
	e := a.win.ThemeEditor
	if !e.Visible() {
		return
	}
	t := e.Theme()
	st.EditBackground, st.EditBox, st.EditFont = &t.Background, &t.Box, t.Font.Family
	st.FontList = e.FontListOpen()
	if st.FontList {
		st.FontMatches = len(e.FontMatches())
	}
}
