package order

import (
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// LineRequest is one requested line: a kit (with swaps and options of its
// items) or a loose item with a quantity.
type LineRequest struct {
	Kit      shared.KitID
	Item     shared.ItemID
	Quantity int
	Swaps    []catalog.Swap
	Options  []OptionChoice
}

// OptionChoice is the choice for one option of one catalog item.
type OptionChoice struct {
	Item   shared.ItemID
	Option shared.OptionID
	Choice shared.ChoiceID
}

// TermsInput is how the order is handed over and paid. Handover is always
// sent. The pointer fields, when nil, take the default (store, kit, no
// freight) on a new order and keep the order's current value on a change;
// a pickup always has no freight. A discount,
// deposit percent or days different from that baseline need order:manage
// (ADR 0030, Amendments).
//
// A delivery names its address with Address or NewAddress, never both.
// NewAddress ("Novo endereço") is added to the customer in the same
// transaction (order:write is enough) and used as the delivery address; a
// quote uses it without saving it. Its ID is ignored.
type TermsInput struct {
	Handover         order.Handover
	Address          shared.AddressID // one of the customer's; delivery needs this or NewAddress
	NewAddress       *order.DeliveryAddress
	FreightCents     *int64
	DiscountCents    *int64
	DepositPercent   *int
	AssemblyDays     *int
	SafetyMarginDays *int
}

// NewCustomer is a customer made inside "Novo pedido": the store's customer
// with that WhatsApp number is used instead, when there is one.
type NewCustomer struct{ Name, Phone string }

// QuoteInput asks what an order would be, without saving it. With Order
// set it quotes a change to that order.
type QuoteInput struct {
	Lines []LineRequest
	Terms TermsInput
	Order shared.OrderID
}

// CreateInput makes an order. Exactly one of Customer and New is set.
type CreateInput struct {
	Customer       shared.CustomerID
	New            *NewCustomer
	Lines          []LineRequest
	Terms          TermsInput
	SendToCustomer bool // "Salvar e enviar ao cliente"
	Idempotency    appshared.IdempotencyKey
}

// ChangeInput replaces an order's lines and terms.
type ChangeInput struct {
	Order            shared.OrderID
	Lines            []LineRequest
	Terms            TermsInput
	CustomerApproved bool
	Version          int
}

// PaymentInput records a payment received by hand.
type PaymentInput struct {
	Order       shared.OrderID
	Kind        order.PaymentKind
	AmountCents int64
	Method      order.PaymentMethod
	Version     int
	Idempotency appshared.IdempotencyKey
}

// VoidPaymentInput voids a wrong payment.
type VoidPaymentInput struct {
	Order   shared.OrderID
	Payment shared.PaymentID
	Reason  string
	Version int
}

// VersionInput names an order at the version the caller read.
type VersionInput struct {
	Order   shared.OrderID
	Version int
}

// AdvanceInput moves an order to another status of its workflow.
type AdvanceInput struct {
	Order    shared.OrderID
	To       workflow.StatusKey
	Handover *order.Handover
	Version  int
}

// MoveItemInput moves one item along its lifecycle.
type MoveItemInput struct {
	Order   shared.OrderID
	Item    shared.OrderItemID
	To      order.ItemStatus
	Check   *order.Check
	Version int
}

// ItemVersionInput names one item of an order at the order's version.
type ItemVersionInput struct {
	Order   shared.OrderID
	Item    shared.OrderItemID
	Version int
}

// DateInput changes the date promised to the client.
type DateInput struct {
	Order   shared.OrderID
	Date    shared.CivilDate
	Reason  string
	Version int
}

// CancelInput cancels an order.
type CancelInput struct {
	Order   shared.OrderID
	Reason  string
	Version int
}

// ListScope says which orders List returns by how far along they are.
type ListScope string

// The scopes of the order list. An unset scope means ScopeActive.
const (
	// ScopeActive is orders neither handed over nor cancelled.
	ScopeActive ListScope = "active"
	// ScopeHandedOver is orders in the handed over phase.
	ScopeHandedOver ListScope = "handed_over"
	// ScopeCancelled is cancelled orders.
	ScopeCancelled ListScope = "cancelled"
	// ScopeAll is every order.
	ScopeAll ListScope = "all"
)

// ListFilter narrows List. Every set field must hold for an order to
// appear. The repository reads the same filter List validated, so an
// adapter never sees an unknown scope or phase.
type ListFilter struct {
	// Status keeps orders whose current status has this key. A cancelled
	// order's status is the workflow's "cancelled" one.
	Status workflow.StatusKey
	// Phase keeps orders whose current status is in this phase.
	Phase workflow.Phase
	// Scope defaults to ScopeActive.
	Scope ListScope
	// Customer keeps one customer's orders.
	Customer shared.CustomerID
	// Query is an order number ("231", "P-0231", see NumberQuery) or the
	// start of the customer's name, ignoring case and surrounding spaces.
	Query string
	// Alert keeps active orders with an alert today. Alerts are computed
	// from each item's dates, so List applies it after the repository
	// reads a page: a page may come back shorter than asked, or empty,
	// with a next cursor.
	Alert bool
}
