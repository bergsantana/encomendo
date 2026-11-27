package models

import (
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// Supplier maps suppliers.
type Supplier struct {
	TenantID      string `gorm:"column:tenant_id"`
	ID            string `gorm:"column:id;primaryKey"`
	Name          string `gorm:"column:name"`
	WhatsAppPhone string `gorm:"column:whatsapp_phone"`
	Version       int    `gorm:"column:version"`
}

// TableName returns suppliers.
func (Supplier) TableName() string { return "suppliers" }

// SupplierFromDomain returns the row of s.
func SupplierFromDomain(s *supplier.Supplier) Supplier {
	return Supplier{
		TenantID: s.TenantID().String(), ID: s.ID().String(), Name: s.Name(),
		WhatsAppPhone: s.WhatsAppPhone(), Version: s.Version(),
	}
}

// ToDomain rebuilds the supplier.
func (m Supplier) ToDomain() (*supplier.Supplier, error) {
	tenant, err := parseID[shared.TenantID](m.TenantID)
	if err != nil {
		return nil, err
	}
	id, err := parseID[shared.SupplierID](m.ID)
	if err != nil {
		return nil, err
	}
	return supplier.Restore(supplier.Record{Tenant: tenant, ID: id, Name: m.Name, Phone: m.WhatsAppPhone, Version: m.Version})
}

// SupplierItem maps supplier_items: one item's main or backup supplier with
// its lead time and cost. CostCents is null when no cost is recorded.
type SupplierItem struct {
	TenantID   string `gorm:"column:tenant_id"`
	ItemID     string `gorm:"column:item_id"`
	SupplierID string `gorm:"column:supplier_id"`
	Role       string `gorm:"column:role"`
	LeadDays   int    `gorm:"column:lead_days"`
	CostCents  *int64 `gorm:"column:cost_cents"`
	Currency   string `gorm:"column:currency"`
}

// TableName returns supplier_items.
func (SupplierItem) TableName() string { return "supplier_items" }

// SupplierItemFromDomain returns the row that links item to si's supplier.
// currency is the item's, used when the link has no cost, because the column
// is never null.
func SupplierItemFromDomain(tenant shared.TenantID, item shared.ItemID, si supplier.SupplierItem, currency string) SupplierItem {
	m := SupplierItem{
		TenantID: tenant.String(), ItemID: item.String(), SupplierID: si.SupplierID().String(),
		Role: string(si.Role()), LeadDays: si.LeadDays(), Currency: currency,
	}
	if c := si.Cost(); c != nil {
		cents := c.Cents()
		m.CostCents, m.Currency = &cents, c.Currency()
	}
	return m
}

// ToDomain rebuilds the link.
func (m SupplierItem) ToDomain() (supplier.SupplierItem, error) {
	id, err := parseID[shared.SupplierID](m.SupplierID)
	if err != nil {
		return supplier.SupplierItem{}, err
	}
	cost, err := optionalMoney(m.CostCents, m.Currency)
	if err != nil {
		return supplier.SupplierItem{}, err
	}
	return supplier.NewSupplierItem(id, supplier.Role(m.Role), m.LeadDays, cost)
}

// optionalMoney returns the amount of a nullable cost column, or nil.
func optionalMoney(cents *int64, currency string) (*shared.Money, error) {
	if cents == nil {
		return nil, nil //nolint:nilnil // a null column is "no amount", not an error
	}
	m, err := shared.NewMoney(*cents, currency)
	if err != nil {
		return nil, err
	}
	return &m, nil
}
