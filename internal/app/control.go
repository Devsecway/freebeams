package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/render"
	"freebeams/internal/ui"

	"graphics.gd/classdb/SceneTree"
)

// answer replies to a control command (nil cmd: nothing to do): with err at
// once, or with the state after this frame's render pass (so the reply
// reflects the projected view, e.g. the output window really being open).
func (a *App) answer(cmd *control.Command, err error) {
	if cmd == nil {
		return
	}
	if err != nil {
		cmd.RespondErr(err)
		return
	}
	a.replies = append(a.replies, func() { cmd.RespondJSON(a.state()) })
}

// flushReplies sends the replies queued by answer.
func (a *App) flushReplies() {
	rs := a.replies
	a.replies = nil
	for _, r := range rs {
		r()
	}
}

// state builds the GET /state document.
func (a *App) state() core.State {
	_, resTitle, resCaption := a.resourcePreviewFrame()
	dialog := core.DialogState{Editor: a.win.Editor.Visible(), ThemeEditor: a.win.ThemeEditor.Visible(),
		Settings: a.win.Settings.Visible(), Alerts: len(a.alerts), Host: a.win.TopDialogHost()}
	if a.win.Confirm.Visible() {
		dialog.Confirm = a.win.Confirm.Title()
	}
	if a.win.Editor.Visible() {
		dialog.SongEditorTheme = a.win.Editor.ThemeName()
	}
	if m := a.win.Message; m.Visible() {
		dialog.Message = &core.Alert{Title: m.Title(), Message: m.Text()}
	}
	dialog.AlertDialog, dialog.AlertsMenu = a.win.AlertDialog(), a.win.Toolbar.AlertsMenuVisible()
	return core.BuildState(core.StateInput{
		Presenter:        a.pres.State,
		Schedule:         a.sched,
		ScheduleSelected: a.schedSel,
		OutputOpen:       a.out.IsOpen(),
		OutputFullscreen: a.out.Fullscreen(),
		OutputScreen:     a.out.Screen(),
		Screens:          len(render.Screens()),
		Songs: core.SongsState{
			Count: a.songCount, Query: a.songQuery, Results: len(a.songs), Selected: a.songSel,
			Current: songTitle(a.song), CountText: a.songsStatus,
			First: core.FirstSongTitle(a.songs), LyricsMatches: core.LyricsMatches(a.songs),
			Theme: a.songThemeName(),
		},
		Bibles:        a.bibles,
		BibleSelected: a.bibleSel,
		Scripture: core.ScriptureState{
			Reference: a.reference, Passage: deckTitle(a.scriptureDeck), Verses: len(a.verseRows),
			Selected: a.verseSelection(), Status: a.scriptStatus, Error: a.scriptErr,
			CopyrightedAvailable: core.CopyrightedBiblesToImport(a.ewBibles, a.bibles),
		},
		Media:         a.mediaStateDoc(),
		Presentations: a.presentationsStateDoc(),
		Themes:        a.themesStateDoc(),
		Resource:      core.ResourceState{Title: resTitle, Caption: resCaption},
		Settings:      a.settings,
		Splitters:     a.splitterPositions(),
		Import:        a.importDoc(),
		Bundled:       a.bundled,
		Playback:      a.playbackStates(),
		Remote:        a.remoteStateDoc(),
		Alerts:        a.alertsStateDoc(),
		LastError:     a.lastError,
		Dialog:        dialog,
		WindowTitle:   a.win.WindowTitle(),
		Tab:           a.win.Resources.Tab(),
		Focus:         a.focus(),
		FocusOwner:    a.focusOwnerPath(),
		Pending:       a.pending,
		Transitioning: a.transitioning(),
		Idle:          !a.busy(),
	})
}

func songTitle(s *models.Song) string {
	if s == nil {
		return ""
	}
	return s.Title
}

func deckTitle(d *models.Deck) string {
	if d == nil {
		return ""
	}
	return d.Title
}

