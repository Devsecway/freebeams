// Package remote is FreeBeams's LAN remote control: an HTTP server that
// serves a phone web app (web/, embedded) and its API, off until the operator
// turns it on.
//
// Security model (docs/REMOTE.md): a phone pairs by entering the six-digit
// code FreeBeams shows, then waits until the operator approves it by name.
// The approved device gets a 256-bit token in an HttpOnly, SameSite=Strict
// cookie; only its SHA-256 hash is stored. Every API call and the WebSocket
// need the token; revoking a device closes its connections at once. Wrong
// codes are rate-limited per address and in total; only local-network peers
// and IP / localhost / .local Host headers are served (no DNS rebinding);
// WebSocket upgrades and POSTs must come from the page's own origin. The
// loopback test control server (internal/control) is separate and is never
// exposed here: phones reach only the actions in actions.go.
//
// Nothing here touches Godot. Phone actions become control.Commands that the
// app drains on the main thread (Commands), exactly like the control API's.
package remote

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"freebeams/internal/control"
)

// DefaultPort is the remote's port unless the operator picks another.
const DefaultPort = 8765

// CookieName holds a paired device's token.
const CookieName = "fb_remote"

// cookieMaxAge is how long a phone keeps its token (Chrome caps cookies at 400 days).
const cookieMaxAge = 400 * 24 * time.Hour

const (
	commandTimeout = 10 * time.Second
	waitPoll       = 20 * time.Second // a pairing long poll
	maxBody        = 64 << 10
)

// SongHit is a song search result for the phone.
type SongHit struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author,omitempty"`
}

// Options configure a Server.
type Options struct {
	// Name is shown on the phone ("FreeBeams on Godwin's MacBook Pro").
	Name string
	// SearchSongs searches the song library for the phone (called on the
	// request's goroutine; nil disables song search).
	SearchSongs func(ctx context.Context, q string) ([]SongHit, error)
	// Now and Logf default to time.Now and log.Printf.
	Now  func() time.Time
	Logf func(format string, args ...any)
}

// Server is the remote control server.
type Server struct {
	opt      Options
	commands chan *control.Command
	changed  chan struct{}

	mu      sync.Mutex
	devices []*Device
	code    string
	pending []*PairRequest
	lim     limiter
	clients map[*client]struct{}
	state   []byte // the last published State, as JSON
	addrs   []string
	port    int
	running bool
	srv     *http.Server
	stop    chan struct{} // closed by Stop: long polls and waiting commands give up
}

// New returns a stopped server with no devices.
func New(opt Options) *Server {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Logf == nil {
		opt.Logf = log.Printf
	}
	return &Server{
		opt:      opt,
		commands: make(chan *control.Command, 16),
		changed:  make(chan struct{}, 1),
		code:     newCode(),
		clients:  map[*client]struct{}{},
		stop:     make(chan struct{}),
	}
}

// Commands is drained by the app's main loop, like control.Server.Commands.
func (s *Server) Commands() <-chan *control.Command { return s.commands }

// Changed signals (coalesced) that the devices, the pending requests, the
// connections or the code changed: the app re-reads Status.
func (s *Server) Changed() <-chan struct{} { return s.changed }

func (s *Server) notify() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// ---- lifecycle -------------------------------------------------------------------

// Start listens on every interface at port (0 picks a free port) with a new
// pairing code. Starting a running server does nothing.
func (s *Server) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("remote control: listen on port %d: %w", port, err)
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.addrs = LANAddresses()
	s.code = newCode()
	s.lim = limiter{}
	s.stop = make(chan struct{})
	s.srv = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    32 << 10,
		ErrorLog:          log.New(logWriter{s.opt.Logf}, "", 0),
	}
	s.running = true
	srv := s.srv
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.opt.Logf("[remote] server error: %v", err)
		}
	}()
	s.opt.Logf("[remote] listening on port %d (%s)", s.port, strings.Join(s.addrs, ", "))
	s.notify()
	return nil
}

// Stop closes the listener, every connection and every pairing request.
func (s *Server) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	srv := s.srv
	s.srv = nil
	close(s.stop)
	clients := s.clientList()
	for _, r := range s.pending {
		if r.status == pairPending {
			r.status = pairExpired
			close(r.done)
		}
	}
	s.pending = nil
	s.mu.Unlock()
	srv.Close()
	for _, c := range clients {
		c.close()
	}
	s.opt.Logf("[remote] stopped")
	s.notify()
}

