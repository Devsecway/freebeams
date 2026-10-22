package remote

import (
	"strconv"
	"strings"

	"freebeams/internal/models"
)

// State is what the phone shows: the schedule, the Preview and Live decks
// and the output mode. The app builds it on the main thread (BuildState) and
// publishes it; every connected phone gets it within a frame of a change.
type State struct {
	Seq      uint64    `json:"seq"`  // the presenter's sequence number
	Name     string    `json:"name"` // this FreeBeams ("FreeBeams on Godwin's MacBook Pro")
	Mode     string    `json:"mode"` // normal | black | clear | logo
	Output   bool      `json:"output"`
	Schedule []ItemDoc `json:"schedule"`
	Title    string    `json:"title"`    // the schedule's title
	Selected int       `json:"selected"` // schedule row in Preview, -1
	LiveItem int       `json:"liveItem"` // schedule row on air, -1
	Preview  DeckDoc   `json:"preview"`
	Live     DeckDoc   `json:"live"`
}

// ItemDoc is one schedule row.
type ItemDoc struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Kind     string `json:"kind"` // song | scripture | media | presentation
	Slides   int    `json:"slides"`
}

// DeckDoc is the Preview or Live deck.
type DeckDoc struct {
	Title  string     `json:"title"`
	Item   int        `json:"item"`  // its schedule row, -1 for library content
	Index  int        `json:"index"` // selected (Preview) or live (Live) slide, -1
	Slides []SlideDoc `json:"slides"`
	Label  string     `json:"label"`  // the selected slide's label
	Text   string     `json:"text"`   // the selected slide's full text
	Footer string     `json:"footer"` // the selected slide's footer
}

// SlideDoc is one slide in a deck's list.
type SlideDoc struct {
	Label   string `json:"label"`
	Snippet string `json:"snippet"`
	Kind    string `json:"kind"`
}

// StateInput is what BuildState reads.
type StateInput struct {
	Name      string
	Presenter models.PresenterState
	Schedule  *models.Schedule
	Selected  int // the schedule row Preview came from, -1
}

// BuildState makes the phone's state document.
func BuildState(in StateInput) State {
	p := in.Presenter
	st := State{
		Seq: p.Seq, Name: in.Name, Mode: string(p.Mode), Output: p.OutputEnabled,
		Schedule: []ItemDoc{}, Selected: in.Selected, LiveItem: -1,
	}
	if st.Mode == "" {
		st.Mode = string(models.ModeNormal)
	}
	if s := in.Schedule; s != nil {
		st.Title = s.Title
		for _, it := range s.Items {
			st.Schedule = append(st.Schedule, ItemDoc{Title: it.Title, Subtitle: it.Subtitle, Kind: string(it.Kind), Slides: len(it.Slides)})
		}
	}
	itemOf := func(d *models.Deck) int {
		if d == nil || d.ItemID == "" || in.Schedule == nil {
			return -1
		}
		return in.Schedule.IndexOf(d.ItemID)
	}
	st.Preview = deckDoc(p.Preview, p.PreviewIndex, itemOf(p.Preview))
	st.Live = deckDoc(p.Live, p.LiveIndex, itemOf(p.Live))
	st.LiveItem = st.Live.Item
	if st.Selected < -1 || st.Selected >= len(st.Schedule) {
		st.Selected = -1
	}
	return st
}

func deckDoc(d *models.Deck, i, item int) DeckDoc {
	doc := DeckDoc{Item: item, Index: -1, Slides: []SlideDoc{}}
	if d == nil {
		return doc
	}
	doc.Title = d.Title
	for n, s := range d.Slides {
		doc.Slides = append(doc.Slides, SlideDoc{Label: SlideLabel(s, n), Snippet: Snippet(s.Text, snippetRunes), Kind: string(s.Kind)})
	}
	if i >= 0 && i < len(d.Slides) {
		s := d.Slides[i]
		doc.Index, doc.Label, doc.Text, doc.Footer = i, SlideLabel(s, i), s.Text, s.Footer
	}
	return doc
}

// SlideLabel is a slide's label, or "Slide n" when it has none.
func SlideLabel(s models.Slide, i int) string {
	if l := strings.TrimSpace(s.Label); l != "" {
		return l
	}
	return "Slide " + strconv.Itoa(i+1)
}

// snippetRunes bounds a slide's snippet in the list.
const snippetRunes = 90

// Snippet joins a slide's lines with " / " and trims it to n runes with "…".
func Snippet(text string, n int) string {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			lines = append(lines, l)
		}
	}
	s := strings.Join(lines, " / ")
	if r := []rune(s); len(r) > n {
		s = strings.TrimRight(string(r[:n-1]), " /") + "…"
	}
	return s
}
