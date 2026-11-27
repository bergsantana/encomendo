package order_test

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// changeSpec returns a change that keeps o's lines and terms, so a test
// edits only what it is about.
func changeSpec(t *testing.T, o *order.Order, today string) order.ChangeSpec {
	t.Helper()
	var lines []order.LineSpec
	for _, l := range o.Lines() {
		ls := order.LineSpec{ID: l.ID(), Price: l.Price()}
		if k, ok := l.Kit(); ok {
			ls.Kit = &k
		}
		for _, i := range l.Items() {
			ls.Items = append(ls.Items, order.ItemSpec{ID: i.ID(), Snapshot: i.Snapshot(), Sourcing: i.Sourcing()})
		}
		lines = append(lines, ls)
	}
	return order.ChangeSpec{Lines: lines, Terms: o.Terms(), By: owner(t, o), Now: day1, Today: date(t, today)}
}

func TestChange_BeforePayment_Free(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	if o.Editable() != order.EditFree {
		t.Fatalf("editable = %s, want free", o.Editable())
	}
	s := changeSpec(t, o, "2026-12-01")
	s.Lines = append(s.Lines, testkit.LooseLine(t, testkit.ItemOpts{Name: "Retrovisor", Price: 20_000, LeadDays: 9}))
	res, err := o.Change(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Paid || res.NewTotal.Cents() != 120_000 || len(res.Added) != 1 || len(res.Removed) != 0 {
		t.Errorf("result = %+v", res)
	}
	// Not released: the promise follows the forecast, from today.
	if got := o.Promise(); got != date(t, "2026-12-15") {
		t.Errorf("promise = %s, want 9 + 3 + 2 days from today", got)
	}
}

func TestChange_AfterPayment_NeedsApproval(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	if _, err := pay(t, o, order.PaymentDeposit, 50_000, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	if o.Editable() != order.EditApproval {
		t.Fatalf("editable = %s, want approval", o.Editable())
	}
	s := changeSpec(t, o, "2026-12-02")
	if _, err := o.Change(s); !errors.Is(err, order.ErrApprovalRequired) {
		t.Fatalf("err = %v, want ErrApprovalRequired", err)
	}
	s.CustomerApproved = true
	res, err := o.Change(s)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Paid {
		t.Error("result does not say the order was paid")
	}
	if h := o.PendingHistory(); !h[len(h)-1].CustomerApproved {
		t.Error("history does not record the approval")
	}
}

func TestChange_RefusedInAssembling(t *testing.T) {
	t.Parallel()

	base := testkit.NewOrder().Build(t)
	for key, want := range map[string]error{"assembling": order.ErrNotEditable, "ready": order.ErrNotEditable, "delivered": order.ErrOrderClosed} {
		o := restoreAt(t, base, workflow.StatusKey(key), nil)
		s := changeSpec(t, o, "2026-12-02")
		s.CustomerApproved = true
		if _, err := o.Change(s); !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", key, err, want)
		}
		if o.Editable() != order.EditClosed {
			t.Errorf("%s: editable = %s, want closed", key, o.Editable())
		}
	}
}

func TestChange_LaterForecastMovesPromise(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 5}).Build(t)
	if _, err := pay(t, o, order.PaymentDeposit, 50_000, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	promise := o.Promise() // 12-01 + 5 + 3 + 2
	s := changeSpec(t, o, "2026-12-03")
	s.CustomerApproved = true
	s.Lines = append(s.Lines, testkit.LooseLine(t, testkit.ItemOpts{Price: 10_000, LeadDays: 20}))
	res, err := o.Change(s)
	if err != nil {
		t.Fatal(err)
	}
	// Added on 12-03 after the release, so the new item can only be bought
	// from today: 12-03 + 20 + 3 + 2.
	want := date(t, "2026-12-28")
	if o.Promise() != want || res.OldPromise != promise || res.NewPromise != want {
		t.Errorf("promise %s (result %s → %s), want %s → %s", o.Promise(), res.OldPromise, res.NewPromise, promise, want)
	}

	// An earlier forecast leaves the promise alone.
	s = changeSpec(t, o, "2026-12-03")
	s.CustomerApproved = true
	s.Lines = s.Lines[:1]
	if _, err := o.Change(s); err != nil {
		t.Fatal(err)
	}
	if o.Promise() != want {
		t.Errorf("earlier forecast moved the promise to %s", o.Promise())
	}
}

func TestChange_LowerTotalShowsRefund(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItems(2).Build(t) // R$ 2.000,00
	if _, err := pay(t, o, order.PaymentDeposit, 150_000, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	s := changeSpec(t, o, "2026-12-02")
	s.CustomerApproved = true
	s.Lines = s.Lines[:1]
	res, err := o.Change(s)
	if err != nil {
		t.Fatal(err)
	}
	if res.RefundDue.Cents() != 50_000 || o.RefundDue().Cents() != 50_000 || !o.BalanceDue().IsZero() {
		t.Errorf("refund %v / %v balance %v, want R$ 500,00 back and nothing due", res.RefundDue, o.RefundDue(), o.BalanceDue())
	}
	if len(res.Removed) != 1 {
		t.Errorf("removed = %v, want one item", res.Removed)
	}
	if !o.DepositPaid() {
		t.Error("a confirmed deposit stopped being paid")
	}
}

func TestChange_KeepsSurvivingItemState(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItems(2).Build(t)
	kept := o.Items()[0].ID()
	if err := o.MoveItem(kept, order.ItemReceived, &ok, date(t, "2026-12-01"), day1, owner(t, o)); err != nil {
		t.Fatal(err)
	}
	s := changeSpec(t, o, "2026-12-02")
	s.Lines = append(s.Lines, testkit.LooseLine(t, testkit.ItemOpts{LeadDays: 3}))
	if _, err := o.Change(s); err != nil {
		t.Fatal(err)
	}
	items := o.Items()
	if items[0].ID() != kept || items[0].Status() != order.ItemReceived || !items[0].Arrived() {
		t.Errorf("kept item = %s %v, want received and arrived", items[0].Status(), items[0].Arrived())
	}
	if items[2].Status() != order.ItemToBuy {
		t.Errorf("new item = %s, want to_buy", items[2].Status())
	}
}

func TestChange_RefusesBadTermsAndKeepsTheOrder(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	before := o.Record()
	s := changeSpec(t, o, "2026-12-02")
	s.Terms.Discount = brl(t, 200_000)
	if _, err := o.Change(s); fieldsOf(err)["discountCents"] == "" {
		t.Fatalf("err = %v, want a discount field error", err)
	}
	s = changeSpec(t, o, "2026-12-02")
	s.Lines = nil
	if _, err := o.Change(s); fieldsOf(err)["lines"] == "" {
		t.Errorf("no lines: err = %v", err)
	}
	if after := o.Record(); after.Terms.Discount != before.Terms.Discount || len(after.Lines) != len(before.Lines) {
		t.Error("a refused change altered the order")
	}
}
