package order_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// The lifecycles of ADR 0030, written out as the allowed (from, to) steps so
// a change to what may move where shows up as a changed line here.
var (
	boughtSteps = map[[2]order.ItemStatus]bool{
		{order.ItemToBuy, order.ItemSentToSupplier}:     true,
		{order.ItemToBuy, order.ItemReceived}:           true, // bought at the counter
		{order.ItemSentToSupplier, order.ItemInTransit}: true,
		{order.ItemSentToSupplier, order.ItemReceived}:  true,
		{order.ItemSentToSupplier, order.ItemToBuy}:     true, // undo
		{order.ItemInTransit, order.ItemReceived}:       true,
		{order.ItemInTransit, order.ItemSentToSupplier}: true, // undo
		{order.ItemReceived, order.ItemInstalled}:       true,
	}
	madeSteps = map[[2]order.ItemStatus]bool{
		{order.ItemPlanned, order.ItemInProduction}: true,
		{order.ItemInProduction, order.ItemReady}:   true,
		{order.ItemInProduction, order.ItemPlanned}: true, // undo
		{order.ItemReady, order.ItemInstalled}:      true,
	}
	allItemStatuses = []order.ItemStatus{
		order.ItemToBuy, order.ItemSentToSupplier, order.ItemInTransit, order.ItemReceived,
		order.ItemPlanned, order.ItemInProduction, order.ItemReady, order.ItemInstalled,
	}
)

func TestCanMoveItem_EveryPairPerSourcingMode(t *testing.T) {
	t.Parallel()

	modes := map[catalog.SourcingMode]map[[2]order.ItemStatus]bool{
		catalog.ModeToOrder: boughtSteps, catalog.ModeStock: boughtSteps, catalog.ModeInHouse: madeSteps,
	}
	for mode, steps := range modes {
		for _, from := range allItemStatuses {
			for _, to := range allItemStatuses {
				for _, check := range []*order.Check{nil, &ok, &bad} {
					step := steps[[2]order.ItemStatus{from, to}]
					checked := to == order.ItemReceived || to == order.ItemReady
					want := step && checked == (check != nil)
					err := order.CanMoveItem(mode, from, to, check)
					if (err == nil) != want {
						t.Errorf("%s %s→%s check %v: err = %v, want allowed %v", mode, from, to, check, err, want)
					}
					if err != nil && !errors.Is(err, order.ErrInvalidItemMove) {
						t.Errorf("%s %s→%s: err = %v, want ErrInvalidItemMove", mode, from, to, err)
					}
				}
			}
		}
	}
}

func TestCanMoveItem_RefusesAnUnknownCheck(t *testing.T) {
	t.Parallel()

	odd := order.Check("maybe")
	if err := order.CanMoveItem(catalog.ModeToOrder, order.ItemInTransit, order.ItemReceived, &odd); !errors.Is(err, order.ErrInvalidItemMove) {
		t.Errorf("err = %v, want ErrInvalidItemMove", err)
	}
}

func TestInitialStatus_PerSourcingMode(t *testing.T) {
	t.Parallel()

	// Stock items take the to_order path until Estoque lands (ADR 0030).
	want := map[catalog.SourcingMode]order.ItemStatus{
		catalog.ModeToOrder: order.ItemToBuy, catalog.ModeStock: order.ItemToBuy, catalog.ModeInHouse: order.ItemPlanned,
	}
	for mode, s := range want {
		if got := order.InitialStatus(mode); got != s {
			t.Errorf("InitialStatus(%s) = %s, want %s", mode, got, s)
		}
	}
}

func TestNextItemMoves_ListsTheLifecycleSteps(t *testing.T) {
	t.Parallel()

	got := order.NextItemMoves(catalog.ModeToOrder, order.ItemSentToSupplier)
	want := []order.ItemStatus{order.ItemInTransit, order.ItemReceived, order.ItemToBuy}
	if !slices.Equal(got, want) {
		t.Errorf("NextItemMoves = %v, want %v", got, want)
	}
	if got := order.NextItemMoves(catalog.ModeInHouse, order.ItemInstalled); len(got) != 0 {
		t.Errorf("installed moves = %v, want none", got)
	}
}

