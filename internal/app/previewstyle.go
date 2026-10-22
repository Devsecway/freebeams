package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/render"
	"freebeams/internal/ui"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEventMouseButton"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Vector2"
)

// Restyling the item in Preview (the Preview Output monitor's context menu,
// UI-SPEC §3.4, ui/previewmenu.go). The override is app state: the Preview
// deck's theme, and the copy behind it:
//
//   - an item loaded from the schedule: the schedule item's own theme copy
//     (ScheduleItem.Theme), so the schedule turns dirty and the .fbs keeps it;
//   - library content (a song, a passage): the library deck behind the
//     resource preview (songRes, scriptureDeck), so selecting another verse,
//     re-selecting the song, Go Live and Add to Schedule carry it along
//     (core.CarryStyle).
//
// Live is never restyled here: the output changes when the item is sent
// live again. The saved theme in the library is never written. The menu, the
// color dialog, the media picker and the item theme editor are dumb views
// projected in renderPreviewStyle.

// previewStyleState is the restyling's own state (the override itself lives
// in the decks and the schedule, see above).
type previewStyleState struct {
	colorFor string         // what the color dialog picks: "background" or "text"
	media    []models.Media // the media picker's tiles: the library's images and videos
	mediaVer int            // bumped when media changes
	mediaGen core.Generation
	itemEdit *models.Deck // the Preview item the item theme editor is open on
	itemSel  int          // its selected slide (the editor's live preview)
}

// previewStyleView is what renderPreviewStyle last projected.
type previewStyleView struct {
	menu     ui.PreviewMenuState
	menuInit bool
	mediaVer int
}

var errNotRestylable = errors.New("the Preview holds nothing to restyle: preview a song, a passage or a schedule item with text first")

// wirePreviewStyle connects the menu and its dialogs (from buildUI, after
// wireThemes: it wraps the theme editor's callbacks for the item mode).
func (a *App) wirePreviewStyle() {
	a.view.ps.mediaVer = -1
	pm := a.win.PreviewMenu
	pm.OnAction = func(item string) { a.event(func() { a.fail("preview menu", a.previewMenuItem(item)) }) }
	pm.OnTheme = func(i int) { a.event(func() { a.fail("preview theme", a.previewChangeTheme(i)) }) }
	pm.OnOpened = func() { a.renderPreviewMenu() }
	a.win.ColorDialog.OnPick = func(c models.RGBA) { a.event(func() { a.fail("preview color", a.previewColorPicked(c)) }) }
	mp := a.win.MediaPicker
	mp.SetThumbnailFactory(func(i int) Control.Instance {
		if i < 0 || i >= len(a.ps.media) {
			return Control.Instance{}
		}
		bg := core.ThumbBackground(a.ps.media[i])
		s := models.Slide{Kind: models.SlideMedia, Background: &bg}
		v := render.NewSlideView()
		v.SetTransitionDuration(0)
		v.SetFrame(models.Frame{Mode: models.ModeNormal, Slide: &s, Theme: models.DefaultMediaTheme()})
		return v.AsControl()
	})
	mp.OnPick = func(i int) { a.event(func() { a.fail("preview media", a.previewMediaPicked(i)) }) }

	// The theme editor in item mode (Edit Theme for This Item…) restyles the
	// item instead of saving a theme.
	e := a.win.ThemeEditor
	save, apply, cancel := e.OnSave, e.OnApply, e.OnCancel
	e.OnSave = func(t models.Theme) {
		if !e.ItemMode() {
			save(t)
			return
		}
		a.event(func() {
			a.th.editing = nil
			err := a.applyItemEditor(t)
			a.ps.itemEdit = nil
			a.fail("item theme", err)
		})
	}
	e.OnApply = func(t models.Theme) {
		if !e.ItemMode() {
			apply(t)
			return
		}
		a.event(func() { a.fail("item theme", a.applyItemEditor(t)) })
	}
	e.OnCancel = func() {
		if e.ItemMode() {
			a.event(func() { a.ps.itemEdit = nil })
		}
		cancel()
	}
}

// ---- derived state -------------------------------------------------------------

// previewBase is the saved theme deck d's look is based on (core.BaseTheme):
// the item's theme from the library, else the default for its type.
func (a *App) previewBase(d *models.Deck) models.Theme {
	kind := core.DeckKind(d)
	if d != nil && d.ItemID != "" {
		if i := a.sched.IndexOf(d.ItemID); i >= 0 {
			kind = a.sched.Items[i].Kind
		}
	}
	if kind != models.ItemScripture {
		kind = models.ItemSong
	}
	return core.BaseTheme(d.Theme, a.th.list, a.defaultTheme(kind))
}

