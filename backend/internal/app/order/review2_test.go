package order_test

import (
	"testing"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// With more active orders than Hoje reads, Hoje and Financeiro say so
// instead of understating counts and money (go-reviewer N2).
func TestTodayAndFinance_SayWhenTheyHitTheirCap(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.staff(t)
	for range 500 {
		h.create(ctx, t, nil)
	}
	today, err := h.svc.Today(ctx)
	if err != nil || today.Truncated || today.Counts.Active != 500 {
		t.Fatalf("at the cap: truncated %v active %d err %v", today.Truncated, today.Counts.Active, err)
	}
	h.create(ctx, t, nil)
	today, err = h.svc.Today(ctx)
	if err != nil || !today.Truncated || today.Counts.Active != 500 {
		t.Errorf("past the cap: truncated %v active %d err %v", today.Truncated, today.Counts.Active, err)
	}
	fin, err := h.svc.Finance(h.owner(t), day2(t, "2026-12-01"), day2(t, "2026-12-31"))
	if err != nil || !fin.Truncated {
		t.Errorf("finance past the cap: truncated %v err %v", fin.Truncated, err)
	}
}

// The period's margin total is computed by the server, over the orders
// whose costs are known (go-reviewer N3).
func TestFinance_MarginsTotal(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	staff := h.staff(t)
	known := h.deliver(staff, t, h.released(staff, t, nil)) // banco: R$ 1.000, cost R$ 400
	unknown := h.addTampa(t)
	h.deliver(staff, t, h.released(staff, t, func(in *apporder.CreateInput) { in.Lines = []apporder.LineRequest{{Item: unknown, Quantity: 1}} }))
	fin, err := h.svc.Finance(h.owner(t), day2(t, "2026-11-01"), day2(t, "2027-01-31"))
	if err != nil {
		t.Fatal(err)
	}
	mt := fin.MarginsTotal
	if len(fin.Margins) != 2 || mt.Cost.Cents() != 40_000 || mt.Margin == nil || mt.Margin.Cents != 60_000 || mt.Margin.BasisPoints != 6000 {
		t.Errorf("margins total = %+v (rows %d); known order %s", mt, len(fin.Margins), known.Number)
	}
	if mt.Total.Cents() != fin.Margins[0].Total.Cents()+fin.Margins[1].Total.Cents() {
		t.Errorf("total = %v, want every handed-over order", mt.Total)
	}
}

// The alert filter reads ahead until its page is full, so "Com alerta" is
// not an empty page with a cursor (go-reviewer N5).
func TestList_AlertFilterFillsItsPage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.staff(t)
	for range 3 {
		h.create(ctx, t, nil) // not released: no alerts, earliest deliveries
	}
	late := h.released(ctx, t, nil)
	h.clock.Advance(20 * day)
	p, err := shared.NewPageRequest("", 2)
	if err != nil {
		t.Fatal(err)
	}
	page, err := h.svc.List(ctx, apporder.ListFilter{Alert: true}, p)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != late.ID {
		t.Errorf("alert page = %+v err %v, want the late order on the first page", page.Items, err)
	}
}
