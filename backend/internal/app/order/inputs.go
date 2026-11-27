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

// TermsInput is how the order is handed over and paid. On a new order a
// nil field takes the default (store, kit); on a change it keeps the
// order's current value. A discount, deposit percent or days different from
// that baseline need order:manage (ADR 0030, Amendments).
type TermsInput struct {
	Handover         order.Handover
	Address          shared.AddressID // one of the customer's; required for delivery
	FreightCents     int64
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
