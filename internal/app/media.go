package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"time"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/importers/easyworship"
	"freebeams/internal/media"
	"freebeams/internal/models"
	"freebeams/internal/render"
	"freebeams/internal/store"
	"freebeams/internal/ui"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/DisplayServer"
)

// The Media tab: its state, event handlers, control actions and the
// background pipeline that derives thumbnails, metadata and (with ffmpeg +
// libtheora) playable transcodes. Authoritative state lives on the App
// (mediaState); render projects it onto ui.MediaPanel.

// mediaPurpose is what to do with a selected media item.
type mediaPurpose int

const (
	mediaPreview mediaPurpose = iota
	mediaGoLive
	mediaSchedule
)

// mediaState is the Media tab's authoritative state.
type mediaState struct {
	query    string
	kind     models.MediaKind
	list     []models.Media // the listed items (query + kind)
	ver      int            // bumped when list changes
	thumbVer int            // bumped when an item's derived files or badge changed
	sel      int            // selected tile, -1 none
	item     *models.Media  // selected item (kept while filtered out)
	deck     *models.Deck   // the selected item's deck (resource preview)
	videos   int            // library counts
	images   int
	status   string // footer text
	gen      core.Generation

	cache      string            // derived-files directory
	tools      media.Tools       // ffmpeg / ffprobe
	toolsKnown bool              // Detect has run
	playable   map[string]string // video source path -> playable file

	working  bool              // the pipeline is running
	again    bool              // rerun when the current pass ends
	progress map[int64]float64 // transcode progress by media id
	failed   map[int64]bool    // items whose derivation failed this session (not retried)

	// The tile menu's View ▸ and Sort by ▸, persisted (menus_items.go).
	view core.ItemView
	sort core.ItemSort

	audio int // library count of audio files
}

// mediaCounts are the library's counts by kind.
func (a *App) mediaCounts() core.MediaCounts {
	return core.MediaCounts{Videos: a.md.videos, Images: a.md.images, Audio: a.md.audio}
}

// progressInterval throttles transcode progress posts to the main thread.
const progressInterval = 250 * time.Millisecond

// mediaFileFilters are the native dialog filters for Add Media Files….
var mediaFileFilters = []string{"*.png, *.jpg, *.jpeg, *.webp, *.bmp, *.mp4, *.mov, *.m4v, *.ogv, *.webm, *.mp3, *.m4a, *.aac, *.wav, *.aiff, *.aif, *.flac, *.ogg ; Images, Videos and Audio"}

// initMedia prepares the cache dir, detects the tools in the background and
// loads the library. Called from Ready.
func (a *App) initMedia() {
	a.md.sel = -1
	a.md.playable = map[string]string{}
	a.md.progress = map[int64]float64{}
	a.md.failed = map[int64]bool{}
	dir, err := media.DefaultCacheDir(os.Getenv(core.CacheEnv))
	if err != nil {
		a.fail("media cache", err)
	} else {
		a.md.cache = dir
	}
	a.async(nil, func(ctx context.Context) func() error {
		tools := media.Detect(ctx)
		return func() error {
			a.md.tools, a.md.toolsKnown = tools, true
			log.Printf("[freebeams] media tools: %s", tools.Status())
			a.md.status = a.mediaFooter()
			a.startMediaWork()
			return nil
		}
	})
	a.md.view, a.md.sort = a.loadItemPrefs(core.SettingMediaView, core.SettingMediaSort)
	a.loadMedia("", "", nil)
}

// mediaCaps is what the machine can do with videos and audio.
func (a *App) mediaCaps() core.MediaCaps {
	return core.MediaCaps{FFmpeg: a.md.tools.Available(), Theora: a.md.tools.CanTranscode(), Native: render.NativeVideoSupported(),
		Player: render.MediaPlayer()}
}

func (a *App) mediaFooter() string {
	return core.MediaFooter(a.mediaCounts(), a.md.kind, a.mediaCaps(), a.md.working)
}

// lookupPlayable maps a video's source path to its playable file (for
// core.MediaDeck / core.ResolveDeck). With a native player the source plays
// as it is, so nothing is substituted.
func (a *App) lookupPlayable(src string) (string, bool) {
	if render.NativeVideoSupported() {
		return "", false
	}
	p, ok := a.md.playable[src]
	return p, ok
}

