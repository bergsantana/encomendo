package order_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// Sequences found by the review of PR B: each reproduces a reachable order
// that went wrong or got stuck.

func TestChange_HigherTotalKeepsTheConfirmedDeposit(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t) // R$ 1.000,00, 50%
	confirmed, err := pay(t, o, order.PaymentDeposit, 50_000, "2026-12-01")
	if err != nil || !confirmed {
		t.Fatalf("deposit: %v %v", confirmed, err)
	}
	s := changeSpec(t, o, "2026-12-02")
	s.CustomerApproved = true
	s.Terms.DepositPercent = 80
	s.Lines = append(s.Lines, testkit.LooseLine(t, testkit.ItemOpts{Price: 100_000, LeadDays: 3}))
	if _, err := o.Change(s); err != nil {
		t.Fatal(err)
	}
	// ADR 0030: a deposit already confirmed stays confirmed; a higher total
	// adds to the balance, not to the deposit.
	if !o.DepositPaid() || o.DepositDue().Cents() != 50_000 {
		t.Errorf("deposit paid %v due %v, want the confirmed R$ 500,00", o.DepositPaid(), o.DepositDue())
	}
	if o.BalanceDue().Cents() != 150_000 {
		t.Errorf("balance = %v, want R$ 1.500,00", o.BalanceDue())
	}
	if err := o.Advance("purchasing", nil, owner(t, o), date(t, "2026-12-02"), day1); err != nil {
		t.Errorf("advance after the change: %v", err)
	}
	again, err := pay(t, o, order.PaymentDeposit, 10_000, "2026-12-03")
	if err != nil || again {
		t.Errorf("a later payment confirmed the deposit again: %v %v", again, err)
	}
}

func TestChange_KeepsDatesOfSurvivingItems(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 5}).WithItem(testkit.ItemOpts{LeadDays: 5}).Build(t)
	if _, err := pay(t, o, order.PaymentDeposit, 100_000, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	redo, done := o.Items()[0].ID(), o.Items()[1].ID()
	by := owner(t, o)
	// The first item came back defective on 12-10: expected again 12-15.
	if err := o.MoveItem(redo, order.ItemReceived, &bad, date(t, "2026-12-10"), day1, by); err != nil {
		t.Fatal(err)
	}
	// The second arrived late, on 12-20.
	if err := o.MoveItem(done, order.ItemReceived, &ok, date(t, "2026-12-20"), day1, by); err != nil {
		t.Fatal(err)
	}
	s := changeSpec(t, o, "2026-12-11")
	s.CustomerApproved = true
	if _, err := o.Change(s); err != nil {
		t.Fatal(err)
	}
	if got := o.Items()[0].Expected(); got != date(t, "2026-12-15") {
		t.Errorf("redone item expected = %s, want 2026-12-15 kept", got)
	}
	// Its later date is a later forecast, so the promise moves with it
	// (ADR 0030): 12-15 + 3 + 2.
	if got := o.Promise(); got != date(t, "2026-12-20") {
		t.Errorf("promise = %s, want 2026-12-20", got)
	}
	if got := o.Alerts(date(t, "2026-12-16"))[redo]; len(got) == 0 || got[0] != schedule.AlertLate {
		t.Errorf("redone item alerts on 12-16 = %v, want late", got)
	}
}

func TestChange_NewItemAfterReleaseCountsFromToday(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 5}).Build(t)
	if _, err := pay(t, o, order.PaymentDeposit, 50_000, "2026-12-01"); err != nil {
		t.Fatal(err)
	}
	s := changeSpec(t, o, "2026-12-03")
	s.CustomerApproved = true
	s.Lines = append(s.Lines, testkit.LooseLine(t, testkit.ItemOpts{Price: 10_000, LeadDays: 20}))
	if _, err := o.Change(s); err != nil {
		t.Fatal(err)
	}
	if got := o.Items()[1].Expected(); got != date(t, "2026-12-23") {
		t.Errorf("new item expected = %s, want today + 20", got)
	}
	if got := o.Items()[0].Expected(); got != date(t, "2026-12-06") {
		t.Errorf("old item expected = %s, want release + 5 kept", got)
	}
	if got := o.Promise(); got != date(t, "2026-12-28") {
		t.Errorf("promise = %s, want 12-23 + 3 + 2", got)
	}
}

func TestAlerts_NoneBeforeTheRelease(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 2}).Build(t)
	// Weeks later, still waiting for the deposit: nothing was bought, so
	// nothing is late.
	if got := o.Alerts(date(t, "2027-01-10")); len(got) != 0 {
		t.Errorf("alerts before release = %v", got)
	}
}

