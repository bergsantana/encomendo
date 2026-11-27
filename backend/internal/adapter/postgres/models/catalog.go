package models

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// Category maps categories.
type Category struct {
	TenantID string `gorm:"column:tenant_id"`
	ID       string `gorm:"column:id;primaryKey"`
	Name     string `gorm:"column:name"`
	Position int    `gorm:"column:position"`
	Version  int    `gorm:"column:version"`
}

// TableName returns categories.
func (Category) TableName() string { return "categories" }

// CategoryFromDomain returns the row of c.
func CategoryFromDomain(c *catalog.Category) Category {
	return Category{
		TenantID: c.TenantID().String(), ID: c.ID().String(), Name: c.Name(),
		Position: c.Position(), Version: c.Version(),
	}
}

// ToDomain rebuilds the category.
func (m Category) ToDomain() (*catalog.Category, error) {
	tenant, err := parseID[shared.TenantID](m.TenantID)
	if err != nil {
		return nil, err
	}
	id, err := parseID[shared.CategoryID](m.ID)
	if err != nil {
		return nil, err
	}
	return catalog.RestoreCategory(catalog.CategoryRecord{Tenant: tenant, ID: id, Name: m.Name, Position: m.Position, Version: m.Version})
}

// Attribute maps attributes (the item fields, "Serve em").
type Attribute struct {
	TenantID string `gorm:"column:tenant_id"`
	ID       string `gorm:"column:id;primaryKey"`
	Name     string `gorm:"column:name"`
	Position int    `gorm:"column:position"`
	Version  int    `gorm:"column:version"`
}

// TableName returns attributes.
func (Attribute) TableName() string { return "attributes" }

// AttributeValue maps attribute_values.
type AttributeValue struct {
	TenantID    string `gorm:"column:tenant_id"`
	ID          string `gorm:"column:id;primaryKey"`
	AttributeID string `gorm:"column:attribute_id"`
	Label       string `gorm:"column:label"`
	Position    int    `gorm:"column:position"`
}

// TableName returns attribute_values.
func (AttributeValue) TableName() string { return "attribute_values" }

// AttributeRows is one field with its values, as the repository writes and
// reads them.
type AttributeRows struct {
	Attribute Attribute
	Values    []AttributeValue
}

// AttributeFromDomain returns the rows of a at position.
func AttributeFromDomain(a *catalog.Attribute, position int) AttributeRows {
	rows := AttributeRows{Attribute: Attribute{
		TenantID: a.TenantID().String(), ID: a.ID().String(), Name: a.Name(), Position: position, Version: a.Version(),
	}}
	for i, v := range a.Values() {
		rows.Values = append(rows.Values, AttributeValue{
			TenantID: rows.Attribute.TenantID, ID: v.ID().String(), AttributeID: rows.Attribute.ID, Label: v.Label(), Position: i,
		})
	}
	return rows
}

// ToDomain rebuilds the field; the values must already be in order.
func (r AttributeRows) ToDomain() (*catalog.Attribute, error) {
	tenant, err := parseID[shared.TenantID](r.Attribute.TenantID)
	if err != nil {
		return nil, err
	}
	id, err := parseID[shared.AttributeID](r.Attribute.ID)
	if err != nil {
		return nil, err
	}
	values := make([]catalog.ValueInput, 0, len(r.Values))
	for _, v := range r.Values {
		vid, err := parseID[shared.AttributeValueID](v.ID)
		if err != nil {
			return nil, err
		}
		values = append(values, catalog.ValueInput{ID: vid, Label: v.Label})
	}
	return catalog.RestoreAttribute(catalog.AttributeRecord{
		Tenant: tenant, ID: id, Name: r.Attribute.Name, Values: values, Version: r.Attribute.Version,
	})
}

// Item maps items.
type Item struct {
	TenantID       string     `gorm:"column:tenant_id"`
	ID             string     `gorm:"column:id;primaryKey"`
	Name           string     `gorm:"column:name"`
	Description    string     `gorm:"column:description"`
	CategoryID     *string    `gorm:"column:category_id"`
	SourcingMode   string     `gorm:"column:sourcing_mode"`
	PriceCents     int64      `gorm:"column:price_cents"`
	Currency       string     `gorm:"column:currency"`
	ProductionDays *int       `gorm:"column:production_days"`
	ReadyDays      int        `gorm:"column:ready_days"`
	Published      bool       `gorm:"column:published"`
	ArchivedAt     *time.Time `gorm:"column:archived_at"`
	Version        int        `gorm:"column:version"`
}

