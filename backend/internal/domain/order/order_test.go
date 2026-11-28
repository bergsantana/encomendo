package order_test

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

func TestNew_StartsAtFirstDraftStatus_ForEveryPreset(t *testing.T) {
	t.Parallel()

	for _, wf := range testkit.Presets(t) {
		o := testkit.NewOrder().WithPreset(wf.Key()).Build(t)
		if o.Status() != wf.First() || o.Phase() != workflow.PhaseDraft {
			t.Errorf("%s: status = %+v, want the first status, a draft", wf.Key(), o.Status())
		}
		if o.Workflow().ID() != wf.ID() || o.Workflow().Version() != wf.Version() {
			t.Errorf("%s: the order does not keep its workflow version", wf.Key())
		}
		if o.Version() != 1 || len(o.PendingHistory()) != 1 || o.PendingHistory()[0].Kind != order.HistoryCreated {
			t.Errorf("%s: version %d history %+v", wf.Key(), o.Version(), o.PendingHistory())
		}
		if _, ok := o.ReleaseDate(); ok {
			t.Errorf("%s: a new order is released", wf.Key())
		}
	}
}

func TestNew_RefusesEmptyLinesMixedCurrencyDiscountAboveSubtotal(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		edit  func(t *testing.T, s *order.NewSpec)
		field string
	}{
		"no lines":       {func(_ *testing.T, s *order.NewSpec) { s.Lines = nil }, "lines"},
		"mixed currency": {func(t *testing.T, s *order.NewSpec) { s.Lines[0].Price = usd(t, 100_000) }, "lines[0].price"},
		"item currency": {func(t *testing.T, s *order.NewSpec) {
			s.Lines[0].Items[0].Snapshot.UnitPrice = usd(t, 1)
		}, "lines[0].items[0].unitPrice"},
		"discount above subtotal": {func(t *testing.T, s *order.NewSpec) { s.Terms.Discount = brl(t, 100_001) }, "discountCents"},
		"negative discount":       {func(t *testing.T, s *order.NewSpec) { s.Terms.Discount = brl(t, -1) }, "discountCents"},
		"freight on pickup":       {func(t *testing.T, s *order.NewSpec) { s.Terms.Freight = brl(t, 1_500) }, "freightCents"},
		"delivery without address": {func(_ *testing.T, s *order.NewSpec) {
			s.Terms.Handover = order.HandoverDelivery
		}, "addressId"},
		"unknown handover": {func(_ *testing.T, s *order.NewSpec) { s.Terms.Handover = "drone" }, "handover"},
		"deposit over 100": {func(_ *testing.T, s *order.NewSpec) { s.Terms.DepositPercent = 101 }, "depositPercent"},
		"assembly over 60": {func(_ *testing.T, s *order.NewSpec) { s.Terms.AssemblyDays = 61 }, "assemblyDays"},
		"negative margin":  {func(_ *testing.T, s *order.NewSpec) { s.Terms.SafetyMarginDays = -1 }, "safetyMarginDays"},
		"quantity 100": {func(_ *testing.T, s *order.NewSpec) {
			s.Lines[0].Items[0].Snapshot.Quantity = 100
		}, "lines[0].items[0].quantity"},
		"no name": {func(_ *testing.T, s *order.NewSpec) { s.Lines[0].Items[0].Snapshot.Name = " " }, "lines[0].items[0].name"},
		"loose line with two items": {func(t *testing.T, s *order.NewSpec) {
			s.Lines[0].Items = append(s.Lines[0].Items, testkit.ItemSpec(t, testkit.ItemOpts{}))
		}, "lines[0]"},
		"unknown mode": {func(_ *testing.T, s *order.NewSpec) {
			s.Lines[0].Items[0].Snapshot.SourcingMode = "magic"
		}, "lines[0].items[0].sourcingMode"},
		"lead over 365": {func(_ *testing.T, s *order.NewSpec) { s.Lines[0].Items[0].Snapshot.LeadDays = 366 }, "lines[0].items[0].leadDays"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := testkit.NewOrder().Spec(t)
			tt.edit(t, &s)
			_, err := order.New(s)
			if !errors.Is(err, shared.ErrInvalid) || fieldsOf(err)[tt.field] == "" {
				t.Errorf("err = %v (fields %v), want a field error on %s", err, fieldsOf(err), tt.field)
			}
		})
	}
}

func TestNew_DiscountMessageIsTheDesignDocuments(t *testing.T) {
	t.Parallel()

	s := testkit.NewOrder().Spec(t)
	s.Terms.Discount = brl(t, 100_001)
	_, err := order.New(s)
	if got := fieldsOf(err)["discountCents"]; got != "O desconto não pode passar do valor dos itens." {
		t.Errorf("message = %q", got)
	}
}