// actionArgs is the union of every action's JSON body.
type actionArgs struct {
	Q        string          `json:"q"`
	Index    *int            `json:"index"`
	Title    string          `json:"title"`
	Text     string          `json:"text"`
	Bible    string          `json:"bible"`
	I        *int            `json:"i"`
	Slide    *int            `json:"slide"`
	Kind     string          `json:"kind"`
	N        int             `json:"n"`
	On       *bool           `json:"on"`
	Source   string          `json:"source"`
	From     *int            `json:"from"`
	To       *int            `json:"to"`
	Path     string          `json:"path"`
	Paths    []string        `json:"paths"` // add-media: several files / folders
	Name     string          `json:"name"`
	Key      string          `json:"key"`      // set-setting: the setting (settings.go)
	Value    json.RawMessage `json:"value"`    // set-setting: its JSON value
	Duration *int            `json:"duration"` // transition: milliseconds
	Item     string          `json:"item"`     // menu: the menu item
	Page     string          `json:"page"`     // options: the section to show
	Ctrl     bool            `json:"ctrl"`
	Meta     bool            `json:"meta"`
	Shift    bool            `json:"shift"`
	Alt      bool            `json:"alt"`
	Tab      string          `json:"tab"`

	// UI-driving hooks (hooks.go).
	Target    string  `json:"target"`
	Mode      string  `json:"mode"`
	ID        int64   `json:"id"`
	Author    string  `json:"author"`
	Copyright string  `json:"copyright"`
	CCLI      string  `json:"ccli"`
	Lyrics    string  `json:"lyrics"`
	Confirm   bool    `json:"confirm"`
	Window    string  `json:"window"`  // native-key: "main" (default) or "output"
	Percent   float64 `json:"percent"` // split: the new splitter position (0..1)
	Delta     float64 `json:"delta"`   // drag-split: logical pixels per frame
	Steps     int     `json:"steps"`   // drag-split: number of frames to drag
	Burst     int     `json:"burst"`   // drag-split: mouse motions per frame (default 1)

	// Themes (themes.go).
	Theme *models.Theme `json:"theme"` // save-theme: the theme to save (id 0 inserts)
	Row   *int          `json:"row"`   // apply-theme: schedule row (default: the selected one)

	// Per-song themes (songthemes.go): set-song-theme, song-editor-theme and
	// save-song take a theme name; "" or "(Default)" is the default song theme.
	ThemeName string `json:"themeName"`

	// Playback (playback.go); volume takes Value, loop takes On, all take Target.
	Seconds *float64 `json:"seconds"` // seek: the position in seconds; countdown / alert: a length

	// The remote control (remote.go): the server's port and a pairing
	// request's or a paired device's id (Name matches by name).
	Port   *int   `json:"port"`
	Device string `json:"device"`

	// Alerts (alerts.go).
	Values   map[string]string `json:"values"`   // alert / alert-fill: token values
	Alert    *models.Alert     `json:"alert"`    // save-alert / alert-editor
	Code     string            `json:"code"`     // nursery: the code
	Caption  string            `json:"caption"`  // countdown: the line above the time
	Position string            `json:"position"` // countdown: top-left … center
	End      string            `json:"end"`      // countdown: hide|hold|overrun; alert: count|manual|token|duration
	Time     string            `json:"time"`     // countdown: to a time of day
	Minutes  *float64          `json:"minutes"`  // countdown: a length in minutes
	Display  string            `json:"display"`  // alert: ticker|static|auto
	Location string            `json:"location"` // alert: top|bottom
	Count    *int              `json:"count"`    // alert: passes
	Align    string            `json:"align"`    // alert: left|center|right
	Run      *int              `json:"run"`      // alert-clear: one showing
}

var errArgs = errors.New("missing or invalid arguments")

// sectionKinds maps `jump {kind}` names to section kinds.
var sectionKinds = map[string]models.SectionKind{
	"verse": models.SectionVerse, "v": models.SectionVerse,
	"chorus": models.SectionChorus, "c": models.SectionChorus,
	"bridge": models.SectionBridge, "b": models.SectionBridge,
	"prechorus": models.SectionPreChorus, "pre-chorus": models.SectionPreChorus, "r": models.SectionPreChorus,
	"tag": models.SectionTag, "t": models.SectionTag,
	"intro": models.SectionIntro, "i": models.SectionIntro,
	"ending": models.SectionEnding, "e": models.SectionEnding,
}