// Running reports whether the server is listening.
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// logWriter sends net/http's own log lines (bad requests, TLS probes) to Logf.
type logWriter struct{ logf func(string, ...any) }

func (w logWriter) Write(p []byte) (int, error) {
	w.logf("[remote] %s", strings.TrimSpace(string(p)))
	return len(p), nil
}

// ---- operator side (main thread) ----------------------------------------------------

// Status is the remote as the operator sees it.
type Status struct {
	Running   bool          `json:"running"`
	Port      int           `json:"port"`
	Addresses []string      `json:"addresses"`
	URL       string        `json:"url"` // what the QR code encodes
	Code      string        `json:"code"`
	Pending   []PairRequest `json:"pending"`
	Devices   []DeviceInfo  `json:"devices"`
	Clients   int           `json:"clients"` // open connections
}

// Status returns a snapshot (pending requests that timed out are dropped).
func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(s.opt.Now())
	st := Status{Running: s.running, Port: s.port, Addresses: slices.Clone(s.addrs), Code: s.code,
		Pending: []PairRequest{}, Devices: s.deviceInfosLocked(), Clients: len(s.clients)}
	if st.Addresses == nil {
		st.Addresses = []string{}
	}
	if s.running {
		st.URL = s.urlLocked()
	}
	for _, r := range s.pending {
		if r.status == pairPending {
			st.Pending = append(st.Pending, *r)
		}
	}
	return st
}

func (s *Server) urlLocked() string {
	host := "localhost"
	if len(s.addrs) > 0 {
		host = s.addrs[0]
	} else if h, err := os.Hostname(); err == nil && h != "" {
		host = h
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(s.port)) + "/"
}

// RefreshAddresses re-reads the LAN addresses (the Wi-Fi may have changed).
func (s *Server) RefreshAddresses() {
	addrs := LANAddresses()
	s.mu.Lock()
	changed := !slices.Equal(addrs, s.addrs)
	s.addrs = addrs
	s.mu.Unlock()
	if changed {
		s.notify()
	}
}

// NewCode replaces the pairing code.
func (s *Server) NewCode() {
	s.mu.Lock()
	s.code = newCode()
	s.mu.Unlock()
	s.notify()
}

// SetDevices replaces the approved devices (loaded from the store).
func (s *Server) SetDevices(ds []Device) {
	s.mu.Lock()
	s.devices = s.devices[:0]
	for _, d := range ds {
		if d.ID == "" || d.TokenHash == "" {
			continue
		}
		d := d
		s.devices = append(s.devices, &d)
	}
	s.mu.Unlock()
	s.notify()
}

// StoredDevices returns the approved devices for persisting.
func (s *Server) StoredDevices() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, *d)
	}
	return out
}

func (s *Server) deviceInfosLocked() []DeviceInfo {
	online := map[string]bool{}
	for c := range s.clients {
		online[c.dev.ID] = true
	}
	out := make([]DeviceInfo, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, DeviceInfo{ID: d.ID, Name: d.Name, Kind: d.Kind, Online: online[d.ID], Created: d.Created, LastSeen: d.LastSeen})
	}
	return out
}

