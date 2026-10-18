package ui

import (
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Viewport"
)

// graphics.gd buffers engine calls that return nothing and flushes the buffer
// before the next call that returns a value (or at the end of the frame). Its
// _gui_input / _input virtual wrappers do not flush when the Go handler
// returns (only _draw does), so a bare AcceptEvent / SetInputAsHandled at the
// end of a handler reaches Godot after it has already propagated the event:
// an arrow key a grid "accepted" still moves keyboard focus to the
// neighbouring control. These helpers follow the call with a cheap getter,
// which flushes the buffer while Godot is still waiting on the handler.

// acceptEvent marks the event being handled as accepted, immediately.
func acceptEvent(c Control.Instance) {
	c.AcceptEvent()
	_ = c.AsNode().IsInsideTree() // flush
}

// setInputHandled marks the viewport's current input as handled, immediately.
func setInputHandled(vp Viewport.Instance) {
	vp.SetInputAsHandled()
	_ = vp.IsInputHandled() // flush
}
