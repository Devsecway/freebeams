package core

import "freebeams/internal/models"

// DeckState summarises the Preview or Live deck for GET /state.
type DeckState struct {
	Title  string `json:"title"`
	ItemID string `json:"itemId,omitempty"`
	Index  int    `json:"index"`
	Count  int    `json:"count"`
	Label  string `json:"label"`  // selected/live slide label ("Verse 2", "John 3:16")
	Text   string `json:"text"`   // selected/live slide text
	Footer string `json:"footer"` // selected/live slide footer

	// Theme is the name of the theme the deck is drawn with.
	Theme string `json:"theme"`

	// Style is the deck's look (the fields the Preview Output menu changes);
	// nil without a deck.
	Style *DeckStyle `json:"style,omitempty"`
}

// ScheduleState summarises the schedule.
type ScheduleState struct {
	Title    string   `json:"title"`
	Path     string   `json:"path"`
	Dirty    bool     `json:"dirty"`
	Items    []string `json:"items"`
	Selected int      `json:"selected"`
	Live     int      `json:"live"` // index of the live item, -1 when none
}

// ImportState is the first-run EasyWorship import status.
type ImportState struct {
	Running bool   `json:"running"`
	Done    bool   `json:"done"`
	Summary string `json:"summary,omitempty"`
	Error   string `json:"error,omitempty"`
}

// SongsState summarises the Songs tab.
type SongsState struct {
	Count     int    `json:"count"`   // songs in the library
	Query     string `json:"query"`   // current search
	Results   int    `json:"results"` // rows in the list
	Selected  int    `json:"selected"`
	Current   string `json:"current,omitempty"` // title of the loaded (selected) song
	CountText string `json:"countText"`

	// The search ranking (store.SearchSongs): First is the title of the first
	// row, LyricsMatches the rows that needed their lyrics to match.
	First         string `json:"first,omitempty"`
	LyricsMatches int    `json:"lyricsMatches"`

	// Theme is the loaded song's own theme ("" when it uses the default song
	// theme): the per-song theme.
	Theme string `json:"theme"`
}

// ScriptureState summarises the Scriptures tab.
type ScriptureState struct {
	Bibles    []string `json:"bibles"`
	Bible     string   `json:"bible"`
	Reference string   `json:"reference"`
	Passage   string   `json:"passage,omitempty"`
	Verses    int      `json:"verses"`
	Selected  int      `json:"selected"`
	Status    string   `json:"status"`
	Error     bool     `json:"error"`
	// CopyrightedAvailable lists EasyWorship's copyrighted Bibles that an
	// on-request import would add (CopyrightedBiblesToImport).
	CopyrightedAvailable []string `json:"copyrightedAvailable"`

	// The verse list lists whole chapters (Listed, Verses rows); Rows are the
	// selected rows (Passage names them, Selected is the current one). Field
	// is the reference field's text, Suggestions the books it offers for it
	// (Suggestion highlighted), shown in the dropdown while it has the focus.
	Listed           string   `json:"listed"`
	Rows             []int    `json:"rows"`
	Field            string   `json:"field"`
	Suggestions      []string `json:"suggestions"`
	Suggestion       int      `json:"suggestion"`
	SuggestionsShown bool     `json:"suggestionsShown"`
}

