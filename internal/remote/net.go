package remote

import (
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strings"
)

// cgnat is 100.64.0.0/10 (carrier-grade NAT; also Tailscale's range).
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// localPeer reports whether a client address may use the remote at all: the
// remote is for the local network, so loopback, private (RFC 1918, fc00::/7),
// link-local and CGNAT addresses only. A machine with a public address does
// not serve the internet.
func localPeer(remoteAddr string) bool {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || cgnat.Contains(a)
}

// localHost reports whether a request's Host header names this machine the
// way a LAN client does: an IP literal, localhost or a .local (Bonjour) name.
// Anything else is a DNS-rebinding attempt (an attacker's domain that
// resolves to the LAN address, which would make a hostile page same-origin).
func localHost(host string) bool {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	h = strings.TrimSuffix(strings.Trim(h, "[]"), ".")
	if h == "" {
		return false
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return true
	}
	h = strings.ToLower(h)
	return h == "localhost" || strings.HasSuffix(h, ".local")
}

// sameOrigin reports whether an Origin header names the host the request was
// sent to (scheme http or https, same host and port).
func sameOrigin(origin, host string) bool {
	if origin == "" || origin == "null" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, host)
}

// LANAddresses lists this machine's IPv4 addresses on the local network,
// the most likely one first: private addresses on up, non-loopback
// interfaces, Wi-Fi / Ethernet (en*, eth*, wl*) before virtual ones.
func LANAddresses() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	type cand struct {
		ip   string
		rank int
	}
	var cs []cand
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, ad := range addrs {
			pfx, err := netip.ParsePrefix(ad.String())
			if err != nil {
				continue
			}
			a := pfx.Addr().Unmap()
			if !a.Is4() || a.IsLoopback() || a.IsLinkLocalUnicast() {
				continue
			}
			rank := 3
			switch {
			case a.IsPrivate() && physicalInterface(ifc.Name):
				rank = 0
			case a.IsPrivate():
				rank = 1
			case cgnat.Contains(a):
				rank = 2
			}
			cs = append(cs, cand{a.String(), rank})
		}
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].rank < cs[j].rank })
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		if c.rank < 3 {
			out = append(out, c.ip)
		}
	}
	return out
}

// physicalInterface guesses whether an interface is Wi-Fi or Ethernet
// (rather than a VPN, bridge or container network).
func physicalInterface(name string) bool {
	for _, p := range []string{"en", "eth", "wl", "Wi-Fi", "Ethernet"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