// wireMedia connects the Media panel's callbacks (called from buildUI).
func (a *App) wireMedia() {
	m := a.win.Media
	m.SetThumbnailFactory(func(i int) Control.Instance {
		if i < 0 || i >= len(a.md.list) {
			return Control.Instance{}
		}
		bg := core.ThumbBackground(a.md.list[i])
		s := models.Slide{Kind: models.SlideMedia, Background: &bg}
		v := render.NewSlideView()
		v.SetTransitionDuration(0)
		v.SetFrame(models.Frame{Mode: models.ModeNormal, Slide: &s, Theme: models.DefaultMediaTheme()})
		return v.AsControl()
	})
	m.OnSearch = func(q string) { a.event(func() { a.loadMedia(q, a.md.kind, nil) }) }
	m.OnFilter = func(kind models.MediaKind) { a.event(func() { a.loadMedia(a.md.query, kind, nil) }) }
	m.OnSelect = func(i int) { a.event(func() { a.selectMedia(i, mediaPreview, nil) }) }
	m.OnActivate = func(i int) { a.event(func() { a.selectMedia(i, mediaGoLive, nil) }) }
	m.OnGoLive = func(i int) { a.event(func() { a.selectMedia(i, mediaGoLive, nil) }) }
	m.OnAddToSchedule = func(i int) { a.event(func() { a.selectMedia(i, mediaSchedule, nil) }) }
	m.OnRemove = func(i int) { a.event(func() { a.removeMedia(i) }) }
	m.OnAddFiles = func() { a.event(a.addMediaFilesDialog) }
	m.OnAddFolder = func() { a.event(a.addMediaFolderDialog) }
	a.wireMediaMenu() // the tile menu's further items (menus_items.go)
}

// mediaRows builds the grid rows (title + badge) for the listed items.
func (a *App) mediaRows() []ui.MediaRow {
	rows := make([]ui.MediaRow, len(a.md.list))
	caps := a.mediaCaps()
	for i, m := range a.md.list {
		playable := m.PlayablePath() != "" && !media.NeedsTranscode(m)
		rows[i] = ui.MediaRow{
			Title: m.Title,
			Badge: core.MediaBadge(m, playable, caps, a.md.progress[m.ID], a.md.failed[m.ID]),
			Thumb: m.ThumbPath,

			Kind: m.Kind, Aspect: m.Aspect, Repeat: m.Repeat,
		}
	}
	return rows
}

// mediaStateDoc summarises the tab for GET /state.
func (a *App) mediaStateDoc() core.MediaState {
	native := render.NativeVideoSupported()
	st := core.MediaState{
		Count: a.mediaCounts().Total(), Videos: a.md.videos, Images: a.md.images,
		Query: a.md.query, Kind: string(a.md.kind), Results: len(a.md.list), Selected: a.md.sel,
		CountText: a.md.status, Working: a.md.working,
		FFmpeg: a.md.tools.Available(), Theora: a.md.tools.CanTranscode(), Tools: a.md.tools.Status(),
		Audio: a.md.audio, Native: native, Player: render.MediaPlayer(),
	}
	if a.md.item != nil {
		st.Current = a.md.item.Title
		st.Aspect, st.Repeat = string(a.md.item.Aspect.Normalized()), string(a.md.item.Repeat.Normalized())
	}
	st.View, st.Sort = string(a.md.view), string(a.md.sort)
	for _, m := range a.md.list {
		if media.NeedsThumbnail(m) {
			st.Pending++
		}
		if m.Kind == models.MediaVideo && (native || !media.NeedsTranscode(m)) {
			st.Playable++
		}
	}
	return st
}

// ---- events -------------------------------------------------------------------------