func TestVoidedDeposit_OwnerCanReleaseOrClientCanPayAgain(t *testing.T) {
	t.Parallel()

	newVoided := func(t *testing.T) *order.Order {
		o := testkit.NewOrder().WithItem(testkit.ItemOpts{LeadDays: 5}).Build(t)
		if _, err := pay(t, o, order.PaymentDeposit, 50_000, "2026-12-01"); err != nil {
			t.Fatal(err)
		}
		if err := o.VoidPayment(o.Payments()[0].ID(), "Pix não caiu", owner(t, o), day1); err != nil {
			t.Fatal(err)
		}
		return o
	}
	t.Run("release", func(t *testing.T) {
		t.Parallel()
		o := newVoided(t)
		if err := o.Release(owner(t, o), date(t, "2026-12-04"), day1); err != nil {
			t.Fatalf("release after a void: %v", err)
		}
		if got, _ := o.ReleaseDate(); got != date(t, "2026-12-04") {
			t.Errorf("release date = %s, want the release day", got)
		}
		if err := o.Advance("purchasing", nil, owner(t, o), date(t, "2026-12-04"), day1); err != nil {
			t.Errorf("advance: %v", err)
		}
	})
	t.Run("pay again", func(t *testing.T) {
		t.Parallel()
		o := newVoided(t)
		confirmed, err := pay(t, o, order.PaymentDeposit, 50_000, "2026-12-05")
		if err != nil || !confirmed {
			t.Fatalf("pay again: %v %v", confirmed, err)
		}
		if got, _ := o.ReleaseDate(); got != date(t, "2026-12-05") {
			t.Errorf("release date = %s, want the day it was paid again", got)
		}
		if got := o.Promise(); got != date(t, "2026-12-15") {
			t.Errorf("promise = %s, want 12-05 + 5 + 3 + 2", got)
		}
	})
}

func TestNew_RefusesAmountsAboveTheCatalogBound(t *testing.T) {
	t.Parallel()

	huge := int64(100_000_000_001)
	edits := map[string]func(t *testing.T, s *order.NewSpec){
		"line price": func(t *testing.T, s *order.NewSpec) { s.Lines[0].Price = brl(t, 5_000_000_000_000_000_000) },
		"unit price": func(t *testing.T, s *order.NewSpec) { s.Lines[0].Items[0].Snapshot.UnitPrice = brl(t, huge) },
		"freight": func(t *testing.T, s *order.NewSpec) {
			s.Terms.Handover = order.HandoverDelivery
			s.Terms.Address = &order.DeliveryAddress{ID: shared.AddressID(testkit.NextID())}
			s.Terms.Freight = brl(t, huge)
		},
		"cost": func(t *testing.T, s *order.NewSpec) {
			c := brl(t, huge)
			s.Lines[0].Items[0].Sourcing.Cost = &c
		},
		"negative cost": func(t *testing.T, s *order.NewSpec) {
			c := brl(t, -1)
			s.Lines[0].Items[0].Sourcing.Cost = &c
		},
		"cost currency": func(t *testing.T, s *order.NewSpec) {
			c := usd(t, 1)
			s.Lines[0].Items[0].Sourcing.Cost = &c
		},
		"backup lead days": func(_ *testing.T, s *order.NewSpec) {
			s.Lines[0].Items[0].Sourcing.Backup = &order.BackupSource{Supplier: shared.SupplierID(testkit.NextID()), LeadDays: 400}
		},
	}
	for name, edit := range edits {
		s := testkit.NewOrder().Spec(t)
		edit(t, &s)
		if _, err := order.New(s); !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestNew_RefusesAnOrderWithNoCurrency(t *testing.T) {
	t.Parallel()

	s := testkit.NewOrder().Spec(t)
	s.Terms.Freight, s.Terms.Discount = shared.Money{}, shared.Money{}
	if _, err := order.New(s); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestMoves_RefuseAZeroTodayOrPrincipal(t *testing.T) {
	t.Parallel()

	o := testkit.NewOrder().Build(t)
	by := owner(t, o)
	zero := shared.CivilDate{}
	if err := o.Release(shared.Principal{}, date(t, "2026-12-02"), day1); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("release without a user: err = %v", err)
	}
	if err := o.Release(by, zero, day1); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("release without today: err = %v", err)
	}
	if err := o.Advance("awaiting_deposit", nil, by, zero, day1); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("advance without today: err = %v", err)
	}
	if err := o.MoveItem(o.Items()[0].ID(), order.ItemSentToSupplier, nil, zero, day1, by); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("move without today: err = %v", err)
	}
	if _, err := o.ChangeDeliveryDate(date(t, "2026-12-20"), "", by, zero, day1); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("date without today: err = %v", err)
	}
	s := changeSpec(t, o, "2026-12-02")
	s.Today = zero
	if _, err := o.Change(s); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("change without today: err = %v", err)
	}
	if err := o.Cancel("x", by, time.Time{}); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("cancel without a time: err = %v", err)
	}

}
