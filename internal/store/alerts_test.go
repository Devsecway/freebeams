package store

import (
	"context"
	"errors"
	"testing"

	"freebeams/internal/models"
)

func TestAlertCRUD(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	// Migration 6 seeds three examples, listed by name.
	seeded, err := st.ListAlerts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeded) != 3 || seeded[0].Name != "Parent Needed" || seeded[1].Name != "Service Starting" || seeded[2].Name != "Vehicle" {
		t.Fatalf("seeded alerts: %+v", seeded)
	}
	if v := seeded[2]; v.Message != "The owner of the vehicle with plate {plate}, please move it." || v.Display != models.AlertScroll || v.Count != 3 {
		t.Errorf("Vehicle: %+v", v)
	}
	if s := seeded[1]; s.End != models.AlertEndToken || s.EndToken != models.TokenCountdown || s.Display != models.AlertStatic {
		t.Errorf("Service Starting: %+v", s)
	}

	a := models.Alert{Name: "  Fire drill ", Message: "Please leave by the {exit} exit", Display: models.AlertStatic, Count: 2}
	if err := st.SaveAlert(ctx, &a); err != nil {
		t.Fatal(err)
	}
	if a.ID == 0 || a.Name != "Fire drill" || a.End != models.AlertEndCount || a.Align != models.AlignCenter {
		t.Fatalf("after insert: %+v", a)
	}
	got, err := st.GetAlert(ctx, a.ID)
	if err != nil || got != a {
		t.Fatalf("GetAlert = %+v, %v; want %+v", got, err, a)
	}
	got.Name, got.End, got.Seconds = "Drill", models.AlertEndDuration, 45
	if err := st.SaveAlert(ctx, &got); err != nil {
		t.Fatal(err)
	}
	again, err := st.GetAlert(ctx, a.ID)
	if err != nil || again.Name != "Drill" || again.Seconds != 45 || again.ID != a.ID {
		t.Fatalf("after update: %+v, %v", again, err)
	}

	bad := []struct {
		name string
		a    models.Alert
		nf   bool
	}{
		{"no name", models.Alert{Message: "x"}, false},
		{"no message", models.Alert{Name: "x"}, false},
		{"missing id", models.Alert{ID: 9999, Name: "x", Message: "y"}, true},
	}
	for _, b := range bad {
		err := st.SaveAlert(ctx, &b.a)
		if err == nil || errors.Is(err, ErrNotFound) != b.nf {
			t.Errorf("%s: SaveAlert error %v", b.name, err)
		}
	}
	if err := st.SaveAlert(ctx, nil); err == nil {
		t.Error("SaveAlert(nil) succeeded")
	}

	if err := st.DeleteAlert(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAlert(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
	if _, err := st.GetAlert(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAlert after delete: %v", err)
	}
	if list, _ := st.ListAlerts(ctx); len(list) != 3 {
		t.Errorf("after delete: %d alerts", len(list))
	}
}
