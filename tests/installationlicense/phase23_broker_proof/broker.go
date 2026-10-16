//go:build linux

// Package brokerproof is a NONPRODUCTION demonstration of a single effect-owning
// process. It is not a license verifier, production broker, or authentication API.
// No real software operations are registered; requests are TEST ONLY.
package brokerproof

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"time"
)

const protocol = "TELMIVO_LIC_A9_PHASE23_TEST_ONLY_V1"
const operation = "SYNTHETIC_RECORD_EFFECT"
const maxFrame = 512

var requestIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var ErrDenied = errors.New("PHASE23_DENIED")

type Request struct {
	Protocol  string `json:"protocol"`
	Operation string `json:"operation"`
	ID        string `json:"id"`
}
type Response struct {
	Status string `json:"status"`
}

// EffectGate models the trusted leader's immediate license/RBAC recheck. The
// callback is never invoked directly by the client. A real deployment requires
// the real Manager.Effect and authenticated domain authorization, not this stub.
type EffectGate func(context.Context, func() error) error

type Broker struct {
	listener  *net.UnixListener
	journal   string
	owner     *os.File // exclusive lifetime flock; socket name is not an ownership proof
	mu        sync.Mutex
	acceptMu  sync.Mutex // barrier: never Wait before last handler registration
	gate      EffectGate
	effect    func() error
	peerUID   func(*net.UnixConn) (uint32, error)
	cancel    chan struct{}
	once      sync.Once
	closeDone chan struct{}
	closeErr  error
	wg        sync.WaitGroup
}

func sameUID(conn *net.UnixConn) (uint32, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid uint32
	var errInside error
	err = raw.Control(func(fd uintptr) {
		var c *syscall.Ucred
		c, errInside = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if errInside == nil {
			uid = c.Uid
		}
	})
	if err != nil {
		return 0, err
	}
	return uid, errInside
}

// Open only admits a new isolated 0700 owned temporary root. It does not
// create a socket within a shared or production installation state directory.
func Open(root string, gate EffectGate, effect func() error) (*Broker, error) {
	if gate == nil || effect == nil || !filepath.IsAbs(root) {
		return nil, ErrDenied
	}
	// Reject a symlink anywhere in the path, not just at the leaf root.
	// This demonstration intentionally does not implement openat2-based
	// race-free resolution; it is not safe for an adversarial shared UID.
	if filepath.Clean(root) != root {
		return nil, ErrDenied
	}
	for dir := root; dir != "/"; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrDenied
		}
	}
	st, err := os.Lstat(root)
	if err != nil || !st.IsDir() || st.Mode().Perm() != 0700 || st.Mode()&os.ModeSymlink != 0 {
		return nil, ErrDenied
	}
	raw, ok := st.Sys().(*syscall.Stat_t)
	if !ok || int(raw.Uid) != os.Geteuid() {
		return nil, ErrDenied
	}
	// Hold one kernel flock for the entire owner lifetime. Removing the
	// pathname of the live Unix listener must NOT permit a second owner.
	fd, e := syscall.Open(filepath.Join(root, "broker-owner"),
		syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return nil, ErrDenied
	}
	owner := os.NewFile(uintptr(fd), "broker-owner")
	var ownerStat syscall.Stat_t
	if syscall.Fstat(fd, &ownerStat) != nil ||
		ownerStat.Mode&syscall.S_IFMT != syscall.S_IFREG ||
		ownerStat.Nlink != 1 || ownerStat.Mode&0077 != 0 ||
		int(ownerStat.Uid) != os.Geteuid() ||
		syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		owner.Close()
		return nil, ErrDenied
	}
	// All unsuccessful Open paths release the exclusive lock.
	accepted := false
	defer func() {
		if !accepted {
			_ = owner.Close()
		}
	}()
	journal := filepath.Join(root, "journal")
	if err = os.Mkdir(journal, 0700); err != nil && !os.IsExist(err) {
		return nil, ErrDenied
	}
	js, je := os.Lstat(journal)
	if je != nil || !js.IsDir() || js.Mode().Perm() != 0700 || js.Mode()&os.ModeSymlink != 0 {
		return nil, ErrDenied
	}
	jraw, jok := js.Sys().(*syscall.Stat_t)
	if !jok || int(jraw.Uid) != os.Geteuid() {
		return nil, ErrDenied
	}
	socket := filepath.Join(root, "broker.sock")
	if len(socket) > 100 {
		return nil, ErrDenied
	}
	addr := &net.UnixAddr{Name: socket, Net: "unix"}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(socket, 0600); err != nil {
		listener.Close()
		return nil, ErrDenied
	}
	accepted = true
	return &Broker{listener: listener, journal: journal, owner: owner, gate: gate, effect: effect, peerUID: sameUID, cancel: make(chan struct{}), closeDone: make(chan struct{})}, nil
}

