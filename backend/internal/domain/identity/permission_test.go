package identity

import (
	"slices"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The whole role map, written out so a change to who may do what shows up
// as a changed line in review (docs/architecture.md section 5.4).
var wantGrants = map[Permission][]shared.Role{
	PermOrderRead:       {shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer},
	PermOrderWrite:      {shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff},
	PermPurchaseWrite:   {shared.RoleOwner, shared.RoleAdmin},
	PermReceivingWrite:  {shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff},
	PermCatalogWrite:    {shared.RoleOwner, shared.RoleAdmin},
	PermInventoryAdjust: {shared.RoleOwner, shared.RoleAdmin},
	PermProductionWrite: {shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff},
	PermFinanceRead:     {shared.RoleOwner},
	PermMembersManage:   {shared.RoleOwner, shared.RoleAdmin},
	PermPortalWrite:     {shared.RoleOwner, shared.RoleAdmin},
	PermSettingsRead:    {shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer},
	PermSettingsWrite:   {shared.RoleOwner},
}

var allRoles = []shared.Role{shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer}

func TestAllows_MatchesTheRoleTable(t *testing.T) {
	t.Parallel()

	for _, perm := range AllPermissions() {
		granted, listed := wantGrants[perm]
		if !listed {
			t.Errorf("%s has no row in wantGrants; add it to the table", perm)
			continue
		}
		for _, role := range allRoles {
			t.Run(string(perm)+"/"+string(role), func(t *testing.T) {
				t.Parallel()

				want := slices.Contains(granted, role)
				if got := Allows(role, perm); got != want {
					t.Errorf("Allows(%s, %s) = %v, want %v", role, perm, got, want)
				}
			})
		}
	}
	if len(wantGrants) != len(AllPermissions()) {
		t.Errorf("wantGrants has %d rows, AllPermissions %d", len(wantGrants), len(AllPermissions()))
	}
}

func TestPermissionsOf_GrantsNothingOutsideTheTable(t *testing.T) {
	t.Parallel()

	// The test above walks AllPermissions, which is the owner's list. This one
	// walks every role's own list, so a permission given only to admin or
	// staff (and forgotten for the owner) still fails here.
	for _, role := range allRoles {
		for _, perm := range PermissionsOf(role) {
			if !slices.Contains(wantGrants[perm], role) {
				t.Errorf("%s grants %s, which wantGrants does not list for it", role, perm)
			}
			if !slices.Contains(AllPermissions(), perm) {
				t.Errorf("%s grants %s, which the owner lacks", role, perm)
			}
		}
	}
}

func TestAllows_ViewerOnlyReads(t *testing.T) {
	t.Parallel()

	got := PermissionsOf(shared.RoleViewer)
	want := []Permission{PermOrderRead, PermSettingsRead}
	if !slices.Equal(got, want) {
		t.Errorf("viewer permissions = %v, want %v", got, want)
	}
}

func TestAllows_UnknownRoleOrPermissionGrantsNothing(t *testing.T) {
	t.Parallel()

	if Allows(shared.Role("client"), PermOrderRead) || Allows("", PermOrderRead) {
		t.Error("a role outside the table was granted a permission")
	}
	if Allows(shared.RoleOwner, Permission("order:delete")) {
		t.Error("an unknown permission was granted")
	}
	if PermissionsOf(shared.Role("client")) != nil {
		t.Error("PermissionsOf an unknown role is not nil")
	}
}

func TestPermissionsOf_IsSortedAndSafeToModify(t *testing.T) {
	t.Parallel()

	first := PermissionsOf(shared.RoleAdmin)
	if !slices.IsSorted(first) {
		t.Errorf("PermissionsOf(admin) = %v, not sorted", first)
	}
	first[0] = "tampered"
	if PermissionsOf(shared.RoleAdmin)[0] == "tampered" {
		t.Error("PermissionsOf returned shared state")
	}
}

func TestAllPermissions_SortedAndUnique(t *testing.T) {
	t.Parallel()

	all := AllPermissions()
	if !slices.IsSorted(all) || len(slices.Compact(slices.Clone(all))) != len(all) {
		t.Errorf("AllPermissions() = %v", all)
	}
}
