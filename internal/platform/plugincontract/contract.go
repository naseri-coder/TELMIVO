// Package plugincontract is the small, infrastructure-free contract for trusted plugins.
package plugincontract

import (
	"context"
	"encoding/json"
	"errors"
)

var (
	ErrManifest      = errors.New("PLUGIN_MANIFEST_INVALID")
	ErrVersion       = errors.New("PLUGIN_VERSION_INCOMPATIBLE")
	ErrDependency    = errors.New("PLUGIN_DEPENDENCY_INVALID")
	ErrCycle         = errors.New("PLUGIN_DEPENDENCY_CYCLE")
	ErrConflict      = errors.New("PLUGIN_CONFLICT")
	ErrPermission    = errors.New("PLUGIN_CAPABILITY_DENIED")
	ErrState         = errors.New("PLUGIN_STATE_INVALID")
	ErrConfiguration = errors.New("PLUGIN_CONFIGURATION_INVALID")
)

type State string

const (
	Available        State = "AVAILABLE"
	Installed        State = "INSTALLED"
	Active           State = "ACTIVE"
	Disabled         State = "DISABLED"
	UninstallPending State = "UNINSTALL_PENDING"
	Uninstalled      State = "UNINSTALLED"
)

type Action string

const (
	Install           Action = "INSTALL"
	Activate          Action = "ACTIVATE"
	Disable           Action = "DISABLE"
	RequestUninstall  Action = "REQUEST_UNINSTALL"
	CompleteUninstall Action = "COMPLETE_UNINSTALL"
	Execute           Action = "EXECUTE"
)

type Dependency struct {
	PluginID string       `json:"plugin_id"`
	Versions VersionRange `json:"versions"`
}

// Extensions contain references to externally owned policies, never prices or plan rules.
type Extensions struct {
	PricingPolicy      string       `json:"pricing_policy,omitempty"`
	PlanPolicy         string       `json:"plan_policy,omitempty"`
	TrialPolicy        string       `json:"trial_policy,omitempty"`
	QuotaPolicy        string       `json:"quota_policy,omitempty"`
	BillingScopePolicy string       `json:"billing_scope_policy,omitempty"`
	BundleKeys         []string     `json:"bundle_keys,omitempty"`
	Marketplace        *Marketplace `json:"marketplace,omitempty"`
}

type Marketplace struct {
	DeveloperID string `json:"developer_id"`
	ListingID   string `json:"listing_id"`
}

type Manifest struct {
	SchemaVersion        int            `json:"schema_version"`
	PluginID             string         `json:"plugin_id"`
	Name                 string         `json:"name"`
	Version              string         `json:"version"`
	Description          string         `json:"description"`
	CoreVersions         VersionRange   `json:"core_versions"`
	Dependencies         []Dependency   `json:"dependencies,omitempty"`
	OptionalDependencies []Dependency   `json:"optional_dependencies,omitempty"`
	Conflicts            []string       `json:"conflicts,omitempty"`
	Permissions          []string       `json:"requested_permissions,omitempty"`
	Settings             SettingsSchema `json:"settings_schema"`
	LifecycleHooks       []Action       `json:"lifecycle_hooks,omitempty"`
	DataVersion          int            `json:"data_version"`
	MigrationKeys        []string       `json:"migration_keys,omitempty"`
	Extensions           Extensions     `json:"extensions"`
}

type Scope struct {
	// AccountID is the existing workspace billing subject, not a Telegram user ID.
	AccountID string `json:"account_id"`
	BotID     string `json:"bot_id"`
	PluginID  string `json:"plugin_id"`
}

type LicenseScope string

const (
	PerBot     LicenseScope = "PER_BOT"
	PerAccount LicenseScope = "PER_ACCOUNT"
)

type Admission struct {
	Scope      Scope
	ActorID    string
	Action     Action
	Capability string
	Manifest   Manifest
}

type Grant struct {
	Capabilities []string
	LicenseScope LicenseScope
	// A future immutable entitlement/price revision; the runtime never interprets it.
	PolicyRevision string
}

// AdmissionPolicy is the fail-closed entitlement/quota boundary. Implementations must
// evaluate current grants, trials, grace and quota policy, not trust manifest references.
// It must be read-only here; charging/reservation settlement belongs to a later stage.
type AdmissionPolicy interface {
	Authorize(context.Context, Admission) (Grant, error)
}

type Installation struct {
	Scope          Scope           `json:"scope"`
	Version        string          `json:"version"`
	DataVersion    int             `json:"data_version"`
	State          State           `json:"state"`
	Configuration  json.RawMessage `json:"configuration"`
	Data           json.RawMessage `json:"data"`
	Capabilities   []string        `json:"capabilities"`
	LicenseScope   LicenseScope    `json:"license_scope"`
	PolicyRevision string          `json:"policy_revision"`
	Revision       int64           `json:"revision"`
}

// Invoker is the only runtime handle given to a plugin. It contains no credentials,
// database transaction, global service locator or caller-controlled tenant coordinates.
type Invoker interface {
	Invoke(context.Context, string, json.RawMessage) (json.RawMessage, error)
}

// LifecycleValidator may veto a transition; no side effects are permitted. Executable
// install/migration/cleanup hooks are deferred. A declared hook without a validator fails.
type LifecycleValidator interface {
	Validate(context.Context, Action, Installation, Manifest) error
}
