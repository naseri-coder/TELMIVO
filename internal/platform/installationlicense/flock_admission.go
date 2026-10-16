package installationlicense

import (
	"context"
	"errors"
	"syscall"
	"time"
)

// acquireExclusiveFlock is the bounded admission path for the real Manager's
// externally exposed Effect and Accept entrypoints. It NEVER weakens the
// file-lock hold during the protected callback or signed V2 state updates.
// Callers must validate the opened lock descriptor's owner/mode/link count
// before invoking this helper and must unlock the acquired descriptor after fn.
func acquireExclusiveFlock(ctx context.Context, fd int, maxWait time.Duration, busy error) error {
	if ctx == nil || maxWait <= 0 || busy == nil {
		return ErrState
	}
	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	retry := time.NewTicker(10 * time.Millisecond)
	defer retry.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EINTR) {
			return ErrState
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return busy
		case <-retry.C:
		}
	}
}