// Approve approves a pending request: the device is added at once and its
// phone collects the token. The pairing code changes (each code pairs once).
func (s *Server) Approve(id string) (Device, error) {
	token, err := newToken()
	if err != nil {
		return Device{}, fmt.Errorf("approve: %w", err)
	}
	devID, err := randomID(6)
	if err != nil {
		return Device{}, fmt.Errorf("approve: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.opt.Now()
	s.expireLocked(now)
	r := s.requestLocked(id)
	if r == nil || r.status != pairPending {
		return Device{}, fmt.Errorf("approve: no device is waiting with id %q", id)
	}
	d := Device{ID: devID, Name: r.Name, Kind: r.Kind, TokenHash: hashToken(token), Permissions: PermAll, Created: now}
	s.devices = append(s.devices, &d)
	r.status, r.token, r.device, r.decided = pairApproved, token, d, now
	close(r.done)
	s.code = newCode()
	s.opt.Logf("[remote] approved %q (%s, %s)", d.Name, d.Kind, r.Address)
	s.notify()
	return d, nil
}

// Deny refuses a pending request.
func (s *Server) Deny(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(s.opt.Now())
	r := s.requestLocked(id)
	if r == nil || r.status != pairPending {
		return fmt.Errorf("deny: no device is waiting with id %q", id)
	}
	r.status, r.decided = pairDenied, s.opt.Now()
	close(r.done)
	s.opt.Logf("[remote] denied %q (%s)", r.Name, r.Address)
	s.notify()
	return nil
}

// Revoke removes an approved device and closes its connections.
func (s *Server) Revoke(id string) error {
	s.mu.Lock()
	i := slices.IndexFunc(s.devices, func(d *Device) bool { return d.ID == id })
	if i < 0 {
		s.mu.Unlock()
		return fmt.Errorf("revoke: no device with id %q", id)
	}
	name := s.devices[i].Name
	s.devices = slices.Delete(s.devices, i, i+1)
	var drop []*client
	for c := range s.clients {
		if c.dev.ID == id {
			drop = append(drop, c)
		}
	}
	s.mu.Unlock()
	for _, c := range drop {
		c.revoke()
	}
	s.opt.Logf("[remote] revoked %q", name)
	s.notify()
	return nil
}

func (s *Server) requestLocked(id string) *PairRequest {
	for _, r := range s.pending {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// expireLocked drops requests the operator did not answer in time and
// approved tokens nobody collected.
func (s *Server) expireLocked(now time.Time) {
	keep := s.pending[:0]
	changed := false
	for _, r := range s.pending {
		switch {
		case r.status == pairPending && now.Sub(r.Created) > pendingTTL:
			r.status = pairExpired
			close(r.done)
			changed = true
		case r.status != pairPending && now.Sub(r.decided) > collectTTL:
			changed = true
		default:
			keep = append(keep, r)
		}
	}
	clear(s.pending[len(keep):])
	s.pending = keep
	if changed {
		s.notify()
	}
}

// Publish sends st to every connected phone (nothing when it is unchanged).
func (s *Server) Publish(st State) {
	b, err := json.Marshal(st)
	if err != nil {
		s.opt.Logf("[remote] marshal state: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if bytes.Equal(b, s.state) {
		return
	}
	s.state = b
	msg := stateMessage(b)
	for c := range s.clients {
		c.push(msg)
	}
}

func stateMessage(state []byte) []byte {
	return slices.Concat([]byte(`{"type":"state","state":`), state, []byte(`}`))
}

// ---- HTTP ----------------------------------------------------------------------------

// Handler returns the server's HTTP handler (exposed for tests).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", staticHandler())
	mux.HandleFunc("POST /api/pair", s.handlePair)
	mux.HandleFunc("POST /api/pair/wait", s.handlePairWait)
	mux.HandleFunc("POST /api/pair/cancel", s.handlePairCancel)
	mux.HandleFunc("GET /api/session", s.authed(PermView, s.handleSession))
	mux.HandleFunc("GET /api/state", s.authed(PermView, s.handleState))
	mux.HandleFunc("POST /api/action/{name}", s.authed(0, s.handleAction))
	mux.HandleFunc("GET /api/songs", s.authed(PermSchedule, s.handleSongs))
	mux.HandleFunc("POST /api/forget", s.authed(0, s.handleForget))
	mux.HandleFunc("GET /api/ws", s.authed(PermView, s.handleWS))
	return s.guard(mux)
}

// guard applies the checks every request passes: a local-network peer, a
// Host that is this machine's address (DNS rebinding), and for POSTs a JSON
// body from the page's own origin (cross-site forms cannot send JSON without
// a CORS preflight, which is never answered).
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if !localPeer(r.RemoteAddr) {
			writeJSON(w, http.StatusForbidden, errBody("the remote serves the local network only"))
			return
		}
		if !localHost(r.Host) {
			writeJSON(w, http.StatusMisdirectedRequest, errBody("open the remote by its IP address"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" && !sameOrigin(o, r.Host) {
				writeJSON(w, http.StatusForbidden, errBody("cross-origin request refused"))
				return
			}
			if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				writeJSON(w, http.StatusUnsupportedMediaType, errBody("send JSON"))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		next.ServeHTTP(w, r)
	})
}

// peerIP is the client's address without the port.
func peerIP(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

// tokenOf returns the token a request carries: the cookie, or a bearer
// header (scripts and tests).
func tokenOf(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	if c, err := r.Cookie(CookieName); err == nil {
		return c.Value
	}
	return ""
}

// lookup finds the device a token belongs to and marks it seen.
func (s *Server) lookup(token string) (Device, bool) {
	if token == "" {
		return Device{}, false
	}
	hash := []byte(hashToken(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.devices {
		if subtle.ConstantTimeCompare(hash, []byte(d.TokenHash)) == 1 {
			now := s.opt.Now()
			if now.Sub(d.LastSeen) > time.Minute {
				s.notify() // the devices list shows when it was last seen
			}
			d.LastSeen = now
			return *d, true
		}
	}
	return Device{}, false
}

// deviceByID returns an approved device (false once it was revoked).
func (s *Server) deviceByID(id string) (Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.devices {
		if d.ID == id {
			return *d, true
		}
	}
	return Device{}, false
}

// authed wraps a handler that needs a paired device with permission perm.
func (s *Server) authed(perm Permissions, h func(http.ResponseWriter, *http.Request, Device)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := s.lookup(tokenOf(r))
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "this device is not paired", "paired": false})
			return
		}
		if !d.Permissions.Has(perm) {
			writeJSON(w, http.StatusForbidden, errBody(errForbidden.Error()))
			return
		}
		h(w, r, d)
	}
}

func errBody(msg string) map[string]any { return map[string]any{"ok": false, "error": msg} }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func decodeBody(r *http.Request, dst any) error {
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return errors.New("the body is not valid JSON")
	}
	return nil
}

// ---- pairing -----------------------------------------------------------------------------

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	ip := peerIP(r)
	secret, err := randomID(16)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("no randomness"))
		return
	}
	reqID, err := randomID(8)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("no randomness"))
		return
	}

	s.mu.Lock()
	now := s.opt.Now()
	s.expireLocked(now)
	if wait := s.lim.blocked(ip, now); wait > 0 {
		s.mu.Unlock()
		secs := int(wait.Seconds() + 0.999)
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "retryAfter": secs,
			"error": fmt.Sprintf("Too many wrong codes. Try again in %s.", humanWait(secs))})
		return
	}
	code := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, body.Code)
	if !codeMatches(code, s.code) {
		if s.lim.fail(ip, now) {
			s.code = newCode()
			s.opt.Logf("[remote] too many wrong pairing codes: the code changed")
			s.notify()
		}
		left := s.lim.remaining(ip, now)
		s.mu.Unlock()
		s.opt.Logf("[remote] wrong pairing code from %s", ip)
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "remaining": left,
			"error": "That code is not right. Check the code shown in FreeBeams (Remote ▸ Show Pairing Code)."})
		return
	}
	// One waiting request per address: asking again replaces the old one.
	s.pending = slices.DeleteFunc(s.pending, func(p *PairRequest) bool {
		if p.Address == ip && p.status == pairPending {
			p.status = pairExpired
			close(p.done)
			return true
		}
		return false
	})
	waiting := 0
	for _, p := range s.pending {
		if p.status == pairPending {
			waiting++
		}
	}
	if waiting >= maxPending {
		s.mu.Unlock()
		writeJSON(w, http.StatusServiceUnavailable, errBody("Too many devices are waiting for approval. Try again in a minute."))
		return
	}
	req := &PairRequest{ID: reqID, Name: CleanName(body.Name), Kind: DeviceKind(r.UserAgent()), Address: ip,
		Created: now, secretHash: hashToken(secret), status: pairPending, done: make(chan struct{})}
	s.pending = append(s.pending, req)
	s.mu.Unlock()
	s.opt.Logf("[remote] %q (%s) asks to pair", req.Name, ip)
	s.notify()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": req.ID, "secret": secret, "expiresIn": int(pendingTTL.Seconds())})
}

