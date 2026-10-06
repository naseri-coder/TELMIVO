package appleservices

import (
	"context"
	"errors"
	"time"
)

var ErrIdentityUnavailable = errors.New("apple identity provider unavailable")
var ErrIdentityRejected = errors.New("apple identity rejected")
var ErrIdentityUnknown = errors.New("apple identity outcome unknown")

type VerifiedAppleIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	PrivateEmail  bool
	Nonce         string
}

type AppleIdentityProvider interface {
	ClientID() string
	AuthorizationURL(state, nonce string) (string, error)
	Exchange(context.Context, string) (VerifiedAppleIdentity, error)
}

type IdentityStart struct {
	Operation        Operation `json:"operation"`
	AuthorizationURL string    `json:"authorization_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type IdentityLink struct {
	Linked         bool      `json:"linked"`
	ClientID       string    `json:"client_id,omitempty"`
	EmailVerified  bool      `json:"email_verified"`
	PrivateEmail   bool      `json:"private_email"`
	LastVerifiedAt time.Time `json:"last_verified_at,omitempty"`
}

type IdentityCallback struct {
	State string
	Code  string
	Error string
}

type identityStatePayload struct {
	Workspace string `json:"w"`
	User      string `json:"u"`
	Operation string `json:"o"`
	Expires   int64  `json:"e"`
}
