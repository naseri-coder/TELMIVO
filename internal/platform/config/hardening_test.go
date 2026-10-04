package config

import (
	"strings"
	"testing"
)

func TestProductionSecretsFailClosed(t *testing.T) {
	env := map[string]string{"TELMIVO_ENV": "production", "DATABASE_URL": "postgres://u:p@127.0.0.1/db", "REDIS_URL": "redis://127.0.0.1:6379", "NATS_URL": "nats://127.0.0.1:4222", "MINIAPP_URL": "https://mini.example", "CREDENTIAL_KEY": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", "WEBHOOK_SECRET": strings.Repeat("x", 32)}
	get := func(k string) string { return env[k] }
	if _, e := Load(get); e != nil {
		t.Fatal(e)
	}
	for _, change := range []struct{ k, v string }{{"CREDENTIAL_KEY", "not-base64-secret"}, {"WEBHOOK_SECRET", "short"}, {"MINIAPP_URL", "https://user:password@mini.example"}} {
		old := env[change.k]
		env[change.k] = change.v
		_, e := Load(get)
		if e == nil || strings.Contains(e.Error(), change.v) {
			t.Fatal(change.k, e)
		}
		env[change.k] = old
	}
}
