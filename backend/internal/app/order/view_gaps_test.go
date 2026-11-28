package order_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

var pickup = apporder.TermsInput{Handover: order.HandoverPickup}

func fieldsNamed(t *testing.T, err error) map[string]string {
	t.Helper()
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a ValidationError", err)
	}
	out := map[string]string{}
	for _, f := range ve.Fields {
		out[f.Field] = f.Message
	}
	return out
}

func goodNewAddress() *order.DeliveryAddress {
	return &order.DeliveryAddress{Label: "Obra", CEP: "69000000", Street: "Rua Nova", Number: "5", City: "Manaus", State: "AM"}
}

func TestQuote_NewOrderCountsDaysFromTodayAndOwesNoRefund(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	q, err := h.svc.Quote(h.staff(t), apporder.QuoteInput{Lines: []apporder.LineRequest{h.bancoLine()}, Terms: pickup})
	if err != nil {
		t.Fatal(err)
	}
	// 15 + 3 + 2 days after the deposit day, which a new order's quote assumes is today.
	if q.DaysAfterDeposit != 20 || !q.RefundDue.IsZero() {
		t.Errorf("days after deposit %d refund %v, want 20 and zero", q.DaysAfterDeposit, q.RefundDue)
	}
	if q.DaysAfterDeposit != testkit.Date(t, "2026-12-01").DaysUntil(q.DeliveryDate) {
		t.Errorf("days %d do not match the delivery date %s", q.DaysAfterDeposit, q.DeliveryDate)
	}
}

func TestQuote_ChangeOfAPaidOrderShowsTheRefundAndCountsFromTheReleaseDay(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.pay(h.staff(t), t, h.create(h.staff(t), t, nil).Order, order.PaymentDeposit, 50_000)
	h.clock.Advance(4 * 24 * time.Hour) // today is 12-05; the deposit released the order on 12-01
	terms := pickup
	terms.DiscountCents = ptr[int64](80_000)
	q, err := h.svc.Quote(h.admin(t), apporder.QuoteInput{Order: v.ID, Lines: []apporder.LineRequest{h.bancoLine()}, Terms: terms})
	if err != nil {
		t.Fatal(err)
	}
	// Total R$ 200,00 against R$ 500,00 paid.
	if q.Total.Cents() != 20_000 || q.RefundDue.Cents() != 30_000 {
		t.Errorf("total %v refund %v, want R$ 200,00 and R$ 300,00 back", q.Total, q.RefundDue)
	}
	if q.DaysAfterDeposit != 20 {
		t.Errorf("days after deposit = %d, want 20 from the release date, not 16 from today", q.DaysAfterDeposit)
	}
}

func TestQuote_CostAndMarginAreTheOwnersOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	in := apporder.QuoteInput{Lines: []apporder.LineRequest{h.bancoLine()}, Terms: pickup}
	q, err := h.svc.Quote(h.owner(t), in)
	if err != nil {
		t.Fatal(err)
	}
	if q.Cost == nil || q.Cost.Cents() != 40_000 || q.Margin == nil {
		t.Errorf("owner's quote: cost %v margin %v, want R$ 400,00 and a margin", q.Cost, q.Margin)
	}
	for name, ctx := range map[string]func(*testing.T) *apporder.QuoteView{
		"admin": func(t *testing.T) *apporder.QuoteView { return quoteAs(h.admin(t), t, h, in) },
		"staff": func(t *testing.T) *apporder.QuoteView { return quoteAs(h.staff(t), t, h, in) },
	} {
		if got := ctx(t); got.Cost != nil || got.Margin != nil {
			t.Errorf("%s's quote carries cost %v margin %v", name, got.Cost, got.Margin)
		}
	}
	// An owner's change quote has them too, and nobody else's.
	v := h.create(h.owner(t), t, nil).Order
	change := apporder.QuoteInput{Order: v.ID, Lines: in.Lines, Terms: pickup}
	if got := quoteAs(h.owner(t), t, h, change); got.Cost == nil {
		t.Error("owner's change quote has no cost")
	}
	if got := quoteAs(h.admin(t), t, h, change); got.Cost != nil || got.Margin != nil {
		t.Error("admin's change quote carries a cost or margin")
	}
}

