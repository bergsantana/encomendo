package catalog

import (
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// CostField is one cost field of a write as the client sent it, in the three
// states of the contract (ADR 0025), built only by its constructors:
//   - KeepCost: the key is absent; keep the stored cost;
//   - ClearCost: the key is sent as null; clear it;
//   - SetCost: the key is sent with an amount; set it, in the store's
//     currency.
//
// The zero value is KeepCost. A handler uses ClearCost or SetCost whenever
// the JSON key is present, even with null, because a role that does not see
// costs is refused for sending the key at all.
type CostField struct {
	sent  bool
	cents *int64
}

// KeepCost is the absent cost field.
func KeepCost() CostField { return CostField{} }

// ClearCost is a cost field sent as null.
func ClearCost() CostField { return CostField{sent: true} }

// SetCost is a cost field sent with an amount in cents.
func SetCost(cents int64) CostField { return CostField{sent: true, cents: &cents} }

// Sent reports whether the key was in the request, with a value or null.
func (c CostField) Sent() bool { return c.sent }

// change returns the domain's cost change in currency.
func (c CostField) change(currency string) (catalog.CostChange, error) {
	switch {
	case !c.sent:
		return catalog.KeepCost(), nil
	case c.cents == nil:
		return catalog.ClearCost(), nil
	}
	m, err := shared.NewMoney(*c.cents, currency)
	if err != nil {
		return catalog.CostChange{}, err
	}
	return catalog.SetCost(m), nil
}

// CreateCategoryInput adds a category at the end of the list (POST
// /api/v1/admin/catalog/categories).
type CreateCategoryInput struct {
	Name string
}

// UpdateCategoryInput renames a category, moves it, or both (PATCH
// /api/v1/admin/catalog/categories/{id}). A nil field is left as it is. A
// position past the end moves the category to the end.
type UpdateCategoryInput struct {
	ID       shared.CategoryID
	Name     *string
	Position *int
}

// ValueInput is one value of an item field. A zero ID adds a value; the id
// of one of the field's values keeps it, with the items and kits that
// picked it. Any other id is treated as a new value.
type ValueInput struct {
	ID    shared.AttributeValueID
	Label string
}

// AttributeInput is an item field's name and its ordered values (POST
// /api/v1/admin/catalog/attributes, the body of PUT).
type AttributeInput struct {
	Name   string
	Values []ValueInput
}

// ReplaceAttributeInput replaces a field (PUT
// /api/v1/admin/catalog/attributes/{id}). A value left out leaves every item
// and kit that picked it.
type ReplaceAttributeInput struct {
	ID        shared.AttributeID
	Attribute AttributeInput
}

// SupplierInput is the main or backup supplier of an item.
type SupplierInput struct {
	SupplierID shared.SupplierID
	LeadDays   int
	Cost       CostField
}

// ChoiceInput is one choice of a customization option. A zero ID, or one
// the option does not have, adds a choice.
type ChoiceInput struct {
	ID              shared.ChoiceID
	Label           string
	ExtraPriceCents int64
	ExtraDays       int
}

// OptionInput is one customization option of an item. A zero ID, or one the
// item does not have, adds an option; the id of one of the item's options
// keeps it.
type OptionInput struct {
	ID       shared.OptionID
	Name     string
	Required bool
	Choices  []ChoiceInput
}

// ItemInput is the contract's ItemInput: everything about an item but its
// photos. Prices are in the store's currency. ProductionDays is 0 when the
// field is absent or null. CategoryID is zero for "Sem categoria".
type ItemInput struct {
	Name              string
	Description       string
	CategoryID        shared.CategoryID
	SourcingMode      catalog.SourcingMode
	PriceCents        int64
	ProductionDays    int
	ProductionCost    CostField
	MainSupplier      *SupplierInput
	BackupSupplier    *SupplierInput
	AttributeValueIDs []shared.AttributeValueID
	Options           []OptionInput
	Published         bool
}

// sendsCosts reports whether any cost field was sent, even as null.
func (in ItemInput) sendsCosts() bool {
	for _, s := range []*SupplierInput{in.MainSupplier, in.BackupSupplier} {
		if s != nil && s.Cost.sent {
			return true
		}
	}
	return in.ProductionCost.sent
}

// ReplaceItemInput replaces an item at the version read (PUT
// /api/v1/admin/catalog/items/{id}).
type ReplaceItemInput struct {
	ID      shared.ItemID
	Item    ItemInput
	Version int
}

// SetItemPublishedInput shows or hides an item on the store page.
type SetItemPublishedInput struct {
	ID        shared.ItemID
	Published bool
	Version   int
}

// ItemVersionInput names an item at the version read, for archive and
// restore.
type ItemVersionInput struct {
	ID      shared.ItemID
	Version int
}

// KitInput is the contract's KitInput. A nil AssemblyDays or
// SafetyMarginDays uses the store's default; an empty SwapDiscount means
// keep.
type KitInput struct {
	Name              string
	Description       string
	PriceCents        int64
	SwapDiscount      catalog.SwapMode
	AssemblyDays      *int
	SafetyMarginDays  *int
	Items             []catalog.KitItem
	AttributeValueIDs []shared.AttributeValueID
	Published         bool
}

// ReplaceKitInput replaces a kit at the version read (PUT
// /api/v1/admin/catalog/kits/{id}).
type ReplaceKitInput struct {
	ID      shared.KitID
	Kit     KitInput
	Version int
}

// SetKitPublishedInput shows or hides a kit on the store page.
type SetKitPublishedInput struct {
	ID        shared.KitID
	Published bool
	Version   int
}

// KitVersionInput names a kit at the version read, for archive and restore.
type KitVersionInput struct {
	ID      shared.KitID
	Version int
}
