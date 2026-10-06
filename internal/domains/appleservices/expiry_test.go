package appleservices

import (
	"testing"
	"time"
)

func TestFoundationExpiryRetainsSafeRecovery(t *testing.T) {
	now := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	past := now.Add(-time.Second)
	future := now.Add(time.Second)
	for _, kind := range []Kind{IdentityVerification, BusinessConnection, DirectoryProvisioning, ManagedAccount, Reconciliation} {
		if !CanTransition(kind, Requested, Expired) {
			t.Fatal("no safe expiry", kind)
		}
		if !expiryBlocks(kind, Requested, Failed, &past, now) || expiryBlocks(kind, Requested, Expired, &past, now) || expiryBlocks(kind, Requested, Failed, &future, now) {
			t.Fatal("deadline fence", kind)
		}
		if CanTransition(kind, Expired, Requested) {
			t.Fatal("terminal resurrection", kind)
		}
	}
	for _, kind := range []Kind{DirectoryProvisioning, ManagedAccount} {
		if CanTransition(kind, IDPCreating, Expired) || expiryBlocks(kind, IDPCreating, IDPCreated, &past, now) || !CanTransition(kind, IDPCreating, IDPCreated) {
			t.Fatal("deadline traps ambiguous effect", kind)
		}
	}
}
