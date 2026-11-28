package orderports

import (
	"context"
	"errors"
	"testing"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
)

// isolationCases are the writes that must never reach another store's
// order, or another order's rows (security review of 4.12).
var isolationCases = []contractCase{
	{"another store's update changes nothing", updateFromAnotherStore},
	{"a line id held by another order is refused", lineOfAnotherOrder},
	{"a line id held by another store's order is refused", lineOfAnotherStoresOrder},
	{"a cursor the repository did not issue is refused", foreignCursors},
	{"a filter on another store's customer or name finds nothing", filtersStayInStore},
}

// updateFromAnotherStore: store B's scope updates store A's stored order.
// The update fails and A's order reads back unchanged.
func updateFromAnotherStore(t *testing.T, h Harness) {
	a, b := h.newStore(t), h.newStore(t)
	o := h.insert(t, a, rich())
	before := h.record(t, a, o.ID())
	loaded := h.load(t, a, o.ID())
	if err := h.update(b, loaded); err == nil {
		t.Fatal("store B updated store A's order")
	}
	sameRecord(t, h.record(t, a, o.ID()), before)
}

// insertWithLineOf inserts into s a new order whose first line takes the id
// of other's first line, and returns the insert's error.
func (h Harness) insertWithLineOf(t *testing.T, s store, other *order.Order) error {
	t.Helper()
	return h.try(s.scope(), func(ctx context.Context) error {
		n, err := h.Orders.NextNumber(ctx, s.id)
		if err != nil {
			return err
		}
		r := h.newOrder(t, s, n).Record()
		r.Lines[0].ID = other.Lines()[0].ID()
		o, err := order.Restore(r, h.Workflow)
		if err != nil {
			return err
		}
		return h.Orders.Insert(ctx, o)
	})
}

func lineOfAnotherOrder(t *testing.T, h Harness) {
	s := h.newStore(t)
	first := h.insert(t, s)
	before := h.record(t, s, first.ID())
	if err := h.insertWithLineOf(t, s, first); err == nil {
		t.Fatal("an order took another order's line id")
	}
	sameRecord(t, h.record(t, s, first.ID()), before)
}

func lineOfAnotherStoresOrder(t *testing.T, h Harness) {
	a, b := h.newStore(t), h.newStore(t)
	first := h.insert(t, a)
	before := h.record(t, a, first.ID())
	if err := h.insertWithLineOf(t, b, first); err == nil {
		t.Fatal("store B's order took store A's line id")
	}
	sameRecord(t, h.record(t, a, first.ID()), before)
}

// foreignCursors: List and History refuse a cursor they did not issue with
// shared.ErrInvalid, so a crafted cursor neither reads past a page nor
// passes silently.
func foreignCursors(t *testing.T, h Harness) {
	s := h.newStore(t)
	o := h.insert(t, s)
	bad := page(t, "zzz", 10)
	err := h.try(s.scope(), func(ctx context.Context) error {
		_, err := h.Orders.List(ctx, s.id, apporder.ListFilter{Scope: apporder.ScopeAll}, bad)
		return err
	})
	if !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("List with a foreign cursor: err = %v, want shared.ErrInvalid", err)
	}
	err = h.try(s.scope(), func(ctx context.Context) error {
		_, err := h.Orders.History(ctx, s.id, o.ID(), bad)
		return err
	})
	if !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("History with a foreign cursor: err = %v, want shared.ErrInvalid", err)
	}
}

// filtersStayInStore: store A's customer and order are findable by customer id
// and by name in A, and store B, filtering by that customer id or that name,
// gets nothing, and naming A's store from B's scope reads nothing either. A control in A keeps the empty answers from being an empty
// fixture.
func filtersStayInStore(t *testing.T, h Harness) {
	a, b := h.newStore(t), h.newStore(t)
	zelda := h.NewCustomer(t, a.id, "Zelda Unica")
	mine := h.insert(t, a, withCustomer(zelda))
	h.insert(t, b)
	filters := []struct {
		name   string
		filter apporder.ListFilter
	}{
		{"the customer id", apporder.ListFilter{Scope: apporder.ScopeAll, Customer: zelda}},
		{"the customer id and a query", apporder.ListFilter{Scope: apporder.ScopeAll, Customer: zelda, Query: "Zelda"}},
		{"the name", apporder.ListFilter{Scope: apporder.ScopeAll, Query: "Zelda"}},
		{"the name in capitals", apporder.ListFilter{Scope: apporder.ScopeAll, Query: "ZELDA UNICA"}},
		{"the number of A's order", apporder.ListFilter{Scope: apporder.ScopeAll, Query: mine.Number().String(), Customer: zelda}},
	}
	h.in(t, a, func(ctx context.Context) error {
		for _, f := range filters {
			got, err := h.Orders.List(ctx, a.id, f.filter, page(t, "", 10))
			if err != nil || len(got.Items) != 1 || got.Items[0].ID != mine.ID() {
				t.Errorf("A filtering by %s = %v, %v; want its own order", f.name, got.Items, err)
			}
		}
		return nil
	})
	h.in(t, b, func(ctx context.Context) error {
		for _, f := range filters {
			got, err := h.Orders.List(ctx, b.id, f.filter, page(t, "", 10))
			if err != nil || len(got.Items) != 0 {
				t.Errorf("B filtering by %s = %v, %v; want nothing", f.name, got.Items, err)
			}
		}
		return nil
	})
	// Naming A's store from B's scope is refused or empty, never A's order:
	// the fake refuses, Postgres returns no rows under RLS. A transaction the
	// refusal rolls back is expected, so each call has its own.
	for _, f := range filters {
		// The refusal itself is the fake's way to say "nothing", so only rows fail the case.
		_ = h.try(b.scope(), func(ctx context.Context) error {
			got, err := h.Orders.List(ctx, a.id, f.filter, page(t, "", 10))
			if err == nil && len(got.Items) != 0 {
				t.Errorf("B naming A's store, filtering by %s, read %v", f.name, got.Items)
			}
			return err
		})
	}
}
