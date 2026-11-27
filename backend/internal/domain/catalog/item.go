package catalog

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// ItemDetails is what a person edits about an item apart from how it is
// sourced. Category is the zero CategoryID for "Sem categoria". Price is in
// the store's currency, which the caller reads from the store's settings.
type ItemDetails struct {
	Name            string
	Description     string
	Category        shared.CategoryID
	Price           shared.Money
	AttributeValues []shared.AttributeValueID
	Options         []OptionInput
}

// ItemInput is everything the contract's ItemInput replaces at once: the
// details and the sourcing.
type ItemInput struct {
	ItemDetails
	Sourcing SourcingInput
}

// details is the validated form of ItemDetails.
type details struct {
	name        string
	description string
	category    shared.CategoryID
	price       shared.Money
	attributes  []shared.AttributeValueID
	options     []CustomizationOption
}

// Item is one thing the store sells: what it is, what it costs the client,
// how the store gets it, and what the client may customize. Fields are
// unexported; state changes only through methods, which keep every rule
// below true.
//
// Costs and margins are the owner's only (ADR 0025): Cost and Margin compute
// them, and the app layer sends them to the owner and nobody else.
type Item struct {
	tenant shared.TenantID
	id     shared.ItemID
	details
	src       sourcing
	published bool
	archived  bool
	version   int
	photoSet
}

// NewItem returns an unpublished item at version 1. Names and descriptions
// are trimmed. It returns a ValidationError listing every invalid field,
// named as in the contract: "name", "description", "priceCents" (above zero
// and not above R$ 1.000.000.000,00), "attributeValueIds", "options[i]..."
// (see NewOption), "sourcingMode", and the sourcing fields ("mainSupplier.…",
// "backupSupplier.…", "productionDays", "productionCostCents"; see
// ReplaceSourcing). A cost in SetCost counts as a new cost; there is none to
// keep yet. It returns ErrInvalidItem for a zero tenant or id, a price or
// cost without a currency or in another currency, and the errors of
// NewOption and supplier.NewSupplierItem for a bug in their ids.
func NewItem(tenant shared.TenantID, id shared.ItemID, in ItemInput) (*Item, error) {
	if tenant.IsZero() || id.IsZero() {
		return nil, fmt.Errorf("new item %v: %w", id, ErrInvalidItem)
	}
	d, derr := buildDetails(in.ItemDetails)
	if derr != nil && !isValidation(derr) {
		return nil, derr
	}
	src, serr := buildSourcing(sourcing{}, in.Sourcing, in.Price.Currency())
	if err := shared.MergeValidation(derr, serr); err != nil {
		return nil, err
	}
	return &Item{tenant: tenant, id: id, details: d, src: src, version: 1}, nil
}

func isValidation(err error) bool {
	var ve *shared.ValidationError
	return errors.As(err, &ve)
}

func buildDetails(d ItemDetails) (details, error) {
	if d.Price.Currency() == "" {
		return details{}, fmt.Errorf("item price has no currency: %w", ErrInvalidItem)
	}
	name := strings.TrimSpace(d.Name)
	description := strings.TrimSpace(d.Description)
	options, oerr := buildOptions(d.Options, d.Price.Currency())
	errs := []error{
		itemNameRule.check(name),
		checkDescription(description),
		checkPrice(d.Price),
		checkAttributeValues(d.AttributeValues, ErrInvalidItem),
		oerr,
	}
	if err := shared.MergeValidation(errs...); err != nil {
		return details{}, err
	}
	return details{
		name: name, description: description, category: d.Category, price: d.Price,
		attributes: cloneSlice(d.AttributeValues), options: options,
	}, nil
}

func checkPrice(price shared.Money) error {
	if price.Cents() <= 0 {
		return fieldError(fieldPrice, msgItemPrice)
	}
	return checkAmount(fieldPrice, price)
}

