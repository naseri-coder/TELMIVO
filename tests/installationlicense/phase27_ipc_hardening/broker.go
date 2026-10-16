//go:build linux && amd64

// Package ipc is a NONPRODUCTION Linux test of authenticated, bounded IPC.
// It has no real Manager.Effect, license key, business operation or server.
package ipc

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const Protocol = "TELMIVO_LIC_A9_PHASE27_TEST_ONLY_V1"
const Operation = "SYNTHETIC_RECORD_EFFECT"
const maxFrame = 2048
const sysOpenat2 = 437 // Linux amd64 openat2; deliberately platform-restricted
const resolveNoMagiclinks = 0x02
const resolveNoSymlinks = 0x04
const resolveBeneath = 0x08

var hexID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var ErrDenied = errors.New("PHASE27_TEST_ONLY_DENIED")

type how struct {
	Flags   uint64
	Mode    uint64
	Resolve uint64
}

// openDirectory uses kernel-enforced fd-relative path resolution, not Lstat checks.
func openDirectory(path string) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return nil, ErrDenied
	}
	root, err := os.Open("/")
	if err != nil {
		return nil, ErrDenied
	}
	defer root.Close()
	relative := strings.TrimPrefix(path, "/")
	h := how{Flags: uint64(syscall.O_RDONLY | syscall.O_DIRECTORY | syscall.O_CLOEXEC), Resolve: resolveBeneath | resolveNoSymlinks | resolveNoMagiclinks}
	b := append([]byte(relative), 0)
	fd, _, errno := syscall.Syscall6(sysOpenat2, root.Fd(), uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&h)), unsafe.Sizeof(h), 0, 0)
	if errno != 0 {
		return nil, ErrDenied
	}
	f := os.NewFile(fd, "secure-root")
	s, e := f.Stat()
	if e != nil || !s.IsDir() || s.Mode().Perm() != 0700 {
		f.Close()
		return nil, ErrDenied
	}
	st, ok := s.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() {
		f.Close()
		return nil, ErrDenied
	}
	return f, nil
}

type Claim struct {
	Protocol    string `json:"protocol"`
	RequestID   string `json:"request_id"`
	WorkerID    string `json:"worker_id"`
	WorkspaceID string `json:"workspace_id"`
	Operation   string `json:"operation"`
	Nonce       string `json:"nonce"`
	IssuedAt    int64  `json:"issued_at"`
}
type Wire struct {
	Claim     Claim  `json:"claim"`
	Signature string `json:"signature"`
}
type Worker struct {
	UID       uint32
	Workspace string
	Key       ed25519.PublicKey
}
type Broker struct {
	listener *net.UnixListener
	owner    *os.File
	root     *os.File
	workers  map[string]Worker
	fn       func(Claim) error
	mu       sync.Mutex
	acceptMu sync.Mutex
	wg       sync.WaitGroup
	once     sync.Once
	stopped  chan struct{}
	complete chan struct{}
}