func TestNew_RefusesMissingIdentity(t *testing.T) {
	t.Parallel()

	edits := map[string]func(*order.NewSpec){
		"tenant":   func(s *order.NewSpec) { s.Tenant = shared.TenantID{} },
		"id":       func(s *order.NewSpec) { s.ID = shared.OrderID{} },
		"customer": func(s *order.NewSpec) { s.Customer = shared.CustomerID{} },
		"number":   func(s *order.NewSpec) { s.Number = 0 },
		"workflow": func(s *order.NewSpec) { s.Workflow = workflow.Workflow{} },
		"tracking": func(s *order.NewSpec) { s.Tracking = shared.SecretHash{} },
		"line id":  func(s *order.NewSpec) { s.Lines[0].ID = shared.OrderLineID{} },
		"item id":  func(s *order.NewSpec) { s.Lines[0].Items[0].ID = shared.OrderItemID{} },
	}
	for name, edit := range edits {
		s := testkit.NewOrder().Spec(t)
		edit(&s)
		if _, err := order.New(s); !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("no %s: err = %v, want ErrInvalid", name, err)
		}
	}
	s := testkit.NewOrder().Spec(t)
	s.Today = shared.CivilDate{}
	if _, err := order.New(s); !isBug(err) {
		t.Errorf("no today: err = %v, want a caller's bug (no sentinel)", err)
	}
}

func TestNew_SnapshotsAreCopies(t *testing.T) {
	t.Parallel()

	extra := brl(t, 5_000)
	s := testkit.NewOrder().WithItem(testkit.ItemOpts{Options: []order.ChosenOption{{Name: "Cor", Label: "Vermelha", ExtraPrice: extra, ExtraDays: 2}}}).Spec(t)
	o, err := order.New(s)
	if err != nil {
		t.Fatal(err)
	}
	s.Lines[0].Items[0].Snapshot.Options[0].Label = "Azul"
	if got := o.Items()[0].Snapshot().Options[0].Label; got != "Vermelha" {
		t.Errorf("the order shares the spec's options: label = %q", got)
	}
	snap := o.Items()[0].Snapshot()
	snap.Options[0].Label = "Verde"
	if got := o.Items()[0].Snapshot().Options[0].Label; got != "Vermelha" {
		t.Errorf("a returned snapshot changed the order: label = %q", got)
	}
}

func TestNumber_String(t *testing.T) {
	t.Parallel()

	for n, want := range map[order.Number]string{1: "P-0001", 231: "P-0231", 9999: "P-9999", 12345: "P-12345"} {
		if got := n.String(); got != want {
			t.Errorf("%d = %q, want %q", n, got, want)
		}
	}
}

func TestTotals_KitKeepAndLoseWithOptionsFreightDiscount(t *testing.T) {
	t.Parallel()

	// The catalog prices a kit line (PriceWithSwaps, in keep or lose mode)
	// and passes the price in; the order adds lines, freight and discount.
	keep, lose := cb500x(t), cb500x(t)
	keep.Price = brl(t, 260_000) // kit 2.500 − 800 + 900
	lose.Price = brl(t, 300_000) // single-item prices summed
	keep.Kit.Swaps = []order.SwapSnapshot{{RemovedName: "Guidão", AddedName: "Guidão alto"}}
	address := &order.DeliveryAddress{ID: shared.AddressID(testkit.NextID()), Label: "Casa", CEP: "69057000", Street: "Rua A", City: "Manaus", State: "AM"}
	opt := []order.ChosenOption{{Name: "Cor", Label: "Vermelha", ExtraPrice: brl(t, 5_000), ExtraDays: 2}}
	o := testkit.NewOrder().WithLine(keep).WithLine(lose).
		WithItem(testkit.ItemOpts{Price: 10_000, Quantity: 2, Options: opt}).
		WithTerms(func(tr *order.Terms) {
			tr.Handover, tr.Address = order.HandoverDelivery, address
			tr.Freight, tr.Discount = brl(t, 8_000), brl(t, 20_000)
		}).Build(t)
	// 260.000 + 300.000 + (10.000 + 5.000) × 2 = 590.000
	if got := o.Subtotal().Cents(); got != 590_000 {
		t.Errorf("subtotal = %d, want 590000", got)
	}
	total, err := o.Total()
	if err != nil || total.Cents() != 578_000 {
		t.Errorf("total = %v %v, want 590000 + 8000 − 20000", total, err)
	}
	if got := o.DepositDue().Cents(); got != 289_000 {
		t.Errorf("deposit = %d, want half the total", got)
	}
	// The option's extra days count in the item's date: 0 lead + 2 days.
	if got := o.Items()[6].Expected(); got != date(t, "2026-12-03") {
		t.Errorf("option item expected = %s, want 2 extra days", got)
	}
}