// TableName returns items.
func (Item) TableName() string { return "items" }

// ItemCost maps item_costs, the own cost of an in-house item.
type ItemCost struct {
	TenantID  string `gorm:"column:tenant_id"`
	ItemID    string `gorm:"column:item_id"`
	CostCents int64  `gorm:"column:cost_cents"`
	Currency  string `gorm:"column:currency"`
}

// TableName returns item_costs.
func (ItemCost) TableName() string { return "item_costs" }

// ItemAttributeValue maps item_attribute_values.
type ItemAttributeValue struct {
	TenantID string `gorm:"column:tenant_id"`
	ItemID   string `gorm:"column:item_id"`
	ValueID  string `gorm:"column:value_id"`
}

// TableName returns item_attribute_values.
func (ItemAttributeValue) TableName() string { return "item_attribute_values" }

// Option maps customization_options. Choices holds the jsonb array of
// ADR 0024: [{"id", "label", "extraPriceCents", "extraDays"}].
type Option struct {
	TenantID string `gorm:"column:tenant_id"`
	ID       string `gorm:"column:id;primaryKey"`
	ItemID   string `gorm:"column:item_id"`
	Name     string `gorm:"column:name"`
	Required bool   `gorm:"column:required"`
	Position int    `gorm:"column:position"`
	Choices  []byte `gorm:"column:choices;type:jsonb"`
}

// TableName returns customization_options.
func (Option) TableName() string { return "customization_options" }

// choiceJSON is the stored shape of one choice (ADR 0024).
type choiceJSON struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	ExtraPriceCents int64  `json:"extraPriceCents"`
	ExtraDays       int    `json:"extraDays"`
}

// OptionFromDomain returns the row of o, the position-th option of item.
func OptionFromDomain(tenant shared.TenantID, item shared.ItemID, o catalog.CustomizationOption, position int) (Option, error) {
	choices := make([]choiceJSON, 0, len(o.Choices()))
	for _, c := range o.Choices() {
		choices = append(choices, choiceJSON{
			ID: c.ID().String(), Label: c.Label(), ExtraPriceCents: c.ExtraPrice().Cents(), ExtraDays: c.ExtraDays(),
		})
	}
	raw, err := json.Marshal(choices)
	if err != nil {
		return Option{}, fmt.Errorf("encode choices of option %v: %w", o.ID(), err)
	}
	return Option{
		TenantID: tenant.String(), ID: o.ID().String(), ItemID: item.String(), Name: o.Name(),
		Required: o.Required(), Position: position, Choices: raw,
	}, nil
}

// ToInput rebuilds the option as the domain's input; currency is the item's.
func (m Option) ToInput(currency string) (catalog.OptionInput, error) {
	id, err := parseID[shared.OptionID](m.ID)
	if err != nil {
		return catalog.OptionInput{}, err
	}
	var stored []choiceJSON
	if err := json.Unmarshal(m.Choices, &stored); err != nil {
		return catalog.OptionInput{}, fmt.Errorf("decode choices of option %v: %w", id, err)
	}
	in := catalog.OptionInput{ID: id, Name: m.Name, Required: m.Required}
	for _, c := range stored {
		cid, err := parseID[shared.ChoiceID](c.ID)
		if err != nil {
			return catalog.OptionInput{}, err
		}
		price, err := shared.NewMoney(c.ExtraPriceCents, currency)
		if err != nil {
			return catalog.OptionInput{}, err
		}
		in.Choices = append(in.Choices, catalog.ChoiceInput{ID: cid, Label: c.Label, ExtraPrice: price, ExtraDays: c.ExtraDays})
	}
	return in, nil
}

// ItemRows is an item with every row hanging on it. Values are in the
// store's field order, Options by position and Photos by position. Links
// and Cost come from staff-only tables and are empty in a scope that cannot
// read them.
type ItemRows struct {
	Item    Item
	Links   []SupplierItem
	Cost    *ItemCost
	Values  []string
	Options []Option
	Photos  []PhotoRead
}

