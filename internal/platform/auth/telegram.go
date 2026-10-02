package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid Telegram identity")

type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

func ValidateInitData(raw, token string, now time.Time, maxAge time.Duration) (TelegramUser, error) {
	var user TelegramUser
	if token == "" || len(raw) > 16384 || maxAge <= 0 {
		return user, ErrInvalid
	}
	values, e := url.ParseQuery(raw)
	if e != nil {
		return user, ErrInvalid
	}
	for _, vs := range values {
		if len(vs) != 1 {
			return user, ErrInvalid
		}
	}
	hash, e := hex.DecodeString(values.Get("hash"))
	if e != nil || len(hash) != 32 {
		return user, ErrInvalid
	}
	values.Del("hash")
	// Bot-token HMAC includes all received fields except hash (including signature).

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+"="+values.Get(k))
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	if !hmac.Equal(hash, mac.Sum(nil)) {
		return user, ErrInvalid
	}
	stamp, e := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if e != nil {
		return user, ErrInvalid
	}
	date := time.Unix(stamp, 0)
	if date.After(now.Add(30*time.Second)) || now.Sub(date) > maxAge {
		return user, ErrInvalid
	}
	if e = json.Unmarshal([]byte(values.Get("user")), &user); e != nil || user.ID <= 0 || len(user.FirstName) > 256 || len(user.Username) > 64 {
		return user, ErrInvalid
	}
	return user, nil
}
func Token() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func Digest(token string) string { h := sha256.Sum256([]byte(token)); return hex.EncodeToString(h[:]) }
