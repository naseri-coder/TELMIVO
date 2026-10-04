package contract

import (
	"github.com/naseri-coder/TELMIVO/internal/domains/contentplugins"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	gate "github.com/naseri-coder/TELMIVO/internal/plugins/accessgate"
	files "github.com/naseri-coder/TELMIVO/internal/plugins/filemanager"
	"testing"
)

func TestR3D4PluginManifests(t *testing.T) {
	r, e := contentplugins.Registry()
	if e != nil {
		t.Fatal(e)
	}
	order, e := r.Resolve([]string{files.ID})
	if e != nil || len(order) != 1 || order[0] != files.ID {
		t.Fatal("optional gate forced", e)
	}
	m, e := r.Manifest(gate.ID)
	if e != nil || len(m.Permissions) != 1 || m.Permissions[0] != "telegram.read_member_status" {
		t.Fatal("overgrant")
	}
}
func TestR3D4FixtureGuard(t *testing.T) {
	for _, env := range []string{"production", "development", "test"} {
		v := map[string]string{"TELMIVO_ENV": env, "DATABASE_URL": "postgres://example@127.0.0.1/telmivo_test", "REDIS_URL": "redis://127.0.0.1:6379/15", "NATS_URL": "nats://127.0.0.1:4222", "SYNTHETIC_MODE": "true", "TEST_CONTENT_PLUGIN_FIXTURE": "true"}
		c, e := config.Load(func(k string) string { return v[k] })
		if env == "test" {
			if e != nil || !c.ContentTestFixture {
				t.Fatal(e)
			}
		} else if e == nil {
			t.Fatal("production fixture allowed")
		}
	}
}
