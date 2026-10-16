package installationlicense

import (
	"context"
	"syscall"
	"time"
)

// administrativeLock bounds waiting for the same secure Linux installation
// state lock used by signed-V2 mutation and authority refresh operations.
// Once admitted, the original lock is held through the complete callback,
// including an in-flight Authority request. A busy lock returns ErrPending;
// callers MUST NOT blindly retry a request with an unknown remote outcome.
func (m *Manager) administrativeLock(ctx context.Context, fn func() error) error {
	return m.lockModeWithWait(ctx, syscall.LOCK_EX, 2*time.Second, ErrPending, fn)
}
