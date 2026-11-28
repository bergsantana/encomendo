package orderports

import (
	"context"
	"errors"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

var roundTripCases = []contractCase{
	{"a plain order reads back whole", roundTripPlain},
	{"kit snapshot, options, sourcing, backup, delivery address and freight survive", roundTripRich},
	{"a released order with payments, a void, an arrived item and a frozen deposit", roundTripReleased},
	{"a cancelled order keeps the status it left", roundTripCancelled},
	{"a handed over order", roundTripHandedOver},
	{"an order with no user, as a store page order is", roundTripAnonymous},
	{"an unknown order is not found", byIDUnknown},
	{"another store's order is not found", byIDOtherStore},
}

func roundTripPlain(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s)
	sameRecord(t, h.record(t, s, o.ID()), o.Record())
}

func roundTripRich(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s, rich())
	got := h.record(t, s, o.ID())
	sameRecord(t, got, o.Record())
	if got.Terms.Address == nil || got.Lines[0].Kit == nil || len(got.Lines[0].Kit.Swaps) != 1 {
		t.Errorf("address or kit snapshot lost: %+v", got)
	}
}

func roundTripReleased(t *testing.T, h Harness) {
	s := h.newStore(t)
	deposit := int64(39000)
	o := h.insertAs(t, s, func(r *order.Record) {
		r.ReleaseDate, r.ReleasedBy, r.DepositConfirmedCents = date(t, "2026-12-01"), s.user, &deposit
		r.PromiseChangedByHand = true
		r.StatusKey = h.status(t, workflow.PhaseSourcing).Key
		check := order.CheckOK
		item := &r.Lines[0].Items[0]
		item.Status, item.Check, item.ArrivedOn = order.ItemReceived, &check, date(t, "2026-12-03")
		r.Payments = []order.PaymentRecord{
			{ID: shared.PaymentID(h.NewID()), Kind: order.PaymentDeposit, Amount: brl(t, 39000), Method: order.MethodPix, By: s.user, At: epoch.Add(time.Hour)},
			{
				ID: shared.PaymentID(h.NewID()), Kind: order.PaymentBalance, Amount: brl(t, 500), Method: order.MethodCash,
				At: epoch.Add(2 * time.Hour), VoidedBy: s.user, VoidedAt: epoch.Add(3 * time.Hour), VoidReason: "Lançado errado",
			},
		}
	}, rich())
	sameRecord(t, h.record(t, s, o.ID()), o.Record())
}

func roundTripCancelled(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insertAs(t, s, func(r *order.Record) {
		r.CancelledAt, r.CancelReason = epoch.Add(time.Hour), "Desistiu"
	})
	got := h.record(t, s, o.ID())
	sameRecord(t, got, o.Record())
	if got.StatusKey != h.Workflow.First().Key {
		t.Errorf("status key = %q, want the one the order left, %q", got.StatusKey, h.Workflow.First().Key)
	}
}

func roundTripHandedOver(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insertAs(t, s, handOver(h, epoch.Add(time.Hour)))
	sameRecord(t, h.record(t, s, o.ID()), o.Record())
}

func roundTripAnonymous(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s, anonymous())
	got := h.record(t, s, o.ID())
	sameRecord(t, got, o.Record())
	if !got.CreatedBy.IsZero() {
		t.Errorf("created by = %v, want none", got.CreatedBy)
	}
}

func (h Harness) byIDErr(s store, id shared.OrderID) error {
	return h.try(s.scope(), func(ctx context.Context) error {
		_, err := h.Orders.ByID(ctx, s.id, id)
		return err
	})
}

func byIDUnknown(t *testing.T, h Harness) {
	s := h.newStore(t)
	err := h.byIDErr(s, shared.OrderID(h.NewID()))
	if !errors.Is(err, apporder.ErrNotFound) || !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("ByID err = %v, want apporder.ErrNotFound", err)
	}
}

func byIDOtherStore(t *testing.T, h Harness) {
	s, other := h.newStore(t), h.newStore(t)
	o := h.insert(t, s)
	if err := h.byIDErr(other, o.ID()); !errors.Is(err, apporder.ErrNotFound) {
		t.Errorf("ByID in another store err = %v, want apporder.ErrNotFound", err)
	}
}

// handOver returns a change that makes the order handed over at at, on the
// workflow's last status.
func handOver(h Harness, at time.Time) func(*order.Record) {
	statuses := h.Workflow.Statuses()
	return func(r *order.Record) {
		r.StatusKey = statuses[len(statuses)-1].Key
		r.HandedOverAs, r.HandedOverAt = order.HandoverPickup, at
	}
}
