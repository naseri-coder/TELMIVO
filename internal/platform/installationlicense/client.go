package installationlicense

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	c "github.com/naseri-coder/TELMIVO/internal/platform/installationlicense/licensecrypto"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"syscall"
	"time"
)

var ErrUnavailable = errors.New("LICENSE_AUTHORITY_UNAVAILABLE")
var ErrPending = errors.New("LICENSE_ISSUANCE_PENDING")

type Client struct {
	manager *Manager
	http    *http.Client
	base    string
}

func NewClient(m *Manager) (*Client, error) {
	u, e := url.Parse(m.cfg.URL)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, ErrState
	}
	return &Client{m, &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, u.String()}, nil
}

type authorityStatus struct {
	Edition          string     `json:"edition"`
	Capabilities     []string   `json:"capabilities"`
	CommercialExpiry time.Time  `json:"commercial_expires_at"`
	ServerTime       time.Time  `json:"server_time"`
	RefreshAfter     *time.Time `json:"refresh_after"`
	OnlineUntil      *time.Time `json:"online_until"`
	OfflineUntil     *time.Time `json:"offline_until"`

	LicenseID         string `json:"license_id"`
	ActivationID      string `json:"activation_id"`
	InstallationID    string `json:"installation_id"`
	LicenseState      string `json:"license_state"`
	ActivationState   string `json:"activation_state"`
	InstallationState string `json:"installation_state"`
	Revision          int64  `json:"license_revision"`
	Generation        int64  `json:"activation_generation"`
	Epoch             int64  `json:"authority_epoch"`
	RefreshEligible   bool   `json:"refresh_eligible"`
	RestrictedReason  string `json:"restricted_reason"`
}
type reply struct {
	RequestID string `json:"request_id"`
	Error     *struct {
		Code      string `json:"code"`
		Retryable bool   `json:"retryable"`
	} `json:"error"`
	OperationID  string           `json:"operation_id"`
	ActivationID string           `json:"activation_id"`
	IssuanceID   string           `json:"issuance_id"`
	Envelope     string           `json:"license_envelope"`
	State        string           `json:"state"`
	Status       *authorityStatus `json:"status"`
}

