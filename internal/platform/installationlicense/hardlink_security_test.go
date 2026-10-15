package installationlicense

import (
    "context"
    "errors"
    "os"
    "path/filepath"
    "testing"
)

// A same-UID process must not be able to present a second pathname for
// persisted identity, lease state or monotonic high-water. These fixtures
// contain only synthetic TEST installation data, never customer credentials.
func TestLICA9Phase03HardlinkedLicenseFilesFailClosed(t *testing.T) {
    for _, name := range []string{"identity.json", "state.json", "highwater.json", "lock"} {
        t.Run(name, func(t *testing.T) {
            m, token := fixture(t)
            if e := m.Accept(token(1, 1, 1)); e != nil {
                t.Fatal("synthetic signed baseline", e)
            }
            if d := m.Decision(); d.State != "VALID_ONLINE" {
                t.Fatal("fixture not admitted before attack", d)
            }
            source := filepath.Join(m.cfg.Directory, name)
            alias := filepath.Join(t.TempDir(), "hardlinked-alias")
            if e := os.Link(source, alias); e != nil {
                t.Fatal("hardlink attack fixture could not be created", e)
            }
            if d := m.Decision(); d.State != "RESTRICTED" {
                t.Fatal("hardlinked licensing file did not restrict", name, d)
            }
            executed := false
            if e := m.Effect(context.Background(), func() error {
                executed = true
                return nil
            }); !errors.Is(e, ErrRestricted) || executed {
                t.Fatal("effect escaped hardlink denial", name, e)
            }
            if _, e := Open(m.cfg); e == nil && (name == "identity.json" || name == "lock") {
                t.Fatal("Open accepted a hardlinked identity or lock")
            }
        })
    }
}

func TestLICA9Phase03NewClientMissingManagerFailsClosed(t *testing.T) {
    client, err := NewClient(nil)
    if client != nil || !errors.Is(err, ErrState) {
        t.Fatal("missing licensing manager did not return explicit state error", err)
    }
}

func TestLICA9Phase03HardlinkedRefreshLeaderFailsClosed(t *testing.T) {
    m, _ := fixture(t)
    first, e := m.acquireRefreshLeader()
    if e != nil {
        t.Fatal("baseline leader not acquired", e)
    }
    if e = first.Close(); e != nil {
        t.Fatal(e)
    }
    source := filepath.Join(m.cfg.Directory, "refresh-leader")
    alias := filepath.Join(t.TempDir(), "aliased-refresh-leader")
    if e := os.Link(source, alias); e != nil {
        t.Fatal("failed to construct same-filesystem hardlink fixture", e)
    }
    leader, e := m.acquireRefreshLeader()
    if leader != nil {
        leader.Close()
    }
    if !errors.Is(e, ErrState) || leader != nil {
        t.Fatal("hardlinked refresh-leader became authoritative", e)
    }
}