func TestCostAndMargin_OverWhatWasSold(t *testing.T) {
	t.Parallel()

	c1, c2 := int64(30_000), int64(10_000)
	o := testkit.NewOrder().
		WithItem(testkit.ItemOpts{Price: 50_000, Quantity: 2, Cost: &c1}).
		WithItem(testkit.ItemOpts{Price: 20_000, Cost: &c2}).
		WithTerms(func(tr *order.Terms) { tr.Discount = brl(t, 20_000) }).Build(t)
	cost, ok := o.Cost()
	if !ok || cost.Cents() != 70_000 {
		t.Fatalf("cost = %v %v, want 30000 × 2 + 10000", cost, ok)
	}
	m, ok := o.Margin()
	// Sold 120.000 − 20.000 = 100.000; margin 30.000 = 30%.
	if !ok || m.Cents != 30_000 || m.BasisPoints != 3000 {
		t.Errorf("margin = %+v %v, want 30000 and 3000 bp", m, ok)
	}
	unknown := testkit.NewOrder().WithItem(testkit.ItemOpts{Cost: &c1}).WithItem(testkit.ItemOpts{}).Build(t)
	if _, ok := unknown.Cost(); ok {
		t.Error("cost known with an item's cost missing")
	}
	if _, ok := unknown.Margin(); ok {
		t.Error("margin known with an item's cost missing")
	}
}

func TestAlerts_AtRiskAndLateOnlyBeforeArrival(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 5}).WithItem(testkit.ItemOpts{LeadDays: 5}).Build(t)
	if _, err := pay(t, o, order.PaymentDeposit, 100_000, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	// Promise 12-11, limit 12-08. Push the promise earlier by hand so the
	// items (expected 12-06) are past the limit: limit becomes 12-03.
	if _, err := o.ChangeDeliveryDate(date(t, "2026-12-06"), "", owner(t, o), date(t, "2026-12-01"), day1); err != nil {
		t.Fatal(err)
	}
	ids := []shared.OrderItemID{o.Items()[0].ID(), o.Items()[1].ID()}
	if got := o.Alerts(date(t, "2026-12-02"))[ids[0]]; len(got) != 1 || got[0] != schedule.AlertAtRisk {
		t.Errorf("before the expected date: %v, want at_risk", got)
	}
	if got := o.Alerts(date(t, "2026-12-07"))[ids[0]]; len(got) != 2 {
		t.Errorf("after the expected date: %v, want at_risk and late", got)
	}
	if err := move(o, ids[1], order.ItemReceived, &ok, date(t, "2026-12-07"), owner(t, o)); err != nil {
		t.Fatal(err)
	}
	if got, has := o.Alerts(date(t, "2026-12-07"))[ids[1]]; has {
		t.Errorf("arrived item has alerts %v", got)
	}
	if err := o.Cancel("cliente desistiu", owner(t, o), day1); err != nil {
		t.Fatal(err)
	}
	if got := o.Alerts(date(t, "2026-12-07")); len(got) != 0 {
		t.Errorf("cancelled order has alerts %v", got)
	}
}

func TestChangeDeliveryDate_OwnerPromiseSurvivesDeposit(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 5}).Build(t)
	chosen := date(t, "2026-12-30")
	old, err := o.ChangeDeliveryDate(chosen, "Cliente viaja", owner(t, o), date(t, "2026-12-01"), day1)
	if err != nil {
		t.Fatal(err)
	}
	if old != date(t, "2026-12-11") || !o.PromiseChangedByHand() {
		t.Errorf("old = %s by hand %v", old, o.PromiseChangedByHand())
	}
	if _, err := pay(t, o, order.PaymentDeposit, 50_000, "2026-12-03"); err != nil {
		t.Fatal(err)
	}
	if o.Promise() != chosen {
		t.Errorf("the deposit reset the promise to %s", o.Promise())
	}
	h := o.PendingHistory()
	var dated bool
	for _, e := range h {
		if e.Kind == order.HistoryDate && e.OldDate == old && e.NewDate == chosen && e.Reason == "Cliente viaja" {
			dated = true
		}
	}
	if !dated {
		t.Errorf("history %+v lacks the date change", h)
	}
}

