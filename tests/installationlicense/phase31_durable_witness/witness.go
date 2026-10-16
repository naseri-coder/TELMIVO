// Package witness is a NONPRODUCTION Linux-local durability and fencing proof.
// It is neither an independent remote witness nor an actual external provider.
// Real payment/exchange/Telegram APIs do NOT enforce its epochs.
package witness

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
)

var (
	ErrUnsafe    = errors.New("PHASE31_WITNESS_UNSAFE_ROOT")
	ErrCorrupt   = errors.New("PHASE31_WITNESS_CORRUPT")
	ErrStale     = errors.New("PHASE31_WITNESS_STALE_EPOCH")
	ErrDuplicate = errors.New("PHASE31_WITNESS_DUPLICATE_INTENT")
	ErrBadIntent = errors.New("PHASE31_WITNESS_BAD_INTENT")
)

const stateFile = "epoch.json"
const lockFile = "witness.lock"
const maxBytes = 8192
const version = "TELMIVO_PHASE31_TEST_WITNESS_V1"

var simpleID = regexp.MustCompile("^[A-Za-z0-9_-]{1,96}$")

type state struct {
	Version string `json:"version"`
	Epoch   uint64 `json:"epoch"`
}

type Applied struct {
	Version   string `json:"version"`
	Epoch     uint64 `json:"epoch"`
	RequestID string `json:"request_id"`
	// This entry is the entire synthetic effect. No arbitrary callback is invoked.
	SyntheticResult string `json:"synthetic_result"`
}

type Witness struct {
	dir *os.File
	mu  sync.Mutex
}

// Open requires a supervisor-owned (or single-principal) root mode 0700.
// It does NOT secure untrusted same-UID writers or symlinked ancestors.
func Open(root string) (*Witness, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" {
		return nil, ErrUnsafe
	}
	fd, err := syscall.Open(root, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrUnsafe
	}
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFDIR || st.Mode&0077 != 0 || int(st.Uid) != os.Geteuid() {
		syscall.Close(fd)
		return nil, ErrUnsafe
	}
	w := &Witness{dir: os.NewFile(uintptr(fd), "test-witness-root")}
	// Marker is never reset: a torn first initialization is restrictive.
	marker, e := syscall.Openat(fd, lockFile, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	fresh := e == nil
	if fresh {
		if syscall.Fsync(marker) != nil {
			syscall.Close(marker)
			w.Close()
			return nil, ErrCorrupt
		}
		syscall.Close(marker)
		if w.dir.Sync() != nil {
			w.Close()
			return nil, ErrCorrupt
		}
	} else if !errors.Is(e, syscall.EEXIST) {
		w.Close()
		return nil, ErrCorrupt
	}
	if fresh {
		e = w.withLock(func() error { return w.atomicState(state{Version: version, Epoch: 0}) })
	} else {
		e = w.withLock(func() error { _, er := w.readState(); return er })
	}
	if e != nil {
		w.Close()
		return nil, e
	}
	return w, nil
}
func (w *Witness) Close() error { return w.dir.Close() }

