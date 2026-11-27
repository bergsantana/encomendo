package order_test

import (
	"errors"
	"testing"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/app/order/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

var errDown = errors.New("postgres is down")

// Every use case returns its repository's failure, wrapped, and saves
// nothing.
func TestUseCases_ReturnTheRepositorysFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.owner(t), t, nil).Order
	h.repo.Fail(errDown)
	calls := map[string]func() error{
		"get": func() error { _, err := h.svc.Get(h.owner(t), v.ID); return err },
		"create": func() error {
			_, err := h.svc.Create(h.owner(t), apporder.CreateInput{Customer: h.joao.ID, Lines: []apporder.LineRequest{h.bancoLine()},
				Terms: apporder.TermsInput{Handover: order.HandoverPickup}, Idempotency: h.key()})
			return err
		},
		"change": func() error {
			_, err := h.svc.Change(h.owner(t), apporder.ChangeInput{Order: v.ID, Version: v.Version, Lines: []apporder.LineRequest{h.bancoLine()}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
			return err
		},
		"payment": func() error {
			_, err := h.svc.RecordPayment(h.owner(t), apporder.PaymentInput{Order: v.ID, Kind: order.PaymentDeposit, AmountCents: 1, Method: order.MethodPix, Version: v.Version, Idempotency: h.key()})
			return err
		},
		"void": func() error {
			_, err := h.svc.VoidPayment(h.owner(t), apporder.VoidPaymentInput{Order: v.ID, Version: v.Version})
			return err
		},
		"release": func() error {
			_, err := h.svc.Release(h.owner(t), apporder.VersionInput{Order: v.ID, Version: v.Version})
			return err
		},
		"date": func() error {
			_, err := h.svc.ChangeDeliveryDate(h.owner(t), apporder.DateInput{Order: v.ID, Date: testkit.Date(t, "2026-12-30"), Version: v.Version})
			return err
		},
		"link": func() error { _, err := h.svc.SendTrackingLink(h.owner(t), v.ID); return err },
		"cancel": func() error {
			_, err := h.svc.Cancel(h.owner(t), apporder.CancelInput{Order: v.ID, Reason: "x", Version: v.Version})
			return err
		},
		"quote": func() error {
			_, err := h.svc.Quote(h.owner(t), apporder.QuoteInput{Order: v.ID, Lines: []apporder.LineRequest{h.bancoLine()}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
			return err
		},
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, errDown) {
			t.Errorf("%s: err = %v, want the repository's failure", name, err)
		}
	}
	if n := len(h.events.Names()); n != 1 {
		t.Errorf("%d events after failures, want only the first order's", n)
	}
}

func TestGetAndWrites_UnknownOrder(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	id := shared.OrderID(h.ids.NewID())
	if _, err := h.svc.Get(h.staff(t), id); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("get: err = %v", err)
	}
	if _, err := h.svc.MoveItem(h.staff(t), apporder.MoveItemInput{Order: id, To: order.ItemSentToSupplier, Version: 1}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("move: err = %v", err)
	}
	if _, err := h.svc.Get(h.viewer(t), id); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("viewer get: err = %v, want not found after the permission", err)
	}
}

func (h *harness) kit(t *testing.T) shared.KitID {
	t.Helper()
	guidao := shared.ItemID(h.ids.NewID())
	h.catalog.AddItem(fake.CatalogItem{ID: guidao, Name: "Guidão", Mode: catalog.ModeInHouse, Price: brl(t, 80_000), LeadDays: 5})
	id := shared.KitID(h.ids.NewID())
	days := 6
	h.catalog.AddKit(fake.CatalogKit{ID: id, Name: "Kit Scrambler", Price: brl(t, 160_000),
		Items: map[shared.ItemID]int{h.banco: 1, guidao: 1}, Order: []shared.ItemID{h.banco, guidao}, Assembly: &days})
	return id
}

