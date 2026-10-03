package config

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment, HTTPAddr, DatabaseURL, RedisURL, NATSURL, MiniAppURL      string
	BotToken, WebhookSecret, CredentialKey, StorageEndpoint, StorageBucket string
	GroupBotToken, DownloaderBotToken                                      string
	AdminIDs                                                               []int64
	Synthetic                                                              bool
	GroupTestAuthority                                                     bool
	DownloaderTestFixture                                                  bool
	SessionTTL, JobTimeout                                                 time.Duration
	MediaConcurrency, MaxMediaBytes                                        int64
}

func Load(get func(string) string) (Config, error) {
	c := Config{Environment: get("TELMIVO_ENV"), HTTPAddr: get("HTTP_ADDR"), DatabaseURL: get("DATABASE_URL"), RedisURL: get("REDIS_URL"), NATSURL: get("NATS_URL"), MiniAppURL: get("MINIAPP_URL"), BotToken: get("BOT_TOKEN"), GroupBotToken: get("GROUP_BOT_TOKEN"), DownloaderBotToken: get("DOWNLOADER_BOT_TOKEN"), WebhookSecret: get("WEBHOOK_SECRET"), CredentialKey: get("CREDENTIAL_KEY"), StorageEndpoint: get("S3_ENDPOINT"), StorageBucket: get("S3_BUCKET"), SessionTTL: time.Hour, JobTimeout: 5 * time.Minute, MediaConcurrency: 2, MaxMediaBytes: 49 << 20}
	if c.Environment == "" {
		c.Environment = "development"
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = ":8080"
	}
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return c, errors.New("invalid environment")
	}
	for _, v := range []struct{ value, scheme string }{{c.DatabaseURL, "postgres"}, {c.RedisURL, "redis"}, {c.NATSURL, "nats"}} {
		u, e := url.Parse(v.value)
		if e != nil || u.Host == "" || (u.Scheme != v.scheme && !(v.scheme == "postgres" && u.Scheme == "postgresql") && !(v.scheme == "redis" && u.Scheme == "rediss") && !(v.scheme == "nats" && u.Scheme == "tls")) {
			return c, errors.New("invalid infrastructure URL")
		}
	}
	if v := get("SYNTHETIC_MODE"); v != "" {
		var e error
		c.Synthetic, e = strconv.ParseBool(v)
		if e != nil {
			return c, errors.New("invalid synthetic mode")
		}
	}
	if v := get("TEST_GROUP_AUTHORITY_FIXTURE"); v != "" {
		var e error
		c.GroupTestAuthority, e = strconv.ParseBool(v)
		if e != nil || c.GroupTestAuthority && (c.Environment != "test" || !c.Synthetic) {
			return c, errors.New("group fixture requires explicit synthetic test mode")
		}
	}
	if v := get("TEST_DOWNLOADER_FIXTURE"); v != "" {
		var e error
		c.DownloaderTestFixture, e = strconv.ParseBool(v)
		if e != nil || c.DownloaderTestFixture && (c.Environment != "test" || !c.Synthetic) {
			return c, errors.New("downloader fixture requires synthetic test mode")
		}
	}
	if v := get("MAX_MEDIA_BYTES"); v != "" {
		n, e := strconv.ParseInt(v, 10, 64)
		if e != nil || n < 1024 || n > 49<<20 {
			return c, errors.New("invalid media safety cap")
		}
		c.MaxMediaBytes = n
	}
	for _, id := range strings.Split(get("ADMIN_TELEGRAM_IDS"), ",") {
		if id == "" {
			continue
		}
		v, e := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
		if e != nil || v <= 0 {
			return c, errors.New("invalid admin ID")
		}
		c.AdminIDs = append(c.AdminIDs, v)
	}
	if v := get("MEDIA_CONCURRENCY"); v != "" {
		n, e := strconv.ParseInt(v, 10, 64)
		if e != nil || n < 1 || n > 32 {
			return c, errors.New("invalid media concurrency")
		}
		c.MediaConcurrency = n
	}
	if c.Environment == "production" {
		if c.Synthetic {
			return c, errors.New("synthetic mode forbidden in production")
		}
		u, e := url.Parse(c.MiniAppURL)
		if e != nil || u.Scheme != "https" || u.Host == "" || c.WebhookSecret == "" || c.CredentialKey == "" {
			return c, errors.New("production secrets and HTTPS required")
		}
	}
	return c, nil
}
func FromEnv() (Config, error) { return Load(os.Getenv) }
