// Package app is the FreeBeams application node: it wires the library store,
// the presenter state machine, the schedule, the operator UI, the renderer and
// the control server together.
//
// Architecture (see CLAUDE.md, "UI rule"): authoritative state lives in plain
// Go (models.Presenter, models.Schedule and the App's own fields). Events
// (widget callbacks, hotkeys, control commands, async results) mutate that
// state and set the dirty flag. Process then runs one idempotent render()
// pass that projects the state onto the Godot nodes. render never writes
// state; it only updates its own view cache (what it last applied), which is
// how it skips rebuilding unchanged widgets.
package app

import (
	"context"
	"errors"
	"fmt"
	"freebeams/internal/appdirs"
	"log"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/importers/easyworship"
	"freebeams/internal/keepalive"
	"freebeams/internal/models"
	"freebeams/internal/render"
	"freebeams/internal/store"
	"freebeams/internal/ui"

	"graphics.gd/classdb"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Object"
)

// RegisterAll registers the app's Godot classes.
func RegisterAll() {
	classdb.Register[App]()
}

// errNoLibrary is returned when the library database could not be opened.
var errNoLibrary = errors.New("the song library is not available")

// idleFrames is how many consecutive idle frames wait-idle needs, so that
// the last change has been drawn (viewport textures hold the previous frame).
const idleFrames = 3

// resourcePreview is what the resource preview shows for one tab.
type resourcePreview struct {
	deck  *models.Deck
	index int
}

