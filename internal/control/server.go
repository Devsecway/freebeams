// Package control is a small HTTP server that lets tests and automation drive
// the running app. It is adapted from Bufflehead's control server.
//
// Every request must carry `Authorization: Bearer <key>`. The key is minted
// per launch (or pinned with FREEBEAMS_CONTROL_KEY for test harnesses) and
// printed to stdout together with the address, so other local software can't
// drive the app by blindly POSTing to the port.
//
// Handlers never touch Godot. They enqueue a Command that the app drains on
// the main thread (in Process) and answers with Respond.
package control

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// KeyEnv pins the control key (used by test harnesses).
const KeyEnv = "FREEBEAMS_CONTROL_KEY"

// commandTimeout bounds how long a request waits for the main thread.
const commandTimeout = 30 * time.Second

// Command is an action to execute on the main thread.
type Command struct {
	// Action is "state", "screenshot", or a POST /action/{name} name.
	Action string
	// Data is the request JSON body (may be empty).
	Data json.RawMessage
	// Query holds URL query parameters (e.g. screenshot?window=output).
	Query map[string]string

	result chan Result
}

// Result is returned after the main thread processes a command.
type Result struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
	// PNG, when set, is returned as image/png instead of JSON.
	PNG []byte `json:"-"`
}

// Respond answers the command. It must be called exactly once.
func (c *Command) Respond(r Result) {
	select {
	case c.result <- r:
	default:
	}
}

// RespondJSON answers with v marshalled as the result data.
func (c *Command) RespondJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		c.Respond(Result{Error: fmt.Sprintf("marshal result: %v", err)})
		return
	}
	c.Respond(Result{OK: true, Data: b})
}

// RespondErr answers with an error.
func (c *Command) RespondErr(err error) {
	c.Respond(Result{Error: err.Error()})
}

// Server is the HTTP control server.
type Server struct {
	commands chan *Command
	port     int
	addr     string
	key      string
}

// New creates a server on port (0 = random free port).
func New(port int) *Server {
	return &Server{commands: make(chan *Command, 16), port: port, key: generateKey()}
}

func generateKey() string {
	if k := os.Getenv(KeyEnv); k != "" {
		return k
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "freebeams-control-key-unavailable"
	}
	return hex.EncodeToString(buf)
}

// Key returns the bearer key.
func (s *Server) Key() string { return s.key }

// Addr returns host:port; valid after Start.
func (s *Server) Addr() string { return s.addr }

// Commands is drained by the app's main loop.
func (s *Server) Commands() <-chan *Command { return s.commands }

// Start listens on 127.0.0.1 and serves in a goroutine.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return fmt.Errorf("control server listen: %w", err)
	}
	s.addr = ln.Addr().String()
	fmt.Printf("Control server: http://%s\n", s.addr)
	fmt.Printf("Control key: %s\n", s.key)
	go func() {
		if err := http.Serve(ln, s.Handler()); err != nil {
			fmt.Printf("Control server error: %v\n", err)
		}
	}()
	return nil
}

var actionName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// Handler returns the authenticated HTTP handler (exposed for tests).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		s.dispatch(w, r, "state")
	})
	mux.HandleFunc("GET /screenshot", func(w http.ResponseWriter, r *http.Request) {
		s.dispatch(w, r, "screenshot")
	})
	mux.HandleFunc("POST /action/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !actionName.MatchString(name) {
			writeJSON(w, http.StatusBadRequest, Result{Error: "invalid action name"})
			return
		}
		s.dispatch(w, r, name)
	})
	return s.requireAuth(mux)
}

func (s *Server) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	got := strings.TrimSpace(h[len(prefix):])
	return subtle.ConstantTimeCompare([]byte(got), []byte(s.key)) == 1
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeJSON(w, http.StatusUnauthorized, Result{Error: "unauthorized: missing or invalid control key"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) dispatch(w http.ResponseWriter, r *http.Request, action string) {
	cmd := &Command{Action: action, Query: map[string]string{}, result: make(chan Result, 1)}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			cmd.Query[k] = v[0]
		}
	}
	if r.Body != nil {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, Result{Error: "read body: " + err.Error()})
			return
		}
		if len(strings.TrimSpace(string(body))) > 0 {
			if !json.Valid(body) {
				writeJSON(w, http.StatusBadRequest, Result{Error: "body is not valid JSON"})
				return
			}
			cmd.Data = body
		}
	}

	select {
	case s.commands <- cmd:
	case <-r.Context().Done():
		return
	case <-time.After(commandTimeout):
		writeJSON(w, http.StatusServiceUnavailable, Result{Error: "app busy: command queue full"})
		return
	}

	select {
	case res := <-cmd.result:
		if res.PNG != nil {
			w.Header().Set("Content-Type", "image/png")
			w.WriteHeader(http.StatusOK)
			w.Write(res.PNG)
			return
		}
		status := http.StatusOK
		if !res.OK {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, res)
	case <-r.Context().Done():
	case <-time.After(commandTimeout):
		writeJSON(w, http.StatusGatewayTimeout, Result{Error: "timed out waiting for the app"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