func TestMoveItem_DefectiveGoesBack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     catalog.SourcingMode
		from, to order.ItemStatus
		back     order.ItemStatus
	}{
		{"bought", catalog.ModeToOrder, order.ItemInTransit, order.ItemReceived, order.ItemToBuy},
		{"stock", catalog.ModeStock, order.ItemToBuy, order.ItemReceived, order.ItemToBuy},
		{"made", catalog.ModeInHouse, order.ItemInProduction, order.ItemReady, order.ItemInProduction},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := testkit.NewOrder().WithItem(testkit.ItemOpts{Mode: tt.mode, LeadDays: 10}).Build(t)
			o = restoreAt(t, o, "purchasing", func(r *order.Record) { r.Lines[0].Items[0].Status = tt.from })
			id := o.Items()[0].ID()
			today := date(t, "2026-12-05")
			if err := o.MoveItem(id, tt.to, &bad, today, day1, owner(t, o)); err != nil {
				t.Fatal(err)
			}
			it := o.Items()[0]
			if it.Status() != tt.back || it.Arrived() {
				t.Errorf("status = %s arrived %v, want %s and not arrived", it.Status(), it.Arrived(), tt.back)
			}
			if c, _ := it.Check(); c != order.CheckDefective {
				t.Errorf("check = %q, want defective kept", c)
			}
			if got := it.Expected(); got != today.AddDays(10) {
				t.Errorf("expected = %s, want today + 10 days", got)
			}
		})
	}
}

func TestMoveItem_ReceivedOkArrivesToday(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	promise := o.Promise()
	id := o.Items()[0].ID()
	today := date(t, "2026-12-03")
	if err := o.MoveItem(id, order.ItemReceived, &ok, today, day1, owner(t, o)); err != nil {
		t.Fatal(err)
	}
	it := o.Items()[0]
	on, arrived := it.ArrivedOn()
	if !it.Arrived() || !arrived || on != today {
		t.Errorf("arrived = %v on %s, want today", it.Arrived(), on)
	}
	if o.Promise() != promise {
		t.Errorf("promise moved to %s, want it kept at %s", o.Promise(), promise)
	}
	if h := o.PendingHistory(); h[len(h)-1].Kind != order.HistoryItem || h[len(h)-1].Check != order.CheckOK {
		t.Errorf("last history = %+v, want the item move with its check", h[len(h)-1])
	}
}

func TestMoveItem_Refusals(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	by := owner(t, o)
	id := o.Items()[0].ID()
	today := date(t, "2026-12-03")
	if err := o.MoveItem(id, order.ItemInstalled, nil, today, day1, by); !errors.Is(err, order.ErrInvalidItemMove) {
		t.Errorf("skip to installed: err = %v, want ErrInvalidItemMove", err)
	}
	if err := o.MoveItem(id, order.ItemReceived, nil, today, day1, by); !errors.Is(err, order.ErrInvalidItemMove) {
		t.Errorf("receive without check: err = %v, want ErrInvalidItemMove", err)
	}
	if err := o.MoveItem(shared.OrderItemID(testkit.NextID()), order.ItemSentToSupplier, nil, today, day1, by); !errors.Is(err, order.ErrItemNotFound) {
		t.Errorf("unknown item: err = %v, want ErrItemNotFound", err)
	}
	if err := o.Cancel("desistiu", by, day1); err != nil {
		t.Fatal(err)
	}
	if err := o.MoveItem(id, order.ItemSentToSupplier, nil, today, day1, by); !errors.Is(err, order.ErrOrderClosed) {
		t.Errorf("cancelled: err = %v, want ErrOrderClosed", err)
	}
}
