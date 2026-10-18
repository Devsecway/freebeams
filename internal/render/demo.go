package render

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"freeworship/internal/models"

	"graphics.gd/classdb"
	"graphics.gd/classdb/ColorRect"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/HBoxContainer"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/MarginContainer"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/classdb/VBoxContainer"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// Demo adds a render test bench to the scene: thumbnails, monitors and the
// output window showing lyrics, an auto-shrinking verse, image and gradient
// backgrounds, footers and the Clear/Black/Logo modes. Call it from main after
// startup.LoadingScene() and RegisterAll(), before startup.Scene().
//
// With FW_RENDER_DEMO_OUT=<dir> set it saves screenshots (render-main.png,
// render-output.png, a mid-crossfade capture and the settled result) into dir
// and quits. Otherwise it keeps running and steps the live slide every 3s.
func Demo() {
	classdb.Register[renderDemo]()
	d := new(renderDemo)
	SceneTree.Add(d.AsNode())
}

type renderDemo struct {
	Node.Extension[renderDemo] `gd:"FwRenderDemo"`

	out     *Output
	live    *SlideView
	frames  []models.Frame
	idx     int
	seq     uint64
	elapsed float64
	step    int
	outDir  string
	next    float64
}

const (
	demoLyric = "Amazing grace, how sweet the sound\nThat saved a wretch like me\nI once was lost, but now am found\nWas blind, but now I see"
	demoVerse = "For God so loved the world, that he gave his only begotten Son, that whosoever believeth in him should not perish, but have everlasting life. " +
		"For God sent not his Son into the world to condemn the world; but that the world through him might be saved. " +
		"He that believeth on him is not condemned: but he that believeth not is condemned already, because he hath not believed in the name of the only begotten Son of God. " +
		"And this is the condemnation, that light is come into the world, and men loved darkness rather than light, because their deeds were evil."
	demoPsalm = "The LORD is my shepherd; I shall not want. He maketh me to lie down in green pastures: he leadeth me beside the still waters. " +
		"He restoreth my soul: he leadeth me in the paths of righteousness for his name's sake. " +
		"Yea, though I walk through the valley of the shadow of death, I will fear no evil: for thou art with me; thy rod and thy staff they comfort me. " +
		"Thou preparest a table before me in the presence of mine enemies: thou anointest my head with oil; my cup runneth over. " +
		"Surely goodness and mercy shall follow me all the days of my life: and I will dwell in the house of the LORD for ever."
	demoChorus = "My chains are gone\nI've been set free\nMy God, my Savior has ransomed me"
)

func demoImages() []string {
	var out []string
	for _, pat := range []string{"*.jpg", "*.png"} {
		m, _ := filepath.Glob(filepath.Join("/Users/Shared/Softouch/Easyworship/Default/Resources/Images", pat))
		out = append(out, m...)
	}
	sort.Strings(out)
	return out
}

func (d *renderDemo) frame(mode models.OutputMode, s *models.Slide, th models.Theme) models.Frame {
	d.seq++
	return models.Frame{Seq: d.seq, Mode: mode, Slide: s, Theme: th, Logo: d.logo()}
}

func (d *renderDemo) logo() models.Background {
	imgs := demoImages()
	if len(imgs) > 1 {
		return models.Background{Kind: models.BackgroundImage, Path: imgs[1], Color: models.Hex(0x202020)}
	}
	return models.Background{Kind: models.BackgroundGradient, Color: models.Hex(0x3A1C71), Color2: models.Hex(0xD76D77)}
}

func demoLabel(text string) Label.Instance {
	l := Label.New()
	l.SetText(text)
	l.AsControl().AddThemeColorOverride("font_color", Color.RGBA{R: 0.8, G: 0.8, B: 0.8, A: 1})
	l.AsControl().AddThemeFontSizeOverride("font_size", 13)
	return l
}

