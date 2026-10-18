package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"freeworship/internal/models"
	"freeworship/internal/ui/uicore"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/DisplayServer"
	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Image"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/OS"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Viewport"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Rect2i"
	"graphics.gd/variant/Vector2"
	"graphics.gd/variant/Vector2i"
)

// ---- placeholder thumbnail ---------------------------------------------------

// demoThumb is a placeholder slide: a diagonal two-color gradient with
// centered white lyric text scaled to its size. The real app uses render.SlideView.
type demoThumb struct {
	Control.Extension[demoThumb] `gd:"FwDemoThumb"`

	text string
	a, b Color.RGBA
}

func newDemoThumb(text string, bg [2]uint32) Control.Instance {
	t := &demoThumb{text: text, a: uicore.Hex(bg[0]), b: uicore.Hex(bg[1])}
	t.AsControl().SetMouseFilter(Control.MouseFilterIgnore)
	return t.AsControl()
}

func (t *demoThumb) Draw() {
	c := t.AsControl()
	ci := t.AsCanvasItem()
	s := c.Size()
	mid := Color.RGBA{R: (t.a.R + t.b.R) / 2, G: (t.a.G + t.b.G) / 2, B: (t.a.B + t.b.B) / 2, A: 1}
	ci.DrawPolygon([]Vector2.XY{Vector2.New(0, 0), Vector2.New(s.X, 0), Vector2.New(s.X, s.Y), Vector2.New(0, s.Y)},
		[]Color.RGBA{t.a, mid, t.b, mid})
	if t.text == "" || s.Y < 10 {
		return
	}
	f := fwFont(c, "semibold")
	size := max(5, int(s.Y*0.075))
	lines := strings.Split(t.text, "\n")
	lh := float32(f.MoreArgs().GetHeight(size))
	y := (s.Y-lh*float32(len(lines)))/2 + float32(f.MoreArgs().GetAscent(size))
	for _, line := range lines {
		pos := Vector2.New(s.X*0.05, y)
		shadow := Vector2.New(pos.X+1, pos.Y+1)
		for _, p := range []struct {
			at Vector2.XY
			c  Color.RGBA
		}{{shadow, Color.RGBA{A: 0.6}}, {pos, Color.RGBA{R: 1, G: 1, B: 1, A: 1}}} {
			ci.MoreArgs().DrawString(f, p.at, line, GUI.HorizontalAlignmentCenter, s.X*0.9, size, p.c,
				TextServer.JustificationKashida|TextServer.JustificationWordBound, TextServer.DirectionAuto, TextServer.OrientationHorizontal, 0)
		}
		y += lh
	}
}

// ---- sample data ---------------------------------------------------------------

var (
	bgSunset = [2]uint32{0x3a1c71, 0xffaf7b}
	bgOcean  = [2]uint32{0x0b1d3a, 0x3a6073}
)

var demoAmazing = [][2]string{
	{"Verse 1", "Amazing grace! How sweet the sound\nThat saved a wretch like me!\nI once was lost, but now am found;\nWas blind, but now I see."},
	{"Verse 2", "'Twas grace that taught my heart to fear,\nAnd grace my fears relieved;\nHow precious did that grace appear\nThe hour I first believed."},
	{"Verse 3", "Through many dangers, toils and snares,\nI have already come;\n'Tis grace hath brought me safe thus far,\nAnd grace will lead me home."},
	{"Verse 4", "The Lord has promised good to me,\nHis word my hope secures;\nHe will my shield and portion be,\nAs long as life endures."},
	{"Verse 5", "When we've been there ten thousand years,\nBright shining as the sun,\nWe've no less days to sing God's praise\nThan when we'd first begun."},
	{"Ending", "Praise God, praise God,\nPraise God, praise God."},
}

var demoAbide = [][2]string{
	{"Verse 1", "Abide with me; fast falls the eventide;\nThe darkness deepens; Lord with me abide."},
	{"Verse 2", "Swift to its close ebbs out life's little day;\nEarth's joys grow dim; its glories pass away."},
	{"Verse 3", "I need Thy presence every passing hour.\nWhat but Thy grace can foil the tempter's power?"},
	{"Verse 4", "Hold Thou Thy cross before my closing eyes;\nShine through the gloom and point me to the skies."},
}

var demoSongs = []models.SongSummary{
	{ID: 1, Title: "A Heart Like Thine", Author: "Judson Van DeVenter", Copyright: "Public Domain"},
	{ID: 2, Title: "A Mighty Fortress Is Our God", Author: "Hans Leo Hassler, Martin Luther", Copyright: "Public Domain", CCLI: "42964"},
	{ID: 3, Title: "A New Name In Glory", Author: "C. Austin Miles", Copyright: "Public Domain"},
	{ID: 4, Title: "A Robe Of White", Author: "Haldor Lillenas", Copyright: "Public Domain"},
	{ID: 5, Title: "Abide With Me", Author: "Henry F. Lyte, William H. Monk", Copyright: "Public Domain", CCLI: "28158"},
	{ID: 6, Title: "Amazing Grace", Author: "John Newton", Copyright: "Public Domain", CCLI: "22025"},
	{ID: 7, Title: "Blessed Assurance", Author: "Fanny Crosby", Copyright: "Public Domain", CCLI: "22324"},
	{ID: 8, Title: "Come Thou Fount", Author: "Robert Robinson", Copyright: "Public Domain", CCLI: "108389"},
	{ID: 9, Title: "Great Is Thy Faithfulness", Author: "Thomas Chisholm", Copyright: "© 1923 Hope Publishing", CCLI: "18723"},
	{ID: 10, Title: "How Great Thou Art", Author: "Carl Boberg", Copyright: "© 1953 Stuart Hine", CCLI: "14181"},
}

