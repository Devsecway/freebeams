package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Live updates use one WebSocket per phone rather than Server-Sent Events:
// the same connection carries the state pushes and the phone's taps, so a tap
// needs no new HTTP request (and no TCP handshake on a busy church Wi-Fi), the
// server knows which devices are online, and revoking a device closes its
// socket at once. The page reconnects by itself (web/app.js).

const (
	writeWait = 10 * time.Second
	pongWait  = 45 * time.Second
	pingEvery = 20 * time.Second
	readLimit = 16 << 10
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 8192,
	// handleWS has checked the origin already; this is the same check, kept so
	// the upgrader never falls back to accepting anything.
	CheckOrigin: func(r *http.Request) bool { return sameOrigin(r.Header.Get("Origin"), r.Host) },
}

// client is one phone's WebSocket.
type client struct {
	s    *Server
	dev  Device
	conn *websocket.Conn

	mu      sync.Mutex
	state   []byte        // the newest state message not yet written
	wake    chan struct{} // a new state is waiting
	out     chan []byte   // replies
	quit    chan struct{}
	once    sync.Once
	revoked bool
}

// wsMessage is what a phone sends: an action with an id for the reply.
type wsMessage struct {
	ID     int             `json:"id"`
	Action string          `json:"action"`
	Args   json.RawMessage `json:"args"`
}

type wsReply struct {
	Type  string `json:"type"` // "reply"
	ID    int    `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request, d Device) {
	if !sameOrigin(r.Header.Get("Origin"), r.Host) {
		writeJSON(w, http.StatusForbidden, errBody("cross-origin WebSocket refused"))
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // the upgrader has answered
	}
	c := &client{s: s, dev: d, conn: conn, wake: make(chan struct{}, 1), out: make(chan []byte, 32), quit: make(chan struct{})}
	hello, _ := json.Marshal(map[string]any{"type": "hello", "session": s.session(d)})
	c.out <- hello

	s.mu.Lock()
	if s.stopped() {
		s.mu.Unlock()
		conn.Close()
		return
	}
	s.clients[c] = struct{}{}
	if s.state != nil {
		c.state = stateMessage(s.state)
		c.wake <- struct{}{}
	}
	s.mu.Unlock()
	s.notify()
	go c.writeLoop()
	c.readLoop()
}

// stopped reports whether Stop closed the current run (s.mu held).
func (s *Server) stopped() bool {
	select {
	case <-s.stop:
		return true
	default:
		return false
	}
}

func (s *Server) clientList() []*client {
	out := make([]*client, 0, len(s.clients))
	for c := range s.clients {
		out = append(out, c)
	}
	return out
}

func (s *Server) removeClient(c *client) {
	s.mu.Lock()
	_, ok := s.clients[c]
	delete(s.clients, c)
	s.mu.Unlock()
	if ok {
		s.notify()
	}
}

// push queues a state message; an older one not yet written is replaced.
func (c *client) push(msg []byte) {
	c.mu.Lock()
	c.state = msg
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// send queues a reply; a phone that does not read its replies is dropped.
func (c *client) send(msg []byte) {
	select {
	case c.out <- msg:
	case <-c.quit:
	default:
		c.close()
	}
}

func (c *client) close() { c.once.Do(func() { close(c.quit) }) }

// revoke tells the phone it was removed, then closes.
func (c *client) revoke() {
	c.mu.Lock()
	c.revoked = true
	c.mu.Unlock()
	c.close()
}

func (c *client) write(msg []byte) error {
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.conn.WriteMessage(websocket.TextMessage, msg)
}

func (c *client) writeLoop() {
	ping := time.NewTicker(pingEvery)
	defer func() {
		ping.Stop()
		c.conn.Close()
		c.s.removeClient(c)
	}()
	for {
		select {
		case <-c.wake:
			c.mu.Lock()
			msg := c.state
			c.state = nil
			c.mu.Unlock()
			if msg != nil && c.write(msg) != nil {
				c.close()
				return
			}
		case msg := <-c.out:
			if c.write(msg) != nil {
				c.close()
				return
			}
		case <-ping.C:
			if c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)) != nil {
				c.close()
				return
			}
		case <-c.quit:
			c.mu.Lock()
			revoked := c.revoked
			c.mu.Unlock()
			code, text := websocket.CloseGoingAway, "remote turned off"
			if revoked {
				c.write([]byte(`{"type":"revoked"}`))
				code, text = websocket.ClosePolicyViolation, "device revoked"
			}
			c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, text), time.Now().Add(time.Second))
			return
		}
	}
}

func (c *client) readLoop() {
	defer c.close()
	c.conn.SetReadLimit(readLimit)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-c.quit
		cancel()
	}()
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		var m wsMessage
		if err := json.Unmarshal(data, &m); err != nil || m.Action == "" {
			c.reply(m.ID, errorString("send {id, action, args}"))
			continue
		}
		if m.Action == "ping" { // the page's own keep-alive (it cannot send ping frames)
			c.reply(m.ID, nil)
			continue
		}
		// A device revoked while connected is closed by Revoke; re-check
		// anyway so no action slips through in between.
		d, ok := c.s.deviceByID(c.dev.ID)
		if !ok {
			c.revoke()
			return
		}
		c.reply(m.ID, c.s.run(ctx, d, m.Action, m.Args))
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }

func (c *client) reply(id int, err error) {
	r := wsReply{Type: "reply", ID: id, OK: err == nil}
	if err != nil {
		r.Error = err.Error()
	}
	b, _ := json.Marshal(r)
	c.send(b)
}
