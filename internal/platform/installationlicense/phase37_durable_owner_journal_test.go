package installationlicense

// OFFLINE TEST ONLY: crash-aware request reservations for the Phase36 relay.
// These files and Ed25519 receipts do not implement a production domain ledger,
// PostgreSQL outbox, independent provider, or remotely fenced Owner service.

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
	"sync"
	"syscall"
)

var (
	phase37Duplicate  = errors.New("PHASE37_TEST_ALREADY_RESERVED_RECONCILE_REQUIRED")
	phase37Corrupt    = errors.New("PHASE37_TEST_CORRUPT_FAIL_CLOSED")
	phase37BadReceipt = errors.New("PHASE37_TEST_INVALID_PROVIDER_RECEIPT")
)

const phase37Version = "LIC_A9_PHASE37_SYNTHETIC_LEDGER_V1"
const phase37ReceiptDomain = "LIC_A9_PHASE37_TEST_RECEIPT\x00"

type phase37Journal struct {
	root      string
	directory *os.File
	provider  ed25519.PublicKey
	local     sync.Mutex
}

type phase37Record struct {
	Version     string `json:"version"`
	RequestID   string `json:"request_id"`
	ClaimDigest string `json:"claim_digest"`
}

type phase37Receipt struct {
	Version     string `json:"version"`
	RequestID   string `json:"request_id"`
	ClaimDigest string `json:"claim_digest"`
	ProviderID  string `json:"provider_id"`
	Result      string `json:"result"`
	Signature   string `json:"signature"`
}

type phase37ReceiptBody struct {
	Version     string `json:"version"`
	RequestID   string `json:"request_id"`
	ClaimDigest string `json:"claim_digest"`
	ProviderID  string `json:"provider_id"`
	Result      string `json:"result"`
}

