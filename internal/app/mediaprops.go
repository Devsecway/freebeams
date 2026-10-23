package app

import (
	"context"
	"errors"
	"fmt"
	"log"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
)

// The media tile menu's Edit Properties… (EasyWorship's Edit Video / Audio
// Properties, Form_VideoProperties): Title, Aspect Ratio, Repeating, Volume /
// Mute and Tags of a library item. The volume is the default a monitor
// starts the item at (core.PlaybackDefaults). The tile menu's Aspect Ratio ▸ / Repeating ▸ go
// through the same setMediaProperties. The library deck and Preview (when it
// shows the library item) follow at once; Live and the schedule keep their
// snapshots until Update items in Schedule, as in EasyWorship.

// propsFor is the library item the open Properties dialog edits (its ID; 0 none).
type propsFor struct{ id int64 }

// wireMediaProperties connects the menu entry and the dialog (from wireMediaMenu).
func (a *App) wireMediaProperties() {
	a.win.Media.OnProperties = func(i int) { a.event(func() { a.openMediaProperties(i) }) }
	a.win.MediaProperties.OnOK = func(p models.MediaProperties) {
		a.event(func() {
			id := a.mdProps.id
			a.mdProps.id = 0
			if i := core.MediaIndexOf(a.md.list, id); i >= 0 {
				a.setMediaProperties(i, p, nil)
			}
		})
	}
}

// openMediaProperties shows the dialog for media item i.
func (a *App) openMediaProperties(i int) {
	if i < 0 || i >= len(a.md.list) {
		return
	}
	m := a.md.list[i]
	a.mdProps.id = m.ID
	a.win.MediaProperties.Open(m.Kind, m.Path, m.Properties())
}

// setMediaProperties applies p to media item i (models.Media.WithProperties)
// and saves it in the background (store.SetMediaProperties: a targeted
// update the pipeline's whole-row saves keep).
func (a *App) setMediaProperties(i int, p models.MediaProperties, cmd *control.Command) {
	if i < 0 || i >= len(a.md.list) {
		a.answer(cmd, fmt.Errorf("no media item at index %d", i))
		return
	}
	cur := a.md.list[i]
	next := cur.WithProperties(p)
	if next.Properties() == cur.Properties() {
		a.answer(cmd, nil)
		return
	}
	a.md.list[i] = next
	a.md.thumbVer++
	a.md.ver++ // a new title or tags re-label the tile
	if a.md.item != nil && a.md.item.ID == next.ID {
		old := a.md.deck
		cp := next
		a.md.item = &cp
		deck := core.MediaDeck(cp, a.lookupPlayable)
		a.md.deck = &deck
		if old != nil && core.SameDeck(a.pres.State.Preview, old) {
			a.setPreview(&deck)
		}
	}
	np := next.Properties()
	log.Printf("[freebeams] media %d %q: aspect %s, repeat %s, volume %d, mute %v, tags %q", next.ID, next.Title,
		np.Aspect, np.Repeat, np.Volume, np.Mute, np.Tags)
	st := a.st()
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	id, title := next.ID, next.Title
	a.async(cmd, func(ctx context.Context) func() error {
		err := st.SetMediaProperties(ctx, id, np)
		return func() error {
			if err != nil {
				a.alert("Media Properties", fmt.Errorf("Could not save the properties of %q.\n\n%w", title, err))
			}
			return err
		}
	})
}

// handleMediaPropsCommand runs `media-properties {props}` (fill the open
// dialog and press OK) and `media-properties {cancel: true}`; false when the
// action is another one. An empty aspect or repeat keeps the dialog's
// choice; title, tags, volume (percent, 0 = Carry Over) and mute are
// always filled in.
func (a *App) handleMediaPropsCommand(cmd *control.Command, args actionArgs) bool {
	if cmd.Action != "media-properties" {
		return false
	}
	d := a.win.MediaProperties
	switch {
	case !d.Visible():
		cmd.RespondErr(errors.New("media-properties: the dialog is not open (media-menu {item: \"properties\"})"))
	case args.Cancel:
		d.Cancel()
		a.mdProps.id = 0
		a.answer(cmd, nil)
	case args.Props == nil:
		cmd.RespondErr(errors.New("media-properties: \"props\" {title, tags, aspect, repeat, volume, mute} or \"cancel\" is required"))
	default:
		cur := d.Properties()
		p := *args.Props
		if p.Aspect == "" {
			p.Aspect = cur.Aspect
		}
		if p.Repeat == "" {
			p.Repeat = cur.Repeat
		}
		if !p.Aspect.Valid() || !p.Repeat.Valid() {
			cmd.RespondErr(fmt.Errorf("media-properties: unknown aspect %q or repeat %q", p.Aspect, p.Repeat))
			return true
		}
		if p.Volume != models.ClampMediaVolume(p.Volume) {
			cmd.RespondErr(fmt.Errorf("media-properties: volume %d is not 0 (carry over) .. %d", p.Volume, models.MaxMediaVolume))
			return true
		}
		d.SetFields(p)
		d.Accept() // OnOK posts the event; answer after it ran
		a.after(1, func() { a.answer(cmd, nil) })
	}
	return true
}