func Sign(c Claim, key ed25519.PrivateKey) Wire {
	b, _ := json.Marshal(c)
	return Wire{Claim: c, Signature: hex.EncodeToString(ed25519.Sign(key, b))}
}
func Open(root string, workers map[string]Worker, effect func(Claim) error) (*Broker, error) {
	if effect == nil || len(workers) == 0 {
		return nil, ErrDenied
	}
	dir, err := openDirectory(root)
	if err != nil {
		return nil, err
	}
	fd, e := syscall.Openat(int(dir.Fd()), "owner.lock", syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		dir.Close()
		return nil, ErrDenied
	}
	owner := os.NewFile(uintptr(fd), "owner.lock")
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Nlink != 1 || st.Mode&0077 != 0 || int(st.Uid) != os.Geteuid() || syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		owner.Close()
		dir.Close()
		return nil, ErrDenied
	}
	socket := filepath.Join(root, "broker.sock")
	if _, e := os.Lstat(socket); !os.IsNotExist(e) {
		owner.Close()
		dir.Close()
		return nil, ErrDenied
	}
	addr := &net.UnixAddr{Name: socket, Net: "unix"}
	listener, e := net.ListenUnix("unix", addr)
	if e != nil {
		owner.Close()
		dir.Close()
		return nil, ErrDenied
	}
	if os.Chmod(socket, 0600) != nil {
		listener.Close()
		owner.Close()
		dir.Close()
		return nil, ErrDenied
	}
	b := &Broker{listener: listener, owner: owner, root: dir, workers: workers, fn: effect, stopped: make(chan struct{}), complete: make(chan struct{})}
	return b, nil
}
func (b *Broker) Address() string { return b.listener.Addr().String() }
func (b *Broker) Close() {
	b.once.Do(func() {
		close(b.stopped)
		b.listener.Close()
		b.acceptMu.Lock()
		b.acceptMu.Unlock()
		b.wg.Wait()
		b.owner.Close()
		b.root.Close()
		close(b.complete)
	})
	<-b.complete
}
func (b *Broker) Serve() {
	for {
		c, e := b.listener.AcceptUnix()
		if e != nil {
			return
		}
		b.acceptMu.Lock()
		select {
		case <-b.stopped:
			b.acceptMu.Unlock()
			c.Close()
			return
		default:
		}
		b.wg.Add(1)
		b.acceptMu.Unlock()
		go func() { defer b.wg.Done(); defer c.Close(); b.handle(c) }()
	}
}
func peerUID(c *net.UnixConn) (uint32, error) {
	r, e := c.SyscallConn()
	if e != nil {
		return 0, e
	}
	var uid uint32
	var inner error
	e = r.Control(func(fd uintptr) {
		var u *syscall.Ucred
		u, inner = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if inner == nil {
			uid = u.Uid
		}
	})
	if e != nil {
		return 0, e
	}
	return uid, inner
}
func valid(raw []byte) (Wire, bool) {
	var w Wire
	if len(raw) == 0 || len(raw) > maxFrame || json.Unmarshal(raw, &w) != nil {
		return w, false
	}
	canonical, e := json.Marshal(w)
	if e != nil || !bytes.Equal(raw, canonical) {
		return w, false
	}
	c := w.Claim
	if c.Protocol != Protocol || c.Operation != Operation || !hexID.MatchString(c.RequestID) || !hexID.MatchString(c.Nonce) || c.WorkerID == "" || c.WorkspaceID == "" || len(w.Signature) != 128 {
		return w, false
	}
	return w, true
}
func answer(c *net.UnixConn, s string) { _, _ = c.Write([]byte(s + "\n")) }
func (b *Broker) handle(c *net.UnixConn) {
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	raw, e := bufio.NewReader(io.LimitReader(c, maxFrame+2)).ReadBytes('\n')
	if e != nil || len(raw) < 2 || len(raw) > maxFrame+1 {
		answer(c, "DENIED_FRAME")
		return
	}
	uid, e := peerUID(c)
	if e != nil {
		answer(c, "DENIED_PEER")
		return
	}
	w, ok := valid(raw[:len(raw)-1])
	if !ok {
		answer(c, "DENIED_REQUEST")
		return
	}
	enrol, ok := b.workers[w.Claim.WorkerID]
	if !ok || enrol.UID != uid || enrol.Workspace != w.Claim.WorkspaceID || len(enrol.Key) != ed25519.PublicKeySize {
		answer(c, "DENIED_IDENTITY")
		return
	}
	signed, _ := json.Marshal(w.Claim)
	sig, e := hex.DecodeString(w.Signature)
	if e != nil || !ed25519.Verify(enrol.Key, signed, sig) {
		answer(c, "DENIED_SIGNATURE")
		return
	}
	now := time.Now().Unix()
	if w.Claim.IssuedAt < now-30 || w.Claim.IssuedAt > now+5 {
		answer(c, "DENIED_TIME")
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.stopped:
		answer(c, "DENIED_STOPPED")
		return
	default:
	}
	// Both nonce and request ID must be persisted before any mock effect.
	for _, name := range []string{"id-" + w.Claim.RequestID, "nonce-" + w.Claim.Nonce} {
		fd, e := syscall.Openat(int(b.root.Fd()), name, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
		if e != nil {
			answer(c, "DENIED_REPLAY_OR_IO")
			return
		}
		f := os.NewFile(uintptr(fd), name)
		_, e = f.Write([]byte(w.Claim.WorkerID + "\n"))
		if e == nil {
			e = f.Sync()
		}
		if z := f.Close(); e == nil {
			e = z
		}
		if e != nil {
			answer(c, "DENIED_DURABILITY")
			return
		}
	}
	if b.root.Sync() != nil {
		answer(c, "DENIED_DURABILITY")
		return
	}
	if e := b.fn(w.Claim); e != nil {
		answer(c, "UNKNOWN_RECONCILE_REQUIRED")
		return
	}
	answer(c, "EXECUTED_TEST_ONLY")
}
func Call(ctx context.Context, address string, w Wire) (string, error) {
	raw, e := json.Marshal(w)
	if e != nil {
		return "", e
	}
	c, e := (&net.Dialer{}).DialContext(ctx, "unix", address)
	if e != nil {
		return "", e
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	_, e = c.Write(append(raw, '\n'))
	if e != nil {
		return "", e
	}
	result, e := bufio.NewReader(c).ReadString('\n')
	return strings.TrimSpace(result), e
}
