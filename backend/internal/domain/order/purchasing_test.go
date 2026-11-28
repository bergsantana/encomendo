package order_test

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// released returns an order of one item of mode, released on 2026-12-01,
// in purchasing, with its item at status from.
func released(t *testing.T, mode catalog.SourcingMode, from order.ItemStatus) *order.Order {
	t.Helper()
	o := testkit.NewOrder().WithItem(testkit.ItemOpts{Mode: mode, LeadDays: 10}).Build(t)
	return restoreAt(t, o, "purchasing", func(r *order.Record) {
		r.ReleaseDate = date(t, "2026-12-01")
		r.Lines[0].Items[0].Status = from
		r.Lines[0].Items[0].Expected = date(t, "2026-12-11")
	})
}

func TestMoveItem_BoughtOnlyInstalls(t *testing.T) {
	t.Parallel()

	today := date(t, "2026-12-05")
	for _, mode := range []catalog.SourcingMode{catalog.ModeToOrder, catalog.ModeStock} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()

			for _, to := range []order.ItemStatus{order.ItemSentToSupplier, order.ItemInTransit, order.ItemReceived, order.ItemToBuy} {
				o := released(t, mode, order.ItemToBuy)
				check := (*order.Check)(nil)
				if to == order.ItemReceived {
					check = &ok
				}
				if err := o.MoveItem(o.Items()[0].ID(), to, check, today, day1, owner(t, o)); !errors.Is(err, order.ErrItemMovedByPurchasing) {
					t.Errorf("by hand to %s: err = %v, want ErrItemMovedByPurchasing", to, err)
				}
			}
			o := released(t, mode, order.ItemReceived)
			if err := o.MoveItem(o.Items()[0].ID(), order.ItemInstalled, nil, today, day1, owner(t, o)); err != nil {
				t.Errorf("received → installed by hand: %v", err)
			}
		})
	}
	t.Run("in-house moves by hand", func(t *testing.T) {
		t.Parallel()

		o := released(t, catalog.ModeInHouse, order.ItemPlanned)
		if err := o.MoveItem(o.Items()[0].ID(), order.ItemInProduction, nil, today, day1, owner(t, o)); err != nil {
			t.Errorf("planned → in_production: %v", err)
		}
	})
}

func TestMovePurchased_Lifecycle(t *testing.T) {
	t.Parallel()

	today := date(t, "2026-12-05")
	tests := []struct {
		name     string
		mode     catalog.SourcingMode
		from, to order.ItemStatus
		check    *order.Check
		want     error
	}{
		{"send", catalog.ModeToOrder, order.ItemToBuy, order.ItemSentToSupplier, nil, nil},
		{"in transit", catalog.ModeToOrder, order.ItemSentToSupplier, order.ItemInTransit, nil, nil},
		{"receive", catalog.ModeToOrder, order.ItemInTransit, order.ItemReceived, &ok, nil},
		{"counter", catalog.ModeStock, order.ItemToBuy, order.ItemReceived, &ok, nil},
		{"undo sent", catalog.ModeToOrder, order.ItemSentToSupplier, order.ItemToBuy, nil, nil},
		{"undo in transit", catalog.ModeToOrder, order.ItemInTransit, order.ItemToBuy, nil, nil},
		{"receive without check", catalog.ModeToOrder, order.ItemSentToSupplier, order.ItemReceived, nil, order.ErrInvalidItemMove},
		{"undo with a check", catalog.ModeToOrder, order.ItemInTransit, order.ItemToBuy, &ok, order.ErrInvalidItemMove},
		{"to transit before sent", catalog.ModeToOrder, order.ItemToBuy, order.ItemInTransit, nil, order.ErrInvalidItemMove},
		{"installed is MoveItem's", catalog.ModeToOrder, order.ItemReceived, order.ItemInstalled, nil, order.ErrInvalidItemMove},
		{"in-house", catalog.ModeInHouse, order.ItemPlanned, order.ItemReceived, &ok, order.ErrInvalidItemMove},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := released(t, tt.mode, tt.from)
			id := o.Items()[0].ID()
			err := o.MovePurchased(order.PurchaseMove{Item: id, To: tt.to, Check: tt.check, Ref: "C-0012"}, today, day1, owner(t, o))
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if tt.want != nil {
				return
			}
			if got := o.Items()[0].Status(); got != tt.to {
				t.Errorf("status = %s, want %s", got, tt.to)
			}
			h := o.PendingHistory()
			if last := h[len(h)-1]; last.Kind != order.HistoryItem || last.Ref != "C-0012" || last.ItemTo != tt.to {
				t.Errorf("last history = %+v, want the item move with its ref", last)
			}
		})
	}
}

