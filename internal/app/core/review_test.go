package core

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"freebeams/internal/models"
)

// TestSequencer: song loads finish in any order (the store has a pool of
// connections), but their results are applied in request order, so an older
// request never overrides a newer one.
func TestSequencer(t *testing.T) {
	tests := []struct {
		name     string
		requests int
		finish   []int // ticket completion order (1-based tickets)
		want     []int // applied order
		waiting  int   // results still held back at the end
	}{
		{"in order", 3, []int{1, 2, 3}, []int{1, 2, 3}, 0},
		{"reversed", 3, []int{3, 2, 1}, []int{1, 2, 3}, 0},
		{"newest first then the rest", 3, []int{3, 1, 2}, []int{1, 2, 3}, 0},
		{"a gap holds later results back", 3, []int{2, 3}, nil, 2},
		{"gap filled later", 4, []int{2, 4, 1, 3}, []int{1, 2, 3, 4}, 0},
		{"duplicates and unknown tickets are ignored", 2, []int{1, 1, 7, 2, 2}, []int{1, 2}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Sequencer
			for i := 0; i < tt.requests; i++ {
				s.Next()
			}
			var got []int
			for _, ticket := range tt.finish {
				tk := ticket
				s.Done(uint64(tk), func() { got = append(got, tk) })
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("applied %v, want %v", got, tt.want)
			}
			if s.Waiting() != tt.waiting {
				t.Errorf("waiting = %d, want %d", s.Waiting(), tt.waiting)
			}
		})
	}
}

// TestSequencerSupersededIntents is the app's rule for song loads: every
// result applies in order, and a preview that a newer request overtook is
// dropped (it has no side effect), while add-to-schedule still happens.
func TestSequencerSupersededIntents(t *testing.T) {
	type req struct {
		intent string
		song   string
	}
	reqs := []req{{"schedule", "A"}, {"preview", "B"}, {"golive", "C"}, {"preview", "D"}}
	var s Sequencer
	tickets := make([]uint64, len(reqs))
	for i := range reqs {
		tickets[i] = s.Next()
	}
	var log []string
	current := ""
	for _, i := range []int{3, 2, 0, 1} { // completion order
		r, tk := reqs[i], tickets[i]
		s.Done(tk, func() {
			if r.intent == "preview" && s.Superseded(tk) {
				log = append(log, "drop "+r.song)
				return
			}
			current = r.song
			log = append(log, r.intent+" "+r.song)
		})
	}
	want := []string{"schedule A", "drop B", "golive C", "preview D"}
	if !reflect.DeepEqual(log, want) || current != "D" {
		t.Errorf("log %v current %q, want %v current D", log, current, want)
	}
}

func TestWaitTimeout(t *testing.T) {
	var wg sync.WaitGroup
	if !WaitTimeout(&wg, time.Second) {
		t.Error("an idle WaitGroup must finish at once")
	}
	wg.Add(1)
	start := time.Now()
	if WaitTimeout(&wg, 50*time.Millisecond) {
		t.Error("a busy WaitGroup must time out")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("WaitTimeout took %v", d)
	}
	go func() { time.Sleep(10 * time.Millisecond); wg.Done() }()
	if !WaitTimeout(&wg, 5*time.Second) {
		t.Error("the job finished; WaitTimeout must see it")
	}
}

func passageDeck(t *testing.T, ref string, n int) *models.Deck {
	t.Helper()
	p, err := models.ParseReference(ref)
	if err != nil {
		t.Fatal(err)
	}
	var verses []models.Verse
	for i := 0; i < n; i++ {
		verses = append(verses, models.Verse{Book: 43, Chapter: 3, Verse: 16 + i, Text: "verse text " + string(rune('a'+i))})
	}
	d := ScriptureDeck(p, "KJV", verses)
	return &d
}

