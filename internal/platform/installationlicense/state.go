// Package installationlicense owns software installation authorization only.
// Customer subscriptions, RBAC and quotas remain independent required checks.
package installationlicense

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	c "github.com/naseri-coder/TELMIVO/internal/platform/installationlicense/licensecrypto"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

var ErrRestricted = errors.New("INSTALLATION_LICENSE_RESTRICTED")
var ErrState = errors.New("INSTALLATION_LICENSE_STATE_INVALID")
var ErrRollback = errors.New("INSTALLATION_LICENSE_ROLLBACK")

type Identity struct {
	ID         string
	PrivateKey []byte
}

func UUID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func (i Identity) PublicKey() ed25519.PublicKey {
	return ed25519.PrivateKey(i.PrivateKey).Public().(ed25519.PublicKey)
}

type HighWater struct {
	ConcurrencyGeneration                              int64  `json:",omitempty"`
	RecoveryGeneration                                 int64  `json:",omitempty"`
	ConcurrencyLeaseID                                 string `json:",omitempty"`
	Epoch, Revision, Generation, IssuedAt, TrustedTime int64
	LicenseID, ActivationID                            string
}
type State struct {
	BaseEnvelope []byte `json:",omitempty"`
	Envelope     []byte
	Restricted   string
	Pending      *Operation
}
type Operation struct {
	Kind, Key string
	Body      []byte
}
type Config struct {
	RequireConcurrency                       bool
	Directory, AuthorityID, Environment, URL string
	Roots                                    c.Roots
	// Clock is an injected validation clock; production forbids it.
	Clock func() time.Time
}
type Manager struct {
	runtimeSession string
	cfg            Config
	now            func() time.Time
}
type Decision struct {
	ConcurrencyGeneration, RecoveryGeneration int64
	RuntimeSessionID, ConcurrencyLeaseID      string
	ConcurrencyExpiresAt                      int64
	State, Reason                             string
	RefreshDue                                bool
	Epoch, Revision, Generation               int64
}

func Open(cfg Config) (*Manager, error) {
	if !filepath.IsAbs(cfg.Directory) || !c.ValidID(cfg.AuthorityID) || (cfg.Environment != "TEST" && cfg.Environment != "DEVELOPMENT" && cfg.Environment != "PRODUCTION") || len(cfg.Roots) == 0 {
		return nil, ErrState
	}
	// Never create private installation material inside a Git worktree.
	for p := cfg.Directory; p != "/"; p = filepath.Dir(p) {
		if _, e := os.Lstat(filepath.Join(p, ".git")); e == nil {
			return nil, ErrState
		} else if !os.IsNotExist(e) {
			return nil, ErrState
		}
	}
	roots := make(c.Roots, len(cfg.Roots))
	for id, root := range cfg.Roots {
		root.PublicKey = append(ed25519.PublicKey(nil), root.PublicKey...)
		roots[id] = root
	}
	cfg.Roots = roots
	// Reject symlink components before touching installation material.
	for p := cfg.Directory; p != "/"; p = filepath.Dir(p) {
		if f, e := os.Lstat(p); e == nil && f.Mode()&os.ModeSymlink != 0 {
			return nil, ErrState
		} else if e != nil && !os.IsNotExist(e) {
			return nil, ErrState
		}
	}
	if e := os.MkdirAll(cfg.Directory, 0700); e != nil {
		return nil, ErrState
	}
	f, e := os.Stat(cfg.Directory)
	if e != nil || !f.IsDir() || f.Mode().Perm()&0077 != 0 {
		return nil, ErrState
	}
	if st, ok := f.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Geteuid() {
		return nil, ErrState
	}
	if cfg.Clock != nil && cfg.Environment == "PRODUCTION" {
		return nil, ErrState
	}
	session, e := UUID()
	if e != nil {
		return nil, e
	}
	cfg.RequireConcurrency = cfg.RequireConcurrency || cfg.Environment == "PRODUCTION"
	m := &Manager{cfg: cfg, now: time.Now, runtimeSession: session}
	if cfg.Clock != nil {
		m.now = cfg.Clock
	}
	e = m.lock(func() error {
		var i Identity
		e := m.read("identity.json", &i)
		if os.IsNotExist(e) {
			for _, n := range []string{"state.json", "highwater.json"} {
				if _, e := os.Lstat(filepath.Join(cfg.Directory, n)); !os.IsNotExist(e) {
					return ErrState
				}
			}
			id, e := UUID()
			if e != nil {
				return e
			}
			_, key, e := ed25519.GenerateKey(rand.Reader)
			if e != nil {
				return e
			}
			i = Identity{id, key}
			return m.write("identity.json", i)
		}
		if e != nil || !c.ValidID(i.ID) || len(i.PrivateKey) != ed25519.PrivateKeySize {
			return ErrState
		}
		seedKey := ed25519.NewKeyFromSeed(i.PrivateKey[:32])
		if !bytes.Equal(seedKey, i.PrivateKey) {
			return ErrState
		}
		return nil
	})
	return m, e
}
func (m *Manager) lock(fn func() error) error {
	return m.lockMode(syscall.LOCK_EX, fn)
}