// ItemFromDomain returns the rows of it that Insert and Update write: the
// item, its supplier links, its in-house cost, its values and its options.
// Photos are their own resource and never written here. archivedAt stamps a
// new row of an archived item.
func ItemFromDomain(it *catalog.Item, archivedAt time.Time) (ItemRows, error) {
	tenant, id := it.TenantID(), it.ID()
	m := Item{
		TenantID: tenant.String(), ID: id.String(), Name: it.Name(), Description: it.Description(),
		SourcingMode: string(it.SourcingMode()), PriceCents: it.Price().Cents(), Currency: it.Price().Currency(),
		ReadyDays: it.ReadyDays(), Published: it.Published(), Version: it.Version(),
	}
	if !it.CategoryID().IsZero() {
		c := it.CategoryID().String()
		m.CategoryID = &c
	}
	if it.SourcingMode() == catalog.ModeInHouse {
		days := it.ProductionDays()
		m.ProductionDays = &days
	}
	if it.Archived() {
		m.ArchivedAt = &archivedAt
	}
	rows := ItemRows{Item: m}
	for _, link := range []func() (supplier.SupplierItem, bool){it.MainSupplier, it.BackupSupplier} {
		if si, ok := link(); ok {
			rows.Links = append(rows.Links, SupplierItemFromDomain(tenant, id, si, m.Currency))
		}
	}
	if c := it.ProductionCost(); c != nil {
		rows.Cost = &ItemCost{TenantID: m.TenantID, ItemID: m.ID, CostCents: c.Cents(), Currency: c.Currency()}
	}
	for _, v := range it.AttributeValueIDs() {
		rows.Values = append(rows.Values, v.String())
	}
	for i, o := range it.Options() {
		opt, err := OptionFromDomain(tenant, id, o, i)
		if err != nil {
			return ItemRows{}, err
		}
		rows.Options = append(rows.Options, opt)
	}
	return rows, nil
}

// ToDomain rebuilds the item with its sourcing, options and photos.
func (r ItemRows) ToDomain() (*catalog.Item, error) {
	m := r.Item
	rec, err := m.record()
	if err != nil {
		return nil, err
	}
	if m.ProductionDays != nil {
		rec.Sourcing.ProductionDays = *m.ProductionDays
	}
	if r.Cost != nil {
		if rec.Sourcing.ProductionCost, err = optionalMoney(&r.Cost.CostCents, r.Cost.Currency); err != nil {
			return nil, err
		}
	}
	for _, l := range r.Links {
		si, err := l.ToDomain()
		if err != nil {
			return nil, err
		}
		if si.Role() == supplier.RoleMain {
			rec.Sourcing.Main = &si
		} else {
			rec.Sourcing.Backup = &si
		}
	}
	for _, v := range r.Values {
		vid, err := parseID[shared.AttributeValueID](v)
		if err != nil {
			return nil, err
		}
		rec.Attributes = append(rec.Attributes, vid)
	}
	for _, o := range r.Options {
		in, err := o.ToInput(m.Currency)
		if err != nil {
			return nil, err
		}
		rec.Options = append(rec.Options, in)
	}
	if rec.Photos, err = photosOf(r.Photos); err != nil {
		return nil, err
	}
	return catalog.RestoreItem(rec)
}

// record returns the part of the item's record that the items row holds.
func (m Item) record() (catalog.ItemRecord, error) {
	tenant, err := parseID[shared.TenantID](m.TenantID)
	if err != nil {
		return catalog.ItemRecord{}, err
	}
	id, err := parseID[shared.ItemID](m.ID)
	if err != nil {
		return catalog.ItemRecord{}, err
	}
	price, err := shared.NewMoney(m.PriceCents, m.Currency)
	if err != nil {
		return catalog.ItemRecord{}, err
	}
	rec := catalog.ItemRecord{
		Tenant: tenant, ID: id, Name: m.Name, Description: m.Description, Price: price,
		Sourcing:  catalog.SourcingRecord{Mode: catalog.SourcingMode(m.SourcingMode)},
		Published: m.Published, Archived: m.ArchivedAt != nil, Version: m.Version,
	}
	if m.CategoryID != nil {
		if rec.Category, err = parseID[shared.CategoryID](*m.CategoryID); err != nil {
			return catalog.ItemRecord{}, err
		}
	}
	return rec, nil
}

// PublishedItemRows is a published item with the public rows hanging on it,
// as the store page reads them: no supplier, no cost.
type PublishedItemRows struct {
	Item    Item
	Values  []string
	Options []Option
}

