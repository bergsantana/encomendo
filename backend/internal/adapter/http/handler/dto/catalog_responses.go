package dto

import (
	"time"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The response types mirror the contract's schemas. Costs and margins are
// the owner's (ADR 0025): the use cases leave the Costs part of a view nil
// for every other role, and these mappers then leave the keys out of the
// JSON altogether (Owned), so a role that cannot see them gets no `cost`,
// `productionCost` or `margin` key at any depth. For the owner the keys are
// there, null when nothing is recorded.

// Margin is the Margin schema.
type Margin struct {
	Cents       int64 `json:"cents"`
	BasisPoints int64 `json:"basisPoints"`
}

func ownedMoney(m *shared.Money) *Owned[shared.Money] { return Own(m) }

func ownedMargin(m *catalog.Margin) *Owned[Margin] {
	if m == nil {
		return Own[Margin](nil)
	}
	return Own(&Margin{Cents: m.Cents, BasisPoints: m.BasisPoints})
}

// Category is the Category schema.
type Category struct {
	ID       shared.CategoryID `json:"id"`
	Name     string            `json:"name"`
	Position int               `json:"position"`
}

// NewCategory maps a category.
func NewCategory(c appcatalog.CategoryView) Category {
	return Category{ID: c.ID, Name: c.Name, Position: c.Position}
}

// List is a list with no paging: {"items": [...]}.
type List[T any] struct {
	Items []T `json:"items"`
}

// NewList maps every element with each; the list is never null.
func NewList[V, T any](in []V, each func(V) T) List[T] {
	out := List[T]{Items: make([]T, 0, len(in))}
	for _, v := range in {
		out.Items = append(out.Items, each(v))
	}
	return out
}

// AttributeValue is the AttributeValue schema.
type AttributeValue struct {
	ID         shared.AttributeValueID `json:"id"`
	Label      string                  `json:"label"`
	UsageCount int                     `json:"usageCount"`
}

// Attribute is the Attribute schema.
type Attribute struct {
	ID       shared.AttributeID `json:"id"`
	Name     string             `json:"name"`
	Position int                `json:"position"`
	Values   []AttributeValue   `json:"values"`
}

// NewAttribute maps an item field.
func NewAttribute(a appcatalog.AttributeView) Attribute {
	out := Attribute{ID: a.ID, Name: a.Name, Position: a.Position, Values: make([]AttributeValue, 0, len(a.Values))}
	for _, v := range a.Values {
		out.Values = append(out.Values, AttributeValue{ID: v.ID, Label: v.Label, UsageCount: v.UsageCount})
	}
	return out
}

// Photo is the Photo schema.
type Photo struct {
	ID        shared.PhotoID `json:"id"`
	URL       string         `json:"url"`
	ExpiresAt time.Time      `json:"expiresAt"`
	Position  int            `json:"position"`
}

// NewPhoto maps a photo with its signed read URL.
func NewPhoto(p appcatalog.PhotoView) Photo {
	return Photo{ID: p.ID, URL: p.URL, ExpiresAt: p.ExpiresAt.UTC(), Position: p.Position}
}

func newCover(p *appcatalog.PhotoView) *Photo {
	if p == nil {
		return nil
	}
	c := NewPhoto(*p)
	return &c
}

func newPhotos(in []appcatalog.PhotoView) []Photo {
	return NewList(in, NewPhoto).Items
}

// NewPhotoList maps the photos in their order: {"items": [...]}.
func NewPhotoList(in []appcatalog.PhotoView) List[Photo] { return NewList(in, NewPhoto) }

// PhotoUpload is the PhotoUpload schema.
type PhotoUpload struct {
	Key       string            `json:"key"`
	UploadURL string            `json:"uploadUrl"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

// NewPhotoUpload maps a signed upload. Headers is never null.
func NewPhotoUpload(u appcatalog.PhotoUploadView) PhotoUpload {
	headers := u.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	return PhotoUpload{Key: u.Key, UploadURL: u.URL, Method: u.Method, Headers: headers, ExpiresAt: u.ExpiresAt.UTC()}
}

// SupplierItem is the SupplierItem schema; Cost is the owner's.
type SupplierItem struct {
	SupplierID   shared.SupplierID    `json:"supplierId"`
	SupplierName string               `json:"supplierName"`
	LeadDays     int                  `json:"leadDays"`
	Cost         *Owned[shared.Money] `json:"cost,omitempty"`
}

// Choice is one choice of a customization option.
type Choice struct {
	ID         shared.ChoiceID `json:"id"`
	Label      string          `json:"label"`
	ExtraPrice shared.Money    `json:"extraPrice"`
	ExtraDays  int             `json:"extraDays"`
}

// Option is the CustomizationOption schema.
type Option struct {
	ID       shared.OptionID `json:"id"`
	Name     string          `json:"name"`
	Required bool            `json:"required"`
	Choices  []Choice        `json:"choices"`
}

func newOptions(in []appcatalog.OptionView) []Option {
	out := make([]Option, 0, len(in))
	for _, o := range in {
		opt := Option{ID: o.ID, Name: o.Name, Required: o.Required, Choices: make([]Choice, 0, len(o.Choices))}
		for _, c := range o.Choices {
			opt.Choices = append(opt.Choices, Choice{ID: c.ID, Label: c.Label, ExtraPrice: c.ExtraPrice, ExtraDays: c.ExtraDays})
		}
		out = append(out, opt)
	}
	return out
}

// categoryOf is the contract's nullable category id: null for "Sem
// categoria".
func categoryOf(id shared.CategoryID) *shared.CategoryID {
	if id.IsZero() {
		return nil
	}
	return &id
}

// Item is the Item schema. ProductionCost, Cost and Margin are the owner's;
// ProductionCost is also only for in_house items.
type Item struct {
	ID                shared.ItemID             `json:"id"`
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	CategoryID        *shared.CategoryID        `json:"categoryId"`
	SourcingMode      string                    `json:"sourcingMode"`
	Price             shared.Money              `json:"price"`
	ProductionDays    *int                      `json:"productionDays"`
	ReadyDays         int                       `json:"readyDays"`
	MainSupplier      *SupplierItem             `json:"mainSupplier"`
	BackupSupplier    *SupplierItem             `json:"backupSupplier"`
	AttributeValueIDs []shared.AttributeValueID `json:"attributeValueIds"`
	Options           []Option                  `json:"options"`
	Photos            []Photo                   `json:"photos"`
	Published         bool                      `json:"published"`
	Archived          bool                      `json:"archived"`
	Version           int                       `json:"version"`
	ProductionCost    *Owned[shared.Money]      `json:"productionCost,omitempty"`
	Cost              *Owned[shared.Money]      `json:"cost,omitempty"`
	Margin            *Owned[Margin]            `json:"margin,omitempty"`
}

// NewItem maps an item view.
func NewItem(v appcatalog.ItemView) Item {
	out := Item{
		ID: v.ID, Name: v.Name, Description: v.Description, CategoryID: categoryOf(v.CategoryID),
		SourcingMode: string(v.SourcingMode), Price: v.Price, ReadyDays: v.ReadyDays,
		MainSupplier: newSupplierItem(v.MainSupplier, nil), BackupSupplier: newSupplierItem(v.BackupSupplier, nil),
		AttributeValueIDs: nonNil(v.AttributeValueIDs), Options: newOptions(v.Options), Photos: newPhotos(v.Photos),
		Published: v.Published, Archived: v.Archived, Version: v.Version,
	}
	if v.ProductionDays > 0 {
		days := v.ProductionDays
		out.ProductionDays = &days
	}
	if c := v.Costs; c != nil {
		out.MainSupplier = newSupplierItem(v.MainSupplier, &c.MainSupplierCost)
		out.BackupSupplier = newSupplierItem(v.BackupSupplier, &c.BackupSupplierCost)
		out.Cost, out.Margin = ownedMoney(c.Cost), ownedMargin(c.Margin)
		if v.SourcingMode == catalog.ModeInHouse {
			out.ProductionCost = ownedMoney(c.ProductionCost)
		}
	}
	return out
}

// newSupplierItem maps one supplier slot, nil when it is empty. cost is
// nil for a role that does not see costs, and points to the supplier's own
// cost (itself nil when none is recorded) for the owner.
func newSupplierItem(s *appcatalog.SupplierItemView, cost **shared.Money) *SupplierItem {
	if s == nil {
		return nil
	}
	out := &SupplierItem{SupplierID: s.SupplierID, SupplierName: s.SupplierName, LeadDays: s.LeadDays}
	if cost != nil {
		out.Cost = ownedMoney(*cost)
	}
	return out
}

func nonNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

// ItemSummary is the ItemSummary schema; Cost and Margin are the owner's.
type ItemSummary struct {
	ID               shared.ItemID        `json:"id"`
	Name             string               `json:"name"`
	CategoryID       *shared.CategoryID   `json:"categoryId"`
	SourcingMode     string               `json:"sourcingMode"`
	Price            shared.Money         `json:"price"`
	ReadyDays        int                  `json:"readyDays"`
	MainSupplierName *string              `json:"mainSupplierName"`
	Published        bool                 `json:"published"`
	Archived         bool                 `json:"archived"`
	Cover            *Photo               `json:"cover"`
	Version          int                  `json:"version"`
	Cost             *Owned[shared.Money] `json:"cost,omitempty"`
	Margin           *Owned[Margin]       `json:"margin,omitempty"`
}

// NewItemSummary maps a row of the Itens table.
func NewItemSummary(v appcatalog.ItemSummaryView) ItemSummary {
	out := ItemSummary{
		ID: v.ID, Name: v.Name, CategoryID: categoryOf(v.CategoryID), SourcingMode: string(v.SourcingMode),
		Price: v.Price, ReadyDays: v.ReadyDays, Published: v.Published, Archived: v.Archived,
		Cover: newCover(v.Cover), Version: v.Version,
	}
	if v.MainSupplierName != "" {
		name := v.MainSupplierName
		out.MainSupplierName = &name
	}
	if v.Costs != nil {
		out.Cost, out.Margin = ownedMoney(v.Costs.Cost), ownedMargin(v.Costs.Margin)
	}
	return out
}

// KitDays is the KitDays schema.
type KitDays struct {
	SlowestItem  int `json:"slowestItem"`
	Assembly     int `json:"assembly"`
	SafetyMargin int `json:"safetyMargin"`
	Total        int `json:"total"`
}

// KitItem is the KitItem schema.
type KitItem struct {
	ItemID             shared.ItemID   `json:"itemId"`
	ItemName           string          `json:"itemName"`
	Quantity           int             `json:"quantity"`
	Swappable          bool            `json:"swappable"`
	AlternativeItemIDs []shared.ItemID `json:"alternativeItemIds"`
	UnitPrice          shared.Money    `json:"unitPrice"`
	ReadyDays          int             `json:"readyDays"`
	Archived           bool            `json:"archived"`
}

// Kit is the Kit schema; Cost and Margin are the owner's.
type Kit struct {
	ID                shared.KitID              `json:"id"`
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	Price             shared.Money              `json:"price"`
	SingleItemsTotal  shared.Money              `json:"singleItemsTotal"`
	Discount          shared.Money              `json:"discount"`
	SwapDiscount      string                    `json:"swapDiscount"`
	AssemblyDays      *int                      `json:"assemblyDays"`
	SafetyMarginDays  *int                      `json:"safetyMarginDays"`
	Days              KitDays                   `json:"days"`
	Items             []KitItem                 `json:"items"`
	AttributeValueIDs []shared.AttributeValueID `json:"attributeValueIds"`
	Photos            []Photo                   `json:"photos"`
	Published         bool                      `json:"published"`
	Archived          bool                      `json:"archived"`
	Version           int                       `json:"version"`
	Cost              *Owned[shared.Money]      `json:"cost,omitempty"`
	Margin            *Owned[Margin]            `json:"margin,omitempty"`
}

// NewKit maps a kit view.
func NewKit(v appcatalog.KitView) Kit {
	out := Kit{
		ID: v.ID, Name: v.Name, Description: v.Description, Price: v.Price, SingleItemsTotal: v.SingleItemsTotal,
		Discount: v.Discount, SwapDiscount: string(v.SwapDiscount), AssemblyDays: v.AssemblyDays, SafetyMarginDays: v.SafetyMarginDays,
		Days:  KitDays{SlowestItem: v.Days.SlowestItem, Assembly: v.Days.Assembly, SafetyMargin: v.Days.SafetyMargin, Total: v.Days.Total},
		Items: make([]KitItem, 0, len(v.Items)), AttributeValueIDs: nonNil(v.AttributeValueIDs), Photos: newPhotos(v.Photos),
		Published: v.Published, Archived: v.Archived, Version: v.Version,
	}
	for _, it := range v.Items {
		out.Items = append(out.Items, KitItem{
			ItemID: it.ItemID, ItemName: it.ItemName, Quantity: it.Quantity, Swappable: it.Swappable,
			AlternativeItemIDs: nonNil(it.AlternativeItemIDs), UnitPrice: it.UnitPrice, ReadyDays: it.ReadyDays, Archived: it.Archived,
		})
	}
	if v.Costs != nil {
		out.Cost, out.Margin = ownedMoney(v.Costs.Cost), ownedMargin(v.Costs.Margin)
	}
	return out
}

// KitSummary is the KitSummary schema; Cost and Margin are the owner's.
type KitSummary struct {
	ID               shared.KitID         `json:"id"`
	Name             string               `json:"name"`
	ItemCount        int                  `json:"itemCount"`
	Price            shared.Money         `json:"price"`
	SingleItemsTotal shared.Money         `json:"singleItemsTotal"`
	TotalDays        int                  `json:"totalDays"`
	Published        bool                 `json:"published"`
	Archived         bool                 `json:"archived"`
	Cover            *Photo               `json:"cover"`
	Version          int                  `json:"version"`
	Cost             *Owned[shared.Money] `json:"cost,omitempty"`
	Margin           *Owned[Margin]       `json:"margin,omitempty"`
}

// NewKitSummary maps a row of the Kits table.
func NewKitSummary(v appcatalog.KitSummaryView) KitSummary {
	out := KitSummary{
		ID: v.ID, Name: v.Name, ItemCount: v.ItemCount, Price: v.Price, SingleItemsTotal: v.SingleItemsTotal,
		TotalDays: v.TotalDays, Published: v.Published, Archived: v.Archived, Cover: newCover(v.Cover), Version: v.Version,
	}
	if v.Costs != nil {
		out.Cost, out.Margin = ownedMoney(v.Costs.Cost), ownedMargin(v.Costs.Margin)
	}
	return out
}

// Supplier is the Supplier schema. It carries no cost.
type Supplier struct {
	ID            shared.SupplierID `json:"id"`
	Name          string            `json:"name"`
	WhatsAppPhone string            `json:"whatsappPhone"`
	Version       int               `json:"version"`
}

// NewSupplier maps a supplier view.
func NewSupplier(v appsupplier.View) Supplier {
	return Supplier{ID: v.ID, Name: v.Name, WhatsAppPhone: v.WhatsAppPhone, Version: v.Version}
}

// SupplierRequest is the body of POST /api/v1/admin/suppliers.
type SupplierRequest struct {
	Name          *string `json:"name"`
	WhatsAppPhone *string `json:"whatsappPhone"`
}

// Input returns the use case's input, or a ValidationError.
func (r SupplierRequest) Input() (appsupplier.CreateInput, error) {
	var f fields
	in := appsupplier.CreateInput{Name: need(&f, r.Name, "name"), WhatsAppPhone: need(&f, r.WhatsAppPhone, "whatsappPhone")}
	return in, f.err()
}

// ReplaceSupplierRequest is PUT /api/v1/admin/suppliers/{id}: a
// SupplierRequest and the version read, flat.
type ReplaceSupplierRequest struct {
	Name          *string `json:"name"`
	WhatsAppPhone *string `json:"whatsappPhone"`
	Version       *int    `json:"version"`
}

// Input returns the use case's input for the supplier id, or a
// ValidationError.
func (r ReplaceSupplierRequest) Input(id shared.SupplierID) (appsupplier.ReplaceInput, error) {
	var f fields
	in := appsupplier.ReplaceInput{
		ID: id, Name: need(&f, r.Name, "name"), WhatsAppPhone: need(&f, r.WhatsAppPhone, "whatsappPhone"),
		Version: needVersion(&f, r.Version),
	}
	return in, f.err()
}
