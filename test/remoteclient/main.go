// Command remoteclient plays a phone against FreeBeams's LAN remote
// (internal/remote) for test/smoke.sh. It speaks the same API as the phone
// web app: same-origin JSON POSTs for pairing, then a WebSocket.
//
//	remoteclient pair  -base URL -code 123456 -name "Smoke Phone"   # {ok, id, secret, …}
//	remoteclient wait  -base URL -id ID -secret S                   # {ok, status, token, …}
//	remoteclient ws    -base URL -token T -do next -do 'black {"on":true}'
//	remoteclient watch -base URL -token T -for 10s                  # until revoked / closed
//
// Every command prints one JSON document on stdout. ws sends the actions in
// order over one socket; after each successful reply it waits for the state
// push that follows (up to -settle) and prints {ok, hello, replies, state,
// states}: state is the last state document received.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: remoteclient pair|wait|ws|watch [flags]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	base := fs.String("base", "", "the remote's URL (http://127.0.0.1:PORT)")
	code := fs.String("code", "", "pair: the pairing code")
	name := fs.String("name", "Smoke Phone", "pair: the device name")
	id := fs.String("id", "", "wait: the request id")
	secret := fs.String("secret", "", "wait: the request secret")
	token := fs.String("token", "", "ws, watch: the device token")
	settle := fs.Duration("settle", 3*time.Second, "ws: how long to wait for the state push after an action")
	dur := fs.Duration("for", 10*time.Second, "watch: how long to listen")
	var actions []string
	fs.Func("do", "ws: an action, optionally followed by its JSON arguments (repeatable)", func(s string) error {
		actions = append(actions, s)
		return nil
	})
	fs.Parse(os.Args[2:])

	var (
		out any
		err error
	)
	switch os.Args[1] {
	case "pair":
		out, err = post(*base, "/api/pair", map[string]string{"code": *code, "name": *name})
	case "wait":
		out, err = post(*base, "/api/pair/wait", map[string]string{"id": *id, "secret": *secret})
	case "ws":
		out, err = session(*base, *token, actions, *settle)
	case "watch":
		out, err = watch(*base, *token, *dur)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		out = map[string]any{"ok": false, "error": err.Error()}
	}
	json.NewEncoder(os.Stdout).Encode(out)
}

// post sends a same-origin JSON POST like the page does and returns the
// response body (with the HTTP status as "httpStatus").
func post(base, path string, body any) (map[string]any, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(base, "/")+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", strings.TrimRight(base, "/"))
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) Mobile/15E148 remoteclient")
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s: HTTP %d, not JSON: %.200s", path, resp.StatusCode, raw)
	}
	m["httpStatus"] = resp.StatusCode
	return m, nil
}

// dial opens the WebSocket with the token as the phone's cookie.
func dial(base, token string) (*websocket.Conn, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	h.Set("Origin", u.Scheme+"://"+u.Host)
	h.Set("Cookie", "fb_remote="+token)
	conn, resp, err := websocket.DefaultDialer.Dial("ws://"+u.Host+"/api/ws", h)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("dial: %w (HTTP %d)", err, resp.StatusCode)
		}
		return nil, fmt.Errorf("dial: %w", err)
	}
	return conn, nil
}

type message struct {
	Type    string          `json:"type"`
	ID      int             `json:"id"`
	OK      bool            `json:"ok"`
	Error   string          `json:"error"`
	State   json.RawMessage `json:"state"`
	Session json.RawMessage `json:"session"`
}

// reader reads the socket on a goroutine: gorilla's connection is unusable
// after a read deadline passes, so waits time out on a channel instead.
type reader struct {
	msgs  chan []byte
	err   error // why the read loop ended, once msgs is closed
	state json.RawMessage
	n     int // state messages received
	hello json.RawMessage
}

func newReader(conn *websocket.Conn) *reader {
	r := &reader{msgs: make(chan []byte, 64)}
	go func() {
		defer close(r.msgs)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				r.err = err
				return
			}
			r.msgs <- data
		}
	}()
	return r
}

var errTimeout = errors.New("timed out")

// next returns the next message, errTimeout after deadline, or the read
// error once the socket closed.
func (r *reader) next(deadline time.Time) (message, error) {
	var m message
	t := time.NewTimer(time.Until(deadline))
	defer t.Stop()
	var data []byte
	select {
	case d, ok := <-r.msgs:
		if !ok {
			return m, r.err
		}
		data = d
	case <-t.C:
		return m, errTimeout
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("bad message %.200s: %w", data, err)
	}
	switch m.Type {
	case "state":
		r.state, r.n = m.State, r.n+1
	case "hello":
		r.hello = m.Session
	}
	return m, nil
}

func timeout(err error) bool { return errors.Is(err, errTimeout) }

func session(base, token string, actions []string, settle time.Duration) (any, error) {
	conn, err := dial(base, token)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	r := newReader(conn)
	// The hello and the current state arrive first.
	for end := time.Now().Add(5 * time.Second); r.hello == nil || r.state == nil; {
		if _, err := r.next(end); err != nil {
			return nil, fmt.Errorf("waiting for hello and state: %w", err)
		}
	}
	type reply struct {
		Action string `json:"action"`
		OK     bool   `json:"ok"`
		Error  string `json:"error,omitempty"`
	}
	replies := []reply{}
	allOK := true
	for i, a := range actions {
		name, args, _ := strings.Cut(strings.TrimSpace(a), " ")
		var raw json.RawMessage
		if args = strings.TrimSpace(args); args != "" {
			raw = json.RawMessage(args)
		}
		msg, _ := json.Marshal(map[string]any{"id": i + 1, "action": name, "args": raw})
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return nil, fmt.Errorf("send %s: %w", name, err)
		}
		seen := r.n
		var rep *reply
		for end := time.Now().Add(15 * time.Second); rep == nil; {
			m, err := r.next(end)
			if err != nil {
				return nil, fmt.Errorf("waiting for the reply to %s: %w", name, err)
			}
			if m.Type == "reply" && m.ID == i+1 {
				rep = &reply{Action: name, OK: m.OK, Error: m.Error}
			}
		}
		replies = append(replies, *rep)
		allOK = allOK && rep.OK
		if !rep.OK {
			continue
		}
		// The push that shows the action's effect (none when nothing changed).
		for end := time.Now().Add(settle); r.n == seen; {
			if _, err := r.next(end); err != nil {
				if timeout(err) {
					break
				}
				return nil, fmt.Errorf("waiting for the state after %s: %w", name, err)
			}
		}
	}
	return map[string]any{"ok": allOK, "hello": r.hello, "replies": replies, "state": r.state, "states": r.n}, nil
}

// watch listens until the server says the device was revoked or closes the
// socket (or until d passes).
func watch(base, token string, d time.Duration) (any, error) {
	conn, err := dial(base, token)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	fmt.Fprintln(os.Stderr, "remoteclient: connected")
	r := newReader(conn)
	revoked := false
	for end := time.Now().Add(d); ; {
		m, err := r.next(end)
		if err != nil {
			var ce *websocket.CloseError
			closed := errors.As(err, &ce)
			res := map[string]any{"ok": true, "revoked": revoked, "closed": closed, "timedOut": timeout(err), "states": r.n}
			if closed {
				res["closeCode"], res["closeText"] = ce.Code, ce.Text
			}
			return res, nil
		}
		if m.Type == "revoked" {
			revoked = true
		}
	}
}
