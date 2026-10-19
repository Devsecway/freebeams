package core

import (
	"encoding/json"
	"reflect"
	"testing"

	"freeworship/internal/models"
)

func TestLayoutRoundTrip(t *testing.T) {
	in := Layout{Splitters: map[string]SplitPos{
		"main":     {Percent: 0.71},
		"resource": {Percent: 0.8542, Collapsed: 2},
	}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Layout
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip: got %+v, want %+v", out, in)
	}
}

func TestBuildStateSplitters(t *testing.T) {
	st := BuildState(StateInput{Presenter: models.NewPresenter().State})
	if st.Splitters == nil || st.Scripture.CopyrightedAvailable == nil {
		t.Fatalf("nil maps/slices in state: %+v", st)
	}
	pos := map[string]SplitPos{"top": {Percent: 0.1944, Collapsed: 1}}
	st = BuildState(StateInput{Presenter: models.NewPresenter().State, Splitters: pos})
	if !reflect.DeepEqual(st.Splitters, pos) {
		t.Errorf("splitters = %+v, want %+v", st.Splitters, pos)
	}
}