func TestMovePurchased_Refusals(t *testing.T) {
	t.Parallel()

	today := date(t, "2026-12-05")
	o := released(t, catalog.ModeToOrder, order.ItemToBuy)
	by := owner(t, o)
	if err := o.MovePurchased(order.PurchaseMove{Item: shared.OrderItemID(testkit.NextID()), To: order.ItemSentToSupplier}, today, day1, by); !errors.Is(err, order.ErrItemNotFound) {
		t.Errorf("unknown item: err = %v, want ErrItemNotFound", err)
	}
	if err := o.MovePurchased(order.PurchaseMove{Item: o.Items()[0].ID(), To: order.ItemSentToSupplier}, shared.CivilDate{}, day1, by); err == nil {
		t.Error("no today: err = nil, want a caller bug")
	}
	if err := o.Cancel("desistiu", by, day1); err != nil {
		t.Fatal(err)
	}
	if err := o.MovePurchased(order.PurchaseMove{Item: o.Items()[0].ID(), To: order.ItemSentToSupplier}, today, day1, by); !errors.Is(err, order.ErrOrderClosed) {
		t.Errorf("cancelled: err = %v, want ErrOrderClosed", err)
	}
}

func TestMovePurchased_DefectLandsOnToBuyAndClearsPromise(t *testing.T) {
	t.Parallel()

	o := released(t, catalog.ModeToOrder, order.ItemSentToSupplier)
	id := o.Items()[0].ID()
	by := owner(t, o)
	if err := o.RecordPromisedDate(id, date(t, "2026-12-09"), "C-0012", date(t, "2026-12-02"), day1, by); err != nil {
		t.Fatal(err)
	}
	today := date(t, "2026-12-09")
	if err := o.MovePurchased(order.PurchaseMove{Item: id, To: order.ItemReceived, Check: &bad, Note: "Costura solta", Ref: "C-0012"}, today, day1, by); err != nil {
		t.Fatal(err)
	}
	it := o.Items()[0]
	if it.Status() != order.ItemToBuy || it.Arrived() {
		t.Errorf("status = %s arrived %v, want to_buy, not arrived", it.Status(), it.Arrived())
	}
	if _, has := it.Promised(); has {
		t.Error("the promised date stayed after the defect, want it dropped")
	}
	if got := it.Expected(); got != today.AddDays(10) {
		t.Errorf("expected = %s, want today + the lead days", got)
	}
	h := o.PendingHistory()
	if last := h[len(h)-1]; last.Check != order.CheckDefective || last.Reason != "Costura solta" {
		t.Errorf("last history = %+v, want the defect with its note", last)
	}
}

func TestMovePurchased_UndoClearsPromise(t *testing.T) {
	t.Parallel()

	o := released(t, catalog.ModeToOrder, order.ItemInTransit)
	id := o.Items()[0].ID()
	by := owner(t, o)
	today := date(t, "2026-12-03")
	if err := o.RecordPromisedDate(id, date(t, "2026-12-20"), "C-0012", today, day1, by); err != nil {
		t.Fatal(err)
	}
	if err := o.MovePurchased(order.PurchaseMove{Item: id, To: order.ItemToBuy, Ref: "C-0012"}, today, day1, by); err != nil {
		t.Fatal(err)
	}
	it := o.Items()[0]
	if _, has := it.Promised(); has || it.Expected() != today.AddDays(10) {
		t.Errorf("promised kept %v, expected %s; want none and today + lead days", has, it.Expected())
	}
}

func TestRecordPromisedDate_LateRaisesAtRiskKeepsPromise(t *testing.T) {
	t.Parallel()

	o := released(t, catalog.ModeToOrder, order.ItemSentToSupplier)
	id := o.Items()[0].ID()
	promise := o.Promise()
	limit := o.ArrivalLimit()
	today := date(t, "2026-12-03")
	late := limit.AddDays(4)
	if err := o.RecordPromisedDate(id, late, "C-0012", today, day1, owner(t, o)); err != nil {
		t.Fatal(err)
	}
	it := o.Items()[0]
	if got, has := it.Promised(); !has || got != late {
		t.Errorf("promised = %s %v, want %s", got, has, late)
	}
	if it.Expected() != late {
		t.Errorf("expected = %s, want the promised date %s", it.Expected(), late)
	}
	if o.Promise() != promise {
		t.Errorf("promise moved to %s, want %s kept", o.Promise(), promise)
	}
	if a := o.Alerts(today)[id]; len(a) == 0 || a[0] != schedule.AlertAtRisk {
		t.Errorf("alerts = %v, want at_risk", a)
	}
	h := o.PendingHistory()
	if last := h[len(h)-1]; last.Kind != order.HistoryPromised || last.NewDate != late || last.Ref != "C-0012" {
		t.Errorf("last history = %+v, want the promised date", last)
	}
	if got := o.Record().Lines[0].Items[0].Promised; got != late {
		t.Errorf("record promised = %s, want %s", got, late)
	}
}

