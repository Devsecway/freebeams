package store

import (
	"context"
	"reflect"
	"testing"

	"freebeams/internal/models"
)

func TestEnsureBuiltinThemes(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if _, err := st.EnsureDefaultThemes(ctx); err != nil {
		t.Fatal(err)
	}
	// A theme of the same name already there is left alone.
	mine := models.DefaultSongTheme()
	mine.Name = "high contrast"
	if err := st.SaveTheme(ctx, &mine); err != nil {
		t.Fatal(err)
	}
	added, err := st.EnsureBuiltinThemes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Clean Dark", "Warm Light", "Scripture Lower Third"}; !reflect.DeepEqual(added, want) {
		t.Fatalf("added %v, want %v", added, want)
	}
	list, _ := st.ListThemes(ctx)
	if len(list) != 6 {
		t.Fatalf("%d themes: %v", len(list), themeNames(list))
	}
	for _, th := range list {
		if th.Name == "Scripture Lower Third" && !th.Box.Enabled {
			t.Error("the stored lower third lost its band")
		}
	}
	// Deleted afterwards: not brought back.
	for _, th := range list {
		if th.Name == "Clean Dark" {
			if err := st.DeleteTheme(ctx, th.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	added, err = st.EnsureBuiltinThemes(ctx)
	if err != nil || len(added) != 0 {
		t.Fatalf("second run added %v, %v", added, err)
	}
	if n, _ := st.CountThemes(ctx); n != 5 {
		t.Fatalf("%d themes after the second run", n)
	}
}
