package orderports

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// listSet is the orders the List cases filter, stored in this order and
// numbered 1 to 6.
type listSet struct {
	s                                  store
	ana, bruno, fibra                  shared.CustomerID
	draftA, draftB, awaiting, sourcing *order.Order
	cancelled, delivered               *order.Order
	byID                               map[shared.OrderID]order.Record
	awaitingKey, sourcingKey, draftKey workflow.StatusKey
}

func seedList(t *testing.T, h Harness) listSet {
	t.Helper()
	s := h.newStore(t)
	l := listSet{s: s, byID: map[shared.OrderID]order.Record{}}
	l.ana, l.bruno = h.NewCustomer(t, s.id, "Ana Lima"), h.NewCustomer(t, s.id, "Bruno Dias")
	l.fibra = h.NewCustomer(t, s.id, "100% Fibra_Nova")
	l.awaitingKey, l.sourcingKey = h.status(t, workflow.PhaseAwaitingDeposit).Key, h.status(t, workflow.PhaseSourcing).Key
	l.draftKey = h.Workflow.First().Key
	add := func(change func(*order.Record), opts ...buildOpt) *order.Order {
		o := h.insertAs(t, s, change, opts...)
		l.byID[o.ID()] = o.Record()
		return o
	}
	l.draftA = add(nil, withCustomer(l.ana), withLead(3))
	l.draftB = add(nil, withCustomer(l.ana), withLead(3))
	l.awaiting = add(func(r *order.Record) { r.StatusKey = l.awaitingKey }, withCustomer(l.bruno), withLead(8))
	l.sourcing = add(func(r *order.Record) { r.StatusKey = l.sourcingKey }, withCustomer(l.bruno), withLead(12))
	l.cancelled = add(func(r *order.Record) {
		r.CancelledAt, r.CancelReason = epoch.Add(time.Hour), "Desistiu"
	}, withCustomer(l.fibra), withLead(1))
	l.delivered = add(handOver(h, epoch.Add(2*time.Hour)), withCustomer(l.ana), withLead(20))
	return l
}