// previewMenuState projects the Preview deck onto the menu.
func (a *App) previewMenuState() ui.PreviewMenuState {
	s := ui.PreviewMenuState{Themes: a.themeNames(), Theme: -1}
	d := a.pres.State.Preview
	if !core.Restylable(d) {
		return s
	}
	t, base := d.Theme, a.previewBase(d)
	s.Enabled = true
	s.Theme = core.ThemeIndexOf(a.th.list, t.ID)
	s.Custom = t.Custom
	s.BackgroundCustom = t.Background != base.Background
	text := t
	text.Background = base.Background
	s.TextCustom = !text.SameLook(base)
	s.Bold, s.Italic = t.Font.Bold, t.Font.Italic
	s.Outline, s.Shadow = t.Outline.Enabled, t.Shadow.Enabled
	s.Align = t.HAlign
	return s
}

// editorPreviewFrame is what the theme editor's live preview shows: the
// sample lyric, or in item mode the item's own slide.
func (a *App) editorPreviewFrame(t models.Theme) models.Frame {
	if d := a.ps.itemEdit; d != nil && a.win.ThemeEditor.ItemMode() && len(d.Slides) > 0 {
		s := d.Slides[max(0, min(a.ps.itemSel, len(d.Slides)-1))]
		s.Background = core.StillBackground(s.Background)
		return models.Frame{Mode: models.ModeNormal, Slide: &s, Theme: t}
	}
	return editorFrame(t)
}

// previewDialogState is the menu's part of /state.dialog.
func (a *App) previewDialogState() (menu bool, dialog string) {
	switch {
	case a.win.ColorDialog.Visible():
		dialog = "color"
	case a.win.MediaPicker.Visible():
		dialog = "media"
	case a.win.ThemeEditor.Visible() && a.win.ThemeEditor.ItemMode():
		dialog = "item-theme"
	}
	return a.win.PreviewMenu.Visible(), dialog
}

// renderPreviewMenu projects the menu (also right before it opens).
func (a *App) renderPreviewMenu() {
	v := &a.view.ps
	if s := a.previewMenuState(); !v.menuInit || !reflect.DeepEqual(s, v.menu) {
		v.menu, v.menuInit = s, true
		a.win.PreviewMenu.SetState(s)
	}
}

// renderPreviewStyle projects the menu and the media picker's tiles (from render).
func (a *App) renderPreviewStyle() {
	a.renderPreviewMenu()
	if v := &a.view.ps; v.mediaVer != a.ps.mediaVer {
		v.mediaVer = a.ps.mediaVer
		items := make([]ui.SlideGridItem, len(a.ps.media))
		for i, m := range a.ps.media {
			badge := "Image"
			if m.Kind == models.MediaVideo {
				badge = "Video"
			}
			items[i] = ui.SlideGridItem{Label: m.Title, Badge: badge}
		}
		a.win.MediaPicker.SetItems(items)
	}
}

// ---- events -------------------------------------------------------------------------

// restylePreview gives the item in Preview theme t (see the file comment
// for where the override is kept).
func (a *App) restylePreview(t models.Theme) error {
	d := a.pres.State.Preview
	if !core.Restylable(d) {
		return errNotRestylable
	}
	old := *d
	if d.ItemID != "" {
		if i := a.sched.IndexOf(d.ItemID); i >= 0 && core.ApplyItemTheme(a.sched, i, t) {
			a.schedVer++
		}
	} else {
		for _, ld := range []*models.Deck{a.songRes.deck, a.scriptureDeck} {
			if ld != nil && core.SameContent(ld, &old) {
				ld.Theme = t
			}
		}
	}
	if a.pres.RestylePreview(t) {
		log.Printf("[freebeams] preview %q restyled (theme %q, custom %v)", d.Title, t.Name, t.Custom)
	}
	return nil
}

// restyle applies edit e to the Preview item's theme.
func (a *App) restyle(e core.StyleEdit) error {
	d := a.pres.State.Preview
	if !core.Restylable(d) {
		return errNotRestylable
	}
	return a.restylePreview(core.RestyleTheme(d.Theme, a.previewBase(d), e))
}

