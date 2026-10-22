package remote

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"freebeams/internal/control"
	"freebeams/internal/models"

	"github.com/gorilla/websocket"
)

// clock is a settable time source.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// fixture is a server behind httptest with a fake main loop that records the
// commands it receives and answers them (an action named "fail" fails).
type fixture struct {
	t     *testing.T
	s     *Server
	ts    *httptest.Server
	clock *clock

	mu   sync.Mutex
	cmds []*control.Command
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, clock: &clock{now: time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)}}
	f.s = New(Options{Name: "FreeBeams Test", Now: f.clock.Now, Logf: t.Logf,
		SearchSongs: func(_ context.Context, q string) ([]SongHit, error) {
			return []SongHit{{ID: 7, Title: "Amazing Grace (" + q + ")", Author: "John Newton"}}, nil
		}})
	f.ts = httptest.NewServer(f.s.Handler())
	done := make(chan struct{})
	go func() {
		for {
			select {
			case cmd := <-f.s.Commands():
				f.mu.Lock()
				f.cmds = append(f.cmds, cmd)
				f.mu.Unlock()
				if cmd.Action == "jump" && strings.Contains(string(cmd.Data), `"n":99`) {
					cmd.RespondErr(errorString("jump: no slide 99"))
					continue
				}
				cmd.RespondJSON(map[string]bool{"done": true})
			case <-done:
				return
			}
		}
	}()
	t.Cleanup(func() { close(done); f.ts.Close() })
	return f
}

func (f *fixture) commands() []*control.Command {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*control.Command(nil), f.cmds...)
}

type resp struct {
	status int
	header http.Header
	body   map[string]any
	raw    string
}

