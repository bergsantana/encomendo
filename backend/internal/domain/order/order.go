package order

import (
	"fmt"
	"slices"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Order is a store's order: its lines and items, its terms, its payments,
// its place in its own status workflow and its dates. Fields are unexported
// and change only through methods, which keep the invariants listed in the
// package comment.
type Order struct {
	tenant   shared.TenantID
	id       shared.OrderID
	number   Number
	customer shared.CustomerID
	workflow workflow.Workflow
	status   workflow.Status
	lines    []Line
	terms    Terms
	payments []Payment

	releaseDate shared.CivilDate // the day the deposit was paid or the order released; zero before
	releasedBy  shared.UserID    // set only for a release without the deposit
	// depositFrozen is the deposit due when it was first confirmed, in
	// cents; nil before. A deposit once confirmed stays confirmed: a later
	// change adds to the balance, not to the deposit (ADR 0030).
	depositFrozen *int64
	promise       shared.CivilDate // delivery_date, the promise to the client
	promiseByHand bool             // changed with ChangeDeliveryDate: deposits no longer reset it

	handedOverAs  Handover
	handedOverAt  time.Time
	cancelledAt   time.Time
	cancelReason  string
	cancelledFrom workflow.Status // the status a cancelled order left

	tracking  shared.SecretHash
	createdAt time.Time
	createdBy shared.UserID
	version   int

	pending []HistoryEntry
}

// NewSpec is what New needs to make an order.
type NewSpec struct {
	Tenant   shared.TenantID
	ID       shared.OrderID
	Number   Number
	Customer shared.CustomerID
	Workflow workflow.Workflow // the store's current workflow; the order keeps this version
	Lines    []LineSpec
	Terms    Terms
	Tracking shared.SecretHash // hash of the order's first tracking token
	By       shared.Principal  // zero for a store page order (M7)
	Now      time.Time
	Today    shared.CivilDate // Now in the store's time zone
}

// New makes an order at its workflow's first status (a draft). Nothing is
// released yet, so every item's expected date and the promise are forecast
// from Today. It returns a *shared.ValidationError listing every field that
// breaks a rule, and shared.ErrInvalid for a missing id, number, customer,
// workflow or tracking hash.
func New(s NewSpec) (*Order, error) {
	if err := s.checkIdentity(); err != nil {
		return nil, err
	}
	currency := s.Terms.Freight.Currency()
	lines, err := buildLines(s.Lines, currency, nil)
	if err != nil {
		return nil, err
	}
	o := &Order{
		tenant: s.Tenant, id: s.ID, number: s.Number, customer: s.Customer,
		workflow: s.Workflow, status: s.Workflow.First(),
		lines: lines, terms: cloneTerms(s.Terms), tracking: s.Tracking,
		createdAt: s.Now, createdBy: s.By.UserID(), version: 1,
	}
	if err := o.terms.check(o.Subtotal()); err != nil {
		return nil, err
	}
	if _, err := o.Total(); err != nil {
		return nil, err
	}
	o.redate(s.Today)
	o.promise = o.forecast(s.Today)
	o.addHistory(HistoryEntry{Kind: HistoryCreated, By: s.By.UserID(), At: s.Now, To: o.status})
	return o, nil
}

func (s NewSpec) checkIdentity() error {
	switch {
	case s.Tenant.IsZero(), s.ID.IsZero(), s.Customer.IsZero():
		return fmt.Errorf("order: tenant, id and customer are required: %w", shared.ErrInvalid)
	case !s.Number.Valid():
		return fmt.Errorf("order: number %d: %w", s.Number, shared.ErrInvalid)
	case s.Workflow.ID().IsZero():
		return fmt.Errorf("order: workflow is required: %w", shared.ErrInvalid)
	case s.Tracking == shared.SecretHash{}:
		return fmt.Errorf("order: tracking hash is required: %w", shared.ErrInvalid)
	case s.Today.IsZero():
		return errMissingToday
	}
	if _, err := shared.ZeroMoney(s.Terms.Freight.Currency()); err != nil {
		return fmt.Errorf("order: currency from the freight: %w", err)
	}
	return nil
}

// TenantID returns the store the order belongs to.
func (o *Order) TenantID() shared.TenantID { return o.tenant }

// ID returns the order's id.
func (o *Order) ID() shared.OrderID { return o.id }

// Number returns the order's number in its store.
func (o *Order) Number() Number { return o.number }

// Customer returns the store customer the order is for.
func (o *Order) Customer() shared.CustomerID { return o.customer }

// Workflow returns the status workflow the order was made with, at its
// version.
func (o *Order) Workflow() workflow.Workflow { return o.workflow }

// Status returns the order's status: one of its workflow's, or
// workflow.Cancelled.
func (o *Order) Status() workflow.Status { return o.status }

// Phase returns the phase of the order's status. Rules branch on it, never
// on the status key or label (ADR 0004).
func (o *Order) Phase() workflow.Phase { return o.status.Phase }

// Lines returns the order's lines, as a new slice.
func (o *Order) Lines() []Line { return slices.Clone(o.lines) }

// Items returns every item of every line, in line order, as a new slice.
func (o *Order) Items() []Item {
	var out []Item
	for _, l := range o.lines {
		out = append(out, l.items...)
	}
	return out
}

// Terms returns the order's terms.
func (o *Order) Terms() Terms { return cloneTerms(o.terms) }

// Payments returns every payment recorded on the order, void ones
// included, as a new slice.
func (o *Order) Payments() []Payment { return slices.Clone(o.payments) }

// ReleaseDate returns the day the deposit was paid or the order released,
// from which dates count, and false before.
func (o *Order) ReleaseDate() (shared.CivilDate, bool) { return o.releaseDate, !o.releaseDate.IsZero() }

// Released returns who released the order without the deposit, and false
// when nobody did.
func (o *Order) Released() (shared.UserID, bool) { return o.releasedBy, !o.releasedBy.IsZero() }

// Promise returns the delivery date promised to the client.
func (o *Order) Promise() shared.CivilDate { return o.promise }

// PromiseChangedByHand reports whether the promise was changed with
// ChangeDeliveryDate, so a deposit or release no longer resets it.
func (o *Order) PromiseChangedByHand() bool { return o.promiseByHand }

// HandedOver returns how and when the order was handed over, and false
// before.
func (o *Order) HandedOver() (Handover, time.Time, bool) {
	return o.handedOverAs, o.handedOverAt, o.handedOverAs != ""
}

// CancelReason returns why the order was cancelled, and false when it was
// not.
func (o *Order) CancelReason() (string, bool) {
	return o.cancelReason, o.Phase() == workflow.PhaseCancelled
}

// TrackingHash returns the hash of the order's current tracking token.
func (o *Order) TrackingHash() shared.SecretHash { return o.tracking }

// CreatedAt returns when the order was made.
func (o *Order) CreatedAt() time.Time { return o.createdAt }

// CreatedBy returns the staff member who made the order; zero for a store
// page order.
func (o *Order) CreatedBy() shared.UserID { return o.createdBy }

// Version returns the order's version, for optimistic locking.
func (o *Order) Version() int { return o.version }

// CheckVersion returns shared.ErrVersionConflict unless expected is the
// order's version.
func (o *Order) CheckVersion(expected int) error {
	if expected != o.version {
		return shared.ErrVersionConflict
	}
	return nil
}

// Closed reports whether the order was handed over or cancelled.
func (o *Order) Closed() bool {
	p := o.Phase()
	return p == workflow.PhaseHandedOver || p == workflow.PhaseCancelled
}

// Editable says what a change to the order needs now: EditFree before any
// payment, EditApproval once paid (order:manage and the customer's
// approval), EditClosed from assembling on.
func (o *Order) Editable() Editability {
	switch {
	case o.Closed(), o.pastEditing():
		return EditClosed
	case o.paid():
		return EditApproval
	default:
		return EditFree
	}
}

// Editability is what changing an order needs. The values are part of the
// API contract.
type Editability string

// The editabilities.
const (
	EditFree     Editability = "free"
	EditApproval Editability = "approval"
	EditClosed   Editability = "closed"
)

// pastEditing reports whether the order entered assembling, or ready in a
// workflow without assembling (ADR 0030: changes stop before Montagem).
func (o *Order) pastEditing() bool {
	p := o.Phase()
	return p == workflow.PhaseAssembling || p == workflow.PhaseReady
}

// paid reports whether a change needs the customer's approval: the order
// has a payment that is not void, was released, or is past
// awaiting_deposit (ADR 0030).
func (o *Order) paid() bool {
	if !o.Paid().IsZero() || !o.releaseDate.IsZero() {
		return true
	}
	return o.Phase().Rank() > workflow.PhaseAwaitingDeposit.Rank()
}

// addHistory appends a history entry for the repository to write.
func (o *Order) addHistory(e HistoryEntry) { o.pending = append(o.pending, e) }

// PendingHistory returns the history entries added since New or Restore,
// in order. The repository writes them with the order.
func (o *Order) PendingHistory() []HistoryEntry { return slices.Clone(o.pending) }

func cloneTerms(t Terms) Terms {
	t.Address = clonePtr(t.Address)
	return t
}