// handleCommand executes a control command on the main thread. Commands that
// start background work are answered when the work completes.
func (a *App) handleCommand(cmd *control.Command) {
	defer func() {
		if r := recover(); r != nil {
			cmd.RespondErr(fmt.Errorf("panic handling %s: %v", cmd.Action, r))
		}
	}()
	var args actionArgs
	if len(cmd.Data) > 0 {
		if err := json.Unmarshal(cmd.Data, &args); err != nil {
			cmd.RespondErr(fmt.Errorf("%s: bad JSON body: %w", cmd.Action, err))
			return
		}
	}
	p := a.pres
	ok := func() { a.answer(cmd, nil) }

	switch cmd.Action {
	case "state":
		ok()
	case "screenshot":
		a.screenshot(cmd)
	case "wait-idle":
		a.waiters = append(a.waiters, cmd)

	// Songs.
	case "search-songs":
		a.win.Songs.SetSearchText(args.Q)
		a.searchSongs(args.Q, cmd)
	case "select-song":
		i := -1
		switch {
		case args.Index != nil:
			i = *args.Index
		case args.Title != "":
			i = core.FindSong(a.songs, args.Title)
		}
		if i < 0 || i >= len(a.songs) {
			cmd.RespondErr(fmt.Errorf("select-song: no song matches %s", string(cmd.Data)))
			return
		}
		a.win.Resources.SetTab(ui.TabSongs)
		a.selectSong(i, songPreview, cmd)

	// Scriptures.
	case "reference":
		if args.Bible != "" {
			i := core.FindBible(a.bibles, args.Bible)
			if i < 0 {
				cmd.RespondErr(fmt.Errorf("reference: Bible %q is not installed", args.Bible))
				return
			}
			a.bibleSel = i
		}
		a.win.Resources.SetTab(ui.TabScriptures)
		a.win.Scriptures.SetReferenceText(args.Text)
		a.lookupReference(args.Text, cmd)
	case "select-preview-slide":
		i := args.I
		if i == nil {
			i = args.Index
		}
		if i == nil {
			cmd.RespondErr(errArgs)
			return
		}
		p.SelectPreviewSlide(*i)
		ok()

	// Presenting.
	case "go-live":
		if args.Slide != nil {
			a.liveChange(func() { p.GoLiveSlide(*args.Slide) })
		} else {
			a.goLive()
		}
		ok()
	case "next":
		p.Next()
		ok()
	case "prev":
		p.Prev()
		ok()
	case "first":
		p.First()
		ok()
	case "last":
		p.Last()
		ok()
	case "jump":
		kind := strings.ToLower(args.Kind)
		if kind == "slide" || kind == "s" {
			if args.N <= 0 {
				cmd.RespondErr(errArgs)
				return
			}
			p.SelectLiveSlide(args.N - 1)
			ok()
			return
		}
		k, found := sectionKinds[kind]
		if !found {
			cmd.RespondErr(fmt.Errorf("jump: unknown kind %q", args.Kind))
			return
		}
		if !p.JumpToSection(k, args.N) {
			cmd.RespondErr(fmt.Errorf("jump: no %s %d in the live item", k, args.N))
			return
		}
		ok()
	case "black": // {on} sets the mode instead of toggling it (phones, remote.go)
		a.setMode(models.ModeBlack, args.On, p.ToggleBlack)
		ok()
	case "clear":
		a.setMode(models.ModeClear, args.On, p.ToggleClear)
		ok()
	case "logo":
		a.setMode(models.ModeLogo, args.On, p.ToggleLogo)
		ok()
	case "output":
		on := !p.State.OutputEnabled
		if args.On != nil {
			on = *args.On
		}
		a.setOutput(on)
		ok()
	case "go-back":
		a.goBack()
		ok()

	// Schedule.
	case "add-to-schedule":
		switch args.Source {
		case "song":
			if a.song == nil {
				cmd.RespondErr(errors.New("add-to-schedule: no song selected"))
				return
			}
			a.addSongToSchedule(*a.song)
			ok()
		case "scripture":
			a.addScriptureToSchedule(cmd)
		case "media":
			a.addMediaToSchedule(cmd)
		case "presentation":
			a.addPresentationToSchedule(cmd)
		default:
			cmd.RespondErr(fmt.Errorf("add-to-schedule: source must be \"song\", \"scripture\", \"media\" or \"presentation\", got %q", args.Source))
		}

	// Media.
	case "search-media":
		kind := models.MediaKind(strings.ToLower(args.Kind))
		if kind != "" && kind != models.MediaImage && kind != models.MediaVideo && kind != models.MediaAudio {
			cmd.RespondErr(fmt.Errorf("search-media: kind must be \"\", \"image\", \"video\" or \"audio\", got %q", args.Kind))
			return
		}
		a.win.Media.SetSearchText(args.Q)
		a.loadMedia(args.Q, kind, cmd)
	case "select-media":
		i := -1
		switch {
		case args.Index != nil:
			i = *args.Index
		case args.Title != "":
			i = core.FindMedia(a.md.list, args.Title)
		}
		if i < 0 || i >= len(a.md.list) {
			cmd.RespondErr(fmt.Errorf("select-media: no media item matches %s", string(cmd.Data)))
			return
		}
		a.win.Resources.SetTab(ui.TabMedia)
		a.selectMedia(i, mediaPreview, cmd)
	case "add-media":
		paths := args.Paths
		if args.Path != "" {
			paths = append(paths, args.Path)
		}
		a.addMediaPaths(paths, cmd)
	case "remove-media":
		i := -1
		switch {
		case args.Index != nil:
			i = *args.Index
		case args.Title != "":
			i = core.FindMedia(a.md.list, args.Title)
		}
		if i < 0 || i >= len(a.md.list) {
			cmd.RespondErr(fmt.Errorf("remove-media: no media item matches %s", string(cmd.Data)))
			return
		}
		if !args.Confirm {
			a.removeMedia(i) // asks first, like the UI
			ok()
			return
		}
		a.doRemoveMedia(a.md.list[i], cmd)

	// Presentations.
	case "search-presentations":
		a.win.Presentations.SetSearchText(args.Q)
		a.loadPresentations(args.Q, cmd)
	case "select-presentation":
		i := -1
		switch {
		case args.Index != nil:
			i = *args.Index
		case args.Title != "":
			i = core.FindPresentation(a.pr.list, args.Title)
		}
		if i < 0 || i >= len(a.pr.list) {
			cmd.RespondErr(fmt.Errorf("select-presentation: no presentation matches %s", string(cmd.Data)))
			return
		}
		a.win.Resources.SetTab(ui.TabPresentations)
		a.selectPresentation(i, presPreview, cmd)
	case "add-presentation":
		paths := args.Paths
		if args.Path != "" {
			paths = append(paths, args.Path)
		}
		a.addPresentationPaths(paths, cmd)
	case "remove-presentation":
		i := -1
		switch {
		case args.Index != nil:
			i = *args.Index
		case args.Title != "":
			i = core.FindPresentation(a.pr.list, args.Title)
		}
		if i < 0 || i >= len(a.pr.list) {
			cmd.RespondErr(fmt.Errorf("remove-presentation: no presentation matches %s", string(cmd.Data)))
			return
		}
		if !args.Confirm {
			a.removePresentation(i) // asks first, like the UI
			ok()
			return
		}
		a.doRemovePresentation(a.pr.list[i], cmd)
	case "select-schedule":
		if args.Index == nil || *args.Index < 0 || *args.Index >= len(a.sched.Items) {
			cmd.RespondErr(errArgs)
			return
		}
		a.selectScheduleItem(*args.Index)
		ok()
	case "remove-schedule":
		if args.Index == nil || !a.removeScheduleItem(*args.Index) {
			cmd.RespondErr(errArgs)
			return
		}
		ok()
	case "move-schedule":
		if args.From == nil || args.To == nil || !a.moveScheduleItem(*args.From, *args.To) {
			cmd.RespondErr(errArgs)
			return
		}
		ok()
	case "save-schedule":
		path := args.Path
		if path == "" {
			path = a.sched.Path
		}
		if path == "" {
			cmd.RespondErr(errors.New("save-schedule: no path"))
			return
		}
		a.answer(cmd, a.saveSchedule(path))
	case "open-schedule":
		if args.Path == "" {
			cmd.RespondErr(errArgs)
			return
		}
		a.answer(cmd, a.openSchedule(args.Path))
	case "new-schedule":
		a.newSchedule()
		ok()

	// Keys and UI.
	case "import-bibles":
		// EasyWorship's copyrighted Bibles; asks first unless confirm is set.
		a.importCopyrightedBibles(cmd, args.Confirm)
	case "import-bible-file":
		// A Bible file or translation folder (biblefile.go); asks first unless confirm is set.
		a.importBibleFile(cmd, args.Path, args.Confirm)
	case "key":
		name := core.NormalizeKeyName(args.Name)
		if name == "" {
			cmd.RespondErr(errArgs)
			return
		}
		ev := core.KeyEvent{Key: name, Ctrl: args.Ctrl, Meta: args.Meta, Shift: args.Shift, Alt: args.Alt}
		consumed := a.handleKey(ev)
		a.replies = append(a.replies, func() {
			cmd.RespondJSON(struct {
				Consumed bool       `json:"consumed"`
				State    core.State `json:"state"`
			}{consumed, a.state()})
		})
	case "tab":
		tabs := map[string]int{"songs": ui.TabSongs, "scriptures": ui.TabScriptures, "media": ui.TabMedia,
			"presentations": ui.TabPresentations, "themes": ui.TabThemes}
		t, found := tabs[strings.ToLower(args.Tab)]
		if !found {
			cmd.RespondErr(errArgs)
			return
		}
		a.win.Resources.SetTab(t)
		ok()
	case "quit":
		a.quitNext = true
		cmd.RespondJSON(map[string]bool{"quitting": true})
	default:
		if !a.handleThemeCommand(cmd, args) && !a.handleSettingsCommand(cmd, args) && !a.handleItemMenuCommand(cmd, args) && !a.handlePlaybackCommand(cmd, args) && !a.handleRemoteCommand(cmd, args) && !a.handleAlertsCommand(cmd, args) && !a.handleHook(cmd, args) {
			cmd.RespondErr(fmt.Errorf("unknown action %q", cmd.Action))
		}
	}
}