// MediaState summarises the Media tab.
type MediaState struct {
	Count     int    `json:"count"`  // items in the library
	Videos    int    `json:"videos"` // of which videos
	Images    int    `json:"images"` // of which images
	Query     string `json:"query"`
	Kind      string `json:"kind"`    // collection filter: "", "video", "image"
	Results   int    `json:"results"` // tiles in the grid
	Selected  int    `json:"selected"`
	Current   string `json:"current,omitempty"` // title of the selected item
	CountText string `json:"countText"`
	Working   bool   `json:"working"`  // thumbnails / transcodes being made
	Pending   int    `json:"pending"`  // listed items still without a thumbnail
	Playable  int    `json:"playable"` // listed videos with a playable file
	FFmpeg    bool   `json:"ffmpeg"`
	Theora    bool   `json:"theora"`
	Tools     string `json:"tools"` // media.Tools.Status()

	// The tile menus (menus_items.go): the selected item's Aspect Ratio ▸ and
	// Repeating ▸ choices, and the panel's View ▸ and Sort by ▸.
	Aspect string `json:"aspect,omitempty"` // fill | fit | stretch
	Repeat string `json:"repeat,omitempty"` // loop | once
	View   string `json:"view"`             // large | medium | small | details
	Sort   string `json:"sort"`             // name | date | kind

	// Audio files in the library, and whether videos and audio play natively
	// (AVFoundation) so that no transcode is needed (playback.go).
	Audio  int  `json:"audio"`
	Native bool `json:"native"`

	// Player is the media player backend: avfoundation, ffmpeg or theora.
	Player string `json:"player"`

	// Posters names what takes video poster frames and reads durations:
	// "avfoundation" (macOS, ffmpeg for what it cannot read), "ffmpeg" or ""
	// (none: videos show a plain tile).
	Posters string `json:"posters"`

	// Tags are the selected item's tags (the Properties dialog).
	Tags string `json:"tags,omitempty"`

	// Volume and Mute are the selected item's own volume (percent; 0 =
	// none, the monitor's carries over) and Mute (the Properties dialog).
	Volume int  `json:"volume"`
	Mute   bool `json:"mute"`
}

// PresentationsState summarises the Presentations tab.
type PresentationsState struct {
	Count     int    `json:"count"` // presentations in the library
	Query     string `json:"query"`
	Results   int    `json:"results"` // tiles in the grid
	Selected  int    `json:"selected"`
	Current   string `json:"current,omitempty"` // title of the selected presentation
	CountText string `json:"countText"`
	Working   bool   `json:"working"` // pages being rendered
	Pending   int    `json:"pending"` // listed items still without pages
	Failed    int    `json:"failed"`  // listed items whose render failed this session
	// The selected presentation's render: its page count, whether only its
	// embedded images could be used, and which backend made it.
	CurrentPages    int    `json:"currentPages"`
	CurrentPartial  bool   `json:"currentPartial"`
	CurrentRenderer string `json:"currentRenderer,omitempty"`
	PDF             bool   `json:"pdf"`    // a PDF renderer exists
	Office          bool   `json:"office"` // LibreOffice converts office documents
	Tools           string `json:"tools"`  // presentations.Tools.Status()

	// The tile menu's View ▸ and Sort by ▸ (menus_items.go).
	View string `json:"view"` // large | medium | small | details
	Sort string `json:"sort"` // name | date | kind
}

// Alert is an error message for the operator, shown in the message dialog.
type Alert struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// DialogState describes the open dialogs.
type DialogState struct {
	Editor   bool   `json:"editor"`            // the song editor is open
	Settings bool   `json:"settings"`          // the Options dialog is open
	Confirm  string `json:"confirm,omitempty"` // title of the open question
	Message  *Alert `json:"message,omitempty"` // the error message on screen
	Alerts   int    `json:"alerts"`            // error messages queued (shown one at a time)
	// ThemeEditor reports the theme editor being open.
	ThemeEditor bool `json:"themeEditor,omitempty"`
	// SongEditorTheme is the song editor's Theme picker entry ("(Default)" or
	// a theme name) while the editor is open.
	SongEditorTheme string `json:"songEditorTheme,omitempty"`
	// Host is the window the topmost dialog is a child of: "main" for the
	// operator window, else the dialog under it ("editor", …). It is never the
	// output window. "" when no dialog is open.
	Host string `json:"host,omitempty"`

	// StageMessage reports Live ▸ Stage Message… being open.
	StageMessage bool `json:"stageMessage,omitempty"`

	// MediaProperties reports the media tile's Edit Properties… being open.
	MediaProperties bool `json:"mediaProperties,omitempty"`

	// AlertDialog names the topmost open alert dialog: alert-manager,
	// alert-editor, alert-fill, nursery or countdown ("" when none).
	AlertDialog string `json:"alertDialog,omitempty"`
	// AlertsMenu reports the toolbar's Alerts ▾ menu being open.
	AlertsMenu bool `json:"alertsMenu,omitempty"`

	// PreviewMenu reports the Preview Output monitor's context menu being
	// open; PreviewDialog names its open dialog: color, media or item-theme.
	PreviewMenu   bool   `json:"previewMenu,omitempty"`
	PreviewDialog string `json:"previewDialog,omitempty"`

	// Spell is the song editor's spelling checker while the editor is open
	// (spell.go).
	Spell *SpellState `json:"spell,omitempty"`
}