// ToDomain rebuilds the options and ids the store page shows.
func (r PublishedItemRows) ToDomain() (PublishedItemParts, error) {
	id, err := parseID[shared.ItemID](r.Item.ID)
	if err != nil {
		return PublishedItemParts{}, err
	}
	price, err := shared.NewMoney(r.Item.PriceCents, r.Item.Currency)
	if err != nil {
		return PublishedItemParts{}, err
	}
	parts := PublishedItemParts{ID: id, Price: price}
	if r.Item.CategoryID != nil {
		if parts.Category, err = parseID[shared.CategoryID](*r.Item.CategoryID); err != nil {
			return PublishedItemParts{}, err
		}
	}
	for _, v := range r.Values {
		vid, err := parseID[shared.AttributeValueID](v)
		if err != nil {
			return PublishedItemParts{}, err
		}
		parts.Values = append(parts.Values, vid)
	}
	for _, o := range r.Options {
		in, err := o.ToInput(r.Item.Currency)
		if err != nil {
			return PublishedItemParts{}, err
		}
		opt, err := catalog.NewOption(in)
		if err != nil {
			return PublishedItemParts{}, fmt.Errorf("stored option %v: %w: %s", in.ID, catalog.ErrInvalidOption, err.Error())
		}
		parts.Options = append(parts.Options, opt)
	}
	return parts, nil
}

// PublishedItemParts is what PublishedItemRows rebuilds.
type PublishedItemParts struct {
	ID       shared.ItemID
	Category shared.CategoryID
	Price    shared.Money
	Values   []shared.AttributeValueID
	Options  []catalog.CustomizationOption
}

// Kit maps kits.
type Kit struct {
	TenantID         string     `gorm:"column:tenant_id"`
	ID               string     `gorm:"column:id;primaryKey"`
	Name             string     `gorm:"column:name"`
	Description      string     `gorm:"column:description"`
	PriceCents       int64      `gorm:"column:price_cents"`
	Currency         string     `gorm:"column:currency"`
	SwapDiscount     string     `gorm:"column:swap_discount"`
	AssemblyDays     *int       `gorm:"column:assembly_days"`
	SafetyMarginDays *int       `gorm:"column:safety_margin_days"`
	Published        bool       `gorm:"column:published"`
	ArchivedAt       *time.Time `gorm:"column:archived_at"`
	Version          int        `gorm:"column:version"`
}

// TableName returns kits.
func (Kit) TableName() string { return "kits" }

// KitItem maps kit_items.
type KitItem struct {
	TenantID  string `gorm:"column:tenant_id"`
	KitID     string `gorm:"column:kit_id"`
	ItemID    string `gorm:"column:item_id"`
	Quantity  int    `gorm:"column:quantity"`
	Swappable bool   `gorm:"column:swappable"`
	Position  int    `gorm:"column:position"`
}

// TableName returns kit_items.
func (KitItem) TableName() string { return "kit_items" }

// KitAlternative maps kit_item_alternatives.
type KitAlternative struct {
	TenantID          string `gorm:"column:tenant_id"`
	KitID             string `gorm:"column:kit_id"`
	ItemID            string `gorm:"column:item_id"`
	AlternativeItemID string `gorm:"column:alternative_item_id"`
}

// TableName returns kit_item_alternatives.
func (KitAlternative) TableName() string { return "kit_item_alternatives" }

// KitAttributeValue maps kit_attribute_values.
type KitAttributeValue struct {
	TenantID string `gorm:"column:tenant_id"`
	KitID    string `gorm:"column:kit_id"`
	ValueID  string `gorm:"column:value_id"`
}

// TableName returns kit_attribute_values.
func (KitAttributeValue) TableName() string { return "kit_attribute_values" }

// KitRows is a kit with every row hanging on it. Items are by position,
// Alternatives by item then alternative id, Values in the store's field
// order and Photos by position.
type KitRows struct {
	Kit          Kit
	Items        []KitItem
	Alternatives []KitAlternative
	Values       []string
	Photos       []PhotoRead
}