func TestKitLine_TakesTheKitsDaysAndSurvivesAnUnchangedChange(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	kit := h.kit(t)
	v := h.create(h.staff(t), t, func(in *apporder.CreateInput) { in.Lines = []apporder.LineRequest{{Kit: kit}} }).Order
	// The kit's own 6 assembly days: 12-01 + 15 + 6 + 2.
	if v.Terms.AssemblyDays != 6 || v.DeliveryDate != testkit.Date(t, "2026-12-24") || len(v.Items) != 2 {
		t.Fatalf("kit order = days %d delivery %s items %d", v.Terms.AssemblyDays, v.DeliveryDate, len(v.Items))
	}
	moved, err := h.svc.MoveItem(h.staff(t), apporder.MoveItemInput{Order: v.ID, Item: v.Items[1].ID, To: order.ItemInProduction, Version: v.Version})
	if err != nil {
		t.Fatal(err)
	}
	h.catalog.AddKit(fake.CatalogKit{ID: kit, Name: "Kit novo", Price: brl(t, 999_000)})
	got, err := h.svc.Change(h.staff(t), apporder.ChangeInput{Order: v.ID, Version: moved.Version,
		Lines: []apporder.LineRequest{{Kit: kit}}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Order.Subtotal.Cents() != 160_000 || got.Order.Items[1].Status != order.ItemInProduction || got.Order.Lines[0].ID != v.Lines[0].ID {
		t.Errorf("after an unchanged change: subtotal %v, item %s", got.Order.Subtotal, got.Order.Items[1].Status)
	}
}

func TestQuote_PaidOrderNeedsManage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.pay(h.staff(t), t, h.create(h.staff(t), t, nil).Order, order.PaymentDeposit, 50_000)
	in := apporder.QuoteInput{Order: v.ID, Lines: []apporder.LineRequest{h.bancoLine()}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}}
	if _, err := h.svc.Quote(h.staff(t), in); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("staff quoting a paid order's change: err = %v", err)
	}
	if _, err := h.svc.Quote(h.admin(t), in); err != nil {
		t.Errorf("admin: %v", err)
	}
	if _, err := h.svc.Quote(h.viewer(t), in); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("viewer: err = %v", err)
	}
}

func TestQuote_DeliveryNeedsOnlyANamedAddress(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	in := apporder.QuoteInput{Lines: []apporder.LineRequest{h.bancoLine()},
		Terms: apporder.TermsInput{Handover: order.HandoverDelivery, Address: shared.AddressID(h.ids.NewID()), FreightCents: 2_000}}
	q, err := h.svc.Quote(h.staff(t), in)
	if err != nil || q.Freight.Cents() != 2_000 || q.Total.Cents() != 102_000 {
		t.Errorf("quote = %+v %v", q, err)
	}
	in.Terms.Address = shared.AddressID{}
	if _, err := h.svc.Quote(h.staff(t), in); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("no address: err = %v", err)
	}
}

func TestChange_KeepsTheOrdersOwnAddressCopy(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	addr := h.joao.Addresses[0].ID
	v := h.create(h.staff(t), t, func(in *apporder.CreateInput) {
		in.Terms = apporder.TermsInput{Handover: order.HandoverDelivery, Address: addr}
	}).Order
	// The customer's address is removed; the order still delivers there.
	h.customers.Add(apporder.CustomerFacts{ID: h.joao.ID, Name: h.joao.Name, Phone: h.joao.Phone})
	got, err := h.svc.Change(h.staff(t), apporder.ChangeInput{Order: v.ID, Version: v.Version,
		Lines: []apporder.LineRequest{{Item: h.banco, Quantity: 2}}, Terms: apporder.TermsInput{Handover: order.HandoverDelivery, Address: addr}})
	if err != nil || got.Order.Terms.Address == nil || got.Order.Terms.Address.City != "Manaus" {
		t.Errorf("change: %v address %+v", err, got.Order.Terms.Address)
	}
}