func proof(i Identity, method, path, query, timestamp, nonce, key string, body []byte) []byte {
	b := []byte("TELMIVO_INSTALLATION_REQUEST_V1\x00")
	pk := sha256.Sum256(i.PublicKey())
	bh := sha256.Sum256(body)
	for _, f := range [][]byte{[]byte(method), []byte(path), []byte(query), []byte(timestamp), []byte(nonce), []byte(key), []byte(i.ID), pk[:], bh[:]} {
		b = binary.BigEndian.AppendUint32(b, uint32(len(f)))
		b = append(b, f...)
	}
	return b
}
func (cl *Client) request(ctx context.Context, i Identity, op Operation, query string) (reply, int, error) {
	var out reply
	path := "/v1/licenses/" + op.Kind
	method := "POST"
	if op.Kind == "status" {
		method = "GET"
	}
	nonce := make([]byte, 32)
	if _, e := rand.Read(nonce); e != nil {
		return out, 0, ErrUnavailable
	}
	ns := base64.RawURLEncoding.EncodeToString(nonce)
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	target := cl.base + path
	if query != "" {
		target += "?" + query
	}
	req, e := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(op.Body))
	if e != nil {
		return out, 0, ErrState
	}
	req.Header.Set("X-Telmivo-Installation-ID", i.ID)
	req.Header.Set("X-Telmivo-Timestamp", ts)
	req.Header.Set("X-Telmivo-Nonce", ns)
	req.Header.Set("X-Telmivo-Signature", base64.RawURLEncoding.EncodeToString(ed25519.Sign(i.PrivateKey, proof(i, method, path, query, ts, ns, op.Key, op.Body))))
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", op.Key)
	}
	r, e := cl.http.Do(req)
	if e != nil {
		return out, 0, ErrUnavailable
	}
	defer r.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(r.Body, 131073))
	if e != nil || len(raw) > 131072 || StrictJSON(raw, &out) != nil {
		return out, r.StatusCode, ErrUnavailable
	}
	return out, r.StatusCode, nil
}
func terminal(code string) bool {
	switch code {
	case "LICENSE_NOT_ACTIVE", "LICENSE_SUSPENDED", "LICENSE_EXPIRED", "LICENSE_REVOKED", "INSTALLATION_DISABLED", "INSTALLATION_RETIRED", "INSTALLATION_SUPERSEDED", "ACTIVATION_NOT_ACTIVE", "AUTHORITY_RECOVERY_HOLD", "STALE_CONCURRENT_INSTALLATION_STATE", "RECOVERY_GENERATION_MISMATCH":
		return true
	}
	return false
}
func (cl *Client) mutate(ctx context.Context, kind string, body []byte) error {
	m := cl.manager
	return m.lock(func() error {
		i, s, h, e := m.load()
		if e != nil {
			return e
		}
		if s.Pending == nil {
			key, e := UUID()
			if e != nil {
				return e
			}
			if kind == "concurrency" {
				r, e := c.ParseConcurrencyRequest(body)
				if e != nil {
					return e
				}
				key = r.RequestID
			}
			s.Pending = &Operation{kind, key, body}
			if e = m.write("highwater.json", h); e != nil {
				return e
			}
			if e = m.write("state.json", s); e != nil {
				return e
			}
		} else if s.Pending.Kind != kind || !bytes.Equal(s.Pending.Body, body) {
			return errors.New("LICENSE_OPERATION_PENDING")
		}
		out, status, e := cl.request(ctx, i, *s.Pending, "")
		if e != nil {
			return e
		}
		if out.Error != nil {
			if terminal(out.Error.Code) {
				s.Restricted = out.Error.Code
				if out.Error.Code == "STALE_CONCURRENT_INSTALLATION_STATE" {
					s.Restricted = "CONCURRENT_INSTALLATION_CONFLICT"
				}
				s.Pending = nil
				if e = m.write("state.json", s); e != nil {
					return e
				}
				return ErrRestricted
			}
			if status == 202 {
				return ErrPending
			}
			if !out.Error.Retryable {
				s.Pending = nil
				if e = m.write("state.json", s); e != nil {
					return e
				}
			}
			return ErrUnavailable
		}
		if status != 200 && status != 201 {
			return ErrUnavailable
		}
		if kind == "deactivate" {
			if out.State != "COMPLETED" {
				return ErrUnavailable
			}
			s.Restricted = "DEACTIVATED"
			s.Pending = nil
			return m.write("state.json", s)
		}
		raw, e := base64.RawURLEncoding.Strict().DecodeString(out.Envelope)
		if e != nil || base64.RawURLEncoding.EncodeToString(raw) != out.Envelope || len(raw) > c.MaximumArtifact {
			return c.Malformed
		}
		return m.acceptLocked(raw)
	})
}
func (cl *Client) Activate(ctx context.Context, code, license string, epoch int64) error {
	var i Identity
	if e := cl.manager.lock(func() error { return cl.manager.read("identity.json", &i) }); e != nil {
		return e
	}
	if !c.ValidID(license) || epoch < 1 || len(code) == 0 || len(code) > 512 {
		return ErrState
	}
	body, _ := json.Marshal(struct {
		Code    string `json:"activation_code"`
		ID      string `json:"installation_id"`
		Key     string `json:"installation_public_key"`
		License string `json:"license_id"`
		Epoch   int64  `json:"authority_epoch"`
	}{code, i.ID, base64.RawURLEncoding.EncodeToString(i.PublicKey()), license, epoch})
	if e := cl.mutate(ctx, "activate", body); e != nil {
		return e
	}
	if cl.manager.cfg.RequireConcurrency {
		return cl.Concurrency(ctx)
	}
	return nil
}
func (cl *Client) registered(ctx context.Context, kind string) error {
	var body []byte
	e := cl.manager.lock(func() error {
		_, s, h, e := cl.manager.load()
		if e != nil {
			return e
		}
		if s.Pending != nil && s.Pending.Kind == kind {
			body = s.Pending.Body
			return nil
		}
		if !c.ValidID(h.ActivationID) || h.Epoch < 1 {
			return ErrState
		}
		body, _ = json.Marshal(struct {
			ID    string `json:"activation_id"`
			Epoch int64  `json:"authority_epoch"`
		}{h.ActivationID, h.Epoch})
		return nil
	})
	if e != nil {
		return e
	}
	return cl.mutate(ctx, kind, body)
}
func (cl *Client) Refresh(ctx context.Context) error {
	if !cl.manager.cfg.RequireConcurrency {
		return cl.registered(ctx, "refresh")
	}
	var pending bool
	if e := cl.manager.lock(func() error {
		_, s, _, e := cl.manager.load()
		pending = s.Pending != nil && s.Pending.Kind == "concurrency"
		return e
	}); e != nil {
		return e
	}
	if pending {
		return cl.Concurrency(ctx)
	}
	if e := cl.registered(ctx, "refresh"); e != nil {
		return e
	}
	return cl.Concurrency(ctx)
}
func (cl *Client) Deactivate(ctx context.Context) error { return cl.registered(ctx, "deactivate") }
func (cl *Client) Status(ctx context.Context) error {
	m := cl.manager
	return m.lock(func() error {
		i, s, h, e := m.load()
		if e != nil {
			return e
		}
		if !c.ValidID(h.ActivationID) {
			return ErrState
		}
		out, status, e := cl.request(ctx, i, Operation{Kind: "status"}, "activation_id="+h.ActivationID)
		if e != nil {
			return e
		}
		if out.Error != nil && terminal(out.Error.Code) {
			s.Restricted = out.Error.Code
			return m.write("state.json", s)
		}
		if status != 200 || out.Status == nil {
			return ErrUnavailable
		}
		x := out.Status
		if x.InstallationID != i.ID || x.LicenseID != h.LicenseID || x.ActivationID != h.ActivationID || x.Epoch < h.Epoch || x.Revision < h.Revision || x.Generation < h.Generation {
			return ErrRollback
		}
		// Authenticated HTTPS status can revoke cached access or raise floors, but
		// never create access or clear restriction. Only a verified lease can do so.
		h.Epoch = x.Epoch
		h.Revision = x.Revision
		h.Generation = x.Generation
		if e = m.write("highwater.json", h); e != nil {
			return e
		}
		if !x.RefreshEligible || x.RestrictedReason != "" || x.LicenseState != "ACTIVE" || x.ActivationState != "ACTIVE" || x.InstallationState != "REGISTERED" {
			s.Restricted = "AUTHORITY_RESTRICTED"
			return m.write("state.json", s)
		}
		return nil
	})
}
func (cl *Client) Poll(ctx context.Context) {
	var leader *os.File
	for {
		acquired, e := cl.manager.acquireRefreshLeader()
		if e == nil {
			leader = acquired
			break
		}
		if !errors.Is(e, ErrPending) {
			return
		}
		timer := time.NewTimer(15 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	defer leader.Close()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		d := cl.manager.Decision()
		if d.RefreshDue || d.State == "RESTRICTED" {
			_ = cl.Refresh(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// OpenEnvironment takes only explicit public trust. It never fetches roots from
// the Authority or permits a production test/bypass switch.
func OpenEnvironment() (*Manager, *Client, error) {
	path := os.Getenv("TELMIVO_LICENSE_TRUST_FILE")
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, nil, ErrState
	}
	f := os.NewFile(uintptr(fd), "public-trust")
	defer f.Close()
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Mode&0022 != 0 || (int(st.Uid) != os.Geteuid() && st.Uid != 0) {
		return nil, nil, ErrState
	}
	b, e := io.ReadAll(io.LimitReader(f, 65537))
	if e != nil || len(b) > 65536 {
		return nil, nil, ErrState
	}
	roots, e := ParseRoots(b, os.Getenv("TELMIVO_ENV") == "production" || os.Getenv("TELMIVO_LICENSE_ENVIRONMENT") == "PRODUCTION")
	if e != nil {
		return nil, nil, e
	}
	cfg := Config{RequireConcurrency: true, Directory: os.Getenv("TELMIVO_LICENSE_STATE_DIR"), AuthorityID: os.Getenv("TELMIVO_LICENSE_AUTHORITY_ID"), Environment: os.Getenv("TELMIVO_LICENSE_ENVIRONMENT"), URL: os.Getenv("TELMIVO_LICENSE_AUTHORITY_URL"), Roots: roots}
	if os.Getenv("TELMIVO_ENV") == "production" && cfg.Environment != "PRODUCTION" {
		return nil, nil, ErrState
	}
	m, e := Open(cfg)
	if e != nil {
		return nil, nil, e
	}
	cl, e := NewClient(m)
	return m, cl, e
}
