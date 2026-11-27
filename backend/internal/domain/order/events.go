package order

import (
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Events of the order context. The outbox stores each under the store of
// the transaction that published it. They carry ids, statuses, phases and
// dates, never a customer's name, phone or address, nor a tracking token.
// The names are stable; consumers select on them.

// Created is published when an order is made.
type Created struct {
	Order    shared.OrderID    `json:"order"`
	Number   Number            `json:"number"`
	Workflow shared.WorkflowID `json:"workflow"`
	Version  int               `json:"version"`
}

// EventName implements appshared.Event.
func (Created) EventName() string { return "order.created" }

// Advanced is published when an order moves to another status.
type Advanced struct {
	Order shared.OrderID     `json:"order"`
	From  workflow.StatusKey `json:"from"`
	To    workflow.StatusKey `json:"to"`
	Phase workflow.Phase     `json:"phase"`
}

// EventName implements appshared.Event.
func (Advanced) EventName() string { return "order.advanced" }

// DepositConfirmed is published when the deposit payments first reach the
// deposit due. Milestone 5 generates purchases from it.
type DepositConfirmed struct {
	Order       shared.OrderID   `json:"order"`
	ReleaseDate shared.CivilDate `json:"releaseDate"`
}

// EventName implements appshared.Event.
func (DepositConfirmed) EventName() string { return "order.deposit_confirmed" }

// Released is published when the owner releases an order without the
// deposit.
type Released struct {
	Order       shared.OrderID   `json:"order"`
	ReleaseDate shared.CivilDate `json:"releaseDate"`
}

// EventName implements appshared.Event.
func (Released) EventName() string { return "order.released" }

// PaymentRecorded is published for each payment recorded.
type PaymentRecorded struct {
	Order   shared.OrderID   `json:"order"`
	Payment shared.PaymentID `json:"payment"`
	Kind    PaymentKind      `json:"kind"`
}

// EventName implements appshared.Event.
func (PaymentRecorded) EventName() string { return "order.payment_recorded" }

// PaymentVoided is published when a payment is voided.
type PaymentVoided struct {
	Order   shared.OrderID   `json:"order"`
	Payment shared.PaymentID `json:"payment"`
}

// EventName implements appshared.Event.
func (PaymentVoided) EventName() string { return "order.payment_voided" }

// ItemMoved is published when an item moves along its lifecycle.
type ItemMoved struct {
	Order shared.OrderID     `json:"order"`
	Item  shared.OrderItemID `json:"item"`
	To    ItemStatus         `json:"to"`
}

// EventName implements appshared.Event.
func (ItemMoved) EventName() string { return "order.item_moved" }

// Changed is published when an order's lines or terms change.
type Changed struct {
	Order   shared.OrderID       `json:"order"`
	Added   []shared.OrderItemID `json:"added"`
	Removed []shared.OrderItemID `json:"removed"`
}

// EventName implements appshared.Event.
func (Changed) EventName() string { return "order.changed" }

// DateChanged is published when someone changes the promised date.
type DateChanged struct {
	Order shared.OrderID   `json:"order"`
	From  shared.CivilDate `json:"from"`
	To    shared.CivilDate `json:"to"`
}

// EventName implements appshared.Event.
func (DateChanged) EventName() string { return "order.date_changed" }

// Cancelled is published when an order is cancelled. Milestones 5 and 6
// release its purchases and reservations from it.
type Cancelled struct {
	Order shared.OrderID `json:"order"`
}

// EventName implements appshared.Event.
func (Cancelled) EventName() string { return "order.cancelled" }
