package keepalive

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"net/http"
	"os/exec"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	"freebeams/internal/control"
	"freebeams/internal/media"
	"freebeams/internal/media/ffplayer"
	"freebeams/internal/models"
	"freebeams/internal/presentations"
	"freebeams/internal/remote"
	"freebeams/internal/store"

	"github.com/gorilla/websocket"
)

func TestHidden(t *testing.T) {
	var zero Hidden[int]
	if !zero.Nil() || zero.Get() != nil {
		t.Fatalf("zero Hidden holds %v", zero.Get())
	}
	v := 42
	h := Hide(&v)
	if h.Nil() || h.Get() != &v || *h.Get() != 42 {
		t.Fatalf("Hide(&v).Get() = %v, want %p", h.Get(), &v)
	}
	if Hide[int](nil) != zero {
		t.Fatal("Hide(nil) is not the zero Hidden")
	}
	// The walk skips unsafe.Pointer fields: a Hidden must be nothing else.
	rt := reflect.TypeFor[Hidden[sql.DB]]()
	if rt.NumField() != 1 || rt.Field(0).Type.Kind() != reflect.UnsafePointer {
		t.Fatalf("Hidden's fields changed: %v", rt)
	}
}

// TestHiddenKeepsTargetAlive checks that the garbage collector traces the
// unsafe.Pointer: the hidden value must not be collected while held.
func TestHiddenKeepsTargetAlive(t *testing.T) {
	type big struct{ b [1 << 16]byte }
	collected := make(chan struct{})
	h := func() Hidden[big] {
		p := &big{}
		p.b[0] = 7
		runtime.AddCleanup(p, func(c chan struct{}) { close(c) }, collected)
		return Hide(p)
	}()
	for range 5 {
		runtime.GC()
	}
	select {
	case <-collected:
		t.Fatal("a value held only by a Hidden was collected")
	case <-time.After(50 * time.Millisecond):
	}
	if h.Get().b[0] != 7 {
		t.Fatal("hidden value corrupted")
	}
	runtime.KeepAlive(h)
}

type fakeStoreLike struct{ db *sql.DB }

func TestRisk(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tests := []struct {
		name string
		t    reflect.Type
		bad  bool
	}{
		{"sql.DB", reflect.TypeFor[sql.DB](), true},
		{"*sql.DB", reflect.TypeFor[*sql.DB](), true},
		{"map key from sql", reflect.TypeFor[map[*sql.Conn]bool](), true},
		{"slice of http servers", reflect.TypeFor[[]*http.Server](), true},
		{"cancelCtx (dynamic type)", reflect.TypeOf(ctx), true},
		{"exec.Cmd", reflect.TypeFor[exec.Cmd](), true},
		{"bufio.Writer", reflect.TypeFor[bufio.Writer](), true},
		{"sync.Map", reflect.TypeFor[sync.Map](), true},
		{"time.Timer", reflect.TypeFor[time.Timer](), true},
		{"time.Time", reflect.TypeFor[time.Time](), false},
		{"*time.Location", reflect.TypeFor[*time.Location](), false},
		{"context.Context interface", reflect.TypeFor[context.Context](), false},
		{"error interface", reflect.TypeFor[error](), false},
		{"fs.PathError", reflect.TypeFor[fs.PathError](), false},
		{"errors.New dynamic type", reflect.TypeOf(errors.New("x")), false},
		{"own struct holding a DB", reflect.TypeFor[fakeStoreLike](), false}, // its field is judged on its own
		{"Hidden", reflect.TypeFor[Hidden[sql.DB]](), false},
		{"unsafe.Pointer", reflect.TypeFor[unsafe.Pointer](), false},
		{"int", reflect.TypeFor[int](), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			why := Risk(tt.t)
			if (why != "") != tt.bad {
				t.Fatalf("Risk(%v) = %q, want denied=%v", tt.t, why, tt.bad)
			}
		})
	}
}

// TestRiskOwnServices pins the services the app hides: their packages
// must stay denied, the main-thread model packages allowed.
func TestRiskOwnServices(t *testing.T) {
	for _, rt := range []reflect.Type{
		reflect.TypeFor[store.Store](),
		reflect.TypeFor[control.Server](),
		reflect.TypeFor[remote.Server](),
		reflect.TypeFor[ffplayer.Player](),
		reflect.TypeFor[media.Processor](),
		reflect.TypeFor[presentations.Service](),
		reflect.TypeFor[websocket.Conn](),
	} {
		if Risk(rt) == "" {
			t.Errorf("Risk(%v) allows it", rt)
		}
	}
	for _, rt := range []reflect.Type{
		reflect.TypeFor[models.Deck](),
		reflect.TypeFor[models.Theme](),
		reflect.TypeFor[*models.Schedule](),
	} {
		if why := Risk(rt); why != "" {
			t.Errorf("Risk(%v) = %q", rt, why)
		}
	}
}
