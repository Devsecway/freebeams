package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"freeworship/internal/importers/easyworship"
	"freeworship/internal/models"
)

func amazingGrace() models.Song {
	return models.Song{
		ID: 7, Title: "Amazing Grace", Author: "John Newton", Copyright: "Public Domain",
		Sections: []models.Section{
			{Kind: models.SectionVerse, Number: 1, Label: "Verse 1", Lines: []string{"Amazing grace how sweet the sound", "That saved a wretch like me"}},
			{Kind: models.SectionVerse, Number: 2, Label: "Verse 2", Lines: []string{"'Twas grace that taught my heart to fear"}},
			{Kind: models.SectionChorus, Label: "Chorus", Lines: []string{"My chains are gone"}},
		},
	}
}

func TestSongDeckAndItem(t *testing.T) {
	song := amazingGrace()
	deck := SongDeck(song)
	if deck.Title != "Amazing Grace" || deck.ItemID != "" || len(deck.Slides) != 3 {
		t.Fatalf("deck = %+v", deck)
	}
	if deck.Theme.Name != models.DefaultSongTheme().Name {
		t.Errorf("theme = %q", deck.Theme.Name)
	}
	if !strings.Contains(deck.Slides[0].Text, "Amazing grace") || deck.Slides[1].Label != "Verse 2" {
		t.Errorf("slides = %+v", deck.Slides)
	}
	item := SongItem(song, deck)
	if item.Kind != models.ItemSong || item.SongID != 7 || item.Title != "Amazing Grace" || item.Subtitle != "John Newton" {
		t.Errorf("item = %+v", item)
	}
	// The item is a snapshot: changing the deck afterwards does not change it.
	deck.Slides[0].Text = "changed"
	if item.Slides[0].Text == "changed" {
		t.Error("item shares slides with the deck")
	}
}

func TestScriptureDeckAndItem(t *testing.T) {
	p, err := models.ParseReference("jn 3:16-17")
	if err != nil {
		t.Fatal(err)
	}
	verses := []models.Verse{
		{Book: 43, Chapter: 3, Verse: 16, Text: "For God so loved the world"},
		{Book: 43, Chapter: 3, Verse: 17, Text: "For God sent not his Son"},
	}
	deck := ScriptureDeck(p, "KJV", verses)
	if deck.Title != p.String() || len(deck.Slides) != 2 {
		t.Fatalf("deck = %+v", deck)
	}
	if deck.Slides[0].Footer != "John 3:16 (KJV)" {
		t.Errorf("footer = %q", deck.Slides[0].Footer)
	}
	if deck.Theme.Name != models.DefaultScriptureTheme().Name {
		t.Errorf("theme = %q", deck.Theme.Name)
	}
	item := ScriptureItem(p, models.Bible{ID: 3, Abbrev: "KJV"}, deck)
	if item.Kind != models.ItemScripture || item.BibleID != 3 || item.Passage == nil || item.Passage.Book != 43 ||
		item.Subtitle != "KJV · Scripture" || item.Title != "John 3:16-17" {
		t.Errorf("item = %+v", item)
	}
	rows := VerseRows(verses)
	if !reflect.DeepEqual(rows, []string{"16  For God so loved the world", "17  For God sent not his Son"}) {
		t.Errorf("rows = %q", rows)
	}
}

func TestVerseRowsCrossChapterAndBlank(t *testing.T) {
	rows := VerseRows([]models.Verse{
		{Chapter: 3, Verse: 36, Text: "He that believeth"},
		{Chapter: 4, Verse: 1, Text: "  "},
		{Chapter: 4, Verse: 2, Text: "Though Jesus\nhimself"},
	})
	want := []string{"36  He that believeth", "4:2  Though Jesus himself"}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %q, want %q", rows, want)
	}
}

func TestItemDeck(t *testing.T) {
	tests := []struct {
		name string
		item models.ScheduleItem
	}{
		{"song", models.ScheduleItem{ID: "a", Kind: models.ItemSong, Title: "Amazing Grace", Theme: models.DefaultSongTheme(),
			Slides: []models.Slide{{Label: "Verse 1", Text: "x"}, {Label: "Verse 2", Text: "y"}}}},
		{"scripture with background override", models.ScheduleItem{ID: "b", Kind: models.ItemScripture, Title: "John 3:16",
			Theme:  models.DefaultScriptureTheme(),
			Slides: []models.Slide{{Label: "John 3:16", Text: "For God", Background: &models.Background{Kind: models.BackgroundColor}}}}},
		{"empty", models.ScheduleItem{ID: "c", Title: "Empty"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ItemDeck(tt.item)
			if d.ItemID != tt.item.ID || d.Title != tt.item.Title || d.Theme != tt.item.Theme ||
				!reflect.DeepEqual(d.Slides, tt.item.Slides) {
				t.Fatalf("deck = %+v", d)
			}
			for i := range d.Slides {
				if d.Slides[i].Background != nil && d.Slides[i].Background == tt.item.Slides[i].Background {
					t.Error("background pointer shared with the item")
				}
			}
		})
	}
}