// loadMedia lists the library for a query and collection in the background.
// Stale results are dropped. The selection follows the selected item's id.
func (a *App) loadMedia(q string, kind models.MediaKind, cmd *control.Command) {
	a.md.query, a.md.kind = q, kind
	a.md.status = a.mediaFooter()
	if a.st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	ticket := a.md.gen.Next()
	st := a.st
	a.async(cmd, func(ctx context.Context) func() error {
		rows, err := st.SearchMedia(ctx, q, kind, 0)
		videos, verr := st.CountMedia(ctx, models.MediaVideo)
		images, ierr := st.CountMedia(ctx, models.MediaImage)
		audio, aerr := st.CountMedia(ctx, models.MediaAudio)
		return func() error {
			if !a.md.gen.Current(ticket) {
				return nil // superseded
			}
			if verr == nil && ierr == nil && aerr == nil {
				a.md.videos, a.md.images, a.md.audio = videos, images, audio
			}
			if err != nil {
				a.fail("search media", err)
				a.md.status = "Search failed"
				return err
			}
			a.md.list = core.SortMedia(rows, a.md.sort)
			a.md.ver++
			a.md.sel = -1
			if a.md.item != nil {
				a.md.sel = core.MediaIndexOf(rows, a.md.item.ID)
			}
			for src, p := range core.Playables(rows) {
				a.md.playable[src] = p
			}
			a.md.status = a.mediaFooter()
			if a.mediaNeedsWork() {
				a.startMediaWork()
			}
			return nil
		}
	})
}

// selectMedia selects tile i and previews it, sends it live or adds it to
// the schedule. The data is in memory, so this is synchronous.
func (a *App) selectMedia(i int, purpose mediaPurpose, cmd *control.Command) {
	if i < 0 || i >= len(a.md.list) {
		a.answer(cmd, fmt.Errorf("no media item at index %d", i))
		return
	}
	m := a.md.list[i]
	a.md.sel = i
	a.md.item = &m
	deck := core.MediaDeck(m, a.lookupPlayable)
	a.md.deck = &deck
	switch purpose {
	case mediaSchedule:
		a.addToSchedule(core.MediaItem(m))
	case mediaGoLive:
		a.previewDeck(&deck)
		a.goLive()
	default:
		a.previewDeck(&deck)
	}
	a.answer(cmd, nil)
}

// addMediaToSchedule adds the selected media item (the resource tab bar's
// Add to Schedule, and the control action).
func (a *App) addMediaToSchedule(cmd *control.Command) {
	if a.md.item == nil {
		a.answer(cmd, errors.New("select a media item first"))
		return
	}
	a.addToSchedule(core.MediaItem(*a.md.item))
	a.answer(cmd, nil)
}

// mediaGoLive sends the selected media item live (the resource tab bar's Go Live).
func (a *App) mediaGoLive() {
	if a.md.sel >= 0 {
		a.selectMedia(a.md.sel, mediaGoLive, nil)
	}
}

// removeMedia asks for confirmation, then removes tile i from the library.
func (a *App) removeMedia(i int) {
	if i < 0 || i >= len(a.md.list) || a.st == nil {
		return
	}
	m := a.md.list[i]
	a.win.Confirm.Ask("Remove Media", "Remove \""+m.Title+"\" from the media library? The file is not deleted.", func() {
		a.event(func() { a.doRemoveMedia(m, nil) })
	})
}

// doRemoveMedia removes an item from the library (after confirmation).
func (a *App) doRemoveMedia(m models.Media, cmd *control.Command) {
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		err := st.DeleteMedia(ctx, m.ID)
		return func() error {
			if err != nil {
				a.alert("Remove Media", fmt.Errorf("Could not remove %q.\n\n%w", m.Title, err))
				a.answer(cmd, err)
				return err
			}
			log.Printf("[freebeams] removed media %d %q", m.ID, m.Title)
			if a.md.item != nil && a.md.item.ID == m.ID {
				a.md.item, a.md.deck = nil, nil
			}
			a.loadMedia(a.md.query, a.md.kind, cmd)
			return nil
		}
	})
}

