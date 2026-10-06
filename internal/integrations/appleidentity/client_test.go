package appleidentity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/naseri-coder/TELMIVO/internal/domains/appleservices"
)

func testECPEM(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return k, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw})
}

func jwtPart(v any) string {
	raw, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func signRS256(key *rsa.PrivateKey, kid string, claims map[string]any) (string, error) {
	head := jwtPart(map[string]any{"alg": "RS256", "kid": kid, "typ": "JWT"})
	body := jwtPart(claims)
	unsigned := head + "." + body
	h := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func jwkN(k *rsa.PublicKey) string { return base64.RawURLEncoding.EncodeToString(k.N.Bytes()) }
func jwkE(k *rsa.PublicKey) string {
	e := big.NewInt(int64(k.E)).Bytes()
	return base64.RawURLEncoding.EncodeToString(e)
}

func verifyClientSecret(raw string, public *ecdsa.PublicKey, kid, team, client string) error {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return errors.New("bad client secret")
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}
	var head struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if json.Unmarshal(hb, &head) != nil || head.Alg != "ES256" || head.Kid != kid {
		return errors.New("client secret header")
	}
	cb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	var claims struct {
		Iss string `json:"iss"`
		Sub string `json:"sub"`
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Iat int64  `json:"iat"`
	}
	if json.Unmarshal(cb, &claims) != nil || claims.Iss != team || claims.Sub != client || claims.Aud != appleIssuer || claims.Exp <= claims.Iat {
		return errors.New("client secret claims")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return errors.New("client secret signature")
	}
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(public, h[:], r, s) {
		return errors.New("client secret signature invalid")
	}
	return nil
}

func TestClientAuthorizationExchangeAndClassification(t *testing.T) {
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	clientKey, pemKey := testECPEM(t)
	appleKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := "apple-test-key"
	mode := "ok"
	wantNonce := "nonce-not-set"
	handlerErrors := make(chan error, 8)
	report := func(w http.ResponseWriter, err error) {
		select {
		case handlerErrors <- err:
		default:
		}
		http.Error(w, "synthetic handler failure", http.StatusInternalServerError)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			if mode == "server" {
				http.Error(w, "down", 500)
				return
			}
			if mode == "reject" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
				return
			}
			if err := r.ParseForm(); err != nil {
				report(w, err)
				return
			}
			if r.Form.Get("client_id") != "com.example.web" || r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("redirect_uri") != "https://example.test/apple/callback" || r.Form.Get("code") != "one-time-code" {
				report(w, errors.New("token form"))
				return
			}
			if err := verifyClientSecret(r.Form.Get("client_secret"), &clientKey.PublicKey, "KEY123", "TEAM123", "com.example.web"); err != nil {
				report(w, err)
				return
			}
			token, err := signRS256(appleKey, kid, map[string]any{
				"iss": appleIssuer, "aud": "com.example.web", "exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix(),
				"sub": "apple-subject-1", "nonce": wantNonce, "email": "user@example.test", "email_verified": "true", "is_private_email": "false",
			})
			if err != nil {
				report(w, fmt.Errorf("sign synthetic token: %w", err))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id_token": token, "access_token": "discard-me", "refresh_token": "discard-me"})
		case "/auth/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
				"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256", "n": jwkN(&appleKey.PublicKey), "e": jwkE(&appleKey.PublicKey),
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer func() {
		server.Close()
		close(handlerErrors)
		for err := range handlerErrors {
			t.Error(err)
		}
	}()

	c, err := New(Config{
		ClientID: "com.example.web", TeamID: "TEAM123", KeyID: "KEY123", RedirectURI: "https://example.test/apple/callback",
		PrivateKeyPEM: pemKey, HTTP: boundedTestHTTPClient(server), Clock: func() time.Time { return now },
		AuthorizeURL: server.URL + "/auth/authorize", TokenURL: server.URL + "/auth/token", KeysURL: server.URL + "/auth/keys",
	})
	if err != nil {
		t.Fatal(err)
	}
	authURL, err := c.AuthorizationURL("state-value-12345678901234567890", "nonce-value-12345678901234567890")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("client_id") != "com.example.web" || q.Get("response_type") != "code" || q.Get("response_mode") != "form_post" || q.Get("scope") != "name email" || q.Get("state") == "" || q.Get("nonce") == "" {
		t.Fatal("authorization URL")
	}
	wantNonce = "nonce-value-12345678901234567890"
	got, err := c.Exchange(t.Context(), "one-time-code")
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "apple-subject-1" || got.Nonce != wantNonce || got.Email != "user@example.test" || !got.EmailVerified || got.PrivateEmail {
		t.Fatal("verified identity", got)
	}

	mode = "reject"
	if _, err = c.Exchange(t.Context(), "one-time-code"); !errors.Is(err, appleservices.ErrIdentityRejected) {
		t.Fatal("400 classification", err)
	}
	mode = "server"
	if _, err = c.Exchange(t.Context(), "one-time-code"); !errors.Is(err, appleservices.ErrIdentityUnknown) {
		t.Fatal("500 classification", err)
	}
}

func TestClientRejectsWrongTokenAudienceAndSignature(t *testing.T) {
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	_, pemKey := testECPEM(t)
	appleKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := "apple-test-key"
	tokenKey := appleKey
	audience := "wrong.client"
	handlerErrors := make(chan error, 8)
	report := func(w http.ResponseWriter, err error) {
		select {
		case handlerErrors <- err:
		default:
		}
		http.Error(w, "synthetic handler failure", http.StatusInternalServerError)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			token, err := signRS256(tokenKey, kid, map[string]any{"iss": appleIssuer, "aud": audience, "exp": now.Add(time.Minute).Unix(), "iat": now.Unix(), "sub": "subject", "nonce": "nonce-value"})
			if err != nil {
				report(w, err)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id_token": token})
		case "/auth/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": kid, "alg": "RS256", "n": jwkN(&appleKey.PublicKey), "e": jwkE(&appleKey.PublicKey)}}})
		}
	}))
	defer func() {
		server.Close()
		close(handlerErrors)
		for err := range handlerErrors {
			t.Error(err)
		}
	}()
	c, err := New(Config{ClientID: "com.example.web", TeamID: "TEAM123", KeyID: "KEY123", RedirectURI: "https://example.test/apple/callback", PrivateKeyPEM: pemKey, HTTP: boundedTestHTTPClient(server), Clock: func() time.Time { return now }, TokenURL: server.URL + "/auth/token", KeysURL: server.URL + "/auth/keys", AuthorizeURL: server.URL + "/auth/authorize"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Exchange(t.Context(), "code"); !errors.Is(err, appleservices.ErrIdentityRejected) {
		t.Fatal("audience accepted", err)
	}
	audience = "com.example.web"
	tokenKey = otherKey
	if _, err = c.Exchange(t.Context(), "code"); !errors.Is(err, appleservices.ErrIdentityRejected) {
		t.Fatal("signature accepted", err)
	}
}

func boundedTestHTTPClient(server *httptest.Server) *http.Client {
	c := server.Client()
	c.Timeout = 5 * time.Second
	return c
}
