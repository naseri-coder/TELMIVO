package httpapi

import (
	"net/http"
	"net/netip"
	"strings"
)

func clientIP(r *http.Request, trusted []netip.Prefix) (string, bool) {
	peer, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return "", false
	}
	ip := peer.Addr().Unmap()
	isTrusted := func(a netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	if !isTrusted(ip) {
		return ip.String(), true
	}
	// Walk from the trusted immediate peer toward the first untrusted hop.
	chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(chain) > 16 {
		return "", false
	}
	for i := len(chain) - 1; i >= 0 && isTrusted(ip); i-- {
		next, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if err != nil {
			return "", false
		}
		ip = next.Unmap()
	}
	return ip.String(), true
}