// lockMode preserves the same ownership/regular-file/hardlink checks for both
// blocking mutation/effect locks and nonblocking authorization probes.
func (m *Manager) lockMode(mode int, fn func() error) error {
	fd, e := syscall.Open(filepath.Join(m.cfg.Directory, "lock"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return ErrState
	}
	f := os.NewFile(uintptr(fd), "lock")
	defer f.Close()
	var s syscall.Stat_t
	if syscall.Fstat(fd, &s) != nil || s.Mode&syscall.S_IFMT != syscall.S_IFREG || s.Nlink != 1 || s.Mode&0077 != 0 || int(s.Uid) != os.Geteuid() {
		return ErrState
	}
	if e = syscall.Flock(fd, mode); e != nil {
		return ErrState
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	return fn()
}
func (m *Manager) read(name string, v any) error {
	fd, e := syscall.Open(filepath.Join(m.cfg.Directory, name), syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	var s syscall.Stat_t
	if syscall.Fstat(fd, &s) != nil || s.Mode&syscall.S_IFMT != syscall.S_IFREG || s.Nlink != 1 || s.Mode&0077 != 0 || int(s.Uid) != os.Geteuid() || s.Size > 262144 {
		return ErrState
	}
	b, e := io.ReadAll(io.LimitReader(f, 262145))
	if e != nil {
		return ErrState
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return ErrState
	}
	canonical, e := json.Marshal(v)
	if e != nil || !bytes.Equal(b, append(canonical, '\n')) {
		return ErrState
	}
	return nil
}
func (m *Manager) write(name string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return ErrState
	}
	b = append(b, '\n')
	f, e := os.CreateTemp(m.cfg.Directory, ".atomic-")
	if e != nil {
		return ErrState
	}
	n := f.Name()
	defer os.Remove(n)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(n, filepath.Join(m.cfg.Directory, name))
	}
	if e == nil {
		var d *os.File
		d, e = os.Open(m.cfg.Directory)
		if e == nil {
			e = d.Sync()
			d.Close()
		}
	}
	if e != nil {
		return ErrState
	}
	return nil
}
func (m *Manager) load() (Identity, State, HighWater, error) {
	var i Identity
	var s State
	var h HighWater
	if e := m.read("identity.json", &i); e != nil || len(i.PrivateKey) != 64 || !c.ValidID(i.ID) {
		return i, s, h, ErrState
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(i.PrivateKey[:32]), i.PrivateKey) {
		return i, s, h, ErrState
	}
	se := m.read("state.json", &s)
	he := m.read("highwater.json", &h)
	if os.IsNotExist(se) && os.IsNotExist(he) {
		return i, s, h, nil
	}
	if se != nil || he != nil {
		return i, s, h, ErrState
	}
	if len(s.Envelope) > 0 && (h.Epoch < 1 || h.Revision < 1 || h.Generation < 1 || h.IssuedAt < 1 || h.TrustedTime < 1 || !c.ValidID(h.LicenseID) || !c.ValidID(h.ActivationID)) {
		return i, s, h, ErrState
	}
	return i, s, h, nil
}
func (m *Manager) verify(i Identity, s State, h HighWater) (c.Result, error) {
	hash := sha256.Sum256(i.PublicKey())
	r, e := c.Verify(s.Envelope, m.cfg.Roots, c.Policy{Now: m.now().UTC(), ExpectedProduct: c.Product, ExpectedAudience: c.Audience, ExpectedAuthority: m.cfg.AuthorityID, ExpectedEnvironment: m.cfg.Environment, InstallationID: i.ID, InstallationKeyHash: hash[:], MinimumEpoch: h.Epoch, MinimumRevision: h.Revision, LastTrustedTime: time.UnixMicro(h.TrustedTime)})
	if e != nil {
		return r, e
	}
	x := r.Claims
	if x.Version == 2 && (x.ConcurrencyIssuedAt > m.now().UnixMicro()+c.ClockTolerance.Microseconds() || x.ConcurrencyGeneration < h.ConcurrencyGeneration || x.RecoveryGeneration < h.RecoveryGeneration || (x.ConcurrencyGeneration == h.ConcurrencyGeneration && h.ConcurrencyLeaseID != "" && x.ConcurrencyLeaseID != h.ConcurrencyLeaseID) || m.now().UnixMicro() > x.ConcurrencyExpiresAt) {
		return c.Result{}, ErrRollback
	}
	if x.ActivationGeneration < h.Generation || x.IssuedAt < h.IssuedAt || (h.LicenseID != "" && x.LicenseID != h.LicenseID) || (h.ActivationID != "" && x.ActivationGeneration == h.Generation && x.ActivationID != h.ActivationID) {
		return c.Result{}, ErrRollback
	}
	return r, nil
}
func (m *Manager) decisionLocked() (Decision, error) {
	i, s, h, e := m.load()
	if e != nil {
		return Decision{State: "RESTRICTED", Reason: "STATE_INVALID"}, e
	}
	// Retain time even when an already-installed lease is expired/restricted.
	// Otherwise rolling the clock back after observing expiry could revive it.
	now := m.now().UnixMicro()
	if len(s.Envelope) > 0 || s.Pending != nil || h.TrustedTime > 0 {
		if now < h.TrustedTime-c.ClockTolerance.Microseconds() {
			return Decision{State: "RESTRICTED", Reason: "CLOCK_ROLLBACK"}, ErrRollback
		}
		if now > h.TrustedTime {
			h.TrustedTime = now
			if e = m.write("highwater.json", h); e != nil {
				return Decision{State: "RESTRICTED", Reason: "PERSISTENCE_FAILED"}, e
			}
		}
	}
	if s.Restricted != "" {
		return Decision{State: "RESTRICTED", Reason: s.Restricted}, ErrRestricted
	}
	if len(s.Envelope) == 0 {
		return Decision{State: "RESTRICTED", Reason: "NOT_ACTIVATED"}, ErrRestricted
	}
	r, e := m.verify(i, s, h)
	if e != nil {
		return Decision{State: "RESTRICTED", Reason: "VERIFICATION_FAILED"}, e
	}
	x := r.Claims
	if (m.cfg.RequireConcurrency || h.ConcurrencyGeneration > 0) && x.Version != 2 {
		return Decision{State: "RESTRICTED", Reason: "CONCURRENCY_REFRESH_REQUIRED", RefreshDue: true}, ErrRestricted
	}
	// A concurrency token is bound to the process session that requested it.
	// A cloned or restarted runtime cannot perform protected effects using the
	// previous process's signed lease before obtaining its own fresh lease.
	// Validate after cryptographic verification, without trusting local claims.
	if x.Version == 2 && x.RuntimeSessionID != m.runtimeSession {
		return Decision{State: "RESTRICTED", Reason: "CONCURRENCY_REFRESH_REQUIRED", RefreshDue: true}, ErrRestricted
	}
	allowed := false
	for _, cap := range x.Capabilities {
		if cap == "product.use" {
			allowed = true
		}
	}
	if !allowed {
		return Decision{State: "RESTRICTED", Reason: "CAPABILITY_MISSING"}, ErrRestricted
	}
	return Decision{ConcurrencyGeneration: x.ConcurrencyGeneration, RecoveryGeneration: x.RecoveryGeneration, RuntimeSessionID: x.RuntimeSessionID, ConcurrencyLeaseID: x.ConcurrencyLeaseID, ConcurrencyExpiresAt: x.ConcurrencyExpiresAt, State: r.State, RefreshDue: now >= x.RefreshAfter, Epoch: x.AuthorityEpoch, Revision: x.LicenseRevision, Generation: x.ActivationGeneration}, nil
}
// Decision always performs real signed-state verification when the file lock
// is available, but fails closed instead of waiting on a lock held by a nested
// Manager.Effect callback. It never issues an authorization from cached data.
func (m *Manager) Decision() Decision {
	return m.decisionNonblocking()
}

// decisionNonblocking uses the genuine signed-V2 decisionLocked validation,
// but fails closed if another effect already holds the installation file lock.
// Never wait on a second independently opened flock from inside that effect.
func (m *Manager) decisionNonblocking() Decision {
	d := Decision{State: "RESTRICTED", Reason: "STATE_INVALID"}
	acquired := false
	e := m.lockMode(syscall.LOCK_EX|syscall.LOCK_NB, func() error {
		acquired = true
		var err error
		d, err = m.decisionLocked()
		return err
	})
	if e != nil && !acquired {
		return Decision{State: "RESTRICTED", Reason: "STATE_LOCK_UNAVAILABLE"}
	}
	// Retain the real reason (expiry, session replay, cancellation of lease)
	// if a locked decision was evaluated but returned a restriction error.
	return d
}

// Effect serializes the final local authoritative recheck with state updates
// across all roles sharing this installation directory. Existing domain outcome
// and idempotency fences remain responsible for durable external outcomes.
func (m *Manager) Effect(ctx context.Context, fn func() error) error {
	return m.lock(func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, e := m.decisionLocked(); e != nil {
			return ErrRestricted
		}
		return fn()
	})
}
func (m *Manager) Accept(envelope []byte) error {
	return m.lock(func() error { return m.acceptLocked(envelope) })
}
func (m *Manager) acceptLocked(envelope []byte) error {
	i, s, h, e := m.load()
	if e != nil {
		return e
	}
	candidate := s
	candidate.Envelope = envelope
	candidate.Restricted = ""
	r, e := m.verify(i, candidate, h)
	if e != nil {
		return e
	}
	x := r.Claims

	if x.Version == 2 && s.Pending != nil && s.Pending.Kind == "concurrency" {
		request, e := c.ParseConcurrencyRequest(s.Pending.Body)
		if e != nil || x.ConcurrentPolicy != request.ProtocolID || x.LicenseID != request.LicenseID || x.InstallationID != request.InstallationID || x.ActivationID != request.ActivationID || x.AuthorityEpoch != request.AuthorityEpoch || x.LicenseRevision != request.LicenseRevision || x.ActivationGeneration != request.ActivationGeneration || x.ConcurrencyGeneration != request.RequestedGeneration+1 || x.RecoveryGeneration != request.RecoveryGeneration || x.RuntimeSessionID != request.RuntimeSessionID {
			return ErrRollback
		}
	}
	if x.Version == 2 && x.ConcurrencyGeneration <= h.ConcurrencyGeneration && h.ConcurrencyGeneration > 0 {
		repair := false
		if s.Pending != nil && s.Pending.Kind == "concurrency" {
			pending, e := c.ParseConcurrencyRequest(s.Pending.Body)
			repair = e == nil && pending.RequestedGeneration+1 == x.ConcurrencyGeneration && pending.RuntimeSessionID == x.RuntimeSessionID
		}
		if !repair {
			return ErrRollback
		}
	}
	if s.Restricted != "" && x.AuthorityEpoch <= h.Epoch && x.LicenseRevision <= h.Revision && x.ActivationGeneration <= h.Generation && x.IssuedAt <= h.IssuedAt && x.ConcurrencyGeneration <= h.ConcurrencyGeneration {
		return ErrRollback
	}
	if x.Version == 2 {
		h.ConcurrencyGeneration = x.ConcurrencyGeneration
		h.RecoveryGeneration = x.RecoveryGeneration
		h.ConcurrencyLeaseID = x.ConcurrencyLeaseID
	}
	if x.Version == 1 && (m.cfg.RequireConcurrency || h.ConcurrencyGeneration > 0) {
		candidate = s
		candidate.BaseEnvelope = append([]byte(nil), envelope...)
		candidate.Pending = nil
	}
	h.Epoch = x.AuthorityEpoch
	h.Revision = x.LicenseRevision
	h.Generation = x.ActivationGeneration
	h.IssuedAt = x.IssuedAt
	h.LicenseID = x.LicenseID
	h.ActivationID = x.ActivationID
	if n := m.now().UnixMicro(); n > h.TrustedTime {
		h.TrustedTime = n
	}
	// A torn two-file commit is restrictive: high-water is always committed first.
	if e = m.write("highwater.json", h); e != nil {
		return e
	}
	candidate.Pending = nil
	return m.write("state.json", candidate)
}

type contextKey struct{}

func WithManager(ctx context.Context, m *Manager) context.Context {
	return context.WithValue(ctx, contextKey{}, m)
}
func FromContext(ctx context.Context) *Manager { m, _ := ctx.Value(contextKey{}).(*Manager); return m }
// Effect is the context-bound compatibility entrypoint for external effects.
// Do not bypass the process-wide fail-closed Runtime installation lifecycle by
// calling a valid Context Manager directly.
func Effect(ctx context.Context, fn func() error) error {
	return RuntimeEffect(ctx, fn)
}

func (Identity) Format(w fmt.State, _ rune) { io.WriteString(w, "<INSTALLATION_IDENTITY REDACTED>") }
func (State) Format(w fmt.State, _ rune)    { io.WriteString(w, "<INSTALLATION_LICENSE_STATE REDACTED>") }
func (Operation) Format(w fmt.State, _ rune) {
	io.WriteString(w, "<INSTALLATION_LICENSE_OPERATION REDACTED>")
}