// previewChangeTheme is Change Theme ▸ entry i: the saved theme as it is.
func (a *App) previewChangeTheme(i int) error {
	if i < 0 || i >= len(a.th.list) {
		return fmt.Errorf("no theme at index %d", i)
	}
	return a.restylePreview(a.th.list[i])
}

// previewMenuItem runs one menu entry (a click, or preview-menu).
func (a *App) previewMenuItem(item string) error {
	d := a.pres.State.Preview
	if !core.Restylable(d) {
		return errNotRestylable
	}
	t := d.Theme
	toggle := func(on bool) *bool { on = !on; return &on }
	align := func(h models.HAlign) *models.HAlign { return &h }
	switch item {
	case ui.PreviewBackgroundMedia:
		a.openMediaPicker(nil)
	case ui.PreviewBackgroundColor:
		a.ps.colorFor = "background"
		a.win.ColorDialog.Open("Solid Color", t.Background.Color)
	case ui.PreviewTextColor:
		a.ps.colorFor = "text"
		a.win.ColorDialog.Open("Font Color", t.Font.Color)
	case ui.PreviewEditTheme:
		a.openItemEditor()
	case ui.PreviewRevert:
		return a.restylePreview(a.previewBase(d))
	case ui.PreviewBackgroundReset:
		return a.restyle(core.StyleEdit{ResetBackground: true})
	case ui.PreviewTextReset:
		return a.restyle(core.StyleEdit{ResetText: true})
	case ui.PreviewFontBigger:
		return a.restyle(core.StyleEdit{FontStep: +1})
	case ui.PreviewFontSmaller:
		return a.restyle(core.StyleEdit{FontStep: -1})
	case ui.PreviewBold:
		return a.restyle(core.StyleEdit{Bold: toggle(t.Font.Bold)})
	case ui.PreviewItalic:
		return a.restyle(core.StyleEdit{Italic: toggle(t.Font.Italic)})
	case ui.PreviewOutline:
		return a.restyle(core.StyleEdit{Outline: toggle(t.Outline.Enabled)})
	case ui.PreviewShadow:
		return a.restyle(core.StyleEdit{Shadow: toggle(t.Shadow.Enabled)})
	case ui.PreviewAlignLeft:
		return a.restyle(core.StyleEdit{Align: align(models.AlignLeft)})
	case ui.PreviewAlignCenter:
		return a.restyle(core.StyleEdit{Align: align(models.AlignCenter)})
	case ui.PreviewAlignRight:
		return a.restyle(core.StyleEdit{Align: align(models.AlignRight)})
	default:
		return fmt.Errorf("unknown preview menu item %q", item)
	}
	return nil
}

// previewColorPicked is the color dialog's OK.
func (a *App) previewColorPicked(c models.RGBA) error {
	switch a.ps.colorFor {
	case "background":
		bg := core.SolidBackground(c)
		return a.restyle(core.StyleEdit{Background: &bg})
	case "text":
		c.A = 1
		return a.restyle(core.StyleEdit{FontColor: &c})
	}
	return nil
}

// loadBackgroundMedia lists the library's images and videos for the media
// picker in the background, then runs then (main thread). cmd, when not
// nil, gets preview-menu's reply (or the error).
func (a *App) loadBackgroundMedia(cmd *control.Command, then func() error) {
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	ticket := a.ps.mediaGen.Next()
	a.async(nil, func(ctx context.Context) func() error {
		list, err := st.SearchMedia(ctx, "", "", 0)
		return func() error {
			err := a.applyBackgroundMedia(ticket, list, err, then)
			if cmd != nil {
				if err != nil {
					cmd.RespondErr(fmt.Errorf("preview-menu: %w", err))
				} else {
					a.previewMenuReply(cmd)
				}
			}
			return err
		}
	})
}

// applyBackgroundMedia takes a listing into the picker's state (main thread).
func (a *App) applyBackgroundMedia(ticket uint64, list []models.Media, err error, then func() error) error {
	if err != nil {
		a.alert("Select Media", fmt.Errorf("Could not list the media library.\n\n%w", err))
		return err
	}
	if !a.ps.mediaGen.Current(ticket) {
		return nil // a newer listing is on its way
	}
	a.ps.media = core.BackgroundMedia(list)
	a.ps.mediaVer++
	if then != nil {
		return then()
	}
	return nil
}