// App is the FreeBeams root node (FwApp).
type App struct {
	Node.Extension[App] `gd:"FwApp"`

	// ---- services
	//
	// graphics.gd walks every field of an extension instance each frame (on
	// the main thread, without a lock) and follows pointers, maps, slices and
	// interfaces. Whatever a goroutine mutates must stay out of that walk, or
	// a map written meanwhile is "concurrent map iteration and map write"
	// (the app dies): services sit behind a keepalive.Hidden (see
	// internal/keepalive; read them through st() / ctrl()), and the context
	// behind a func (a *cancelCtx's children map is written by every derived
	// context). test/smoke.sh fails when the walk reaches such a type
	// (FREEBEAMS_KEEPALIVE_CHECK, keepalivecheck.go).
	inputRec keepalive.Hidden[inputRecorder]  // FREEBEAMS_INPUT_LOG debugging aid (*os.File, *bufio.Writer)
	lib      keepalive.Hidden[store.Store]    // the library: *sql.DB, used from every async job
	ctrlSrv  keepalive.Hidden[control.Server] // the control HTTP server
	ctx      func() context.Context
	cancel   context.CancelFunc

	// results carries main-thread continuations of async work.
	results chan func()
	pending int            // async jobs in flight
	jobs    sync.WaitGroup // async goroutines, awaited (bounded) at shutdown

	// ---- authoritative state
	pres  *models.Presenter
	sched *models.Schedule
	keys  core.Hotkeys

	// schedSel is the selected schedule row: the item Preview was loaded from
	// (-1 when Preview holds library content). setPreview keeps it in step.
	schedSel int
	schedVer int // bumped on every schedule mutation (rebuilds rows/thumbnails)

	songQuery   string
	songs       []models.SongSummary
	songsVer    int
	songSel     int
	song        *models.Song // loaded (selected) song
	songCount   int
	songsStatus string // Songs footer text ("223 songs", "Importing…")
	searchGen   core.Generation
	songSeq     core.Sequencer // song loads apply in request order

	bibles    []models.Bible
	biblesVer int
	bibleSel  int

	reference     string
	passage       *models.Passage
	passageBible  models.Bible
	scriptureDeck *models.Deck
	verseRows     []string
	versesVer     int
	verseSel      core.RowSelection // the selected rows of the verse list (scripture.go)
	scriptStatus  string
	scriptErr     bool
	verseGen      core.Generation

	// The verse list and the reference field (scripture.go).
	listVerses []models.Verse // the verses of the listed chapters, one per row
	refText    string         // the reference field's text (typed, or set by the app)
	refVer     int            // bumped when the app sets the field's text
	refFocused bool           // the reference field has keyboard focus
	suggestSel int            // the highlighted book suggestion
	suggestOff string         // the text whose suggestions Esc dismissed

	songRes resourcePreview // the Scriptures preview derives from verseSelection

	md mediaState // the Media tab (media.go)

	mdProps propsFor // the media item the Properties dialog edits (mediaprops.go)

	th themeState // the Themes tab and theme editor (themes.go)

	pr presentationState // the Presentations tab (presentations.go)

	pb playbackState // video / audio transport per monitor (playback.go)

	stage stageState // the stage display / Foldback output (stage.go)

	rm remoteApp // the LAN remote control (remote.go)

	al alertsState // the alert overlay and the saved alerts (alerts.go)

	au audioState // the schedule's background audio playlist (audio.go)

	ps previewStyleState // restyling the item in Preview (previewstyle.go)

	thp themePolishState // video backgrounds, apply-to-schedule, theme files (themepolish.go)

	sp spellState // the song editor's spelling checker (spell.go)

	importState core.ImportState
	ewBibles    []easyworship.BibleFile // EasyWorship's Bible files (scanEWBibles)

	bundled   core.BundledState // the bundled translations' installation (bundled.go)
	bibleFile *bibleFileImport  // a parsed Bible file awaiting confirmation (biblefile.go)

	layout           core.Layout // splitter positions, persisted (layout.go)
	layoutSaveQueued bool

	settings     core.Settings // the Options dialog's values, persisted per key (settings.go)
	settingsVer  int           // bumped on every settings change (re-projects the dialog)
	settingsOpen bool          // the Options dialog is up
	settingsPage int           // the section it opens on (ui.SettingsPage…)
	settingsSnap core.Settings // the settings when the dialog opened (Cancel restores them)
	recent       []string      // recent schedule files, newest first (File ▸ Open Recent)

	// opt is the Options dialog's hotkey and default-theme editors (hotkeys.go).
	opt optionsState

	// logoDir holds the built-in Logo screen image (defaultLogoDir, logo.go);
	// "" keeps the gradient as the default logo.
	logoDir string

	prevLive      *models.Deck // for Go Back
	prevLiveIndex int

	lastError string
	alerts    []core.Alert // errors for the operator; the first is on screen

	// ---- view
	win         *ui.MainWindow
	previewView *render.SlideView
	liveView    *render.SlideView
	resView     *render.SlideView
	logoView    *render.SlideView // the Options dialog's logo preview
	out         *render.Output

	dirty    bool
	view     viewCache
	waiters  []*control.Command
	replies  []func() // successful control replies, sent after render
	timers   []timer  // main-thread callbacks due in a few frames
	idleRun  int
	quitNext bool
	quitting bool

	// bm is the Bible Manager (biblemanager.go).
	bm bibleManagerState

	// Drag and drop into the schedule (dragdrop.go): while drop.inserting,
	// addToSchedule inserts at drop.at instead of appending; drop.held is
	// the pointer of a drag-to-schedule hook left pressed (mode "hold").
	drop dropState

	// Files given on the command line or by `open-files` (openfiles.go),
	// opened once the library is ready; instanceFile is where this launch
	// published its control server for later launches to hand files to.
	opening      core.OpenFiles
	instanceFile string
}

// New returns the app node; add it to the scene tree (main.go does this).
func New() *App { return new(App) }

// st is the library (nil when it could not be opened).
func (a *App) st() *store.Store { return a.lib.Get() }

// ctrl is the control server (nil when it could not start).
func (a *App) ctrl() *control.Server { return a.ctrlSrv.Get() }

