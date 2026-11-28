package purchasing

import (
	"context"
	"errors"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// OrderItems is purchasing's view of the order context
// (adapter/inproc/purchasing_orders.go over app/order's ForPurchasing; ADR
// 0031). Every method checks the caller's permission on the order side and
// joins the caller's transaction, and returns the order domain's refusals
// unchanged (order.ErrOrderClosed, order.ErrItemNotFound,
// order.ErrInvalidItemMove, order.ErrDateInPast, shared.ErrForbidden,
// shared.ErrVersionConflict).
type OrderItems interface {
	// ToBuy returns the bought items still to buy of purchasable orders,
	// earliest delivery first. Costs are set only for a caller who sees
	// them (identity.SeesCosts).
	ToBuy(ctx context.Context) (ToBuyItems, error)
	// Orders returns each order of ids, each once, in the order asked; an
	// order the store does not have is an error matching shared.ErrNotFound.
	Orders(ctx context.Context, ids []shared.OrderID) ([]OrderFacts, error)
	// Move applies moves to bought items in one go: all or none.
	Move(ctx context.Context, moves []ItemMove) error
	// RecordPromisedDates records the supplier's promised dates, which
	// redate the items' forecast; the promise to the client never moves.
	RecordPromisedDates(ctx context.Context, dates []ItemPromise) error
	// RecordPaidCosts records the unit prices paid (owner only).
	RecordPaidCosts(ctx context.Context, costs []ItemCost) error
}

// ToBuyItems is "A comprar" as the order context gives it.
type ToBuyItems struct {
	Items     []ToBuyItem
	Truncated bool // more active orders than one read covers
}

// ToBuyItem is one bought item still to buy, with the supplier it was sold
// with. Cost is the unit cost at order time, nil unless the caller sees
// costs; it is shown, never stored (the repository copies it in SQL).
type ToBuyItem struct {
	Item         purchasing.Item
	Number       string // the order's, "P-0231": staff see it, the supplier never does
	Supplier     shared.SupplierID
	SupplierName string
	ArrivalLimit shared.CivilDate
	Cost         *shared.Money
}

// OrderFacts is one order as purchasing needs it now.
type OrderFacts struct {
	Order        shared.OrderID
	Number       string
	Cancelled    bool
	Closed       bool // handed over or cancelled
	ArrivalLimit shared.CivilDate
	Items        []OrderItemFacts
}

// Item returns the order's item id, and false when the order has none
// (removed).
func (f OrderFacts) Item(id shared.OrderItemID) (OrderItemFacts, bool) {
	for _, it := range f.Items {
		if it.Item == id {
			return it, true
		}
	}
	return OrderItemFacts{}, false
}

// OrderItemFacts is one item of an order as it is now.
type OrderItemFacts struct {
	Item         shared.OrderItemID
	Bought       bool
	Status       order.ItemStatus
	Arrived      bool
	Name         string
	Quantity     int
	Choices      []purchasing.Choice
	Expected     shared.CivilDate
	Promised     shared.CivilDate // zero when the supplier promised no date
	Alerts       []schedule.Alert
	Supplier     shared.SupplierID
	SupplierName string
}

// ItemMove is one move of a bought item: to sent_to_supplier, in_transit,
// received with its check (a defect lands on to_buy), or back to to_buy.
type ItemMove struct {
	Order shared.OrderID
	Item  shared.OrderItemID
	To    order.ItemStatus
	Check *purchasing.Check
	Note  string // a defect's note
	Ref   string // the pedido de compra, "C-0012"; empty at the counter
}

// ItemPromise is a supplier's promised date for one item.
type ItemPromise struct {
	Order shared.OrderID
	Item  shared.OrderItemID
	Date  shared.CivilDate
	Ref   string
}

// ItemCost is the unit price paid for one item.
type ItemCost struct {
	Order shared.OrderID
	Item  shared.OrderItemID
	Cost  shared.Money
}

// Suppliers reads the store's suppliers (inproc over app/supplier).
type Suppliers interface {
	// Contact returns the supplier's name and WhatsApp, or an error
	// matching shared.ErrNotFound.
	Contact(ctx context.Context, id shared.SupplierID) (SupplierContact, error)
}

// SupplierContact is who a pedido de compra goes to.
type SupplierContact struct {
	ID    shared.SupplierID
	Name  string
	Phone string // digits for wa.me: 55 + DDD + number
}

// Store gives the store's facts purchasing needs (inproc over tenancy and
// the clock): its name for the supplier texts and its day, in its time
// zone, for dates.
type Store interface {
	Name(ctx context.Context) (string, error)
	Today(ctx context.Context) (shared.CivilDate, error)
}

// SupplierLinks builds wa.me links with a prefilled text (adapter/notify).
// Nothing is sent (CLAUDE.md, rule 8).
type SupplierLinks interface {
	Link(phone, text string) (string, error)
}

// Errors of the receipt photo store, mapped by its adapter from the
// storage's own.
var (
	// ErrObjectMissing means nothing was uploaded to the key.
	ErrObjectMissing = errors.New("purchasing: receipt photo not uploaded")
	// ErrObjectChanged means the upload was replaced after it was checked.
	ErrObjectChanged = errors.New("purchasing: receipt photo changed")
)

// SignedUpload is how an app uploads one object: Method to URL, sending
// exactly Headers, before ExpiresAt.
type SignedUpload struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// ObjectInfo is what storage knows of an uploaded object.
type ObjectInfo struct {
	Size int64
	ETag string
}

// ReceiptPhotos is the object store of receiving photos
// (adapter/storage/receipts.go), staged and promoted as ADR 0028 says, under
// purchasing.PhotoKey. Every failure but those named matches
// shared.ErrUnavailable.
type ReceiptPhotos interface {
	PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (SignedUpload, error)
	// Stat returns the staged upload at key, or ErrObjectMissing.
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	// Head returns up to n leading bytes of the staged upload at key while
	// it is still the version etag names: ErrObjectMissing, ErrObjectChanged.
	Head(ctx context.Context, key, etag string, n int) ([]byte, error)
	// Promote copies the staged upload to its place: ErrObjectMissing,
	// ErrObjectChanged. Promoting again is harmless.
	Promote(ctx context.Context, key, etag, contentType string) error
	// DeleteUpload removes a staged upload; a missing one is no error.
	DeleteUpload(ctx context.Context, key string) error
	// ReportOrphan logs a promoted object with no row. It never fails.
	ReportOrphan(ctx context.Context, key string)
}