// do sends a request; token "" sends none. POST bodies are JSON.
func (f *fixture) do(method, path, token, body string, hdr ...string) resp {
	f.t.Helper()
	req, err := http.NewRequest(method, f.ts.URL+path, strings.NewReader(body))
	if err != nil {
		f.t.Fatal(err)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		if strings.EqualFold(hdr[i], "Host") {
			req.Host = hdr[i+1]
		} else {
			req.Header.Set(hdr[i], hdr[i+1])
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	r := resp{status: res.StatusCode, header: res.Header, raw: string(b)}
	json.Unmarshal(b, &r.body)
	return r
}

// pair runs the whole pairing flow and returns the device token.
func (f *fixture) pair(name string) (string, Device) {
	f.t.Helper()
	code := f.s.Status().Code
	r := f.do("POST", "/api/pair", "", `{"code":"`+code+`","name":"`+name+`"}`)
	if r.status != 200 {
		f.t.Fatalf("pair: %d %s", r.status, r.raw)
	}
	id, secret := r.body["id"].(string), r.body["secret"].(string)
	var pending []PairRequest
	for _, p := range f.s.Status().Pending {
		if p.ID == id {
			pending = append(pending, p)
		}
	}
	if len(pending) != 1 || pending[0].Name != name {
		f.t.Fatalf("pending = %+v, want %q", f.s.Status().Pending, name)
	}
	d, err := f.s.Approve(id)
	if err != nil {
		f.t.Fatal(err)
	}
	w := f.do("POST", "/api/pair/wait", "", `{"id":"`+id+`","secret":"`+secret+`"}`)
	if w.status != 200 || w.body["status"] != "approved" {
		f.t.Fatalf("wait: %d %s", w.status, w.raw)
	}
	return w.body["token"].(string), d
}

func TestPairingFlow(t *testing.T) {
	f := newFixture(t)
	code := f.s.Status().Code
	if !regexp.MustCompile(`^\d{6}$`).MatchString(code) {
		t.Fatalf("code %q is not six digits", code)
	}

	// A wrong code is refused and says how many tries are left.
	r := f.do("POST", "/api/pair", "", `{"code":"`+wrongCode(code)+`","name":"iPhone of John"}`)
	if r.status != http.StatusForbidden || r.body["remaining"] != float64(maxFailsPerIP-1) {
		t.Fatalf("wrong code: %d %s", r.status, r.raw)
	}

	// The right code makes a pending request the operator sees by name.
	r = f.do("POST", "/api/pair", "", `{"code":"`+code[:3]+` `+code[3:]+`","name":"  iPhone of\tJohn  "}`, "User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)")
	if r.status != 200 {
		t.Fatalf("pair: %d %s", r.status, r.raw)
	}
	id, secret := r.body["id"].(string), r.body["secret"].(string)
	st := f.s.Status()
	if len(st.Pending) != 1 || st.Pending[0].Name != "iPhone of John" || st.Pending[0].Kind != "phone" || st.Pending[0].Address != "127.0.0.1" {
		t.Fatalf("pending = %+v", st.Pending)
	}

	// The phone long-polls; the operator approves meanwhile.
	type waitResult struct{ r resp }
	got := make(chan waitResult, 1)
	go func() { got <- waitResult{f.do("POST", "/api/pair/wait", "", `{"id":"`+id+`","secret":"`+secret+`"}`)} }()
	time.Sleep(50 * time.Millisecond)
	// A wrong secret learns nothing.
	if bad := f.do("POST", "/api/pair/wait", "", `{"id":"`+id+`","secret":"nope"}`); bad.status != http.StatusNotFound || bad.body["token"] != nil {
		t.Fatalf("wait with a wrong secret: %d %s", bad.status, bad.raw)
	}
	dev, err := f.s.Approve(id)
	if err != nil {
		t.Fatal(err)
	}
	var w resp
	select {
	case res := <-got:
		w = res.r
	case <-time.After(5 * time.Second):
		t.Fatal("the long poll did not return after the approval")
	}
	if w.status != 200 || w.body["status"] != "approved" {
		t.Fatalf("wait: %d %s", w.status, w.raw)
	}
	token := w.body["token"].(string)
	if len(token) < 40 {
		t.Fatalf("token %q is too short", token)
	}
	cookie := w.header.Get("Set-Cookie")
	for _, want := range []string{CookieName + "=" + token, "HttpOnly", "SameSite=Strict", "Path=/"} {
		if !strings.Contains(cookie, want) {
			t.Errorf("Set-Cookie %q lacks %q", cookie, want)
		}
	}
	// The token is handed out once.
	if again := f.do("POST", "/api/pair/wait", "", `{"id":"`+id+`","secret":"`+secret+`"}`); again.body["token"] != nil {
		t.Fatalf("the token was handed out twice: %s", again.raw)
	}
	// Each code pairs once.
	if f.s.Status().Code == code {
		t.Error("the pairing code did not change after an approval")
	}
	// Only the token's hash is stored.
	stored := f.s.StoredDevices()
	if len(stored) != 1 || stored[0].ID != dev.ID || stored[0].TokenHash != hashToken(token) || strings.Contains(mustJSON(t, stored), token) {
		t.Fatalf("stored devices = %s", mustJSON(t, stored))
	}
	if stored[0].Permissions != PermAll || stored[0].Name != "iPhone of John" {
		t.Fatalf("device = %+v", stored[0])
	}

	// The cookie works like the bearer header.
	req, _ := http.NewRequest("GET", f.ts.URL+"/api/session", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var sess sessionDoc
	json.NewDecoder(res.Body).Decode(&sess)
	res.Body.Close()
	if res.StatusCode != 200 || !sess.Paired || sess.Device.ID != dev.ID || sess.Name != "FreeBeams Test" || !sess.Search {
		t.Fatalf("session: %d %+v", res.StatusCode, sess)
	}
}

func wrongCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDenyAndExpiry(t *testing.T) {
	f := newFixture(t)
	r := f.do("POST", "/api/pair", "", `{"code":"`+f.s.Status().Code+`","name":"Tablet"}`)
	id, secret := r.body["id"].(string), r.body["secret"].(string)
	if err := f.s.Deny(id); err != nil {
		t.Fatal(err)
	}
	w := f.do("POST", "/api/pair/wait", "", `{"id":"`+id+`","secret":"`+secret+`"}`)
	if w.body["status"] != "denied" || w.body["token"] != nil || len(f.s.StoredDevices()) != 0 {
		t.Fatalf("denied wait: %s", w.raw)
	}
	if _, err := f.s.Approve(id); err == nil {
		t.Fatal("a denied request was approved")
	}

	// A request nobody answers expires.
	r = f.do("POST", "/api/pair", "", `{"code":"`+f.s.Status().Code+`","name":"Late"}`)
	id = r.body["id"].(string)
	f.clock.Add(pendingTTL + time.Second)
	if p := f.s.Status().Pending; len(p) != 0 {
		t.Fatalf("pending after the TTL: %+v", p)
	}
	if _, err := f.s.Approve(id); err == nil {
		t.Fatal("an expired request was approved")
	}

	// Asking again from the same address replaces the waiting request.
	f.do("POST", "/api/pair", "", `{"code":"`+f.s.Status().Code+`","name":"First"}`)
	f.do("POST", "/api/pair", "", `{"code":"`+f.s.Status().Code+`","name":"Second"}`)
	if p := f.s.Status().Pending; len(p) != 1 || p[0].Name != "Second" {
		t.Fatalf("pending = %+v", p)
	}
}

func TestTokenAuth(t *testing.T) {
	f := newFixture(t)
	token, _ := f.pair("Phone")
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/session", ""},
		{"GET", "/api/state", ""},
		{"POST", "/api/action/next", "{}"},
		{"GET", "/api/songs?q=a", ""},
		{"POST", "/api/forget", "{}"},
		{"GET", "/api/ws", ""},
	} {
		for _, tok := range []string{"", "not-a-token", token + "x"} {
			if r := f.do(c.method, c.path, tok, c.body); r.status != http.StatusUnauthorized {
				t.Errorf("%s %s with token %q: %d, want 401", c.method, c.path, tok, r.status)
			}
		}
	}
	if r := f.do("GET", "/api/session", token, ""); r.status != 200 {
		t.Fatalf("session with the token: %d %s", r.status, r.raw)
	}
	if r := f.do("GET", "/api/songs?q=grace", token, ""); r.status != 200 || !strings.Contains(r.raw, "Amazing Grace (grace)") {
		t.Fatalf("songs: %d %s", r.status, r.raw)
	}
	// The page itself needs no token (it holds no data).
	if r := f.do("GET", "/", "", ""); r.status != 200 || !strings.Contains(r.raw, "FreeBeams Remote") {
		t.Fatalf("GET /: %d", r.status)
	}
}

func TestActionsAreWhitelisted(t *testing.T) {
	f := newFixture(t)
	token, _ := f.pair("Phone")
	cases := []struct {
		action, body  string
		status        int
		control, data string
	}{
		{"next", "", 200, "next", ""},
		{"prev", "{}", 200, "prev", ""},
		{"black", `{"on":true}`, 200, "black", `{"on":true}`},
		{"logo", `{}`, 200, "logo", ""},
		{"preview-item", `{"index":2,"path":"/etc/passwd"}`, 200, "select-schedule", `{"index":2}`},
		{"live-item", `{"index":0}`, 200, "remote-live-item", `{"index":0}`},
		{"live-slide", `{"index":3}`, 200, "jump", `{"kind":"slide","n":4}`},
		{"go-live", `{"slide":1}`, 200, "go-live", `{"slide":1}`},
		{"add-song", `{"id":42}`, 200, "remote-add-song", `{"id":42}`},
		{"audio-toggle", `{"index":3}`, 200, "audio-toggle", ""},
		{"audio-next", "", 200, "audio-next", ""},
		{"audio-prev", "{}", 200, "audio-prev", ""},
		{"audio-stop", "{}", 400, "", ""},
		{"playlist-add", `{"path":"/etc/passwd"}`, 400, "", ""},
		{"live-slide", `{"index":98}`, 400, "jump", `{"kind":"slide","n":99}`}, // the app's error comes back
		{"preview-item", `{}`, 400, "", ""},
		{"quit", `{}`, 400, "", ""},
		{"open-schedule", `{"path":"/tmp/x.fbs"}`, 400, "", ""},
		{"remote-approve", `{}`, 400, "", ""},
		{"native-key", `{}`, 400, "", ""},
	}
	for _, c := range cases {
		before := len(f.commands())
		r := f.do("POST", "/api/action/"+c.action, token, c.body)
		if r.status != c.status {
			t.Errorf("%s: %d %s, want %d", c.action, r.status, r.raw, c.status)
			continue
		}
		cmds := f.commands()
		if c.control == "" {
			if len(cmds) != before {
				t.Errorf("%s reached the app", c.action)
			}
			continue
		}
		if len(cmds) != before+1 {
			t.Errorf("%s: %d commands reached the app", c.action, len(cmds)-before)
			continue
		}
		got := cmds[len(cmds)-1]
		if got.Action != c.control || string(got.Data) != c.data || got.Query["remote"] == "" {
			t.Errorf("%s → %s %s (remote %q), want %s %s", c.action, got.Action, got.Data, got.Query["remote"], c.control, c.data)
		}
	}
}

func TestPermissions(t *testing.T) {
	f := newFixture(t)
	token, d := f.pair("Viewer")
	stored := f.s.StoredDevices()
	stored[0].Permissions = PermView
	f.s.SetDevices(stored)
	if r := f.do("POST", "/api/action/next", token, "{}"); r.status != http.StatusForbidden {
		t.Fatalf("next without PermControl: %d %s", r.status, r.raw)
	}
	if r := f.do("GET", "/api/songs", token, ""); r.status != http.StatusForbidden {
		t.Fatalf("songs without PermSchedule: %d", r.status)
	}
	if r := f.do("GET", "/api/state", token, ""); r.status != 200 {
		t.Fatalf("state with PermView: %d", r.status)
	}
	if len(f.commands()) != 0 {
		t.Fatal("a forbidden action reached the app")
	}
	_ = d
}

func TestRevocation(t *testing.T) {
	f := newFixture(t)
	token, d := f.pair("Phone")
	other, _ := f.pair("Tablet")
	conn := f.dial(t, token, "")
	readUntil(t, conn, "hello")

	if err := f.s.Revoke(d.ID); err != nil {
		t.Fatal(err)
	}
	// The open socket is told and closed at once.
	msg := readUntil(t, conn, "revoked")
	if msg["type"] != "revoked" {
		t.Fatalf("got %v", msg)
	}
	if _, _, err := conn.ReadMessage(); !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("after revoked: %v, want a policy-violation close", err)
	}
	for _, p := range []string{"/api/session", "/api/state"} {
		if r := f.do("GET", p, token, ""); r.status != http.StatusUnauthorized {
			t.Fatalf("%s after revoke: %d", p, r.status)
		}
	}
	if r := f.do("POST", "/api/action/next", token, "{}"); r.status != http.StatusUnauthorized {
		t.Fatalf("next after revoke: %d", r.status)
	}
	// Other devices keep working; a device can also forget itself.
	if r := f.do("GET", "/api/session", other, ""); r.status != 200 {
		t.Fatalf("the other device: %d", r.status)
	}
	if r := f.do("POST", "/api/forget", other, "{}"); r.status != 200 || !strings.Contains(r.header.Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("forget: %d %v", r.status, r.header)
	}
	if r := f.do("GET", "/api/session", other, ""); r.status != http.StatusUnauthorized {
		t.Fatalf("after forget: %d", r.status)
	}
	if n := len(f.s.StoredDevices()); n != 0 {
		t.Fatalf("%d devices left", n)
	}
	if err := f.s.Revoke(d.ID); err == nil {
		t.Fatal("revoking twice did not fail")
	}
}

func TestRateLimit(t *testing.T) {
	f := newFixture(t)
	h := f.s.Handler()
	post := func(ip, code string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "http://192.168.1.10:8765/api/pair", strings.NewReader(`{"code":"`+code+`","name":"x"}`))
		req.RemoteAddr = ip + ":50000"
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	code := f.s.Status().Code
	bad := wrongCode(code)
	for i := 0; i < maxFailsPerIP; i++ {
		if rec := post("192.168.1.50", bad); rec.Code != http.StatusForbidden {
			t.Fatalf("attempt %d: %d", i+1, rec.Code)
		}
	}
	// Locked out, even with the right code.
	rec := post("192.168.1.50", code)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("after %d wrong codes: %d %v", maxFailsPerIP, rec.Code, rec.Header())
	}
	// Another address is not locked out.
	if rec := post("192.168.1.51", code); rec.Code != 200 {
		t.Fatalf("another address: %d %s", rec.Code, rec.Body)
	}
	// The window slides.
	f.clock.Add(pairWindow)
	if rec := post("192.168.1.50", f.s.Status().Code); rec.Code != 200 {
		t.Fatalf("after the window: %d %s", rec.Code, rec.Body)
	}

	// Many addresses guessing together trip the global limit, which changes the code.
	f.clock.Add(pairWindow)
	code = f.s.Status().Code
	bad = wrongCode(code)
	for i := 0; i < maxFailsGlobal; i++ {
		post("10.0.0."+itoa(i+1), bad)
	}
	if f.s.Status().Code == code {
		t.Fatal("the code did not change after the global limit")
	}
	if rec := post("10.0.1.1", f.s.Status().Code); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("a new address during the global lockout: %d", rec.Code)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestRequestGuards(t *testing.T) {
	f := newFixture(t)
	token, _ := f.pair("Phone")
	h := f.s.Handler()
	serve := func(method, target, remote, host string, hdr map[string]string) int {
		req := httptest.NewRequest(method, target, strings.NewReader(`{}`))
		req.RemoteAddr = remote
		req.Host = host
		req.Header.Set("Authorization", "Bearer "+token)
		if method == "POST" {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	cases := []struct {
		name                 string
		method, remote, host string
		hdr                  map[string]string
		want                 int
	}{
		{"LAN peer", "GET", "192.168.1.20:5000", "192.168.1.10:8765", nil, 200},
		{"public peer", "GET", "8.8.8.8:5000", "192.168.1.10:8765", nil, http.StatusForbidden},
		{"IPv6 ULA peer", "GET", "[fd00::5]:5000", "[fd00::1]:8765", nil, 200},
		{"rebinding Host", "GET", "192.168.1.20:5000", "evil.example:8765", nil, http.StatusMisdirectedRequest},
		{"bonjour Host", "GET", "192.168.1.20:5000", "Godwins-Mac.local:8765", nil, 200},
		{"cross-origin POST", "POST", "192.168.1.20:5000", "192.168.1.10:8765", map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		{"same-origin POST", "POST", "192.168.1.20:5000", "192.168.1.10:8765", map[string]string{"Origin": "http://192.168.1.10:8765"}, 200},
		{"form POST", "POST", "192.168.1.20:5000", "192.168.1.10:8765", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, http.StatusUnsupportedMediaType},
	}
	for _, c := range cases {
		target := "http://" + c.host + "/api/session"
		if c.method == "POST" {
			target = "http://" + c.host + "/api/action/next"
		}
		if got := serve(c.method, target, c.remote, c.host, c.hdr); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

// dial opens the WebSocket with the token as a cookie and origin as Origin
// ("" = the server's own origin).
func (f *fixture) dial(t *testing.T, token, origin string) *websocket.Conn {
	t.Helper()
	u, _ := url.Parse(f.ts.URL)
	if origin == "" {
		origin = "http://" + u.Host
	}
	hdr := http.Header{"Origin": {origin}, "Cookie": {CookieName + "=" + token}}
	conn, res, err := websocket.DefaultDialer.Dial("ws://"+u.Host+"/api/ws", hdr)
	if err != nil {
		status := 0
		if res != nil {
			status = res.StatusCode
		}
		t.Fatalf("dial: %v (%d)", err, status)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// readUntil reads messages until one has the given type.
func readUntil(t *testing.T, conn *websocket.Conn, typ string) map[string]any {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for %q: %v", typ, err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("not JSON: %s", data)
		}
		if m["type"] == typ {
			return m
		}
	}
}

func TestWebSocketOriginCheck(t *testing.T) {
	f := newFixture(t)
	token, _ := f.pair("Phone")
	u, _ := url.Parse(f.ts.URL)
	for _, origin := range []string{"http://evil.example", "null", "http://" + u.Hostname() + ":1", "file://"} {
		hdr := http.Header{"Origin": {origin}, "Cookie": {CookieName + "=" + token}}
		_, res, err := websocket.DefaultDialer.Dial("ws://"+u.Host+"/api/ws", hdr)
		if err == nil || res == nil || res.StatusCode != http.StatusForbidden {
			t.Errorf("origin %q: err %v, status %v; want 403", origin, err, res)
		}
	}
	// No Origin at all (not a browser) is refused too.
	_, res, err := websocket.DefaultDialer.Dial("ws://"+u.Host+"/api/ws", http.Header{"Cookie": {CookieName + "=" + token}})
	if err == nil || res.StatusCode != http.StatusForbidden {
		t.Errorf("no origin: %v", err)
	}
	f.dial(t, token, "") // the page's own origin is accepted
}

func TestWebSocketStateAndCommands(t *testing.T) {
	f := newFixture(t)
	token, d := f.pair("Phone")
	f.s.Publish(State{Seq: 1, Name: "FreeBeams Test", Mode: "normal", Schedule: []ItemDoc{}})
	conn := f.dial(t, token, "")
	hello := readUntil(t, conn, "hello")
	sess := hello["session"].(map[string]any)
	if sess["device"].(map[string]any)["id"] != d.ID || sess["permissions"] != float64(PermAll) {
		t.Fatalf("hello = %v", hello)
	}
	// The newest state comes first, then every change within a frame.
	if st := readUntil(t, conn, "state")["state"].(map[string]any); st["seq"] != float64(1) {
		t.Fatalf("first state = %v", st)
	}
	start := time.Now()
	f.s.Publish(State{Seq: 2, Mode: "black", Schedule: []ItemDoc{}})
	if st := readUntil(t, conn, "state")["state"].(map[string]any); st["seq"] != float64(2) || st["mode"] != "black" {
		t.Fatalf("pushed state = %v", st)
	}
	if lag := time.Since(start); lag > 200*time.Millisecond {
		t.Errorf("the state took %v to arrive", lag)
	}
	if f.s.Status().Clients != 1 || !f.s.Status().Devices[0].Online {
		t.Errorf("status = %+v", f.s.Status())
	}

	// An action over the socket reaches the main loop and is answered by id.
	conn.WriteJSON(map[string]any{"id": 5, "action": "next"})
	if r := readUntil(t, conn, "reply"); r["id"] != float64(5) || r["ok"] != true {
		t.Fatalf("reply = %v", r)
	}
	if cmds := f.commands(); len(cmds) != 1 || cmds[0].Action != "next" {
		t.Fatalf("commands = %v", cmds)
	}
	conn.WriteJSON(map[string]any{"id": 6, "action": "quit"})
	if r := readUntil(t, conn, "reply"); r["id"] != float64(6) || r["ok"] != false || !strings.Contains(r["error"].(string), "unknown action") {
		t.Fatalf("reply = %v", r)
	}
	conn.WriteJSON(map[string]any{"id": 7, "action": "ping"})
	if r := readUntil(t, conn, "reply"); r["id"] != float64(7) || r["ok"] != true {
		t.Fatalf("ping reply = %v", r)
	}
	// Identical states are not sent again.
	f.s.Publish(State{Seq: 2, Mode: "black", Schedule: []ItemDoc{}})
	f.s.Publish(State{Seq: 3, Mode: "clear", Schedule: []ItemDoc{}})
	if st := readUntil(t, conn, "state")["state"].(map[string]any); st["seq"] != float64(3) {
		t.Fatalf("after a duplicate: %v", st)
	}
	// /api/state serves the same document.
	if r := f.do("GET", "/api/state", token, ""); r.body["mode"] != "clear" {
		t.Fatalf("GET /api/state = %s", r.raw)
	}
}

func TestStopClosesEverything(t *testing.T) {
	s := New(Options{Logf: t.Logf})
	if err := s.Start(0); err != nil {
		t.Fatal(err)
	}
	st := s.Status()
	if !st.Running || st.Port == 0 || !strings.HasPrefix(st.URL, "http://") || !strings.HasSuffix(st.URL, ":"+itoa(st.Port)+"/") {
		t.Fatalf("status = %+v", st)
	}
	res, err := http.Get("http://127.0.0.1:" + itoa(st.Port) + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("GET /: %d %v", res.StatusCode, res.Header)
	}
	s.Stop()
	if s.Running() {
		t.Fatal("still running")
	}
	if _, err := http.Get("http://127.0.0.1:" + itoa(st.Port) + "/"); err == nil {
		t.Fatal("the port still answers after Stop")
	}
	if err := s.Start(st.Port); err != nil { // and it starts again
		t.Fatal(err)
	}
	s.Stop()
}

func TestBuildState(t *testing.T) {
	sched := models.NewSchedule()
	sched.Title = "Sunday"
	song := models.ScheduleItem{ID: "a", Kind: models.ItemSong, Title: "Amazing Grace", Subtitle: "John Newton",
		Slides: []models.Slide{{Kind: models.SlideSong, Label: "Verse 1", Text: "Amazing grace! How sweet the sound\nThat saved a wretch like me!"},
			{Kind: models.SlideSong, Label: "Verse 2", Text: "'Twas grace that taught\nmy heart to fear"}}}
	verse := models.ScheduleItem{ID: "b", Kind: models.ItemScripture, Title: "John 3:16", Subtitle: "KJV",
		Slides: []models.Slide{{Kind: models.SlideScripture, Label: "John 3:16", Text: "For God so loved the world", Footer: "John 3:16 KJV"}}}
	sched.Items = []models.ScheduleItem{song, verse}
	live := &models.Deck{ItemID: "a", Title: song.Title, Slides: song.Slides}
	prev := &models.Deck{ItemID: "b", Title: verse.Title, Slides: verse.Slides}
	st := BuildState(StateInput{Name: "FB", Schedule: sched, Selected: 1, Presenter: models.PresenterState{
		Live: live, LiveIndex: 1, Preview: prev, PreviewIndex: 0, Mode: models.ModeBlack, OutputEnabled: true, Seq: 9}})

	if st.Seq != 9 || st.Mode != "black" || !st.Output || st.Title != "Sunday" || st.Selected != 1 || st.LiveItem != 0 {
		t.Fatalf("state = %+v", st)
	}
	if len(st.Schedule) != 2 || st.Schedule[0] != (ItemDoc{Title: "Amazing Grace", Subtitle: "John Newton", Kind: "song", Slides: 2}) {
		t.Fatalf("schedule = %+v", st.Schedule)
	}
	if st.Live.Item != 0 || st.Live.Index != 1 || st.Live.Label != "Verse 2" || st.Live.Text != "'Twas grace that taught\nmy heart to fear" {
		t.Fatalf("live = %+v", st.Live)
	}
	if st.Live.Slides[0].Snippet != "Amazing grace! How sweet the sound / That saved a wretch like me!" {
		t.Fatalf("snippet = %q", st.Live.Slides[0].Snippet)
	}
	if st.Preview.Item != 1 || st.Preview.Footer != "John 3:16 KJV" || st.Preview.Slides[0].Kind != "scripture" {
		t.Fatalf("preview = %+v", st.Preview)
	}

	// The JSON the phone reads (web/app.js) has these names.
	var doc map[string]any
	json.Unmarshal([]byte(mustJSON(t, st)), &doc)
	for _, k := range []string{"seq", "name", "mode", "output", "schedule", "title", "selected", "liveItem", "preview", "live", "audio"} {
		if _, ok := doc[k]; !ok {
			t.Errorf("state JSON lacks %q", k)
		}
	}
	for _, k := range []string{"title", "item", "index", "slides", "label", "text", "footer"} {
		if _, ok := doc["live"].(map[string]any)[k]; !ok {
			t.Errorf("deck JSON lacks %q", k)
		}
	}

	music := BuildState(StateInput{Selected: -1, Audio: AudioDoc{Title: "Prelude", Status: "playing", Tracks: 3}})
	if b := mustJSON(t, music); !strings.Contains(b, `"audio":{"title":"Prelude","status":"playing","tracks":3}`) {
		t.Fatalf("audio state = %s", b)
	}

	// Nothing loaded: empty lists, not null, and -1 indexes.
	empty := BuildState(StateInput{Selected: -1})
	if empty.Audio.Status != "stopped" || empty.Audio.Tracks != 0 {
		t.Fatalf("empty audio = %+v", empty.Audio)
	}
	if b := mustJSON(t, empty); !strings.Contains(b, `"schedule":[]`) || !strings.Contains(b, `"slides":[]`) || empty.Live.Index != -1 || empty.LiveItem != -1 || empty.Mode != "normal" {
		t.Fatalf("empty state = %s", b)
	}
}

func TestSnippetAndLabels(t *testing.T) {
	cases := []struct {
		text string
		n    int
		want string
	}{
		{"one\ntwo", 90, "one / two"},
		{"  spaced   out \n\n next ", 90, "spaced out / next"},
		{"abcdefghij", 6, "abcde…"},
		{"ab / cd\nef", 7, "ab / c…"},
		{"", 10, ""},
	}
	for _, c := range cases {
		if got := Snippet(c.text, c.n); got != c.want {
			t.Errorf("Snippet(%q, %d) = %q, want %q", c.text, c.n, got, c.want)
		}
	}
	if l := SlideLabel(models.Slide{}, 2); l != "Slide 3" {
		t.Errorf("label = %q", l)
	}
}

func TestNamesAndKinds(t *testing.T) {
	for in, want := range map[string]string{
		"iPhone of John":            "iPhone of John",
		"  a\tb\nc  ":               "a b c",
		"":                          "Unnamed device",
		"x‮y":                       "x y",
		strings.Repeat("é", 50):     strings.Repeat("é", 40),
		"<script>alert(1)</script>": "<script>alert(1)</script>", // shown as text, never as HTML
	} {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
	for ua, want := range map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)":          "phone",
		"Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X)":                   "tablet",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) Mobile Safari/537.36":   "phone",
		"Mozilla/5.0 (Linux; Android 14; SM-X710) Safari/537.36":          "tablet",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15": "computer",
	} {
		if got := DeviceKind(ua); got != want {
			t.Errorf("DeviceKind(%q) = %q, want %q", ua, got, want)
		}
	}
}

func TestAddressChecks(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:1": true, "192.168.0.4:1": true, "10.1.2.3:1": true, "172.16.0.1:1": true, "172.32.0.1:1": false,
		"100.100.1.1:1": true, "[::1]:1": true, "[fe80::1]:1": true, "[fd12::1]:1": true, "[2001:db8::1]:1": false,
		"8.8.8.8:1": false, "[::ffff:192.168.1.2]:1": true, "garbage": false,
	} {
		if got := localPeer(addr); got != want {
			t.Errorf("localPeer(%q) = %v", addr, got)
		}
	}
	for host, want := range map[string]bool{
		"192.168.1.10:8765": true, "[fd00::1]:8765": true, "localhost:8765": true, "Mac.local": true, "mac.local.:8765": true,
		"evil.example": false, "192.168.1.10.nip.io:8765": false, "": false,
	} {
		if got := localHost(host); got != want {
			t.Errorf("localHost(%q) = %v", host, got)
		}
	}
	for _, c := range []struct {
		origin, host string
		want         bool
	}{
		{"http://192.168.1.10:8765", "192.168.1.10:8765", true},
		{"HTTP://Mac.Local:8765", "mac.local:8765", true},
		{"http://192.168.1.10:8766", "192.168.1.10:8765", false},
		{"http://evil.example", "192.168.1.10:8765", false},
		{"null", "192.168.1.10:8765", false},
		{"", "192.168.1.10:8765", false},
	} {
		if got := sameOrigin(c.origin, c.host); got != c.want {
			t.Errorf("sameOrigin(%q, %q) = %v", c.origin, c.host, got)
		}
	}
}

func TestQRCode(t *testing.T) {
	m, err := QRModules("http://192.168.1.10:8765/")
	if err != nil {
		t.Fatal(err)
	}
	n := len(m)
	if n < 21+2*quietZone || (n-2*quietZone-21)%4 != 0 {
		t.Fatalf("size %d is not a QR version", n)
	}
	// The three finder patterns: a dark 7×7 ring around a light ring around a 3×3 core.
	for _, o := range [][2]int{{quietZone, quietZone}, {n - quietZone - 7, quietZone}, {quietZone, n - quietZone - 7}} {
		for y := 0; y < 7; y++ {
			for x := 0; x < 7; x++ {
				ring := x == 0 || y == 0 || x == 6 || y == 6
				core := x >= 2 && x <= 4 && y >= 2 && y <= 4
				if got := m[o[1]+y][o[0]+x]; got != (ring || core) {
					t.Fatalf("finder at %v: module (%d,%d) = %v", o, x, y, got)
				}
			}
		}
	}
	for i := 0; i < n; i++ {
		if m[0][i] || m[i][0] || m[n-1][i] || m[i][n-1] {
			t.Fatal("the quiet zone is not light")
		}
	}
	side, px, err := QRPixels("http://192.168.1.10:8765/", 4)
	if err != nil || side != n*4 || len(px) != side*side*4 || px[0] != 255 || px[(quietZone*4*side+quietZone*4)*4] != 0 {
		t.Fatalf("pixels: side %d, %d bytes, err %v", side, len(px), err)
	}
}

// The phone app must work offline: no CDN, font or analytics URL anywhere.
func TestWebAppIsSelfContained(t *testing.T) {
	ext := regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}`)
	err := fs.WalkDir(web, "web", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := web.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(b)
		if strings.HasSuffix(path, ".svg") {
			text = strings.ReplaceAll(text, "http://www.w3.org/2000/svg", "")
		}
		if m := ext.FindString(text); m != "" {
			t.Errorf("%s refers to %q", path, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"web/index.html", "web/app.js", "web/app.css", "web/icon.svg", "web/manifest.webmanifest"} {
		if _, err := web.ReadFile(p); err != nil {
			t.Errorf("missing %s", p)
		}
	}
}

func TestLimiter(t *testing.T) {
	var l limiter
	now := time.Unix(1000, 0)
	for i := 0; i < maxFailsPerIP; i++ {
		if w := l.blocked("a", now); w != 0 {
			t.Fatalf("blocked after %d", i)
		}
		l.fail("a", now.Add(time.Duration(i)*time.Second))
	}
	if w := l.blocked("a", now.Add(10*time.Second)); w <= 0 || w > pairWindow {
		t.Fatalf("wait = %v", w)
	}
	if w := l.blocked("a", now.Add(pairWindow+5*time.Second)); w != 0 {
		t.Fatalf("still blocked after the window: %v", w)
	}
	if l.remaining("b", now) != maxFailsPerIP {
		t.Fatal("a fresh address has no tries")
	}
}