// Ready builds the UI and starts the services.
func (a *App) Ready() {
	ctx, cancel := context.WithCancel(context.Background())
	a.ctx, a.cancel = func() context.Context { return ctx }, cancel
	initKeepaliveCheck() // FREEBEAMS_KEEPALIVE_CHECK (keepalivecheck.go)
	a.results = make(chan func(), 64)
	a.pres = models.NewPresenter()
	a.sched = models.NewSchedule()
	a.schedSel, a.songSel, a.bibleSel = -1, -1, -1
	a.verseSel = core.NewRowSelection(nil)
	a.view = newViewCache()
	a.inputRec = keepalive.Hide(openInputRecorder())
	a.initPlayback()

	SceneTree.Get(a.AsNode()).SetAutoAcceptQuit(false)
	if a.handOverLaunchFiles() { // another window has this library open: it opens them (openfiles.go)
		return
	}
	setAppName(appdirs.Name) // before buildUI installs the menu bar's own menus
	a.buildUI()
	a.openLibrary()
	a.restoreLayout()
	a.startControl()
	a.publishInstance() // openfiles.go
	log.Printf("[freebeams] %s", versionBanner())

	a.loadSettings() // transition, output display, logo, interface color (settings.go)
	a.initRemote()   // the LAN remote control, when it is enabled (remote.go)

	a.loadStageSettings() // the stage display (stage.go)

	a.initSpell() // the song editor's spelling checker (spell.go)

	// The projector output opens by itself only when a second display exists.
	screens := render.Screens()
	if len(screens) > 1 {
		a.pres.SetOutputEnabled(true)
	}
	log.Printf("[freebeams] %d screen(s); output %s", len(screens), map[bool]string{true: "on", false: "off until Live is pressed"}[len(screens) > 1])

	a.startBundledInstall()
	a.startImport()
	a.initThemes()
	a.scanEWBibles()
	a.searchSongs("", nil)
	a.loadBibles(nil)
	a.initMedia()
	a.initPresentations()
	a.initAlerts()
	a.initAudio()
	a.dirty = true
}

// openLibrary opens the SQLite library (FREEBEAMS_LIBRARY overrides the
// default path).
func (a *App) openLibrary() {
	path, err := libraryPath()
	if err != nil {
		a.fail("library path", err)
		a.songsStatus = "Library unavailable"
		return
	}
	st, err := store.Open(path)
	if err != nil {
		a.fail("open library", err)
		a.songsStatus = "Library unavailable: " + err.Error()
		return
	}
	a.lib = keepalive.Hide(st)
	log.Printf("[freebeams] library: %s", st.Path())
}

// libraryPath is the library file: FREEBEAMS_LIBRARY, else the default.
func libraryPath() (string, error) {
	if path := os.Getenv(core.LibraryEnv); path != "" {
		return path, nil
	}
	return store.DefaultPath()
}

func (a *App) startControl() {
	ctrl := control.New(0)
	if err := ctrl.Start(); err != nil {
		a.fail("start control server", err)
		return
	}
	a.ctrlSrv = keepalive.Hide(ctrl)
}

// startImport runs the first-run EasyWorship import in the background.
func (a *App) startImport() {
	if a.st() == nil {
		return
	}
	var rec core.ImportRecord
	done, err := a.st().GetSetting(a.ctx(), core.SettingImportDone, &rec)
	if err != nil {
		a.fail("read import setting", err)
	}
	if done {
		a.importState = core.ImportState{Done: true, Summary: rec.Summary}
		a.startMediaImport() // libraries imported before the media library existed
		return
	}
	if !easyworship.Installed() {
		a.importState = core.ImportState{Summary: "EasyWorship is not installed"}
		log.Printf("[freebeams] EasyWorship not installed; the library starts empty")
		return
	}
	a.importState = core.ImportState{Running: true}
	a.songsStatus = core.SongsFooter(0, a.importState)
	st := a.st()
	opt := a.ewImportOptions()
	log.Printf("[freebeams] importing from EasyWorship…")
	a.async(nil, func(ctx context.Context) func() error {
		rep, err := easyworship.ImportInto(ctx, st, opt)
		summary := core.ImportSummary(rep)
		if err == nil {
			err = st.SetSetting(ctx, core.SettingImportDone, core.ImportRecord{Done: true, Summary: summary})
		}
		if err == nil {
			err = st.SetSetting(ctx, core.SettingMediaImportDone, true) // ImportInto covered media
		}
		return func() error {
			a.importState.Running = false
			if err != nil {
				// The footer keeps saying so (core.SongsFooter) and a dialog
				// shows the details; a later search must not hide the failure.
				a.importState.Error = err.Error()
				a.alert("EasyWorship Import", fmt.Errorf("The EasyWorship import failed, so the library may be incomplete.\n\n%w", err))
			} else {
				a.importState.Done, a.importState.Summary = true, summary
				log.Printf("[freebeams] EasyWorship import: %s", summary)
				for _, w := range rep.Warnings {
					log.Printf("[freebeams] import note: %s", w)
				}
			}
			a.songsStatus = core.SongsFooter(len(a.songs), a.importState)
			a.searchSongs(a.songQuery, nil)
			a.loadBibles(nil)
			a.loadMedia(a.md.query, a.md.kind, nil)
			return nil
		}
	})
}

