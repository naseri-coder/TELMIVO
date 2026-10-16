//go:build linux

// Package supervisor is a NONPRODUCTION Linux/Unix-socket security proof.
// A root-owned supervisor exclusively opens the synthetic Phase31 file witness;
// unprivileged Owner workers submit signed, bounded local commands only.
// It does not run a real TELMIVO Manager.Effect, signed Authority V2 lease,
// remote quorum, provider transaction, or any customer action.
package supervisor

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
	"sync/atomic"
	"syscall"
	"time"

	witness "telmivo.local/lic-a9-phase31-durable-witness"
)

const Protocol = "TELMIVO_PHASE32_TEST_RPC_V1"
const domain = "TELMIVO_PHASE32_TEST_REQUEST\x00"
const maxFrame = 2048
const ActionFence = "FENCE_SYNTHETIC_EPOCH"
const ActionApply = "APPLY_SYNTHETIC_RECORD"

var idRule = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,96}$`)
var ErrUnsafe = errors.New("PHASE32_TEST_ONLY_UNSAFE_SUPERVISOR")

// Claim is a synthetic, per-action issuer-approved request. This is not a
// production license grant, and no issuer private key is embedded in the repo.
type Claim struct {
	Protocol      string `json:"protocol"`
	Action        string `json:"action"`
	Installation  string `json:"installation"`
	Owner         string `json:"owner"`
	Session       string `json:"session"`
	Epoch         uint64 `json:"epoch"`
	PreviousEpoch uint64 `json:"previous_epoch"`
	RequestID     string `json:"request_id"`
	NotBefore     int64  `json:"not_before"`
	ExpiresAt     int64  `json:"expires_at"`
}
type Wire struct {
	Claim     Claim  `json:"claim"`
	Signature string `json:"signature"`
}

func SignedTestWire(c Claim, private ed25519.PrivateKey) Wire {
	b, _ := json.Marshal(c)
	payload := append([]byte(domain), b...)
	return Wire{Claim: c, Signature: hex.EncodeToString(ed25519.Sign(private, payload))}
}

type Server struct {
	listener     *net.UnixListener
	socketPath   string
	witness      *witness.Witness
	issuer       ed25519.PublicKey
	installation string
	allowedUID   uint32
	allowedGID   uint32
	readyEpoch   atomic.Uint64
	handlers     sync.WaitGroup
	mu           sync.Mutex
	closed       bool
}

// Open is deliberately restricted to an isolated root-owned Linux supervisor.
// socketDir must be 0750 root:authorized-GID, with a 0660 root:GID socket.
// witnessRoot must be a DIFFERENT root-owned 0700 directory. Both directory
// ancestors must be trusted by the deployment supervisor (NOT enforced here).
func Open(socketDir, witnessRoot, installation string, allowedUID, allowedGID uint32, issuer ed25519.PublicKey) (*Server, error) {
	if os.Geteuid() != 0 || !idRule.MatchString(installation) || len(issuer) != ed25519.PublicKeySize || socketDir == witnessRoot || !filepath.IsAbs(socketDir) || !filepath.IsAbs(witnessRoot) || filepath.Clean(socketDir) != socketDir || filepath.Clean(witnessRoot) != witnessRoot {
		return nil, ErrUnsafe
	}
	info, err := os.Lstat(socketDir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0750 || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafe
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || st.Gid != allowedGID {
		return nil, ErrUnsafe
	}
	w, err := witness.Open(witnessRoot)
	if err != nil {
		return nil, ErrUnsafe
	}
	name := filepath.Join(socketDir, "owner.sock")
	if _, err = os.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		w.Close()
		return nil, ErrUnsafe
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: name, Net: "unix"})
	if err != nil {
		w.Close()
		return nil, err
	}
	if err = os.Chown(name, 0, int(allowedGID)); err == nil {
		err = os.Chmod(name, 0660)
	}
	if err != nil {
		listener.Close()
		w.Close()
		return nil, ErrUnsafe
	}
	s := &Server{listener: listener, socketPath: name, witness: w, issuer: append(ed25519.PublicKey(nil), issuer...), installation: installation, allowedUID: allowedUID, allowedGID: allowedGID}
	// A new supervisor MUST authorize a new epoch before any Apply. A crash
	// cannot make a former Owner's signed request usable at prior epoch.
	s.readyEpoch.Store(0)
	return s, nil
}
func (s *Server) Address() string { return s.socketPath }
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.readyEpoch.Store(0)
	s.listener.Close()
	s.mu.Unlock()
	s.handlers.Wait()
	s.witness.Close()
	// Socket namespace is supervisor-owned; only the supervisor unlinks it.
	os.Remove(s.socketPath)
}
func (s *Server) Serve() {
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			conn.Close()
			return
		}
		s.handlers.Add(1)
		s.mu.Unlock()
		go func() { defer s.handlers.Done(); defer conn.Close(); s.handle(conn) }()
	}
}
func peerCred(c *net.UnixConn) (*syscall.Ucred, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return nil, err
	}
	var u *syscall.Ucred
	var inner error
	err = raw.Control(func(fd uintptr) { u, inner = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED) })
	if err != nil {
		return nil, err
	}
	return u, inner
}
func allowedWire(raw []byte, pub ed25519.PublicKey, installation string) (Claim, bool) {
	var w Wire
	if len(raw) < 2 || len(raw) > maxFrame || json.Unmarshal(raw, &w) != nil {
		return Claim{}, false
	}
	canonical, _ := json.Marshal(w)
	if !bytes.Equal(canonical, raw) {
		return Claim{}, false
	}
	c := w.Claim
	if c.Protocol != Protocol || c.Installation != installation || !idRule.MatchString(c.Owner) || !idRule.MatchString(c.Session) || c.Epoch == 0 || c.NotBefore > time.Now().Unix() || c.ExpiresAt <= time.Now().Unix() || c.ExpiresAt-c.NotBefore > 300 {
		return Claim{}, false
	}
	switch c.Action {
	case ActionFence:
		if c.PreviousEpoch == ^uint64(0) || c.PreviousEpoch+1 != c.Epoch || c.RequestID != "" {
			return Claim{}, false
		}
	case ActionApply:
		if !idRule.MatchString(c.RequestID) || c.PreviousEpoch >= c.Epoch {
			return Claim{}, false
		}
	default:
		return Claim{}, false
	}
	signature, err := hex.DecodeString(w.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return Claim{}, false
	}
	b, _ := json.Marshal(c)
	return c, ed25519.Verify(pub, append([]byte(domain), b...), signature)
}
func reply(c *net.UnixConn, status string) { _, _ = c.Write([]byte(status + "\n")) }
func (s *Server) handle(conn *net.UnixConn) {
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	cred, err := peerCred(conn)
	if err != nil || cred.Uid != s.allowedUID {
		reply(conn, "DENIED_PEER")
		return
	}
	raw, err := bufio.NewReader(io.LimitReader(conn, maxFrame+2)).ReadBytes('\n')
	if err != nil || len(raw) < 2 || len(raw) > maxFrame+1 {
		reply(conn, "DENIED_FRAME")
		return
	}
	c, valid := allowedWire(raw[:len(raw)-1], s.issuer, s.installation)
	if !valid {
		reply(conn, "DENIED_SIGNATURE_OR_REQUEST")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// No callback or unsafe external effect is accepted from client JSON.
	if c.Action == ActionFence {
		err = s.witness.Fence(ctx, c.Epoch)
		if err != nil {
			reply(conn, "DENIED_FENCE")
			return
		}
		s.readyEpoch.Store(c.Epoch)
		reply(conn, "FENCED_TEST_ONLY")
		return
	}
	if s.readyEpoch.Load() != c.Epoch {
		reply(conn, "DENIED_NOT_READY_OR_STALE")
		return
	}
	err = s.witness.Apply(ctx, c.Epoch, c.RequestID)
	if err != nil {
		reply(conn, "DENIED_STALE_REPLAY_OR_IO")
		return
	}
	reply(conn, "APPLIED_SYNTHETIC_TEST_ONLY")
}
func Call(ctx context.Context, address string, w Wire) (string, error) {
	raw, err := json.Marshal(w)
	if err != nil {
		return "", err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", address)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
	if _, err = conn.Write(append(raw, '\n')); err != nil {
		return "", err
	}
	result, err := bufio.NewReader(conn).ReadString('\n')
	return strings.TrimSpace(result), err
}
