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
	PermOrderRead      Permission = "order:read"
	PermOrderWrite     Permission = "order:write"
	PermPurchaseWrite  Permission = "purchase:write"
	PermReceivingWrite Permission = "receiving:write"
	// PermCatalogRead lets every role read the catalog and the store's
	// suppliers. Costs and margins are not part of it: SeesCosts decides
	// those (ADR 0025).
	PermCatalogRead Permission = "catalog:read"
	// PermCatalogWrite covers the catalog and, in milestone 3, the suppliers
	// that source it (ADR 0025).
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
			PermCatalogRead, PermCatalogWrite, PermFinanceRead, PermInventoryAdjust, PermMembersManage,
			PermOrderRead, PermOrderWrite, PermPortalWrite, PermProductionWrite,
			PermPurchaseWrite, PermReceivingWrite, PermSettingsRead, PermSettingsWrite,
		}
	case shared.RoleAdmin:
		return []Permission{
			PermCatalogRead, PermCatalogWrite, PermInventoryAdjust, PermMembersManage,
			PermOrderRead, PermOrderWrite, PermPortalWrite, PermProductionWrite,
			PermPurchaseWrite, PermReceivingWrite, PermSettingsRead,
		}
	case shared.RoleStaff:
		return []Permission{
			PermCatalogRead, PermOrderRead, PermOrderWrite, PermProductionWrite, PermReceivingWrite, PermSettingsRead,
		}
	case shared.RoleViewer:
		return []Permission{PermCatalogRead, PermOrderRead, PermSettingsRead}
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

// SeesCosts reports whether role may see costs and margins: item and
// supplier costs, kit costs and every margin computed from them. Only the
// owner may (ADR 0025; docs/product/design-document.md, Screens). It is a
// policy, not a permission, because it changes which fields a view
// carries rather than which calls a role may make: views built for any
// other role leave those fields out, and a write that sends a cost from any
// other role is refused.
func SeesCosts(role shared.Role) bool {
	return role == shared.RoleOwner
}

// AllPermissions returns every permission, sorted. The owner holds them all.
func AllPermissions() []Permission {
	return grantsOf(shared.RoleOwner)
}