func quoteAs(ctx context.Context, t *testing.T, h *harness, in apporder.QuoteInput) *apporder.QuoteView {
	t.Helper()
	q, err := h.svc.Quote(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	return &q
}

func TestView_DepositOwedAndTheCustomersChatLink(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	if v.DepositOwed.Cents() != 50_000 || v.DepositDue.Cents() != 50_000 {
		t.Fatalf("new order: owed %v due %v", v.DepositOwed, v.DepositDue)
	}
	if v.Customer.WhatsAppURL != "https://wa.me/5592981234567" {
		t.Errorf("customer link = %q, want the chat with no text", v.Customer.WhatsAppURL)
	}
	paid := h.pay(h.staff(t), t, v, order.PaymentDeposit, 20_000)
	if paid.DepositOwed.Cents() != 30_000 || paid.DepositDue.Cents() != 50_000 {
		t.Errorf("after R$ 200,00: owed %v due %v, want R$ 300,00 and the full R$ 500,00", paid.DepositOwed, paid.DepositDue)
	}
	got, err := h.svc.Get(h.viewer(t), v.ID)
	if err != nil || got.Customer.WhatsAppURL == "" || strings.Contains(got.Customer.WhatsAppURL, "?") {
		t.Errorf("read: %v link %q", err, got.Customer.WhatsAppURL)
	}
}

func TestCreate_NewAddressIsAddedToTheCustomerAndUsedForDelivery(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, func(in *apporder.CreateInput) {
		in.Terms = apporder.TermsInput{Handover: order.HandoverDelivery, NewAddress: goodNewAddress()}
	}).Order
	got, err := h.customers.Get(h.staff(t), h.joao.ID)
	if err != nil || len(got.Addresses) != 2 {
		t.Fatalf("customer = %+v %v, want the new address added after the old", got, err)
	}
	added := got.Addresses[1]
	if added.ID.IsZero() || added.Street != "Rua Nova" || v.Terms.Address == nil || v.Terms.Address.ID != added.ID {
		t.Errorf("order address %+v, customer's new one %+v", v.Terms.Address, added)
	}
}

func TestCreate_NewAddressForANewCustomerGoesToThatCustomer(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, func(in *apporder.CreateInput) {
		in.Customer, in.New = shared.CustomerID{}, &apporder.NewCustomer{Name: "Maria", Phone: "5592981230000"}
		in.Terms = apporder.TermsInput{Handover: order.HandoverDelivery, NewAddress: goodNewAddress()}
	}).Order
	got, err := h.customers.Get(h.staff(t), v.Customer.ID)
	if err != nil || len(got.Addresses) != 1 || v.Terms.Address == nil || v.Terms.Address.ID != got.Addresses[0].ID {
		t.Errorf("customer %+v err %v order address %+v", got, err, v.Terms.Address)
	}
}

func TestCreate_NewAddressRefusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	bad := goodNewAddress()
	bad.CEP, bad.Street = "12", ""
	tests := []struct {
		name  string
		terms apporder.TermsInput
		field string
	}{
		{"with an addressId", apporder.TermsInput{Handover: order.HandoverDelivery, Address: h.joao.Addresses[0].ID, NewAddress: goodNewAddress()}, "terms.newAddress"},
		{"on a pickup", apporder.TermsInput{Handover: order.HandoverPickup, NewAddress: goodNewAddress()}, "terms.newAddress"},
		{"invalid cep", apporder.TermsInput{Handover: order.HandoverDelivery, NewAddress: bad}, "terms.newAddress.cep"},
		{"invalid street", apporder.TermsInput{Handover: order.HandoverDelivery, NewAddress: bad}, "terms.newAddress.street"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.svc.Create(h.staff(t), apporder.CreateInput{Customer: h.joao.ID, Lines: []apporder.LineRequest{h.bancoLine()},
				Terms: tt.terms, Idempotency: h.key()})
			if f := fieldsNamed(t, err); f[tt.field] == "" {
				t.Errorf("fields = %v, want %s", f, tt.field)
			}
		})
	}
	if got, _ := h.customers.Get(h.staff(t), h.joao.ID); len(got.Addresses) != 1 {
		t.Errorf("a refused order left %d addresses on the customer", len(got.Addresses))
	}
}

func TestCreate_NewAddressUndoneWhenTheOrderIsRefusedLater(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, err := h.svc.Create(h.staff(t), apporder.CreateInput{Customer: h.joao.ID, Lines: []apporder.LineRequest{{Item: shared.ItemID(h.ids.NewID()), Quantity: 1}},
		Terms: apporder.TermsInput{Handover: order.HandoverDelivery, NewAddress: goodNewAddress()}, Idempotency: h.key()})
	if err == nil {
		t.Fatal("an order with a bad line was made")
	}
	if got, _ := h.customers.Get(h.staff(t), h.joao.ID); len(got.Addresses) != 1 {
		t.Errorf("customer has %d addresses, want the new one rolled back with the order", len(got.Addresses))
	}
}