func humanWait(secs int) string {
	if secs >= 90 {
		return fmt.Sprintf("%d minutes", (secs+59)/60)
	}
	return fmt.Sprintf("%d seconds", secs)
}

type pairRef struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

// request finds the request ref names, checking its secret.
func (s *Server) requestFor(ref pairRef) *PairRequest {
	r := s.requestLocked(ref.ID)
	if r == nil || subtle.ConstantTimeCompare([]byte(hashToken(ref.Secret)), []byte(r.secretHash)) != 1 {
		return nil
	}
	return r
}

// handlePairWait long-polls a request until the operator decides (or 20 s).
// On approval it hands out the token, once, as a cookie and in the body.
func (s *Server) handlePairWait(w http.ResponseWriter, r *http.Request) {
	var ref pairRef
	if err := decodeBody(r, &ref); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	s.mu.Lock()
	s.expireLocked(s.opt.Now())
	req := s.requestFor(ref)
	stop := s.stop
	s.mu.Unlock()
	if req == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "status": pairExpired, "error": "The request expired. Pair again."})
		return
	}
	t := time.NewTimer(waitPoll)
	defer t.Stop()
	select {
	case <-req.done:
	case <-t.C:
	case <-stop:
	case <-r.Context().Done():
		return
	}

	s.mu.Lock()
	status := req.status
	token, dev := req.token, req.device
	if status != pairPending {
		s.pending = slices.DeleteFunc(s.pending, func(p *PairRequest) bool { return p == req })
	}
	s.mu.Unlock()
	switch status {
	case pairApproved:
		http.SetCookie(w, &http.Cookie{Name: CookieName, Value: token, Path: "/", HttpOnly: true,
			SameSite: http.SameSiteStrictMode, MaxAge: int(cookieMaxAge.Seconds())})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": status, "token": token,
			"device": map[string]string{"id": dev.ID, "name": dev.Name, "kind": dev.Kind}})
	case pairPending:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": status})
	default:
		msg := "The request expired. Pair again."
		if status == pairDenied {
			msg = "The operator did not approve this device."
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "status": status, "error": msg})
	}
}

