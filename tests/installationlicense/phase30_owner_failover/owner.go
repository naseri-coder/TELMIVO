// Package ownerproof is an OFFLINE, NONPRODUCTION demonstration of fail-closed
// single-effect-owner handoff. It is NOT a License Authority, a production
// supervisor, a real Manager.Effect, or a distributed fencing solution.
package ownerproof

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"time"

	effectgate "telmivo.local/lic-a9-phase28-effect-proof"
	ledger "telmivo.local/lic-a9-phase29-outcome-ledger"
)

const Protocol = "TELMIVO_LIC_A9_PHASE30_TEST_GRANT_V1"
const signDomain = "TELMIVO_PHASE30_TEST_GRANT\x00"

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,96}$`)

var (
	ErrDenied           = errors.New("PHASE30_TEST_ONLY_DENIED")
	ErrStale            = errors.New("PHASE30_TEST_ONLY_STALE_OWNER")
	ErrPending          = errors.New("PHASE30_TEST_ONLY_OUTCOME_RECONCILIATION_REQUIRED")
	ErrCorrupt          = errors.New("PHASE30_TEST_ONLY_PERSISTENCE_CORRUPT")
	ErrFenceUnsupported = errors.New("PHASE30_TEST_ONLY_PROVIDER_FENCING_UNAVAILABLE")
)

type Grant struct {
	Protocol      string `json:"protocol"`
	Installation  string `json:"installation"`
	Owner         string `json:"owner"`
	Session       string `json:"session"`
	PreviousEpoch uint64 `json:"previous_epoch"`
	Epoch         uint64 `json:"epoch"`
	NotBeforeUnix int64  `json:"not_before_unix"`
	ExpiresUnix   int64  `json:"expires_unix"`
	Signature     string `json:"signature"`
}

type signedGrant struct {
	Protocol      string `json:"protocol"`
	Installation  string `json:"installation"`
	Owner         string `json:"owner"`
	Session       string `json:"session"`
	PreviousEpoch uint64 `json:"previous_epoch"`
	Epoch         uint64 `json:"epoch"`
	NotBeforeUnix int64  `json:"not_before_unix"`
	ExpiresUnix   int64  `json:"expires_unix"`
}

func (g Grant) payload() []byte {
	fields := signedGrant{g.Protocol, g.Installation, g.Owner, g.Session, g.PreviousEpoch, g.Epoch, g.NotBeforeUnix, g.ExpiresUnix}
	b, _ := json.Marshal(fields)
	return append([]byte(signDomain), b...)
}

// SignTestGrant is deliberately exported ONLY within this standalone test module.
// It uses synthetic test-generated keys; no real issuer can call this function.
func SignTestGrant(private ed25519.PrivateKey, g Grant) Grant {
	g.Signature = hex.EncodeToString(ed25519.Sign(private, g.payload()))
	return g
}

type state struct {
	Version      string            `json:"version"`
	Installation string            `json:"installation"`
	Epoch        uint64            `json:"epoch"`
	Owner        string            `json:"owner"`
	Session      string            `json:"session"`
	ExpiresUnix  int64             `json:"expires_unix"`
	Pending      map[string]uint64 `json:"pending"`
}

const stateVersion = "PHASE30_TEST_SUPERVISOR_STATE_V1"

func freshState(install string) state {
	return state{Version: stateVersion, Installation: install, Pending: make(map[string]uint64)}
}

// Fencer is an external sink that atomically records/rejects owner epochs.
// It is modeled in tests only; actual providers may have no such capability.
type Fencer interface {
	Fence(context.Context, uint64) error
}

type Supervisor struct {
	dir          *os.File
	rootPath     string
	key          ed25519.PublicKey
	installation string
	mu           sync.Mutex
}

func Open(path, installation string, key ed25519.PublicKey) (*Supervisor, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !validID.MatchString(installation) || len(key) != ed25519.PublicKeySize {
		return nil, ErrDenied
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrDenied
	}
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFDIR || st.Mode&0077 != 0 || int(st.Uid) != os.Geteuid() {
		syscall.Close(fd)
		return nil, ErrDenied
	}
	s := &Supervisor{dir: os.NewFile(uintptr(fd), "supervisor-root"), rootPath: path, key: append(ed25519.PublicKey(nil), key...), installation: installation}
	// Bootstrap marker is exclusive: preexisting owner lock with missing state
	// fails closed instead of silently resetting the epoch to zero.
	newFD, e := syscall.Openat(fd, "_supervisor.lock", syscall.O_CREAT|syscall.O_EXCL|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	created := e == nil
	if errors.Is(e, syscall.EEXIST) {
		newFD, e = syscall.Openat(fd, "_supervisor.lock", syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	}
	if e != nil {
		s.Close()
		return nil, ErrCorrupt
	}
	syscall.Close(newFD)
	if created {
		// New installation only: initialization is explicit and fsynced.
		e = s.bootstrap()
	} else {
		e = s.withLock(func(st *state) error { return nil })
	}
	if e != nil {
		s.Close()
		return nil, e
	}
	return s, nil
}
func (s *Supervisor) bootstrap() error {
	return s.lockFile(func() error {
		_, err := s.readState()
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.ENOENT) {
			return ErrCorrupt
		}
		return s.saveState(freshState(s.installation))
	})
}
func (s *Supervisor) Close() error { return s.dir.Close() }
func (s *Supervisor) lockFile(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fd, e := syscall.Openat(int(s.dir.Fd()), "_supervisor.lock", syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return ErrCorrupt
	}
	defer syscall.Close(fd)
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Nlink != 1 || st.Mode&0077 != 0 || int(st.Uid) != os.Geteuid() {
		return ErrCorrupt
	}
	if e = syscall.Flock(fd, syscall.LOCK_EX); e != nil {
		return ErrCorrupt
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	return fn()
}
func (s *Supervisor) withLock(fn func(*state) error) error {
	return s.lockFile(func() error {
		st, e := s.readState()
		if e != nil {
			return ErrCorrupt
		}
		return fn(&st)
	})
}
func (s *Supervisor) readState() (state, error) {
	var v state
	fd, e := syscall.Openat(int(s.dir.Fd()), "current.json", syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return v, e
	}
	f := os.NewFile(uintptr(fd), "current-state")
	defer f.Close()
	var si syscall.Stat_t
	if syscall.Fstat(fd, &si) != nil || si.Mode&syscall.S_IFMT != syscall.S_IFREG || si.Nlink != 1 || si.Mode&0077 != 0 || int(si.Uid) != os.Geteuid() || si.Size > 16384 {
		return v, ErrCorrupt
	}
	b, e := io.ReadAll(io.LimitReader(f, 16385))
	if e != nil || len(b) > 16384 || json.Unmarshal(b, &v) != nil {
		return v, ErrCorrupt
	}
	canonical, _ := json.Marshal(v)
	if !bytes.Equal(b, append(canonical, '\n')) || v.Version != stateVersion || v.Installation != s.installation || v.Pending == nil {
		return v, ErrCorrupt
	}
	if v.Epoch == 0 && (v.Owner != "" || v.Session != "" || v.ExpiresUnix != 0) {
		return v, ErrCorrupt
	}
	if v.Epoch > 0 && (!validID.MatchString(v.Owner) || !validID.MatchString(v.Session) || v.ExpiresUnix == 0) {
		return v, ErrCorrupt
	}
	for id, epoch := range v.Pending {
		if !validID.MatchString(id) || epoch == 0 || epoch > v.Epoch {
			return v, ErrCorrupt
		}
	}
	return v, nil
}
func (s *Supervisor) saveState(v state) error {
	b, e := json.Marshal(v)
	if e != nil || len(b) > 16000 {
		return ErrCorrupt
	}
	b = append(b, '\n')
	f, e := os.CreateTemp(s.rootPath, ".atomic-")
	if e != nil {
		return ErrCorrupt
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return ErrCorrupt
	}
	if e = os.Rename(name, filepath.Join(s.rootPath, "current.json")); e != nil {
		return ErrCorrupt
	}
	if e = s.dir.Sync(); e != nil {
		return ErrCorrupt
	}
	return nil
}
func (s *Supervisor) validGrant(g Grant) bool {
	if g.Protocol != Protocol || g.Installation != s.installation || !validID.MatchString(g.Owner) || !validID.MatchString(g.Session) || g.Epoch == 0 || g.Epoch != g.PreviousEpoch+1 {
		return false
	}
	now := time.Now().Unix()
	if g.NotBeforeUnix > now || g.ExpiresUnix <= now || g.ExpiresUnix-g.NotBeforeUnix > 300 {
		return false
	}
	raw, e := hex.DecodeString(g.Signature)
	return e == nil && len(raw) == ed25519.SignatureSize && ed25519.Verify(s.key, g.payload(), raw)
}

// Acquire accepts only a new, test-signed mock grant with strict monotonic
// epoch and a durable reconciled journal. Provider Fence must succeed BEFORE
// local epoch commit; a provider-fence/commit crash may cause safe unavailability.
func (s *Supervisor) Acquire(ctx context.Context, g Grant, fencer Fencer) error {
	if fencer == nil || !s.validGrant(g) {
		return ErrDenied
	}
	return s.withLock(func(st *state) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(st.Pending) > 0 {
			return ErrPending
		}
		if g.PreviousEpoch != st.Epoch || g.Epoch != st.Epoch+1 {
			return ErrStale
		}
		if g.Session == st.Session {
			return ErrStale
		}
		if err := fencer.Fence(ctx, g.Epoch); err != nil {
			return ErrFenceUnsupported
		}
		st.Epoch, st.Owner, st.Session, st.ExpiresUnix = g.Epoch, g.Owner, g.Session, g.ExpiresUnix
		return s.saveState(*st)
	})
}
func (s *Supervisor) IsCurrent(g Grant) error {
	if !s.validGrant(g) {
		return ErrDenied
	}
	return s.withLock(func(st *state) error {
		if st.Epoch != g.Epoch || st.Owner != g.Owner || st.Session != g.Session || st.ExpiresUnix != g.ExpiresUnix {
			return ErrStale
		}
		return nil
	})
}

// Begin writes a durable pending fence BEFORE the Phase29 local intent journal.
// Any crash between these two writes stays pending and requires reconciliation.
func (s *Supervisor) Begin(g Grant, i effectgate.Intent, l *ledger.Ledger) error {
	if l == nil || !validID.MatchString(i.RequestID) || !s.validGrant(g) {
		return ErrDenied
	}
	return s.withLock(func(st *state) error {
		if st.Epoch != g.Epoch || st.Session != g.Session || st.Owner != g.Owner {
			return ErrStale
		}
		if _, exists := st.Pending[i.RequestID]; exists {
			return ErrPending
		}
		st.Pending[i.RequestID] = g.Epoch
		if err := s.saveState(*st); err != nil {
			return err
		}
		if err := l.Reserve(i); err != nil {
			return ErrPending
		}
		return nil
	})
}

// Settle releases takeover blocking only after Phase29 independently verifies
// provider-signed synthetic outcome evidence. No manual/unsigned clear path.
func (s *Supervisor) Settle(id string, l *ledger.Ledger) error {
	if !validID.MatchString(id) || l == nil {
		return ErrDenied
	}
	return s.withLock(func(st *state) error {
		if _, ok := st.Pending[id]; !ok {
			return ErrPending
		}
		status, e := l.Inspect(id)
		if e != nil || status != ledger.Confirmed {
			return ErrPending
		}
		delete(st.Pending, id)
		return s.saveState(*st)
	})
}
func (s *Supervisor) CurrentEpoch() (uint64, error) {
	var epoch uint64
	e := s.withLock(func(st *state) error { epoch = st.Epoch; return nil })
	return epoch, e
}
func (s *Supervisor) PendingCount() (int, error) {
	var count int
	e := s.withLock(func(st *state) error { count = len(st.Pending); return nil })
	return count, e
}

// TestProvider is a synthetic epoch-fencing provider with atomic in-memory
// acceptance. A real remote provider's semantics are NOT established here.
type TestProvider struct {
	mu              sync.Mutex
	epoch           uint64
	executed        []string
	supportsFencing bool
}

func NewTestProvider(supports bool) *TestProvider { return &TestProvider{supportsFencing: supports} }
func (p *TestProvider) Fence(ctx context.Context, epoch uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.supportsFencing || epoch <= p.epoch {
		return ErrFenceUnsupported
	}
	p.epoch = epoch
	return nil
}
func (p *TestProvider) Apply(epoch uint64, requestID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.supportsFencing || epoch != p.epoch {
		return ErrStale
	}
	p.executed = append(p.executed, fmt.Sprintf("%d:%s", epoch, requestID))
	return nil
}
func (p *TestProvider) Count() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.executed) }
