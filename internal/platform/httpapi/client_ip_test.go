package httpapi

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestTrustedProxyIdentity(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("192.0.2.1/32")}
	for _, tc := range []struct {
		peer, forward, want string
		valid               bool
	}{
		{"192.0.2.1:8080", "198.51.100.1", "198.51.100.1", true},
		{"192.0.2.1:8080", "198.51.100.2", "198.51.100.2", true},
		{"198.51.100.3:8080", "198.51.100.99", "198.51.100.3", true},
		{"192.0.2.1:8080", "spoof, 198.51.100.4", "198.51.100.4", true},
		{"192.0.2.1:8080", "garbage", "", false},
		{"garbage", "198.51.100.1", "", false}} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-Forwarded-For", tc.forward)
		got, ok := clientIP(r, trusted)
		if got != tc.want || ok != tc.valid {
			t.Fatalf("%+v got=%q valid=%v", tc, got, ok)
		}
	}
}