func TestLiveItemIndex(t *testing.T) {
	s := models.NewSchedule()
	s.Add(models.ScheduleItem{ID: "one", Title: "One"})
	s.Add(models.ScheduleItem{ID: "two", Title: "Two"})
	tests := []struct {
		name string
		live *models.Deck
		want int
	}{
		{"nothing live", nil, -1},
		{"library item live", &models.Deck{Title: "x"}, -1},
		{"second item live", ItemDeck(s.Items[1]), 1},
		{"removed item live", &models.Deck{ItemID: "gone"}, -1},
	}
	for _, tt := range tests {
		if got := LiveItemIndex(s, tt.live); got != tt.want {
			t.Errorf("%s: got %d want %d", tt.name, got, tt.want)
		}
	}
	if LiveItemIndex(nil, ItemDeck(s.Items[0])) != -1 {
		t.Error("nil schedule")
	}
}

func TestStepIndex(t *testing.T) {
	tests := []struct {
		cur, n, delta int
		want          int
		ok            bool
	}{
		{-1, 3, +1, 0, true},
		{-1, 3, -1, 2, true},
		{0, 3, +1, 1, true},
		{2, 3, +1, 2, false},
		{0, 3, -1, 0, false},
		{1, 3, -1, 0, true},
		{0, 0, +1, 0, false},
		{5, 3, +1, 0, true}, // stale selection
		{1, 3, 0, 1, false},
	}
	for _, tt := range tests {
		got, ok := StepIndex(tt.cur, tt.n, tt.delta)
		if got != tt.want || ok != tt.ok {
			t.Errorf("StepIndex(%d,%d,%d) = %d,%v want %d,%v", tt.cur, tt.n, tt.delta, got, ok, tt.want, tt.ok)
		}
	}
}

func TestGridLabelsAndCaption(t *testing.T) {
	d := &models.Deck{Slides: []models.Slide{{Label: "Verse 1"}, {Label: ""}}}
	if got := GridLabels(d); !reflect.DeepEqual(got, []GridLabel{{"Verse 1", 1}, {"", 2}}) {
		t.Errorf("labels = %+v", got)
	}
	if GridLabels(nil) != nil {
		t.Error("nil deck")
	}
	for _, tt := range []struct {
		i    int
		want string
	}{{0, "Verse 1 · 1 / 2"}, {1, "2 / 2"}, {2, ""}, {-1, ""}} {
		if got := SlideCaption(d, tt.i); got != tt.want {
			t.Errorf("caption(%d) = %q want %q", tt.i, got, tt.want)
		}
	}
}

func TestFindSongAndBible(t *testing.T) {
	rows := []models.SongSummary{{Title: "A Mighty Fortress"}, {Title: "Amazing Grace (My Chains)"}, {Title: "Amazing Grace"}}
	for _, tt := range []struct {
		q    string
		want int
	}{{"amazing grace", 2}, {"Amazing", 1}, {"fortress", 0}, {"nope", -1}, {"", -1}} {
		if got := FindSong(rows, tt.q); got != tt.want {
			t.Errorf("FindSong(%q) = %d want %d", tt.q, got, tt.want)
		}
	}
	bibles := []models.Bible{{Abbrev: "RVA", Name: "Reina-Valera Antigua"}, {Abbrev: "KJV", Name: "King James Version"}}
	if FindBible(bibles, "kjv") != 1 || FindBible(bibles, "Reina-Valera Antigua") != 0 || FindBible(bibles, "x") != -1 {
		t.Error("FindBible")
	}
	if DefaultBible(bibles) != 1 || DefaultBible(bibles[:1]) != 0 || DefaultBible(nil) != -1 {
		t.Error("DefaultBible")
	}
}

func TestGeneration(t *testing.T) {
	var g Generation
	if g.Current(0) {
		t.Fatal("ticket 0 must never be current")
	}
	// Three searches are typed; their results arrive out of order.
	a, b, c := g.Next(), g.Next(), g.Next()
	type result struct {
		ticket uint64
		query  string
	}
	var applied []string
	for _, r := range []result{{c, "ama"}, {a, "a"}, {b, "am"}} {
		if g.Current(r.ticket) {
			applied = append(applied, r.query)
		}
	}
	if !reflect.DeepEqual(applied, []string{"ama"}) {
		t.Errorf("applied = %v, want only the latest", applied)
	}
	if g.Latest() != c {
		t.Errorf("latest = %d", g.Latest())
	}
	d := g.Next()
	if g.Current(c) || !g.Current(d) {
		t.Error("a newer request must supersede the old one")
	}
}

