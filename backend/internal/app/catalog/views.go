package catalog

import (
	"context"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// photoURLTTL is how long a signed photo URL in an admin view reads (plan
// 3.9): long enough to render a list, short enough that a leaked URL dies.
const photoURLTTL = 10 * time.Minute

// CategoryView is a category (the Category schema).
type CategoryView struct {
	ID       shared.CategoryID
	Name     string
	Position int
}

// AttributeValueView is one value of an item field, with how many items and
// kits, archived ones included, picked it.
type AttributeValueView struct {
	ID         shared.AttributeValueID
	Label      string
	UsageCount int
}

// AttributeView is an item field (the Attribute schema).
type AttributeView struct {
	ID       shared.AttributeID
	Name     string
	Position int
	Values   []AttributeValueView
}

// ChoiceView is one choice of a customization option.
type ChoiceView struct {
	ID         shared.ChoiceID
	Label      string
	ExtraPrice shared.Money
	ExtraDays  int
}

// OptionView is a customization option (the CustomizationOption schema). It
// holds nothing internal, so the store page uses it too.
type OptionView struct {
	ID       shared.OptionID
	Name     string
	Required bool
	Choices  []ChoiceView
}

// PhotoView is a photo with a signed read URL (the Photo schema).
type PhotoView struct {
	ID        shared.PhotoID
	URL       string
	ExpiresAt time.Time
	Position  int
}

// SupplierItemView is one supplier of an item: who, and its lead time. Its
// cost is in ItemCosts, the owner's part of the item.
type SupplierItemView struct {
	SupplierID shared.SupplierID
	// SupplierName is empty when the supplier is not readable (it cannot
	// be: supplier_items has a foreign key to the store's suppliers).
	SupplierName string
	LeadDays     int
}

// ItemCosts is the owner's part of an item view (ADR 0025). The view holds
// it only for a role that sees costs; for every other role it is nil, so the
// fields are absent, not null. Inside, a nil field means "none recorded" and
// is written as null.
type ItemCosts struct {
	// Cost is the cost the margin uses: the main supplier's, or the
	// production cost for in_house.
	Cost *shared.Money
	// ProductionCost is the own cost of an in_house item. The contract
	// shows it only for in_house items; for the others it is always nil.
	ProductionCost *shared.Money
	// MainSupplierCost and BackupSupplierCost are each supplier's own
	// cost, written in mainSupplier.cost and backupSupplier.cost.
	MainSupplierCost   *shared.Money
	BackupSupplierCost *shared.Money
	// Margin is price − Cost, nil without a cost.
	Margin *catalog.Margin
}

// Costs is the owner's part of an item row, a kit or a kit row: the cost and
// the margin. As ItemCosts, it is nil for any role that does not see costs,
// and a nil field inside means none.
type Costs struct {
	Cost   *shared.Money
	Margin *catalog.Margin
}

// ItemView is one item (the Item schema). ProductionDays is 0 unless the
// item is in_house (null in the contract). CategoryID is zero for "Sem
// categoria". MainSupplier is nil for in_house items, BackupSupplier when
// there is none. Costs is nil unless the caller sees costs.
type ItemView struct {
	ID                shared.ItemID
	Name              string
	Description       string
	CategoryID        shared.CategoryID
	SourcingMode      catalog.SourcingMode
	Price             shared.Money
	ProductionDays    int
	ReadyDays         int
	MainSupplier      *SupplierItemView
	BackupSupplier    *SupplierItemView
	AttributeValueIDs []shared.AttributeValueID
	Options           []OptionView
	Photos            []PhotoView
	Published         bool
	Archived          bool
	Version           int
	Costs             *ItemCosts
}

// ItemSummaryView is a row of the Itens table (the ItemSummary schema).
// MainSupplierName is empty for in_house items (null in the contract).
type ItemSummaryView struct {
	ID               shared.ItemID
	Name             string
	CategoryID       shared.CategoryID
	SourcingMode     catalog.SourcingMode
	Price            shared.Money
	ReadyDays        int
	MainSupplierName string
	Published        bool
	Archived         bool
	Cover            *PhotoView
	Version          int
	Costs            *Costs
}

// KitItemView is one item of a kit, with what the kit editor shows of it.
// Archived is true when the item was archived after the kit was saved.
type KitItemView struct {
	ItemID             shared.ItemID
	ItemName           string
	Quantity           int
	Swappable          bool
	AlternativeItemIDs []shared.ItemID
	UnitPrice          shared.Money
	ReadyDays          int
	Archived           bool
}

// KitView is one kit (the Kit schema): its totals and "Prazo calculado" come
// from its items and the store's defaults. AssemblyDays and
// SafetyMarginDays are nil when the kit uses the store's default. Costs is
// nil unless the caller sees costs.
type KitView struct {
	ID                shared.KitID
	Name              string
	Description       string
	Price             shared.Money
	SingleItemsTotal  shared.Money
	Discount          shared.Money
	SwapDiscount      catalog.SwapMode
	AssemblyDays      *int
	SafetyMarginDays  *int
	Days              catalog.KitDays
	Items             []KitItemView
	AttributeValueIDs []shared.AttributeValueID
	Photos            []PhotoView
	Published         bool
	Archived          bool
	Version           int
	Costs             *Costs
}

// KitSummaryView is a row of the Kits table (the KitSummary schema).
type KitSummaryView struct {
	ID               shared.KitID
	Name             string
	ItemCount        int
	Price            shared.Money
	SingleItemsTotal shared.Money
	TotalDays        int
	Published        bool
	Archived         bool
	Cover            *PhotoView
	Version          int
	Costs            *Costs
}

func categoryView(c *catalog.Category) CategoryView {
	return CategoryView{ID: c.ID(), Name: c.Name(), Position: c.Position()}
}

func attributeView(p PlacedAttribute, usage map[shared.AttributeValueID]int) AttributeView {
	values := p.Attribute.Values()
	out := AttributeView{ID: p.Attribute.ID(), Name: p.Attribute.Name(), Position: p.Position, Values: make([]AttributeValueView, 0, len(values))}
	for _, v := range values {
		out.Values = append(out.Values, AttributeValueView{ID: v.ID(), Label: v.Label(), UsageCount: usage[v.ID()]})
	}
	return out
}

func optionViews(options []catalog.CustomizationOption) []OptionView {
	out := make([]OptionView, 0, len(options))
	for _, o := range options {
		choices := o.Choices()
		v := OptionView{ID: o.ID(), Name: o.Name(), Required: o.Required(), Choices: make([]ChoiceView, 0, len(choices))}
		for _, c := range choices {
			v.Choices = append(v.Choices, ChoiceView{ID: c.ID(), Label: c.Label(), ExtraPrice: c.ExtraPrice(), ExtraDays: c.ExtraDays()})
		}
		out = append(out, v)
	}
	return out
}

// photoViews signs a read URL for each photo, in order.
func (s *Service) photoViews(ctx context.Context, photos []catalog.Photo) ([]PhotoView, error) {
	out := make([]PhotoView, 0, len(photos))
	for i, p := range photos {
		url, expires, err := s.photos.SignGet(ctx, p.Key(), photoURLTTL)
		if err != nil {
			return nil, fmt.Errorf("sign photo %s: %w", p.ID(), err)
		}
		out = append(out, PhotoView{ID: p.ID(), URL: url, ExpiresAt: expires, Position: i})
	}
	return out, nil
}

// cover signs the first photo only, for a list row; nil without photos.
func (s *Service) cover(ctx context.Context, photos []catalog.Photo) (*PhotoView, error) {
	if len(photos) == 0 {
		return nil, nil
	}
	views, err := s.photoViews(ctx, photos[:1])
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

// supplierView returns the view of one supplier slot, nil when it is empty.
func supplierView(si supplier.SupplierItem, ok bool, names map[shared.SupplierID]string) *SupplierItemView {
	if !ok {
		return nil
	}
	return &SupplierItemView{SupplierID: si.SupplierID(), SupplierName: names[si.SupplierID()], LeadDays: si.LeadDays()}
}

// itemView builds the item's view at version for a caller who sees costs
// or not. It is the only builder of ItemView, so the cost rule lives here.
func itemView(it *catalog.Item, version int, names map[shared.SupplierID]string, photos []PhotoView, seesCosts bool) ItemView {
	main, hasMain := it.MainSupplier()
	backup, hasBackup := it.BackupSupplier()
	v := ItemView{
		ID: it.ID(), Name: it.Name(), Description: it.Description(), CategoryID: it.CategoryID(),
		SourcingMode: it.SourcingMode(), Price: it.Price(), ProductionDays: it.ProductionDays(), ReadyDays: it.ReadyDays(),
		MainSupplier: supplierView(main, hasMain, names), BackupSupplier: supplierView(backup, hasBackup, names),
		AttributeValueIDs: it.AttributeValueIDs(), Options: optionViews(it.Options()), Photos: photos,
		Published: it.Published(), Archived: it.Archived(), Version: version,
	}
	if seesCosts {
		v.Costs = itemCosts(it)
	}
	return v
}

// itemCosts is the owner's part of an item. Call it only for a caller who
// sees costs (identity.SeesCosts).
func itemCosts(it *catalog.Item) *ItemCosts {
	c := &ItemCosts{Cost: it.Cost(), ProductionCost: it.ProductionCost()}
	if m, ok := it.MainSupplier(); ok {
		c.MainSupplierCost = m.Cost()
	}
	if b, ok := it.BackupSupplier(); ok {
		c.BackupSupplierCost = b.Cost()
	}
	if m, ok := it.Margin(); ok {
		c.Margin = &m
	}
	return c
}

func itemSummary(it *catalog.Item, names map[shared.SupplierID]string, cover *PhotoView, seesCosts bool) ItemSummaryView {
	v := ItemSummaryView{
		ID: it.ID(), Name: it.Name(), CategoryID: it.CategoryID(), SourcingMode: it.SourcingMode(), Price: it.Price(),
		ReadyDays: it.ReadyDays(), Published: it.Published(), Archived: it.Archived(), Cover: cover, Version: it.Version(),
	}
	if m, ok := it.MainSupplier(); ok {
		v.MainSupplierName = names[m.SupplierID()]
	}
	if seesCosts {
		v.Costs = &Costs{Cost: it.Cost()}
		if m, ok := it.Margin(); ok {
			v.Costs.Margin = &m
		}
	}
	return v
}

// lookupOf returns the domain's lookup of the facts.
func lookupOf(facts map[shared.ItemID]ItemFacts) catalog.ItemLookup {
	out := make(catalog.ItemLookup, len(facts))
	for id, f := range facts {
		out[id] = f.Facts
	}
	return out
}

// kitTotals are the figures every kit view shows, computed from its items.
type kitTotals struct {
	single   shared.Money
	discount shared.Money
	days     catalog.KitDays
	costs    *Costs
}

// totalsOf computes the kit's totals, its days with the store's defaults
// and, for a caller who sees costs only, its cost and margin.
func totalsOf(k *catalog.Kit, lookup catalog.ItemLookup, d Defaults, seesCosts bool) (kitTotals, error) {
	single, err := k.SingleItemsTotal(lookup)
	if err != nil {
		return kitTotals{}, fmt.Errorf("kit %s total: %w", k.ID(), err)
	}
	discount, err := k.Discount(lookup)
	if err != nil {
		return kitTotals{}, fmt.Errorf("kit %s discount: %w", k.ID(), err)
	}
	days, err := k.Days(lookup, d.AssemblyDays, d.SafetyMarginDays)
	if err != nil {
		return kitTotals{}, fmt.Errorf("kit %s days: %w", k.ID(), err)
	}
	t := kitTotals{single: single, discount: discount, days: days}
	if !seesCosts {
		return t, nil
	}
	cost, err := k.Cost(lookup)
	if err != nil {
		return kitTotals{}, fmt.Errorf("kit %s cost: %w", k.ID(), err)
	}
	t.costs = &Costs{Cost: cost}
	m, ok, err := k.Margin(lookup)
	if err != nil {
		return kitTotals{}, fmt.Errorf("kit %s margin: %w", k.ID(), err)
	}
	if ok {
		t.costs.Margin = &m
	}
	return t, nil
}

func kitItemViews(k *catalog.Kit, facts map[shared.ItemID]ItemFacts) []KitItemView {
	items := k.Items()
	out := make([]KitItemView, 0, len(items))
	for _, ki := range items {
		f := facts[ki.Item]
		alts := ki.Alternatives
		if alts == nil {
			alts = []shared.ItemID{}
		}
		out = append(out, KitItemView{
			ItemID: ki.Item, ItemName: f.Name, Quantity: ki.Quantity, Swappable: ki.Swappable,
			AlternativeItemIDs: alts, UnitPrice: f.Facts.Price, ReadyDays: f.Facts.ReadyDays, Archived: f.Facts.Archived,
		})
	}
	return out
}

// kitView builds the kit's view at version. It is the only builder of
// KitView, so the cost rule lives here (and in totalsOf).
func kitView(k *catalog.Kit, version int, facts map[shared.ItemID]ItemFacts, d Defaults, photos []PhotoView, seesCosts bool) (KitView, error) {
	t, err := totalsOf(k, lookupOf(facts), d, seesCosts)
	if err != nil {
		return KitView{}, err
	}
	v := KitView{
		ID: k.ID(), Name: k.Name(), Description: k.Description(), Price: k.Price(),
		SingleItemsTotal: t.single, Discount: t.discount, SwapDiscount: k.SwapMode(), Days: t.days,
		Items: kitItemViews(k, facts), AttributeValueIDs: k.AttributeValueIDs(), Photos: photos,
		Published: k.Published(), Archived: k.Archived(), Version: version, Costs: t.costs,
	}
	if days, ok := k.AssemblyDays(); ok {
		v.AssemblyDays = &days
	}
	if days, ok := k.SafetyMarginDays(); ok {
		v.SafetyMarginDays = &days
	}
	return v, nil
}

func kitSummary(k *catalog.Kit, facts map[shared.ItemID]ItemFacts, d Defaults, cover *PhotoView, seesCosts bool) (KitSummaryView, error) {
	t, err := totalsOf(k, lookupOf(facts), d, seesCosts)
	if err != nil {
		return KitSummaryView{}, err
	}
	return KitSummaryView{
		ID: k.ID(), Name: k.Name(), ItemCount: len(k.Items()), Price: k.Price(), SingleItemsTotal: t.single,
		TotalDays: t.days.Total, Published: k.Published(), Archived: k.Archived(), Cover: cover,
		Version: k.Version(), Costs: t.costs,
	}, nil
}
