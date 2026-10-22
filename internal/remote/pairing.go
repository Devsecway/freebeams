package remote

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"time"
)

// Pairing: a phone that knows the pairing code shown in FreeBeams asks to be
// paired; the operator approves (or denies) the request by name; the phone,
// which holds the request's secret, then collects a device token. Wrong codes
// are rate-limited per address and in total.
const (
	codeDigits = 6

	pairWindow     = 5 * time.Minute // the rate limiter's window
	maxFailsPerIP  = 5               // wrong codes per address per window
	maxFailsGlobal = 20              // wrong codes from everyone per window (then the code changes)

	pendingTTL = 2 * time.Minute // how long a request waits for the operator
	collectTTL = 2 * time.Minute // how long an approved token waits for its phone
	maxPending = 5               // requests waiting at once (one per address)
)

// PairRequest is a phone waiting for the operator's approval.
type PairRequest struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Kind    string    `json:"kind"`
	Address string    `json:"address"` // the phone's IP address
	Created time.Time `json:"created"`

	secretHash string
	status     string // pairPending | pairApproved | pairDenied
	token      string // the device token, from approval until collected
	device     Device
	decided    time.Time
	done       chan struct{} // closed when the operator decides
}

const (
	pairPending  = "pending"
	pairApproved = "approved"
	pairDenied   = "denied"
	pairExpired  = "expired"
)

// newCode returns a random six-digit pairing code.
func newCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		// crypto/rand does not fail on supported platforms; a code nobody can
		// guess is still better than none.
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1_000_000)
	}
	return fmt.Sprintf("%0*d", codeDigits, n.Int64())
}

// codeMatches compares a submitted code with the current one in constant time.
func codeMatches(got, want string) bool {
	return len(got) == len(want) && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// limiter counts failed pairing attempts in a sliding window.
type limiter struct {
	fails map[string][]time.Time // by address
	all   []time.Time
}

func prune(ts []time.Time, now time.Time) []time.Time {
	i := 0
	for i < len(ts) && now.Sub(ts[i]) >= pairWindow {
		i++
	}
	return ts[i:]
}

// blocked returns how long ip must wait before trying again (0: it may try).
func (l *limiter) blocked(ip string, now time.Time) time.Duration {
	l.all = prune(l.all, now)
	if l.fails == nil {
		l.fails = map[string][]time.Time{}
	}
	f := prune(l.fails[ip], now)
	if len(f) == 0 {
		delete(l.fails, ip)
	} else {
		l.fails[ip] = f
	}
	var wait time.Duration
	if len(f) >= maxFailsPerIP {
		wait = pairWindow - now.Sub(f[len(f)-maxFailsPerIP])
	}
	if len(l.all) >= maxFailsGlobal {
		wait = max(wait, pairWindow-now.Sub(l.all[len(l.all)-maxFailsGlobal]))
	}
	return wait
}

// fail records a wrong code from ip. It reports whether the global limit
// was reached by this attempt (the caller then changes the code).
func (l *limiter) fail(ip string, now time.Time) bool {
	if l.fails == nil {
		l.fails = map[string][]time.Time{}
	}
	l.fails[ip] = append(l.fails[ip], now)
	l.all = append(l.all, now)
	return len(prune(l.all, now)) == maxFailsGlobal
}

// remaining is how many wrong codes ip may still send in the window.
func (l *limiter) remaining(ip string, now time.Time) int {
	return max(0, maxFailsPerIP-len(prune(l.fails[ip], now)))
}
