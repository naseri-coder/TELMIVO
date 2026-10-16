//go:build lic_a9_acceptance

// Package lica9test exposes only test adapters around actual LIC-A9 client code.
// Ordinary application builds exclude this file.
package lica9test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	l "github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	c "github.com/naseri-coder/TELMIVO/internal/platform/installationlicense/licensecrypto"
	"net/http"
	"time"
)

type Client struct {
	Manager *l.Manager
	Network *l.Client
	Config  l.Config
}

func New(directory, authority, endpoint string, trust []byte, httpClient *http.Client) (*Client, error) {
	roots, e := l.ParseRoots(trust, false)
	if e != nil {
		return nil, e
	}
	cfg := l.Config{Directory: directory, AuthorityID: authority, URL: endpoint, Environment: "TEST", Roots: roots, RequireConcurrency: true}
	m, e := l.Open(cfg)
	if e != nil {
		return nil, e
	}
	cl, e := l.NewClient(m)
	if e != nil {
		return nil, e
	}
	// Test transport is a real trusted TLS CA transport, never a TLS bypass.
	cl.SetAcceptanceTransport(httpClient.Transport)
	return &Client{m, cl, cfg}, nil
}
func (c *Client) Activate(code, license string) error {
	return c.Network.Activate(context.Background(), code, license, 1)
}
func (c *Client) Refresh() error             { return c.Network.Refresh(context.Background()) }
func (c *Client) Concurrency() error         { return c.Network.Concurrency(context.Background()) }
func (c *Client) Decision() (string, string) { d := c.Manager.Decision(); return d.State, d.Reason }
func (c *Client) Accept(b []byte) error      { return c.Manager.Accept(b) }
func (c *Client) SetClock(now time.Time)     { c.Manager.SetAcceptanceClock(now) }

func (c *Client) SetTransport(t http.RoundTripper) { c.Network.SetAcceptanceTransport(t) }
func (c *Client) Session() string                  { return c.Manager.AcceptanceSession() }
func (c *Client) SetSession(s string)              { c.Manager.SetAcceptanceSession(s) }

func (cl *Client) WrongDelegated(raw []byte) []byte {
	e, err := c.DecodeEnvelope(raw)
	if err != nil {
		return nil
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil
	}
	e.Signature = ed25519.Sign(key, c.LicenseSigningInput(e.Payload))
	out, _ := c.EncodeEnvelope(e)
	return out
}

func (c *Client) SignBody(r *http.Request, b []byte) error { return c.Network.SignAcceptanceBody(r, b) }
func CanonicalRequest(raw []byte) ([]byte, error) {
	request, e := c.ParseConcurrencyRequest(raw)
	if e != nil {
		return nil, e
	}
	return request.Canonical()
}
func (c *Client) Effect(fn func() error) error { return c.Manager.Effect(context.Background(), fn) }