func TestChangeDeliveryDate_Refusals(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	by := owner(t, o)
	_, err := o.ChangeDeliveryDate(date(t, "2026-11-30"), "", by, date(t, "2026-12-01"), day1)
	if !errors.Is(err, order.ErrDateInPast) || fieldsOf(err)["date"] != "Escolha uma data a partir de hoje." {
		t.Errorf("past date: err = %v", err)
	}
	long := make([]byte, 501)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := o.ChangeDeliveryDate(date(t, "2026-12-20"), string(long), by, date(t, "2026-12-01"), day1); fieldsOf(err)["reason"] == "" {
		t.Errorf("long reason: err = %v", err)
	}
	if err := o.Cancel("x", by, day1); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ChangeDeliveryDate(date(t, "2026-12-20"), "", by, date(t, "2026-12-01"), day1); !errors.Is(err, order.ErrOrderClosed) {
		t.Errorf("cancelled: err = %v", err)
	}
}

func TestCancel_RefusedAfterHandover(t *testing.T) {
	t.Parallel()

	base := testkit.NewOrder().Build(t)
	done := restoreAt(t, base, "delivered", nil)
	if err := done.Cancel("tarde demais", owner(t, done), day1); !errors.Is(err, order.ErrOrderClosed) {
		t.Errorf("after handover: err = %v, want ErrOrderClosed", err)
	}
	o := restoreAt(t, base, "items_arriving", nil)
	if err := o.Cancel(" ", owner(t, o), day1); fieldsOf(err)["reason"] == "" {
		t.Errorf("no reason: err = %v", err)
	}
	if err := o.Cancel("Cliente desistiu", owner(t, o), day1); err != nil {
		t.Fatal(err)
	}
	if reason, ok := o.CancelReason(); !ok || reason != "Cliente desistiu" || o.Phase() != workflow.PhaseCancelled || !o.Closed() {
		t.Errorf("cancel = %q %v phase %s", reason, ok, o.Phase())
	}
	if err := o.Cancel("again", owner(t, o), day1); !errors.Is(err, order.ErrOrderClosed) {
		t.Errorf("twice: err = %v", err)
	}
	if _, err := pay(t, o, order.PaymentBalance, 1, "2026-12-02"); !errors.Is(err, order.ErrOrderClosed) {
		t.Errorf("payment on a cancelled order: err = %v", err)
	}
}

func TestRecord_RestoreRoundTrip_KeepsCancelledFrom(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItems(2).Build(t)
	if _, err := pay(t, o, order.PaymentDeposit, 100_000, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	if err := o.Advance("purchasing", nil, owner(t, o), date(t, "2026-12-01"), day1); err != nil {
		t.Fatal(err)
	}
	if err := o.Cancel("desistiu", owner(t, o), day1); err != nil {
		t.Fatal(err)
	}
	r := o.Record()
	if r.StatusKey != "purchasing" || r.CancelledAt.IsZero() {
		t.Fatalf("record status %q cancelled %v, want the last real status and the cancel time", r.StatusKey, r.CancelledAt)
	}
	back, err := order.Restore(r, o.Workflow())
	if err != nil {
		t.Fatal(err)
	}
	if back.Phase() != workflow.PhaseCancelled || back.Promise() != o.Promise() || len(back.Items()) != 2 || back.Paid() != o.Paid() {
		t.Errorf("restored = %+v", back.Record())
	}
	if len(back.PendingHistory()) != 0 {
		t.Error("a restored order has pending history")
	}
	if back.Record().StatusKey != "purchasing" {
		t.Error("restore lost the status a cancelled order left")
	}
}

func TestRestore_RefusesBrokenRecords(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	other := testkit.Preset(t, testkit.PresetProntaEntrega)
	if _, err := order.Restore(o.Record(), other); !errors.Is(err, order.ErrInvalidRecord) {
		t.Errorf("wrong workflow: err = %v", err)
	}
	edits := map[string]func(*order.Record){
		"unknown status": func(r *order.Record) { r.StatusKey = "flying" },
		"item status":    func(r *order.Record) { r.Lines[0].Items[0].Status = order.ItemPlanned },
		"no lines":       func(r *order.Record) { r.Lines = nil },
		"version 0":      func(r *order.Record) { r.Version = 0 },
	}
	for name, edit := range edits {
		r := o.Record()
		edit(&r)
		_, err := order.Restore(r, o.Workflow())
		if !errors.Is(err, order.ErrInvalidRecord) || errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalidRecord and never ErrInvalid", name, err)
		}
	}
}

func usd(t *testing.T, cents int64) shared.Money {
	t.Helper()
	m, err := shared.NewMoney(cents, "USD")
	if err != nil {
		t.Fatal(err)
	}
	return m
}
