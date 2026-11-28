package postgres

import (
	"context"
	"errors"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// errPurchasingNotBuilt is what every Purchasing method answers until the
// real repository lands.
var errPurchasingNotBuilt = errors.New("purchasing repository not built")

// Purchasing implements apppurchasing.Repository.
//
// Temporary until step 5.9 lands; replaced on merge. It lets the HTTP
// wiring of step 5.10 build and be tested on its own branch; every method
// answers an error.
type Purchasing struct{}

var _ apppurchasing.Repository = (*Purchasing)(nil)

// NewPurchasing returns the placeholder repository over ids.
func NewPurchasing(appshared.IDGenerator) *Purchasing { return &Purchasing{} }

// NextNumber implements apppurchasing.Repository.
func (*Purchasing) NextNumber(context.Context, shared.TenantID) (purchasing.Number, error) {
	return 0, errPurchasingNotBuilt
}

// PeekNumber implements apppurchasing.Repository.
func (*Purchasing) PeekNumber(context.Context, shared.TenantID) (purchasing.Number, error) {
	return 0, errPurchasingNotBuilt
}

// InsertSupplierOrder implements apppurchasing.Repository.
func (*Purchasing) InsertSupplierOrder(context.Context, *purchasing.SupplierOrder, string, string) error {
	return errPurchasingNotBuilt
}

// UpdateSupplierOrder implements apppurchasing.Repository.
func (*Purchasing) UpdateSupplierOrder(context.Context, *purchasing.SupplierOrder) error {
	return errPurchasingNotBuilt
}

// SupplierOrder implements apppurchasing.Repository.
func (*Purchasing) SupplierOrder(context.Context, shared.TenantID, shared.SupplierOrderID) (purchasing.Record, error) {
	return purchasing.Record{}, errPurchasingNotBuilt
}

// SupplierOrderOfLine implements apppurchasing.Repository.
func (*Purchasing) SupplierOrderOfLine(context.Context, shared.TenantID, shared.PurchaseLineID) (purchasing.Record, error) {
	return purchasing.Record{}, errPurchasingNotBuilt
}

// Message implements apppurchasing.Repository.
func (*Purchasing) Message(context.Context, shared.TenantID, shared.SupplierOrderID) (apppurchasing.SentMessage, error) {
	return apppurchasing.SentMessage{}, errPurchasingNotBuilt
}

// SupplierOrders implements apppurchasing.Repository.
func (*Purchasing) SupplierOrders(context.Context, shared.TenantID, bool, shared.PageRequest) (shared.Page[purchasing.Record], error) {
	return shared.Page[purchasing.Record]{}, errPurchasingNotBuilt
}

// OpenLines implements apppurchasing.Repository.
func (*Purchasing) OpenLines(context.Context, shared.TenantID) ([]apppurchasing.StoredLine, error) {
	return nil, errPurchasingNotBuilt
}

// LinesOfOrder implements apppurchasing.Repository.
func (*Purchasing) LinesOfOrder(context.Context, shared.TenantID, shared.OrderID) ([]apppurchasing.StoredLine, error) {
	return nil, errPurchasingNotBuilt
}

// DefectiveItems implements apppurchasing.Repository.
func (*Purchasing) DefectiveItems(context.Context, shared.TenantID, []shared.OrderItemID) (map[shared.OrderItemID]bool, error) {
	return nil, errPurchasingNotBuilt
}

// InsertCounterLine implements apppurchasing.Repository.
func (*Purchasing) InsertCounterLine(context.Context, shared.TenantID, purchasing.Line) error {
	return errPurchasingNotBuilt
}

// InsertReceipt implements apppurchasing.Repository.
func (*Purchasing) InsertReceipt(context.Context, *purchasing.Receipt) error {
	return errPurchasingNotBuilt
}

// Receipt implements apppurchasing.Repository.
func (*Purchasing) Receipt(context.Context, shared.TenantID, shared.ReceiptID) (apppurchasing.StoredReceipt, error) {
	return apppurchasing.StoredReceipt{}, errPurchasingNotBuilt
}

// StaffNames implements apppurchasing.Repository.
func (*Purchasing) StaffNames(context.Context, []shared.UserID) (map[shared.UserID]string, error) {
	return nil, errPurchasingNotBuilt
}
