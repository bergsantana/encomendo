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
// the clock): its name for the supplier texts, its currency for the prices
// paid, and its day and time zone for dates.
type Store interface {
	Name(ctx context.Context) (string, error)
	// Today returns the store's current day in its time zone.
	Today(ctx context.Context) (shared.CivilDate, error)
	// Currency returns the store's ISO 4217 code, such as "BRL": the
	// currency of the prices paid, which the app sends as bare cents.
	Currency(ctx context.Context) (string, error)
	// Location returns the store's time zone, to turn an instant into the
	// day it fell on in the store (ADR 0030).
	Location(ctx context.Context) (*time.Location, error)
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

// UseCases is the driving port of Compras and Receber itens, which the HTTP
// adapter calls (docs/plans/m5-code-design.md section 3.2). Every method
// checks its permission first and returns an error matching
// shared.ErrForbidden without it; the writes that create (Send, Receive)
// also need an Idempotency-Key.
type UseCases interface {
	// ToBuy is "A comprar" by supplier (purchase:write). Costs are the
	// owner's only.
	ToBuy(ctx context.Context) (ToBuyView, error)
	// Preview returns the message a pedido de compra would send, taking no
	// number and writing nothing (purchase:write).
	Preview(ctx context.Context, in PickInput) (string, error)
	// Send makes the pedido de compra of the picked items (purchase:write).
	Send(ctx context.Context, in SendInput) (SentView, error)
	// SupplierOrders lists the pedidos de compra, newest first
	// (purchase:write); open leaves out the received and the undone.
	SupplierOrders(ctx context.Context, open bool, p shared.PageRequest) (shared.Page[SupplierOrderSummary], error)
	// SupplierOrder returns one pedido de compra with its lines
	// (purchase:write).
	SupplierOrder(ctx context.Context, id shared.SupplierOrderID) (SupplierOrderView, error)
	// RecordPromisedDate records the supplier's promised dates (purchase:write).
	RecordPromisedDate(ctx context.Context, in PromisedDateInput) (SupplierOrderView, error)
	// MarkInTransit marks every sent line in transit (purchase:write).
	MarkInTransit(ctx context.Context, in VersionInput) (SupplierOrderView, error)
	// Undo is "Desfazer envio" (purchase:write).
	Undo(ctx context.Context, in VersionInput) (SupplierOrderView, error)
	// RecordPaidPrices records the unit prices paid (the owner only).
	RecordPaidPrices(ctx context.Context, in PaidPricesInput) (SupplierOrderView, error)
	// ToReceive is what Receber itens lists (receiving:write); it carries no cost.
	ToReceive(ctx context.Context) (ToReceiveView, error)
	// RequestPhotoUpload signs the upload of one receiving photo (receiving:write).
	RequestPhotoUpload(ctx context.Context, contentType string) (PhotoUploadView, error)
	// Receive saves one receipt (receiving:write).
	Receive(ctx context.Context, in ReceiptInput) (ReceiptView, error)
	// Notices lists the sent lines no longer needed (purchase:write).
	Notices(ctx context.Context) ([]NoticeView, error)
	// ResolveNotice is "Marcar como resolvido" (purchase:write).
	ResolveNotice(ctx context.Context, line shared.PurchaseLineID) error
	// OrderLines is what purchasing knows of one order's bought items
	// (order:read); the chat link only for purchase:write.
	OrderLines(ctx context.Context, id shared.OrderID) ([]OrderLineView, error)
	// Today is Hoje's purchasing entries (purchase:write).
	Today(ctx context.Context) (TodayView, error)
}

// Repository stores supplier orders, their lines, counter lines and
// receipts (adapter/postgres/purchasing_repo.go; the fake in fake/ holds to
// the same contract). Every method runs inside TxManager.WithinTx (else
// appshared.ErrNoTransaction), in the scope of tenant: a tenant other than
// the scope's is refused, as RLS would. Every purchasing table is staff
// only, so a scope without a staff user sees nothing and writes nothing.
//
// Costs are the repository's, not the app's. InsertSupplierOrder and
// InsertCounterLine copy each line's order-time unit cost from the order's
// sourcing in the same transaction, so a line sent by an admin, who never
// sees a cost, keeps the owner's. Reads return Cost and Paid on every
// line; the app strips them for anyone but the owner. Updates never change
// Cost, and write Paid (ADR 0031).
type Repository interface {
	// NextNumber takes the store's next supplier order number: 1, then 2,
	// and so on, each issued once. Two concurrent callers never get the
	// same number. A rolled back transaction gives its number back; a
	// committed one never does, so an undone order's number is never
	// reused.
	NextNumber(ctx context.Context, tenant shared.TenantID) (purchasing.Number, error)
	// PeekNumber returns the number NextNumber would give next and takes
	// nothing: for Preview.
	PeekNumber(ctx context.Context, tenant shared.TenantID) (purchasing.Number, error)
	// InsertSupplierOrder stores a new supplier order (version 1) with its
	// lines, in the order given, and the message as it was sent: text and
	// the wa.me url. It returns ErrOpenLine (a shared.ErrConflict), storing
	// nothing, when any line's order item already has an open (sent or in
	// transit) line, and another shared.ErrConflict when the id, the number
	// or a line id is taken.
	InsertSupplierOrder(ctx context.Context, o *purchasing.SupplierOrder, text, url string) error
	// UpdateSupplierOrder stores what changes on a supplier order: its
	// promised date, in-transit and undone marks, and for each line its
	// status, promised date, note, cancel reason, received day and paid
	// price. The stored version must be o.Version(), else it returns
	// shared.ErrVersionConflict; the next version is stored. It returns
	// purchasing.ErrSupplierOrderNotFound for an unknown id. It adds and
	// removes no line.
	UpdateSupplierOrder(ctx context.Context, o *purchasing.SupplierOrder) error
	// SupplierOrder returns the supplier order with its lines in the order
	// they were picked, or purchasing.ErrSupplierOrderNotFound.
	SupplierOrder(ctx context.Context, tenant shared.TenantID, id shared.SupplierOrderID) (purchasing.Record, error)
	// SupplierOrderOfLine returns the supplier order that holds the line,
	// or purchasing.ErrLineNotFound for an unknown line and for a line
	// received at the counter, which has none.
	SupplierOrderOfLine(ctx context.Context, tenant shared.TenantID, line shared.PurchaseLineID) (purchasing.Record, error)
	// Message returns the message a supplier order was sent with, or
	// purchasing.ErrSupplierOrderNotFound.
	Message(ctx context.Context, tenant shared.TenantID, id shared.SupplierOrderID) (SentMessage, error)
	// SupplierOrders returns one page of the store's supplier orders, newest
	// sent first (ties by id, newest first). With open, only the orders
	// with a line still sent or in transit. A cursor the repository did not
	// issue is a *shared.ValidationError on "cursor".
	SupplierOrders(ctx context.Context, tenant shared.TenantID, open bool, p shared.PageRequest) (shared.Page[purchasing.Record], error)
	// OpenLines returns every sent or in-transit line of the store, oldest
	// supplier order first (sent time, then id), then in the order picked.
	OpenLines(ctx context.Context, tenant shared.TenantID) ([]StoredLine, error)
	// LinesOfOrder returns every line, in any status, whose order is id,
	// counter lines included, in the order they were written (lines written
	// together keep the order picked). A line whose item was removed from
	// the order is still there, with a zero order item.
	LinesOfOrder(ctx context.Context, tenant shared.TenantID, id shared.OrderID) ([]StoredLine, error)
	// DefectiveItems reports which of items have a defective line: the map
	// holds a true entry for each of those items, and no entry for the rest.
	DefectiveItems(ctx context.Context, tenant shared.TenantID, items []shared.OrderItemID) (map[shared.OrderItemID]bool, error)
	// InsertCounterLine stores a line received at the counter: received or
	// defective, with no supplier order. It returns shared.ErrConflict when
	// the id is taken.
	InsertCounterLine(ctx context.Context, tenant shared.TenantID, l purchasing.Line) error
	// InsertReceipt stores a receipt of lines already stored received or
	// defective, with its photos. It returns purchasing.ErrLineClosed when
	// a line is already in a receipt, and shared.ErrConflict when the id or
	// a photo key is taken.
	InsertReceipt(ctx context.Context, r *purchasing.Receipt) error
	// Receipt returns a stored receipt, or ErrReceiptNotFound.
	Receipt(ctx context.Context, tenant shared.TenantID, id shared.ReceiptID) (StoredReceipt, error)
	// StaffNames returns the names of the users in ids who are members of
	// tenant; any other id, a user of another store included, is left out
	// (security review of 5.6–5.8: never a read of users by id alone).
	StaffNames(ctx context.Context, tenant shared.TenantID, ids []shared.UserID) (map[shared.UserID]string, error)
}

// SentMessage is the message a supplier order was sent with, kept as it
// went out (ADR 0031).
type SentMessage struct {
	Text string
	URL  string // the wa.me link with Text
}

// StoredLine is a stored line with the facts of its supplier order, which
// the repository joins. For a line received at the counter the supplier
// order facts are all zero.
type StoredLine struct {
	purchasing.LineRecord
	Number        purchasing.Number
	SupplierName  string // as sent
	SupplierPhone string // as sent
	SentAt        time.Time
	OrderPromised shared.CivilDate // the supplier order's promised date
}

// EffectivePromised returns the day the supplier promised the line: its
// own date, or its supplier order's.
func (l StoredLine) EffectivePromised() shared.CivilDate {
	if !l.Promised.IsZero() {
		return l.Promised
	}
	return l.OrderPromised
}

// StoredReceipt is a stored receipt: when it was saved and the lines it
// checked, in the order of its entries. A line's status says its check.
type StoredReceipt struct {
	ID    shared.ReceiptID
	At    time.Time
	Lines []purchasing.LineRecord
}
