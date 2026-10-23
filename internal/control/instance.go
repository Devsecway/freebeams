package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Instance is what a running app publishes about its control server so that
// a second launch (a double-clicked schedule or Bible package while the app
// is already open) can hand its files over instead of opening another window
// on the same library. The file sits next to the library (InstancePath), is
// readable by its owner only, and is removed when the app quits.
type Instance struct {
	Addr string `json:"addr"`
	Key  string `json:"key"`
	PID  int    `json:"pid"`
}

// InstancePath is the instance file of the app that has library open.
func InstancePath(library string) string { return library + ".instance" }

// WriteInstance publishes s at path.
func WriteInstance(path string, s *Server) error {
	b, err := json.Marshal(Instance{Addr: s.Addr(), Key: s.Key(), PID: os.Getpid()})
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write instance file: %w", err)
	}
	return nil
}

// RemoveInstance deletes path when it still describes s (a later instance
// may have replaced it).
func RemoveInstance(path string, s *Server) {
	if in, err := ReadInstance(path); err == nil && in.Addr == s.Addr() && in.Key == s.Key() {
		os.Remove(path)
	}
}

// ReadInstance reads an instance file.
func ReadInstance(path string) (Instance, error) {
	var in Instance
	b, err := os.ReadFile(path)
	if err != nil {
		return in, err
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return in, fmt.Errorf("instance file %s: %w", path, err)
	}
	if in.Addr == "" || in.Key == "" {
		return in, fmt.Errorf("instance file %s: no address or key", path)
	}
	return in, nil
}

// forwardTimeout bounds a hand-over: a stale file (the app crashed) points
// at a closed port and fails at once; a hung app must not hang the launch.
const forwardTimeout = 3 * time.Second

// Forward runs action with body on the instance's control server.
func Forward(in Instance, action string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+in.Addr+"/action/"+action, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+in.Key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: forwardTimeout}).Do(req)
	if err != nil {
		return fmt.Errorf("forward %s: %w", action, err)
	}
	defer resp.Body.Close()
	var res Result
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("forward %s: %s: %w", action, resp.Status, err)
	}
	if !res.OK {
		if res.Error == "" {
			res.Error = resp.Status
		}
		return errors.New(res.Error)
	}
	return nil
}
