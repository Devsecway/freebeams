package core

import (
	"fmt"
	"strings"
	"time"

	"freebeams/internal/remote"
)

// Setting keys of the LAN remote (internal/remote, app/remote.go).
const (
	SettingRemoteEnabled = "remote.enabled" // bool: start the remote server at launch
	SettingRemotePort    = "remote.port"    // int: its TCP port
	SettingRemoteDevices = "remote.devices" // []remote.Device: approved devices (token hashes only)
)

// RemoteState is the remote control in GET /state: the setting, the
// server's status (address, pairing code, waiting and approved devices) and
// which remote dialog is open.
type RemoteState struct {
	Enabled        bool   `json:"enabled"`
	ConfiguredPort int    `json:"configuredPort"`
	Error          string `json:"error,omitempty"` // why the server could not start
	Dialog         string `json:"dialog,omitempty"`
	remote.Status
}

// normalized gives the lists empty values instead of null.
func (r RemoteState) normalized() RemoteState {
	if r.Pending == nil {
		r.Pending = []remote.PairRequest{}
	}
	if r.Devices == nil {
		r.Devices = []remote.DeviceInfo{}
	}
	if r.Addresses == nil {
		r.Addresses = []string{}
	}
	return r
}

// ValidRemotePort checks a remote port setting (0 is not allowed: a phone
// must find the same port every time).
func ValidRemotePort(p int) error {
	if p < 1024 || p > 65535 {
		return fmt.Errorf("remote port %d: choose a port between 1024 and 65535", p)
	}
	return nil
}

// RemoteServerName is what the phone shows as the computer's name:
// "FreeBeams on Godwins-MacBook-Pro" from the host name (the domain, such as
// .local or a router's .lan, is dropped).
func RemoteServerName(hostname string) string {
	h, _, _ := strings.Cut(strings.TrimSpace(hostname), ".")
	if h == "" {
		return "FreeBeams"
	}
	return "FreeBeams on " + h
}

// RemoteDeviceStatus describes a device for the devices list: "Connected",
// or when it was last seen ("Seen 5 min ago", "Not seen yet").
func RemoteDeviceStatus(d remote.DeviceInfo, now time.Time) string {
	if d.Online {
		return "Connected"
	}
	if d.LastSeen.IsZero() {
		return "Not connected yet"
	}
	ago := now.Sub(d.LastSeen)
	switch {
	case ago < time.Minute:
		return "Seen just now"
	case ago < time.Hour:
		return fmt.Sprintf("Seen %d min ago", int(ago.Minutes()))
	case ago < 24*time.Hour:
		return fmt.Sprintf("Seen %d h ago", int(ago.Hours()))
	}
	return "Seen " + d.LastSeen.Local().Format("Jan 2")
}

// RemoteDeviceKind is a device kind for display ("phone" → "Phone").
func RemoteDeviceKind(kind string) string {
	switch kind {
	case "phone":
		return "Phone"
	case "tablet":
		return "Tablet"
	case "computer":
		return "Computer"
	}
	return "Device"
}

// FormatPairingCode spaces a six-digit code for reading aloud ("123 456").
func FormatPairingCode(code string) string {
	if len(code) == 6 {
		return code[:3] + " " + code[3:]
	}
	return code
}

// RemoteStatusText is the remote's status line in the pairing dialog and the
// Options page, and whether it reports an error.
func RemoteStatusText(enabled bool, st remote.Status, startErr string) (string, bool) {
	switch {
	case startErr != "":
		return "The remote control could not start: " + startErr, true
	case !enabled || !st.Running:
		return "Off. Turn it on to pair a phone or tablet.", false
	}
	text := "On at " + st.URL
	switch st.Clients {
	case 0:
		text += ". No device is connected."
	case 1:
		text += ". 1 device is connected."
	default:
		text += fmt.Sprintf(". %d devices are connected.", st.Clients)
	}
	return text, false
}

// FindRemoteRequest finds a waiting pairing request by id or by name
// (case-insensitive); "" for both picks the first one. -1 when none matches.
func FindRemoteRequest(pending []remote.PairRequest, id, name string) int {
	for i, p := range pending {
		switch {
		case id != "" && p.ID == id:
			return i
		case id == "" && name != "" && strings.EqualFold(p.Name, name):
			return i
		case id == "" && name == "":
			return i
		}
	}
	return -1
}

// FindRemoteDevice finds an approved device by id or name; -1 when none.
func FindRemoteDevice(devices []remote.DeviceInfo, id, name string) int {
	for i, d := range devices {
		if (id != "" && d.ID == id) || (id == "" && name != "" && strings.EqualFold(d.Name, name)) {
			return i
		}
	}
	return -1
}