func TestRecordPromisedDate_AddsOptionDays(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 10, Options: []order.ChosenOption{{
		Option: shared.OptionID(testkit.NextID()), Choice: shared.ChoiceID(testkit.NextID()), Name: "Costura", Label: "Dupla",
		ExtraPrice: brl(t, 0), ExtraDays: 2,
	}}}).Build(t)
	o = restoreAt(t, o, "purchasing", func(r *order.Record) {
		r.ReleaseDate = date(t, "2026-12-01")
		r.Lines[0].Items[0].Status = order.ItemSentToSupplier
	})
	if err := o.RecordPromisedDate(o.Items()[0].ID(), date(t, "2026-12-08"), "C-0001", date(t, "2026-12-02"), day1, owner(t, o)); err != nil {
		t.Fatal(err)
	}
	if got := o.Items()[0].Expected(); got != date(t, "2026-12-10") {
		t.Errorf("expected = %s, want the promised date + 2 option days", got)
	}
}

func TestRecordPromisedDate_Refusals(t *testing.T) {
	t.Parallel()

	today := date(t, "2026-12-05")
	tests := []struct {
		name string
		mode catalog.SourcingMode
		from order.ItemStatus
		day  string
		want error
	}{
		{"not sent", catalog.ModeToOrder, order.ItemToBuy, "2026-12-09", order.ErrInvalidItemMove},
		{"received", catalog.ModeToOrder, order.ItemReceived, "2026-12-09", order.ErrInvalidItemMove},
		{"in-house", catalog.ModeInHouse, order.ItemInProduction, "2026-12-09", order.ErrInvalidItemMove},
		{"past day", catalog.ModeToOrder, order.ItemInTransit, "2026-12-04", order.ErrDateInPast},
		{"today is fine", catalog.ModeToOrder, order.ItemInTransit, "2026-12-05", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := released(t, tt.mode, tt.from)
			if tt.from == order.ItemReceived {
				o = restoreAt(t, o, "purchasing", func(r *order.Record) {
					c := order.CheckOK
					r.Lines[0].Items[0].Check, r.Lines[0].Items[0].ArrivedOn = &c, date(t, "2026-12-04")
				})
			}
			err := o.RecordPromisedDate(o.Items()[0].ID(), date(t, tt.day), "C-0001", today, day1, owner(t, o))
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
			var v *shared.ValidationError
			if errors.Is(tt.want, order.ErrDateInPast) && !errors.As(err, &v) {
				t.Errorf("err = %v, want a ValidationError on date", err)
			}
		})
	}
}

func TestRecordPaidCost(t *testing.T) {
	t.Parallel()

	o := released(t, catalog.ModeToOrder, order.ItemReceived)
	id := o.Items()[0].ID()
	if err := o.RecordPaidCost(id, brl(t, 41000)); err != nil {
		t.Fatal(err)
	}
	if got := o.Items()[0].Sourcing().PaidCost; got == nil || got.Cents() != 41000 {
		t.Errorf("paid cost = %v, want R$ 410,00", got)
	}
	if got := o.Record().Lines[0].Items[0].Sourcing.PaidCost; got == nil || got.Cents() != 41000 {
		t.Errorf("record paid cost = %v, want it kept", got)
	}
	for _, h := range o.PendingHistory() {
		if h.Amount.Cents() == 41000 {
			t.Errorf("history %+v names the cost; costs are the owner's", h)
		}
	}
	usd, err := shared.NewMoney(100, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if err := o.RecordPaidCost(id, usd); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("other currency: err = %v, want ErrInvalid", err)
	}
	if err := o.RecordPaidCost(shared.OrderItemID(testkit.NextID()), brl(t, 1)); !errors.Is(err, order.ErrItemNotFound) {
		t.Errorf("unknown item: err = %v, want ErrItemNotFound", err)
	}
	made := released(t, catalog.ModeInHouse, order.ItemReady)
	if err := made.RecordPaidCost(made.Items()[0].ID(), brl(t, 1)); !errors.Is(err, order.ErrInvalidItemMove) {
		t.Errorf("in-house: err = %v, want ErrInvalidItemMove", err)
	}
}

