//go:build lic_a9_acceptance

package installationlicense

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"io"
	"net/http"
	"time"
)

// Explicit test tag only; there is no corresponding production environment flag.
func (c *Client) SetAcceptanceTransport(t http.RoundTripper) { c.http.Transport = t }
func (m *Manager) SetAcceptanceClock(now time.Time)          { m.now = func() time.Time { return now } }

func (m *Manager) AcceptanceSession() string     { return m.runtimeSession }
func (m *Manager) SetAcceptanceSession(s string) { m.runtimeSession = s }

// SignAcceptanceBody uses the actual permanent installation identity and LIC-A5
// canonical proof; key bytes never leave the adapter.
func (c *Client) SignAcceptanceBody(req *http.Request, body []byte) error {
	var identity Identity
	if e := c.manager.read("identity.json", &identity); e != nil {
		return e
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	signature := ed25519.Sign(identity.PrivateKey, proof(identity, req.Method, req.URL.Path, req.URL.RawQuery, req.Header.Get("X-Telmivo-Timestamp"), req.Header.Get("X-Telmivo-Nonce"), req.Header.Get("Idempotency-Key"), body))
	req.Header.Set("X-Telmivo-Signature", base64.RawURLEncoding.EncodeToString(signature))
	return nil
}
