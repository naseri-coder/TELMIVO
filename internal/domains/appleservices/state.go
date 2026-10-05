package appleservices

import plugin "github.com/naseri-coder/TELMIVO/internal/plugins/appleservices"

func Capability(k Kind) string {
	switch k {
	case ConsumerAccount:
		return plugin.ConsumerSetup
	case IdentityVerification:
		return plugin.VerifyIdentity
	case BusinessConnection:
		return plugin.BusinessConnect
	case DirectoryProvisioning:
		return plugin.Provision
	case ManagedAccount:
		return plugin.Users
	case Reconciliation:
		return plugin.Reconcile
	default:
		return ""
	}
}

func ValidState(k Kind, s State) bool {
	if !k.Valid() {
		return false
	}
	switch k {
	case ConsumerAccount:
		return containsState(s, Requested, Precheck, AppleActionRequired, AppleCompleted, VerificationPending, Ready, Failed, Cancelled, Expired)
	case IdentityVerification:
		return containsState(s, Requested, Precheck, AppleActionRequired, VerificationPending, Ready, Failed, Cancelled, Expired)
	case BusinessConnection:
		return containsState(s, Requested, Precheck, AppleActionRequired, VerificationPending, Ready, ReconcileRequired, Suspended, Failed, Cancelled, Expired)
	case DirectoryProvisioning:
		return containsState(s, Requested, IDPCreating, IDPCreated, AppleSyncPending, Active, SyncError, RetryPending, ReconcileRequired, Suspended, Failed, Cancelled, Expired)
	case ManagedAccount:
		return containsState(s, Requested, IDPCreating, IDPCreated, AppleSyncPending, Active, SyncError, RetryPending, ReconcileRequired, Suspended, Failed, Cancelled, Expired)
	case Reconciliation:
		return containsState(s, Requested, Precheck, ReconcileRequired, Ready, Failed, Cancelled, Expired)
	default:
		return false
	}
}

func Terminal(s State) bool {
	return s == Failed || s == Cancelled || s == Expired
}

func CanTransition(k Kind, from, to State) bool {
	if !ValidState(k, from) || !ValidState(k, to) || from == to || Terminal(from) {
		return false
	}
	switch k {
	case ConsumerAccount:
		return edge(from, to,
			Requested, Precheck,
			Precheck, AppleActionRequired,
			Precheck, Failed,
			Precheck, Cancelled,
			AppleActionRequired, AppleCompleted,
			AppleActionRequired, Cancelled,
			AppleActionRequired, Expired,
			AppleCompleted, VerificationPending,
			AppleCompleted, Failed,
			VerificationPending, Ready,
			VerificationPending, Failed,
			VerificationPending, Expired,
		)
	case IdentityVerification:
		return edge(from, to,
			Requested, Precheck,
			Precheck, AppleActionRequired,
			Precheck, Failed,
			Precheck, Cancelled,
			AppleActionRequired, VerificationPending,
			AppleActionRequired, Cancelled,
			AppleActionRequired, Expired,
			VerificationPending, Ready,
			VerificationPending, Failed,
			VerificationPending, Expired,
		)
	case BusinessConnection:
		return edge(from, to,
			Requested, Precheck,
			Precheck, AppleActionRequired,
			Precheck, Failed,
			Precheck, Cancelled,
			AppleActionRequired, VerificationPending,
			AppleActionRequired, Cancelled,
			AppleActionRequired, Expired,
			VerificationPending, Ready,
			VerificationPending, ReconcileRequired,
			VerificationPending, Failed,
			Ready, ReconcileRequired,
			Ready, Suspended,
			ReconcileRequired, Ready,
			ReconcileRequired, Suspended,
			ReconcileRequired, Failed,
			Suspended, ReconcileRequired,
			Suspended, Cancelled,
		)
	case DirectoryProvisioning:
		return edge(from, to,
			Requested, IDPCreating,
			IDPCreating, IDPCreated,
			IDPCreating, SyncError,
			IDPCreating, Failed,
			IDPCreating, Cancelled,
			IDPCreated, AppleSyncPending,
			IDPCreated, Failed,
			AppleSyncPending, Active,
			AppleSyncPending, SyncError,
			AppleSyncPending, ReconcileRequired,
			SyncError, RetryPending,
			SyncError, ReconcileRequired,
			SyncError, Failed,
			RetryPending, AppleSyncPending,
			RetryPending, Failed,
			RetryPending, Cancelled,
			Active, ReconcileRequired,
			Active, Suspended,
			ReconcileRequired, Active,
			ReconcileRequired, RetryPending,
			ReconcileRequired, Suspended,
			ReconcileRequired, Failed,
			Suspended, RetryPending,
			Suspended, Cancelled,
		)
	case ManagedAccount:
		return edge(from, to,
			Requested, IDPCreating,
			IDPCreating, IDPCreated,
			IDPCreating, SyncError,
			IDPCreating, Failed,
			IDPCreating, Cancelled,
			IDPCreated, AppleSyncPending,
			IDPCreated, Failed,
			AppleSyncPending, Active,
			AppleSyncPending, SyncError,
			AppleSyncPending, ReconcileRequired,
			SyncError, RetryPending,
			SyncError, ReconcileRequired,
			SyncError, Failed,
			RetryPending, AppleSyncPending,
			RetryPending, Failed,
			RetryPending, Cancelled,
			Active, ReconcileRequired,
			Active, Suspended,
			ReconcileRequired, Active,
			ReconcileRequired, RetryPending,
			ReconcileRequired, Suspended,
			ReconcileRequired, Failed,
			Suspended, RetryPending,
			Suspended, Cancelled,
		)
	case Reconciliation:
		return edge(from, to,
			Requested, Precheck,
			Precheck, ReconcileRequired,
			Precheck, Ready,
			Precheck, Failed,
			ReconcileRequired, Ready,
			ReconcileRequired, Failed,
			ReconcileRequired, Cancelled,
		)
	default:
		return false
	}
}

func containsState(got State, values ...State) bool {
	for _, v := range values {
		if got == v {
			return true
		}
	}
	return false
}

func edge(from, to State, pairs ...State) bool {
	for i := 0; i+1 < len(pairs); i += 2 {
		if from == pairs[i] && to == pairs[i+1] {
			return true
		}
	}
	return false
}