func (b *Broker) Address() string { return b.listener.Addr().String() }
func (b *Broker) Close() error {
	b.once.Do(func() {
		close(b.cancel)
		b.closeErr = b.listener.Close()
		b.acceptMu.Lock()
		b.acceptMu.Unlock()
		b.wg.Wait()
		_ = b.owner.Close() // release lifetime ownership AFTER all admitted handlers end
		close(b.closeDone)
	})
	<-b.closeDone
	return b.closeErr
}
func (b *Broker) Serve() {
	for {
		conn, err := b.listener.AcceptUnix()
		if err != nil {
			return
		}
		b.acceptMu.Lock()
		select {
		case <-b.cancel:
			b.acceptMu.Unlock()
			_ = conn.Close()
			return
		default:
		}
		b.wg.Add(1)
		b.acceptMu.Unlock()
		go func() { defer b.wg.Done(); defer conn.Close(); b.handle(conn) }()
	}
}
func valid(frame []byte) (Request, bool) {
	var r Request
	if len(frame) == 0 || len(frame) > maxFrame || json.Unmarshal(frame, &r) != nil {
		return r, false
	}
	canonical, err := json.Marshal(r)
	if err != nil || !bytes.Equal(frame, canonical) || r.Protocol != protocol || r.Operation != operation || !requestIDPattern.MatchString(r.ID) {
		return r, false
	}
	return r, true
}
func send(conn *net.UnixConn, status string) {
	raw, _ := json.Marshal(Response{Status: status})
	_, _ = conn.Write(append(raw, '\n'))
}
func (b *Broker) handle(conn *net.UnixConn) {
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	raw, err := bufio.NewReader(io.LimitReader(conn, maxFrame+2)).ReadBytes('\n')
	if err != nil || len(raw) < 2 || len(raw) > maxFrame+1 || raw[len(raw)-1] != '\n' {
		send(conn, "DENIED_FRAME")
		return
	}
	uid, err := b.peerUID(conn)
	if err != nil || uid != uint32(os.Geteuid()) {
		send(conn, "DENIED_PEER")
		return
	}
	r, ok := valid(raw[:len(raw)-1])
	if !ok {
		send(conn, "DENIED_REQUEST")
		return
	}
	select {
	case <-b.cancel:
		send(conn, "DENIED_CLOSED")
		return
	default:
	}
	// Durable at-most-once reservation BEFORE the effect. Ambiguous outcomes
	// cannot be retried without an externally reviewed reconciliation process.
	// The journal is not a full crash-safe distributed exactly-once protocol.
	b.mu.Lock()
	defer b.mu.Unlock()
	// A queued request must recheck cancellation after acquiring the
	// serialization lock. Checking only before Lock allowed a pre-shutdown
	// request to execute AFTER cancellation was already signalled.
	select {
	case <-b.cancel:
		send(conn, "DENIED_CLOSED")
		return
	default:
	}
	hash := sha256.Sum256([]byte(r.ID))
	path := filepath.Join(b.journal, hex.EncodeToString(hash[:]))
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		send(conn, "DENIED_DUPLICATE_OR_IO")
		return
	}
	file := os.NewFile(uintptr(fd), "reserved-intent")
	_, e := file.Write([]byte(protocol + ":" + r.ID + "\n"))
	if e == nil {
		e = file.Sync()
	}
	if c := file.Close(); e == nil {
		e = c
	}
	dir, e2 := os.Open(b.journal)
	if e2 == nil {
		if c := dir.Sync(); e2 == nil {
			e2 = c
		}
		_ = dir.Close()
	}
	if e != nil || e2 != nil {
		send(conn, "DENIED_DURABILITY")
		return
	}
	err = b.gate(context.Background(), b.effect)
	if err != nil {
		send(conn, "DENIED_LICENSE_OR_EFFECT")
		return
	}
	send(conn, "EXECUTED_TEST_ONLY")
}

// Call is a local test client only. It has no arbitrary command/path payload.
func Call(socket string, r Request) (string, error) {
	if !filepath.IsAbs(socket) {
		return "", ErrDenied
	}
	payload, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	c, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = c.Write(append(payload, '\n')); err != nil {
		return "", err
	}
	raw, err := io.ReadAll(io.LimitReader(c, 512))
	if err != nil {
		return "", err
	}
	var out Response
	if err = json.Unmarshal(bytes.TrimSpace(raw), &out); err != nil {
		return "", err
	}
	if out.Status == "" {
		return "", fmt.Errorf("missing response status")
	}
	return out.Status, nil
}
