package ui

import (
	"strconv"

	"freebeams/internal/app/core"

	"graphics.gd/classdb/BaseButton"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/HSlider"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/TextureRect"
	"graphics.gd/classdb/Tree"
	"graphics.gd/classdb/VBoxContainer"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// AudioPlaylist is the schedule's background audio panel, under the schedule
// list (docs/MEDIA.md "Audio playlist"): a header with the add and settings
// menus, the track list (the current track marked ▶ / ❚❚, missing files
// dimmed) and its own transport: the track title and time, a seek slider,
// previous / play-pause / next / stop, repeat (off → all → one) and shuffle,
// and the volume. It plays independently of the Live slide, so it is not
// part of the output monitors' transport bars.
//
// It is a dumb view: SetRows / SetState project the app's state (every frame
// while a track plays) and the On… callbacks report the operator's intent.
type AudioPlaylist struct {
	OnPlayPause   func()
	OnStop        func()
	OnNext        func()
	OnPrev        func()
	OnSeek        func(frac float64) // fraction of the track
	OnVolume      func(v float64)    // 0..1
	OnRepeat      func()             // the repeat button: the next mode
	OnShuffle     func(on bool)
	OnPlayTrack   func(i int) // double-click / Enter / Play Track
	OnRemove      func(i int)
	OnMove        func(from, to int)
	OnAddSelected func()             // + ▸ Add Selected Audio (the Media tab's selection)
	OnAddFiles    func()             // + ▸ Add Audio Files…
	OnClear       func()             // ⚙ ▸ Clear Playlist
	OnFadeOut     func(secs float64) // ⚙ ▸ Fade Out on Stop ▸

	root    VBoxContainer.Instance
	list    Tree.Instance
	empty   Label.Instance
	hint    Control.Instance   // the empty label with its margins
	menu    PopupMenu.Instance // a row's context menu
	addMenu PopupMenu.Instance
	gear    PopupMenu.Instance
	fade    *radioMenu

	title   Label.Instance
	time    Label.Instance
	seek    HSlider.Instance
	prev    *miniButton
	play    *transportButton
	next    *miniButton
	stop    *miniButton
	repeat  *miniButton
	shuffle *miniButton
	volIcon TextureRect.Instance
	volume  HSlider.Instance

	rows    []PlaylistRow
	state   PlaylistState
	menuRow int
	seeking bool
	built   bool
}

// PlaylistRow is one track of the list.
type PlaylistRow struct {
	Title   string
	Time    string // "3:05" ("" unknown)
	Marker  string // the current track's icon: "play" (playing), "pause" (paused), "" none
	Missing bool   // the file is gone or could not be played: dimmed
}

// PlaylistState is what the transport shows.
type PlaylistState struct {
	Title    string // the current (or cued) track
	Playing  bool
	Stopped  bool
	Position float64 // seconds
	Duration float64 // seconds (0 unknown)
	Volume   float64
	Repeat   string // off | all | one
	Shuffle  bool
	FadeOut  float64 // seconds, 0 off
	Enabled  bool    // the playlist has a track
	CanAdd   bool    // the Media tab's selection is an audio file
}

// Context, add and settings menu ids.
const (
	plMenuPlay = iota
	plMenuUp
	plMenuDown
	plMenuRemove
	plMenuAddSelected
	plMenuAddFiles
	plMenuClear
	plMenuFade
)

// fadeChoices are the Fade Out on Stop ▸ lengths (seconds).
var fadeChoices = []choice{{"0", "Off"}, {"2", "2 Seconds"}, {"5", "5 Seconds"}, {"10", "10 Seconds"}}

// playlistListH is the track list's height: four rows.
const playlistListH = 110

// NewAudioPlaylist builds an empty panel.
func NewAudioPlaylist() *AudioPlaylist {
	p := &AudioPlaylist{menuRow: -1}
	p.root = vbox(0)

	// Header: Audio Playlist   [+▾] | [⚙▾]
	plus := newMiniButton(miniDropdown, "plus", 40, 24, "Add Audio")
	p.addMenu = PopupMenu.New()
	p.addMenu.MoreArgs().AddItem("Add Selected Audio", plMenuAddSelected, 0)
	p.addMenu.MoreArgs().AddItem("Add Audio Files…", plMenuAddFiles, 0)
	p.addMenu.OnIdPressed(p.menuPressed)
	plus.AsNode().AddChild(p.addMenu.AsNode())
	plus.AsBaseButton().OnPressed(func() {
		if idx := p.addMenu.GetItemIndex(plMenuAddSelected); idx >= 0 {
			p.addMenu.SetItemDisabled(idx, !p.state.CanAdd)
		}
		popupBelow(p.addMenu, plus.AsControl())
	})
	gearBtn := newMiniButton(miniDropdown, "gear", 40, 24, "Playlist Actions")
	p.gear = PopupMenu.New()
	p.fade = newRadioMenu(p.gear, "FadeOut", "Fade Out on Stop", plMenuFade, fadeChoices, func(v string) {
		if secs, err := strconv.ParseFloat(v, 64); err == nil && p.OnFadeOut != nil {
			p.OnFadeOut(secs)
		}
	})
	p.gear.AddSeparator()
	p.gear.MoreArgs().AddItem("Remove Track", plMenuRemove, 0)
	p.gear.MoreArgs().AddItem("Clear Playlist", plMenuClear, 0)
	p.gear.OnIdPressed(func(id int) {
		p.menuRow = p.selectedRow()
		p.menuPressed(id)
	})
	gearBtn.AsNode().AddChild(p.gear.AsNode())
	gearBtn.AsBaseButton().OnPressed(func() {
		for idx := 0; idx < p.gear.ItemCount(); idx++ {
			switch p.gear.GetItemId(idx) {
			case plMenuRemove:
				p.gear.SetItemDisabled(idx, p.selectedRow() < 0)
			case plMenuClear:
				p.gear.SetItemDisabled(idx, len(p.rows) == 0)
			}
		}
		popupBelow(p.gear, gearBtn.AsControl())
	})
	header := panelHeader("Audio Playlist", nil, []Control.Instance{
		plus.AsControl(), withMargins(newDivider(true).AsControl(), 2, 2), gearBtn.AsControl(),
	})

	// The track list (or a hint while it is empty).
	body := Control.New()
	body.SetCustomMinimumSize(Vector2.New(0, playlistListH))
	body.SetClipContents(true)
	p.list = Tree.New()
	p.list.SetColumns(2)
	p.list.SetHideRoot(true)
	p.list.SetSelectMode(Tree.SelectRow)
	p.list.SetAllowRmbSelect(true)
	p.list.SetColumnExpand(1, false)
	p.list.SetColumnCustomMinimumWidth(1, 48)
	p.list.SetColumnClipContent(0, true)
	p.list.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	p.list.OnItemActivated(func() {
		if i := treeIndex(p.list.GetSelected()); i >= 0 && p.OnPlayTrack != nil {
			p.OnPlayTrack(i)
		}
	})
	p.list.OnItemMouseSelected(func(_ Vector2.XY, button int) {
		if Input.MouseButton(button) != Input.MouseButtonRight {
			return
		}
		p.menuRow = p.selectedRow()
		if p.menuRow < 0 {
			return
		}
		p.enableMenu()
		popupAtMouse(p.menu)
	})
	p.empty = label("No audio yet. Add files with +, or right-click an audio file in the Media tab.", vDim)
	p.empty.SetClipText(false)
	p.empty.SetAutowrapMode(TextServer.AutowrapWordSmart)
	p.empty.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	p.empty.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	p.hint = withMargins(p.empty.AsControl(), 8, 8)
	p.hint.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	listBg := panel(vBk1)
	listBg.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	add(body, listBg.AsControl(), p.list.AsControl(), p.hint)
	onThemeReady(p.list.AsControl(), func() { p.built = false; p.render() })

	p.menu = PopupMenu.New()
	p.menu.MoreArgs().AddItem("Play Track", plMenuPlay, 0)
	p.menu.AddSeparator()
	p.menu.MoreArgs().AddItem("Move Track Up", plMenuUp, 0)
	p.menu.MoreArgs().AddItem("Move Track Down", plMenuDown, 0)
	p.menu.AddSeparator()
	p.menu.MoreArgs().AddItem("Remove from Playlist", plMenuRemove, 0)
	p.menu.OnIdPressed(p.menuPressed)
	p.list.AsNode().AddChild(p.menu.AsNode())

	add(p.root, header, body, p.buildTransport())
	p.state.Volume = 1
	p.render()
	return p
}

// buildTransport builds the playlist's transport: title and time, the seek
// slider, then ⏮ ⏯ ⏭ ⏹ … repeat shuffle 🔊━●.
func (p *AudioPlaylist) buildTransport() Control.Instance {
	bg := panel(vBk1)
	col := vbox(2)
	add(bg, withMargins(col.AsControl(), 8, 8))

	row1 := hbox(6)
	p.title = label("", "")
	p.title.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	p.time = label("0:00 / 0:00", vCaption)
	p.time.SetClipText(false)
	p.time.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	p.time.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
	add(row1, p.title.AsControl(), p.time.AsControl())

	p.seek = HSlider.New()
	p.seek.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	p.seek.AsControl().SetCustomMinimumSize(Vector2.New(0, 18))
	p.seek.AsControl().SetFocusMode(Control.FocusNone)
	rg := p.seek.AsRange()
	rg.SetMinValue(0)
	rg.SetMaxValue(1)
	rg.SetStep(0.001)
	p.seek.AsSlider().SetScrollable(false)
	p.seek.AsSlider().OnDragStarted(func() { p.seeking = true })
	p.seek.AsSlider().OnDragEnded(func(changed bool) {
		p.seeking = false
		if changed && p.OnSeek != nil {
			p.OnSeek(float64(p.seek.AsRange().Value()))
		}
	})
	rg.OnValueChanged(func(v Float.X) {
		if !p.seeking && p.OnSeek != nil { // a click on the track or a key step seeks at once
			p.OnSeek(float64(v))
		}
	})

	row3 := hbox(2)
	pressed := func(b BaseButton.Instance, fn *func()) {
		b.OnPressed(func() {
			if *fn != nil {
				(*fn)()
			}
		})
	}
	p.prev = newMiniButton(miniPlain, "skip_back", 24, 24, "Previous Track")
	pressed(p.prev.AsBaseButton(), &p.OnPrev)
	p.play = newTransportButton("play", "Play / Pause")
	p.play.AsControl().SetCustomMinimumSize(Vector2.New(30, 30))
	pressed(p.play.AsBaseButton(), &p.OnPlayPause)
	p.next = newMiniButton(miniPlain, "skip_fwd", 24, 24, "Next Track")
	pressed(p.next.AsBaseButton(), &p.OnNext)
	p.stop = newMiniButton(miniPlain, "stop", 24, 24, "Stop")
	pressed(p.stop.AsBaseButton(), &p.OnStop)
	p.repeat = newMiniButton(miniPlain, "loop", 24, 24, "Repeat: Off / All / One")
	p.repeat.AsBaseButton().SetToggleMode(true)
	pressed(p.repeat.AsBaseButton(), &p.OnRepeat)
	p.shuffle = newMiniButton(miniPlain, "shuffle", 24, 24, "Shuffle")
	p.shuffle.AsBaseButton().SetToggleMode(true)
	p.shuffle.AsBaseButton().OnToggled(func(on bool) {
		if p.OnShuffle != nil {
			p.OnShuffle(on)
		}
	})
	p.volIcon = TextureRect.New()
	p.volIcon.SetStretchMode(TextureRect.StretchKeepCentered)
	p.volIcon.AsControl().SetCustomMinimumSize(Vector2.New(18, 24))
	p.volIcon.AsControl().SetSizeFlagsVertical(Control.SizeShrinkCenter)
	p.volIcon.AsControl().SetMouseFilter(Control.MouseFilterIgnore)
	onThemeReady(p.volIcon.AsControl(), func() { p.volIcon.SetTexture(fwIcon(p.volIcon.AsControl(), "volume")) })
	p.volume = HSlider.New()
	p.volume.AsControl().SetCustomMinimumSize(Vector2.New(48, 18))
	p.volume.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	p.volume.AsControl().SetSizeFlagsVertical(Control.SizeShrinkCenter)
	p.volume.AsControl().SetFocusMode(Control.FocusNone)
	vr := p.volume.AsRange()
	vr.SetMinValue(0)
	vr.SetMaxValue(1)
	vr.SetStep(0.01)
	vr.SetValueNoSignal(1)
	vr.OnValueChanged(func(v Float.X) {
		if p.OnVolume != nil {
			p.OnVolume(float64(v))
		}
	})
	add(row3, p.prev.AsControl(), p.play.AsControl(), p.next.AsControl(), p.stop.AsControl(), fixed(4, 0),
		p.repeat.AsControl(), p.shuffle.AsControl(), fixed(4, 0), p.volIcon.AsControl(), p.volume.AsControl())
	add(col, fixed(0, 2), row1.AsControl(), p.seek.AsControl(), row3.AsControl(), fixed(0, 2))
	return bg.AsControl()
}

// AsControl returns the panel's root.
func (p *AudioPlaylist) AsControl() Control.Instance { return p.root.AsControl() }

// List returns the track list (keyboard focus, automation).
func (p *AudioPlaylist) List() Tree.Instance { return p.list }

// SetRows replaces the track list. The selection is kept while in range.
func (p *AudioPlaylist) SetRows(rows []PlaylistRow) {
	p.rows = append([]PlaylistRow(nil), rows...)
	p.built = false
	p.render()
}

// SetState projects the transport. Idempotent; it never emits the sliders'
// or buttons' signals (no-signal setters, flushed before the guard ends).
func (p *AudioPlaylist) SetState(s PlaylistState) {
	p.state = s
	p.renderTransport()
}

// State returns what the transport shows.
func (p *AudioPlaylist) State() PlaylistState { return p.state }

// selectedRow is the selected track, -1 none.
func (p *AudioPlaylist) selectedRow() int {
	i := treeIndex(p.list.GetSelected())
	if i >= len(p.rows) {
		return -1
	}
	return i
}

func (p *AudioPlaylist) render() {
	if !p.built {
		p.built = true
		sel := p.selectedRow()
		quietly(p.list.AsControl(), func() {
			p.list.Clear()
			root := p.list.CreateItem()
			dim := alpha(fwColor(p.list.AsControl(), "text"), 0.45)
			for _, r := range p.rows {
				it := root.CreateChild()
				it.SetText(0, r.Title)
				it.SetText(1, r.Time)
				it.SetTextAlignment(1, GUI.HorizontalAlignmentRight)
				it.SetTooltipText(0, r.Title)
				if r.Marker != "" {
					it.SetIcon(0, fwIcon(p.list.AsControl(), r.Marker))
					it.SetIconMaxWidth(0, 12)
				}
				if r.Missing {
					it.SetCustomColor(0, dim)
					it.SetCustomColor(1, dim)
				}
			}
			if sel >= 0 && sel < len(p.rows) {
				selectTreeRow(p.list, sel)
			}
		})
	}
	p.hint.AsCanvasItem().SetVisible(len(p.rows) == 0)
	p.list.AsCanvasItem().SetVisible(len(p.rows) > 0)
	p.renderTransport()
}

func (p *AudioPlaylist) renderTransport() {
	s := p.state
	title := s.Title
	if title == "" {
		title = "No track"
	}
	p.title.SetText(title)
	if !p.seeking {
		frac := 0.0
		if s.Duration > 0 {
			frac = min(max(s.Position/s.Duration, 0), 1)
		}
		p.seek.AsRange().SetValueNoSignal(Float.X(frac))
	}
	p.time.SetText(core.TransportTime(s.Position, s.Duration))
	icon := "play"
	if s.Playing {
		icon = "pause"
	}
	p.play.setIcon(icon)
	repeatIcon := "loop"
	if s.Repeat == "one" {
		repeatIcon = "repeat_one"
	}
	if p.repeat.icon != repeatIcon {
		p.repeat.icon = repeatIcon
		p.repeat.AsCanvasItem().QueueRedraw()
	}
	p.repeat.AsBaseButton().SetPressedNoSignal(s.Repeat == "all" || s.Repeat == "one")
	p.shuffle.AsBaseButton().SetPressedNoSignal(s.Shuffle)
	p.volume.AsRange().SetValueNoSignal(Float.X(min(max(s.Volume, 0), 1)))
	for _, b := range []BaseButton.Instance{p.play.AsBaseButton(), p.prev.AsBaseButton(), p.next.AsBaseButton()} {
		b.SetDisabled(!s.Enabled)
	}
	p.stop.AsBaseButton().SetDisabled(s.Stopped)
	p.seek.AsSlider().SetEditable(!s.Stopped && s.Duration > 0)
	p.fade.setChecked(strconv.FormatFloat(s.FadeOut, 'f', -1, 64))
	flushCalls(p.root.AsControl())
}

// enableMenu greys out the context menu entries that do not apply to the row.
func (p *AudioPlaylist) enableMenu() {
	for idx := 0; idx < p.menu.ItemCount(); idx++ {
		switch p.menu.GetItemId(idx) {
		case plMenuUp:
			p.menu.SetItemDisabled(idx, p.menuRow <= 0)
		case plMenuDown:
			p.menu.SetItemDisabled(idx, p.menuRow >= len(p.rows)-1)
		case plMenuPlay:
			p.menu.SetItemDisabled(idx, p.menuRow < 0 || p.rows[p.menuRow].Missing)
		}
	}
}

func (p *AudioPlaylist) menuPressed(id int) {
	row := p.menuRow
	valid := row >= 0 && row < len(p.rows)
	switch id {
	case plMenuPlay:
		if valid && p.OnPlayTrack != nil {
			p.OnPlayTrack(row)
		}
	case plMenuUp:
		if valid && row > 0 && p.OnMove != nil {
			p.OnMove(row, row-1)
		}
	case plMenuDown:
		if valid && row < len(p.rows)-1 && p.OnMove != nil {
			p.OnMove(row, row+1)
		}
	case plMenuRemove:
		if valid && p.OnRemove != nil {
			p.OnRemove(row)
		}
	case plMenuAddSelected:
		if p.OnAddSelected != nil {
			p.OnAddSelected()
		}
	case plMenuAddFiles:
		if p.OnAddFiles != nil {
			p.OnAddFiles()
		}
	case plMenuClear:
		if p.OnClear != nil {
			p.OnClear()
		}
	}
}
