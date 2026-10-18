package app

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"freeworship/internal/control"

	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventMouseButton"
	"graphics.gd/classdb/InputEventMouseMotion"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// InputLogEnv names a file that receives every mouse event the main window
// gets, one line per event. It is a debugging aid: a crash a person can
// trigger by hand is recorded, then replayed exactly with the replay-input
// control action.
const InputLogEnv = "FREEWORSHIP_INPUT_LOG"

// inputRecorder writes mouse events as
// "<frame> <kind> <x> <y> <buttonMask> <button> <pressed> <double>" in logical
// (content-scale independent) coordinates.
type inputRecorder struct {
	f     *os.File
	w     *bufio.Writer
	start uint64
}

func openInputRecorder() *inputRecorder {
	path := os.Getenv(InputLogEnv)
	if path == "" {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		log.Printf("[freeworship] input log: %v", err)
		return nil
	}
	log.Printf("[freeworship] recording mouse input to %s", path)
	return &inputRecorder{f: f, w: bufio.NewWriter(f), start: uint64(Engine.GetProcessFrames())}
}

func (r *inputRecorder) record(ev InputEvent.Instance) {
	if r == nil {
		return
	}
	frame := uint64(Engine.GetProcessFrames()) - r.start
	if mb, ok := Object.As[InputEventMouseButton.Instance](ev); ok {
		m := mb.AsInputEventMouse()
		p := m.GlobalPosition()
		fmt.Fprintf(r.w, "%d button %.1f %.1f %d %d %t %t\n", frame, p.X, p.Y, m.ButtonMask(), mb.ButtonIndex(),
			mb.AsInputEvent().IsPressed(), mb.DoubleClick())
	} else if mm, ok := Object.As[InputEventMouseMotion.Instance](ev); ok {
		m := mm.AsInputEventMouse()
		p := m.GlobalPosition()
		fmt.Fprintf(r.w, "%d motion %.1f %.1f %d 0 false false\n", frame, p.X, p.Y, m.ButtonMask())
	} else {
		return
	}
	r.w.Flush() // the point is surviving a crash
}

func (r *inputRecorder) close() {
	if r == nil {
		return
	}
	r.w.Flush()
	r.f.Close()
}

// replayInput re-injects a recorded input log through Godot's input pipeline,
// at the recorded frame offsets, and answers when the last event is sent.
func (a *App) replayInput(cmd *control.Command, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		cmd.RespondErr(fmt.Errorf("replay-input: %w", err))
		return
	}
	scale := Window.Get(a.AsNode()).ContentScaleFactor()
	last := 0
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) != 8 {
			continue
		}
		frame, err1 := strconv.Atoi(f[0])
		x, err2 := strconv.ParseFloat(f[2], 32)
		y, err3 := strconv.ParseFloat(f[3], 32)
		mask, err4 := strconv.Atoi(f[4])
		button, err5 := strconv.Atoi(f[5])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
			continue
		}
		kind, pressed, double := f[1], f[6] == "true", f[7] == "true"
		pos := Vector2.New(float32(x)*scale, float32(y)*scale)
		a.after(frame+1, func() {
			switch kind {
			case "button":
				ev := InputEventMouseButton.New()
				ev.SetButtonIndex(Input.MouseButton(button))
				ev.SetPressed(pressed)
				ev.SetDoubleClick(double)
				ev.AsInputEventMouse().SetButtonMask(Input.MouseButtonMask(mask))
				ev.AsInputEventMouse().SetPosition(pos)
				ev.AsInputEventMouse().SetGlobalPosition(pos)
				Input.ParseInputEvent(ev.AsInputEvent())
			case "motion":
				ev := InputEventMouseMotion.New()
				ev.AsInputEventMouse().SetButtonMask(Input.MouseButtonMask(mask))
				ev.AsInputEventMouse().SetPosition(pos)
				ev.AsInputEventMouse().SetGlobalPosition(pos)
				Input.ParseInputEvent(ev.AsInputEvent())
			}
		})
		last = max(last, frame)
		n++
	}
	log.Printf("[freeworship] replaying %d input events over %d frames", n, last)
	a.after(last+1+nativeKeyFrames, func() { a.answer(cmd, nil) })
}
