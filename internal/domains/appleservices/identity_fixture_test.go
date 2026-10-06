package appleservices

import "context"

type SyntheticAppleIdentityProvider struct {
	ID       string
	URL      string
	Result   VerifiedAppleIdentity
	Err      error
	Calls    int
	LastCode string
}

func (p *SyntheticAppleIdentityProvider) ClientID() string { return p.ID }
func (p *SyntheticAppleIdentityProvider) AuthorizationURL(state, nonce string) (string, error) {
	return p.URL + "?state=" + state + "&nonce=" + nonce, nil
}
func (p *SyntheticAppleIdentityProvider) Exchange(_ context.Context, code string) (VerifiedAppleIdentity, error) {
	p.Calls++
	p.LastCode = code
	return p.Result, p.Err
}
