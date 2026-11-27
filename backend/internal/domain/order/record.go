package order

import (
	"errors"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// ErrInvalidRecord means a stored order cannot be restored: a bug or a
// broken row, never bad input, so it does not match shared.ErrInvalid and
// never becomes a 400.
var ErrInvalidRecord = errors.New("order: invalid stored order")

// Record is every field of an order, as repositories store it.
type Record struct {
	Tenant          shared.TenantID
	ID              shared.OrderID
	Number          Number
	Customer        shared.CustomerID
	WorkflowID      shared.WorkflowID
	WorkflowVersion int
	StatusKey       workflow.StatusKey // the last real status; a cancelled order keeps the one it left
	Lines           []LineRecord
	Terms           Terms
	Payments        []PaymentRecord

	ReleaseDate          shared.CivilDate
	ReleasedBy           shared.UserID
	Promise              shared.CivilDate
	PromiseChangedByHand bool
	HandedOverAs         Handover
	HandedOverAt         time.Time
	CancelledAt          time.Time
	CancelReason         string

	Tracking  shared.SecretHash
	CreatedAt time.Time
	CreatedBy shared.UserID
	Version   int
}

// LineRecord is a stored line.
type LineRecord struct {
	ID                             shared.OrderLineID
	Kit                            *KitSnapshot
	Price                          shared.Money
	AssemblyDays, SafetyMarginDays *int
	Items                          []ItemRecord
}

// ItemRecord is a stored item. Sourcing comes from the staff-only table.
type ItemRecord struct {
	ID        shared.OrderItemID
	Snapshot  ItemSnapshot
	Sourcing  Sourcing
	Status    ItemStatus
	Check     *Check
	Expected  shared.CivilDate
	ArrivedOn shared.CivilDate
}

// PaymentRecord is a stored payment.
type PaymentRecord struct {
	ID         shared.PaymentID
	Kind       PaymentKind
	Amount     shared.Money
	Method     PaymentMethod
	By         shared.UserID
	At         time.Time
	VoidedBy   shared.UserID
	VoidedAt   time.Time
	VoidReason string
}

// Record returns every field of the order, for the repository to store.
func (o *Order) Record() Record {
	r := Record{
		Tenant: o.tenant, ID: o.id, Number: o.number, Customer: o.customer,
		WorkflowID: o.workflow.ID(), WorkflowVersion: o.workflow.Version(), StatusKey: o.status.Key,
		Terms: cloneTerms(o.terms), ReleaseDate: o.releaseDate, ReleasedBy: o.releasedBy,
		Promise: o.promise, PromiseChangedByHand: o.promiseByHand,
		HandedOverAs: o.handedOverAs, HandedOverAt: o.handedOverAt,
		CancelledAt: o.cancelledAt, CancelReason: o.cancelReason,
		Tracking: o.tracking, CreatedAt: o.createdAt, CreatedBy: o.createdBy, Version: o.version,
	}
	if o.Phase() == workflow.PhaseCancelled {
		r.StatusKey = o.lastStatus().Key
	}
	for _, l := range o.lines {
		lr := LineRecord{ID: l.id, Kit: clonePtr(l.kit), Price: l.price, AssemblyDays: clonePtr(l.assemblyDays), SafetyMarginDays: clonePtr(l.marginDays)}
		for _, i := range l.items {
			lr.Items = append(lr.Items, ItemRecord{
				ID: i.id, Snapshot: cloneSnapshot(i.snapshot), Sourcing: cloneSourcing(i.sourcing),
				Status: i.status, Check: clonePtr(i.check), Expected: i.expected, ArrivedOn: i.arrivedOn,
			})
		}
		r.Lines = append(r.Lines, lr)
	}
	for _, p := range o.payments {
		r.Payments = append(r.Payments, PaymentRecord{p.id, p.kind, p.amount, p.method, p.by, p.at, p.voidedBy, p.voidedAt, p.voidReason})
	}
	return r
}

// lastStatus returns the status a cancelled order left. Cancel records it
// as the From of its history entry, and Restore keeps it from the record.
func (o *Order) lastStatus() workflow.Status {
	if o.cancelledFrom.Key != "" {
		return o.cancelledFrom
	}
	return o.workflow.First()
}

// Restore rebuilds an order from its record and the workflow it was made
// with, which the caller loads by r.WorkflowID and r.WorkflowVersion. It
// returns ErrInvalidRecord when wf is not that workflow, a status or item
// state does not belong to it, or a field is missing.
func Restore(r Record, wf workflow.Workflow) (*Order, error) {
	if wf.ID() != r.WorkflowID || wf.Version() != r.WorkflowVersion {
		return nil, fmt.Errorf("%w: order %v is on workflow %v v%d", ErrInvalidRecord, r.ID, r.WorkflowID, r.WorkflowVersion)
	}
	status, ok := wf.Status(r.StatusKey)
	if !ok || status.Phase == workflow.PhaseCancelled {
		return nil, fmt.Errorf("%w: order %v has status %q", ErrInvalidRecord, r.ID, r.StatusKey)
	}
	if r.ID.IsZero() || r.Tenant.IsZero() || !r.Number.Valid() || r.Version < 1 || len(r.Lines) == 0 {
		return nil, fmt.Errorf("%w: order %v is missing fields", ErrInvalidRecord, r.ID)
	}
	o := &Order{
		tenant: r.Tenant, id: r.ID, number: r.Number, customer: r.Customer, workflow: wf, status: status,
		terms: cloneTerms(r.Terms), releaseDate: r.ReleaseDate, releasedBy: r.ReleasedBy,
		promise: r.Promise, promiseByHand: r.PromiseChangedByHand,
		handedOverAs: r.HandedOverAs, handedOverAt: r.HandedOverAt,
		tracking: r.Tracking, createdAt: r.CreatedAt, createdBy: r.CreatedBy, version: r.Version,
	}
	if !r.CancelledAt.IsZero() {
		o.cancelledFrom, o.status = status, workflow.Cancelled
		o.cancelledAt, o.cancelReason = r.CancelledAt, r.CancelReason
	}
	if err := o.restoreLines(r.Lines); err != nil {
		return nil, err
	}
	for _, p := range r.Payments {
		o.payments = append(o.payments, Payment{p.ID, p.Kind, p.Amount, p.Method, p.By, p.At, p.VoidedBy, p.VoidedAt, p.VoidReason})
	}
	return o, nil
}

func (o *Order) restoreLines(lines []LineRecord) error {
	for _, lr := range lines {
		l := Line{id: lr.ID, kit: clonePtr(lr.Kit), price: lr.Price, assemblyDays: clonePtr(lr.AssemblyDays), marginDays: clonePtr(lr.SafetyMarginDays)}
		for _, ir := range lr.Items {
			if !validItemStatus(ir.Snapshot.SourcingMode, ir.Status) {
				return fmt.Errorf("%w: item %v has status %q", ErrInvalidRecord, ir.ID, ir.Status)
			}
			l.items = append(l.items, Item{
				id: ir.ID, line: lr.ID, snapshot: cloneSnapshot(ir.Snapshot), sourcing: cloneSourcing(ir.Sourcing),
				status: ir.Status, check: clonePtr(ir.Check), expected: ir.Expected, arrivedOn: ir.ArrivedOn,
			})
		}
		o.lines = append(o.lines, l)
	}
	return nil
}
