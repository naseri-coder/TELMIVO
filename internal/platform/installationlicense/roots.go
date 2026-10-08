package installationlicense

import (
	"crypto/ed25519"
	"encoding/base64"
	c "github.com/naseri-coder/TELMIVO/internal/platform/installationlicense/licensecrypto"
	"time"
)

type RootRecord struct {
	ID        string    `json:"id"`
	Version   int64     `json:"version"`
	PublicKey string    `json:"public_key"`
	NotBefore time.Time `json:"not_before"`
	NotAfter  time.Time `json:"not_after"`
	Status    c.Status  `json:"status"`
}
type RootSet struct {
	Version int          `json:"version"`
	Purpose string       `json:"purpose"`
	Roots   []RootRecord `json:"roots"`
}

func ParseRoots(b []byte, production bool) (c.Roots, error) {
	var cfg RootSet
	if StrictJSON(b, &cfg) != nil || cfg.Version != 1 || len(cfg.Roots) < 1 || len(cfg.Roots) > 8 {
		return nil, ErrState
	}
	if production && cfg.Purpose != "PRODUCTION" {
		return nil, ErrState
	}
	if cfg.Purpose != "PRODUCTION" && cfg.Purpose != "TEST_ONLY" && cfg.Purpose != "DEVELOPMENT" {
		return nil, ErrState
	}
	roots := c.Roots{}
	for _, r := range cfg.Roots {
		pub, e := base64.StdEncoding.Strict().DecodeString(r.PublicKey)
		if e != nil || len(pub) != ed25519.PublicKeySize || !c.ValidID(r.ID) || r.Version < 1 || r.NotBefore.IsZero() || !r.NotAfter.After(r.NotBefore) || roots[r.ID].ID != "" {
			return nil, ErrState
		}
		switch r.Status {
		case c.Active, c.Retiring, c.Revoked, c.Expired:
		default:
			return nil, ErrState
		}
		roots[r.ID] = c.Root{ID: r.ID, Version: r.Version, PublicKey: pub, NotBefore: r.NotBefore.UTC(), NotAfter: r.NotAfter.UTC(), Status: r.Status}
	}
	return roots, nil
}
