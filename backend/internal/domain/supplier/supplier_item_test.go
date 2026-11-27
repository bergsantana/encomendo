package supplier

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func money(t *testing.T, cents int64) *shared.Money {
	t.Helper()
	m, err := shared.NewMoney(cents, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	return &m
}

func TestNewSupplierItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		role     Role
		leadDays int
		cost     *shared.Money
	}{
		{"main without cost", RoleMain, 15, nil},
		{"backup with cost", RoleBackup, 7, money(t, 12990)},
		{"same-day delivery", RoleMain, 0, nil},
		{"longest lead time", RoleMain, 365, nil},
		{"free sample", RoleBackup, 3, money(t, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			i, err := NewSupplierItem(supplierID, tt.role, tt.leadDays, tt.cost)
			if err != nil {
				t.Fatal(err)
			}
			if i.SupplierID() != supplierID || i.Role() != tt.role || i.LeadDays() != tt.leadDays || i.IsZero() {
				t.Errorf("item = %v %q %d", i.SupplierID(), i.Role(), i.LeadDays())
			}
			got := i.Cost()
			if (got == nil) != (tt.cost == nil) || (got != nil && *got != *tt.cost) {
				t.Errorf("Cost() = %v, want %v", got, tt.cost)
			}
		})
	}
}

func TestNewSupplierItem_RejectsInvalidFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		leadDays  int
		cost      *shared.Money
		wantField string
		wantMsg   string
	}{
		{"negative lead days", -1, nil, "leadDays", "Use um prazo de 0 a 365 dias."},
		{"lead days above a year", 366, nil, "leadDays", "Use um prazo de 0 a 365 dias."},
		{"negative cost", 5, money(t, -1), "costCents", "Informe um custo de zero ou mais."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewSupplierItem(supplierID, RoleMain, tt.leadDays, tt.cost)
			if got := fieldsOf(t, err)[tt.wantField]; got != tt.wantMsg {
				t.Errorf("message on %s = %q, want %q", tt.wantField, got, tt.wantMsg)
			}
		})
	}

	_, err := NewSupplierItem(supplierID, RoleMain, -1, money(t, -1))
	if got := fieldsOf(t, err); len(got) != 2 {
		t.Errorf("fields = %v, want leadDays and costCents together", got)
	}
}

func TestNewSupplierItem_RejectsBrokenParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		supplier shared.SupplierID
		role     Role
		cost     *shared.Money
	}{
		{"zero supplier", shared.SupplierID{}, RoleMain, nil},
		{"unknown role", supplierID, Role("third"), nil},
		{"empty role", supplierID, Role(""), nil},
		{"cost without currency", supplierID, RoleMain, &shared.Money{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewSupplierItem(tt.supplier, tt.role, 5, tt.cost)
			if !errors.Is(err, ErrInvalidSupplierItem) {
				t.Errorf("error = %v, want ErrInvalidSupplierItem", err)
			}
			if errors.Is(err, shared.ErrInvalid) {
				t.Errorf("a bug matched shared.ErrInvalid, so the API would answer 400: %v", err)
			}
		})
	}
}

func TestSupplierItem_OwnsItsCost(t *testing.T) {
	t.Parallel()

	in := money(t, 1000)
	i, err := NewSupplierItem(supplierID, RoleMain, 5, in)
	if err != nil {
		t.Fatal(err)
	}
	*in = *money(t, 1)
	out := i.Cost()
	*out = *money(t, 2)
	if got := i.Cost(); got.Cents() != 1000 {
		t.Errorf("Cost() = %v, want 1000 cents: the value shared its cost with a caller", got)
	}
}

func TestSupplierItem_ZeroValue(t *testing.T) {
	t.Parallel()

	var i SupplierItem
	if !i.IsZero() || i.Cost() != nil || i.Role().Valid() {
		t.Errorf("zero SupplierItem = %+v", i)
	}
}

func TestRole_Valid(t *testing.T) {
	t.Parallel()

	for r, want := range map[Role]bool{RoleMain: true, RoleBackup: true, "": false, "Main": false, "reserve": false} {
		if got := r.Valid(); got != want {
			t.Errorf("Role(%q).Valid() = %v, want %v", r, got, want)
		}
	}
}