// openMediaPicker shows Select Media… with the library's images and videos.
func (a *App) openMediaPicker(cmd *control.Command) {
	a.loadBackgroundMedia(cmd, func() error {
		a.renderPreviewStyle() // the tiles before the window shows
		a.win.MediaPicker.Open()
		return nil
	})
}

// previewMediaPicked is the media picker's OK: tile i becomes the background.
func (a *App) previewMediaPicked(i int) error {
	if i < 0 || i >= len(a.ps.media) {
		return fmt.Errorf("no media tile %d", i)
	}
	return a.useMediaBackground(a.ps.media[i])
}

// useMediaBackground makes library item m the Preview item's background. A
// video plays from its transcode where the library has one.
func (a *App) useMediaBackground(m models.Media) error {
	bg, err := core.MediaBackground(m)
	if err != nil {
		return err
	}
	bg = models.ResolveVideo(bg, a.lookupPlayable)
	log.Printf("[freebeams] preview background: media %q", m.Title)
	return a.restyle(core.StyleEdit{Background: &bg})
}

// openItemEditor is Edit Theme for This Item…: the theme editor on the
// Preview item's own copy.
func (a *App) openItemEditor() {
	d := a.pres.State.Preview
	if !core.Restylable(d) {
		return
	}
	cp := *d
	cp.Slides = append([]models.Slide(nil), d.Slides...)
	a.ps.itemEdit, a.ps.itemSel = &cp, max(a.pres.State.PreviewIndex, 0)
	t := d.Theme
	a.th.editing = &t
	a.win.ThemeEditor.OpenItem(t, d.Title)
}

// applyItemEditor is the item theme editor's OK / Apply: the edited copy
// restyles the item it was opened on, if that is still in Preview.
func (a *App) applyItemEditor(t models.Theme) error {
	target := a.ps.itemEdit
	d := a.pres.State.Preview
	if target == nil || !core.SameContent(d, target) {
		return errors.New("the item the theme editor was opened on is no longer in Preview")
	}
	t.ID, t.Name = d.Theme.ID, d.Theme.Name
	return a.restylePreview(core.Customize(t, a.previewBase(d)))
}

// ---- control actions ------------------------------------------------------------------

// previewMenuArgs is the body of preview-menu.
type previewMenuArgs struct {
	Item  string `json:"item"`
	Sub   string `json:"sub"`   // open: also open this submenu (theme | background | text)
	Name  string `json:"name"`  // theme: the theme
	Index *int   `json:"index"` // theme: the theme; preview-media: the tile
	Title string `json:"title"` // background-media: the library item (skips the picker)
	Color string `json:"color"` // background-color / text-color: the color (skips the dialog)
	On    *bool  `json:"on"`    // bold / italic / outline / shadow: set instead of toggling
}

const previewMenuItems = "open|close|theme|background-media|background-color|background-reset|font-bigger|font-smaller|" +
	"text-color|bold|italic|outline|shadow|align-left|align-center|align-right|text-reset|edit-theme|revert"

// previewMenuReply is preview-menu's reply: the menus as projected and the state.
func (a *App) previewMenuReply(cmd *control.Command) {
	a.replies = append(a.replies, func() {
		pm := a.win.PreviewMenu
		cmd.RespondJSON(struct {
			Menu       []ui.MenuEntry `json:"menu"`
			Theme      []ui.MenuEntry `json:"theme"`
			Background []ui.MenuEntry `json:"background"`
			Text       []ui.MenuEntry `json:"text"`
			State      core.State     `json:"state"`
		}{pm.Entries(""), pm.Entries("theme"), pm.Entries("background"), pm.Entries("text"), a.state()})
	})
}