func demoItems(slides [][2]string) []SlideGridItem {
	items := make([]SlideGridItem, len(slides))
	for i, s := range slides {
		items[i] = SlideGridItem{Label: s[0], Number: i + 1}
	}
	return items
}

// fillDemo puts the sample service into a main window.
func fillDemo(w *MainWindow) {
	w.SetScheduleTitle("Sunday Service.fws", true)
	w.Schedule.SetRows([]ScheduleRow{
		{Title: "Welcome loop", Subtitle: "Video · 0:30"},
		{Title: "Abide With Me", Subtitle: "Henry F. Lyte", Live: true},
		{Title: "Amazing Grace", Subtitle: "John Newton"},
		{Title: "John 3:16-18", Subtitle: "KJV · Scripture"},
		{Title: "A Mighty Fortress Is Our God", Subtitle: "Martin Luther"},
		{Title: "Announcements", Subtitle: "Presentation · 6 slides"},
		{Title: "A Robe Of White", Subtitle: "Haldor Lillenas"},
	})
	schedBg := [][2]uint32{{0x1d3557, 0x457b9d}, bgOcean, bgSunset, {0x2c3e50, 0x4ca1af}, {0x42275a, 0x734b6d}, {0x1d3557, 0x457b9d}, {0x283c86, 0x45a247}}
	w.Schedule.SetThumbnailFactory(func(i int) Control.Instance { return newDemoThumb("", schedBg[i]) })
	w.Schedule.SetSelected(2)

	w.PreviewGrid.SetItems(demoItems(demoAmazing))
	w.PreviewGrid.SetThumbnailFactory(func(i int) Control.Instance { return newDemoThumb(demoAmazing[i][1], bgSunset) })
	w.PreviewGrid.SetSelected(0)
	w.LiveGrid.SetItems(demoItems(demoAbide))
	w.LiveGrid.SetThumbnailFactory(func(i int) Control.Instance { return newDemoThumb(demoAbide[i][1], bgOcean) })
	w.LiveGrid.SetSelected(1)
	w.PreviewMonitor.SetContent(newDemoThumb(demoAmazing[0][1], bgSunset))
	w.LiveMonitor.SetContent(newDemoThumb(demoAbide[1][1], bgOcean))

	w.Songs.SetSongs(demoSongs)
	w.Songs.SetSelected(5)
	w.Songs.SetCountText("223 songs")
	w.ResourcePreview.SetContent(newDemoThumb(demoAmazing[0][1], bgSunset))
	w.SetResourcePreviewInfo("Amazing Grace", "Verse 1 · 1 / 6")

	w.Scriptures.SetBibles([]models.Bible{
		{ID: 1, Abbrev: "KJV", Name: "King James Version"},
		{ID: 2, Abbrev: "ASV", Name: "American Standard Version"},
		{ID: 3, Abbrev: "WEB", Name: "World English Bible"},
	}, 0)
	w.Scriptures.SetReferenceText("John 3:16-18")
	w.Scriptures.SetVerses([]string{
		"16  For God so loved the world, that he gave his only begotten Son, that whosoever believeth in him should not perish, but have everlasting life.",
		"17  For God sent not his Son into the world to condemn the world; but that the world through him might be saved.",
		"18  He that believeth on him is not condemned: but he that believeth not is condemned already, because he hath not believed in the name of the only begotten Son of God.",
	}, 0)
	w.Scriptures.SetStatus("John 3:16-18 · 3 verses", false)
	w.Toolbar.SetToggled(ActionLive, true)
}

// ---- demo driver ---------------------------------------------------------------

// demoStep is one scripted action of the demo, run at a time offset.
type demoStep struct {
	at   float64
	name string
	fn   func()
}

// demoDriver runs the scripted demo steps from Process.
type demoDriver struct {
	Node.Extension[demoDriver] `gd:"FwDemoDriver"`

	t     float64
	next  int
	steps []demoStep
}

func (d *demoDriver) Process(delta Float.X) {
	d.t += float64(delta)
	for d.next < len(d.steps) && d.t >= d.steps[d.next].at {
		s := d.steps[d.next]
		d.next++
		if s.name != "" {
			fmt.Println("[demo]", s.name)
		}
		s.fn()
	}
}