// ResourceState is what the resource preview (right of the resource tabs) shows.
type ResourceState struct {
	Title   string `json:"title"`
	Caption string `json:"caption"` // "John 3:18 · 3 / 3"
}

// State is the GET /state document.
type State struct {
	Seq              uint64         `json:"seq"`
	Mode             string         `json:"mode"`
	Preview          DeckState      `json:"preview"`
	Live             DeckState      `json:"live"`
	OutputEnabled    bool           `json:"outputEnabled"`
	OutputOpen       bool           `json:"outputOpen"`
	OutputFullscreen bool           `json:"outputFullscreen"`
	OutputScreen     int            `json:"outputScreen"`
	Screens          int            `json:"screens"`
	Schedule         ScheduleState  `json:"schedule"`
	Songs            SongsState     `json:"songs"`
	SongCount        int            `json:"songCount"`
	Scripture        ScriptureState `json:"scripture"`
	Media            MediaState     `json:"media"`
	Themes           ThemesState    `json:"themes"`
	Resource         ResourceState  `json:"resource"`
	Settings         Settings       `json:"settings"` // the Options dialog's values
	Bibles           []string       `json:"bibles"`
	Import           ImportState    `json:"import"`
	LastError        string         `json:"lastError"`
	Dialog           DialogState    `json:"dialog"`
	WindowTitle      string         `json:"windowTitle"`
	Tab              int            `json:"tab"`
	Focus            string         `json:"focus"`
	FocusOwner       string         `json:"focusOwner"` // node path of the focused control (debugging)
	Pending          int            `json:"pending"`
	Transitioning    bool           `json:"transitioning"`
	Idle             bool           `json:"idle"`
	// Splitters is where every splitter is, by name (see the split hooks).
	Splitters map[string]SplitPos `json:"splitters"`

	// Presentations is the Presentations tab (its own block keeps gofmt from
	// realigning the fields above).
	Presentations PresentationsState `json:"presentations"`

	// Bundled is the bundled translations' installation (bundled.go).
	Bundled BundledState `json:"bundled"`

	// Playback is the video / audio transport of the live and preview
	// monitors (playback.go).
	Playback PlaybackStates `json:"playback"`

	// Stage is the stage display / Foldback output (stage.go).
	Stage StageState `json:"stage"`

	// Remote is the LAN remote control (remote.go).
	Remote RemoteState `json:"remote"`

	// Alerts is the alert overlay: message alerts, nursery codes, the
	// countdown and the clock (alerts.go).
	Alerts AlertsState `json:"alerts"`

	// Audio is the schedule's background audio playlist and its player (audio.go).
	Audio AudioState `json:"audio"`

	// Options is the Options dialog's editors (keymap.go): a hotkey capture
	// and its outcome.
	Options OptionsState `json:"options"`

	// TransitionProgress is the output's running transition, 0..1 (0 when
	// none; the live monitor when the output is closed).
	TransitionProgress float64 `json:"transitionProgress"`

	// BibleManager is the Bible Manager: every translation it can add or
	// remove, the Bibles folder and the running task (biblemanager.go).
	BibleManager BibleManagerState `json:"bibleManager"`
}

// StateInput is everything BuildState reads. The app fills it on the main
// thread; BuildState itself is pure.
type StateInput struct {
	Splitters        map[string]SplitPos
	Presenter        models.PresenterState
	Schedule         *models.Schedule
	ScheduleSelected int
	OutputOpen       bool
	OutputFullscreen bool
	OutputScreen     int
	Screens          int
	Songs            SongsState
	Bibles           []models.Bible
	BibleSelected    int
	Scripture        ScriptureState // Bibles/Bible are filled from Bibles/BibleSelected
	Media            MediaState
	Themes           ThemesState
	Resource         ResourceState
	Settings         Settings
	Import           ImportState
	LastError        string
	Dialog           DialogState
	WindowTitle      string
	Tab              int
	Focus            Focus
	FocusOwner       string
	Pending          int
	Transitioning    bool
	Idle             bool

	Presentations PresentationsState

	Bundled BundledState

	Playback PlaybackStates

	Stage StageState

	Remote RemoteState
	Alerts AlertsState

	Audio AudioState

	Options OptionsState

	TransitionProgress float64

	BibleManager BibleManagerState
}

