package dto

import (
	"fmt"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The request types below mirror the contract's request schemas. The
// handler decodes them with unknown fields refused, and every required key
// that is missing is reported by name (400 invalid). A Replace…Request
// embeds its base request next to the version read. That is safe only while
// the base has no UnmarshalJSON of its own: a promoted one would decode the
// whole body and swallow version. TestCatalog_ReplaceReachesTheUseCaseWhole
// fails if that ever happens.

// CategoryRequest is POST /api/v1/admin/catalog/categories.
type CategoryRequest struct {
	Name *string `json:"name"`
}

// Input returns the use case's input, or a ValidationError.
func (r CategoryRequest) Input() (appcatalog.CreateCategoryInput, error) {
	var f fields
	in := appcatalog.CreateCategoryInput{Name: need(&f, r.Name, "name")}
	return in, f.err()
}

// UpdateCategoryRequest is PATCH /api/v1/admin/catalog/categories/{id}:
// name, position or both.
type UpdateCategoryRequest struct {
	Name     *string `json:"name"`
	Position *int    `json:"position"`
}

// Input returns the use case's input for the category id, or a
// ValidationError when the body names nothing to change.
func (r UpdateCategoryRequest) Input(id shared.CategoryID) (appcatalog.UpdateCategoryInput, error) {
	if r.Name == nil && r.Position == nil {
		return appcatalog.UpdateCategoryInput{}, shared.NewValidationError(
			shared.FieldError{Field: "name", Message: "Informe o nome ou a posição."})
	}
	return appcatalog.UpdateCategoryInput{ID: id, Name: r.Name, Position: r.Position}, nil
}

// AttributeValueRequest is one value of an item field.
type AttributeValueRequest struct {
	ID    *string `json:"id"`
	Label *string `json:"label"`
}

// AttributeRequest is the body of POST /api/v1/admin/catalog/attributes
// and of PUT /attributes/{id}.
type AttributeRequest struct {
	Name   *string                 `json:"name"`
	Values []AttributeValueRequest `json:"values"`
}

// Input returns the use case's input, or a ValidationError.
func (r AttributeRequest) Input() (appcatalog.AttributeInput, error) {
	var f fields
	in := appcatalog.AttributeInput{Name: need(&f, r.Name, "name")}
	for i, v := range needList(&f, r.Values, "values") {
		path := fmt.Sprintf("values[%d]", i)
		vi := appcatalog.ValueInput{Label: need(&f, v.Label, path+".label")}
		if v.ID != nil {
			vi.ID = parseID[shared.AttributeValueID](&f, *v.ID, path+".id")
		}
		in.Values = append(in.Values, vi)
	}
	return in, f.err()
}

// SupplierItemRequest is the SupplierItemInput schema: a supplier of the
// item, its lead days and, from the owner only, its cost.
type SupplierItemRequest struct {
	SupplierID *string         `json:"supplierId"`
	LeadDays   *int            `json:"leadDays"`
	CostCents  Optional[int64] `json:"costCents"`
}

func (r *SupplierItemRequest) input(f *fields, path string) *appcatalog.SupplierInput {
	if r == nil {
		return nil
	}
	return &appcatalog.SupplierInput{
		SupplierID: needID[shared.SupplierID](f, r.SupplierID, path+".supplierId"),
		LeadDays:   need(f, r.LeadDays, path+".leadDays"),
		Cost:       CostField(r.CostCents),
	}
}

// ChoiceRequest is one choice of a customization option.
type ChoiceRequest struct {
	ID              *string `json:"id"`
	Label           *string `json:"label"`
	ExtraPriceCents *int64  `json:"extraPriceCents"`
	ExtraDays       *int    `json:"extraDays"`
}

// OptionRequest is the CustomizationOptionInput schema.
type OptionRequest struct {
	ID       *string         `json:"id"`
	Name     *string         `json:"name"`
	Required *bool           `json:"required"`
	Choices  []ChoiceRequest `json:"choices"`
}

func (r OptionRequest) input(f *fields, path string) appcatalog.OptionInput {
	o := appcatalog.OptionInput{Name: need(f, r.Name, path+".name"), Required: need(f, r.Required, path+".required")}
	if r.ID != nil {
		o.ID = parseID[shared.OptionID](f, *r.ID, path+".id")
	}
	for i, c := range needList(f, r.Choices, path+".choices") {
		cp := fmt.Sprintf("%s.choices[%d]", path, i)
		ci := appcatalog.ChoiceInput{
			Label: need(f, c.Label, cp+".label"), ExtraPriceCents: need(f, c.ExtraPriceCents, cp+".extraPriceCents"),
			ExtraDays: need(f, c.ExtraDays, cp+".extraDays"),
		}
		if c.ID != nil {
			ci.ID = parseID[shared.ChoiceID](f, *c.ID, cp+".id")
		}
		o.Choices = append(o.Choices, ci)
	}
	return o
}

// ItemRequest is the ItemInput schema (POST /api/v1/admin/catalog/items).
// The nullable fields (categoryId, productionDays, the suppliers) mean "none"
// when null; the cost keys are tri-state (Optional).
type ItemRequest struct {
	Name                *string              `json:"name"`
	Description         *string              `json:"description"`
	CategoryID          Optional[string]     `json:"categoryId"`
	SourcingMode        *string              `json:"sourcingMode"`
	PriceCents          *int64               `json:"priceCents"`
	ProductionDays      *int                 `json:"productionDays"`
	ProductionCostCents Optional[int64]      `json:"productionCostCents"`
	MainSupplier        *SupplierItemRequest `json:"mainSupplier"`
	BackupSupplier      *SupplierItemRequest `json:"backupSupplier"`
	AttributeValueIDs   []string             `json:"attributeValueIds"`
	Options             []OptionRequest      `json:"options"`
	Published           *bool                `json:"published"`
}

// Input returns the use case's input, or a ValidationError naming every
// missing or malformed field.
func (r ItemRequest) Input() (appcatalog.ItemInput, error) {
	var f fields
	in := r.input(&f)
	return in, f.err()
}

func (r ItemRequest) input(f *fields) appcatalog.ItemInput {
	in := appcatalog.ItemInput{
		Name: need(f, r.Name, "name"), Description: need(f, r.Description, "description"),
		SourcingMode: catalog.SourcingMode(need(f, r.SourcingMode, "sourcingMode")), PriceCents: need(f, r.PriceCents, "priceCents"),
		ProductionCost: CostField(r.ProductionCostCents),
		MainSupplier:   r.MainSupplier.input(f, "mainSupplier"), BackupSupplier: r.BackupSupplier.input(f, "backupSupplier"),
		AttributeValueIDs: parseIDs[shared.AttributeValueID](f, needList(f, r.AttributeValueIDs, "attributeValueIds"), "attributeValueIds"),
		Published:         need(f, r.Published, "published"),
	}
	if r.ProductionDays != nil {
		// 0 means "none" to the use case, so a 0 sent is refused here,
		// not read as absent (the contract's minimum is 1).
		if *r.ProductionDays < 1 {
			f.add("productionDays", "Informe os dias de produção, a partir de 1.")
		}
		in.ProductionDays = *r.ProductionDays
	}
	needKey(f, r.CategoryID, "categoryId")
	if raw, ok := nullable(r.CategoryID); ok {
		in.CategoryID = parseID[shared.CategoryID](f, raw, "categoryId")
	}
	for i, o := range needList(f, r.Options, "options") {
		in.Options = append(in.Options, o.input(f, fmt.Sprintf("options[%d]", i)))
	}
	return in
}

// ReplaceItemRequest is PUT /api/v1/admin/catalog/items/{id}: an
// ItemRequest and the version read.
type ReplaceItemRequest struct {
	ItemRequest
	Version *int `json:"version"`
}

// Input returns the use case's input for the item id, or a
// ValidationError.
func (r ReplaceItemRequest) Input(id shared.ItemID) (appcatalog.ReplaceItemInput, error) {
	var f fields
	item := r.input(&f)
	version := needVersion(&f, r.Version)
	return appcatalog.ReplaceItemInput{ID: id, Item: item, Version: version}, f.err()
}

// needVersion returns the version read, which must be at least 1.
func needVersion(f *fields, v *int) int {
	n := need(f, v, "version")
	if v != nil && n < 1 {
		f.add("version", "Informe a versão que você leu.")
	}
	return n
}

// VersionRequest is the body of archive and restore.
type VersionRequest struct {
	Version *int `json:"version"`
}

// Input returns the version read, or a ValidationError.
func (r VersionRequest) Input() (int, error) {
	var f fields
	v := needVersion(&f, r.Version)
	return v, f.err()
}

// SetPublishedRequest is the body of the published routes.
type SetPublishedRequest struct {
	Published *bool `json:"published"`
	Version   *int  `json:"version"`
}

// Input returns the published flag and the version read, or a
// ValidationError.
func (r SetPublishedRequest) Input() (published bool, version int, err error) {
	var f fields
	published = need(&f, r.Published, "published")
	version = needVersion(&f, r.Version)
	return published, version, f.err()
}

// KitItemRequest is one item of a kit.
type KitItemRequest struct {
	ItemID             *string  `json:"itemId"`
	Quantity           *int     `json:"quantity"`
	Swappable          *bool    `json:"swappable"`
	AlternativeItemIDs []string `json:"alternativeItemIds"`
}

// KitRequest is the KitInput schema (POST /api/v1/admin/catalog/kits).
type KitRequest struct {
	Name              *string          `json:"name"`
	Description       *string          `json:"description"`
	PriceCents        *int64           `json:"priceCents"`
	SwapDiscount      *string          `json:"swapDiscount"`
	AssemblyDays      Optional[int]    `json:"assemblyDays"`
	SafetyMarginDays  Optional[int]    `json:"safetyMarginDays"`
	Items             []KitItemRequest `json:"items"`
	AttributeValueIDs []string         `json:"attributeValueIds"`
	Published         *bool            `json:"published"`
}

// Input returns the use case's input, or a ValidationError.
func (r KitRequest) Input() (appcatalog.KitInput, error) {
	var f fields
	in := r.input(&f)
	return in, f.err()
}

func (r KitRequest) input(f *fields) appcatalog.KitInput {
	in := appcatalog.KitInput{
		Name: need(f, r.Name, "name"), Description: need(f, r.Description, "description"),
		PriceCents: need(f, r.PriceCents, "priceCents"), SwapDiscount: catalog.SwapMode(need(f, r.SwapDiscount, "swapDiscount")),
		AttributeValueIDs: parseIDs[shared.AttributeValueID](f, needList(f, r.AttributeValueIDs, "attributeValueIds"), "attributeValueIds"),
		Published:         need(f, r.Published, "published"),
	}
	needKey(f, r.AssemblyDays, "assemblyDays")
	needKey(f, r.SafetyMarginDays, "safetyMarginDays")
	in.AssemblyDays, in.SafetyMarginDays = daysOf(r.AssemblyDays), daysOf(r.SafetyMarginDays)
	for i, it := range needList(f, r.Items, "items") {
		path := fmt.Sprintf("items[%d]", i)
		in.Items = append(in.Items, catalog.KitItem{
			Item:         needID[shared.ItemID](f, it.ItemID, path+".itemId"),
			Quantity:     need(f, it.Quantity, path+".quantity"),
			Swappable:    need(f, it.Swappable, path+".swappable"),
			Alternatives: parseIDs[shared.ItemID](f, needList(f, it.AlternativeItemIDs, path+".alternativeItemIds"), path+".alternativeItemIds"),
		})
	}
	return in
}

// daysOf returns a pointer to the days sent, nil for the store's default
// (absent or null).
func daysOf(o Optional[int]) *int {
	if v, ok := nullable(o); ok {
		return &v
	}
	return nil
}

// ReplaceKitRequest is PUT /api/v1/admin/catalog/kits/{id}: a KitRequest
// and the version read.
type ReplaceKitRequest struct {
	KitRequest
	Version *int `json:"version"`
}

// Input returns the use case's input for the kit id, or a ValidationError.
func (r ReplaceKitRequest) Input(id shared.KitID) (appcatalog.ReplaceKitInput, error) {
	var f fields
	kit := r.input(&f)
	version := needVersion(&f, r.Version)
	return appcatalog.ReplaceKitInput{ID: id, Kit: kit, Version: version}, f.err()
}

// PhotoUploadRequest is POST /api/v1/admin/catalog/photo-uploads.
type PhotoUploadRequest struct {
	OwnerType   *string `json:"ownerType"`
	OwnerID     *string `json:"ownerId"`
	ContentType *string `json:"contentType"`
}

// Input returns the use case's input, or a ValidationError. The content
// type is checked by the use case, which owns the list of types.
func (r PhotoUploadRequest) Input() (appcatalog.PhotoUploadInput, error) {
	var f fields
	in := appcatalog.PhotoUploadInput{ContentType: need(&f, r.ContentType, "contentType")}
	kind := catalog.OwnerKind(need(&f, r.OwnerType, "ownerType"))
	if r.OwnerType != nil && kind != catalog.OwnerItem && kind != catalog.OwnerKit {
		f.add("ownerType", "Informe item ou kit.")
	}
	in.Owner = catalog.PhotoOwner{Kind: kind, ID: needID[shared.ID](&f, r.OwnerID, "ownerId")}
	return in, f.err()
}

// ConfirmPhotoRequest is the body of the photo confirm routes.
type ConfirmPhotoRequest struct {
	Key *string `json:"key"`
}

// Input returns the key, or a ValidationError when it is missing or empty.
func (r ConfirmPhotoRequest) Input() (string, error) {
	var f fields
	key := need(&f, r.Key, "key")
	if r.Key != nil && key == "" {
		f.add("key", msgRequired)
	}
	return key, f.err()
}

// OrderPhotosRequest is the body of the photo order routes.
type OrderPhotosRequest struct {
	PhotoIDs []string `json:"photoIds"`
}

// Input returns the photo ids in the new order, or a ValidationError.
func (r OrderPhotosRequest) Input() ([]shared.PhotoID, error) {
	var f fields
	ids := parseIDs[shared.PhotoID](&f, needList(&f, r.PhotoIDs, "photoIds"), "photoIds")
	return ids, f.err()
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
// SupplierRequest and the version read.
type ReplaceSupplierRequest struct {
	SupplierRequest
	Version *int `json:"version"`
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