func TestPurchasable_EveryPhase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		key            string
		releasedOn     string
		cancel         bool
		before, inTime bool // want with the setting off, and on
	}{
		{"draft", "draft", "", false, false, false},
		{"awaiting deposit", "awaiting_deposit", "", false, false, true},
		{"released", "purchasing", "2026-12-01", false, true, true},
		{"assembling", "assembling", "2026-12-01", false, true, true},
		{"delivered", "delivered", "2026-12-01", false, false, false},
		{"cancelled", "awaiting_deposit", "2026-12-01", true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := restoreAt(t, testkit.NewOrder().Build(t), "draft", nil)
			o = restoreAt(t, o, workflow.StatusKey(tt.key), func(r *order.Record) {
				if tt.releasedOn != "" {
					r.ReleaseDate = date(t, tt.releasedOn)
				}
			})
			if tt.cancel {
				if err := o.Cancel("desistiu", owner(t, o), day1); err != nil {
					t.Fatal(err)
				}
			}
			if got := o.Purchasable(false); got != tt.before {
				t.Errorf("Purchasable(false) = %v, want %v", got, tt.before)
			}
			if got := o.Purchasable(true); got != tt.inTime {
				t.Errorf("Purchasable(true) = %v, want %v", got, tt.inTime)
			}
		})
	}
}

func TestRedate_KeepsArrived(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 10}).WithItem(testkit.ItemOpts{LeadDays: 4}).Build(t)
	arrivedOn := date(t, "2026-12-03")
	o = restoreAt(t, o, "purchasing", func(r *order.Record) {
		r.ReleaseDate = date(t, "2026-12-01")
		r.Lines[0].Items[0].Expected = date(t, "2027-01-30") // stale
		r.Lines[0].Items[0].Status = order.ItemSentToSupplier
		r.Lines[0].Items[0].Promised = date(t, "2026-12-07")
		it := &r.Lines[1].Items[0]
		c := order.CheckOK
		it.Status, it.Check, it.ArrivedOn, it.Expected = order.ItemReceived, &c, arrivedOn, arrivedOn
	})
	promise := o.Promise()
	o.Redate(date(t, "2026-12-04"))
	if got := o.Items()[0].Expected(); got != date(t, "2026-12-07") {
		t.Errorf("sent item expected = %s, want its promised date", got)
	}
	if got := o.Items()[1].Expected(); got != arrivedOn {
		t.Errorf("arrived item expected = %s, want %s kept", got, arrivedOn)
	}
	if o.Promise() != promise {
		t.Errorf("promise moved to %s", o.Promise())
	}
}

func TestNextMoves_BoughtOnlyInstalled(t *testing.T) {
	t.Parallel()

	for status, want := range map[order.ItemStatus]int{order.ItemToBuy: 0, order.ItemSentToSupplier: 0, order.ItemInTransit: 0, order.ItemReceived: 1} {
		o := released(t, catalog.ModeToOrder, status)
		if got := o.Items()[0].NextMoves(); len(got) != want || (want == 1 && got[0] != order.ItemInstalled) {
			t.Errorf("%s: next moves = %v, want %d (installed)", status, got, want)
		}
	}
	made := released(t, catalog.ModeInHouse, order.ItemPlanned)
	if got := made.Items()[0].NextMoves(); len(got) != 1 || got[0] != order.ItemInProduction {
		t.Errorf("in-house planned: next moves = %v, want in_production", got)
	}
}

func TestCost_PrefersThePricePaid(t *testing.T) {
	t.Parallel()

	cost := int64(40_000)
	o := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 10, Cost: &cost}).Build(t)
	before, ok := o.Cost()
	if !ok || before.Cents() != 40_000 {
		t.Fatalf("cost = %v %v, want the order-time R$ 400,00", before, ok)
	}
	if err := o.RecordPaidCost(o.Items()[0].ID(), brl(t, 45_000)); err != nil {
		t.Fatal(err)
	}
	if after, ok := o.Cost(); !ok || after.Cents() != 45_000 {
		t.Errorf("cost = %v %v, want the price paid R$ 450,00", after, ok)
	}
}
