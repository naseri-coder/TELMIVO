package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestVerifiedInitDataCanonicalReplayIdentity(t *testing.T) {
	now := time.Unix(1700000000, 0)
	token := "synthetic-bot-token"
	user := `{"id":123,"first_name":"A B"}`
	values := url.Values{"auth_date": {"1700000000"}, "user": {user}}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte("auth_date=1700000000\nuser=" + user))
	values.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	raw := values.Encode()
	first, err := InitDataDigest(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, alternate := range []string{raw, "user=" + url.QueryEscape(user) + "&hash=" + strings.ToUpper(values.Get("hash")) + "&auth_date=1700000000", strings.ReplaceAll(raw, "+", "%20")} {
		if _, err = ValidateInitData(alternate, token, now, time.Minute); err != nil {
			t.Fatal(err)
		}
		next, err := InitDataDigest(alternate)
		if err != nil || next != first {
			t.Fatal("equivalent signature has different replay identity", err)
		}
	}
	if _, err := InitDataDigest(raw + "&user=duplicate"); err == nil {
		t.Fatal("duplicate accepted")
	}
}