// TestSelectedVerse: the verse Go Live sends is the one the list highlights.
func TestSelectedVerse(t *testing.T) {
	pass := passageDeck(t, "jn 3:16-18", 3)
	copyOf := func(d *models.Deck) *models.Deck {
		c := *d
		c.Slides = append([]models.Slide(nil), d.Slides...)
		return &c
	}
	other := passageDeck(t, "jn 3:16-17", 2)
	song := SongDeck(amazingGrace())
	sched := models.NewSchedule()
	sched.Add(ScriptureItem(models.Passage{}, models.Bible{}, *pass))
	asItem := ItemDeck(sched.Items[0])
	tests := []struct {
		name         string
		passage      *models.Deck
		preview      *models.Deck
		previewIndex int
		lastRow      int
		want         int
	}{
		{"no passage", nil, pass, 1, 2, -1},
		{"preview holds the passage: the preview selection wins", pass, pass, 2, 0, 2},
		{"an equal copy of the passage counts", pass, copyOf(pass), 1, 0, 1},
		{"preview moved to slide 1 after a row click on 0", pass, pass, 1, 0, 1},
		{"preview shows a song: the last row", pass, &song, 1, 2, 2},
		{"preview shows another passage: the last row", pass, other, 0, 1, 1},
		{"preview shows the passage from the schedule: the last row", pass, asItem, 2, 0, 0},
		{"nothing in preview: the last row", pass, nil, -1, 1, 1},
		{"no row chosen", pass, &song, 0, -1, -1},
		{"stale row out of range", pass, nil, -1, 5, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SelectedVerse(tt.passage, tt.preview, tt.previewIndex, tt.lastRow); got != tt.want {
				t.Errorf("SelectedVerse = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSameDeck(t *testing.T) {
	a := passageDeck(t, "jn 3:16-18", 3)
	b := passageDeck(t, "jn 3:16-18", 3)
	c := passageDeck(t, "jn 3:16-17", 2)
	item := *a
	item.ItemID = "x"
	tests := []struct {
		name string
		x, y *models.Deck
		want bool
	}{
		{"both nil", nil, nil, true},
		{"one nil", a, nil, false},
		{"same pointer", a, a, true},
		{"equal content", a, b, true},
		{"different passage", a, c, false},
		{"schedule item vs library", a, &item, false},
	}
	for _, tt := range tests {
		if got := SameDeck(tt.x, tt.y); got != tt.want {
			t.Errorf("%s: SameDeck = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestScheduleSelection: the selected schedule row follows Preview, so after
// Preview is loaded from the library no row is selected and clicking the
// previously selected row loads it again.
func TestScheduleSelection(t *testing.T) {
	sched := models.NewSchedule()
	song := amazingGrace()
	sched.Add(SongItem(song, SongDeck(song)))
	sched.Add(SongItem(song, SongDeck(song)))
	library := SongDeck(song)
	tests := []struct {
		name    string
		preview *models.Deck
		want    int
	}{
		{"nothing in preview", nil, -1},
		{"item 0", ItemDeck(sched.Items[0]), 0},
		{"item 1 (same song, other item)", ItemDeck(sched.Items[1]), 1},
		{"library song", &library, -1},
		{"item of another schedule", &models.Deck{ItemID: "gone"}, -1},
	}
	for _, tt := range tests {
		if got := ScheduleSelection(sched, tt.preview); got != tt.want {
			t.Errorf("%s: ScheduleSelection = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestSongsFooter(t *testing.T) {
	tests := []struct {
		name string
		n    int
		imp  ImportState
		want string
	}{
		{"count", 223, ImportState{Done: true}, "223 songs"},
		{"one", 1, ImportState{}, "1 song"},
		{"importing", 0, ImportState{Running: true}, "Importing from EasyWorship…"},
		{"failed import stays visible", 12, ImportState{Error: "disk full"}, "12 songs · EasyWorship import failed"},
	}
	for _, tt := range tests {
		if got := SongsFooter(tt.n, tt.imp); got != tt.want {
			t.Errorf("%s: SongsFooter = %q, want %q", tt.name, got, tt.want)
		}
	}
	matched := []struct {
		name      string
		n, lyrics int
		imp       ImportState
		want      string
	}{
		{"no lyrics matches", 5, 0, ImportState{Done: true}, "5 songs"},
		{"lyrics matches", 5, 2, ImportState{Done: true}, "5 songs · 2 matched in lyrics"},
		{"one of one", 1, 1, ImportState{}, "1 song · 1 matched in lyrics"},
		{"importing wins", 5, 2, ImportState{Running: true}, "Importing from EasyWorship…"},
		{"failed import stays visible", 5, 2, ImportState{Error: "x"}, "5 songs · 2 matched in lyrics · EasyWorship import failed"},
	}
	for _, tt := range matched {
		if got := SongsFooterMatches(tt.n, tt.lyrics, tt.imp); got != tt.want {
			t.Errorf("%s: SongsFooterMatches = %q, want %q", tt.name, got, tt.want)
		}
	}
	rows := []models.SongSummary{{Title: "Amazing Grace", Matched: models.MatchedTitle}, {Title: "Alas", Matched: models.MatchedLyrics}, {Title: "At The Cross", Matched: models.MatchedLyrics}}
	if LyricsMatches(rows) != 2 || LyricsMatches(nil) != 0 || FirstSongTitle(rows) != "Amazing Grace" || FirstSongTitle(nil) != "" {
		t.Error("LyricsMatches / FirstSongTitle")
	}
}

func TestIsSaveAs(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "Sunday.fbs")
	tests := []struct {
		name        string
		cur, target string
		want        bool
	}{
		{"never saved", "", filepath.Join(dir, "Sunday.fbs"), true},
		{"same file", cur, cur, false},
		{"same file without the extension", cur, filepath.Join(dir, "Sunday"), false},
		{"same file, unclean path", cur, filepath.Join(dir, "x", "..", "Sunday.fbs"), false},
		{"another name", cur, filepath.Join(dir, "Evening.fbs"), true},
		{"another folder", cur, filepath.Join(dir, "sub", "Sunday.fbs"), true},
		{"no target", cur, "", false},
	}
	for _, tt := range tests {
		if got := IsSaveAs(tt.cur, tt.target); got != tt.want {
			t.Errorf("%s: IsSaveAs(%q, %q) = %v, want %v", tt.name, tt.cur, tt.target, got, tt.want)
		}
	}
}

func TestBuildStateDialog(t *testing.T) {
	st := BuildState(StateInput{
		Presenter: models.NewPresenter().State,
		Dialog:    DialogState{Editor: true, Confirm: "Quit FreeBeams", Message: &Alert{Title: "Open Schedule", Message: "bad file"}, Alerts: 2, Host: "editor"},
	})
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"dialog":{"editor":true,"settings":false,"confirm":"Quit FreeBeams","message":{"title":"Open Schedule","message":"bad file"},"alerts":2,"host":"editor"}`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("state JSON %s\nlacks %s", b, want)
		}
	}
}