func TestBuildState(t *testing.T) {
	p := models.NewPresenter()
	sched := models.NewSchedule()
	song := amazingGrace()
	sched.Add(SongItem(song, SongDeck(song)))
	pass, _ := models.ParseReference("jn 3:16")
	sd := ScriptureDeck(pass, "KJV", []models.Verse{{Book: 43, Chapter: 3, Verse: 16, Text: "For God so loved the world"}})
	sched.Add(ScriptureItem(pass, models.Bible{ID: 1, Abbrev: "KJV"}, sd))
	p.SetPreview(ItemDeck(sched.Items[0]))
	p.GoLive()
	p.Next()
	p.SetPreview(ItemDeck(sched.Items[1]))
	p.ToggleBlack()

	st := BuildState(StateInput{
		Presenter:        p.State,
		Schedule:         sched,
		ScheduleSelected: 1,
		OutputOpen:       true,
		Screens:          1,
		Songs:            SongsState{Count: 223, Query: "amazing", Results: 2, Selected: 0, CountText: "2 songs"},
		Bibles:           []models.Bible{{Abbrev: "KJV"}, {Abbrev: "RVA"}},
		BibleSelected:    0,
		Scripture:        ScriptureState{Reference: "jn 3:16", Verses: 1},
		Import:           ImportState{Done: true, Summary: "223 songs imported"},
		LastError:        "boom",
		Focus:            FocusList,
	})
	if st.Mode != "black" || st.Live.Title != "Amazing Grace" || st.Live.Index != 1 || st.Live.Label != "Verse 2" ||
		st.Live.Count != 3 || !strings.Contains(st.Live.Text, "grace that taught") {
		t.Errorf("live = %+v mode %s", st.Live, st.Mode)
	}
	if st.Preview.Title != "John 3:16" || st.Preview.Index != 0 || !strings.HasPrefix(st.Preview.Text, "For God so loved") {
		t.Errorf("preview = %+v", st.Preview)
	}
	if !reflect.DeepEqual(st.Schedule.Items, []string{"Amazing Grace", "John 3:16"}) || st.Schedule.Live != 0 ||
		!st.Schedule.Dirty || st.Schedule.Title != "Untitled" || st.Schedule.Selected != 1 {
		t.Errorf("schedule = %+v", st.Schedule)
	}
	if st.SongCount != 223 || st.Songs.Results != 2 || !reflect.DeepEqual(st.Bibles, []string{"KJV", "RVA"}) ||
		st.Scripture.Bible != "KJV" || st.LastError != "boom" || !st.Import.Done || st.Focus != "list" || !st.OutputOpen {
		t.Errorf("state = %+v", st)
	}
	// It must marshal, with the documented keys.
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"preview", "live", "mode", "outputOpen", "schedule", "songCount", "bibles", "lastError", "import"} {
		if _, ok := m[key]; !ok {
			t.Errorf("state JSON has no %q", key)
		}
	}
}

func TestBuildStateEmpty(t *testing.T) {
	st := BuildState(StateInput{Presenter: models.NewPresenter().State, ScheduleSelected: -1, BibleSelected: -1})
	if st.Mode != "normal" || st.Live.Index != -1 || st.Live.Title != "" || st.Schedule.Live != -1 ||
		st.Schedule.Items == nil || st.Bibles == nil {
		t.Errorf("state = %+v", st)
	}
}

func TestImportSummary(t *testing.T) {
	got := ImportSummary(easyworship.Report{SongsImported: 223, BiblesImported: []string{"KJV", "RVA"},
		BiblesSkipped: []string{"HCSB (copyrighted)"}})
	want := "223 songs imported; Bibles: KJV, RVA; skipped HCSB (copyrighted)"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestLogoBackground(t *testing.T) {
	dir := t.TempDir()
	if bg := LogoBackground(dir); bg != LogoGradient() {
		t.Errorf("empty dir: %+v", bg)
	}
	if bg := LogoBackground(""); bg != LogoGradient() {
		t.Errorf("no dir: %+v", bg)
	}
	for _, n := range []string{"b.png", "a.jpg", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bg := LogoBackground(dir)
	if bg.Kind != models.BackgroundImage || filepath.Base(bg.Path) != "a.jpg" {
		t.Errorf("bg = %+v", bg)
	}
}

func TestRecentAndExt(t *testing.T) {
	got := AddRecent([]string{"a", "b", "c"}, "b", 3)
	if !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Errorf("recent = %v", got)
	}
	got = AddRecent([]string{"a", "b", "c"}, "d", 3)
	if !reflect.DeepEqual(got, []string{"d", "a", "b"}) {
		t.Errorf("recent = %v", got)
	}
	for in, want := range map[string]string{"x": "x.fws", "x.fws": "x.fws", "x.FWS": "x.FWS", "": ""} {
		if got := WithScheduleExt(in); got != want {
			t.Errorf("WithScheduleExt(%q) = %q", in, got)
		}
	}
	if SongCountText(1) != "1 song" || SongCountText(223) != "223 songs" {
		t.Error("SongCountText")
	}
}
