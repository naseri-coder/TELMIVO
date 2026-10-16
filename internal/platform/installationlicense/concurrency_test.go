package installationlicense

import (
	"errors"
	"testing"
)

func TestLICA9BRefreshLeaderOwnershipAndRelease(t *testing.T) {
	m, _ := fixture(t)
	other, e := Open(m.cfg)
	if e != nil {
		t.Fatal(e)
	}
	if m.runtimeSession == other.runtimeSession {
		t.Fatal("restart reused runtime session")
	}
	first, e := m.acquireRefreshLeader()
	if e != nil {
		t.Fatal(e)
	}
	if second, e := other.acquireRefreshLeader(); !errors.Is(e, ErrPending) || second != nil {
		t.Fatal("two refresh leaders", e)
	}
	if e = first.Close(); e != nil {
		t.Fatal(e)
	}
	second, e := other.acquireRefreshLeader()
	if e != nil {
		t.Fatal("leader takeover after release", e)
	}
	second.Close()
}