// screenshot answers GET /screenshot?window=main|output with a PNG.
func (a *App) screenshot(cmd *control.Command) {
	var (
		png []byte
		err error
	)
	switch cmd.Query["window"] {
	case "", "main":
		png, err = render.WindowScreenshot(SceneTree.Get(a.AsNode()).Root())
	case "editor":
		png, err = render.WindowScreenshot(a.win.Editor.AsWindow())
	case "theme-editor":
		png, err = render.WindowScreenshot(a.win.ThemeEditor.AsWindow())
	case "settings":
		png, err = render.WindowScreenshot(a.win.Settings.AsWindow())
	case "dialog":
		png, err = render.WindowScreenshot(a.win.Confirm.AsWindow())
	case "message":
		png, err = render.WindowScreenshot(a.win.Message.AsWindow())
	case "remote": // the remote control's pairing dialog
		png, err = render.WindowScreenshot(a.win.Remote.AsWindow())
	case "song-menu", "song-theme-menu": // the Songs list's context menu / its Theme ▸ submenu
		w := a.win.Songs.MenuWindow()
		if cmd.Query["window"] == "song-theme-menu" {
			w = a.win.Songs.ThemeMenuWindow()
		}
		png, err = render.WindowScreenshot(w)
	case "output":
		png, err = a.out.Screenshot()
	case "alert-manager", "alert-editor", "alert-fill", "nursery", "countdown":
		w, _ := a.win.AlertWindow(cmd.Query["window"])
		png, err = render.WindowScreenshot(w)
	case "alerts-menu":
		png, err = render.WindowScreenshot(a.win.Toolbar.AlertsMenuWindow())
	default:
		err = fmt.Errorf("screenshot: unknown window %q (main|output|editor|settings|dialog|message)", cmd.Query["window"])
	}
	if err != nil {
		cmd.RespondErr(err)
		return
	}
	cmd.Respond(control.Result{OK: true, PNG: png})
}
