package ui

import "graphics.gd/classdb"

// RegisterAll registers every Godot extension class of the ui package. Call it
// once at startup, before building any widget.
func RegisterAll() {
	classdb.Register[toolButton]()
	classdb.Register[miniButton]()
	classdb.Register[divider]()
	classdb.Register[ticker]()
	classdb.Register[searchIcon]()
	classdb.Register[Splitter]()
	classdb.Register[MonitorHost]()
	classdb.Register[tileFlow]()
	classdb.Register[slideTile]()
	classdb.Register[SlideGrid]()
	classdb.Register[scheduleRowView]()
	classdb.Register[ScheduleList]()
	classdb.Register[tabStrip]()
	classdb.Register[tabButton]()
	classdb.Register[ThemePanel]()
	classdb.Register[MainWindow]()
	classdb.Register[demoDriver]()
	classdb.Register[demoThumb]()
}
