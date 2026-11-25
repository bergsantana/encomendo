package identity

import (
	"slices"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Permission names one thing a staff role may do in its tenant. The values
// are part of the API contract: sessions list them so the web and Android
// apps can hide what the user cannot do. The backend stays the authority
// (docs/architecture.md section 5.4).
type Permission string

// Permissions. Each bounded context adds its own when it lands, with a row in
// the role table test.
const (
	PermOrderRead       Permission = "order:read"
	PermOrderWrite      Permission = "order:write"
	PermPurchaseWrite   Permission = "purchase:write"
	PermReceivingWrite  Permission = "receiving:write"
	PermCatalogWrite    Permission = "catalog:write"
	PermInventoryAdjust Permission = "inventory:adjust"
	PermProductionWrite Permission = "production:write"
	PermFinanceRead     Permission = "finance:read"
	PermMembersManage   Permission = "members:manage"
	PermPortalWrite     Permission = "portal:write"
	// PermSettingsRead lets every role read the store settings, because the
	// apps hide Produção and Estoque when the store turned them off.
	PermSettingsRead  Permission = "settings:read"
	PermSettingsWrite Permission = "settings:write"
)

// grantsOf is the role → permission map of docs/architecture.md section
// 5.4: the owner does everything; admins run the store except finance,
// settings and the owner's own membership; staff work orders, receiving and
// production; viewers only read, and never see costs or margins (that rule is
// a policy function in the contexts that hold costs, not a permission).
// Each list is sorted, and a new slice is built on every call so no caller
// can change the map.
func grantsOf(role shared.Role) []Permission {
	switch role {
	case shared.RoleOwner:
		return []Permission{
			PermCatalogWrite, PermFinanceRead, PermInventoryAdjust, PermMembersManage,
			PermOrderRead, PermOrderWrite, PermPortalWrite, PermProductionWrite,
			PermPurchaseWrite, PermReceivingWrite, PermSettingsRead, PermSettingsWrite,
		}
	case shared.RoleAdmin:
		return []Permission{
			PermCatalogWrite, PermInventoryAdjust, PermMembersManage,
			PermOrderRead, PermOrderWrite, PermPortalWrite, PermProductionWrite,
			PermPurchaseWrite, PermReceivingWrite, PermSettingsRead,
		}
	case shared.RoleStaff:
		return []Permission{PermOrderRead, PermOrderWrite, PermProductionWrite, PermReceivingWrite, PermSettingsRead}
	case shared.RoleViewer:
		return []Permission{PermOrderRead, PermSettingsRead}
	default:
		return nil
	}
}

// Allows reports whether role grants perm. Unknown roles and permissions
// grant nothing.
func Allows(role shared.Role, perm Permission) bool {
	return slices.Contains(grantsOf(role), perm)
}

// PermissionsOf returns the permissions role grants, sorted, as a new slice
// the caller may keep or change. It returns nil for an unknown role.
func PermissionsOf(role shared.Role) []Permission {
	return grantsOf(role)
}

// AllPermissions returns every permission, sorted. The owner holds them all.
func AllPermissions() []Permission {
	return grantsOf(shared.RoleOwner)
}