func demoView(parent Node.Instance, w, h float64, f models.Frame, fade bool) *SlideView {
	v := NewSlideView()
	if !fade {
		v.SetTransitionDuration(0)
	}
	v.AsControl().SetCustomMinimumSize(Vector2.New(w, h))
	parent.AddChild(v.AsNode())
	v.SetFrame(f)
	return v
}

func (d *renderDemo) Ready() {
	d.outDir = os.Getenv("FW_RENDER_DEMO_OUT")
	song := models.DefaultSongTheme()
	scr := models.DefaultScriptureTheme()
	imgs := demoImages()
	imgBg := &models.Background{Kind: models.BackgroundGradient, Color: models.Hex(0x003300), Color2: models.Hex(0x001100)}
	if len(imgs) > 0 {
		imgBg = &models.Background{Kind: models.BackgroundImage, Path: imgs[0], Color: models.Hex(0x333333)}
	}
	lyric := &models.Slide{Kind: models.SlideSong, Label: "Verse 1", Text: demoLyric, Footer: "Amazing Grace · John Newton · Public Domain"}
	verse := &models.Slide{Kind: models.SlideScripture, Label: "John 3:16-19", Text: demoVerse, Footer: "John 3:16-19 (KJV)"}
	imgSlide := &models.Slide{Kind: models.SlideSong, Label: "Chorus", Text: demoChorus, Background: imgBg}
	missing := &models.Slide{Kind: models.SlideSong, Label: "Missing image", Text: "Missing image falls back to color",
		Background: &models.Background{Kind: models.BackgroundImage, Path: "/nonexistent.jpg", Color: models.Hex(0x552222)}}
	psalm := &models.Slide{Kind: models.SlideScripture, Label: "Psalm 23", Text: demoPsalm, Footer: "Psalm 23:1-6 (KJV)"}
	leftTop := song
	leftTop.HAlign, leftTop.VAlign = models.AlignLeft, models.AlignTop

	bg := ColorRect.New()
	bg.SetColor(Color.RGBA{R: 0.13, G: 0.13, B: 0.14, A: 1})
	bg.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	d.AsNode().AddChild(bg.AsNode())

	margin := MarginContainer.New()
	margin.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	for _, k := range []string{"margin_left", "margin_top", "margin_right", "margin_bottom"} {
		margin.AsControl().AddThemeConstantOverride(k, 16)
	}
	d.AsNode().AddChild(margin.AsNode())
	col := VBoxContainer.New()
	col.AsControl().AddThemeConstantOverride("separation", 8)
	margin.AsNode().AddChild(col.AsNode())

	row := func(title string) Node.Instance {
		col.AsNode().AddChild(demoLabel(title).AsNode())
		r := HBoxContainer.New()
		r.AsControl().AddThemeConstantOverride("separation", 10)
		col.AsNode().AddChild(r.AsNode())
		return r.AsNode()
	}

	r1 := row("Thumbnails 160×90: lyric/gradient · long verse (auto-fit) · image · missing image · clear · black · logo · left/top")
	demoView(r1, 160, 90, d.frame(models.ModeNormal, lyric, song), false)
	demoView(r1, 160, 90, d.frame(models.ModeNormal, verse, scr), false)
	demoView(r1, 160, 90, d.frame(models.ModeNormal, imgSlide, song), false)
	demoView(r1, 160, 90, d.frame(models.ModeNormal, missing, song), false)
	demoView(r1, 160, 90, d.frame(models.ModeClear, lyric, song), false)
	demoView(r1, 160, 90, d.frame(models.ModeBlack, lyric, song), false)
	demoView(r1, 160, 90, d.frame(models.ModeLogo, lyric, song), false)
	demoView(r1, 160, 90, d.frame(models.ModeNormal, lyric, leftTop), false)

	r2 := row("Monitors 400×225: lyric + footer · long verse + footer · image background (live, crossfades)")
	demoView(r2, 400, 225, d.frame(models.ModeNormal, lyric, song), false)
	demoView(r2, 400, 225, d.frame(models.ModeNormal, verse, scr), false)
	d.live = demoView(r2, 400, 225, d.frame(models.ModeNormal, imgSlide, song), true)

	r3 := row("640×360 verse · 300×300 letterbox · 160×200 pillar")
	demoView(r3, 640, 360, d.frame(models.ModeNormal, verse, scr), false)
	demoView(r3, 300, 300, d.frame(models.ModeNormal, lyric, song), false)
	demoView(r3, 160, 200, d.frame(models.ModeNormal, imgSlide, song), false)

	d.frames = []models.Frame{
		d.frame(models.ModeNormal, imgSlide, song),
		d.frame(models.ModeNormal, lyric, song),
		d.frame(models.ModeNormal, psalm, scr), // fitted for the first time on air
		d.frame(models.ModeClear, verse, scr),
		d.frame(models.ModeLogo, verse, scr),
		d.frame(models.ModeBlack, verse, scr),
	}

	d.out = NewOutput(d.AsNode())
	d.out.OnClosed = func() { fmt.Println("[render demo] output closed by user") }
	d.out.SetFrame(d.frames[0])
	d.out.Open(DefaultOutputScreen())
	d.next = 3
}