// ---- async plumbing ----------------------------------------------------------

// async runs work on a goroutine. work returns the continuation, which runs on
// the main thread (from Process). cmd, when not nil, is answered after the
// continuation: with its error, or with the state.
func (a *App) async(cmd *control.Command, work func(ctx context.Context) func() error) {
	a.pending++
	ctx := a.ctx()
	a.jobs.Add(1)
	go func() {
		defer a.jobs.Done()
		var apply func() error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err := fmt.Errorf("panic in background task: %v", r)
					log.Printf("[freebeams] %v\n%s", err, debug.Stack())
					apply = func() error { return err }
				}
			}()
			apply = work(ctx)
		}()
		a.post(func() {
			a.pending--
			var err error
			if apply != nil {
				err = apply()
			}
			if cmd != nil {
				a.answer(cmd, err)
			}
		})
	}()
}

// post queues fn for the main thread. It never blocks after shutdown.
func (a *App) post(fn func()) {
	select {
	case a.results <- fn:
	case <-a.ctx().Done():
	}
}

// event wraps a widget callback: it recovers panics (the app must never
// crash) and marks the state dirty.
func (a *App) event(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			a.fail("internal error", fmt.Errorf("%v", r))
			log.Printf("%s", debug.Stack())
		}
	}()
	fn()
	a.dirty = true
}

// fail logs an error with context and records it as the last error.
func (a *App) fail(context string, err error) {
	if err == nil {
		return
	}
	a.lastError = context + ": " + err.Error()
	log.Printf("[freebeams] %s", a.lastError)
}

// maxAlerts bounds the queue of error messages waiting to be shown.
const maxAlerts = 5

// alert logs a user-facing failure (like fail) and shows it to the operator
// in the message dialog. render projects a.alerts onto the dialog; closing it
// shows the next one.
func (a *App) alert(title string, err error) {
	if err == nil {
		return
	}
	a.fail(title, err)
	if len(a.alerts) < maxAlerts {
		a.alerts = append(a.alerts, core.Alert{Title: title, Message: err.Error()})
	}
}

// dismissAlert removes the alert on screen (the message dialog was closed).
func (a *App) dismissAlert() {
	if len(a.alerts) > 0 {
		a.alerts = a.alerts[1:]
	}
}

// ---- frame loop ----------------------------------------------------------------

// Process drains async results and control commands, then renders.
func (a *App) Process(delta Float.X) {
	if a.quitting {
		return
	}
	checkKeepalive() // FREEBEAMS_KEEPALIVE_CHECK: nothing goroutine-shared is walked
	for drained := false; !drained; {
		select {
		case fn := <-a.results:
			a.event(fn)
		default:
			drained = true
		}
	}
	if a.ctrl() != nil {
		for drained := false; !drained; {
			select {
			case cmd := <-a.ctrl().Commands():
				a.event(func() { a.handleCommand(cmd) })
			default:
				drained = true
			}
		}
	}
	a.processRemote() // phone actions and pairing changes (remote.go)
	a.runTimers()
	a.openPendingFiles() // openfiles.go
	if a.quitNext {
		a.shutdown()
		return
	}
	if a.playbackEvents() { // a file ended, a new item arrived on a monitor (playback.go)
		a.dirty = true
	}
	a.tickAlerts() // a finished alert gives way to the next (alerts.go)

	// The audio playlist: a track ended or failed, a fade-out finished (audio.go).
	if a.audioEvents() {
		a.dirty = true
	}
	if a.dirty || a.pres.State.Seq != a.view.seq {
		a.render()
		a.dirty = false
	} else if render.AnyMediaActive() {
		a.renderPlayback() // the transport follows the player every frame
		a.renderAudio()    // the playlist's position and fade-out too (audio.go)
	}
	a.tickStage() // the stage display's clock and timer (stage.go)

	a.renderOverlay() // the alert layer moves every frame (alerts.go)
	a.flushReplies()
	a.serveWaiters()
	a.serveProgressWaiters() // wait-transition (hotkeys.go)
}

// timer is a callback due after a number of frames.
type timer struct {
	frames int
	fn     func()
}

