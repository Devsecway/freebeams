package models

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func titles(p Playlist) string {
	var out []string
	for _, t := range p.Tracks {
		out = append(out, t.Title)
	}
	return strings.Join(out, ",")
}

func TestPlaylistEdits(t *testing.T) {
	var p Playlist
	if got := p.Insert(-1, PlaylistTrack{Path: "/m/a.mp3"}); got != 0 {
		t.Fatalf("insert a: %d", got)
	}
	p.Insert(-1, PlaylistTrack{Path: "/m/b_song.mp3"})
	p.Insert(0, PlaylistTrack{Path: "/m/c.wav", Title: "C"})
	p.Insert(99, PlaylistTrack{Path: "/m/d.m4a", Title: "D"})
	if got := titles(p); got != "C,a,b song,D" {
		t.Fatalf("order after inserts: %s", got)
	}
	for _, tr := range p.Tracks {
		if tr.ID == "" {
			t.Fatalf("track %q has no id", tr.Title)
		}
	}
	// A repeated id is replaced on insert.
	dup := p.Tracks[0]
	p.Insert(-1, dup)
	if p.Tracks[4].ID == dup.ID {
		t.Fatal("a repeated id was kept")
	}
	if !p.Remove(4) || p.Remove(4) || p.Remove(-1) {
		t.Fatal("remove bounds")
	}
	cases := []struct {
		from, to int
		ok       bool
		want     string
	}{
		{0, 3, true, "a,b song,D,C"},
		{3, 0, true, "C,a,b song,D"},
		{1, 1, true, "C,a,b song,D"},
		{2, 1, true, "C,b song,a,D"},
		{0, 4, false, "C,b song,a,D"},
		{-1, 0, false, "C,b song,a,D"},
	}
	for _, c := range cases {
		if ok := p.Move(c.from, c.to); ok != c.ok || titles(p) != c.want {
			t.Errorf("Move(%d, %d) = %v, %s; want %v, %s", c.from, c.to, ok, titles(p), c.ok, c.want)
		}
	}
	id := p.Tracks[2].ID
	if p.IndexOf(id) != 2 || p.IndexOf("") != -1 || p.IndexOf("nope") != -1 {
		t.Fatal("IndexOf")
	}
	if tr, ok := p.Track(id); !ok || tr.Title != "a" {
		t.Fatal("Track")
	}
}

func TestRepeatModes(t *testing.T) {
	if RepeatMode("").Normalized() != RepeatOff || RepeatMode("x").Normalized() != RepeatOff || RepeatOne.Normalized() != RepeatOne {
		t.Fatal("Normalized")
	}
	m := RepeatOff
	var seen []string
	for range 4 {
		m = m.Next()
		seen = append(seen, string(m))
	}
	if strings.Join(seen, ",") != "all,one,off,all" {
		t.Fatalf("cycle: %v", seen)
	}
	for in, want := range map[string]RepeatMode{"": RepeatOff, "OFF": RepeatOff, " all ": RepeatAll, "one": RepeatOne} {
		if got, err := ParseRepeatMode(in); err != nil || got != want {
			t.Errorf("ParseRepeatMode(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseRepeatMode("twice"); err == nil {
		t.Error("ParseRepeatMode accepted twice")
	}
}

func TestPlaylistNormalize(t *testing.T) {
	p := Playlist{Repeat: "forever", FadeOut: 99, Tracks: []PlaylistTrack{
		{ID: "x", Path: "/a.mp3", Title: "A"},
		{ID: "x", Path: "/b.mp3"},
		{ID: "", Path: "", Title: "no file"},
		{Path: "/c.mp3", Title: "C"},
	}}
	if !p.Normalize() {
		t.Fatal("Normalize reported no change")
	}
	if p.Repeat != RepeatOff || p.FadeOut != MaxFadeOut || titles(p) != "A,b,C" {
		t.Fatalf("normalized: %+v", p)
	}
	if p.Tracks[0].ID != "x" || p.Tracks[1].ID == "x" || p.Tracks[2].ID == "" {
		t.Fatalf("ids: %+v", p.Tracks)
	}
	if p.Normalize() {
		t.Fatal("a normalized playlist changed again")
	}
	if ClampFadeOut(math.NaN()) != 0 || ClampFadeOut(-1) != 0 || ClampFadeOut(2.5) != 2.5 {
		t.Fatal("ClampFadeOut")
	}
}

func TestTrackFromMedia(t *testing.T) {
	tr := TrackFromMedia(Media{ID: 7, Path: "/m/walk_in.mp3", Kind: MediaAudio, Duration: 90 * time.Second})
	if tr.ID == "" || tr.MediaID != 7 || tr.Title != "walk in" || tr.Seconds != 90 {
		t.Fatalf("%+v", tr)
	}
}

func TestSchedulePlaylistRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// An empty playlist is left out of the file.
	s := NewSchedule()
	path := filepath.Join(dir, "plain.fbs")
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "playlist") {
		t.Fatalf("an empty playlist was written:\n%s", data)
	}

	// A schedule from before playlists opens with an empty one.
	old := filepath.Join(dir, "old.fws")
	if err := os.WriteFile(old, []byte(`{"version":1,"title":"Old","items":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSchedule(old)
	if err != nil || len(got.Playlist.Tracks) != 0 || got.Playlist.Repeat.Normalized() != RepeatOff {
		t.Fatalf("old schedule: %+v, %v", got, err)
	}

	// The playlist and its settings round-trip; editing marks dirty.
	if !s.EditPlaylist(func(p *Playlist) bool {
		p.Insert(-1, PlaylistTrack{Path: "/m/one.mp3", Title: "One", MediaID: 3, Seconds: 61})
		p.Insert(-1, PlaylistTrack{Path: "/m/two.wav"})
		p.Repeat, p.Shuffle, p.FadeOut = RepeatAll, true, 3
		return true
	}) || !s.Dirty {
		t.Fatal("EditPlaylist did not mark the schedule dirty")
	}
	if s.EditPlaylist(func(*Playlist) bool { return false }) {
		t.Fatal("a no-op edit reported a change")
	}
	path = filepath.Join(dir, "music.fbs")
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err = LoadSchedule(path)
	if err != nil {
		t.Fatal(err)
	}
	p := got.Playlist
	if titles(p) != "One,two" || p.Repeat != RepeatAll || !p.Shuffle || p.FadeOut != 3 || p.Tracks[0].MediaID != 3 ||
		p.Tracks[0].Seconds != 61 || p.Tracks[0].ID != s.Playlist.Tracks[0].ID || got.Dirty {
		t.Fatalf("round trip: %+v", got)
	}
}
