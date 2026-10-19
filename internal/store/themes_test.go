package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"freebeams/internal/models"
)

func themeNames(ts []models.Theme) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	return out
}

func TestThemeCRUD(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	th := models.DefaultSongTheme()
	th.Name = "  Sunday  "
	th.Background = models.Background{Kind: models.BackgroundColor, Color: models.Hex(0x991A80)}
	th.Font.Family = "Georgia"
	if err := st.SaveTheme(ctx, &th); err != nil {
		t.Fatal(err)
	}
	if th.ID == 0 || th.Name != "Sunday" {
		t.Fatalf("after insert: id=%d name=%q", th.ID, th.Name)
	}
	got, err := st.GetTheme(ctx, th.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, th) {
		t.Fatalf("GetTheme = %+v\nwant %+v", got, th)
	}

	// Update: the id is kept, the name column follows the theme.
	got.Name = "Sunday Night"
	got.Font.Size = 120
	if err := st.SaveTheme(ctx, &got); err != nil {
		t.Fatal(err)
	}
	again, err := st.GetTheme(ctx, th.ID)
	if err != nil || again.Name != "Sunday Night" || again.Font.Size != 120 || again.ID != th.ID {
		t.Fatalf("after update: %+v, %v", again, err)
	}
	list, err := st.ListThemes(ctx)
	if err != nil || !reflect.DeepEqual(themeNames(list), []string{"Sunday Night"}) {
		t.Fatalf("ListThemes = %v, %v", themeNames(list), err)
	}

	// Validation and missing rows.
	errs := []struct {
		name  string
		theme *models.Theme
		want  error // nil: any error
	}{
		{"nil", nil, nil},
		{"empty name", &models.Theme{Name: "   "}, nil},
		{"missing id", &models.Theme{ID: 9999, Name: "Ghost"}, ErrNotFound},
	}
	for _, tc := range errs {
		t.Run(tc.name, func(t *testing.T) {
			err := st.SaveTheme(ctx, tc.theme)
			if err == nil {
				t.Fatal("SaveTheme succeeded")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := st.GetTheme(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTheme missing: %v", err)
	}
	if n, _ := st.CountThemes(ctx); n != 1 {
		t.Fatalf("CountThemes = %d after failed saves, want 1", n)
	}

	// Delete.
	if err := st.DeleteTheme(ctx, th.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteTheme(ctx, th.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if n, _ := st.CountThemes(ctx); n != 0 {
		t.Fatalf("CountThemes after delete = %d", n)
	}
}

func TestListThemesOrder(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	for _, name := range []string{"zeta", "Alpha", "beta", "alpha"} {
		th := models.Theme{Name: name}
		if err := st.SaveTheme(ctx, &th); err != nil {
			t.Fatal(err)
		}
	}
	list, err := st.ListThemes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Alpha", "alpha", "beta", "zeta"}
	if got := themeNames(list); !reflect.DeepEqual(got, want) {
		t.Fatalf("ListThemes order = %v, want %v", got, want)
	}
}

func TestEnsureDefaultThemes(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	wrote, err := st.EnsureDefaultThemes(ctx)
	if err != nil || !wrote {
		t.Fatalf("first EnsureDefaultThemes = %v, %v", wrote, err)
	}
	list, err := st.ListThemes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := themeNames(list); !reflect.DeepEqual(got, []string{"Default Scripture", "Default Song"}) {
		t.Fatalf("seeded themes = %v", got)
	}
	tests := []struct {
		kind models.ItemKind
		name string
	}{
		{models.ItemSong, "Default Song"},
		{models.ItemScripture, "Default Scripture"},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			id, err := st.DefaultThemeID(ctx, tc.kind)
			if err != nil || id == 0 {
				t.Fatalf("DefaultThemeID = %d, %v", id, err)
			}
			th, err := st.GetDefaultTheme(ctx, tc.kind)
			if err != nil || th.ID != id || th.Name != tc.name {
				t.Fatalf("GetDefaultTheme = %+v, %v (want id %d, %q)", th, err, id, tc.name)
			}
			// The seeded theme is the built-in one, apart from its id.
			builtin := BuiltinTheme(tc.kind)
			builtin.ID = id
			if !reflect.DeepEqual(th, builtin) {
				t.Fatalf("seeded %s theme differs from the built-in:\n%+v\n%+v", tc.kind, th, builtin)
			}
		})
	}

	// A second launch writes nothing.
	wrote, err = st.EnsureDefaultThemes(ctx)
	if err != nil || wrote {
		t.Fatalf("second EnsureDefaultThemes = %v, %v", wrote, err)
	}
	if n, _ := st.CountThemes(ctx); n != 2 {
		t.Fatalf("CountThemes = %d, want 2", n)
	}
}

func TestDefaultThemeSettings(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if _, err := st.EnsureDefaultThemes(ctx); err != nil {
		t.Fatal(err)
	}
	custom := models.DefaultSongTheme()
	custom.Name = "Custom"
	if err := st.SaveTheme(ctx, &custom); err != nil {
		t.Fatal(err)
	}

	if err := st.SetDefaultTheme(ctx, models.ItemSong, custom.ID); err != nil {
		t.Fatal(err)
	}
	if th, err := st.GetDefaultTheme(ctx, models.ItemSong); err != nil || th.Name != "Custom" {
		t.Fatalf("song default = %+v, %v", th, err)
	}
	if th, err := st.GetDefaultTheme(ctx, models.ItemScripture); err != nil || th.Name != "Default Scripture" {
		t.Fatalf("scripture default = %+v, %v", th, err)
	}
	if err := st.SetDefaultTheme(ctx, models.ItemSong, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetDefaultTheme missing: %v", err)
	}
	if err := st.SetDefaultTheme(ctx, models.ItemMedia, custom.ID); err == nil {
		t.Fatal("SetDefaultTheme for media succeeded")
	}

	// Deleting the default theme clears the setting: the built-in comes back
	// (ID 0) until EnsureDefaultThemes re-seeds it.
	if err := st.DeleteTheme(ctx, custom.ID); err != nil {
		t.Fatal(err)
	}
	if id, err := st.DefaultThemeID(ctx, models.ItemSong); err != nil || id != 0 {
		t.Fatalf("DefaultThemeID after delete = %d, %v", id, err)
	}
	th, err := st.GetDefaultTheme(ctx, models.ItemSong)
	if err != nil || th.ID != 0 || th.Name != "Default Song" {
		t.Fatalf("GetDefaultTheme after delete = %+v, %v", th, err)
	}
	wrote, err := st.EnsureDefaultThemes(ctx)
	if err != nil || !wrote {
		t.Fatalf("re-seed = %v, %v", wrote, err)
	}
	th, err = st.GetDefaultTheme(ctx, models.ItemSong)
	if err != nil || th.ID == 0 || th.Name != "Default Song" {
		t.Fatalf("GetDefaultTheme after re-seed = %+v, %v", th, err)
	}
	// The existing "Default Song" row was reused, not duplicated.
	if n, _ := st.CountThemes(ctx); n != 2 {
		t.Fatalf("CountThemes after re-seed = %d, want 2", n)
	}
}