// SourcingRecord is an item's sourcing as a repository stores it.
type SourcingRecord struct {
	Mode           SourcingMode
	ProductionDays int
	ProductionCost *shared.Money
	Main           *supplier.SupplierItem
	Backup         *supplier.SupplierItem
}

// ItemRecord is an item as a repository stores it.
type ItemRecord struct {
	Tenant      shared.TenantID
	ID          shared.ItemID
	Name        string
	Description string
	Category    shared.CategoryID
	Price       shared.Money
	Sourcing    SourcingRecord
	Attributes  []shared.AttributeValueID
	Options     []OptionInput
	Photos      []Photo
	Published   bool
	Archived    bool
	Version     int
}

// RestoreItem rebuilds a stored item. It returns ErrInvalidItem when the
// record breaks a rule a stored item always keeps: a version below 1, text
// that is not trimmed, an archived item that is published, or anything
// NewItem refuses.
func RestoreItem(r ItemRecord) (*Item, error) {
	bad := r.Version < 1 || (r.Archived && r.Published) ||
		r.Name != strings.TrimSpace(r.Name) || r.Description != strings.TrimSpace(r.Description)
	if bad {
		return nil, fmt.Errorf("restore item %v: %w", r.ID, ErrInvalidItem)
	}
	in, err := recordInput(r)
	if err != nil {
		return nil, err
	}
	i, err := NewItem(r.Tenant, r.ID, in)
	if err != nil {
		// The cause is text only: wrapping a ValidationError would make a bad
		// stored row match shared.ErrInvalid and answer 400, not 500.
		return nil, fmt.Errorf("restore item %v: %w: %s", r.ID, ErrInvalidItem, err.Error())
	}
	photos, err := restorePhotos(r.Photos)
	if err != nil {
		return nil, fmt.Errorf("restore item %v: %w: %s", r.ID, ErrInvalidItem, err.Error())
	}
	i.photoSet, i.published, i.archived, i.version = photos, r.Published, r.Archived, r.Version
	return i, nil
}

// recordInput turns a stored item into the input that rebuilds it: every
// cost is set to what is stored, nothing is kept.
func recordInput(r ItemRecord) (ItemInput, error) {
	s := r.Sourcing
	in := ItemInput{
		ItemDetails: ItemDetails{
			Name: r.Name, Description: r.Description, Category: r.Category, Price: r.Price,
			AttributeValues: r.Attributes, Options: r.Options,
		},
		Sourcing: SourcingInput{Mode: s.Mode, ProductionDays: s.ProductionDays, ProductionCost: fromStored(s.ProductionCost)},
	}
	for _, slot := range []struct {
		stored *supplier.SupplierItem
		role   supplier.Role
		dst    **SupplierLink
	}{{s.Main, supplier.RoleMain, &in.Sourcing.Main}, {s.Backup, supplier.RoleBackup, &in.Sourcing.Backup}} {
		if slot.stored == nil {
			continue
		}
		if slot.stored.Role() != slot.role {
			return ItemInput{}, fmt.Errorf("restore item %v: supplier item in the wrong slot: %w", r.ID, ErrInvalidItem)
		}
		*slot.dst = &SupplierLink{Supplier: slot.stored.SupplierID(), LeadDays: slot.stored.LeadDays(), Cost: fromStored(slot.stored.Cost())}
	}
	return in, nil
}

// TenantID returns the store the item belongs to.
func (i *Item) TenantID() shared.TenantID { return i.tenant }

// ID returns the item's id.
func (i *Item) ID() shared.ItemID { return i.id }

// Name returns the item's name, trimmed.
func (i *Item) Name() string { return i.name }

// Description returns the item's description, trimmed; empty when there is
// none.
func (i *Item) Description() string { return i.description }

// CategoryID returns the item's category, or the zero CategoryID for "Sem
// categoria".
func (i *Item) CategoryID() shared.CategoryID { return i.category }

