package control

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve starts the handler and a fake main loop that answers commands with fn.
func serve(t *testing.T, fn func(*Command)) (*Server, *httptest.Server) {
	t.Helper()
	t.Setenv(KeyEnv, "test-key")
	s := New(0)
	ts := httptest.NewServer(s.Handler())
	done := make(chan struct{})
	go func() {
		for {
			select {
			case cmd := <-s.Commands():
				fn(cmd)
			case <-done:
				return
			}
		}
	}()
	t.Cleanup(func() { close(done); ts.Close() })
	return s, ts
}

func do(t *testing.T, ts *httptest.Server, method, path, key, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestAuth(t *testing.T) {
	_, ts := serve(t, func(c *Command) { c.RespondJSON(map[string]string{"a": c.Action}) })
	tests := []struct {
		name, key string
		want      int
	}{
		{"missing key", "", http.StatusUnauthorized},
		{"wrong key", "nope", http.StatusUnauthorized},
		{"right key", "test-key", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, _ := do(t, ts, "GET", "/state", tt.key, "")
			if resp.StatusCode != tt.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.want)
			}
		})
	}
}

func TestActionDispatch(t *testing.T) {
	_, ts := serve(t, func(c *Command) {
		switch c.Action {
		case "go-live":
			var in struct{ Slide int }
			if err := json.Unmarshal(c.Data, &in); err != nil {
				c.Respond(Result{Error: err.Error()})
				return
			}
			c.RespondJSON(map[string]int{"slide": in.Slide})
		case "screenshot":
			c.Respond(Result{OK: true, PNG: []byte("\x89PNG" + c.Query["window"])})
		default:
			c.Respond(Result{Error: "unknown action"})
		}
	})

	resp, body := do(t, ts, "POST", "/action/go-live", "test-key", `{"slide":3}`)
	if resp.StatusCode != 200 || !strings.Contains(body, `"slide":3`) {
		t.Fatalf("go-live: %d %s", resp.StatusCode, body)
	}

	resp, body = do(t, ts, "POST", "/action/nope", "test-key", "")
	if resp.StatusCode != 400 || !strings.Contains(body, "unknown action") {
		t.Fatalf("unknown: %d %s", resp.StatusCode, body)
	}

	resp, body = do(t, ts, "POST", "/action/go-live", "test-key", `{bad json`)
	if resp.StatusCode != 400 || !strings.Contains(body, "not valid JSON") {
		t.Fatalf("bad json: %d %s", resp.StatusCode, body)
	}

	resp, body = do(t, ts, "POST", "/action/Bad.Name", "test-key", "")
	if resp.StatusCode != 400 {
		t.Fatalf("bad name: %d %s", resp.StatusCode, body)
	}

	resp, body = do(t, ts, "GET", "/screenshot?window=output", "test-key", "")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || body != "\x89PNGoutput" {
		t.Fatalf("screenshot: %d %q %q", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
}