// handlePreviewStyleCommand runs the restyling actions; false when the action
// is not one of them.
func (a *App) handlePreviewStyleCommand(cmd *control.Command) bool {
	switch cmd.Action {
	case "preview-menu":
		var args previewMenuArgs
		if err := decodeArgs(cmd, &args); err != nil {
			cmd.RespondErr(err)
			return true
		}
		a.previewMenuAction(cmd, args)
	case "preview-theme":
		// {theme} sets the whole copy; the other fields patch it (core.StylePatch).
		var p core.StylePatch
		if err := decodeArgs(cmd, &p); err != nil {
			cmd.RespondErr(err)
			return true
		}
		a.answer(cmd, a.previewPatch(p))
	case "preview-color":
		// {color}: a pick in the open color dialog (OK applies it: accept).
		var args previewMenuArgs
		if err := decodeArgs(cmd, &args); err != nil {
			cmd.RespondErr(err)
			return true
		}
		if !a.win.ColorDialog.Visible() {
			cmd.RespondErr(errors.New("preview-color: the color dialog is not open"))
			return true
		}
		c, err := core.ParseHexColor(args.Color)
		if err != nil {
			cmd.RespondErr(fmt.Errorf("preview-color: %w", err))
			return true
		}
		a.win.ColorDialog.SetColor(c)
		a.answer(cmd, nil)
	case "preview-media":
		// {title|index}: a click on a tile of the open media picker.
		var args previewMenuArgs
		if err := decodeArgs(cmd, &args); err != nil {
			cmd.RespondErr(err)
			return true
		}
		if !a.win.MediaPicker.Visible() {
			cmd.RespondErr(errors.New("preview-media: the media picker is not open"))
			return true
		}
		i := -1
		switch {
		case args.Index != nil:
			i = *args.Index
		case args.Title != "":
			i = core.FindMedia(a.ps.media, args.Title)
		}
		if i < 0 || i >= len(a.ps.media) {
			cmd.RespondErr(fmt.Errorf("preview-media: no image or video matches %s", string(cmd.Data)))
			return true
		}
		a.win.MediaPicker.Select(i)
		a.answer(cmd, nil)
	case "monitor-context-click":
		// {monitor: preview|live, ctrl}: a right-click (with ctrl: a
		// Control-click of the left button) in the middle of an output
		// monitor, through Godot's input pipeline.
		var args struct {
			Monitor string `json:"monitor"`
			Ctrl    bool   `json:"ctrl"`
		}
		if err := decodeArgs(cmd, &args); err != nil {
			cmd.RespondErr(err)
			return true
		}
		a.monitorContextClick(cmd, args.Monitor, args.Ctrl)
	default:
		return false
	}
	return true
}