// Price returns the price the client pays, above zero, in the store's
// currency.
func (i *Item) Price() shared.Money { return i.price }

// SourcingMode returns how the store gets the item.
func (i *Item) SourcingMode() SourcingMode { return i.src.mode }

// ProductionDays returns the days the store takes to make an in_house item,
// 1 to 365, and 0 for the other modes.
func (i *Item) ProductionDays() int { return i.src.productionDays }

// MainSupplier returns the supplier the store buys from, and false for an
// in_house item.
func (i *Item) MainSupplier() (supplier.SupplierItem, bool) { return linkOf(i.src.main) }

// BackupSupplier returns the fornecedor reserva, and false when there is
// none.
func (i *Item) BackupSupplier() (supplier.SupplierItem, bool) { return linkOf(i.src.backup) }

func linkOf(l *supplier.SupplierItem) (supplier.SupplierItem, bool) {
	if l == nil {
		return supplier.SupplierItem{}, false
	}
	return *l, true
}

// AttributeValueIDs returns the item's values of the store's item fields. The
// result is a copy.
func (i *Item) AttributeValueIDs() []shared.AttributeValueID { return cloneSlice(i.attributes) }

// Options returns the customization options, in order. The result is a copy.
func (i *Item) Options() []CustomizationOption { return cloneSlice(i.options) }

// Published reports whether the item shows on the store page ("No portal").
// It is never true for an archived item.
func (i *Item) Published() bool { return i.published }

// Archived reports whether the item was archived: hidden from the store page
// and from the pickers, though kits and history still resolve it.
func (i *Item) Archived() bool { return i.archived }

// Version returns the version the item was read or created at. A
// repository's update stores the next one, and only while the stored version
// is still this one.
func (i *Item) Version() int { return i.version }

// CheckVersion returns shared.ErrVersionConflict unless expected is the
// version the item was read at.
func (i *Item) CheckVersion(expected int) error {
	if expected != i.version {
		return shared.ErrVersionConflict
	}
	return nil
}

// ReadyDays returns the days the item needs before it is ready: the main
// supplier's lead time for to_order and stock, the production days for
// in_house (ADR 0025).
func (i *Item) ReadyDays() int {
	if i.src.mode == ModeInHouse {
		return i.src.productionDays
	}
	return i.src.main.LeadDays()
}

// Cost returns what the item costs the store: the main supplier's cost, or
// the production cost for in_house; nil when none is recorded. Owner only
// (ADR 0025): the caller decides who receives it. The result is a copy.
func (i *Item) Cost() *shared.Money {
	cost := i.src.productionCost
	if i.src.mode != ModeInHouse {
		cost = i.src.main.Cost()
	}
	if cost == nil {
		return nil
	}
	c := *cost
	return &c
}

// ProductionCost returns the own cost of an in_house item, and nil for other
// modes or when none is recorded. Owner only. The result is a copy.
func (i *Item) ProductionCost() *shared.Money {
	if i.src.mode != ModeInHouse || i.src.productionCost == nil {
		return nil
	}
	c := *i.src.productionCost
	return &c
}

// Margin returns the price minus Cost, and false when there is no cost.
// Owner only. See MarginOf for the rounding.
func (i *Item) Margin() (Margin, bool) {
	cost := i.Cost()
	if cost == nil {
		return Margin{}, false
	}
	return MarginOf(i.price, *cost)
}

// Margin is what a sale leaves after its cost: the difference in cents and
// the same over the price in basis points (hundredths of a percent).
type Margin struct {
	Cents       int64
	BasisPoints int64
}

// MarginOf returns price − cost and that over the price in basis points,
// rounded toward zero; 3000 is 30%. It is negative when the cost is above
// the price. It returns false when the currencies differ or the price is not
// above zero. Amounts are bounded by R$ 1.000.000.000,00 (and a kit holds at
// most 50 × 99 of its items), so the arithmetic cannot overflow.
func MarginOf(price, cost shared.Money) (Margin, bool) {
	if price.Currency() != cost.Currency() || price.Cents() <= 0 {
		return Margin{}, false
	}
	cents := price.Cents() - cost.Cents()
	return Margin{Cents: cents, BasisPoints: cents * 10000 / price.Cents()}, true
}

