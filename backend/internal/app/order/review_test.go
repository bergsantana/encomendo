package order_test

import (
	"errors"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/app/order/fake"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// noCosts fails unless v carries no cost and no margin anywhere.
func noCosts(t *testing.T, name string, v apporder.View) {
	t.Helper()
	if v.Cost != nil || v.Margin != nil {
		t.Errorf("%s: cost %v margin %v reach a non-owner", name, v.Cost, v.Margin)
	}
	for _, i := range v.Items {
		if i.Cost != nil {
			t.Errorf("%s: item cost %v reaches a non-owner", name, i.Cost)
		}
	}
}

// Every operation that answers an order, replays included, leaves costs and
// margins out for every role but the owner (ADR 0030, Amendments).
func TestViews_EveryAnswerHidesCostsFromNonOwners(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	in := apporder.CreateInput{Customer: h.joao.ID, Lines: []apporder.LineRequest{h.bancoLine()},
		Terms: apporder.TermsInput{Handover: order.HandoverPickup}, Idempotency: h.key()}
	created, err := h.svc.Create(h.staff(t), in)
	if err != nil {
		t.Fatal(err)
	}
	noCosts(t, "create", created.Order)
	replay, err := h.svc.Create(h.admin(t), in)
	if err != nil {
		t.Fatal(err)
	}
	noCosts(t, "create replay", replay.Order)
	v := created.Order
	pin := apporder.PaymentInput{Order: v.ID, Kind: order.PaymentDeposit, AmountCents: 50_000, Method: order.MethodPix, Version: v.Version, Idempotency: h.key()}
	paid, err := h.svc.RecordPayment(h.staff(t), pin)
	if err != nil {
		t.Fatal(err)
	}
	noCosts(t, "payment", paid)
	again, err := h.svc.RecordPayment(h.staff(t), pin)
	if err != nil {
		t.Fatal(err)
	}
	noCosts(t, "payment replay", again)
	steps := []struct {
		name string
		run  func(version int) (apporder.View, error)
	}{
		{"change", func(ver int) (apporder.View, error) {
			c, err := h.svc.Change(h.admin(t), apporder.ChangeInput{Order: v.ID, Version: ver, CustomerApproved: true,
				Lines: []apporder.LineRequest{{Item: h.banco, Quantity: 2}}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
			return c.Order, err
		}},
		{"advance", func(ver int) (apporder.View, error) {
			return h.svc.Advance(h.staff(t), apporder.AdvanceInput{Order: v.ID, To: "purchasing", Version: ver})
		}},
		{"move item", func(ver int) (apporder.View, error) {
			sent := order.PurchaseMove{Item: v.Items[0].ID, To: order.ItemSentToSupplier, Ref: "C-0001"}
			if err := h.svc.MovePurchased(h.admin(t), []apporder.PurchaseMoveInput{{Order: v.ID, Move: sent}}); err != nil {
				return apporder.View{}, err
			}
			return h.svc.Get(h.staff(t), v.ID)
		}},
		{"date", func(ver int) (apporder.View, error) {
			m, err := h.svc.ChangeDeliveryDate(h.admin(t), apporder.DateInput{Order: v.ID, Date: testkit.Date(t, "2027-01-10"), Version: ver})
			return m.Order, err
		}},
		{"cancel", func(ver int) (apporder.View, error) {
			return h.svc.Cancel(h.admin(t), apporder.CancelInput{Order: v.ID, Reason: "x", Version: ver})
		}},
	}
	ver := paid.Version
	for _, st := range steps {
		got, err := st.run(ver)
		if err != nil {
			t.Fatalf("%s: %v", st.name, err)
		}
		noCosts(t, st.name, got)
		ver = got.Version
	}
	backupOrder := h.create(h.admin(t), t, nil).Order
	switched, err := h.svc.UseBackupSupplier(h.admin(t), apporder.ItemVersionInput{Order: backupOrder.ID, Item: backupOrder.Items[0].ID, Version: backupOrder.Version})
	if err != nil {
		t.Fatal(err)
	}
	noCosts(t, "backup supplier", switched)
}

func TestRecordPayment_RefusalsAndReplayOnAnotherOrder(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a := h.create(h.staff(t), t, nil).Order
	b := h.create(h.staff(t), t, nil).Order
	in := apporder.PaymentInput{Order: a.ID, Kind: order.PaymentDeposit, AmountCents: 50_000, Method: order.MethodPix, Version: a.Version, Idempotency: h.key()}
	if _, err := h.svc.RecordPayment(h.staff(t), in); err != nil {
		t.Fatal(err)
	}
	other := in
	other.Order, other.Version = b.ID, b.Version
	if _, err := h.svc.RecordPayment(h.staff(t), other); !errors.Is(err, appshared.ErrIdempotencyKeyReused) {
		t.Errorf("a's key on b: err = %v, want ErrIdempotencyKeyReused", err)
	}
	cases := map[string]struct {
		edit func(*apporder.PaymentInput)
		want error
	}{
		"stale version":        {func(p *apporder.PaymentInput) { p.Version = a.Version }, shared.ErrVersionConflict},
		"above the balance":    {func(p *apporder.PaymentInput) { p.Version, p.AmountCents = a.Version+1, 60_000 }, order.ErrPaymentExceedsDue},
		"same key, other body": {func(p *apporder.PaymentInput) { p.Idempotency.RequestHash[0]++ }, appshared.ErrIdempotencyKeyReused},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := in
			p.Idempotency = h.key()
			if name == "same key, other body" {
				p.Idempotency = in.Idempotency
			}
			tc.edit(&p)
			if _, err := h.svc.RecordPayment(h.staff(t), p); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
	paymentKey := apporder.CreateInput{Customer: h.joao.ID, Lines: []apporder.LineRequest{h.bancoLine()},
		Terms: apporder.TermsInput{Handover: order.HandoverPickup}, Idempotency: in.Idempotency}
	if _, err := h.svc.Create(h.staff(t), paymentKey); !errors.Is(err, appshared.ErrIdempotencyKeyReused) {
		t.Errorf("a payment's key on create: err = %v, want ErrIdempotencyKeyReused", err)
	}
	cancelled, err := h.svc.Cancel(h.admin(t), apporder.CancelInput{Order: b.ID, Reason: "x", Version: b.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.RecordPayment(h.staff(t), apporder.PaymentInput{Order: b.ID, Kind: order.PaymentBalance, AmountCents: 1,
		Method: order.MethodCash, Version: cancelled.Version, Idempotency: h.key()}); !errors.Is(err, order.ErrOrderClosed) {
		t.Errorf("payment on a cancelled order: err = %v", err)
	}
}

// At 01:00 UTC it is still the day before in São Paulo: every date counts
// from the store's day, not the server's (ADR 0030).
func TestDates_FollowTheStoresDay(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.clock.Set(time.Date(2026, 12, 2, 1, 0, 0, 0, time.UTC)) // 22:00 on 12-01 in São Paulo
	v := h.create(h.staff(t), t, nil).Order
	if v.DeliveryDate != testkit.Date(t, "2026-12-21") {
		t.Errorf("delivery = %s, want 12-01 + 20 in the store's day", v.DeliveryDate)
	}
	paid := h.pay(h.staff(t), t, v, order.PaymentDeposit, 50_000)
	if paid.ReleaseDate == nil || *paid.ReleaseDate != testkit.Date(t, "2026-12-01") {
		t.Errorf("release = %v, want the store's 12-01", paid.ReleaseDate)
	}
	m := newHarnessIn(t, "America/Manaus")
	m.clock.Set(time.Date(2026, 12, 2, 3, 30, 0, 0, time.UTC)) // 23:30 on 12-01 in Manaus, 00:30 on 12-02 in São Paulo
	mv := m.create(m.staff(t), t, nil).Order
	if mv.DeliveryDate != testkit.Date(t, "2026-12-21") {
		t.Errorf("Manaus delivery = %s, want 12-01 + 20", mv.DeliveryDate)
	}
}

func TestCreate_RefusalLeavesNoCustomerAndNoNumber(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	before := h.customers.Count()
	_, err := h.svc.Create(h.staff(t), apporder.CreateInput{New: &apporder.NewCustomer{Name: "Nova", Phone: "5592970000000"},
		Lines: []apporder.LineRequest{{Item: shared.ItemID(h.ids.NewID()), Quantity: 1}}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}, Idempotency: h.key()})
	if err == nil {
		t.Fatal("create with an unknown item succeeded")
	}
	if h.customers.Count() != before {
		t.Error("a refused create kept the customer it made")
	}
	if got := h.create(h.staff(t), t, nil).Order.Number; got != "P-0001" {
		t.Errorf("next order = %s, want P-0001: a refused create took a number", got)
	}
}

func TestLines_KitOrItemExactly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	kit := h.kit(t)
	for name, l := range map[string]apporder.LineRequest{
		"kit and item":     {Kit: kit, Item: h.banco},
		"kit and quantity": {Kit: kit, Quantity: 2},
		"neither":          {Quantity: 1},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.svc.Quote(h.staff(t), apporder.QuoteInput{Lines: []apporder.LineRequest{l}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
			if !errors.Is(err, shared.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestChange_ChangedKitKeepsItsItemsAndGivesANewOneAFreshID(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	kit := h.kit(t)
	v := h.create(h.staff(t), t, func(in *apporder.CreateInput) { in.Lines = []apporder.LineRequest{{Kit: kit}} }).Order
	extra := shared.ItemID(h.ids.NewID())
	h.catalog.AddItem(fake.CatalogItem{ID: extra, Name: "Retrovisor", Mode: v.Items[1].SourcingMode, Price: brl(t, 5_000), LeadDays: 2})
	h.catalog.AddKit(fake.CatalogKit{ID: kit, Name: "Kit Scrambler", Price: brl(t, 165_000),
		Items: map[shared.ItemID]int{h.banco: 1, v.Items[1].CatalogItem: 1, extra: 1}, Order: []shared.ItemID{h.banco, v.Items[1].CatalogItem, extra}})
	got, err := h.svc.Change(h.staff(t), apporder.ChangeInput{Order: v.ID, Version: v.Version,
		Lines: []apporder.LineRequest{{Kit: kit, Options: []apporder.OptionChoice{h.red}}}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
	if err != nil {
		t.Fatal(err)
	}
	items := got.Order.Items
	if len(items) != 3 || items[0].ID != v.Items[0].ID || items[1].ID != v.Items[1].ID || items[2].ID == v.Items[0].ID || items[2].ID == v.Items[1].ID {
		t.Errorf("items = %+v, want the two kept ids and a new one", items)
	}
	// 1.650,00 + the banco's option 50,00 × 1.
	if got.Order.Subtotal.Cents() != 170_000 || len(got.Summary.Added) != 1 {
		t.Errorf("subtotal %v added %v", got.Order.Subtotal, got.Summary.Added)
	}
}

func TestTrackingView_CancelledShowsOneStep(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	if _, err := h.svc.Cancel(h.admin(t), apporder.CancelInput{Order: v.ID, Reason: "x", Version: v.Version}); err != nil {
		t.Fatal(err)
	}
	rec, _ := h.repo.Stored(h.store, v.ID)
	o, err := order.Restore(rec, testkit.Preset(t, testkit.PresetEncomendaMontagem))
	if err != nil {
		t.Fatal(err)
	}
	tv := apporder.NewTrackingView(o, "Alta RPM")
	if !tv.Cancelled || len(tv.Steps) != 1 || tv.Steps[0].Label != "Pedido cancelado" || !tv.Steps[0].Current {
		t.Errorf("tracking view = %+v", tv)
	}
}

// A change that leaves the freight out keeps the order's own; a switch to
// pickup clears it (go-reviewer and security review of 4.14).
func TestChange_FreightLeftOutIsKept(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	addr := h.joao.Addresses[0].ID
	v := h.create(h.staff(t), t, func(in *apporder.CreateInput) {
		in.Terms = apporder.TermsInput{Handover: order.HandoverDelivery, Address: addr, FreightCents: ptr[int64](3_000)}
	}).Order
	got, err := h.svc.Change(h.staff(t), apporder.ChangeInput{Order: v.ID, Version: v.Version,
		Lines: []apporder.LineRequest{{Item: h.banco, Quantity: 2}}, Terms: apporder.TermsInput{Handover: order.HandoverDelivery, Address: addr}})
	if err != nil || got.Order.Freight.Cents() != 3_000 {
		t.Fatalf("freight left out: %v freight %v, want 3000 kept", err, got.Order.Freight)
	}
	pickup, err := h.svc.Change(h.staff(t), apporder.ChangeInput{Order: v.ID, Version: got.Order.Version,
		Lines: []apporder.LineRequest{{Item: h.banco, Quantity: 2}}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
	if err != nil || !pickup.Order.Freight.IsZero() {
		t.Errorf("switch to pickup: %v freight %v, want zero", err, pickup.Order.Freight)
	}
}
