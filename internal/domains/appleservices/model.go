package appleservices

import "time"

type Kind string

const (
	ConsumerAccount       Kind = "CONSUMER_ACCOUNT"
	IdentityVerification  Kind = "IDENTITY_VERIFICATION"
	BusinessConnection    Kind = "BUSINESS_CONNECTION"
	DirectoryProvisioning Kind = "DIRECTORY_PROVISIONING"
	ManagedAccount        Kind = "MANAGED_ACCOUNT"
	Reconciliation        Kind = "RECONCILIATION"
)

func (k Kind) Valid() bool {
	switch k {
	case ConsumerAccount, IdentityVerification, BusinessConnection, DirectoryProvisioning, ManagedAccount, Reconciliation:
		return true
	default:
		return false
	}
}

type State string

const (
	Requested           State = "REQUESTED"
	Precheck            State = "PRECHECK"
	AppleActionRequired State = "APPLE_ACTION_REQUIRED"
	AppleCompleted      State = "APPLE_COMPLETED"
	VerificationPending State = "VERIFICATION_PENDING"
	IDPCreating         State = "IDP_CREATING"
	IDPCreated          State = "IDP_CREATED"
	AppleSyncPending    State = "APPLE_SYNC_PENDING"
	SyncError           State = "SYNC_ERROR"
	RetryPending        State = "RETRY_PENDING"
	Ready               State = "READY"
	Active              State = "ACTIVE"
	ReconcileRequired   State = "RECONCILE_REQUIRED"
	Suspended           State = "SUSPENDED"
	Failed              State = "FAILED"
	Cancelled           State = "CANCELLED"
	Expired             State = "EXPIRED"
)

type Profile struct {
	Workspace string    `json:"workspace_id"`
	State     string    `json:"state"`
	Revision  int64     `json:"revision"`
	Created   time.Time `json:"created_at"`
	Updated   time.Time `json:"updated_at"`
}

type Operation struct {
	ID         string     `json:"id"`
	Kind       Kind       `json:"kind"`
	State      State      `json:"state"`
	SubjectRef string     `json:"subject_ref"`
	Revision   int64      `json:"revision"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Created    time.Time  `json:"created_at"`
	Updated    time.Time  `json:"updated_at"`
}

type Event struct {
	ID          string    `json:"id"`
	Sequence    int64     `json:"sequence"`
	From        State     `json:"from_state"`
	To          State     `json:"to_state"`
	Reason      string    `json:"reason"`
	Correlation string    `json:"correlation_id"`
	Created     time.Time `json:"created_at"`
}

type CreateRequest struct {
	Kind       Kind       `json:"kind"`
	SubjectRef string     `json:"subject_ref"`
	Key        string     `json:"idempotency_key"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type TransitionRequest struct {
	State    State  `json:"state"`
	Expected int64  `json:"expected_revision"`
	Reason   string `json:"reason"`
}

type ProfileAction struct {
	State    string `json:"state"`
	Expected int64  `json:"expected_revision"`
	Reason   string `json:"reason"`
}
