package catalog

import (
	"fmt"
	"math"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

const (
	maxKitItems     = 50
	maxQuantity     = 99
	maxKitDays      = 60
	maxAlternatives = 50
)

// SwapMode says what a client's swap does to a kit's price (ADR 0023, per
// kit). The zero value means SwapKeepDiscount, the default.
type SwapMode string

// The swap modes of a kit.
const (
	// SwapKeepDiscount prices a swap as kit price − removed item + new item,
	// both at single-item price: the kit's discount stays.
	SwapKeepDiscount SwapMode = "keep"
	// SwapLoseDiscount prices a swapped kit as the sum of its resulting
	// items' single-item prices: the discount is lost.
	SwapLoseDiscount SwapMode = "lose"
)

// ItemFacts is what a kit needs to know about one catalog item. The app
// layer reads it from the items (Item.Facts) and passes it in, because a kit
// cannot know it alone.
type ItemFacts struct {
	Price     shared.Money
	ReadyDays int
	Archived  bool
	// Cost is the item's cost, owner only; nil when none is recorded.
	Cost *shared.Money
}

// ItemLookup holds the facts of every item a kit refers to, members and
// alternatives. A method that needs an item missing from it returns
// ErrItemMissing.
type ItemLookup map[shared.ItemID]ItemFacts

// KitItem is one item of a kit: how many of it, whether the client may swap
// it, and the items the owner allows in its place ("Pode trocar por").
type KitItem struct {
	Item         shared.ItemID
	Quantity     int
	Swappable    bool
	Alternatives []shared.ItemID
}

// KitInput is everything the contract's KitInput replaces at once. Price is
// in the store's currency. SwapMode may be empty for the default. A nil
// AssemblyDays or SafetyMarginDays uses the store's default.
type KitInput struct {
	Name             string
	Description      string
	Price            shared.Money
	SwapMode         SwapMode
	AssemblyDays     *int
	SafetyMarginDays *int
	Items            []KitItem
	AttributeValues  []shared.AttributeValueID
}

// kitDetails is the validated form of KitInput.
type kitDetails struct {
	name        string
	description string
	price       shared.Money
	swapMode    SwapMode
	assembly    *int
	margin      *int
	items       []KitItem
	attributes  []shared.AttributeValueID
}

// Kit is a bundle of catalog items sold at one price, usually below the sum
// of its items. Fields are unexported; state changes only through methods.
// Totals, days, cost and margin need the facts of its items, which the
// caller passes as an ItemLookup.
type Kit struct {
	tenant shared.TenantID
	id     shared.KitID
	kitDetails
	published bool
	archived  bool
	version   int
	photoSet
}

// NewKit returns an unpublished kit at version 1. The lookup holds the facts
// of every item the input names. Names and descriptions are trimmed. It
// returns a ValidationError listing every invalid field: "name",
// "description", "priceCents", "swapDiscount", "assemblyDays" and
// "safetyMarginDays" (0 to 60), "attributeValueIds", "items" (1 to 50), and
// for the item at index i "items[i].itemId" (in the lookup, once in the kit),
// "items[i].quantity" (1 to 99) and "items[i].alternativeItemIds" (at least
// one when swappable, none otherwise, each in the lookup, never the item
// itself, no repeats). Only when those are valid does it return an
// *ArchivedItemsError, matching ErrItemArchived, listing the archived items
// the kit holds or offers. It returns ErrInvalidKit for a zero tenant or id,
// or a price without a currency.
func NewKit(tenant shared.TenantID, id shared.KitID, in KitInput, lookup ItemLookup) (*Kit, error) {
	if tenant.IsZero() || id.IsZero() {
		return nil, fmt.Errorf("new kit %v: %w", id, ErrInvalidKit)
	}
	d, err := buildKit(in, lookup)
	if err != nil {
		return nil, err
	}
	return &Kit{tenant: tenant, id: id, kitDetails: d, version: 1}, nil
}

func buildKit(in KitInput, lookup ItemLookup) (kitDetails, error) {
	if in.Price.Currency() == "" {
		return kitDetails{}, fmt.Errorf("kit price has no currency: %w", ErrInvalidKit)
	}
	name := strings.TrimSpace(in.Name)
	description := strings.TrimSpace(in.Description)
	mode := in.SwapMode
	if mode == "" {
		mode = SwapKeepDiscount
	}
	items, ierr := buildKitItems(in.Items, lookup)
	errs := []error{
		kitNameRule.check(name),
		checkDescription(description),
		checkKitPrice(in.Price),
		checkSwapMode(mode),
		checkDaysOverride(fieldAssemblyDays, in.AssemblyDays),
		checkDaysOverride(fieldSafetyMargin, in.SafetyMarginDays),
		checkAttributeValues(in.AttributeValues, ErrInvalidKit),
		ierr,
	}
	if err := shared.MergeValidation(errs...); err != nil {
		return kitDetails{}, err
	}
	if archived := archivedIn(items, lookup); len(archived) > 0 {
		return kitDetails{}, &ArchivedItemsError{Items: archived}
	}
	return kitDetails{
		name: name, description: description, price: in.Price, swapMode: mode,
		assembly: cloneInt(in.AssemblyDays), margin: cloneInt(in.SafetyMarginDays),
		items: items, attributes: cloneSlice(in.AttributeValues),
	}, nil
}

func checkKitPrice(price shared.Money) error {
	if price.Cents() <= 0 {
		return fieldError(fieldPrice, msgKitPrice)
	}
	return checkAmount(fieldPrice, price)
}

func checkSwapMode(m SwapMode) error {
	if m != SwapKeepDiscount && m != SwapLoseDiscount {
		return fieldError(fieldSwapDiscount, msgSwapMode)
	}
	return nil
}

func checkDaysOverride(field string, days *int) error {
	if days != nil && (*days < 0 || *days > maxKitDays) {
		return fieldError(field, msgDays0To60)
	}
	return nil
}

func cloneInt(v *int) *int {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// buildKitItems validates the items and their alternatives, and returns
// copies. An item or alternative missing from the lookup is a field error.
func buildKitItems(in []KitItem, lookup ItemLookup) ([]KitItem, error) {
	switch {
	case len(in) == 0:
		return nil, fieldError(fieldItems, msgNoKitItems)
	case len(in) > maxKitItems:
		return nil, fieldError(fieldItems, msgTooManyKitItems)
	}
	items := make([]KitItem, len(in))
	seen := make(map[shared.ItemID]struct{}, len(in))
	var errs []error
	for i, ki := range in {
		err := checkKitItem(ki, lookup)
		if _, dup := seen[ki.Item]; dup {
			err = shared.MergeValidation(err, fieldError(fieldItemID, msgKitItemRepeated))
		}
		seen[ki.Item] = struct{}{}
		if err != nil {
			errs = append(errs, prefixFields(err, at(fieldItems, i)))
		}
		items[i] = KitItem{Item: ki.Item, Quantity: ki.Quantity, Swappable: ki.Swappable, Alternatives: cloneSlice(ki.Alternatives)}
	}
	if err := shared.MergeValidation(errs...); err != nil {
		return nil, err
	}
	return items, nil
}

func checkKitItem(ki KitItem, lookup ItemLookup) error {
	var errs []error
	if _, ok := lookup[ki.Item]; !ok {
		errs = append(errs, fieldError(fieldItemID, msgKitItemUnknown))
	}
	if ki.Quantity < 1 || ki.Quantity > maxQuantity {
		errs = append(errs, fieldError(fieldQuantity, msgQuantity))
	}
	return shared.MergeValidation(append(errs, checkAlternatives(ki, lookup))...)
}

func checkAlternatives(ki KitItem, lookup ItemLookup) error {
	alts := ki.Alternatives
	switch {
	case !ki.Swappable && len(alts) > 0:
		return fieldError(fieldAlternatives, msgAlternativesOff)
	case ki.Swappable && len(alts) == 0:
		return fieldError(fieldAlternatives, msgNoAlternatives)
	case len(alts) > maxAlternatives:
		return fieldError(fieldAlternatives, msgTooManyAlts)
	}
	var errs []error
	seen := make(map[shared.ItemID]struct{}, len(alts))
	for j, alt := range alts {
		field := at(fieldAlternatives, j)
		_, known := lookup[alt]
		_, dup := seen[alt]
		seen[alt] = struct{}{}
		switch {
		case alt == ki.Item:
			errs = append(errs, fieldError(field, msgAlternativeSelf))
		case !known:
			errs = append(errs, fieldError(field, msgKitItemUnknown))
		case dup:
			errs = append(errs, fieldError(field, msgAlternativeDup))
		}
	}
	return shared.MergeValidation(errs...)
}

// archivedIn lists the archived items among the members and alternatives, in
// kit order, each once.
func archivedIn(items []KitItem, lookup ItemLookup) []shared.ItemID {
	var out []shared.ItemID
	seen := make(map[shared.ItemID]struct{})
	note := func(id shared.ItemID) {
		if _, done := seen[id]; done || !lookup[id].Archived {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, ki := range items {
		note(ki.Item)
		for _, alt := range ki.Alternatives {
			note(alt)
		}
	}
	return out
}

// KitRecord is a kit as a repository stores it.
type KitRecord struct {
	Tenant           shared.TenantID
	ID               shared.KitID
	Name             string
	Description      string
	Price            shared.Money
	SwapMode         SwapMode
	AssemblyDays     *int
	SafetyMarginDays *int
	Items            []KitItem
	Attributes       []shared.AttributeValueID
	Photos           []Photo
	Published        bool
	Archived         bool
	Version          int
}

// RestoreKit rebuilds a stored kit. The lookup is checked for unknown items
// only, never for archived ones: an item may be archived after the kit was
// saved (the item's archive check only looks at kits that are not archived
// at that moment), and such a kit must stay readable. It can be saved again
// only once the item is replaced, and a swap to an archived alternative is
// refused by PriceWithSwaps. It returns ErrInvalidKit
// when the record breaks a rule a stored kit always keeps: a version below
// 1, text that is not trimmed, an empty swap mode, an archived kit that is
// published, or anything NewKit refuses.
func RestoreKit(r KitRecord, lookup ItemLookup) (*Kit, error) {
	bad := r.Version < 1 || (r.Archived && r.Published) || r.SwapMode == "" ||
		r.Name != strings.TrimSpace(r.Name) || r.Description != strings.TrimSpace(r.Description)
	if bad {
		return nil, fmt.Errorf("restore kit %v: %w", r.ID, ErrInvalidKit)
	}
	in := KitInput{
		Name: r.Name, Description: r.Description, Price: r.Price, SwapMode: r.SwapMode,
		AssemblyDays: r.AssemblyDays, SafetyMarginDays: r.SafetyMarginDays, Items: r.Items,
		AttributeValues: r.Attributes,
	}
	k, err := NewKit(r.Tenant, r.ID, in, withoutArchived(lookup))
	if err != nil {
		// The cause is text only: wrapping a ValidationError would make a bad
		// stored row match shared.ErrInvalid and answer 400, not 500.
		return nil, fmt.Errorf("restore kit %v: %w: %s", r.ID, ErrInvalidKit, err.Error())
	}
	photos, err := restorePhotos(r.Photos)
	if err != nil {
		return nil, fmt.Errorf("restore kit %v: %w: %s", r.ID, ErrInvalidKit, err.Error())
	}
	k.photoSet, k.published, k.archived, k.version = photos, r.Published, r.Archived, r.Version
	return k, nil
}

// withoutArchived returns lookup with every item marked as not archived: a
// stored kit keeps the items it held, even once they are archived.
func withoutArchived(lookup ItemLookup) ItemLookup {
	out := make(ItemLookup, len(lookup))
	for id, f := range lookup {
		f.Archived = false
		out[id] = f
	}
	return out
}

// TenantID returns the store the kit belongs to.
func (k *Kit) TenantID() shared.TenantID { return k.tenant }

// ID returns the kit's id.
func (k *Kit) ID() shared.KitID { return k.id }

// Name returns the kit's name, trimmed.
func (k *Kit) Name() string { return k.name }

// Description returns the kit's description, trimmed; empty when there is
// none.
func (k *Kit) Description() string { return k.description }

// Price returns the kit's price, above zero.
func (k *Kit) Price() shared.Money { return k.price }

// SwapMode returns what a client's swap does to the price.
func (k *Kit) SwapMode() SwapMode { return k.swapMode }

// AssemblyDays returns the kit's own assembly days, and false when it uses
// the store's default.
func (k *Kit) AssemblyDays() (int, bool) { return override(k.assembly) }

// SafetyMarginDays returns the kit's own safety margin, and false when it
// uses the store's default.
func (k *Kit) SafetyMarginDays() (int, bool) { return override(k.margin) }

func override(v *int) (int, bool) {
	if v == nil {
		return 0, false
	}
	return *v, true
}

// Items returns the kit's items, in order, with their alternatives. The
// result is a deep copy.
func (k *Kit) Items() []KitItem {
	out := make([]KitItem, len(k.items))
	for i, ki := range k.items {
		out[i] = KitItem{Item: ki.Item, Quantity: ki.Quantity, Swappable: ki.Swappable, Alternatives: cloneSlice(ki.Alternatives)}
	}
	return out
}

// AttributeValueIDs returns the kit's values of the store's item fields. The
// result is a copy.
func (k *Kit) AttributeValueIDs() []shared.AttributeValueID { return cloneSlice(k.attributes) }

// Published reports whether the kit shows on the store page. It is never true
// for an archived kit.
func (k *Kit) Published() bool { return k.published }

// Archived reports whether the kit was archived.
func (k *Kit) Archived() bool { return k.archived }

// Version returns the version the kit was read or created at.
func (k *Kit) Version() int { return k.version }

// CheckVersion returns shared.ErrVersionConflict unless expected is the
// version the kit was read at.
func (k *Kit) CheckVersion(expected int) error {
	if expected != k.version {
		return shared.ErrVersionConflict
	}
	return nil
}

// Replace replaces everything the contract's KitInput holds; an empty
// SwapMode means the default, keep. It returns the errors of NewKit, so a kit
// holding an archived item can be saved only once the item is replaced;
// nothing changes then.
func (k *Kit) Replace(in KitInput, lookup ItemLookup) error {
	d, err := buildKit(in, lookup)
	if err != nil {
		return err
	}
	k.kitDetails = d
	return nil
}

// Publish shows the kit on the store page. It returns ErrPublishArchived for
// an archived kit.
func (k *Kit) Publish() error {
	if k.archived {
		return fmt.Errorf("publish kit %v: %w", k.id, ErrPublishArchived)
	}
	k.published = true
	return nil
}

// Unpublish hides the kit from the store page.
func (k *Kit) Unpublish() { k.published = false }

// Archive hides the kit and unpublishes it.
func (k *Kit) Archive() {
	k.archived = true
	k.published = false
}

// Restore brings an archived kit back, unpublished. It returns an
// *ArchivedItemsError, matching ErrItemArchived, when an item of the kit or
// one of its alternatives has been archived meanwhile, and ErrItemMissing
// when the lookup lacks an item; the kit stays archived then.
func (k *Kit) Restore(lookup ItemLookup) error {
	if err := k.requireItems(lookup); err != nil {
		return err
	}
	if archived := archivedIn(k.items, lookup); len(archived) > 0 {
		return &ArchivedItemsError{Items: archived}
	}
	k.archived = false
	return nil
}

// requireItems returns ErrItemMissing unless the lookup holds every member
// and alternative.
func (k *Kit) requireItems(lookup ItemLookup) error {
	for _, ki := range k.items {
		for _, id := range append([]shared.ItemID{ki.Item}, ki.Alternatives...) {
			if _, ok := lookup[id]; !ok {
				return fmt.Errorf("kit %v needs item %v: %w", k.id, id, ErrItemMissing)
			}
		}
	}
	return nil
}

// line is one item and how many of it a kit, or a swapped kit, holds.
type line struct {
	item shared.ItemID
	qty  int
}

func (k *Kit) lines() []line {
	out := make([]line, len(k.items))
	for i, ki := range k.items {
		out[i] = line{ki.Item, ki.Quantity}
	}
	return out
}

// facts returns the item's facts, or ErrItemMissing.
func (l ItemLookup) facts(id shared.ItemID) (ItemFacts, error) {
	f, ok := l[id]
	if !ok {
		return ItemFacts{}, fmt.Errorf("item %v: %w", id, ErrItemMissing)
	}
	return f, nil
}

// times returns m × n for a quantity n of at least 1. It returns
// shared.ErrMoneyOverflow when the product does not fit in int64 cents.
func times(m shared.Money, n int) (shared.Money, error) {
	limit := int64(math.MaxInt64) / int64(n)
	if m.Cents() > limit || m.Cents() < -limit {
		return shared.Money{}, shared.ErrMoneyOverflow
	}
	return shared.NewMoney(m.Cents()*int64(n), m.Currency())
}

// sumLines adds pick(item) × quantity over the lines. It returns nil and no
// error when pick returns nil for any item (a cost that is not recorded).
func sumLines(lines []line, lookup ItemLookup, pick func(ItemFacts) *shared.Money) (*shared.Money, error) {
	var total *shared.Money
	for _, l := range lines {
		f, err := lookup.facts(l.item)
		if err != nil {
			return nil, err
		}
		unit := pick(f)
		if unit == nil {
			return nil, nil
		}
		part, err := times(*unit, l.qty)
		if err != nil {
			return nil, err
		}
		if total == nil {
			total = &part
			continue
		}
		sum, err := total.Add(part)
		if err != nil {
			return nil, err
		}
		total = &sum
	}
	return total, nil
}

func priceOf(f ItemFacts) *shared.Money { return &f.Price }
func costOf(f ItemFacts) *shared.Money  { return f.Cost }

// SingleItemsTotal returns "Soma dos itens avulsos": the sum of each item's
// price × quantity. It returns ErrItemMissing when the lookup lacks an item,
// and shared.ErrCurrencyMismatch when an item is priced in another currency.
func (k *Kit) SingleItemsTotal(lookup ItemLookup) (shared.Money, error) {
	return k.singleTotal(k.lines(), lookup)
}

// singleTotal adds the single-item prices of lines. The total must be in the
// kit's currency, so a kit never charges a price made of mixed currencies.
func (k *Kit) singleTotal(lines []line, lookup ItemLookup) (shared.Money, error) {
	total, err := sumLines(lines, lookup, priceOf)
	if err != nil {
		return shared.Money{}, err
	}
	if total == nil {
		return shared.Money{}, fmt.Errorf("total of no items: %w", ErrInvalidKit)
	}
	if total.Currency() != k.price.Currency() {
		return shared.Money{}, shared.ErrCurrencyMismatch
	}
	return *total, nil
}

// Discount returns "Desconto do kit": SingleItemsTotal − Price. It is
// negative when the kit costs more than its items (the product shows a
// warning, without blocking). Errors are those of SingleItemsTotal.
func (k *Kit) Discount(lookup ItemLookup) (shared.Money, error) {
	total, err := k.SingleItemsTotal(lookup)
	if err != nil {
		return shared.Money{}, err
	}
	return total.Sub(k.price)
}

// KitDays is "Prazo calculado": the slowest item's ready days, the assembly
// days and the safety margin, and their sum.
type KitDays struct {
	SlowestItem  int
	Assembly     int
	SafetyMargin int
	Total        int
}

// Days returns the days the kit needs: the slowest item's ready days + the
// assembly days + the safety margin, through schedule.Days. The kit's own
// assembly days and margin win over the store's defaults. It returns
// ErrItemMissing when the lookup lacks an item and schedule.ErrNegativeDays
// for a negative default.
//
// Product rule: delivery = latest item ready date + assembly days + safety
// margin (docs/product/design-document.md, Deadline formula).
func (k *Kit) Days(lookup ItemLookup, defaultAssembly, defaultMargin int) (KitDays, error) {
	assembly, margin := k.effectiveDays(defaultAssembly, defaultMargin)
	return daysOf(k.lines(), lookup, assembly, margin)
}

// effectiveDays returns the kit's own assembly days and safety margin where
// it has them, and the store's defaults where it does not.
func (k *Kit) effectiveDays(defaultAssembly, defaultMargin int) (assembly, margin int) {
	assembly, margin = defaultAssembly, defaultMargin
	if v, ok := k.AssemblyDays(); ok {
		assembly = v
	}
	if v, ok := k.SafetyMarginDays(); ok {
		margin = v
	}
	return assembly, margin
}

func daysOf(lines []line, lookup ItemLookup, assembly, margin int) (KitDays, error) {
	ready := make([]int, len(lines))
	slowest := 0
	for i, l := range lines {
		f, err := lookup.facts(l.item)
		if err != nil {
			return KitDays{}, err
		}
		ready[i] = f.ReadyDays
		slowest = max(slowest, f.ReadyDays)
	}
	total, err := schedule.Days(ready, assembly, margin)
	if err != nil {
		return KitDays{}, err
	}
	return KitDays{SlowestItem: slowest, Assembly: assembly, SafetyMargin: margin, Total: total}, nil
}

// Cost returns what the kit costs the store: the sum of each item's cost ×
// quantity. It returns nil and no error when any item has no cost recorded.
// Owner only (ADR 0025). Errors are those of SingleItemsTotal.
func (k *Kit) Cost(lookup ItemLookup) (*shared.Money, error) {
	return sumLines(k.lines(), lookup, costOf)
}

// Margin returns the kit's price minus Cost, and false when there is no
// cost. Owner only. See MarginOf for the rounding.
func (k *Kit) Margin(lookup ItemLookup) (Margin, bool, error) {
	cost, err := k.Cost(lookup)
	if err != nil || cost == nil {
		return Margin{}, false, err
	}
	m, ok := MarginOf(k.price, *cost)
	return m, ok, nil
}
