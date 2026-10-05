package appleservices

import (
	"testing"

	plugin "github.com/naseri-coder/TELMIVO/internal/plugins/appleservices"
)

func TestR5AStateMachines(t *testing.T) {
	tests := []struct {
		kind Kind
		from State
		to   State
	}{
		{ConsumerAccount, Requested, Precheck},
		{ConsumerAccount, Precheck, AppleActionRequired},
		{ConsumerAccount, AppleActionRequired, AppleCompleted},
		{ConsumerAccount, AppleCompleted, VerificationPending},
		{ConsumerAccount, VerificationPending, Ready},
		{IdentityVerification, AppleActionRequired, VerificationPending},
		{BusinessConnection, Ready, ReconcileRequired},
		{DirectoryProvisioning, Requested, IDPCreating},
		{DirectoryProvisioning, IDPCreating, IDPCreated},
		{DirectoryProvisioning, IDPCreated, AppleSyncPending},
		{DirectoryProvisioning, AppleSyncPending, Active},
		{ManagedAccount, IDPCreated, AppleSyncPending},
		{ManagedAccount, Active, Suspended},
		{Reconciliation, ReconcileRequired, Ready},
	}
	for _, tc := range tests {
		if !CanTransition(tc.kind, tc.from, tc.to) {
			t.Fatalf("expected transition %s %s -> %s", tc.kind, tc.from, tc.to)
		}
	}
	for _, tc := range []struct {
		kind Kind
		from State
		to   State
	}{
		{ConsumerAccount, Requested, Ready},
		{ConsumerAccount, AppleCompleted, Ready},
		{IdentityVerification, Ready, Precheck},
		{BusinessConnection, Failed, Ready},
		{DirectoryProvisioning, Requested, Active},
		{ManagedAccount, Requested, AppleActionRequired},
		{Reconciliation, Ready, ReconcileRequired},
	} {
		if CanTransition(tc.kind, tc.from, tc.to) {
			t.Fatalf("unexpected transition %s %s -> %s", tc.kind, tc.from, tc.to)
		}
	}
	if Capability(ConsumerAccount) != plugin.ConsumerSetup ||
		Capability(IdentityVerification) != plugin.VerifyIdentity ||
		Capability(BusinessConnection) != plugin.BusinessConnect ||
		Capability(DirectoryProvisioning) != plugin.Provision ||
		Capability(ManagedAccount) != plugin.Users ||
		Capability(Reconciliation) != plugin.Reconcile {
		t.Fatal("capability mapping drift")
	}
	if !Terminal(Failed) || !Terminal(Cancelled) || !Terminal(Expired) || Terminal(Ready) {
		t.Fatal("terminal state contract drift")
	}
}