func (w *Witness) withLock(fn func() error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	fd, e := syscall.Openat(int(w.dir.Fd()), lockFile, syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
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
func encode(v any) []byte { b, _ := json.Marshal(v); return append(b, '\n') }
func (w *Witness) read(name string) ([]byte, error) {
	fd, e := syscall.Openat(int(w.dir.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), "test-entry")
	defer f.Close()
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Nlink != 1 || st.Mode&0077 != 0 || int(st.Uid) != os.Geteuid() || st.Size > maxBytes {
		return nil, ErrCorrupt
	}
	b, e := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if e != nil || len(b) > maxBytes {
		return nil, ErrCorrupt
	}
	return b, nil
}
func (w *Witness) readState() (state, error) {
	var v state
	b, e := w.read(stateFile)
	if e != nil {
		return v, ErrCorrupt
	}
	if json.Unmarshal(b, &v) != nil || v.Version != version || !bytes.Equal(b, encode(v)) {
		return v, ErrCorrupt
	}
	return v, nil
}
func (w *Witness) atomicState(s state) error {
	var random [16]byte
	if _, e := rand.Read(random[:]); e != nil {
		return ErrCorrupt
	}
	temporary := ".phase31-" + hex.EncodeToString(random[:])
	fd, e := syscall.Openat(int(w.dir.Fd()), temporary, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return ErrCorrupt
	}
	defer syscall.Unlinkat(int(w.dir.Fd()), temporary) // cleanup after successful rename is harmless
	file := os.NewFile(uintptr(fd), "temporary-state")
	b := encode(s)
	n, writeErr := file.Write(b)
	if writeErr == nil && n != len(b) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return ErrCorrupt
	}
	if syscall.Renameat(int(w.dir.Fd()), temporary, int(w.dir.Fd()), stateFile) != nil {
		return ErrCorrupt
	}
	if w.dir.Sync() != nil {
		return ErrCorrupt
	}
	return nil
}
func filename(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "applied-" + hex.EncodeToString(sum[:])
}

// Fence serializes against *local witness-side* Apply, and persists new epoch.
// Cross-node quorum and an external provider atomic epoch boundary are NOT proved.
func (w *Witness) Fence(ctx context.Context, next uint64) error {
	if next == 0 {
		return ErrStale
	}
	return w.withLock(func() error {
		if e := ctx.Err(); e != nil {
			return e
		}
		s, e := w.readState()
		if e != nil {
			return e
		}
		if s.Epoch == ^uint64(0) || next != s.Epoch+1 {
			return ErrStale
		}
		s.Epoch = next
		return w.atomicState(s)
	})
}
func (w *Witness) CurrentEpoch() (uint64, error) {
	var epoch uint64
	e := w.withLock(func() error {
		s, er := w.readState()
		if er == nil {
			epoch = s.Epoch
		}
		return er
	})
	return epoch, e
}
func (w *Witness) inspectApplied(name string) (Applied, error) {
	var a Applied
	b, e := w.read(name)
	if e != nil {
		return a, e
	}
	if json.Unmarshal(b, &a) != nil || a.Version != version || a.Epoch == 0 || !simpleID.MatchString(a.RequestID) || a.SyntheticResult != "SYNTHETIC_APPLIED" || !bytes.Equal(b, encode(a)) || filename(a.RequestID) != name {
		return a, ErrCorrupt
	}
	return a, nil
}

// Apply is the synthetic provider's entire bounded effect: append a durable
// local record only after checking the latest epoch under the SAME witness lock.
// No network calls, side effects outside the witness, or provider settlement.
func (w *Witness) Apply(ctx context.Context, epoch uint64, requestID string) error {
	if !simpleID.MatchString(requestID) || epoch == 0 {
		return ErrBadIntent
	}
	return w.withLock(func() error {
		if e := ctx.Err(); e != nil {
			return e
		}
		st, e := w.readState()
		if e != nil {
			return e
		}
		if epoch != st.Epoch {
			return ErrStale
		}
		name := filename(requestID)
		old, e := w.inspectApplied(name)
		if e == nil {
			if old.Epoch != epoch {
				return ErrBadIntent
			}
			return ErrDuplicate
		}
		if !errors.Is(e, syscall.ENOENT) {
			return ErrCorrupt
		}
		fd, e := syscall.Openat(int(w.dir.Fd()), name, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
		if e != nil {
			return ErrCorrupt
		}
		file := os.NewFile(uintptr(fd), "synthetic-applied")
		b := encode(Applied{Version: version, Epoch: epoch, RequestID: requestID, SyntheticResult: "SYNTHETIC_APPLIED"})
		n, writeErr := file.Write(b)
		if writeErr == nil && n != len(b) {
			writeErr = io.ErrShortWrite
		}
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
		if writeErr != nil {
			return ErrCorrupt
		} // incomplete entry stays as denial
		if w.dir.Sync() != nil {
			return ErrCorrupt
		}
		return nil
	})
}
func (w *Witness) WasApplied(id string) (bool, error) {
	if !simpleID.MatchString(id) {
		return false, ErrBadIntent
	}
	found := false
	e := w.withLock(func() error {
		_, e := w.inspectApplied(filename(id))
		if errors.Is(e, syscall.ENOENT) {
			return nil
		}
		if e == nil {
			found = true
		}
		return e
	})
	return found, e
}
