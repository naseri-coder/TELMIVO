package config

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func key(v byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{v}, 32))
}

func productionEnv() map[string]string {
	return map[string]string{
		"TELMIVO_ENV":          "production",
		"SYNTHETIC_MODE":       "false",
		"DATABASE_URL":         "postgresql://u:p@db.example/telmivo?sslmode=verify-full",
		"REDIS_URL":            "rediss://redis.example:6380/0",
		"NATS_URL":             "tls://nats.example:4222",
		"MINIAPP_URL":          "https://mini.example",
		"BOT_TOKEN":            "12345:abcdefghijklmnopqrstuvwxyzABCDE",
		"GROUP_BOT_TOKEN":      "23456:abcdefghijklmnopqrstuvwxyzABCDE",
		"DOWNLOADER_BOT_TOKEN": "34567:abcdefghijklmnopqrstuvwxyzABCDE",
		"WEBHOOK_SECRET":       strings.Repeat("w", 32),
		"WEBHOOK_DIGEST_KEY":   key(3),
		"CREDENTIAL_KEY_ID":    "active-v2",
		"CREDENTIAL_KEYRING":   "legacy-v1=" + key(1) + ";active-v2=" + key(2),
		"ADMIN_TELEGRAM_IDS":   "1",
		"MEDIA_STORAGE_ROOT":   "/var/lib/telmivo/media",
		"MEDIA_WORK_ROOT":      "/var/lib/telmivo/work",
		"OBJECT_STORAGE_ENDPOINT":   "https://objects.example",
		"OBJECT_STORAGE_BUCKET":     "telmivo-private",
		"OBJECT_STORAGE_REGION":     "eu-test-1",
		"OBJECT_STORAGE_ACCESS_KEY": "access-key",
		"OBJECT_STORAGE_SECRET_KEY": "secret-key",
	}
}

func TestProductionSecretsFailClosed(t *testing.T) {
	env := productionEnv()
	get := func(k string) string { return env[k] }
	c, e := Load(get)
	if e != nil {
		t.Fatal(e)
	}
	if c.CredentialKeyID != "active-v2" || len(c.CredentialKeys) != 2 || !bytes.Equal(c.WebhookDigest(), bytes.Repeat([]byte{3}, 32)) {
		t.Fatal("production key material was not parsed as expected")
	}
	for _, change := range []struct{ k, v string }{
		{"CREDENTIAL_KEYRING", "active-v2=invalid"},
		{"WEBHOOK_DIGEST_KEY", "not-base64-secret"},
		{"WEBHOOK_SECRET", "short"},
		{"MINIAPP_URL", "https://user:password@mini.example"},
		{"DATABASE_URL", "postgresql://u:p@db.example/telmivo?sslmode=disable"},
		{"REDIS_URL", "redis://redis.example:6379/0"},
		{"NATS_URL", "nats://nats.example:4222"},
		{"MEDIA_STORAGE_ROOT", "/tmp/media"},
		{"OBJECT_STORAGE_ENDPOINT", "http://objects.example"},
		{"OBJECT_STORAGE_SECRET_KEY", ""},
		{"SYNTHETIC_MODE", "true"},
	} {
		old := env[change.k]
		env[change.k] = change.v
		_, e := Load(get)
		if e == nil || (change.v != "" && strings.Contains(e.Error(), change.v)) {
			t.Fatal(change.k, e)
		}
		env[change.k] = old
	}
}

func TestDevelopmentLegacyCredentialCompatibility(t *testing.T) {
	env := map[string]string{
		"TELMIVO_ENV":    "development",
		"DATABASE_URL":   "postgres://u:p@127.0.0.1/db",
		"REDIS_URL":      "redis://127.0.0.1:6379",
		"NATS_URL":       "nats://127.0.0.1:4222",
		"MINIAPP_URL":    "http://127.0.0.1:5173",
		"CREDENTIAL_KEY": key(7),
	}
	c, e := Load(func(k string) string { return env[k] })
	if e != nil {
		t.Fatal(e)
	}
	if c.CredentialKeyID != "primary-v1" || !bytes.Equal(c.WebhookDigest(), bytes.Repeat([]byte{7}, 32)) {
		t.Fatal("legacy development key compatibility changed")
	}
}