// Facts returns the item's data a kit needs; see ItemLookup.
func (i *Item) Facts() ItemFacts {
	return ItemFacts{Price: i.price, ReadyDays: i.ReadyDays(), Archived: i.archived, Cost: i.Cost()}
}

// Edit replaces the item's details. It returns the ValidationError of
// NewItem for the details, and ErrInvalidItem when the price is in another
// currency than before; nothing changes then.
func (i *Item) Edit(d ItemDetails) error {
	built, err := buildDetails(d)
	if err != nil {
		return err
	}
	if !sameCurrency(built.price, i.price.Currency()) {
		return fmt.Errorf("edit item %v: price currency changed: %w", i.id, ErrInvalidItem)
	}
	i.details = built
	return nil
}

// ReplaceSourcing replaces the item's sourcing: mode, production days and
// cost, main supplier and backup. A cost in the input is "keep", "clear" or
// "set" (KeepCost, ClearCost, SetCost), and keep takes the item's current
// cost of the same supplier, never of the same slot (ADR 0025):
//   - a supplier the item did not have starts with no cost;
//   - swapping main and backup keeps each supplier's own cost;
//   - a supplier left out loses its cost, and linking it again starts with
//     none;
//   - leaving in_house drops the production cost, and coming back starts
//     with none.
//
// So a write that leaves costs out can never move a cost to another
// supplier. It returns a ValidationError listing every invalid field:
// "sourcingMode"; for in_house, "productionDays" (1 to 365), "mainSupplier"
// and "backupSupplier" (none allowed) and "productionCostCents"; for the
// other modes "mainSupplier.supplierId" (required), "backupSupplier.supplierId"
// (a different supplier), "…leadDays" (0 to 365), "…costCents" and
// "productionDays" / "productionCostCents" (not allowed). It returns
// ErrInvalidItem for a cost in another currency than the price; nothing
// changes then.
func (i *Item) ReplaceSourcing(in SourcingInput) error {
	src, err := buildSourcing(i.src, in, i.price.Currency())
	if err != nil {
		return err
	}
	i.src = src
	return nil
}

// Replace replaces the details and the sourcing together, as the contract's
// replace request does: either both are valid and both change, or nothing
// does. It returns the errors of Edit and ReplaceSourcing, merged.
func (i *Item) Replace(in ItemInput) error {
	d, derr := buildDetails(in.ItemDetails)
	if derr != nil && !isValidation(derr) {
		return derr
	}
	if derr == nil && !sameCurrency(d.price, i.price.Currency()) {
		return fmt.Errorf("replace item %v: price currency changed: %w", i.id, ErrInvalidItem)
	}
	src, serr := buildSourcing(i.src, in.Sourcing, i.price.Currency())
	if err := shared.MergeValidation(derr, serr); err != nil {
		return err
	}
	i.details, i.src = d, src
	return nil
}

// Publish shows the item on the store page. It returns ErrPublishArchived
// for an archived item.
func (i *Item) Publish() error {
	if i.archived {
		return fmt.Errorf("publish item %v: %w", i.id, ErrPublishArchived)
	}
	i.published = true
	return nil
}

// Unpublish hides the item from the store page.
func (i *Item) Unpublish() { i.published = false }

// Archive hides the item from the store page and the pickers, and unpublishes
// it (ADR 0025). Whether kits still hold it is the app layer's check
// (ErrItemInKit).
func (i *Item) Archive() {
	i.archived = true
	i.published = false
}

// Restore brings an archived item back, unpublished: showing it on the store
// page again is a separate choice.
func (i *Item) Restore() { i.archived = false }
