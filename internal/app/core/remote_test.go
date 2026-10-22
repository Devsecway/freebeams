package core

import (
	"strings"
	"testing"
	"time"

	"freebeams/internal/remote"
)

func TestRemoteStatusText(t *testing.T) {
	on := remote.Status{Running: true, URL: "http://192.168.1.20:8765/"}
	tests := []struct {
		name    string
		enabled bool
		st      remote.Status
		err     string
		want    string
		isErr   bool
	}{
		{"off", false, remote.Status{}, "", "Off.", false},
		{"enabled but not running", true, remote.Status{}, "", "Off.", false},
		{"start failed", true, remote.Status{}, "listen on port 80: denied", "could not start: listen on port 80: denied", true},
		{"on, nobody", true, on, "", "On at http://192.168.1.20:8765/. No device is connected.", false},
		{"on, one", true, remote.Status{Running: true, URL: on.URL, Clients: 1}, "", "1 device is connected.", false},
		{"on, two", true, remote.Status{Running: true, URL: on.URL, Clients: 2}, "", "2 devices are connected.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, isErr := RemoteStatusText(tt.enabled, tt.st, tt.err)
			if !strings.Contains(got, tt.want) || isErr != tt.isErr {
				t.Fatalf("got %q, %v; want %q, %v", got, isErr, tt.want, tt.isErr)
			}
		})
	}
}

func TestRemoteHelpers(t *testing.T) {
	for _, tt := range []struct {
		port int
		ok   bool
	}{{8765, true}, {1024, true}, {65535, true}, {0, false}, {80, false}, {70000, false}} {
		if err := ValidRemotePort(tt.port); (err == nil) != tt.ok {
			t.Errorf("ValidRemotePort(%d) = %v", tt.port, err)
		}
	}
	if got := RemoteServerName("Godwins-MacBook-Pro.local"); got != "FreeBeams on Godwins-MacBook-Pro" {
		t.Errorf("RemoteServerName = %q", got)
	}
	if got := RemoteServerName("Mac.lan"); got != "FreeBeams on Mac" {
		t.Errorf("RemoteServerName(Mac.lan) = %q", got)
	}
	if got := RemoteServerName("  "); got != "FreeBeams" {
		t.Errorf("RemoteServerName(blank) = %q", got)
	}
	if FormatPairingCode("123456") != "123 456" || FormatPairingCode("12") != "12" {
		t.Error("FormatPairingCode")
	}
	if RemoteDeviceKind("phone") != "Phone" || RemoteDeviceKind("tablet") != "Tablet" || RemoteDeviceKind("?") != "Device" {
		t.Error("RemoteDeviceKind")
	}

	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		d    remote.DeviceInfo
		want string
	}{
		{remote.DeviceInfo{Online: true}, "Connected"},
		{remote.DeviceInfo{}, "Not connected yet"},
		{remote.DeviceInfo{LastSeen: now.Add(-10 * time.Second)}, "Seen just now"},
		{remote.DeviceInfo{LastSeen: now.Add(-5 * time.Minute)}, "Seen 5 min ago"},
		{remote.DeviceInfo{LastSeen: now.Add(-3 * time.Hour)}, "Seen 3 h ago"},
	} {
		if got := RemoteDeviceStatus(tt.d, now); got != tt.want {
			t.Errorf("RemoteDeviceStatus(%+v) = %q, want %q", tt.d, got, tt.want)
		}
	}

	pending := []remote.PairRequest{{ID: "a1", Name: "Ann's iPhone"}, {ID: "b2", Name: "Pixel"}}
	if FindRemoteRequest(pending, "b2", "") != 1 || FindRemoteRequest(pending, "", "pixel") != 1 ||
		FindRemoteRequest(pending, "", "") != 0 || FindRemoteRequest(pending, "zz", "") != -1 || FindRemoteRequest(nil, "", "") != -1 {
		t.Error("FindRemoteRequest")
	}
	devices := []remote.DeviceInfo{{ID: "d1", Name: "iPad"}}
	if FindRemoteDevice(devices, "d1", "") != 0 || FindRemoteDevice(devices, "", "IPAD") != 0 ||
		FindRemoteDevice(devices, "", "") != -1 || FindRemoteDevice(devices, "x", "") != -1 {
		t.Error("FindRemoteDevice")
	}
}

func TestRemoteStateNormalized(t *testing.T) {
	st := BuildState(StateInput{Remote: RemoteState{Enabled: true}})
	if st.Remote.Pending == nil || st.Remote.Devices == nil || st.Remote.Addresses == nil || !st.Remote.Enabled {
		t.Fatalf("remote state = %+v", st.Remote)
	}
}
