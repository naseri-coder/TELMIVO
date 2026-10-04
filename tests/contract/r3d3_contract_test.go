package contract

import (
	"encoding/json"
	"fmt"
	"github.com/naseri-coder/TELMIVO/internal/domains/botfactory"
	"github.com/naseri-coder/TELMIVO/internal/platform/config"
	"strings"
	"testing"
)

func TestR3D3CredentialSafeRequest(t *testing.T) {
	in := botfactory.Create{Name: "Synthetic", Token: "12345:" + strings.Repeat("x", 35), Key: "request-1"}
	if !botfactory.ValidInput(in) {
		t.Fatal("valid input rejected")
	}
	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		if strings.Contains(fmt.Sprintf(format, in), in.Token) {
			t.Fatal("log representation exposed credential")
		}
	}
	b, _ := json.Marshal(botfactory.Bot{ID: "example", Name: "Synthetic", Status: "DRAFT"})
	if strings.Contains(string(b), "token") || strings.Contains(string(b), "key_id") {
		t.Fatal("response contains credential fields")
	}
}
func TestR3D3FixtureProductionGuard(t *testing.T) {
	for _, env := range []string{"production", "development", "test"} {
		values := map[string]string{"TELMIVO_ENV": env, "DATABASE_URL": "postgres://example@127.0.0.1/telmivo_test", "REDIS_URL": "redis://127.0.0.1:6379/15", "NATS_URL": "nats://127.0.0.1:4222", "SYNTHETIC_MODE": "true", "TEST_BOT_FACTORY_FIXTURE": "true"}
		c, e := config.Load(func(k string) string { return values[k] })
		if env == "test" {
			if e != nil || !c.BotFactoryTestFixture {
				t.Fatal(e)
			}
		} else if e == nil {
			t.Fatal("fixture allowed outside isolated test", env)
		}
	}
}
