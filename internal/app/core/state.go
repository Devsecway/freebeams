package core

import "freeworship/internal/models"

// DeckState summarises the Preview or Live deck for GET /state.
type DeckState struct {
	Title  string `json:"title"`
	ItemID string `json:"itemId,omitempty"`
	Index  int    `json:"index"`
	Count  int    `json:"count"`
	Label  string `json:"label"`  // selected/live slide label ("Verse 2", "John 3:16")
	Text   string `json:"text"`   // selected/live slide text
	Footer string `json:"footer"` // selected/live slide footer
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
}

// Alert is an error message for the operator, shown in the message dialog.
type Alert struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// DialogState describes the open dialogs.
type DialogState struct {
	Editor  bool   `json:"editor"`            // the song editor is open
	Confirm string `json:"confirm,omitempty"` // title of the open question
	Message *Alert `json:"message,omitempty"` // the error message on screen
	Alerts  int    `json:"alerts"`            // error messages queued (shown one at a time)
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
	Bibles           []string       `json:"bibles"`
	Import           ImportState    `json:"import"`
	LastError        string         `json:"lastError"`
	Dialog           DialogState    `json:"dialog"`
	WindowTitle      string         `json:"windowTitle"`
	Tab              int            `json:"tab"`
	Focus            string         `json:"focus"`
	Pending          int            `json:"pending"`
	Transitioning    bool           `json:"transitioning"`
	Idle             bool           `json:"idle"`
}

// StateInput is everything BuildState reads. The app fills it on the main
// thread; BuildState itself is pure.
type StateInput struct {
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
	Import           ImportState
	LastError        string
	Dialog           DialogState
	WindowTitle      string
	Tab              int
	Focus            Focus
	Pending          int
	Transitioning    bool
	Idle             bool
}

// deckState summarises deck d with slide i selected.
func deckState(d *models.Deck, i int) DeckState {
	ds := DeckState{Index: i}
	if d == nil {
		return ds
	}
	ds.Title, ds.ItemID, ds.Count = d.Title, d.ItemID, len(d.Slides)
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
		Import:           in.Import,
		LastError:        in.LastError,
		Dialog:           in.Dialog,
		WindowTitle:      in.WindowTitle,
		Tab:              in.Tab,
		Focus:            in.Focus.String(),
		Pending:          in.Pending,
		Transitioning:    in.Transitioning,
		Idle:             in.Idle,
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
