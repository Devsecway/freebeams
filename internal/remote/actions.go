package remote

import (
	"encoding/json"
	"errors"
	"fmt"
)

// The phone's actions. Each maps to one control action of the app's command
// path (internal/app/control.go and remote.go), with its arguments rebuilt
// from the few fields a phone may send: nothing else of the control API (open
// files, quit, key events, settings) is reachable from the network.

// phoneArgs are the fields a phone may send with an action.
type phoneArgs struct {
	Index *int  `json:"index"`
	Slide *int  `json:"slide"`
	On    *bool `json:"on"`
	ID    int64 `json:"id"`
}

type actionSpec struct {
	control string      // the app's action
	perm    Permissions // what the device needs
	args    func(phoneArgs) (map[string]any, error)
}

var errNeedIndex = errors.New("index is required")

func noArgs(phoneArgs) (map[string]any, error) { return nil, nil }

// onArg passes {on} when given (the mode is set, not toggled, so two phones
// pressing Black at once agree), else toggles.
func onArg(a phoneArgs) (map[string]any, error) {
	if a.On == nil {
		return nil, nil
	}
	return map[string]any{"on": *a.On}, nil
}

func indexArg(a phoneArgs) (map[string]any, error) {
	if a.Index == nil || *a.Index < 0 {
		return nil, errNeedIndex
	}
	return map[string]any{"index": *a.Index}, nil
}

var actions = map[string]actionSpec{
	"next":  {"next", PermControl, noArgs},
	"prev":  {"prev", PermControl, noArgs},
	"first": {"first", PermControl, noArgs},
	"last":  {"last", PermControl, noArgs},
	"black": {"black", PermControl, onArg},
	"clear": {"clear", PermControl, onArg},
	"logo":  {"logo", PermControl, onArg},
	// go-live sends Preview live, on {slide} when given.
	"go-live": {"go-live", PermControl, func(a phoneArgs) (map[string]any, error) {
		if a.Slide == nil {
			return nil, nil
		}
		if *a.Slide < 0 {
			return nil, errors.New("slide must not be negative")
		}
		return map[string]any{"slide": *a.Slide}, nil
	}},
	// preview-item loads schedule row {index} into Preview.
	"preview-item": {"select-schedule", PermControl, indexArg},
	// live-item loads schedule row {index} into Preview and sends it live.
	"live-item": {"remote-live-item", PermControl, indexArg},
	// live-slide shows slide {index} of the live deck.
	"live-slide": {"jump", PermControl, func(a phoneArgs) (map[string]any, error) {
		if a.Index == nil || *a.Index < 0 {
			return nil, errNeedIndex
		}
		return map[string]any{"kind": "slide", "n": *a.Index + 1}, nil
	}},
	// The background audio playlist: play / pause, next and previous track.
	"audio-toggle": {"audio-toggle", PermControl, noArgs},
	"audio-next":   {"audio-next", PermControl, noArgs},
	"audio-prev":   {"audio-prev", PermControl, noArgs},
	// add-song adds library song {id} to the end of the schedule.
	"add-song": {"remote-add-song", PermSchedule, func(a phoneArgs) (map[string]any, error) {
		if a.ID <= 0 {
			return nil, errors.New("id is required")
		}
		return map[string]any{"id": a.ID}, nil
	}},
}

// translate checks a phone action against the device's permissions and
// returns the control action and its JSON body.
func translate(name string, raw json.RawMessage, perm Permissions) (string, json.RawMessage, error) {
	spec, ok := actions[name]
	if !ok {
		return "", nil, fmt.Errorf("unknown action %q", name)
	}
	if !perm.Has(spec.perm) {
		return "", nil, errForbidden
	}
	var a phoneArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &a); err != nil {
			return "", nil, fmt.Errorf("%s: bad arguments: %w", name, err)
		}
	}
	args, err := spec.args(a)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", name, err)
	}
	if args == nil {
		return spec.control, nil, nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", name, err)
	}
	return spec.control, b, nil
}

var errForbidden = errors.New("this device is not allowed to do that")
