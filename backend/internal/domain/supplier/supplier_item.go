package supplier

import (
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

const maxLeadDays = 365

// Role says which slot of an item a supplier fills.
type Role string

// The supplier roles of an item: the one the store buys from and the
// fornecedor reserva.
const (
	RoleMain   Role = "main"
	RoleBackup Role = "backup"
)

// Valid reports whether r is main or backup.
func (r Role) Valid() bool { return r == RoleMain || r == RoleBackup }

// SupplierItem is a supplier's part in one catalog item: its role, how many
// days it takes to deliver and, optionally, what it costs. It is an
// immutable value; the catalog holds it and decides which supplier fills
// which slot.
//
// The cost belongs to the pair of item and supplier, not to the role (ADR
// 0025). The value only holds it, so the catalog can keep each supplier's
// cost when main and backup swap, and start a newly linked supplier with
// none. Only the owner sees a cost (identity.SeesCosts).
type SupplierItem struct { //nolint:revive // the name is fixed by ADR 0025 and the glossary in CLAUDE.md (supplier.SupplierItem), and the catalog also has an Item
	supplier shared.SupplierID
	role     Role
	leadDays int
	cost     *shared.Money
}

// NewSupplierItem returns the link of supplier to an item. cost may be nil,
// meaning none is recorded. It returns a ValidationError listing every
// invalid field ("leadDays" outside 0 to 365, "costCents" below zero) and
// ErrInvalidSupplierItem for a zero supplier, a role other than main or
// backup, or a cost without a currency.
func NewSupplierItem(supplier shared.SupplierID, role Role, leadDays int, cost *shared.Money) (SupplierItem, error) {
	if supplier.IsZero() || !role.Valid() || (cost != nil && cost.Currency() == "") {
		return SupplierItem{}, fmt.Errorf("new supplier item of %v as %q: %w", supplier, role, ErrInvalidSupplierItem)
	}
	if err := shared.MergeValidation(checkLeadDays(leadDays), checkCost(cost)); err != nil {
		return SupplierItem{}, err
	}
	var kept *shared.Money
	if cost != nil {
		c := *cost // the value owns its cost: the caller's pointer is not shared
		kept = &c
	}
	return SupplierItem{supplier: supplier, role: role, leadDays: leadDays, cost: kept}, nil
}

func checkLeadDays(days int) error {
	if days < 0 || days > maxLeadDays {
		return shared.NewValidationError(shared.FieldError{Field: fieldLeadDays, Message: msgLeadDays})
	}
	return nil
}

func checkCost(cost *shared.Money) error {
	if cost != nil && cost.IsNegative() {
		return shared.NewValidationError(shared.FieldError{Field: fieldCostCents, Message: msgCost})
	}
	return nil
}

// IsZero reports whether i is the zero SupplierItem, which links no
// supplier.
func (i SupplierItem) IsZero() bool { return i.supplier.IsZero() }

// SupplierID returns the supplier.
func (i SupplierItem) SupplierID() shared.SupplierID { return i.supplier }

// Role returns the slot the supplier fills.
func (i SupplierItem) Role() Role { return i.role }

// LeadDays returns the days the supplier takes to deliver, 0 to 365.
func (i SupplierItem) LeadDays() int { return i.leadDays }

// Cost returns what the item costs from this supplier, or nil when none is
// recorded. The result is a copy: changing it never changes the value.
func (i SupplierItem) Cost() *shared.Money {
	if i.cost == nil {
		return nil
	}
	c := *i.cost
	return &c
}