// want returns the ids of the stored orders that pass keep, by delivery date
// then id: the order List promises.
func (l listSet) want(keep func(order.Record) bool) []shared.OrderID {
	var recs []order.Record
	for _, r := range l.byID {
		if keep(r) {
			recs = append(recs, r)
		}
	}
	slices.SortFunc(recs, func(a, b order.Record) int {
		if c := strings.Compare(a.Promise.String(), b.Promise.String()); c != 0 {
			return c
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	out := make([]shared.OrderID, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.ID)
	}
	return out
}

func (l listSet) page(t *testing.T, h Harness, f apporder.ListFilter, cursor string, limit int) shared.Page[order.Record] {
	t.Helper()
	var p shared.Page[order.Record]
	h.in(t, l.s, func(ctx context.Context) error {
		var err error
		p, err = h.Orders.List(ctx, l.s.id, f, page(t, cursor, limit))
		return err
	})
	return p
}

func (l listSet) ids(t *testing.T, h Harness, f apporder.ListFilter) []shared.OrderID {
	t.Helper()
	var out []shared.OrderID
	for _, r := range l.page(t, h, f, "", shared.MaxPageLimit).Items {
		out = append(out, r.ID)
	}
	return out
}

// listFilterCase is one filter and the orders of the set it must keep.
type listFilterCase struct {
	name   string
	filter func(l listSet) apporder.ListFilter
	keep   func(l listSet) func(order.Record) bool
}

func keepAll(listSet) func(order.Record) bool { return func(order.Record) bool { return true } }
func keepNone(listSet) func(order.Record) bool {
	return func(order.Record) bool { return false }
}

func keepWhere(f func(order.Record) bool) func(listSet) func(order.Record) bool {
	return func(listSet) func(order.Record) bool { return f }
}

func isOpen(r order.Record) bool      { return r.CancelledAt.IsZero() && r.HandedOverAt.IsZero() }
func isCancelled(r order.Record) bool { return !r.CancelledAt.IsZero() }
func isHandedOver(r order.Record) bool {
	return !r.HandedOverAt.IsZero()
}

func scopeOf(s apporder.ListScope) func(listSet) apporder.ListFilter {
	return func(listSet) apporder.ListFilter { return apporder.ListFilter{Scope: s} }
}

var scopeFilters = []listFilterCase{
	{"active", scopeOf(apporder.ScopeActive), keepWhere(isOpen)},
	{"handed over", scopeOf(apporder.ScopeHandedOver), keepWhere(isHandedOver)},
	{"cancelled", scopeOf(apporder.ScopeCancelled), keepWhere(isCancelled)},
	{"all", scopeOf(apporder.ScopeAll), keepAll},
	{
		"a customer's orders",
		func(l listSet) apporder.ListFilter {
			return apporder.ListFilter{Scope: apporder.ScopeAll, Customer: l.ana}
		},
		func(l listSet) func(order.Record) bool {
			return func(r order.Record) bool { return r.Customer == l.ana }
		},
	},
}

// A cancelled order keeps the status key it left, yet its status is the
// implicit "cancelled": a filter by the key it holds must not find it.
var statusFilters = []listFilterCase{
	{
		"a status key",
		func(l listSet) apporder.ListFilter {
			return apporder.ListFilter{Scope: apporder.ScopeActive, Status: l.awaitingKey}
		},
		func(l listSet) func(order.Record) bool {
			return func(r order.Record) bool { return r.ID == l.awaiting.ID() }
		},
	},
	{
		"the draft key leaves out a cancelled order that holds it",
		func(l listSet) apporder.ListFilter {
			return apporder.ListFilter{Scope: apporder.ScopeAll, Status: l.draftKey}
		},
		func(l listSet) func(order.Record) bool {
			return func(r order.Record) bool { return r.StatusKey == l.draftKey && r.CancelledAt.IsZero() }
		},
	},
	{
		"the cancelled status",
		func(listSet) apporder.ListFilter {
			return apporder.ListFilter{Scope: apporder.ScopeAll, Status: workflow.Cancelled.Key}
		},
		keepWhere(isCancelled),
	},
	{
		"a phase",
		func(listSet) apporder.ListFilter {
			return apporder.ListFilter{Scope: apporder.ScopeActive, Phase: workflow.PhaseSourcing}
		},
		func(l listSet) func(order.Record) bool {
			return func(r order.Record) bool { return r.ID == l.sourcing.ID() }
		},
	},
	{
		"the draft phase leaves out a cancelled order",
		func(listSet) apporder.ListFilter {
			return apporder.ListFilter{Scope: apporder.ScopeAll, Phase: workflow.PhaseDraft}
		},
		func(l listSet) func(order.Record) bool {
			return func(r order.Record) bool { return r.StatusKey == l.draftKey && r.CancelledAt.IsZero() }
		},
	},
	{
		"the cancelled phase",
		func(listSet) apporder.ListFilter {
			return apporder.ListFilter{Scope: apporder.ScopeAll, Phase: workflow.PhaseCancelled}
		},
		keepWhere(isCancelled),
	},
	{
		"the handed over phase",
		func(listSet) apporder.ListFilter {
			return apporder.ListFilter{Scope: apporder.ScopeAll, Phase: workflow.PhaseHandedOver}
		},
		keepWhere(isHandedOver),
	},
}

func queryOf(q string) func(listSet) apporder.ListFilter {
	return func(listSet) apporder.ListFilter { return apporder.ListFilter{Scope: apporder.ScopeAll, Query: q} }
}

func keepNumber(n order.Number) func(listSet) func(order.Record) bool {
	return keepWhere(func(r order.Record) bool { return r.Number == n })
}

func keepCustomer(pick func(listSet) shared.CustomerID) func(listSet) func(order.Record) bool {
	return func(l listSet) func(order.Record) bool {
		id := pick(l)
		return func(r order.Record) bool { return r.Customer == id }
	}
}

var queryFilters = []listFilterCase{
	{"a number", queryOf("2"), keepNumber(2)},
	{"a number with its prefix, in either case", queryOf("p-0003"), keepNumber(3)},
	{"a name prefix, ignoring case", queryOf("aNa"), keepCustomer(func(l listSet) shared.CustomerID { return l.ana })},
	{"a percent in a name is taken literally", queryOf("100%"), keepCustomer(func(l listSet) shared.CustomerID { return l.fibra })},
	{"an underscore in a name is taken literally", queryOf("100% Fibra_"), keepCustomer(func(l listSet) shared.CustomerID { return l.fibra })},
	{"a lone percent matches no name", queryOf("%"), keepNone},
	{"a lone underscore matches no name", queryOf("_"), keepNone},
	{"a name from the middle matches nothing", queryOf("Lima"), keepNone},
	{
		"filters combine",
		func(l listSet) apporder.ListFilter {
			return apporder.ListFilter{Scope: apporder.ScopeAll, Customer: l.ana, Phase: workflow.PhaseDraft}
		},
		func(l listSet) func(order.Record) bool {
			return func(r order.Record) bool { return r.Customer == l.ana && isOpen(r) }
		},
	},
}

func filterCases(filters []listFilterCase) []contractCase {
	out := make([]contractCase, 0, len(filters))
	for _, fc := range filters {
		out = append(out, contractCase{fc.name, func(t *testing.T, h Harness) {
			l := seedList(t, h)
			want := l.want(fc.keep(l))
			if got := l.ids(t, h, fc.filter(l)); !slices.Equal(got, want) {
				t.Errorf("ids = %v, want %v", got, want)
			}
		}})
	}
	return out
}

var listCases = slices.Concat(
	filterCases(scopeFilters), filterCases(statusFilters), filterCases(queryFilters),
	[]contractCase{
		{"returns whole records", listWholeRecords},
		{"pages by delivery date and id without gaps or repeats", listPages},
		{"another store sees none of it", listOtherStore},
	},
)

func listWholeRecords(t *testing.T, h Harness) {
	l := seedList(t, h)
	got := l.page(t, h, apporder.ListFilter{Scope: apporder.ScopeAll}, "", shared.MaxPageLimit)
	if len(got.Items) != len(l.byID) {
		t.Fatalf("list returned %d orders, want %d", len(got.Items), len(l.byID))
	}
	for _, r := range got.Items {
		sameRecord(t, r, l.byID[r.ID])
	}
}

func listPages(t *testing.T, h Harness) {
	l := seedList(t, h)
	f := apporder.ListFilter{Scope: apporder.ScopeAll}
	want := l.want(keepAll(l))
	for _, limit := range []int{1, 2, 3, 6, 7} {
		var got []shared.OrderID
		cursor, pages := "", 0
		for pages <= len(want) {
			p := l.page(t, h, f, cursor, limit)
			for _, r := range p.Items {
				got = append(got, r.ID)
			}
			pages++
			if cursor = p.NextCursor; cursor == "" {
				break
			}
			if len(p.Items) != limit {
				t.Fatalf("limit %d: page %d has %d orders and a next cursor", limit, pages, len(p.Items))
			}
		}
		if wantPages := (len(want) + limit - 1) / limit; !slices.Equal(got, want) || pages != wantPages {
			t.Errorf("limit %d: %d pages of %v, want %d pages of %v", limit, pages, got, wantPages, want)
		}
	}
}

func listOtherStore(t *testing.T, h Harness) {
	seedList(t, h)
	other := listSet{s: h.newStore(t)}
	if got := other.ids(t, h, apporder.ListFilter{Scope: apporder.ScopeAll}); len(got) != 0 {
		t.Errorf("other store lists %v, want nothing", got)
	}
}