// Demo builds a MainWindow (1440 x 900 logical) filled with sample data. When
// the FW_SHOTS environment variable names a directory, it captures screenshots
// of several states (dark, light, scriptures, snapped splitters, context menu,
// song editor) into it and quits.
func Demo() {
	DisplayServer.WindowSetMinSize(Vector2i.New(800, 600), 0)
	root := rootWindow()
	shots := os.Getenv("FW_SHOTS")
	scale := DisplayServer.ScreenGetScale()
	if scale < 1 || (shots != "" && os.Getenv("FW_HIDPI") == "") {
		// Screenshots are taken at exactly 1440 x 900 (a 2x window that large
		// does not fit on a laptop screen).
		scale = 1
	}
	if root != Window.Nil {
		root.SetContentScaleFactor(scale)
	}
	DisplayServer.WindowSetSize(Vector2i.New(int(1440*scale), int(900*scale)), 0)

	w := NewMainWindow(DarkPalette())
	SceneTree.Add(w)
	fillDemo(w)

	d := &demoDriver{}
	d.steps = []demoStep{{0.1, "", func() { w.PreviewGrid.AsControl().GrabFocus() }}}
	if shots != "" {
		OS.SetLowProcessorUsageMode(false)
		snap := func(name string) func() {
			return func() { saveViewport(Viewport.Get(w.AsNode()), filepath.Join(shots, name)) }
		}
		var menuShot func()
		menuShot = func() {
			s := w.Schedule
			s.menuRow = 2
			popupBelow(s.menu, s.views[2].AsControl())
		}
		lyrics := ""
		for _, s := range demoAmazing[:3] {
			lyrics += "[" + s[0] + "]\n" + s[1] + "\n\n"
		}
		d.steps = append(d.steps, []demoStep{
			{3.5, "dark", snap("ui-main-dark.png")},
			{3.6, "light", func() { w.SetPalette(LightPalette()) }},
			{4.4, "", snap("ui-main-light.png")},
			{4.5, "scriptures", func() { w.SetPalette(DarkPalette()); w.Resources.SetTab(TabScriptures) }},
			{5.3, "", snap("ui-scriptures.png")},
			{5.4, "snapped", func() {
				w.Resources.SetTab(TabSongs)
				w.TopSplit.SetCollapsed(CollapseFirst)
				w.MainSplit.SetCollapsed(CollapseSecond)
				w.PreviewSplit.SetCollapsed(CollapseSecond)
			}},
			{6.2, "", snap("ui-splitter-snapped.png")},
			{6.3, "menu", func() {
				w.TopSplit.SetCollapsed(CollapseNone)
				w.MainSplit.SetCollapsed(CollapseNone)
				w.PreviewSplit.SetCollapsed(CollapseNone)
			}},
			{6.8, "", menuShot},
			{7.6, "", func() {
				saveWithPopup(Viewport.Get(w.AsNode()), root, w.Schedule.menu.AsWindow(), filepath.Join(shots, "ui-context-menu.png"))
				w.Schedule.menu.AsWindow().Hide()
			}},
			{7.8, "editor", func() {
				w.Editor.Open(models.Song{ID: 6, Title: "Amazing Grace", Author: "John Newton", Copyright: "Public Domain", CCLI: "22025"}, strings.TrimSpace(lyrics))
			}},
			{8.8, "", func() {
				saveViewport(w.Editor.AsWindow().AsViewport(), filepath.Join(shots, "ui-song-editor.png"))
			}},
			{9.0, "quit", func() { SceneTree.Get(w.AsNode()).Quit() }},
		}...)
	}
	SceneTree.Add(d)
}

func rootWindow() Window.Instance {
	if tree, ok := Object.As[SceneTree.Instance](Engine.GetMainLoop()); ok {
		return tree.Root()
	}
	return Window.Nil
}

// saveViewport writes a viewport's current frame as a PNG.
func saveViewport(vp Viewport.Instance, path string) {
	img := vp.GetTexture().AsTexture2D().GetImage()
	if err := img.SavePng(path); err != nil {
		fmt.Println("[demo] save", path, err)
		return
	}
	fmt.Println("[demo] saved", path, img.GetSize(), "frames", Engine.GetFramesDrawn())
}

// saveWithPopup composites a popup window (a separate OS window) over the main
// viewport at its on-screen position and saves the result.
func saveWithPopup(vp Viewport.Instance, root, popup Window.Instance, path string) {
	img := vp.GetTexture().AsTexture2D().GetImage()
	pop := popup.AsViewport().GetTexture().AsTexture2D().GetImage()
	pop.Convert(img.GetFormat())
	at := popup.Position()
	origin := root.Position()
	size := pop.GetSize()
	img.BlendRect(pop, Rect2i.PositionSize{Size: size}, Vector2i.New(int(at.X-origin.X), int(at.Y-origin.Y)))
	if err := img.SavePng(path); err != nil {
		fmt.Println("[demo] save", path, err)
		return
	}
	fmt.Println("[demo] saved", path, img.GetSize(), "popup at", at, "root at", origin)
}

var _ = Image.FormatRgba8
