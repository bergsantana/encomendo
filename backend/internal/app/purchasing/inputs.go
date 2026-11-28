package purchasing

import (
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// PickInput is the items picked for one supplier (SupplierOrderRequest).
type PickInput struct {
	Supplier   shared.SupplierID
	OrderItems []shared.OrderItemID
}

// SendInput is a pick to send, with its Idempotency-Key.
type SendInput struct {
	PickInput
	Idempotency appshared.IdempotencyKey
}

// VersionInput names a supplier order at the version the caller read.
type VersionInput struct {
	ID      shared.SupplierOrderID
	Version int
}

// LineDate is a promised date for one line.
type LineDate struct {
	Line shared.PurchaseLineID
	Date shared.CivilDate
}

// PromisedDateInput records the supplier's promised date of a supplier
// order, with a different date for some lines (PromisedDateRequest).
type PromisedDateInput struct {
	ID      shared.SupplierOrderID
	Version int
	Date    shared.CivilDate
	Lines   []LineDate
}

// LinePrice is the unit price paid for one line, in cents of the store's
// currency.
type LinePrice struct {
	Line  shared.PurchaseLineID
	Cents int64
}

// PaidPricesInput records the unit prices paid (PaidPricesRequest).
type PaidPricesInput struct {
	ID      shared.SupplierOrderID
	Version int
	Prices  []LinePrice
}

// ReceiptEntryInput is the check of one line. Exactly one of Line (an open
// line of a pedido de compra) and OrderItem (an item still in "A comprar",
// bought at the counter) is set.
type ReceiptEntryInput struct {
	Line      shared.PurchaseLineID
	OrderItem shared.OrderItemID
	Check     purchasing.Check
	Note      string
	PhotoKeys []string
}

// ReceiptInput is one save on Receber itens (ReceiptRequest) with its
// Idempotency-Key.
type ReceiptInput struct {
	Entries          []ReceiptEntryInput
	InvoicePhotoKeys []string
	Idempotency      appshared.IdempotencyKey
}