// addMediaPaths scans files and folders and registers the media files that
// are not in the library yet, then reloads the list (and derives their
// thumbnails). cmd is answered once the list is reloaded.
func (a *App) addMediaPaths(paths []string, cmd *control.Command) {
	if len(paths) == 0 {
		a.answer(cmd, errArgs)
		return
	}
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		items, warnings := media.Scan(paths...)
		var (
			added []*models.Media
			err   error
		)
		for i := range items {
			_, lookupErr := st.MediaByPath(ctx, items[i].Path)
			switch {
			case lookupErr == nil:
				continue // already registered
			case errors.Is(lookupErr, store.ErrNotFound):
				added = append(added, &items[i])
			default:
				err = lookupErr
			}
			if err != nil {
				break
			}
		}
		if err == nil && len(added) > 0 {
			err = st.SaveMediaItems(ctx, added)
		}
		return func() error {
			for _, w := range warnings {
				log.Printf("[freebeams] add media: %s", w)
			}
			if err != nil {
				a.alert("Add Media", fmt.Errorf("Could not add the media files.\n\n%w", err))
				a.answer(cmd, err)
				return err
			}
			if len(items) == 0 {
				err := errors.New("no image, video or audio files found")
				a.alert("Add Media", fmt.Errorf("No image, video or audio files were found in the selection."))
				a.answer(cmd, err)
				return err
			}
			log.Printf("[freebeams] add media: %d found, %d new", len(items), len(added))
			a.loadMedia(a.md.query, a.md.kind, cmd)
			return nil
		}
	})
}

// addMediaFilesDialog shows the native Open dialog for media files.
func (a *App) addMediaFilesDialog() {
	err := DisplayServer.FileDialogShow("Add Media Files", a.dialogDir(), "", false, DisplayServer.FileDialogModeOpenFiles,
		mediaFileFilters, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() { a.addMediaPaths(paths, nil) })
		}, 0)
	if err != nil {
		a.alert("Add Media", fmt.Errorf("Could not show the Open dialog.\n\n%w", err))
	}
}

// addMediaFolderDialog shows the native folder picker.
func (a *App) addMediaFolderDialog() {
	err := DisplayServer.FileDialogShow("Add Media Folder", a.dialogDir(), "", false, DisplayServer.FileDialogModeOpenDir,
		nil, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() { a.addMediaPaths(paths, nil) })
		}, 0)
	if err != nil {
		a.alert("Add Media", fmt.Errorf("Could not show the folder dialog.\n\n%w", err))
	}
}

// startMediaImport registers EasyWorship's media once on libraries that
// finished the Phase 1 import before the media library existed.
func (a *App) startMediaImport() {
	if a.st == nil || !easyworship.Installed() {
		return
	}
	var done bool
	if ok, err := a.st.GetSetting(a.ctx, core.SettingMediaImportDone, &done); err != nil {
		a.fail("read media import setting", err)
	} else if ok && done {
		return
	}
	st := a.st
	a.async(nil, func(ctx context.Context) func() error {
		rep, err := easyworship.ImportMedia(ctx, st, easyworship.Options{})
		if err == nil {
			err = st.SetSetting(ctx, core.SettingMediaImportDone, true)
		}
		return func() error {
			if err != nil {
				a.fail("EasyWorship media import", err)
				return err
			}
			log.Printf("[freebeams] EasyWorship media: %d registered, %d already present", rep.MediaImported, rep.MediaSkipped)
			for _, w := range rep.Warnings {
				log.Printf("[freebeams] media import note: %s", w)
			}
			a.loadMedia(a.md.query, a.md.kind, nil)
			return nil
		}
	})
}

// ---- background pipeline ---------------------------------------------------------

// mediaNeedsWork reports whether a listed item still lacks something the
// pipeline could make (failed items are not retried).
func (a *App) mediaNeedsWork() bool {
	for _, m := range a.md.list {
		if a.md.failed[m.ID] {
			continue
		}
		if media.NeedsThumbnail(m) || media.NeedsDescribe(m) ||
			(media.NeedsTranscode(m) && a.md.tools.CanTranscode() && !render.NativeVideoSupported()) {
			return true
		}
	}
	return false
}