// decodeArgs reads an action's own JSON body.
func decodeArgs(cmd *control.Command, v any) error {
	if len(cmd.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(cmd.Data, v); err != nil {
		return fmt.Errorf("%s: bad JSON body: %w", cmd.Action, err)
	}
	return nil
}

// previewMenuAction runs `preview-menu {item, …}` as the menu does.
func (a *App) previewMenuAction(cmd *control.Command, args previewMenuArgs) {
	pm := a.win.PreviewMenu
	item := strings.ToLower(strings.TrimSpace(args.Item))
	switch item {
	case "open":
		a.renderPreviewMenu()
		pm.Close() // reopened fresh, so only the asked-for submenu is up
		pm.Open()
		if args.Sub != "" && !pm.OpenSubmenu(args.Sub) {
			cmd.RespondErr(fmt.Errorf("preview-menu: unknown submenu %q (theme|background|text)", args.Sub))
			return
		}
		a.previewMenuReply(cmd)
		return
	case "close":
		pm.Close()
		a.previewMenuReply(cmd)
		return
	}
	if !core.Restylable(a.pres.State.Preview) {
		cmd.RespondErr(fmt.Errorf("preview-menu %s: %w", item, errNotRestylable))
		return
	}
	pm.Close() // a click on an entry closes the menu
	var err error
	switch {
	case item == "theme":
		i := -1
		switch {
		case args.Index != nil:
			i = *args.Index
		case args.Name != "":
			i = core.FindTheme(a.th.list, args.Name)
		}
		if i < 0 || i >= len(a.th.list) {
			cmd.RespondErr(fmt.Errorf("preview-menu theme: no theme matches %s", string(cmd.Data)))
			return
		}
		a.renderPreviewMenu()
		pm.PressTheme(i)
	case item == ui.PreviewBackgroundMedia && args.Title != "":
		title := args.Title
		a.loadBackgroundMedia(cmd, func() error {
			i := core.FindMedia(a.ps.media, title)
			if i < 0 {
				return fmt.Errorf("preview-menu background-media: no image or video matches %q", title)
			}
			return a.previewMediaPicked(i)
		})
		return
	case item == ui.PreviewBackgroundMedia:
		a.openMediaPicker(cmd) // answered once the picker is up
		return
	case (item == ui.PreviewBackgroundColor || item == ui.PreviewTextColor) && args.Color != "":
		c, perr := core.ParseHexColor(args.Color)
		if perr != nil {
			cmd.RespondErr(fmt.Errorf("preview-menu %s: %w", item, perr))
			return
		}
		a.ps.colorFor = map[string]string{ui.PreviewBackgroundColor: "background", ui.PreviewTextColor: "text"}[item]
		err = a.previewColorPicked(c)
	case args.On != nil && (item == ui.PreviewBold || item == ui.PreviewItalic || item == ui.PreviewOutline || item == ui.PreviewShadow):
		on := args.On
		e := map[string]core.StyleEdit{ui.PreviewBold: {Bold: on}, ui.PreviewItalic: {Italic: on},
			ui.PreviewOutline: {Outline: on}, ui.PreviewShadow: {Shadow: on}}[item]
		err = a.restyle(e)
	default:
		// Through the menu's own id mapping, as a click: its handler logs a
		// failure as lastError.
		a.renderPreviewMenu()
		prev := a.lastError
		a.lastError = ""
		if !pm.Press(item) {
			a.lastError = prev
			cmd.RespondErr(fmt.Errorf("preview-menu: unknown item %q (%s)", item, previewMenuItems))
			return
		}
		if a.lastError != "" {
			err = errors.New(a.lastError)
		} else {
			a.lastError = prev
		}
	}
	if err != nil {
		cmd.RespondErr(fmt.Errorf("preview-menu %s: %w", item, err))
		return
	}
	a.previewMenuReply(cmd)
}

// previewPatch runs preview-theme.
func (a *App) previewPatch(p core.StylePatch) error {
	d := a.pres.State.Preview
	if !core.Restylable(d) {
		return fmt.Errorf("preview-theme: %w", errNotRestylable)
	}
	if p.Theme != nil {
		t := *p.Theme
		t.ID, t.Name = d.Theme.ID, d.Theme.Name
		if err := a.restylePreview(core.Customize(t, a.previewBase(d))); err != nil {
			return err
		}
	}
	e, err := p.Edit()
	if err != nil {
		return fmt.Errorf("preview-theme: %w", err)
	}
	if e == (core.StyleEdit{}) {
		return nil
	}
	return a.restyle(e)
}

// monitorContextClick pushes a right-click (or a Control-click) at the
// centre of the Preview or Live monitor through Godot's input pipeline.
func (a *App) monitorContextClick(cmd *control.Command, monitor string, ctrl bool) {
	var host Control.Instance
	switch monitor {
	case "", "preview":
		host = a.win.PreviewMonitor.AsControl()
	case "live":
		host = a.win.LiveMonitor.AsControl()
	default:
		cmd.RespondErr(fmt.Errorf("monitor-context-click: unknown monitor %q (preview|live)", monitor))
		return
	}
	r := host.GetGlobalRect()
	scale := Window.Get(a.AsNode()).ContentScaleFactor()
	pos := Vector2.New((r.Position.X+r.Size.X/2)*scale, (r.Position.Y+r.Size.Y/2)*scale)
	button, mask := Input.MouseButtonRight, Input.MouseButtonMaskRight
	if ctrl {
		button, mask = Input.MouseButtonLeft, Input.MouseButtonMaskLeft
	}
	for _, pressed := range []bool{true, false} {
		ev := InputEventMouseButton.New()
		ev.SetButtonIndex(button)
		ev.SetPressed(pressed)
		if pressed {
			ev.AsInputEventMouse().SetButtonMask(mask)
		}
		ev.AsInputEventWithModifiers().SetCtrlPressed(ctrl)
		ev.AsInputEventMouse().SetPosition(pos)
		ev.AsInputEventMouse().SetGlobalPosition(pos)
		Input.ParseInputEvent(ev.AsInputEvent())
	}
	a.after(nativeKeyFrames, func() { a.previewMenuReply(cmd) })
}

// previewStyleWindow is a screenshot window of the menu or its dialogs.
func (a *App) previewStyleWindow(name string) (Window.Instance, bool) {
	pm := a.win.PreviewMenu
	switch name {
	case "preview-menu":
		return pm.Window(""), true
	case "preview-theme-menu":
		return pm.Window("theme"), true
	case "preview-background-menu":
		return pm.Window("background"), true
	case "preview-text-menu":
		return pm.Window("text"), true
	case "color":
		return a.win.ColorDialog.AsWindow(), true
	case "media-picker":
		return a.win.MediaPicker.AsWindow(), true
	}
	return Window.Nil, false
}
