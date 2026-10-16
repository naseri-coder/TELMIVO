//go:build linux

// Package bridge proves a NONPRODUCTION Linux multi-UID IPC + durable journal
// security boundary. The synthetic journal record IS the only effect. There is
// NO real Manager.Effect, Authority-issued license, database or remote provider.
package bridge

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"time"
)

const Protocol = "TELMIVO_LIC_A9_PHASE38_SYNTHETIC_IPC_V1"
const domain = "TELMIVO_PHASE38_REQUEST\x00"
const receiptDomain = "TELMIVO_PHASE38_PROVIDER_EVIDENCE\x00"
const recordVersion = "PHASE38_ROOT_OWNED_RECORD_V1"
const maxFrame = 3072

var idRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,96}$`)
var ErrBoundary = errors.New("PHASE38_UNTRUSTED_BOUNDARY")

type Claim struct {
	Protocol     string `json:"protocol"`
	Action       string `json:"action"`
	Installation string `json:"installation"`
	Owner        string `json:"owner"`
	Session      string `json:"session"`
	Epoch        uint64 `json:"epoch"`
	RequestID    string `json:"request_id"`
	Tenant       string `json:"tenant"`
	Actor        string `json:"actor"`
	Resource     string `json:"resource"`
	NotBefore    int64  `json:"not_before"`
	Expires      int64  `json:"expires"`
}
type ReceiptBody struct {
	RequestID  string `json:"request_id"`
	ClaimHash  string `json:"claim_hash"`
	ProviderID string `json:"provider_id"`
	Result     string `json:"result"`
}
type Receipt struct {
	Body      ReceiptBody `json:"body"`
	Signature string      `json:"signature"`
}
type Wire struct {
	Claim     Claim    `json:"claim"`
	Signature string   `json:"signature"`
	Receipt   *Receipt `json:"receipt,omitempty"`
}

func Sign(private ed25519.PrivateKey, c Claim, receipt *Receipt) Wire {
	b, _ := json.Marshal(c)
	return Wire{Claim: c, Signature: hex.EncodeToString(ed25519.Sign(private, append([]byte(domain), b...))), Receipt: receipt}
}
func SignReceipt(private ed25519.PrivateKey, c Claim, providerID string) Receipt {
	b, _ := json.Marshal(c)
	digest := sha256.Sum256(append([]byte("PHASE38_CLAIM\x00"), b...))
	body := ReceiptBody{RequestID: c.RequestID, ClaimHash: hex.EncodeToString(digest[:]), ProviderID: providerID, Result: "CONFIRMED"}
	canonical, _ := json.Marshal(body)
	return Receipt{body, hex.EncodeToString(ed25519.Sign(private, append([]byte(receiptDomain), canonical...)))}
}
func digest(c Claim) string {
	b, _ := json.Marshal(c)
	d := sha256.Sum256(append([]byte("PHASE38_CLAIM\x00"), b...))
	return hex.EncodeToString(d[:])
}
func fileID(id string) string { h := sha256.Sum256([]byte(id)); return hex.EncodeToString(h[:]) }
func canonical(v any) []byte  { b, _ := json.Marshal(v); return append(b, '\n') }

type persistedEpoch struct {
	Version string `json:"version"`
	Epoch   uint64 `json:"epoch"`
	Owner   string `json:"owner"`
	Session string `json:"session"`
}
type record struct {
	Version   string `json:"version"`
	RequestID string `json:"request_id"`
	ClaimHash string `json:"claim_hash"`
}

type Bridge struct {
	mu           sync.Mutex
	dir          *os.File
	socket       *net.UnixListener
	socketName   string
	socketDir    string
	installation string
	readyEpoch   uint64 // volatile: restart quarantines old signed session until a fresh FENCE
	workerUID    uint32
	issuer       ed25519.PublicKey
	provider     ed25519.PublicKey
	handlers     sync.WaitGroup
	done         chan struct{}
	started      bool
	closed       bool
}

func checkDir(path string, mode os.FileMode, uid, gid uint32) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return ErrBoundary
	}
	st, e := os.Lstat(path)
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm() != mode {
		return ErrBoundary
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok || sys.Uid != uid || sys.Gid != gid {
		return ErrBoundary
	}
	return nil
}
func Open(socketDir, journalDir, installation string, workerUID uint32, issuer, provider ed25519.PublicKey) (*Bridge, error) {
	if os.Geteuid() != 0 || !idRE.MatchString(installation) || workerUID == 0 || len(issuer) != ed25519.PublicKeySize || len(provider) != ed25519.PublicKeySize || socketDir == journalDir {
		return nil, ErrBoundary
	}
	if checkDir(socketDir, 0750, 0, workerUID) != nil || checkDir(journalDir, 0700, 0, 0) != nil {
		return nil, ErrBoundary
	}
	fd, e := syscall.Open(journalDir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrBoundary
	}
	b := &Bridge{dir: os.NewFile(uintptr(fd), "supervisor-journal"), socketDir: socketDir, installation: installation, workerUID: workerUID, issuer: append(ed25519.PublicKey(nil), issuer...), provider: append(ed25519.PublicKey(nil), provider...), done: make(chan struct{})}
	// The first open creates a permanent lock marker. Missing epoch with an
	// existing marker is CORRUPT, never reset silently to epoch zero.
	lockfd, e := syscall.Openat(fd, "_root.lock", syscall.O_CREAT|syscall.O_EXCL|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	fresh := e == nil
	if errors.Is(e, syscall.EEXIST) {
		lockfd, e = syscall.Openat(fd, "_root.lock", syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	}
	if e != nil {
		b.dir.Close()
		return nil, ErrBoundary
	}
	syscall.Close(lockfd)
	if fresh {
		if e = b.dir.Sync(); e != nil {
			b.dir.Close()
			return nil, ErrBoundary
		}
		e = b.locked(func() error { return b.writeExclusive("epoch.json", persistedEpoch{Version: recordVersion}) })
	} else {
		e = b.locked(func() error { var v persistedEpoch; return b.read("epoch.json", &v) })
	}
	if e != nil {
		b.dir.Close()
		return nil, e
	}
	name := filepath.Join(socketDir, "owner.sock")
	if _, e = os.Lstat(name); !errors.Is(e, os.ErrNotExist) {
		b.dir.Close()
		return nil, ErrBoundary
	}
	ln, e := net.ListenUnix("unix", &net.UnixAddr{Name: name, Net: "unix"})
	if e != nil {
		b.dir.Close()
		return nil, e
	}
	if e = os.Chown(name, 0, int(workerUID)); e == nil {
		e = os.Chmod(name, 0660)
	}
	if e != nil {
		ln.Close()
		b.dir.Close()
		return nil, ErrBoundary
	}
	b.socket = ln
	b.socketName = name
	return b, nil
}
func (b *Bridge) Address() string { return b.socketName }
func (b *Bridge) Start() {
	b.mu.Lock()
	if b.started || b.closed {
		b.mu.Unlock()
		return
	}
	b.started = true
	b.mu.Unlock()
	go b.serve()
}
func (b *Bridge) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	started := b.started
	b.socket.Close()
	b.mu.Unlock()
	if started {
		<-b.done
	}
	b.handlers.Wait()
	b.dir.Close()
	_ = os.Remove(b.socketName)
}
func (b *Bridge) serve() {
	defer close(b.done)
	for {
		c, e := b.socket.AcceptUnix()
		if e != nil {
			return
		}
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			c.Close()
			return
		}
		b.handlers.Add(1)
		b.mu.Unlock()
		go func() { defer b.handlers.Done(); defer c.Close(); b.handle(c) }()
	}
}
func cred(c *net.UnixConn) (uint32, error) {
	raw, e := c.SyscallConn()
	if e != nil {
		return 0, e
	}
	var u *syscall.Ucred
	var inner error
	e = raw.Control(func(fd uintptr) { u, inner = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED) })
	if e != nil {
		return 0, e
	}
	if inner != nil {
		return 0, inner
	}
	return u.Uid, nil
}
func (b *Bridge) handle(c *net.UnixConn) {
	_ = c.SetDeadline(time.Now().Add(4 * time.Second))
	reply := func(s string) { _, _ = c.Write([]byte(s + "\n")) }
	uid, e := cred(c)
	if e != nil || uid != b.workerUID {
		reply("DENIED_PEER")
		return
	}
	raw, e := bufio.NewReader(io.LimitReader(c, maxFrame+2)).ReadBytes('\n')
	if e != nil || len(raw) > maxFrame+1 {
		reply("DENIED_FRAME")
		return
	}
	raw = raw[:len(raw)-1]
	var w Wire
	if json.Unmarshal(raw, &w) != nil {
		reply("DENIED_FRAME")
		return
	}
	enc, _ := json.Marshal(w)
	if !bytes.Equal(enc, raw) {
		reply("DENIED_CANONICAL")
		return
	}
	cc := w.Claim
	now := time.Now().Unix()
	if cc.Protocol != Protocol || cc.Installation != b.installation || !idRE.MatchString(cc.Owner) || !idRE.MatchString(cc.Session) || cc.Epoch == 0 || cc.NotBefore > now || cc.Expires <= now || cc.Expires-cc.NotBefore > 300 {
		reply("DENIED_CLAIM")
		return
	}
	sig, e := hex.DecodeString(w.Signature)
	body, _ := json.Marshal(cc)
	if e != nil || !ed25519.Verify(b.issuer, append([]byte(domain), body...), sig) {
		reply("DENIED_SIGNATURE")
		return
	}
	e = b.locked(func() error { return b.apply(cc, w.Receipt, reply) })
	if e != nil {
		reply("DENIED_JOURNAL_OR_IO")
	}
}
func (b *Bridge) locked(f func() error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrBoundary
	}
	fd, e := syscall.Openat(int(b.dir.Fd()), "_root.lock", syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return ErrBoundary
	}
	defer syscall.Close(fd)
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Nlink != 1 || st.Uid != 0 || st.Mode&0077 != 0 {
		return ErrBoundary
	}
	if syscall.Flock(fd, syscall.LOCK_EX) != nil {
		return ErrBoundary
	}
	defer syscall.Flock(fd, syscall.LOCK_UN)
	return f()
}
func (b *Bridge) read(name string, v any) error {
	fd, e := syscall.Openat(int(b.dir.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(fd), "record")
	defer f.Close()
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Nlink != 1 || st.Uid != 0 || st.Mode&0077 != 0 || st.Size > 4096 {
		return ErrBoundary
	}
	raw, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil || len(raw) > 4096 || json.Unmarshal(raw, v) != nil || !bytes.Equal(raw, canonical(v)) {
		return ErrBoundary
	}
	return nil
}
func (b *Bridge) writeExclusive(name string, v any) error {
	fd, e := syscall.Openat(int(b.dir.Fd()), name, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return ErrBoundary
	}
	f := os.NewFile(uintptr(fd), "record")
	raw := canonical(v)
	n, e := f.Write(raw)
	if e == nil && n != len(raw) {
		e = io.ErrShortWrite
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil || b.dir.Sync() != nil {
		return ErrBoundary
	}
	return nil
}
func (b *Bridge) advanceEpoch(old persistedEpoch, updated persistedEpoch) error {
	// Safely replace epoch with fsynced temp+rename; root-only directory means
	// nonprivileged worker cannot substitute lock or epoch path.
	var r [12]byte
	if _, e := rand.Read(r[:]); e != nil {
		return ErrBoundary
	}
	name := ".epoch-" + hex.EncodeToString(r[:])
	fd, e := syscall.Openat(int(b.dir.Fd()), name, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return ErrBoundary
	}
	defer syscall.Unlinkat(int(b.dir.Fd()), name)
	f := os.NewFile(uintptr(fd), "epoch-tmp")
	raw := canonical(updated)
	n, e := f.Write(raw)
	if e == nil && n != len(raw) {
		e = io.ErrShortWrite
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return ErrBoundary
	}
	if syscall.Renameat(int(b.dir.Fd()), name, int(b.dir.Fd()), "epoch.json") != nil {
		return ErrBoundary
	}
	return b.dir.Sync()
}
func (b *Bridge) pending() bool {
	_, seekErr := b.dir.Seek(0, io.SeekStart)
	if seekErr != nil {
		return true
	}
	items, e := b.dir.Readdirnames(-1)
	if e != nil {
		return true
	}
	for _, name := range items {
		if len(name) > 2 && name[:2] == "r-" {
			var r record
			if b.read(name, &r) != nil {
				return true
			}
			var p Receipt
			if b.read("c-"+name[2:], &p) != nil {
				return true
			}
			if p.Body.RequestID != r.RequestID || p.Body.ClaimHash != r.ClaimHash || !b.receiptValid(p) {
				return true
			}
		}
	}
	return false
}
func (b *Bridge) receiptValid(p Receipt) bool {
	if p.Body.Result != "CONFIRMED" || !idRE.MatchString(p.Body.RequestID) || !idRE.MatchString(p.Body.ProviderID) {
		return false
	}
	s, e := hex.DecodeString(p.Signature)
	if e != nil {
		return false
	}
	raw, _ := json.Marshal(p.Body)
	return ed25519.Verify(b.provider, append([]byte(receiptDomain), raw...), s)
}
func (b *Bridge) apply(c Claim, receipt *Receipt, reply func(string)) error {
	var state persistedEpoch
	if b.read("epoch.json", &state) != nil || state.Version != recordVersion {
		return ErrBoundary
	}
	switch c.Action {
	case "FENCE":
		if c.RequestID != "" || c.Tenant != "" || c.Actor != "" || c.Resource != "" || receipt != nil || state.Epoch == ^uint64(0) || c.Epoch != state.Epoch+1 {
			reply("DENIED_EPOCH")
			return nil
		}
		if b.pending() {
			reply("RECONCILE_REQUIRED")
			return nil
		}
		if err := b.advanceEpoch(state, persistedEpoch{Version: recordVersion, Epoch: c.Epoch, Owner: c.Owner, Session: c.Session}); err != nil {
			return err
		}
		b.readyEpoch = c.Epoch
		reply("FENCED_TEST_ONLY")
		return nil
	case "APPLY":
		if !idRE.MatchString(c.RequestID) || !idRE.MatchString(c.Tenant) || !idRE.MatchString(c.Actor) || !idRE.MatchString(c.Resource) || receipt != nil {
			reply("DENIED_CLAIM")
			return nil
		}
		if c.Epoch != state.Epoch || c.Session != state.Session || c.Owner != state.Owner {
			reply("DENIED_STALE_SESSION")
			return nil
		}
		if b.readyEpoch != c.Epoch {
			reply("DENIED_RESTART_REQUIRES_FRESH_FENCE")
			return nil
		}
		if c.Tenant != "tenantA" || c.Actor != "actorA" || c.Resource != "resourceA" {
			reply("DENIED_DOMAIN")
			return nil
		}
		name := "r-" + fileID(c.RequestID)
		var prev record
		e := b.read(name, &prev)
		if e == nil {
			reply("DENIED_DUPLICATE_RECONCILE_REQUIRED")
			return nil
		}
		if !errors.Is(e, syscall.ENOENT) {
			return ErrBoundary
		}
		if b.writeExclusive(name, record{Version: recordVersion, RequestID: c.RequestID, ClaimHash: digest(c)}) != nil {
			return ErrBoundary
		}
		reply("RECONCILE_REQUIRED")
		return nil // NO real external callback or auto-retry
	case "CONFIRM":
		if receipt == nil || !idRE.MatchString(c.RequestID) || !b.receiptValid(*receipt) {
			reply("DENIED_PROVIDER_PROOF")
			return nil
		}
		if c.Epoch != state.Epoch || c.Session != state.Session || c.Owner != state.Owner {
			reply("DENIED_STALE_SESSION")
			return nil
		}
		var r record
		if b.read("r-"+fileID(c.RequestID), &r) != nil || r.ClaimHash != receipt.Body.ClaimHash || r.RequestID != c.RequestID {
			return ErrBoundary
		}
		name := "c-" + fileID(c.RequestID)
		var prior Receipt
		e := b.read(name, &prior)
		if e == nil {
			if bytes.Equal(canonical(prior), canonical(*receipt)) {
				reply("CONFIRMED_TEST_ONLY")
				return nil
			}
			reply("DENIED_CONFLICTING_PROOF")
			return nil
		}
		if !errors.Is(e, syscall.ENOENT) {
			return ErrBoundary
		}
		if b.writeExclusive(name, *receipt) != nil {
			return ErrBoundary
		}
		reply("CONFIRMED_TEST_ONLY")
		return nil
	default:
		reply("DENIED_ACTION")
		return nil
	}
}
func Call(ctx context.Context, path string, wire Wire) (string, error) {
	data, _ := json.Marshal(wire)
	if len(data) > maxFrame {
		return "", ErrBoundary
	}
	conn, e := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if e != nil {
		return "", e
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, e = conn.Write(append(data, '\n')); e != nil {
		return "", e
	}
	line, e := bufio.NewReader(conn).ReadString('\n')
	if e != nil {
		return "", e
	}
	return line[:len(line)-1], nil
}

// DebugEpoch exists for tests only; it never grants an effect.
func (b *Bridge) DebugEpoch() (uint64, error) {
	var n uint64
	e := b.locked(func() error { var v persistedEpoch; er := b.read("epoch.json", &v); n = v.Epoch; return er })
	return n, e
}
