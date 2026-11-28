package order

import (
	"context"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// UseCases is the driving port: what the HTTP adapter (and the M7 store
// page) calls. Every write runs in one transaction with its events.
type UseCases interface {
	Quote(ctx context.Context, in QuoteInput) (QuoteView, error)
	Create(ctx context.Context, in CreateInput) (CreatedView, error)
	Change(ctx context.Context, in ChangeInput) (ChangedView, error)
	RecordPayment(ctx context.Context, in PaymentInput) (View, error)
	VoidPayment(ctx context.Context, in VoidPaymentInput) (View, error)
	Release(ctx context.Context, in VersionInput) (View, error)
	Advance(ctx context.Context, in AdvanceInput) (View, error)
	MoveItem(ctx context.Context, in MoveItemInput) (View, error)
	UseBackupSupplier(ctx context.Context, in ItemVersionInput) (View, error)
	ChangeDeliveryDate(ctx context.Context, in DateInput) (MessagedView, error)
	SendTrackingLink(ctx context.Context, id shared.OrderID) (LinkView, error)
	Cancel(ctx context.Context, in CancelInput) (View, error)
	Get(ctx context.Context, id shared.OrderID) (View, error)
	List(ctx context.Context, f ListFilter, p shared.PageRequest) (shared.Page[SummaryView], error)
	History(ctx context.Context, id shared.OrderID, p shared.PageRequest) (shared.Page[HistoryView], error)
	Today(ctx context.Context) (TodayView, error)
	Finance(ctx context.Context, from, to shared.CivilDate) (FinanceView, error)
}

// Repository stores orders. Every method runs in the caller's transaction.
type Repository interface {
	// NextNumber takes the store's next order number, in the caller's
	// transaction, so two orders never share one.
	NextNumber(ctx context.Context, tenant shared.TenantID) (order.Number, error)
	// Insert stores a new order with its lines, items, sourcing, payments and
	// pending history.
	Insert(ctx context.Context, o *order.Order) error
	// Update stores o while the stored version is still o.Version(), and the
	// next version; it writes the pending history. It returns
	// shared.ErrVersionConflict for a stale version and ErrNotFound when the
	// order is gone.
	Update(ctx context.Context, o *order.Order) error
	// ByID returns the stored order, or ErrNotFound. Outside a staff scope
	// the staff-only sourcing comes back empty.
	ByID(ctx context.Context, tenant shared.TenantID, id shared.OrderID) (order.Record, error)
	// AddMessage records a wa.me message made for an order. Its text never
	// holds a tracking link (ADR 0030, Amendments).
	AddMessage(ctx context.Context, m Message) error
	// StaffNames returns the name of each user in ids, for "Registrado por"
	// on payments; a user that no longer exists is absent, not an error.
	StaffNames(ctx context.Context, ids []shared.UserID) (map[shared.UserID]string, error)

	// List returns one page of the store's orders that match f, by
	// (delivery date, id) ascending. The cursor is the repository's own and
	// opaque; p.Limit() is at most shared.MaxPageLimit. f arrives validated
	// (List normalizes it): Scope is set, Phase is a phase, Query is
	// trimmed. Query is read with NumberQuery: a number matches the order's
	// number, anything else matches the start of the customer's name,
	// ignoring case, with LIKE wildcards in it taken literally. f.Alert is
	// not the repository's: List applies it.
	List(ctx context.Context, tenant shared.TenantID, f ListFilter, p shared.PageRequest) (shared.Page[order.Record], error)
	// Active returns up to limit orders neither handed over nor cancelled,
	// by (delivery date, id) ascending, each with when it entered its
	// current status.
	Active(ctx context.Context, tenant shared.TenantID, limit int) ([]ActiveOrder, error)
	// History returns one page of an order's history, newest first, by
	// (at, id) descending, or ErrNotFound when the store has no such order.
	History(ctx context.Context, tenant shared.TenantID, id shared.OrderID, p shared.PageRequest) (shared.Page[HistoryRow], error)
	// Payments returns the payments recorded in [from, to), voided ones
	// included and marked, oldest first.
	Payments(ctx context.Context, tenant shared.TenantID, from, to time.Time) ([]PaymentRow, error)
	// HandedOver returns the orders handed over in [from, to), by hand-over
	// time ascending, up to HandedOverCap of them.
	HandedOver(ctx context.Context, tenant shared.TenantID, from, to time.Time) ([]order.Record, error)
}

// ActiveOrder is a stored order that is neither handed over nor cancelled,
// with the moment it entered its current status: the time of the latest
// status move into it, or the order's creation when it never moved.
// Hoje counts days from it ("Aguardando o sinal há 4 dias").
type ActiveOrder struct {
	Record      order.Record
	StatusSince time.Time
}

// HistoryRow is one stored history entry with the name of whoever made it.
// ByName is empty for an entry with no user (an order placed on the store
// page) and for a user who no longer exists.
type HistoryRow struct {
	Entry  order.HistoryEntry
	ByName string
}

// PaymentRow is one stored payment, for the finance summary.
type PaymentRow struct {
	Order  shared.OrderID
	Kind   order.PaymentKind
	Method order.PaymentMethod
	Amount shared.Money
	At     time.Time
	Voided bool
}

// Message is one wa.me message made for an order, as order_messages keeps
// it: the text with any tracking link replaced, and the wa.me URL of that
// text.
type Message struct {
	ID      shared.ID
	Order   shared.OrderID
	Kind    MessageKind
	ToPhone string
	Text    string
	URL     string
	By      shared.UserID
	At      time.Time
}

// MessageKind says why a message was made.
type MessageKind string

// The message kinds.
const (
	MessageTrackingLink MessageKind = "tracking_link"
	MessageCreated      MessageKind = "created"
	MessageDateChanged  MessageKind = "date_changed"
	MessageChanged      MessageKind = "changed"
)

// Catalog resolves requested lines against the store's catalog: prices,
// snapshots and the staff-only sourcing. Ids are left zero; the service
// fills them. It refuses archived items and kits (catalog.ErrItemArchived).
type Catalog interface {
	OrderLines(ctx context.Context, reqs []LineRequest) ([]order.LineSpec, error)
}

// Customers reads and makes the store's customers for orders.
type Customers interface {
	// Get returns the customer, or an error matching shared.ErrNotFound.
	Get(ctx context.Context, id shared.CustomerID) (CustomerFacts, error)
	// FindOrCreate returns the store's customer with phone, unchanged, or
	// creates one with name and phone.
	FindOrCreate(ctx context.Context, name, phone string) (CustomerFacts, error)
}

// CustomerFacts is what an order needs of its customer.
type CustomerFacts struct {
	ID        shared.CustomerID
	Name      string
	Phone     string
	Addresses []order.DeliveryAddress
}

// Workflows gives the store's current status workflow for new orders, and a
// given version for existing ones (docs/architecture.md section 4.3,
// WorkflowSource).
type Workflows interface {
	Current(ctx context.Context) (workflow.Workflow, error)
	Get(ctx context.Context, id shared.WorkflowID, version int) (workflow.Workflow, error)
}

// Store gives the store's facts an order needs.
type Store interface {
	Facts(ctx context.Context) (StoreFacts, error)
}

// StoreFacts are the store's settings and identity for orders.
type StoreFacts struct {
	Name, Slug, WhatsApp           string
	Currency                       string
	DepositPercent                 int
	AssemblyDays, SafetyMarginDays int
	Location                       *time.Location // the store's time zone
}

// TrackingURLs builds the tracking page URL of a token on a store's page.
type TrackingURLs interface {
	URL(slug string, token shared.Secret) (string, error)
}

// WhatsApp builds wa.me links with a prefilled text. Nothing is sent.
type WhatsApp interface {
	Link(phone, text string) (string, error)
}
