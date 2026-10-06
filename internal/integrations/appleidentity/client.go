package appleidentity

import (
	"context"
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
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/naseri-coder/TELMIVO/internal/domains/appleservices"
)

const (
	defaultAuthorizeURL = "https://appleid.apple.com/auth/authorize"
	defaultTokenURL     = "https://appleid.apple.com/auth/token"
	defaultKeysURL      = "https://appleid.apple.com/auth/keys"
	appleIssuer         = "https://appleid.apple.com"
	maxBodyBytes        = 1 << 20
)

type Config struct {
	ClientID      string
	TeamID        string
	KeyID         string
	RedirectURI   string
	PrivateKeyPEM []byte
	HTTP          *http.Client
	Clock         func() time.Time
	AuthorizeURL  string
	TokenURL      string
	KeysURL       string
}

type Client struct {
	clientID     string
	teamID       string
	keyID        string
	redirectURI  string
	privateKey   *ecdsa.PrivateKey
	http         *http.Client
	clock        func() time.Time
	authorizeURL string
	tokenURL     string
	keysURL      string
}

func New(c Config) (*Client, error) {
	c.ClientID = strings.TrimSpace(c.ClientID)
	c.TeamID = strings.TrimSpace(c.TeamID)
	c.KeyID = strings.TrimSpace(c.KeyID)
	c.RedirectURI = strings.TrimSpace(c.RedirectURI)
	if c.ClientID == "" || len(c.ClientID) > 200 || c.TeamID == "" || len(c.TeamID) > 64 ||
		c.KeyID == "" || len(c.KeyID) > 64 || c.RedirectURI == "" || len(c.PrivateKeyPEM) == 0 {
		return nil, errors.New("apple identity configuration invalid")
	}
	redirect, err := url.Parse(c.RedirectURI)
	if err != nil || redirect.Scheme != "https" || redirect.Host == "" || redirect.User != nil || redirect.Fragment != "" {
		return nil, errors.New("apple identity configuration invalid")
	}
	block, _ := pem.Decode(c.PrivateKeyPEM)
	if block == nil {
		return nil, errors.New("apple identity private key invalid")
	}
	raw, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("apple identity private key invalid")
	}
	key, ok := raw.(*ecdsa.PrivateKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("apple identity private key invalid")
	}
	if c.HTTP == nil {
		c.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	if c.Clock == nil {
		c.Clock = func() time.Time { return time.Now().UTC() }
	}
	if c.AuthorizeURL == "" {
		c.AuthorizeURL = defaultAuthorizeURL
	}
	if c.TokenURL == "" {
		c.TokenURL = defaultTokenURL
	}
	if c.KeysURL == "" {
		c.KeysURL = defaultKeysURL
	}
	for _, rawURL := range []string{c.AuthorizeURL, c.TokenURL, c.KeysURL} {
		u, err := url.Parse(rawURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, errors.New("apple identity endpoint invalid")
		}
	}
	return &Client{
		clientID: c.ClientID, teamID: c.TeamID, keyID: c.KeyID, redirectURI: c.RedirectURI,
		privateKey: key, http: c.HTTP, clock: c.Clock,
		authorizeURL: c.AuthorizeURL, tokenURL: c.TokenURL, keysURL: c.KeysURL,
	}, nil
}

func (c *Client) ClientID() string { return c.clientID }

