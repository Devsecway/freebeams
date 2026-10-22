// Package keepalive keeps Go-only state out of graphics.gd's per-frame
// keepalive walk, and names the types that must never be in it.
//
// Every frame, on the main thread, graphics.gd (classdb keepalive.go,
// keep_reachable_instances_alive) walks the fields of every extension
// instance by reflection, exported or not. It follows pointers, slices,
// arrays, maps (iterating them) and interfaces (into their dynamic type) down
// to any engine Instance, so that RefCounted objects a Go field holds stay
// alive. It skips func, chan and unsafe.Pointer fields, scalars, exported
// Node-typed fields of extension classes, and pointers to other extension
// classes (they are roots of their own).
//
// A structure that a goroutine mutates must therefore never be reachable from
// an extension instance through walked fields: the walk reads it on the main
// thread without a lock. A map is a "fatal error: concurrent map iteration and
// map write" that kills the app; a slice or an interface being replaced is a
// torn read. Services (the store's *sql.DB, the HTTP servers, the media
// pipelines, the players' decoder goroutines) go behind a [Hidden] instead,
// which the Go garbage collector still traces.
//
// The package is pure Go (no graphics.gd), so it is unit-tested with go test.
package keepalive

import (
	"reflect"
	"strings"
	"unsafe"
)

// Hidden is a pointer that graphics.gd's keepalive walk does not follow: its
// only field is an unsafe.Pointer, which the walk skips and the Go garbage
// collector traces like any other pointer. The zero value holds nil.
//
// Hide only Go-only state. An engine object (an Instance, or a struct holding
// one) behind a Hidden is invisible to the walk, so a RefCounted that only it
// references is released at the end of the frame: keep engine Instances in
// walked fields, or pin them (see CLAUDE.md).
type Hidden[T any] struct{ p unsafe.Pointer }

// Hide wraps p (nil gives the zero Hidden).
func Hide[T any](p *T) Hidden[T] { return Hidden[T]{p: unsafe.Pointer(p)} }

// Get returns the hidden pointer (nil when nothing is held).
func (h Hidden[T]) Get() *T { return (*T)(h.p) }

// Nil reports whether h holds nothing.
func (h Hidden[T]) Nil() bool { return h.p == nil }

// denied are the packages whose values goroutines mutate while they are in
// use: the walk must never reach them. A path ending in "/" denies the
// packages under it.
var denied = []string{
	// The standard library's concurrent machinery.
	"context",      // cancelCtx.children: every derived context writes it
	"database/sql", // DB.dep, DB.openStmt, driverConn.openStmt
	"net",          // listeners and connections
	"net/",         // net/http servers, transports and connections
	"crypto/tls",   // connection state
	"os/exec",      // Cmd: the wait and pipe-copy goroutines write it
	"os/signal",    // signal handlers
	"sync",         // sync.Map, sync.Pool, sync.Cond: shared by design
	"sync/atomic",  // atomic.Value: written from other goroutines by design
	"io",           // io.PipeReader / io.PipeWriter: the other end is a goroutine
	"bufio",        // readers and writers over pipes and connections
	"log",          // log.Logger: every goroutine that logs writes it
	"runtime",      // runtime state
	// The standard library's vendored x/net (the HTTP/2 server's hpack
	// encoder, written per response).
	"vendor/golang.org/x/net/",

	// Third-party services.
	"golang.org/x/net/",            // HTTP/2 and friends outside the standard library
	"github.com/gorilla/websocket", // the remote's connections
	"modernc.org/sqlite",           // the driver's connections and statements
	"modernc.org/libc",             // the driver's C runtime

	// Our own services that run goroutines or are used from them.
	"freebeams/internal/store",         // the library (wraps *sql.DB)
	"freebeams/internal/control",       // the control HTTP server
	"freebeams/internal/remote",        // the LAN remote: HTTP, WebSocket hub, clients
	"freebeams/internal/media",         // the media pipeline (probe, posters, transcodes)
	"freebeams/internal/media/",        // the ffmpeg player and its decoder goroutines
	"freebeams/internal/presentations", // the presentation render pipeline
	"freebeams/internal/importers/",    // the EasyWorship and file importers
	"freebeams/internal/bibles/",       // the bundled Bibles installer
	"freebeams/internal/sysproc",       // child processes
}

// Risk returns why the keepalive walk must not reach t, or "" when it may.
// A named type is judged by its package (see denied; in package time only
// Timer and Ticker, which the runtime's timers share). Unnamed pointers,
// slices, arrays and maps are judged by their elements. Interface types are
// not judged: the walk compiles the dynamic type behind each one it meets,
// and that type is judged on its own.
func Risk(t reflect.Type) string {
	switch {
	case t.Kind() == reflect.Interface:
		return ""
	case t.Name() == "" && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array):
		return Risk(t.Elem())
	case t.Name() == "" && t.Kind() == reflect.Map:
		if why := Risk(t.Key()); why != "" {
			return why
		}
		return Risk(t.Elem())
	}
	pkg := t.PkgPath()
	if pkg == "time" {
		if t.Name() == "Timer" || t.Name() == "Ticker" {
			return "time." + t.Name() + " is shared with the runtime's timers"
		}
		return ""
	}
	for _, d := range denied {
		if pkg == d || (strings.HasSuffix(d, "/") && strings.HasPrefix(pkg, d)) {
			return "package " + pkg + " is mutated by goroutines"
		}
	}
	return ""
}