// after runs fn on the main thread frames frames from now.
func (a *App) after(frames int, fn func()) {
	a.timers = append(a.timers, timer{frames: frames, fn: fn})
}

// runTimers counts the timers down and runs the due ones.
func (a *App) runTimers() {
	if len(a.timers) == 0 {
		return
	}
	var due []func()
	keep := a.timers[:0]
	for _, t := range a.timers {
		if t.frames--; t.frames <= 0 {
			due = append(due, t.fn)
		} else {
			keep = append(keep, t)
		}
	}
	a.timers = keep
	for _, fn := range due {
		a.event(fn)
	}
}

// busy reports async work or a running transition.
func (a *App) busy() bool {
	return a.pending > 0 || len(a.timers) > 0 || a.transitioning() || a.dirty || a.spellBusy()
}

func (a *App) transitioning() bool {
	if a.previewView.Transitioning() || a.liveView.Transitioning() {
		return true
	}
	if v := a.out.View(); v != nil && v.Transitioning() {
		return true
	}
	return false
}

// serveWaiters answers wait-idle commands once the app has been idle for a
// few frames.
func (a *App) serveWaiters() {
	if a.busy() {
		a.idleRun = 0
		return
	}
	if a.idleRun < idleFrames {
		a.idleRun++
	}
	if a.idleRun < idleFrames || len(a.waiters) == 0 {
		return
	}
	ws := a.waiters
	a.waiters = nil
	for _, w := range ws {
		w.RespondJSON(a.state())
	}
}

// Notification handles the window close request.
func (a *App) Notification(what Object.Notification) {
	if what == Node.NotificationWmCloseRequest {
		a.event(a.requestQuit)
	}
}

// requestQuit asks before discarding an open song editor, then before
// discarding unsaved schedule changes, then quits. Cancel keeps everything
// as it was (the editor stays open).
func (a *App) requestQuit() {
	if a.quitting || a.win.Confirm.Visible() {
		return // already quitting, or already asking
	}
	if a.win.Editor.Visible() {
		a.win.Confirm.Ask("Quit FreeBeams", "The song editor is open. Quit without saving the song?", func() {
			a.event(func() {
				a.win.Editor.Close()
				// Ask about the schedule next frame, once this dialog is
				// down (it is reparented when it opens again).
				a.after(1, a.quitIfScheduleSaved)
			})
		})
		return
	}
	a.quitIfScheduleSaved()
}

// quitIfScheduleSaved quits, asking first when the schedule has unsaved changes.
func (a *App) quitIfScheduleSaved() {
	if a.sched.Dirty {
		a.win.Confirm.Ask("Quit FreeBeams", "The schedule \""+a.sched.Title+"\" has unsaved changes. Quit without saving?", func() {
			a.event(func() { a.quitNext = true })
		})
		return
	}
	a.quitNext = true
}

// shutdownWait bounds how long shutdown waits for background jobs (a
// cancelled import unwinding its transaction) before closing the library.
const shutdownWait = 3 * time.Second

// shutdown stops the output, releases render caches, closes the library and
// quits. It is safe to call more than once.
func (a *App) shutdown() {
	if a.quitting {
		return
	}
	a.quitting = true
	log.Printf("[freebeams] shutting down")
	a.cancel()
	a.flushReplies()
	for _, w := range a.waiters {
		w.RespondErr(errors.New("app is quitting"))
	}
	a.waiters = nil
	a.stopRemote() // phones disconnect; the devices are saved before the library closes
	a.unpublishInstance()
	a.out.OnClosed = nil
	a.out.OnInput = nil
	a.out.Close()
	a.closeStage()
	render.ReleaseCaches()
	// The jobs see the cancelled context; wait (bounded) for them to stop
	// using the store, e.g. an import rolling back its write transaction.
	// Their continuations are dropped (post gives up after cancel).
	if !core.WaitTimeout(&a.jobs, shutdownWait) {
		log.Printf("[freebeams] background jobs still running after %v; closing anyway", shutdownWait)
	}
	if a.st() != nil {
		if err := a.st().Close(); err != nil {
			log.Printf("[freebeams] close library: %v", err)
		}
		a.lib = keepalive.Hidden[store.Store]{}
	}
	writeKeepaliveTypes()
	SceneTree.Get(a.AsNode()).Quit()
}
