// FreeWorship: a native worship presentation app (an EasyWorship clone).
package main

import (
	"math"

	"freeworship/internal/app"
	"freeworship/internal/render"
	"freeworship/internal/ui"

	"graphics.gd/classdb/DisplayServer"
	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/classdb/Window"
	"graphics.gd/startup"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2i"
)

// Logical window geometry (multiplied by the screen scale on HiDPI displays).
const (
	windowW, windowH       = 1440, 900
	windowMinW, windowMinH = 1000, 640
)

func main() {
	startup.LoadingScene()
	ui.RegisterAll()
	render.RegisterAll()
	app.RegisterAll()
	setupWindow()
	SceneTree.Add(app.New())
	startup.Scene()
}

// setupWindow sizes and titles the operator window and sets the HiDPI
// content scale.
func setupWindow() {
	scale := float64(DisplayServer.ScreenGetScale())
	if scale < 1 {
		scale = 1
	}
	px := func(v int) int { return int(math.Round(float64(v) * scale)) }
	w, h := px(windowW), px(windowH)
	// Never larger than the usable screen area (menu bar and Dock excluded).
	usable := DisplayServer.ScreenGetUsableRect()
	if uw := int(usable.Size.X); uw > 0 && w > uw {
		w = uw
	}
	if uh := int(usable.Size.Y); uh > 0 && h > uh {
		h = uh
	}
	DisplayServer.WindowSetMinSize(Vector2i.New(min(px(windowMinW), w), min(px(windowMinH), h)), 0)
	DisplayServer.WindowSetSize(Vector2i.New(w, h), 0)
	if usable.Size.X > 0 && usable.Size.Y > 0 {
		x := int(usable.Position.X) + (int(usable.Size.X)-w)/2
		y := int(usable.Position.Y) + (int(usable.Size.Y)-h)/2
		DisplayServer.WindowSetPosition(Vector2i.New(x, y), 0)
	}
	if tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop()); ok {
		if root := tree.Root(); root != Window.Nil {
			root.SetTitle("FreeWorship")
			root.SetContentScaleFactor(DisplayServer.ScreenGetScale())
		}
	}
}
