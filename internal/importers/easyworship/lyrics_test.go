package easyworship

import (
	"reflect"
	"testing"

	"freebeams/internal/models"
)

func sec(kind models.SectionKind, n int, label string, lines ...string) models.Section {
	return models.Section{Kind: kind, Number: n, Label: label, Lines: lines}
}

func TestParseLabel(t *testing.T) {
	tests := []struct {
		in     string
		ok     bool
		kind   models.SectionKind
		number int
		label  string
	}{
		{"Verse 1", true, models.SectionVerse, 1, "Verse 1"},
		{"verse 12", true, models.SectionVerse, 12, "Verse 12"},
		{"  Chorus 1 ", true, models.SectionChorus, 1, "Chorus 1"},
		{"Chorus", true, models.SectionChorus, 0, "Chorus"},
		{"[Chorus 2]", true, models.SectionChorus, 2, "Chorus 2"},
		{"Refrain:", true, models.SectionChorus, 0, "Chorus"},
		{"Pre-Chorus", true, models.SectionPreChorus, 0, "Pre-Chorus"},
		{"prechorus 2", true, models.SectionPreChorus, 2, "Pre-Chorus 2"},
		{"Pre Chorus", true, models.SectionPreChorus, 0, "Pre-Chorus"},
		{"Bridge 2", true, models.SectionBridge, 2, "Bridge 2"},
		{"Tag", true, models.SectionTag, 0, "Tag"},
		{"Intro", true, models.SectionIntro, 0, "Intro"},
		{"Ending", true, models.SectionEnding, 0, "Ending"},
		{"Outro", true, models.SectionEnding, 0, "Ending"},
		{"V1", true, models.SectionVerse, 1, "Verse 1"},
		{"v 3", true, models.SectionVerse, 3, "Verse 3"},
		{"C", true, models.SectionChorus, 0, "Chorus"},
		{"B", true, models.SectionBridge, 0, "Bridge"},
		{"PC", true, models.SectionPreChorus, 0, "Pre-Chorus"},
		{"Verse 1:", true, models.SectionVerse, 1, "Verse 1"},
		{"Amazing grace how sweet the sound", false, "", 0, ""},
		{"Chorus of angels sing", false, "", 0, ""},
		{"Verses", false, "", 0, ""},
		{"I", false, "", 0, ""},
		{"Verse 123", false, "", 0, ""},
		{"", false, "", 0, ""},
	}
	for _, tc := range tests {
		l, ok := parseLabel(tc.in)
		if ok != tc.ok || l.kind != tc.kind || l.number != tc.number || l.text != tc.label {
			t.Errorf("parseLabel(%q) = %+v, %v; want %s %d %q %v", tc.in, l, ok, tc.kind, tc.number, tc.label, tc.ok)
		}
	}
}

func TestParseLyrics(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []models.Section
	}{
		{"empty", "", nil},
		{"blank only", "\n \n\n", nil},
		{
			"EW labelled",
			"Verse 1\nAmazing grace\nThat saved\n\nChorus 1\nMy chains\n\nVerse 2\n'Twas grace\n",
			[]models.Section{
				sec(models.SectionVerse, 1, "Verse 1", "Amazing grace", "That saved"),
				sec(models.SectionChorus, 1, "Chorus 1", "My chains"),
				sec(models.SectionVerse, 2, "Verse 2", "'Twas grace"),
			},
		},
		{
			"slide break inside labelled section",
			"Chorus 1\nThere's a new name\nWritten down\n\nFor there's a new name\nAnd it's mine\n\nVerse 1\nI was once\n",
			[]models.Section{
				sec(models.SectionChorus, 1, "Chorus 1", "There's a new name", "Written down"),
				sec(models.SectionChorus, 1, "Chorus 1", "For there's a new name", "And it's mine"),
				sec(models.SectionVerse, 1, "Verse 1", "I was once"),
			},
		},
		{
			"unlabelled stanzas",
			"Line one\nLine two\n\n\n  Line three  \r\n\r\nLine four",
			[]models.Section{
				sec(models.SectionVerse, 1, "Verse 1", "Line one", "Line two"),
				sec(models.SectionVerse, 2, "Verse 2", "Line three"),
				sec(models.SectionVerse, 3, "Verse 3", "Line four"),
			},
		},
		{
			"leading unlabelled stanza in labelled song",
			"Intro words\n\nChorus\nSing\n",
			[]models.Section{
				sec(models.SectionVerse, 0, "Verse", "Intro words"),
				sec(models.SectionChorus, 0, "Chorus", "Sing"),
			},
		},
		{
			"label without lines is dropped, label directly after lines",
			"Verse 1\nOne\nChorus\nTwo\nTag\n",
			[]models.Section{
				sec(models.SectionVerse, 1, "Verse 1", "One"),
				sec(models.SectionChorus, 0, "Chorus", "Two"),
			},
		},
		{
			"bracketed tags and nbsp",
			"[V1]\nOne\u00A0 \n[C]\nTwo",
			[]models.Section{
				sec(models.SectionVerse, 1, "Verse 1", "One"),
				sec(models.SectionChorus, 0, "Chorus", "Two"),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLyrics(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseLyrics(%q)\n got %+v\nwant %+v", tc.in, got, tc.want)
			}
		})
	}
}
