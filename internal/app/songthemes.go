package app

import (
	"context"
	"errors"
	"fmt"
	"log"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/ui"
)

// Per-song themes: a song deck uses the song's own theme when it has one
// (core.SongTheme), else the default song theme. The theme is chosen in the
// song editor's Theme picker or the Songs list's Theme ▸ submenu. A change
// re-themes the song where it shows as library content (the resource preview
// and Preview); schedule items keep their snapshot, so only adding the song
// again picks the new theme up. Deleting a theme clears it from its songs
// (store.DeleteTheme), and the rows and the loaded song follow here.

// wireSongThemes connects the Songs list's Theme ▸ submenu (from wireThemes).
func (a *App) wireSongThemes() {
	a.win.Songs.OnSetTheme = func(i int, themeID int64) { a.event(func() { a.setSongTheme(i, themeID, nil) }) }
}

// songThemeChoices are the themes the song pickers offer after "(Default)".
func (a *App) songThemeChoices() []ui.ThemeChoice {
	out := make([]ui.ThemeChoice, len(a.th.list))
	for i, t := range a.th.list {
		out[i] = ui.ThemeChoice{ID: t.ID, Name: t.Name}
	}
	return out
}

// songThemeName is the loaded song's own theme for GET /state ("" when it
// uses the default song theme).
func (a *App) songThemeName() string {
	if a.song == nil {
		return ""
	}
	return core.SongThemeName(a.song.ThemeID, a.th.list)
}

// setSongTheme gives song row i the theme themeID (0 = the default song
// theme) without opening the editor.
func (a *App) setSongTheme(i int, themeID int64, cmd *control.Command) {
	if i < 0 || i >= len(a.songs) {
		a.answer(cmd, fmt.Errorf("no song at index %d", i))
		return
	}
	if themeID != 0 && core.ThemeIndexOf(a.th.list, themeID) < 0 {
		a.answer(cmd, fmt.Errorf("no theme with id %d", themeID))
		return
	}
	st := a.st()
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	row := a.songs[i]
	a.async(cmd, func(ctx context.Context) func() error {
		err := st.SetSongTheme(ctx, row.ID, themeID)
		return func() error {
			if err != nil {
				a.alert("Song Theme", fmt.Errorf("Could not change the theme of %q.\n\n%w", row.Title, err))
				return err
			}
			name := core.SongThemeName(themeID, a.th.list)
			if name == "" {
				name = core.DefaultThemeLabel
			}
			log.Printf("[freebeams] song %d %q uses theme %q", row.ID, row.Title, name)
			a.songThemeChanged(row.ID, themeID)
			return nil
		}
	})
}

// songThemeChanged takes a song's new theme into the state: the list rows
// (the submenu's check), the loaded song, its resource preview and Preview
// when Preview shows the song as library content (re-themed in place, the
// selection kept). Live and the schedule keep what they show.
func (a *App) songThemeChanged(songID, themeID int64) {
	changed := false
	for k := range a.songs {
		if a.songs[k].ID == songID && a.songs[k].ThemeID != themeID {
			a.songs[k].ThemeID = themeID
			changed = true
		}
	}
	if changed {
		a.songsVer++
	}
	if a.song == nil || a.song.ID != songID {
		return
	}
	old := a.newSongDeck(*a.song)
	song := *a.song
	song.ThemeID = themeID
	a.song = &song
	deck := a.newSongDeck(song)
	if a.songRes.deck != nil && core.SameDeck(a.songRes.deck, &old) {
		a.songRes.deck = &deck
	}
	restyle := core.RestyleLibraryDeck(old, deck.Theme)
	a.pres.RestyleDecks(func(d *models.Deck) (models.Theme, bool) {
		if d != a.pres.State.Preview {
			return models.Theme{}, false
		}
		return restyle(d)
	})
}

// forgetSongTheme follows a deleted theme: the songs that used it (cleared
// in the library by store.DeleteTheme) go back to the default song theme.
// Called before the fresh theme list is applied.
func (a *App) forgetSongTheme(themeID int64) {
	if core.ForgetSongTheme(a.songs, themeID) > 0 {
		a.songsVer++
	}
	if a.song == nil || a.song.ThemeID != themeID {
		return
	}
	song := *a.song
	song.ThemeID = 0
	a.song = &song
	if a.songRes.deck != nil && a.songRes.deck.Theme.ID == themeID {
		deck := a.newSongDeck(song)
		a.songRes.deck = &deck
	}
}

// songThemeArg resolves an action's themeName ("" or "(Default)" = 0).
func (a *App) songThemeArg(action string, args actionArgs) (int64, error) {
	id, ok := core.SongThemeArg(a.th.list, args.ThemeName)
	if !ok {
		return 0, fmt.Errorf("%s: no theme matches %q", action, args.ThemeName)
	}
	return id, nil
}

// handleSongThemeCommand runs the per-song theme actions; false when the
// action is unknown.
func (a *App) handleSongThemeCommand(cmd *control.Command, args actionArgs) bool {
	switch cmd.Action {
	case "set-song-theme":
		// {index|title, themeName}: the Songs list's Theme ▸ submenu.
		i := -1
		switch {
		case args.Index != nil:
			i = *args.Index
		case args.Title != "":
			i = core.FindSong(a.songs, args.Title)
		}
		if i < 0 || i >= len(a.songs) {
			cmd.RespondErr(fmt.Errorf("set-song-theme: no song matches %s", string(cmd.Data)))
			return true
		}
		id, err := a.songThemeArg("set-song-theme", args)
		if err != nil {
			cmd.RespondErr(err)
			return true
		}
		a.setSongTheme(i, id, cmd)
	case "song-editor-theme":
		// {themeName}: a pick in the open song editor's Theme picker.
		if !a.win.Editor.Visible() {
			cmd.RespondErr(errors.New("song-editor-theme: the song editor is not open"))
			return true
		}
		id, err := a.songThemeArg("song-editor-theme", args)
		if err != nil {
			cmd.RespondErr(err)
			return true
		}
		a.win.Editor.PickTheme(id)
		a.answer(cmd, nil)
	case "song-menu":
		// {mode: "open" | "theme" | "close"}: the selected song's context
		// menu, its Theme ▸ submenu beside it, or neither. The reply lists
		// the submenu's entries and the checked one.
		songs := a.win.Songs
		switch args.Mode {
		case "open", "theme":
			if a.songSel < 0 {
				cmd.RespondErr(errors.New("song-menu: select a song first"))
				return true
			}
			songs.OpenMenu()
			if args.Mode == "theme" {
				songs.OpenThemeMenu()
			}
		case "close":
			songs.CloseMenus()
		default:
			cmd.RespondErr(errors.New(`song-menu: mode must be "open", "theme" or "close"`))
			return true
		}
		entries, checked := songs.ThemeMenuEntries()
		cmd.RespondJSON(struct {
			Entries []string `json:"entries"`
			Checked int      `json:"checked"`
		}{entries, checked})
	default:
		return a.handlePreviewStyleCommand(cmd) // the Preview Output menu (previewstyle.go)
	}
	return true
}
