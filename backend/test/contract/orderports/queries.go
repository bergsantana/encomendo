package orderports

import (
	"context"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

var activeCases = []contractCase{
	{"lists open orders by delivery date with when they entered their status", activeListsOpenOrders},
	{"stops at the limit", activeStopsAtLimit},
}

// activeSet is three open orders, one moved to another status, with a
// cancelled and a handed over order that must stay out.
type activeSet struct {
	s                    store
	first, second, third *order.Order
	movedAt              time.Time
}

func seedActive(t *testing.T, h Harness) activeSet {
	t.Helper()
	s := h.newStore(t)
	a := activeSet{s: s, movedAt: epoch.Add(3 * time.Hour)}
	a.third = h.insert(t, s, withLead(9))
	a.first = h.insert(t, s, withLead(1))
	a.second = h.insert(t, s, withLead(5))
	h.insertAs(t, s, func(r *order.Record) { r.CancelledAt, r.CancelReason = epoch.Add(time.Hour), "Desistiu" }, withLead(2))
	h.insertAs(t, s, handOver(h, epoch.Add(time.Hour)), withLead(3))
	moved := h.load(t, s, a.second.ID())
	awaiting := h.status(t, workflow.PhaseAwaitingDeposit).Key
	if err := moved.Advance(awaiting, nil, s.principal(t), today(a.movedAt), a.movedAt); err != nil {
		t.Fatal(err)
	}
	h.mustUpdate(t, s, moved)
	return a
}

func (h Harness) active(t *testing.T, s store, limit int) []apporder.ActiveOrder {
	t.Helper()
	var out []apporder.ActiveOrder
	h.in(t, s, func(ctx context.Context) error {
		var err error
		out, err = h.Orders.Active(ctx, s.id, limit)
		return err
	})
	return out
}

func activeListsOpenOrders(t *testing.T, h Harness) {
	a := seedActive(t, h)
	got := h.active(t, a.s, 10)
	want := []*order.Order{a.first, a.second, a.third}
	if len(got) != len(want) {
		t.Fatalf("active has %d orders, want %d", len(got), len(want))
	}
	for i, o := range want {
		if got[i].Record.ID != o.ID() {
			t.Errorf("active[%d] = %v, want %v", i, got[i].Record.ID, o.ID())
		}
		since := o.Record().CreatedAt
		if o.ID() == a.second.ID() {
			since = a.movedAt
		}
		if !got[i].StatusSince.Equal(since) {
			t.Errorf("active[%d] since %v, want %v", i, got[i].StatusSince, since)
		}
		sameRecord(t, got[i].Record, h.record(t, a.s, o.ID()))
	}
}

func activeStopsAtLimit(t *testing.T, h Harness) {
	a := seedActive(t, h)
	got := h.active(t, a.s, 2)
	if len(got) != 2 || got[0].Record.ID != a.first.ID() || got[1].Record.ID != a.second.ID() {
		t.Errorf("active(2) = %d orders, want the first two by delivery date", len(got))
	}
}

var paymentCases = []contractCase{
	{"covers the half-open period, oldest first, voided ones marked", paymentsPeriod},
	{"includes the start and leaves out the end", paymentsBounds},
	{"another store sees none of them", paymentsOtherStore},
}

// paymentSet is two orders with payments at four times, one voided.
type paymentSet struct {
	s              store
	a, b           *order.Order
	t0, t1, t2, t3 time.Time
}

func seedPayments(t *testing.T, h Harness) paymentSet {
	t.Helper()
	s := h.newStore(t)
	p := paymentSet{s: s, t0: epoch, t1: epoch.Add(time.Hour), t2: epoch.Add(2 * time.Hour), t3: epoch.Add(3 * time.Hour)}
	pay := func(kind order.PaymentKind, cents int64, at time.Time, void bool) order.PaymentRecord {
		r := order.PaymentRecord{ID: shared.PaymentID(h.NewID()), Kind: kind, Amount: brl(t, cents), Method: order.MethodPix, By: s.user, At: at}
		if void {
			r.VoidedBy, r.VoidedAt, r.VoidReason = s.user, at.Add(time.Minute), "Lançado errado"
		}
		return r
	}
	p.a = h.insertAs(t, s, func(r *order.Record) {
		r.Payments = []order.PaymentRecord{pay(order.PaymentDeposit, 1000, p.t0, false), pay(order.PaymentBalance, 2000, p.t2, true)}
	})
	p.b = h.insertAs(t, s, func(r *order.Record) {
		r.Payments = []order.PaymentRecord{pay(order.PaymentDeposit, 3000, p.t1, false), pay(order.PaymentBalance, 4000, p.t3, false)}
	})
	return p
}

func (h Harness) payments(t *testing.T, s store, from, to time.Time) []apporder.PaymentRow {
	t.Helper()
	var rows []apporder.PaymentRow
	h.in(t, s, func(ctx context.Context) error {
		var err error
		rows, err = h.Orders.Payments(ctx, s.id, from, to)
		return err
	})
	return rows
}

func paymentsPeriod(t *testing.T, h Harness) {
	p := seedPayments(t, h)
	rows := h.payments(t, p.s, p.t0, p.t3)
	type want struct {
		order  shared.OrderID
		kind   order.PaymentKind
		cents  int64
		at     time.Time
		voided bool
	}
	exp := []want{
		{p.a.ID(), order.PaymentDeposit, 1000, p.t0, false},
		{p.b.ID(), order.PaymentDeposit, 3000, p.t1, false},
		{p.a.ID(), order.PaymentBalance, 2000, p.t2, true},
	}
	if len(rows) != len(exp) {
		t.Fatalf("payments = %+v, want %d rows", rows, len(exp))
	}
	for i, e := range exp {
		r := rows[i]
		if r.Order != e.order || r.Kind != e.kind || r.Amount.Cents() != e.cents || !r.At.Equal(e.at) || r.Voided != e.voided || r.Method != order.MethodPix {
			t.Errorf("row %d = %+v, want %+v", i, r, e)
		}
	}
}

func paymentsBounds(t *testing.T, h Harness) {
	p := seedPayments(t, h)
	if rows := h.payments(t, p.s, p.t3, p.t3.Add(time.Hour)); len(rows) != 1 || rows[0].Amount.Cents() != 4000 {
		t.Errorf("payments from the last instant = %+v, want the one at it", rows)
	}
	if rows := h.payments(t, p.s, p.t0, p.t0); len(rows) != 0 {
		t.Errorf("an empty period has %d payments", len(rows))
	}
}

func paymentsOtherStore(t *testing.T, h Harness) {
	p := seedPayments(t, h)
	other := h.newStore(t)
	if rows := h.payments(t, other, p.t0, p.t3.Add(time.Hour)); len(rows) != 0 {
		t.Errorf("other store reads %d payments", len(rows))
	}
}

var handedOverCases = []contractCase{
	{"returns whole records handed over in the half-open period, oldest first", handedOverPeriod},
	{"another store sees none of them", handedOverOtherStore},
}

// handedOverSet is three handed over orders, a cancelled one and an open one.
type handedOverSet struct {
	s                store
	early, mid, late *order.Order
	t1, t3           time.Time
}

func seedHandedOver(t *testing.T, h Harness) handedOverSet {
	t.Helper()
	s := h.newStore(t)
	d := handedOverSet{s: s, t1: epoch.Add(time.Hour), t3: epoch.Add(3 * time.Hour)}
	mk := func(at time.Time, opts ...buildOpt) *order.Order {
		return h.insertAs(t, s, func(r *order.Record) {
			handOver(h, at)(r)
			r.Payments = []order.PaymentRecord{{ID: shared.PaymentID(h.NewID()), Kind: order.PaymentDeposit, Amount: brl(t, 5000), Method: order.MethodCash, At: at}}
		}, opts...)
	}
	d.late = mk(d.t3, rich())
	d.early = mk(d.t1)
	d.mid = mk(epoch.Add(2*time.Hour), rich())
	h.insert(t, s)
	h.insertAs(t, s, func(r *order.Record) { r.CancelledAt, r.CancelReason = d.t1, "Desistiu" })
	return d
}

func (h Harness) handedOver(t *testing.T, s store, from, to time.Time) []order.Record {
	t.Helper()
	var out []order.Record
	h.in(t, s, func(ctx context.Context) error {
		var err error
		out, err = h.Orders.HandedOver(ctx, s.id, from, to)
		return err
	})
	return out
}

func handedOverPeriod(t *testing.T, h Harness) {
	d := seedHandedOver(t, h)
	got := h.handedOver(t, d.s, d.t1, d.t3)
	if len(got) != 2 {
		t.Fatalf("handed over = %d orders, want 2", len(got))
	}
	sameRecord(t, got[0], d.early.Record())
	sameRecord(t, got[1], d.mid.Record())
	// Postgres keeps microseconds, so one more is the smallest step past d.t3.
	all := h.handedOver(t, d.s, d.t1, d.t3.Add(time.Microsecond))
	if len(all) != 3 {
		t.Fatalf("a period reaching past the last has %d orders, want 3", len(all))
	}
	sameRecord(t, all[2], d.late.Record())
}

func handedOverOtherStore(t *testing.T, h Harness) {
	d := seedHandedOver(t, h)
	other := h.newStore(t)
	if got := h.handedOver(t, other, d.t1, d.t3.Add(time.Hour)); len(got) != 0 {
		t.Errorf("other store reads %d orders", len(got))
	}
}
