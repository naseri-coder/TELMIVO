package installationlicense

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	c "github.com/naseri-coder/TELMIVO/internal/platform/installationlicense/licensecrypto"
	"os"
	"path/filepath"
	"syscall"
)

func (cl *Client) Concurrency(ctx context.Context) error {
	var body []byte
	err := cl.manager.administrativeLock(ctx, func() error {
		i, s, h, e := cl.manager.load()
		if e != nil {
			return e
		}
		if s.Pending != nil {
			if s.Pending.Kind != "concurrency" {
				return ErrPending
			}
			body = append([]byte(nil), s.Pending.Body...)
			return nil
		}
		base := s
		if len(s.BaseEnvelope) > 0 {
			base.Envelope = s.BaseEnvelope
		}
		verified, e := cl.manager.verify(i, base, h)
		if e != nil {
			return e
		}
		x := verified.Claims
		id, e := UUID()
		if e != nil {
			return e
		}
		hash := sha256.Sum256(i.PublicKey())
		request := c.ConcurrencyRequest{ProtocolID: c.ConcurrentPolicy, LicenseID: x.LicenseID, InstallationID: i.ID, InstallationKeyHash: base64.RawURLEncoding.EncodeToString(hash[:]), ActivationID: x.ActivationID, AuthorityEpoch: x.AuthorityEpoch, LicenseRevision: x.LicenseRevision, ActivationGeneration: x.ActivationGeneration, RequestedGeneration: h.ConcurrencyGeneration, RecoveryGeneration: h.RecoveryGeneration, RuntimeSessionID: cl.manager.runtimeSession, RequestID: id}
		if h.ConcurrencyGeneration > 0 {
			lease := h.ConcurrencyLeaseID
			request.PreviousLeaseID = &lease
		}
		body, e = request.Canonical()
		return e
	})
	if err != nil {
		return err
	}
	return cl.mutate(ctx, "concurrency", body)
}

// A process lifetime leader keeps multiple bot/runtime roles sharing one state
// directory from independently refreshing. Followers still perform LIC-A8's
// immediate verified local effect recheck; this lock grants no authorization.
func (m *Manager) acquireRefreshLeader() (*os.File, error) {
	fd, e := syscall.Open(filepath.Join(m.cfg.Directory, "refresh-leader"), syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if e != nil {
		return nil, ErrState
	}
	f := os.NewFile(uintptr(fd), "refresh-leader")
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFREG || st.Nlink != 1 || st.Mode&0077 != 0 || int(st.Uid) != os.Geteuid() {
		f.Close()
		return nil, ErrState
	}
	if syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil, ErrPending
	}
	return f, nil
}
