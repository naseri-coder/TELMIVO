package config

import (
	"encoding/base64"
	"errors"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TrustedProxyCIDRs                                                 []netip.Prefix
	Environment, HTTPAddr, DatabaseURL, RedisURL, NATSURL, MiniAppURL string
	BotToken, WebhookSecret, CredentialKey, CredentialKeyID           string
	WebhookDigestKey, StorageEndpoint, StorageBucket                  string
	StorageRegion, StorageAccessKey, StorageSecretKey                 string
	GroupBotToken, DownloaderBotToken                                 string
	MediaStorageRoot, MediaWorkRoot, UploadStorageRoot                string
	CredentialKeys                                                    map[string][]byte
	AdminIDs                                                          []int64
	Synthetic                                                         bool
	GroupTestAuthority                                                bool
	DownloaderTestFixture                                             bool
	BotFactoryTestFixture                                             bool
	ContentTestFixture                                                bool
	SessionTTL, JobTimeout                                            time.Duration
	MediaConcurrency, MaxMediaBytes                                   int64
}

func validKeyID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || (i > 0 && (r == '.' || r == '_' || r == '-')) {
			continue
		}
		return false
	}
	return true
}

func decode32(raw string) ([]byte, error) {
	b, e := base64.StdEncoding.DecodeString(raw)
	if e != nil || len(b) != 32 {
		return nil, errors.New("invalid 256-bit key")
	}
	return b, nil
}

func parseKeyring(raw string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, item := range strings.Split(raw, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || !validKeyID(parts[0]) || out[parts[0]] != nil {
			return nil, errors.New("invalid credential keyring")
		}
		key, e := decode32(parts[1])
		if e != nil {
			return nil, errors.New("invalid credential keyring")
		}
		out[parts[0]] = key
	}
	if len(out) == 0 {
		return nil, errors.New("invalid credential keyring")
	}
	return out, nil
}

func secureDatabaseURL(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil {
		return false
	}
	switch u.Query().Get("sslmode") {
	case "require", "verify-ca", "verify-full":
		return true
	}
	return false
}

func productionPath(raw string) bool {
	if !filepath.IsAbs(raw) {
		return false
	}
	clean := filepath.Clean(raw)
	return clean != "/" && clean != "/tmp" && !strings.HasPrefix(clean, "/tmp/")
}

// WebhookDigest returns a copy of the stable customer-webhook HMAC key.
// Development/test retain the historical active-credential fallback; production
// requires an independent WEBHOOK_DIGEST_KEY so credential rotation does not
// silently invalidate customer bot webhook secrets.
func (c Config) WebhookDigest() []byte {
	if c.WebhookDigestKey != "" {
		if b, e := decode32(c.WebhookDigestKey); e == nil {
			return append([]byte(nil), b...)
		}
	}
	if b := c.CredentialKeys[c.CredentialKeyID]; len(b) == 32 {
		return append([]byte(nil), b...)
	}
	return nil
}

func first(get func(string) string, keys ...string) string {
	for _, key := range keys {
		if value := get(key); value != "" {
			return value
		}
	}
	return ""
}

