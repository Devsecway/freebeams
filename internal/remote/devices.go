package remote

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
	"unicode"
)

// Permissions are what an approved device may do, as EasyWorship's
// RemoteControlClients.db stores per client (device_permissions).
type Permissions uint32

const (
	// PermView reads the live state (schedule, slides, live text).
	PermView Permissions = 1 << iota
	// PermControl drives the presentation: next / previous, go live, black,
	// clear, logo, preview a schedule item.
	PermControl
	// PermSchedule adds library songs to the schedule.
	PermSchedule

	// PermAll is what an approval grants.
	PermAll = PermView | PermControl | PermSchedule
)

// Has reports whether p includes every permission in q.
func (p Permissions) Has(q Permissions) bool { return p&q == q }

// Device is an approved phone or tablet. Only a hash of its token is kept: a
// copy of the library database does not let anyone in.
type Device struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Kind        string      `json:"kind"` // phone | tablet | computer
	TokenHash   string      `json:"tokenHash"`
	Permissions Permissions `json:"permissions"`
	Created     time.Time   `json:"created"`
	LastSeen    time.Time   `json:"lastSeen,omitzero"`
}

// DeviceInfo is a device as the operator sees it (no token hash).
type DeviceInfo struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Online   bool      `json:"online"` // a live connection is open
	Created  time.Time `json:"created"`
	LastSeen time.Time `json:"lastSeen,omitzero"`
}

// tokenBytes is the entropy of a device token (256 bits).
const tokenBytes = 32

// newToken returns a random device token (base64url, 43 characters).
func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken is the stored form of a token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// randomID returns n random bytes as hex (device and request ids, secrets).
func randomID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// maxNameRunes bounds a device name.
const maxNameRunes = 40

// CleanName makes a device name safe to show: control characters dropped,
// whitespace collapsed, at most 40 characters; "" becomes "Unnamed device".
func CleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == ' ' || r == ' ' || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxNameRunes {
		s = strings.TrimSpace(string(r[:maxNameRunes]))
	}
	if s == "" {
		return "Unnamed device"
	}
	return s
}

// DeviceKind guesses phone / tablet / computer from a User-Agent.
func DeviceKind(userAgent string) string {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "ipad"), strings.Contains(ua, "tablet"),
		strings.Contains(ua, "android") && !strings.Contains(ua, "mobile"):
		return "tablet"
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipod"), strings.Contains(ua, "android"),
		strings.Contains(ua, "mobile"):
		return "phone"
	}
	return "computer"
}