func (c *Client) AuthorizationURL(state, nonce string) (string, error) {
	if len(state) < 20 || len(state) > 4096 || len(nonce) < 20 || len(nonce) > 512 {
		return "", appleservices.ErrIdentityRejected
	}
	u, err := url.Parse(c.authorizeURL)
	if err != nil {
		return "", appleservices.ErrIdentityUnavailable
	}
	q := u.Query()
	q.Set("client_id", c.clientID)
	q.Set("redirect_uri", c.redirectURI)
	q.Set("response_type", "code")
	q.Set("response_mode", "form_post")
	q.Set("scope", "name email")
	q.Set("state", state)
	q.Set("nonce", nonce)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func b64(v []byte) string { return base64.RawURLEncoding.EncodeToString(v) }

func (c *Client) clientSecret() (string, error) {
	now := c.clock().UTC()
	header, _ := json.Marshal(map[string]any{"alg": "ES256", "kid": c.keyID, "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss": c.teamID,
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"aud": appleIssuer,
		"sub": c.clientID,
	})
	unsigned := b64(header) + "." + b64(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, c.privateKey, digest[:])
	if err != nil {
		return "", errors.New("apple identity signing failed")
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return unsigned + "." + b64(sig), nil
}

type tokenResponse struct {
	IDToken string `json:"id_token"`
	Error   string `json:"error"`
}

func (c *Client) Exchange(ctx context.Context, code string) (appleservices.VerifiedAppleIdentity, error) {
	var out appleservices.VerifiedAppleIdentity
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 8192 {
		return out, appleservices.ErrIdentityRejected
	}
	secret, err := c.clientSecret()
	if err != nil {
		return out, appleservices.ErrIdentityUnavailable
	}
	form := url.Values{}
	form.Set("client_id", c.clientID)
	form.Set("client_secret", secret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", c.redirectURI)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return out, appleservices.ErrIdentityUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http.Do(req)
	if err != nil {
		return out, appleservices.ErrIdentityUnknown
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBodyBytes+1))
	if err != nil || len(body) > maxBodyBytes {
		return out, appleservices.ErrIdentityUnknown
	}
	var token tokenResponse
	if json.Unmarshal(body, &token) != nil {
		if res.StatusCode >= 400 && res.StatusCode < 500 {
			return out, appleservices.ErrIdentityRejected
		}
		return out, appleservices.ErrIdentityUnknown
	}
	if res.StatusCode != http.StatusOK {
		if res.StatusCode >= 400 && res.StatusCode < 500 {
			return out, appleservices.ErrIdentityRejected
		}
		return out, appleservices.ErrIdentityUnknown
	}
	if token.IDToken == "" {
		return out, appleservices.ErrIdentityRejected
	}
	return c.verifyIDToken(ctx, token.IDToken)
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type audienceClaim []string

func (a *audienceClaim) UnmarshalJSON(raw []byte) error {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		*a = []string{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

type idClaims struct {
	Issuer         string        `json:"iss"`
	Audience       audienceClaim `json:"aud"`
	Expires        int64         `json:"exp"`
	IssuedAt       int64         `json:"iat"`
	Subject        string        `json:"sub"`
	Nonce          string        `json:"nonce"`
	Email          string        `json:"email"`
	EmailVerified  any           `json:"email_verified"`
	IsPrivateEmail any           `json:"is_private_email"`
}

type jwkSet struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func boolClaim(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		b, err := strconv.ParseBool(x)
		return err == nil && b
	default:
		return false
	}
}

func containsAudience(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func (c *Client) verifyIDToken(ctx context.Context, raw string) (appleservices.VerifiedAppleIdentity, error) {
	var out appleservices.VerifiedAppleIdentity
	if len(raw) < 20 || len(raw) > 64<<10 {
		return out, appleservices.ErrIdentityRejected
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return out, appleservices.ErrIdentityRejected
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return out, appleservices.ErrIdentityRejected
	}
	var header jwtHeader
	if json.Unmarshal(headerBytes, &header) != nil || header.Alg != "RS256" || header.Kid == "" {
		return out, appleservices.ErrIdentityRejected
	}
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return out, appleservices.ErrIdentityRejected
	}
	var claims idClaims
	if json.Unmarshal(claimsBytes, &claims) != nil {
		return out, appleservices.ErrIdentityRejected
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return out, appleservices.ErrIdentityRejected
	}
	key, err := c.applePublicKey(ctx, header.Kid)
	if err != nil {
		return out, err
	}
	unsigned := parts[0] + "." + parts[1]
	digest := sha256.Sum256([]byte(unsigned))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig) != nil {
		return out, appleservices.ErrIdentityRejected
	}
	now := c.clock().UTC()
	if claims.Issuer != appleIssuer || !containsAudience(claims.Audience, c.clientID) ||
		claims.Expires <= now.Unix() || claims.IssuedAt > now.Add(time.Minute).Unix() ||
		claims.Subject == "" || len(claims.Subject) > 512 || claims.Nonce == "" || len(claims.Nonce) > 512 {
		return out, appleservices.ErrIdentityRejected
	}
	out.Subject = claims.Subject
	out.Nonce = claims.Nonce
	out.Email = strings.TrimSpace(claims.Email)
	out.EmailVerified = boolClaim(claims.EmailVerified)
	out.PrivateEmail = boolClaim(claims.IsPrivateEmail)
	return out, nil
}

func (c *Client) applePublicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.keysURL, nil)
	if err != nil {
		return nil, appleservices.ErrIdentityUnavailable
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, appleservices.ErrIdentityUnknown
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, appleservices.ErrIdentityUnknown
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBodyBytes+1))
	if err != nil || len(body) > maxBodyBytes {
		return nil, appleservices.ErrIdentityUnknown
	}
	var set jwkSet
	if json.Unmarshal(body, &set) != nil {
		return nil, appleservices.ErrIdentityUnknown
	}
	for _, k := range set.Keys {
		if k.Kid != kid || k.Kty != "RSA" || k.Alg != "RS256" || k.N == "" || k.E == "" {
			continue
		}
		n, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, appleservices.ErrIdentityUnknown
		}
		e, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil || len(e) == 0 || len(e) > 4 {
			return nil, appleservices.ErrIdentityUnknown
		}
		exponent := 0
		for _, b := range e {
			exponent = exponent<<8 | int(b)
		}
		if exponent < 3 {
			return nil, appleservices.ErrIdentityUnknown
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}, nil
	}
	return nil, appleservices.ErrIdentityUnknown
}