func Load(get func(string) string) (Config, error) {
	c := Config{
		Environment:        get("TELMIVO_ENV"),
		HTTPAddr:           get("HTTP_ADDR"),
		DatabaseURL:        get("DATABASE_URL"),
		RedisURL:           get("REDIS_URL"),
		NATSURL:            get("NATS_URL"),
		MiniAppURL:         get("MINIAPP_URL"),
		BotToken:           get("BOT_TOKEN"),
		GroupBotToken:      get("GROUP_BOT_TOKEN"),
		DownloaderBotToken: get("DOWNLOADER_BOT_TOKEN"),
		WebhookSecret:      get("WEBHOOK_SECRET"),
		CredentialKey:      get("CREDENTIAL_KEY"),
		CredentialKeyID:    get("CREDENTIAL_KEY_ID"),
		WebhookDigestKey:   get("WEBHOOK_DIGEST_KEY"),
		StorageEndpoint:    first(get, "OBJECT_STORAGE_ENDPOINT", "S3_ENDPOINT"),
		StorageBucket:      first(get, "OBJECT_STORAGE_BUCKET", "S3_BUCKET"),
		StorageRegion:      first(get, "OBJECT_STORAGE_REGION", "S3_REGION"),
		StorageAccessKey:   first(get, "OBJECT_STORAGE_ACCESS_KEY", "S3_ACCESS_KEY"),
		StorageSecretKey:   first(get, "OBJECT_STORAGE_SECRET_KEY", "S3_SECRET_KEY"),
		MediaStorageRoot:   get("MEDIA_STORAGE_ROOT"),
		MediaWorkRoot:      get("MEDIA_WORK_ROOT"),
		UploadStorageRoot:  get("UPLOAD_STORAGE_ROOT"),
		SessionTTL:         time.Hour,
		JobTimeout:         5 * time.Minute,
		MediaConcurrency:   2,
		MaxMediaBytes:      49 << 20,
	}
	for _, raw := range strings.Split(get("TRUSTED_PROXY_CIDRS"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || prefix.Bits() == 0 {
			return c, errors.New("invalid trusted proxy CIDR")
		}
		c.TrustedProxyCIDRs = append(c.TrustedProxyCIDRs, prefix.Masked())
	}
	if c.Environment == "" {
		c.Environment = "development"
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = ":8080"
	}
	if c.MediaStorageRoot == "" {
		c.MediaStorageRoot = "/tmp/telmivo-downloader-storage"
	}
	if c.MediaWorkRoot == "" {
		c.MediaWorkRoot = "/tmp/telmivo-downloader-work"
	}
	if c.UploadStorageRoot == "" {
		c.UploadStorageRoot = "/tmp/telmivo-uploader-storage"
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
	if v := get("TEST_CONTENT_PLUGIN_FIXTURE"); v != "" {
		value, e := strconv.ParseBool(v)
		c.ContentTestFixture = value
		if e != nil || value && (c.Environment != "test" || !c.Synthetic) {
			return c, errors.New("invalid content plugin test fixture")
		}
	}
	if v := get("TEST_BOT_FACTORY_FIXTURE"); v != "" {
		value, eFactory := strconv.ParseBool(v)
		c.BotFactoryTestFixture = value
		if eFactory != nil || c.BotFactoryTestFixture && (c.Environment != "test" || !c.Synthetic) {
			return c, errors.New("invalid bot factory test fixture")
		}
	}
	if v := get("MEDIA_CONCURRENCY"); v != "" {
		n, e := strconv.ParseInt(v, 10, 64)
		if e != nil || n < 1 || n > 32 {
			return c, errors.New("invalid media concurrency")
		}
		c.MediaConcurrency = n
	}

	ringRaw := get("CREDENTIAL_KEYRING")
	if ringRaw != "" && c.CredentialKey != "" {
		return c, errors.New("ambiguous credential key configuration")
	}
	if ringRaw != "" {
		keys, e := parseKeyring(ringRaw)
		if e != nil || !validKeyID(c.CredentialKeyID) || keys[c.CredentialKeyID] == nil {
			return c, errors.New("invalid credential keyring")
		}
		c.CredentialKeys = keys
	} else if c.CredentialKey != "" {
		key, e := decode32(c.CredentialKey)
		if e != nil {
			return c, errors.New("invalid credential key")
		}
		if c.CredentialKeyID == "" {
			c.CredentialKeyID = "primary-v1"
		}
		if !validKeyID(c.CredentialKeyID) {
			return c, errors.New("invalid credential key ID")
		}
		c.CredentialKeys = map[string][]byte{c.CredentialKeyID: key}
	} else {
		c.CredentialKeys = map[string][]byte{}
	}
	if c.WebhookDigestKey != "" {
		if _, e := decode32(c.WebhookDigestKey); e != nil {
			return c, errors.New("invalid webhook digest key")
		}
	}

	if c.Environment == "production" {
		if c.Synthetic {
			return c, errors.New("synthetic mode forbidden in production")
		}
		mini, e := url.Parse(c.MiniAppURL)
		redis, redisErr := url.Parse(c.RedisURL)
		nats, natsErr := url.Parse(c.NATSURL)
		if e != nil || mini.Scheme != "https" || mini.Host == "" || mini.User != nil || mini.RawQuery != "" || mini.Fragment != "" ||
			redisErr != nil || redis.Scheme != "rediss" || natsErr != nil || nats.Scheme != "tls" || !secureDatabaseURL(c.DatabaseURL) {
			return c, errors.New("production transport security required")
		}
		if len(c.WebhookSecret) < 32 || c.BotToken == "" || c.GroupBotToken == "" || c.DownloaderBotToken == "" || len(c.AdminIDs) == 0 {
			return c, errors.New("production bot credentials required")
		}
		if ringRaw == "" || c.CredentialKeyID == "" || c.CredentialKeys[c.CredentialKeyID] == nil || c.WebhookDigestKey == "" {
			return c, errors.New("production credential keyring required")
		}
		if !productionPath(c.MediaStorageRoot) || !productionPath(c.MediaWorkRoot) {
			return c, errors.New("production media paths must be persistent")
		}
		storageURL, storageErr := url.Parse(c.StorageEndpoint)
		if storageErr != nil || storageURL.Scheme != "https" || storageURL.Host == "" || storageURL.User != nil || storageURL.RawQuery != "" || storageURL.Fragment != "" ||
			c.StorageBucket == "" || len(c.StorageBucket) > 63 || c.StorageRegion == "" || c.StorageAccessKey == "" || c.StorageSecretKey == "" {
			return c, errors.New("production private object storage required")
		}
	}
	return c, nil
}

func FromEnv() (Config, error) { return Load(os.Getenv) }