// handlePairCancel withdraws a waiting request (the phone's Cancel button).
func (s *Server) handlePairCancel(w http.ResponseWriter, r *http.Request) {
	var ref pairRef
	if err := decodeBody(r, &ref); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	s.mu.Lock()
	req := s.requestFor(ref)
	if req != nil && req.status == pairPending {
		req.status = pairExpired
		close(req.done)
		s.pending = slices.DeleteFunc(s.pending, func(p *PairRequest) bool { return p == req })
	}
	s.mu.Unlock()
	s.notify()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- paired API -----------------------------------------------------------------------------

type sessionDoc struct {
	OK          bool        `json:"ok"`
	Paired      bool        `json:"paired"`
	Name        string      `json:"name"`
	Device      DeviceInfo  `json:"device"`
	Permissions Permissions `json:"permissions"`
	Search      bool        `json:"search"` // song search is available
}

func (s *Server) session(d Device) sessionDoc {
	return sessionDoc{OK: true, Paired: true, Name: s.opt.Name, Permissions: d.Permissions, Search: s.opt.SearchSongs != nil,
		Device: DeviceInfo{ID: d.ID, Name: d.Name, Kind: d.Kind, Created: d.Created, LastSeen: d.LastSeen}}
}

func (s *Server) handleSession(w http.ResponseWriter, _ *http.Request, d Device) {
	writeJSON(w, http.StatusOK, s.session(d))
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request, _ Device) {
	s.mu.Lock()
	b := s.state
	s.mu.Unlock()
	if b == nil {
		b = []byte(`{}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func (s *Server) handleAction(w http.ResponseWriter, r *http.Request, d Device) {
	var raw json.RawMessage
	if err := decodeBody(r, &raw); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	if err := s.run(r.Context(), d, r.PathValue("name"), raw); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errForbidden) {
			status = http.StatusForbidden
		}
		writeJSON(w, status, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// run executes a phone action on the main thread (through Commands) and
// waits for the app's answer.
func (s *Server) run(ctx context.Context, d Device, name string, raw json.RawMessage) error {
	action, data, err := translate(name, raw, d.Permissions)
	if err != nil {
		return err
	}
	cmd := control.NewCommand(action, data)
	cmd.Query["remote"] = d.ID
	s.mu.Lock()
	stop := s.stop
	s.mu.Unlock()
	t := time.NewTimer(commandTimeout)
	defer t.Stop()
	select {
	case s.commands <- cmd:
	case <-ctx.Done():
		return ctx.Err()
	case <-stop:
		return errors.New("the remote was turned off")
	case <-t.C:
		return errors.New("FreeBeams is busy; try again")
	}
	res, err := cmd.Wait(stop, commandTimeout)
	if err != nil {
		return err
	}
	if !res.OK {
		return errors.New(res.Error)
	}
	return nil
}

func (s *Server) handleSongs(w http.ResponseWriter, r *http.Request, _ Device) {
	if s.opt.SearchSongs == nil {
		writeJSON(w, http.StatusNotFound, errBody("song search is not available"))
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) > 100 {
		q = string([]rune(q)[:100])
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	hits, err := s.opt.SearchSongs(ctx, q)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody("search failed"))
		s.opt.Logf("[remote] song search: %v", err)
		return
	}
	if hits == nil {
		hits = []SongHit{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "songs": hits})
}

// handleForget unpairs the calling device (the phone's "Forget this device").
func (s *Server) handleForget(w http.ResponseWriter, _ *http.Request, d Device) {
	if err := s.Revoke(d.ID); err != nil {
		writeJSON(w, http.StatusNotFound, errBody(err.Error()))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
