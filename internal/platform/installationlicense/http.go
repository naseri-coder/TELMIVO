package installationlicense

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Middleware keeps liveness and read-only diagnosis available while suppressing
// commercial mutations. Webhook acknowledgments never execute protected work.
func Middleware(m *Manager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(RequireContext(r.Context()))
		d := Decision{State: "RESTRICTED", Reason: "CONFIGURATION_INVALID"}
		if m != nil {
			d = m.Decision()
			r = r.WithContext(WithManager(r.Context(), m))
		}
		if r.Method == "GET" && r.URL.Path == "/health/license" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			json.NewEncoder(w).Encode(d)
			return
		}
		if d.State == "RESTRICTED" {
			if r.Method == "GET" || r.Method == "HEAD" {
				r = r.WithContext(ReadOnly(r.Context()))
			}
			if r.Method == "GET" && (r.URL.Path == "/health/live" || r.URL.Path == "/health/ready") {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method == "POST" && (r.URL.Path == "/webhook" || strings.HasPrefix(r.URL.Path, "/api/v1/customer-bot-updates/")) {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method != "GET" && r.Method != "HEAD" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(403)
				json.NewEncoder(w).Encode(map[string]string{"code": "INSTALLATION_LICENSE_RESTRICTED"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (m *Manager) PublicIdentity() (string, []byte, error) {
	var i Identity
	e := m.lock(func() error { return m.read("identity.json", &i) })
	if e != nil || len(i.PrivateKey) != 64 {
		return "", nil, ErrState
	}
	return i.ID, append([]byte(nil), i.PublicKey()...), nil
}
