package app

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	_ "unsafe" // go:linkname

	"freebeams/internal/keepalive"
)

// The keepalive check (FREEBEAMS_KEEPALIVE_CHECK): graphics.gd compiles one
// walker per type its per-frame keepalive walk reaches, including the dynamic
// types it meets behind interfaces, into classdb's compiled_keepalives (nil
// for a type with nothing to walk). That cache is the ground truth of what the
// walk reads every frame, so the check reads it (under its own mutex) instead
// of re-implementing the walk's rules, and reports every walked type that
// keepalive.Risk denies: state a goroutine mutates while the main thread
// walks it (see the keepalive package). test/smoke.sh runs with the check on
// and fails on any report.
//
// If a graphics.gd update renames these variables the build fails here, which
// is the signal to look at the new walk.

//go:linkname compiledKeepalives graphics.gd/classdb.compiled_keepalives
var compiledKeepalives map[reflect.Type]func(reflect.Value)

//go:linkname compiledKeepalivesMu graphics.gd/classdb.compiled_keepalives_mu
var compiledKeepalivesMu sync.Mutex

// keepaliveCheckEnv turns the check on: "1" logs the violations, a path also
// writes the sorted list of walked types there at shutdown.
const keepaliveCheckEnv = "FREEBEAMS_KEEPALIVE_CHECK"

// keepaliveCheck is the check's state (main thread only). It is a package
// variable, not an App field: nothing in it may be walked.
var keepaliveCheck struct {
	on       bool
	path     string
	seen     int                   // compiled types already checked
	reported map[reflect.Type]bool // violations logged
}

func initKeepaliveCheck() {
	v := os.Getenv(keepaliveCheckEnv)
	if v == "" || v == "0" {
		return
	}
	keepaliveCheck.on = true
	keepaliveCheck.reported = map[reflect.Type]bool{}
	if v != "1" {
		keepaliveCheck.path = v
	}
	log.Printf("[freebeams] keepalive check on")
}

// walkedTypes returns the types the keepalive walk descends into: those
// with a non-nil compiled walker.
func walkedTypes() []reflect.Type {
	compiledKeepalivesMu.Lock()
	defer compiledKeepalivesMu.Unlock()
	var types []reflect.Type
	for t, walk := range compiledKeepalives {
		if walk != nil {
			types = append(types, t)
		}
	}
	return types
}

// checkKeepalive logs every newly walked type keepalive.Risk denies. It is
// cheap when nothing new was compiled (one locked len), so Process calls it
// every frame while the check is on.
func checkKeepalive() {
	if !keepaliveCheck.on {
		return
	}
	compiledKeepalivesMu.Lock()
	n := len(compiledKeepalives)
	compiledKeepalivesMu.Unlock()
	if n == keepaliveCheck.seen {
		return
	}
	keepaliveCheck.seen = n
	for _, t := range walkedTypes() {
		if keepaliveCheck.reported[t] {
			continue
		}
		if why := keepalive.Risk(t); why != "" {
			keepaliveCheck.reported[t] = true
			log.Printf("[keepalive-check] walked %v: %s", t, why)
		}
	}
}

// writeKeepaliveTypes writes the walked types, sorted, one per line (a
// violation is marked "DENIED"), to the check's path.
func writeKeepaliveTypes() {
	if !keepaliveCheck.on {
		return
	}
	checkKeepalive()
	if keepaliveCheck.path == "" {
		return
	}
	var lines []string
	for _, t := range walkedTypes() {
		line := t.String()
		if why := keepalive.Risk(t); why != "" {
			line += "\tDENIED: " + why
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(keepaliveCheck.path, []byte(body), 0o644); err != nil {
		log.Printf("[freebeams] keepalive check: %v", fmt.Errorf("write %s: %w", keepaliveCheck.path, err))
		return
	}
	log.Printf("[freebeams] keepalive check: %d walked types written to %s", len(lines), keepaliveCheck.path)
}
