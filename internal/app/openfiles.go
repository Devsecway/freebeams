package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"freebeams/internal/app/core"
	"freebeams/internal/control"

	"graphics.gd/classdb/OS"
	"graphics.gd/classdb/SceneTree"
)

// Opening files from outside the app: a double-clicked schedule (.fbs, the
// legacy .fws) or Bible package (.fbbible), "Open with FreeBeams", a file
// dropped on the app icon. The installers register those types; the system
// starts FreeBeams with the paths as arguments (macOS: Godot's openFiles
// delegate turns them into arguments too). A schedule opens as File ▸ Open
// would; a package opens the Bible Manager and goes through its Add (the
// licence question first). `open-files {"paths": [...]}` is the same path for
// automation and for a second launch, which hands its files to the window
// that already has the library open (the instance file beside the library)
// and quits instead of opening a second window on the same library.

// launchArgs are the command-line arguments that may name files: Godot's
// (unknown positional arguments stay there) and those after "--".
func launchArgs() []string {
	return append(OS.GetCmdlineArgs(), OS.GetCmdlineUserArgs()...)
}

func isRegularFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

func launchFiles() core.OpenFiles {
	cwd, _ := os.Getwd()
	return core.ClassifyOpenFiles(launchArgs(), cwd, isRegularFile)
}

// handOverLaunchFiles gives this launch's files to a running instance on the
// same library. It reports true when that worked and this launch is quitting.
func (a *App) handOverLaunchFiles() bool {
	files := launchFiles()
	if files.Empty() {
		a.opening = files
		return false
	}
	if lib, err := libraryPath(); err == nil {
		if in, err := control.ReadInstance(control.InstancePath(lib)); err == nil && in.PID != os.Getpid() {
			if err := control.Forward(in, "open-files", map[string][]string{"paths": files.Paths()}); err == nil {
				log.Printf("[freebeams] handed %q to the running FreeBeams (pid %d); quitting", files.Paths(), in.PID)
				a.quitting = true
				SceneTree.Get(a.AsNode()).Quit()
				return true
			} else {
				log.Printf("[freebeams] no running FreeBeams took the files (%v); opening them here", err)
			}
		}
	}
	a.opening = files
	return false
}

// publishInstance writes the instance file once the library and the control
// server are up.
func (a *App) publishInstance() {
	if a.st() == nil || a.ctrl() == nil {
		return
	}
	path := control.InstancePath(a.st().Path())
	if err := control.WriteInstance(path, a.ctrl()); err != nil {
		log.Printf("[freebeams] %v (a second launch will open its own window)", err)
		return
	}
	a.instanceFile = path
}

func (a *App) unpublishInstance() {
	if a.instanceFile != "" && a.ctrl() != nil {
		control.RemoveInstance(a.instanceFile, a.ctrl())
		a.instanceFile = ""
	}
}

// openFilesCommand is `open-files {"paths": [...]}`: it queues the files.
func (a *App) openFilesCommand(cmd *control.Command, paths []string) {
	files := core.ClassifyOpenFiles(paths, "", isRegularFile)
	if files.Empty() {
		a.answer(cmd, fmt.Errorf("open-files: nothing to open in %q (schedules .fbs / .fws and Bible packages .fbbible)", paths))
		return
	}
	a.opening.Schedule = files.Schedule
	a.opening.Bibles = append(a.opening.Bibles, files.Bibles...)
	a.opening.Ignored = append(a.opening.Ignored, files.Ignored...)
	a.answer(cmd, nil) // they open on the next frames: wait-idle, then /state
}

// openPendingFiles opens what is queued, once nothing it would collide with
// is going on: the first-run Bible install, a Bible Manager task, a question
// on screen. A schedule opens first (asking before it replaces unsaved
// changes); then the first Bible package.
func (a *App) openPendingFiles() {
	o := &a.opening
	if o.Empty() && len(o.Ignored) == 0 {
		return
	}
	for _, p := range o.Ignored {
		log.Printf("[freebeams] open: %q is not a schedule or Bible package FreeBeams can open; ignored", p)
	}
	o.Ignored = nil
	if a.win == nil || a.win.Confirm.Visible() || a.bundled.Running || a.bm.task != "" {
		return
	}
	if path := o.Schedule; path != "" {
		o.Schedule = ""
		log.Printf("[freebeams] open: schedule %s", path)
		// openSchedule shows a failure to the operator itself.
		a.confirmDiscard("Open Schedule", func() { a.openSchedule(path) })
		a.dirty = true
		return
	}
	if len(o.Bibles) == 0 || a.st() == nil {
		return
	}
	// One package at a time: its file is read in the background and then
	// asks its question, and a second one would replace that question.
	path := o.Bibles[0]
	for _, p := range o.Bibles[1:] {
		log.Printf("[freebeams] open: one Bible package at a time; %s was not added (open it again)", filepath.Base(p))
	}
	o.Bibles = nil
	log.Printf("[freebeams] open: Bible package %s", filepath.Base(path))
	a.openBibleManager(nil)
	a.addBiblePath(nil, path, false)
	a.dirty = true
}