func (d *renderDemo) save(name string, png []byte, err error) {
	if err != nil {
		fmt.Println("[render demo]", name, "error:", err)
		return
	}
	p := filepath.Join(d.outDir, name)
	if err := os.WriteFile(p, png, 0o644); err != nil {
		fmt.Println("[render demo] write", p, err)
		return
	}
	fmt.Println("[render demo] wrote", p)
}

func (d *renderDemo) saveMain(name string) {
	png, err := WindowScreenshot(SceneTree.Get(d.AsNode()).Root())
	d.save(name, png, err)
}

func (d *renderDemo) advance() {
	d.idx = (d.idx + 1) % len(d.frames)
	f := d.frames[d.idx]
	d.seq++
	f.Seq = d.seq
	d.out.SetFrame(f)
	d.live.SetFrame(f)
}

func (d *renderDemo) Process(delta Float.X) {
	d.elapsed += float64(delta)
	if d.outDir == "" {
		if d.elapsed >= d.next {
			d.next = d.elapsed + 3
			d.advance()
		}
		return
	}
	shot := func(name string) {
		png, err := d.out.Screenshot()
		d.save(name, png, err)
	}
	view := d.out.View()
	switch {
	case d.step == 0 && d.elapsed >= 1.5:
		d.step++
		d.saveMain("render-main.png")
		shot("render-output.png")
		d.advance() // image chorus -> lyric with footer (crossfade)
	case d.step == 1 && view.TransitionProgress() >= 0.4:
		// The viewport texture holds the previous frame, drawn mid-fade.
		d.step++
		fmt.Printf("[render demo] mid-transition progress=%.2f animating=%d\n", view.TransitionProgress(), Animating())
		shot("render-output-mid.png")
		d.saveMain("render-main-mid.png")
	case d.step == 2 && !view.Transitioning():
		d.step++
		d.next = d.elapsed + 0.3
	case d.step == 3 && d.elapsed >= d.next:
		d.step++
		fmt.Println("[render demo] settled: animating =", Animating())
		shot("render-output-lyric.png")
		d.advance() // -> Psalm 23, auto-fitted now (measurement objects reused frames later)
	case d.step == 4 && !view.Transitioning():
		d.step++
		d.next = d.elapsed + 0.3
	case d.step == 5 && d.elapsed >= d.next:
		d.step++
		shot("render-output-verse.png")
		d.out.Close()
		fmt.Println("[render demo] closed:", !d.out.IsOpen(), "animating:", Animating())
		d.next = d.elapsed + 0.3
	case d.step == 6 && d.elapsed >= d.next:
		d.step++
		ReleaseCaches()
		SceneTree.Get(d.AsNode()).Quit()
	}
}
