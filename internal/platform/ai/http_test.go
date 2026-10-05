package ai

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
)

func TestProviderEndpointBoundary(t *testing.T) {
	if !EndpointValid("https://approved.example/v1/chat/completions") {
		t.Fatal("valid")
	}
	for _, u := range []string{"http://approved.example/v1/chat/completions", "https://127.0.0.1/v1/chat/completions", "https://[::1]/v1/chat/completions", "https://localhost/v1/chat/completions", "https://a.local/v1/chat/completions", "https://a:123/v1/chat/completions", "https://user:secret@a/v1/chat/completions", "https://a/v1/chat/completions?url=http://localhost", "https://a/v1/chat/completions#x"} {
		if EndpointValid(u) {
			t.Fatal(u)
		}
	}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "::1", "fc00::1", "2001:db8::1"} {
		if public(netip.MustParseAddr(ip)) {
			t.Fatal(ip)
		}
	}
}
func TestProviderMetadataNeverReturnsSecrets(t *testing.T) {
	m := Model{ID: "approved", Endpoint: "SECRET_ENDPOINT"}
	m.Secret.KeyID = "SECRET_KEY_ID"
	m.Secret.Ciphertext = []byte("SECRET_CIPHERTEXT")
	b, _ := json.Marshal(m)
	if strings.Contains(string(b), "SECRET") {
		t.Fatal(string(b))
	}
	if Code(&Failure{Code: "PROVIDER_TIMEOUT"}) != "PROVIDER_TIMEOUT" {
		t.Fatal("code")
	}
}