// deckState summarises deck d with slide i selected.
func deckState(d *models.Deck, i int) DeckState {
	ds := DeckState{Index: i}
	if d == nil {
		return ds
	}
	ds.Title, ds.ItemID, ds.Count = d.Title, d.ItemID, len(d.Slides)
	ds.Theme = d.Theme.Name
	style := StyleOf(d.Theme)
	ds.Style = &style
	if i >= 0 && i < len(d.Slides) {
		s := d.Slides[i]
		ds.Label, ds.Text, ds.Footer = s.Label, s.Text, s.Footer
	}
	return ds
}

// BuildState builds the /state document.
func BuildState(in StateInput) State {
	p := in.Presenter
	mode := string(p.Mode)
	if mode == "" {
		mode = string(models.ModeNormal)
	}
	st := State{
		Seq:              p.Seq,
		Mode:             mode,
		Preview:          deckState(p.Preview, p.PreviewIndex),
		Live:             deckState(p.Live, p.LiveIndex),
		OutputEnabled:    p.OutputEnabled,
		OutputOpen:       in.OutputOpen,
		OutputFullscreen: in.OutputFullscreen,
		OutputScreen:     in.OutputScreen,
		Screens:          in.Screens,
		Songs:            in.Songs,
		SongCount:        in.Songs.Count,
		Scripture:        in.Scripture,
		Media:            in.Media,
		Themes:           in.Themes,
		Resource:         in.Resource,
		Settings:         in.Settings,
		Import:           in.Import,
		LastError:        in.LastError,
		Dialog:           in.Dialog,
		WindowTitle:      in.WindowTitle,
		Tab:              in.Tab,
		Focus:            in.Focus.String(),
		FocusOwner:       in.FocusOwner,
		Pending:          in.Pending,
		Transitioning:    in.Transitioning,
		Idle:             in.Idle,
		Splitters:        in.Splitters,
		Presentations:    in.Presentations,
	}
	if st.Splitters == nil {
		st.Splitters = map[string]SplitPos{}
	}
	if st.Scripture.CopyrightedAvailable == nil {
		st.Scripture.CopyrightedAvailable = []string{}
	}
	st.Playback = in.Playback
	st.Stage = in.Stage
	st.Remote = in.Remote.normalized()
	st.Alerts = in.Alerts
	st.Audio = in.Audio
	st.Options = in.Options
	st.TransitionProgress = in.TransitionProgress
	if st.Audio.Tracks == nil {
		st.Audio.Tracks = []AudioTrackState{}
	}
	if st.Audio.Order == nil {
		st.Audio.Order = []int{}
	}
	if st.Alerts.Active == nil {
		st.Alerts.Active = []AlertRunState{}
	}
	if st.Alerts.Nursery == nil {
		st.Alerts.Nursery = []string{}
	}
	if st.Alerts.Saved == nil {
		st.Alerts.Saved = []models.Alert{}
	}
	st.Bundled = in.Bundled
	st.BibleManager = in.BibleManager
	if st.BibleManager.Entries == nil {
		st.BibleManager.Entries = []BibleEntry{}
	}
	if st.BibleManager.Problems == nil {
		st.BibleManager.Problems = []string{}
	}
	if st.Bundled.Installed == nil {
		st.Bundled.Installed = []string{}
	}
	if st.Bundled.Skipped == nil {
		st.Bundled.Skipped = []string{}
	}
	st.Bibles = make([]string, 0, len(in.Bibles))
	for _, b := range in.Bibles {
		st.Bibles = append(st.Bibles, b.Abbrev)
	}
	st.Scripture.Bibles = st.Bibles
	if in.BibleSelected >= 0 && in.BibleSelected < len(in.Bibles) {
		st.Scripture.Bible = in.Bibles[in.BibleSelected].Abbrev
	}
	st.Schedule = ScheduleState{Selected: in.ScheduleSelected, Live: -1, Items: []string{}}
	if s := in.Schedule; s != nil {
		st.Schedule.Title, st.Schedule.Path, st.Schedule.Dirty = s.Title, s.Path, s.Dirty
		for _, it := range s.Items {
			st.Schedule.Items = append(st.Schedule.Items, it.Title)
		}
		st.Schedule.Live = LiveItemIndex(s, p.Live)
	}
	return st
}