// KitFromDomain returns the rows of k that Insert and Update write. Photos
// are never written here. archivedAt stamps a new row of an archived kit.
func KitFromDomain(k *catalog.Kit, archivedAt time.Time) KitRows {
	tenant := k.TenantID().String()
	m := Kit{
		TenantID: tenant, ID: k.ID().String(), Name: k.Name(), Description: k.Description(),
		PriceCents: k.Price().Cents(), Currency: k.Price().Currency(), SwapDiscount: string(k.SwapMode()),
		Published: k.Published(), Version: k.Version(),
	}
	if d, ok := k.AssemblyDays(); ok {
		m.AssemblyDays = &d
	}
	if d, ok := k.SafetyMarginDays(); ok {
		m.SafetyMarginDays = &d
	}
	if k.Archived() {
		m.ArchivedAt = &archivedAt
	}
	rows := KitRows{Kit: m}
	for i, ki := range k.Items() {
		rows.Items = append(rows.Items, KitItem{
			TenantID: tenant, KitID: m.ID, ItemID: ki.Item.String(), Quantity: ki.Quantity, Swappable: ki.Swappable, Position: i,
		})
		for _, alt := range ki.Alternatives {
			rows.Alternatives = append(rows.Alternatives, KitAlternative{
				TenantID: tenant, KitID: m.ID, ItemID: ki.Item.String(), AlternativeItemID: alt.String(),
			})
		}
	}
	for _, v := range k.AttributeValueIDs() {
		rows.Values = append(rows.Values, v.String())
	}
	return rows
}

// ToDomain rebuilds the kit.
func (r KitRows) ToDomain() (*catalog.Kit, error) {
	m := r.Kit
	tenant, err := parseID[shared.TenantID](m.TenantID)
	if err != nil {
		return nil, err
	}
	id, err := parseID[shared.KitID](m.ID)
	if err != nil {
		return nil, err
	}
	price, err := shared.NewMoney(m.PriceCents, m.Currency)
	if err != nil {
		return nil, err
	}
	rec := catalog.KitRecord{
		Tenant: tenant, ID: id, Name: m.Name, Description: m.Description, Price: price,
		SwapMode: catalog.SwapMode(m.SwapDiscount), AssemblyDays: m.AssemblyDays, SafetyMarginDays: m.SafetyMarginDays,
		Published: m.Published, Archived: m.ArchivedAt != nil, Version: m.Version,
	}
	// RestoreKit checks every item it names is in the lookup, which the
	// foreign keys of kit_items guarantee, so the lookup holds ids only.
	lookup := catalog.ItemLookup{}
	if rec.Items, err = r.kitItems(lookup); err != nil {
		return nil, err
	}
	for _, v := range r.Values {
		vid, err := parseID[shared.AttributeValueID](v)
		if err != nil {
			return nil, err
		}
		rec.Attributes = append(rec.Attributes, vid)
	}
	if rec.Photos, err = photosOf(r.Photos); err != nil {
		return nil, err
	}
	return catalog.RestoreKit(rec, lookup)
}

func (r KitRows) kitItems(lookup catalog.ItemLookup) ([]catalog.KitItem, error) {
	alts := map[string][]shared.ItemID{}
	for _, a := range r.Alternatives {
		alt, err := parseID[shared.ItemID](a.AlternativeItemID)
		if err != nil {
			return nil, err
		}
		alts[a.ItemID] = append(alts[a.ItemID], alt)
		lookup[alt] = catalog.ItemFacts{}
	}
	items := make([]catalog.KitItem, 0, len(r.Items))
	for _, ki := range r.Items {
		item, err := parseID[shared.ItemID](ki.ItemID)
		if err != nil {
			return nil, err
		}
		lookup[item] = catalog.ItemFacts{}
		items = append(items, catalog.KitItem{Item: item, Quantity: ki.Quantity, Swappable: ki.Swappable, Alternatives: alts[ki.ItemID]})
	}
	return items, nil
}

// PhotoRead maps photos for reading only: Items.ByID and Kits.ByID load the
// photos so views can sign them. The photo repository (step 3.9) writes
// them.
type PhotoRead struct {
	ID          string  `gorm:"column:id"`
	ItemID      *string `gorm:"column:item_id"`
	KitID       *string `gorm:"column:kit_id"`
	ObjectKey   string  `gorm:"column:object_key"`
	ContentType string  `gorm:"column:content_type"`
	SizeBytes   int64   `gorm:"column:size_bytes"`
}

// TableName returns photos.
func (PhotoRead) TableName() string { return "photos" }

func photosOf(rows []PhotoRead) ([]catalog.Photo, error) {
	out := make([]catalog.Photo, 0, len(rows))
	for _, p := range rows {
		id, err := parseID[shared.PhotoID](p.ID)
		if err != nil {
			return nil, err
		}
		photo, err := catalog.NewPhoto(id, p.ObjectKey, p.ContentType, p.SizeBytes)
		if err != nil {
			return nil, err
		}
		out = append(out, photo)
	}
	return out, nil
}