func TestCreate_NewAddressIsAddedOnceWhenTheCreateIsReplayed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	in := apporder.CreateInput{Customer: h.joao.ID, Lines: []apporder.LineRequest{h.bancoLine()},
		Terms: apporder.TermsInput{Handover: order.HandoverDelivery, NewAddress: goodNewAddress()}, Idempotency: h.key()}
	first, err := h.svc.Create(h.staff(t), in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.svc.Create(h.staff(t), in)
	if err != nil || again.Order.ID != first.Order.ID {
		t.Fatalf("replay = %v %v, want the first order", again.Order.ID, err)
	}
	if got, _ := h.customers.Get(h.staff(t), h.joao.ID); len(got.Addresses) != 2 {
		t.Errorf("customer has %d addresses after a replay, want the old one and the new one", len(got.Addresses))
	}
}

func TestCreate_NewAddressNeedsOnlyOrderWrite(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	in := apporder.CreateInput{Customer: h.joao.ID, Lines: []apporder.LineRequest{h.bancoLine()},
		Terms: apporder.TermsInput{Handover: order.HandoverDelivery, NewAddress: goodNewAddress()}, Idempotency: h.key()}
	if _, err := h.svc.Create(h.viewer(t), in); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("viewer: err = %v, want forbidden", err)
	}
	if _, err := h.svc.Create(h.staff(t), in); err != nil {
		t.Errorf("staff: %v", err)
	}
}

func TestQuote_NewAddressIsUsedWithoutBeingSaved(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	in := apporder.QuoteInput{Lines: []apporder.LineRequest{h.bancoLine()},
		Terms: apporder.TermsInput{Handover: order.HandoverDelivery, NewAddress: goodNewAddress(), FreightCents: ptr[int64](2_000)}}
	q, err := h.svc.Quote(h.staff(t), in)
	if err != nil || q.Total.Cents() != 102_000 {
		t.Fatalf("quote = %+v %v", q, err)
	}
	v := h.create(h.staff(t), t, nil).Order
	in.Order = v.ID
	if _, err := h.svc.Quote(h.staff(t), in); err != nil {
		t.Fatalf("change quote: %v", err)
	}
	if got, _ := h.customers.Get(h.staff(t), h.joao.ID); len(got.Addresses) != 1 {
		t.Errorf("a quote saved an address: %d on the customer", len(got.Addresses))
	}
	in.Terms.NewAddress = &order.DeliveryAddress{Label: "x"}
	if _, err := h.svc.Quote(h.staff(t), in); fieldsNamed(t, err)["terms.newAddress.cep"] == "" {
		t.Errorf("an invalid new address: err = %v, want the quote to refuse as Create would", err)
	}
}

func TestQuote_NewAddressPastTenIsRefusedLikeTheSave(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	full := h.joao
	full.Addresses = nil
	for range 10 {
		full.Addresses = append(full.Addresses, order.DeliveryAddress{ID: shared.AddressID(h.ids.NewID()), Label: "x", CEP: "69000000", Street: "R", City: "Manaus", State: "AM"})
	}
	h.customers.Add(full)
	v := h.create(h.staff(t), t, nil).Order
	terms := apporder.TermsInput{Handover: order.HandoverDelivery, NewAddress: goodNewAddress()}
	_, quoteErr := h.svc.Quote(h.staff(t), apporder.QuoteInput{Order: v.ID, Lines: []apporder.LineRequest{h.bancoLine()}, Terms: terms})
	_, changeErr := h.svc.Change(h.staff(t), apporder.ChangeInput{Order: v.ID, Version: v.Version, Lines: []apporder.LineRequest{h.bancoLine()}, Terms: terms})
	for name, err := range map[string]error{"quote": quoteErr, "change": changeErr} {
		if f := fieldsNamed(t, err); f["terms.newAddress"] != "Use até 10 endereços." {
			t.Errorf("%s: fields = %v, want the limit message under terms.newAddress", name, f)
		}
	}
}

func TestChange_NewAddressReplacesThePickupAndIsSaved(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	got, err := h.svc.Change(h.staff(t), apporder.ChangeInput{Order: v.ID, Version: v.Version, Lines: []apporder.LineRequest{h.bancoLine()},
		Terms: apporder.TermsInput{Handover: order.HandoverDelivery, NewAddress: goodNewAddress()}})
	if err != nil {
		t.Fatal(err)
	}
	cust, _ := h.customers.Get(h.staff(t), h.joao.ID)
	if got.Order.Terms.Address == nil || len(cust.Addresses) != 2 || got.Order.Terms.Address.ID != cust.Addresses[1].ID {
		t.Errorf("order address %+v, customer %+v", got.Order.Terms.Address, cust.Addresses)
	}
}
