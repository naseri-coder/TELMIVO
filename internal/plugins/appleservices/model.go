// Package appleservices defines the workspace-level Apple Services contract.
// R5A intentionally contains no Apple/IdP transport, credential or external-effect implementation.
package appleservices

const ID = "apple-services"
const Feature = "apple_services"

const (
	PermissionRead   = "apple.services.read"
	PermissionManage = "apple.services.manage"
	PermissionAudit  = "apple.audit.read"

	ConsumerSetup   = "apple.consumer.setup"
	VerifyIdentity  = "apple.verify"
	BusinessConnect = "apple.business.connect"
	Provision       = "apple.provision"
	Users           = "apple.users"
	Reconcile       = "apple.reconcile"
)

var Capabilities = []string{
	ConsumerSetup,
	VerifyIdentity,
	BusinessConnect,
	Provision,
	Users,
	Reconcile,
}