func (p phase37Receipt) body() phase37ReceiptBody {
	return phase37ReceiptBody{p.Version, p.RequestID, p.ClaimDigest, p.ProviderID, p.Result}
}
func phase37Encode(v any) []byte { b, _ := json.Marshal(v); return append(b, '\n') }
func phase37Digest(c phase36Claim) string {
	b, _ := json.Marshal(c)
	h := sha256.Sum256(append([]byte("LIC_A9_PHASE37_TEST_CLAIM\x00"), b...))
	return hex.EncodeToString(h[:])
}
func phase37Name(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:])
}
func phase37Open(root string, provider ed25519.PublicKey) (*phase37Journal, error) {
	if len(provider) != ed25519.PublicKeySize || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, phase37Corrupt
	}
	dir, err := os.Open(root)
	if err != nil {
		return nil, phase37Corrupt
	}
	st, err := dir.Stat()
	if err != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		dir.Close()
		return nil, phase37Corrupt
	}
	// Directory owner must be this test process, and must not be a symlink.
	ls, e := os.Lstat(root)
	if e != nil || ls.Mode()&os.ModeSymlink != 0 {
		dir.Close()
		return nil, phase37Corrupt
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok || sys.Uid != uint32(os.Geteuid()) {
		dir.Close()
		return nil, phase37Corrupt
	}
	// A lock file may be reused after a process restart; callers MUST NOT
	// delete it to reset the journal. Unsafe same-UID mutations remain a blocker.
	fd, e := syscall.Openat(int(dir.Fd()), "_owner.lock", syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		dir.Close()
		return nil, phase37Corrupt
	}
	syscall.Close(fd)
	if e = dir.Sync(); e != nil {
		dir.Close()
		return nil, phase37Corrupt
	}
	return &phase37Journal{root: root, directory: dir, provider: append(ed25519.PublicKey(nil), provider...)}, nil
}
func (j *phase37Journal) close() error { return j.directory.Close() }
func (j *phase37Journal) locked(fn func() error) error {
	j.local.Lock()
	defer j.local.Unlock()
	fd, e := syscall.Openat(int(j.directory.Fd()), "_owner.lock", syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return phase37Corrupt
	}
	defer syscall.Close(fd)
	var stat syscall.Stat_t
	if syscall.Fstat(fd, &stat) != nil || stat.Mode&syscall.S_IFMT != syscall.S_IFREG || stat.Nlink != 1 || stat.Mode&0077 != 0 || stat.Uid != uint32(os.Geteuid()) {
		return phase37Corrupt
	}
	if syscall.Flock(fd, syscall.LOCK_EX) != nil {
		return phase37Corrupt
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	return fn()
}
func (j *phase37Journal) read(name string, target any) error {
	fd, e := syscall.Openat(int(j.directory.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	var s syscall.Stat_t
	if syscall.Fstat(fd, &s) != nil || s.Mode&syscall.S_IFMT != syscall.S_IFREG || s.Nlink != 1 || s.Mode&0077 != 0 || s.Uid != uint32(os.Geteuid()) || s.Size > 4096 {
		return phase37Corrupt
	}
	b, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil || len(b) > 4096 || json.Unmarshal(b, target) != nil || !bytes.Equal(b, phase37Encode(target)) {
		return phase37Corrupt
	}
	return nil
}
func (j *phase37Journal) writeExclusive(name string, payload any) error {
	b := phase37Encode(payload)
	if len(b) > 4096 {
		return phase37Corrupt
	}
	fd, e := syscall.Openat(int(j.directory.Fd()), name, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return phase37Corrupt
	}
	f := os.NewFile(uintptr(fd), name)
	n, e := f.Write(b)
	if e == nil && n != len(b) {
		e = io.ErrShortWrite
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil || j.directory.Sync() != nil {
		return phase37Corrupt
	}
	return nil
}
func (j *phase37Journal) reserve(c phase36Claim) error {
	if c.Protocol != phase36Protocol || c.RequestID == "" || len(c.RequestID) > 96 {
		return phase37Corrupt
	}
	return j.locked(func() error {
		name := "r-" + phase37Name(c.RequestID)
		var existing phase37Record
		e := j.read(name, &existing)
		if e == nil {
			return phase37Duplicate
		}
		if !errors.Is(e, syscall.ENOENT) {
			return phase37Corrupt
		}
		return j.writeExclusive(name, phase37Record{phase37Version, c.RequestID, phase37Digest(c)})
	})
}
func phase37SyntheticReceipt(priv ed25519.PrivateKey, c phase36Claim, providerID string) phase37Receipt {
	p := phase37Receipt{Version: phase37Version, RequestID: c.RequestID, ClaimDigest: phase37Digest(c), ProviderID: providerID, Result: "CONFIRMED"}
	b, _ := json.Marshal(p.body())
	p.Signature = hex.EncodeToString(ed25519.Sign(priv, append([]byte(phase37ReceiptDomain), b...)))
	return p
}
func (j *phase37Journal) confirm(p phase37Receipt) error {
	if p.Version != phase37Version || p.RequestID == "" || len(p.RequestID) > 96 || p.ProviderID == "" || len(p.ProviderID) > 96 || p.Result != "CONFIRMED" {
		return phase37BadReceipt
	}
	sig, e := hex.DecodeString(p.Signature)
	b, _ := json.Marshal(p.body())
	if e != nil || !ed25519.Verify(j.provider, append([]byte(phase37ReceiptDomain), b...), sig) {
		return phase37BadReceipt
	}
	return j.locked(func() error {
		var r phase37Record
		if e := j.read("r-"+phase37Name(p.RequestID), &r); e != nil {
			return phase37Corrupt
		}
		if r.Version != phase37Version || r.RequestID != p.RequestID || r.ClaimDigest != p.ClaimDigest {
			return phase37BadReceipt
		}
		name := "c-" + phase37Name(p.RequestID)
		var old phase37Receipt
		e := j.read(name, &old)
		if e == nil {
			if bytes.Equal(phase37Encode(old), phase37Encode(p)) {
				return nil
			}
			return phase37BadReceipt
		}
		if !errors.Is(e, syscall.ENOENT) {
			return phase37Corrupt
		}
		return j.writeExclusive(name, p)
	})
}
func (j *phase37Journal) inspect(c phase36Claim) (string, error) {
	var out string
	e := j.locked(func() error {
		var r phase37Record
		if e := j.read("r-"+phase37Name(c.RequestID), &r); e != nil {
			return phase37Corrupt
		}
		if r.Version != phase37Version || r.RequestID != c.RequestID || r.ClaimDigest != phase37Digest(c) {
			return phase37Corrupt
		}
		var p phase37Receipt
		e := j.read("c-"+phase37Name(c.RequestID), &p)
		if errors.Is(e, syscall.ENOENT) {
			out = "RECONCILE_REQUIRED"
			return nil
		}
		if e != nil {
			return phase37Corrupt
		}
		b, _ := json.Marshal(p.body())
		sig, decode := hex.DecodeString(p.Signature)
		if decode != nil || p.Version != phase37Version || p.RequestID != r.RequestID || p.ClaimDigest != r.ClaimDigest || p.ProviderID == "" || p.Result != "CONFIRMED" || !ed25519.Verify(j.provider, append([]byte(phase37ReceiptDomain), b...), sig) {
			return phase37Corrupt
		}
		out = "CONFIRMED_WITH_SYNTHETIC_PROVIDER_EVIDENCE"
		return nil
	})
	return out, e
}
