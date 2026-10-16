// Package ledger is an OFFLINE, NONPRODUCTION outcome journal for synthetic
// effect intents. It is not the TELMIVO runtime ledger, a signed license gate,
// external provider integration, or a claim of exactly-once execution.
package ledger

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"

	effectgate "telmivo.local/lic-a9-phase28-effect-proof"
)

const intentSchema = "TELMIVO_LIC_A9_PHASE29_INTENT_V1"
const outcomeSchema = "TELMIVO_LIC_A9_PHASE29_PROVIDER_OUTCOME_V1"
const signedDomain = "TELMIVO_LIC_A9_PHASE29_OUTCOME_V1\x00"
const maxRecord = 8192

var (
	ErrUnsafeRoot = errors.New("PHASE29_TEST_ONLY_UNSAFE_ROOT")
	ErrConflict   = errors.New("PHASE29_TEST_ONLY_IDENTITY_CONFLICT")
	ErrDuplicate  = errors.New("PHASE29_TEST_ONLY_INTENT_ALREADY_RESERVED")
	ErrCorrupt    = errors.New("PHASE29_TEST_ONLY_JOURNAL_CORRUPT")
	ErrReconcile  = errors.New("PHASE29_TEST_ONLY_RECONCILIATION_REQUIRED")
	ErrBadProof   = errors.New("PHASE29_TEST_ONLY_PROVIDER_PROOF_DENIED")
)
var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,96}$`)

type Status string

const (
	ReconcileRequired Status = "RECONCILE_REQUIRED"
	Confirmed         Status = "CONFIRMED_WITH_SYNTHETIC_PROVIDER_EVIDENCE"
)

type intentEntry struct {
	Schema string            `json:"schema"`
	Intent effectgate.Intent `json:"intent"`
}

type Outcome struct {
	Schema              string `json:"schema"`
	RequestID           string `json:"request_id"`
	IntentHash          string `json:"intent_hash"`
	ProviderOperationID string `json:"provider_operation_id"`
	Result              string `json:"result"`
	Signature           string `json:"signature"`
}

type signedOutcome struct {
	Schema              string `json:"schema"`
	RequestID           string `json:"request_id"`
	IntentHash          string `json:"intent_hash"`
	ProviderOperationID string `json:"provider_operation_id"`
	Result              string `json:"result"`
}

// Ledger assumes a supervisor-enforced private 0700 root. A hostile process
// with write access to this root can replace paths; this is NOT a shared-UID
// security boundary. Tests use only disposable directories.
type Ledger struct {
	dir         *os.File
	providerKey ed25519.PublicKey
}

func Open(path string, providerKey ed25519.PublicKey) (*Ledger, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(providerKey) != ed25519.PublicKeySize {
		return nil, ErrUnsafeRoot
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, ErrUnsafeRoot
	}
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFDIR || st.Mode&0077 != 0 || int(st.Uid) != os.Geteuid() {
		syscall.Close(fd)
		return nil, ErrUnsafeRoot
	}
	return &Ledger{dir: os.NewFile(uintptr(fd), "phase29-test-root"), providerKey: append(ed25519.PublicKey(nil), providerKey...)}, nil
}
func (l *Ledger) Close() error { return l.dir.Close() }

func fileName(prefix, requestID string) string {
	h := sha256.Sum256([]byte(requestID))
	return prefix + hex.EncodeToString(h[:])
}
func jsonLine(v any) []byte {
	b, _ := json.Marshal(v)
	return append(b, '\n')
}
func validateIntent(i effectgate.Intent) bool {
	return validID.MatchString(i.RequestID) && validID.MatchString(i.WorkerID) && validID.MatchString(i.WorkspaceID) && validID.MatchString(i.ActorID) && validID.MatchString(i.ResourceID) && i.Operation == effectgate.SyntheticOperation
}
func intentBytes(i effectgate.Intent) []byte {
	return jsonLine(intentEntry{Schema: intentSchema, Intent: i})
}
func (l *Ledger) lock(fn func() error) error {
	fd, e := syscall.Openat(int(l.dir.Fd()), "_journal.lock", syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return ErrCorrupt
	}
	defer syscall.Close(fd)
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Nlink != 1 || st.Mode&0077 != 0 || int(st.Uid) != os.Geteuid() {
		return ErrCorrupt
	}
	if syscall.Flock(fd, syscall.LOCK_EX) != nil {
		return ErrCorrupt
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	return fn()
}
func (l *Ledger) read(name string) ([]byte, error) {
	fd, e := syscall.Openat(int(l.dir.Fd()), name, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), "entry")
	defer f.Close()
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Nlink != 1 || st.Mode&0077 != 0 || int(st.Uid) != os.Geteuid() || st.Size > maxRecord {
		return nil, ErrCorrupt
	}
	b, e := io.ReadAll(io.LimitReader(f, maxRecord+1))
	if e != nil || len(b) > maxRecord {
		return nil, ErrCorrupt
	}
	return b, nil
}
func (l *Ledger) saveExclusive(name string, b []byte) error {
	if len(b) < 2 || len(b) > maxRecord {
		return ErrCorrupt
	}
	fd, e := syscall.Openat(int(l.dir.Fd()), name, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		if errors.Is(e, syscall.EEXIST) {
			return ErrDuplicate
		}
		return ErrCorrupt
	}
	f := os.NewFile(uintptr(fd), "entry")
	n, e := f.Write(b)
	if e == nil && n != len(b) {
		e = io.ErrShortWrite
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
	} // partial durable file remains a fail-closed tombstone
	if e = l.dir.Sync(); e != nil {
		return ErrCorrupt
	}
	return nil
}
func (l *Ledger) readIntent(id string) (intentEntry, error) {
	var out intentEntry
	b, e := l.read(fileName("i-", id))
	if e != nil {
		return out, e
	}
	if json.Unmarshal(b, &out) != nil || out.Schema != intentSchema || out.Intent.RequestID != id || !validateIntent(out.Intent) || !bytes.Equal(jsonLine(out), b) {
		return out, ErrCorrupt
	}
	return out, nil
}
func (l *Ledger) Reserve(i effectgate.Intent) error {
	if !validateIntent(i) {
		return ErrConflict
	}
	return l.lock(func() error {
		record, e := l.readIntent(i.RequestID)
		if e == nil {
			if !bytes.Equal(intentBytes(record.Intent), intentBytes(i)) {
				return ErrConflict
			}
			return ErrDuplicate
		}
		if !errors.Is(e, syscall.ENOENT) {
			return ErrCorrupt
		}
		return l.saveExclusive(fileName("i-", i.RequestID), intentBytes(i))
	})
}

func outcomePayload(e Outcome) []byte {
	t := signedOutcome{Schema: e.Schema, RequestID: e.RequestID, IntentHash: e.IntentHash, ProviderOperationID: e.ProviderOperationID, Result: e.Result}
	return append([]byte(signedDomain), jsonLine(t)...)
}
func IntentHash(i effectgate.Intent) string {
	sum := sha256.Sum256(intentBytes(i))
	return hex.EncodeToString(sum[:])
}

// SyntheticProof is test-only. No private signing key belongs in the repo.
func SyntheticProof(priv ed25519.PrivateKey, i effectgate.Intent, providerID string) Outcome {
	e := Outcome{Schema: outcomeSchema, RequestID: i.RequestID, IntentHash: IntentHash(i), ProviderOperationID: providerID, Result: "CONFIRMED"}
	e.Signature = hex.EncodeToString(ed25519.Sign(priv, outcomePayload(e)))
	return e
}
func (l *Ledger) checkProof(i effectgate.Intent, e Outcome) bool {
	if e.Schema != outcomeSchema || e.RequestID != i.RequestID || e.IntentHash != IntentHash(i) || !validID.MatchString(e.ProviderOperationID) || e.Result != "CONFIRMED" {
		return false
	}
	sig, err := hex.DecodeString(e.Signature)
	return err == nil && len(sig) == ed25519.SignatureSize && ed25519.Verify(l.providerKey, outcomePayload(e), sig)
}
func (l *Ledger) readOutcome(i effectgate.Intent) (Outcome, error) {
	var o Outcome
	b, e := l.read(fileName("o-", i.RequestID))
	if e != nil {
		return o, e
	}
	if json.Unmarshal(b, &o) != nil || !bytes.Equal(jsonLine(o), b) || !l.checkProof(i, o) {
		return o, ErrCorrupt
	}
	return o, nil
}

// Inspect treats reserved-but-unconfirmed work as UNKNOWN, not retryable.
func (l *Ledger) Inspect(id string) (Status, error) {
	if !validID.MatchString(id) {
		return "", ErrConflict
	}
	var status Status
	e := l.lock(func() error {
		record, e := l.readIntent(id)
		if e != nil {
			if errors.Is(e, syscall.ENOENT) {
				return ErrReconcile
			}
			return ErrCorrupt
		}
		_, e = l.readOutcome(record.Intent)
		if e == nil {
			status = Confirmed
			return nil
		}
		if errors.Is(e, syscall.ENOENT) {
			status = ReconcileRequired
			return nil
		}
		return ErrCorrupt
	})
	return status, e
}

// Confirm requires synthetic provider-signed evidence bound to the FULL intent.
// A real adapter must verify independent provider settlement evidence first.
func (l *Ledger) Confirm(evidence Outcome) error {
	if !validID.MatchString(evidence.RequestID) {
		return ErrBadProof
	}
	return l.lock(func() error {
		record, e := l.readIntent(evidence.RequestID)
		if e != nil {
			return ErrCorrupt
		}
		if !l.checkProof(record.Intent, evidence) {
			return ErrBadProof
		}
		prior, e := l.readOutcome(record.Intent)
		if e == nil {
			if !bytes.Equal(jsonLine(prior), jsonLine(evidence)) {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, syscall.ENOENT) {
			return ErrCorrupt
		}
		return l.saveExclusive(fileName("o-", evidence.RequestID), jsonLine(evidence))
	})
}

// Compile-time check: the real phase28 synthetic gate calls this interface.
var _ effectgate.Reservations = (*Ledger)(nil)