// startMediaWork runs one pass of the pipeline over the whole library in a
// goroutine: metadata and thumbnails first (fast, fills the grid), then
// transcodes one at a time. Results are posted per item. A request while a
// pass runs queues another pass. The job is tracked for shutdown but not
// counted as pending work: a transcode can take minutes.
func (a *App) startMediaWork() {
	if a.st == nil || a.md.cache == "" || !a.md.toolsKnown {
		return
	}
	if a.md.working {
		a.md.again = true
		return
	}
	a.md.working = true
	a.md.status = a.mediaFooter()
	st, ctx := a.st, a.ctx
	proc := media.NewProcessor(a.md.tools, a.md.cache)
	proc.Native = render.NativeVideoSupported() // no transcodes: the sources play as they are
	skip := make(map[int64]bool, len(a.md.failed))
	for id := range a.md.failed {
		skip[id] = true
	}
	a.jobs.Add(1)
	go func() {
		defer a.jobs.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[freebeams] panic in media work: %v\n%s", r, debug.Stack())
			}
			a.post(func() {
				a.md.working = false
				a.md.status = a.mediaFooter()
				if a.md.again {
					a.md.again = false
					a.startMediaWork()
				}
			})
		}()
		a.runMediaWork(ctx, st, proc, skip)
	}()
}

// runMediaWork is the pipeline body (goroutine).
func (a *App) runMediaWork(ctx context.Context, st *store.Store, proc *media.Processor, skip map[int64]bool) {
	items, err := st.SearchMedia(ctx, "", "", 0)
	if err != nil {
		a.post(func() { a.fail("media work", err) })
		return
	}
	save := func(m models.Media) {
		// The operator may have changed the item's Aspect Ratio / Repeating
		// since this pass read it (menus_items.go): keep the current choices.
		if cur, err := st.GetMedia(ctx, m.ID); err == nil {
			m.Aspect, m.Repeat = cur.Aspect, cur.Repeat
		}
		if err := st.SaveMedia(ctx, &m); err != nil {
			a.post(func() { a.fail("save media "+m.Title, err) })
			return
		}
		a.post(func() { a.applyMediaUpdate(m) })
	}
	// Pass 1: metadata and thumbnails.
	for i := range items {
		if ctx.Err() != nil {
			return
		}
		m := &items[i]
		if skip[m.ID] {
			continue
		}
		changed := false
		c, err := proc.Describe(ctx, m)
		changed = changed || c
		if err != nil {
			a.mediaFail(*m, "describe", err)
		}
		c, err = proc.Thumbnail(ctx, m)
		changed = changed || c
		if err != nil {
			a.mediaFail(*m, "thumbnail", err)
		}
		if changed {
			save(*m)
		}
	}
	// Pass 2: transcodes (slow). A native player plays the sources as they are.
	for i := range items {
		if ctx.Err() != nil || proc.Native {
			return
		}
		m := &items[i]
		if skip[m.ID] || !media.NeedsTranscode(*m) {
			continue
		}
		id := m.ID
		last := time.Time{}
		progress := func(f float64) {
			if now := time.Now(); now.Sub(last) >= progressInterval || f >= 1 {
				last = now
				a.post(func() {
					a.md.progress[id] = f
					a.md.thumbVer++
				})
			}
		}
		c, err := proc.Transcode(ctx, m, progress)
		if err != nil {
			if errors.Is(err, media.ErrUnavailable) || errors.Is(err, media.ErrNoTheora) {
				return // nothing more can be done for any video
			}
			a.mediaFail(*m, "transcode", err)
			continue
		}
		if c {
			save(*m)
		}
	}
}

// mediaFail records a derivation failure (the item is not retried this
// session) and logs it. Cancellation is not a failure.
func (a *App) mediaFail(m models.Media, step string, err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	a.post(func() {
		if !a.md.failed[m.ID] {
			a.md.failed[m.ID] = true
			a.md.thumbVer++
			log.Printf("[freebeams] media %s %q: %v", step, m.Title, err)
		}
	})
}

// applyMediaUpdate takes a pipeline result into the state: the listed item,
// the playable map, the selected item's deck and the tile badges.
func (a *App) applyMediaUpdate(m models.Media) {
	delete(a.md.progress, m.ID)
	if i := core.MediaIndexOf(a.md.list, m.ID); i >= 0 {
		a.md.list[i] = m
	}
	a.md.thumbVer++
	if p := m.PlayablePath(); p != "" {
		a.md.playable[m.Path] = p
	}
	if a.md.item != nil && a.md.item.ID == m.ID {
		a.md.item = &m
		deck := core.MediaDeck(m, a.lookupPlayable)
		a.md.deck = &deck
	}
}
