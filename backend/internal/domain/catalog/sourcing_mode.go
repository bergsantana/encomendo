package catalog

import (
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// SourcingMode says how the store gets an item (ADR 0025): "Sob encomenda",
// "Estoque" or "Produção própria". Rules never branch on the label, only on
// the mode.
type SourcingMode string

// The sourcing modes of an item.
const (
	// ModeToOrder buys the item from a supplier once a client orders it.
	ModeToOrder SourcingMode = "to_order"
	// ModeStock sells from stock and restocks from a supplier.
	ModeStock SourcingMode = "stock"
	// ModeInHouse makes the item in the store's own workshop.
	ModeInHouse SourcingMode = "in_house"
)

// Valid reports whether m is one of the three modes.
func (m SourcingMode) Valid() bool {
	return m == ModeToOrder || m == ModeStock || m == ModeInHouse
}

// NeedsSupplier reports whether items of this mode are bought from a
// supplier: to_order and stock.
func (m SourcingMode) NeedsSupplier() bool { return m == ModeToOrder || m == ModeStock }

type costAction uint8

const (
	costKeep costAction = iota
	costClear
	costSet
)

// CostChange says what a write does to one cost: keep what is stored, clear
// it, or set a new amount. The zero value keeps. A write from a role that
// may not see costs sends "keep", so it can edit an item without seeing or
// wiping a cost (ADR 0025).
type CostChange struct {
	action costAction
	amount shared.Money
}

// KeepCost leaves the stored cost as it is. See ReplaceSourcing for which
// cost counts as stored.
func KeepCost() CostChange { return CostChange{action: costKeep} }

// ClearCost removes the cost: none is recorded afterwards.
func ClearCost() CostChange { return CostChange{action: costClear} }

// SetCost records amount as the cost. It is checked where it is used: it
// must not be negative, nor above R$ 1.000.000.000,00, and it must be in the
// item's currency.
func SetCost(amount shared.Money) CostChange { return CostChange{action: costSet, amount: amount} }

// resolve returns the cost after the change, given the stored one.
func (c CostChange) resolve(stored *shared.Money) *shared.Money {
	switch c.action {
	case costSet:
		m := c.amount
		return &m
	case costClear:
		return nil
	default:
		return stored
	}
}

// fromStored turns a stored cost into the change that sets it back.
func fromStored(cost *shared.Money) CostChange {
	if cost == nil {
		return ClearCost()
	}
	return SetCost(*cost)
}

// SupplierLink is the input for one supplier of an item: which supplier, its
// lead time and what happens to the item's cost from it.
type SupplierLink struct {
	Supplier shared.SupplierID
	LeadDays int
	Cost     CostChange
}

// SourcingInput is the input that replaces an item's sourcing: its mode and
// the mode's own parts. Parts that do not belong to the mode must be left
// empty: ProductionDays 0, Main and Backup nil for in_house, and no
// production days for the other two.
type SourcingInput struct {
	Mode           SourcingMode
	ProductionDays int
	ProductionCost CostChange
	Main           *SupplierLink
	Backup         *SupplierLink
}

// sourcing is the validated state of an item's sourcing.
type sourcing struct {
	mode           SourcingMode
	productionDays int
	productionCost *shared.Money
	main           *supplier.SupplierItem
	backup         *supplier.SupplierItem
}

// costFor returns the cost the item has from the linked supplier id, wherever
// it sits (main or backup), and nil when the supplier is not linked or has no
// cost. A cost follows the supplier, never the slot (ADR 0025).
func (s sourcing) costFor(id shared.SupplierID) *shared.Money {
	for _, link := range []*supplier.SupplierItem{s.main, s.backup} {
		if link != nil && link.SupplierID() == id {
			return link.Cost()
		}
	}
	return nil
}

// buildSourcing validates in against the item's current sourcing, which
// resolves every "keep", and returns the new state. currency is the item's
// price currency; it returns a ValidationError for what a person can get
// wrong and ErrInvalidItem for a cost in another currency.
func buildSourcing(current sourcing, in SourcingInput, currency string) (sourcing, error) {
	switch {
	case !in.Mode.Valid():
		return sourcing{}, fieldError(fieldSourcingMode, msgSourcingMode)
	case in.Mode == ModeInHouse:
		return buildInHouse(current, in, currency)
	}
	return buildSupplied(current, in, currency)
}

func buildInHouse(current sourcing, in SourcingInput, currency string) (sourcing, error) {
	var errs []error
	if in.Main != nil {
		errs = append(errs, fieldError(fieldMainSupplier, msgNoSuppliersInHouse))
	}
	if in.Backup != nil {
		errs = append(errs, fieldError(fieldBackupSupplier, msgNoSuppliersInHouse))
	}
	switch {
	case in.ProductionDays == 0:
		errs = append(errs, fieldError(fieldProductionDays, msgProductionDays))
	case in.ProductionDays < 1 || in.ProductionDays > maxProductionDays:
		errs = append(errs, fieldError(fieldProductionDays, msgProductionRange))
	}
	// Coming into in_house starts with no cost: only an item that was already
	// in_house has a production cost to keep.
	var stored *shared.Money
	if current.mode == ModeInHouse {
		stored = current.productionCost
	}
	cost := in.ProductionCost.resolve(stored)
	errs = append(errs, checkCost(fieldProductionCost, cost))
	if err := shared.MergeValidation(errs...); err != nil {
		return sourcing{}, err
	}
	if err := checkCurrency(cost, currency); err != nil {
		return sourcing{}, err
	}
	return sourcing{mode: ModeInHouse, productionDays: in.ProductionDays, productionCost: cost}, nil
}

func buildSupplied(current sourcing, in SourcingInput, currency string) (sourcing, error) {
	var errs []error
	if in.ProductionDays != 0 {
		errs = append(errs, fieldError(fieldProductionDays, msgProductionOnly))
	}
	if in.ProductionCost.action == costSet {
		errs = append(errs, fieldError(fieldProductionCost, msgProductionOnly))
	}
	main, err := buildLink(current, in.Main, mainSlot)
	errs = append(errs, err)
	backup, err := buildLink(current, in.Backup, backupSlot)
	errs = append(errs, err)
	if in.Main != nil && in.Backup != nil && !in.Main.Supplier.IsZero() && in.Main.Supplier == in.Backup.Supplier {
		errs = append(errs, prefixFields(fieldError(fieldSupplierID, msgBackupDiffers), fieldBackupSupplier))
	}
	if in.Main == nil {
		errs = append(errs, prefixFields(fieldError(fieldSupplierID, msgMainSupplier), fieldMainSupplier))
	}
	if err := shared.MergeValidation(errs...); err != nil {
		return sourcing{}, err
	}
	for _, link := range []*supplier.SupplierItem{main, backup} {
		if link == nil {
			continue
		}
		if err := checkCurrency(link.Cost(), currency); err != nil {
			return sourcing{}, err
		}
	}
	return sourcing{mode: in.Mode, main: main, backup: backup}, nil
}

// slot is one place of an item's supplier list, with the field and message
// used when it is wrong.
type slot struct {
	role    supplier.Role
	field   string
	missing string
}

var (
	mainSlot   = slot{supplier.RoleMain, fieldMainSupplier, msgMainSupplier}
	backupSlot = slot{supplier.RoleBackup, fieldBackupSupplier, msgBackupSupplier}
)

// buildLink builds the supplier item of one slot. It returns nil and no error
// when the slot is empty (link nil). A "keep" takes the cost the item has
// from the same supplier, wherever that supplier sits now.
func buildLink(current sourcing, link *SupplierLink, sl slot) (*supplier.SupplierItem, error) {
	if link == nil {
		return nil, nil
	}
	if link.Supplier.IsZero() {
		return nil, prefixFields(fieldError(fieldSupplierID, sl.missing), sl.field)
	}
	cost := link.Cost.resolve(current.costFor(link.Supplier))
	item, err := supplier.NewSupplierItem(link.Supplier, sl.role, link.LeadDays, cost)
	// The supplier package refuses a negative cost; only the upper bound is
	// left to check here.
	if cost != nil {
		err = shared.MergeValidation(err, checkAmount(fieldCostCents, *cost))
	}
	if err != nil {
		return nil, prefixFields(err, sl.field)
	}
	return &item, nil
}

// checkCost reports a ValidationError on field for a cost above the largest
// amount. The supplier package already refuses a negative supplier cost; for
// the production cost the negative case is reported here too.
func checkCost(field string, cost *shared.Money) error {
	if cost == nil {
		return nil
	}
	if cost.IsNegative() {
		return fieldError(field, msgCostNegative)
	}
	return checkAmount(field, *cost)
}

func checkCurrency(cost *shared.Money, currency string) error {
	if cost != nil && !sameCurrency(*cost, currency) {
		return fmt.Errorf("cost in %q, item in %q: %w", cost.Currency(), currency, ErrInvalidItem)
	}
	return nil
}
