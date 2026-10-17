package main

import (
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/startup"
)

func main() {
	startup.LoadingScene()
	hello := Label.New()
	hello.AsControl().SetAnchorsPreset(Control.PresetFullRect)
	hello.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	hello.SetVerticalAlignment(GUI.VerticalAlignmentCenter)
	hello.SetText("FreeWorship")
	SceneTree.Add(hello)
	startup.Scene()
}
