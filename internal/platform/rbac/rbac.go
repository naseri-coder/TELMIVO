package rbac

// Roles are data; this helper never grants permissions based on a plan or role name.
func Allows(permissions map[string]bool, permission string) bool {
	return permission != "" && permissions[permission]
}
func CanRemoveOwner(ownerCount int, removingOwner bool) bool { return !removingOwner || ownerCount > 1 }
