package order_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/app/order/fake"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

var start = time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC) // 12:00 in São Paulo

var _ apporder.UseCases = (*apporder.Service)(nil)

type harness struct {
	svc       *apporder.Service
	repo      *fake.Orders
	catalog   *fake.Catalog
	customers *fake.Customers
	workflows *fake.Workflows
	events    *sharedfake.Events
	tx        *sharedfake.TxManager
	clock     *sharedfake.Clock
	ids       *sharedfake.IDs
	store     shared.TenantID
	// banco is a to_order item (R$ 1.000,00, 15 days, cost R$ 400,00) with a
	// backup supplier; joao is a customer with an address.
	banco  shared.ItemID
	red    apporder.OptionChoice
	joao   apporder.CustomerFacts
	keySeq int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessIn(t, "America/Sao_Paulo")
}

// newHarnessIn is newHarness for a store in the named time zone.
func newHarnessIn(t *testing.T, zone string) *harness {
	t.Helper()
	clock := sharedfake.NewClock(start)
	ids := sharedfake.NewIDs(clock)
	h := &harness{
		repo: fake.NewOrders(), catalog: fake.NewCatalog(), customers: fake.NewCustomers(ids.NewID),
		workflows: fake.NewWorkflows(testkit.Preset(t, testkit.PresetEncomendaMontagem)),
		events:    sharedfake.NewEvents(), tx: sharedfake.NewTxManager(), clock: clock, ids: ids,
		store: shared.TenantID(ids.NewID()),
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := apporder.NewService(apporder.Deps{
		Repo: h.repo, Catalog: h.catalog, Customers: h.customers, Workflows: h.workflows,
		Store: fake.NewStore(apporder.StoreFacts{
			Name: "Alta RPM", Slug: "altarpm", WhatsApp: "5592991234567", Currency: "BRL",
			DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2, Location: loc,
		}),
		Tracking: fake.Tracking{}, WhatsApp: fake.WhatsApp{},
		Idempotency: sharedfake.NewIdempotency(clock), Secrets: sharedfake.NewSecrets(),
		Tx: h.tx, Events: h.events, Clock: clock, IDs: ids,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.svc = svc
	h.repo.Link(h.workflows, h.customers)
	h.seedCatalog(t)
	return h
}

func (h *harness) seedCatalog(t *testing.T) {
	t.Helper()
	h.banco = shared.ItemID(h.ids.NewID())
	cost, backupCost := brl(t, 40_000), brl(t, 45_000)
	opt := order.ChosenOption{Option: shared.OptionID(h.ids.NewID()), Choice: shared.ChoiceID(h.ids.NewID()),
		Name: "Cor da costura", Label: "Vermelha", ExtraPrice: brl(t, 5_000), ExtraDays: 2}
	h.red = apporder.OptionChoice{Item: h.banco, Option: opt.Option, Choice: opt.Choice}
	h.catalog.AddItem(fake.CatalogItem{
		ID: h.banco, Name: "Banco de couro", Mode: catalog.ModeToOrder, Price: brl(t, 100_000), LeadDays: 15,
		Options: []order.ChosenOption{opt},
		Sourcing: order.Sourcing{Supplier: shared.SupplierID(h.ids.NewID()), SupplierName: "Couros Paulista", Cost: &cost,
			Backup: &order.BackupSource{Supplier: shared.SupplierID(h.ids.NewID()), Name: "Couros Rio", LeadDays: 6, Cost: &backupCost}},
	})
	h.joao = apporder.CustomerFacts{ID: shared.CustomerID(h.ids.NewID()), Name: "João Pereira", Phone: "5592981234567",
		Addresses: []order.DeliveryAddress{{ID: shared.AddressID(h.ids.NewID()), Label: "Casa", CEP: "69057000", Street: "Rua A", City: "Manaus", State: "AM"}}}
	h.customers.Add(h.joao)
}

func (h *harness) as(t *testing.T, role shared.Role) context.Context {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(shared.UserID(h.ids.NewID()), h.store, role)
	if err != nil {
		t.Fatal(err)
	}
	return authz.WithPrincipal(context.Background(), p)
}

func (h *harness) owner(t *testing.T) context.Context  { return h.as(t, shared.RoleOwner) }
func (h *harness) admin(t *testing.T) context.Context  { return h.as(t, shared.RoleAdmin) }
func (h *harness) staff(t *testing.T) context.Context  { return h.as(t, shared.RoleStaff) }
func (h *harness) viewer(t *testing.T) context.Context { return h.as(t, shared.RoleViewer) }

func (h *harness) key() appshared.IdempotencyKey {
	h.keySeq++
	return appshared.IdempotencyKey{Key: "k" + string(rune('a'+h.keySeq)), RequestHash: [32]byte{byte(h.keySeq)}}
}

func brl(t *testing.T, c int64) shared.Money { t.Helper(); return testkit.BRL(t, c) }

func (h *harness) bancoLine() apporder.LineRequest {
	return apporder.LineRequest{Item: h.banco, Quantity: 1}
}

func (h *harness) create(ctx context.Context, t *testing.T, edit func(*apporder.CreateInput)) apporder.CreatedView {
	t.Helper()
	in := apporder.CreateInput{
		Customer: h.joao.ID, Lines: []apporder.LineRequest{h.bancoLine()},
		Terms: apporder.TermsInput{Handover: order.HandoverPickup}, Idempotency: h.key(),
	}
	if edit != nil {
		edit(&in)
	}
	v, err := h.svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return v
}

func (h *harness) pay(ctx context.Context, t *testing.T, v apporder.View, kind order.PaymentKind, cents int64) apporder.View {
	t.Helper()
	out, err := h.svc.RecordPayment(ctx, apporder.PaymentInput{Order: v.ID, Kind: kind, AmountCents: cents,
		Method: order.MethodPix, Version: v.Version, Idempotency: h.key()})
	if err != nil {
		t.Fatalf("RecordPayment: %v", err)
	}
	return out
}

func TestNewService_RefusesAMissingDependency(t *testing.T) {
	t.Parallel()
	if _, err := apporder.NewService(apporder.Deps{}); err == nil || !strings.Contains(err.Error(), "Repo") {
		t.Errorf("err = %v, want one naming Repo", err)
	}
}

func TestCreate_TakesTheCurrentWorkflowAndKeepsItAfterTheOwnerPicksAnother(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	first := h.create(h.staff(t), t, nil)
	if first.Order.Workflow.Key != testkit.PresetEncomendaMontagem || first.Order.Status.Key != "draft" {
		t.Fatalf("new order on %s at %s", first.Order.Workflow.Key, first.Order.Status.Key)
	}
	// The owner picks another list; the first order keeps its own.
	h.workflows.Choose(testkit.Preset(t, testkit.PresetProntaEntrega))
	second := h.create(h.staff(t), t, nil)
	if second.Order.Workflow.Key != testkit.PresetProntaEntrega {
		t.Errorf("second order on %s, want the new choice", second.Order.Workflow.Key)
	}
	got, err := h.svc.Get(h.staff(t), first.Order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Workflow.Key != testkit.PresetEncomendaMontagem || got.Workflow.Version != first.Order.Workflow.Version {
		t.Errorf("first order now on %s v%d, want its own workflow kept", got.Workflow.Key, got.Workflow.Version)
	}
	adv, err := h.svc.Advance(h.staff(t), apporder.AdvanceInput{Order: first.Order.ID, To: "awaiting_deposit", Version: got.Version})
	if err != nil || adv.Status.Label != "Aguardando sinal" {
		t.Errorf("first order still moves on its own list: %v %+v", err, adv.Status)
	}
}

func TestCreate_DatesNumberLinkEventsAndReplay(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	in := apporder.CreateInput{Customer: h.joao.ID, Lines: []apporder.LineRequest{h.bancoLine()},
		Terms: apporder.TermsInput{Handover: order.HandoverPickup}, Idempotency: h.key()}
	v, err := h.svc.Create(h.staff(t), in)
	if err != nil {
		t.Fatal(err)
	}
	o := v.Order
	// 12-01 + 15 + 3 + 2, in the store's day.
	if o.Number != "P-0001" || o.DeliveryDate != testkit.Date(t, "2026-12-21") || o.DepositDue.Cents() != 50_000 {
		t.Errorf("order = %s %s deposit %v", o.Number, o.DeliveryDate, o.DepositDue)
	}
	if !strings.HasPrefix(v.TrackingURL, "https://altarpm.encomendo.test/p/") || v.WhatsAppURL != "" {
		t.Errorf("links = %q %q", v.TrackingURL, v.WhatsAppURL)
	}
	if got := h.events.Names(); !slices.Equal(got, []string{"order.created"}) {
		t.Errorf("events = %v", got)
	}
	again, err := h.svc.Create(h.staff(t), in)
	if err != nil {
		t.Fatal(err)
	}
	if again.Order.ID != o.ID || again.TrackingURL != "" || len(h.events.Names()) != 1 {
		t.Errorf("replay = %s link %q events %v, want the same order, no link, no new event", again.Order.ID, again.TrackingURL, h.events.Names())
	}
	in.Idempotency.RequestHash[0]++
	if _, err := h.svc.Create(h.staff(t), in); !errors.Is(err, appshared.ErrIdempotencyKeyReused) {
		t.Errorf("same key, other body: err = %v", err)
	}
}

func TestCreate_ReplayNeverSkipsThePermission(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	in := apporder.CreateInput{Customer: h.joao.ID, Lines: []apporder.LineRequest{h.bancoLine()},
		Terms: apporder.TermsInput{Handover: order.HandoverPickup}, Idempotency: h.key()}
	if _, err := h.svc.Create(h.staff(t), in); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Create(h.viewer(t), in); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("viewer replaying a key: err = %v, want ErrForbidden", err)
	}
}

func TestCreate_Refusals(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	base := apporder.CreateInput{Customer: h.joao.ID, Lines: []apporder.LineRequest{h.bancoLine()}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}}
	tests := map[string]struct {
		edit func(*apporder.CreateInput)
		want error
	}{
		"no key":         {func(in *apporder.CreateInput) { in.Idempotency = appshared.IdempotencyKey{} }, apporder.ErrIdempotencyKeyRequired},
		"no customer":    {func(in *apporder.CreateInput) { in.Customer = shared.CustomerID{} }, shared.ErrInvalid},
		"staff discount": {func(in *apporder.CreateInput) { in.Terms.DiscountCents = ptr[int64](100) }, shared.ErrForbidden},
		"staff deposit":  {func(in *apporder.CreateInput) { in.Terms.DepositPercent = ptr(30) }, shared.ErrForbidden},
		"no address":     {func(in *apporder.CreateInput) { in.Terms.Handover = order.HandoverDelivery }, shared.ErrInvalid},
		"unknown item":   {func(in *apporder.CreateInput) { in.Lines[0].Item = shared.ItemID(h.ids.NewID()) }, catalog.ErrItemNotFound},
		"unknown address": {func(in *apporder.CreateInput) {
			in.Terms = apporder.TermsInput{Handover: order.HandoverDelivery, Address: shared.AddressID(h.ids.NewID())}
		}, shared.ErrInvalid},
	}
	for name, tt := range tests {
		in := base
		in.Lines = []apporder.LineRequest{h.bancoLine()}
		in.Idempotency = h.key()
		tt.edit(&in)
		if _, err := h.svc.Create(h.staff(t), in); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tt.want)
		}
	}
	if _, err := h.svc.Create(h.viewer(t), base); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("viewer: err = %v", err)
	}
	if n := len(h.events.Names()); n != 0 {
		t.Errorf("refused creates published %d events", n)
	}
}

func TestCreate_AdminSetsTermsAndDelivery(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.admin(t), t, func(in *apporder.CreateInput) {
		in.Terms = apporder.TermsInput{Handover: order.HandoverDelivery, Address: h.joao.Addresses[0].ID,
			FreightCents: 3_000, DiscountCents: ptr[int64](10_000), DepositPercent: ptr(30), AssemblyDays: ptr(5)}
	})
	o := v.Order
	// 1.000 + 30 − 100 = 930; 30% = 279.
	if o.Total.Cents() != 93_000 || o.DepositDue.Cents() != 27_900 || o.Terms.Address == nil || o.Terms.Address.City != "Manaus" {
		t.Errorf("order = total %v deposit %v address %+v", o.Total, o.DepositDue, o.Terms.Address)
	}
	if o.DeliveryDate != testkit.Date(t, "2026-12-23") {
		t.Errorf("delivery = %s, want 15 + 5 + 2 days", o.DeliveryDate)
	}
}

func TestCreate_NewCustomerFoundByPhone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, func(in *apporder.CreateInput) {
		in.Customer, in.New = shared.CustomerID{}, &apporder.NewCustomer{Name: "Outro nome", Phone: h.joao.Phone}
	})
	if v.Order.Customer.ID != h.joao.ID || v.Order.Customer.Name != "João Pereira" {
		t.Errorf("customer = %+v, want João, unchanged", v.Order.Customer)
	}
}

func TestCreate_SendToCustomerMovesOnAndStoresNoLink(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, func(in *apporder.CreateInput) { in.SendToCustomer = true })
	if v.Order.Status.Key != "awaiting_deposit" || !strings.HasPrefix(v.WhatsAppURL, "https://wa.me/5592981234567?text=") {
		t.Fatalf("status %s, link %q", v.Order.Status.Key, v.WhatsAppURL)
	}
	token := v.TrackingURL[strings.LastIndex(v.TrackingURL, "/")+1:]
	if !strings.Contains(v.WhatsAppURL, token) {
		t.Error("the message the user sends lacks the tracking link")
	}
	msgs := h.repo.Messages()
	if len(msgs) != 1 || msgs[0].Kind != apporder.MessageCreated {
		t.Fatalf("messages = %+v", msgs)
	}
	if strings.Contains(msgs[0].Text, token) || strings.Contains(msgs[0].URL, token) || strings.Contains(msgs[0].Text, "/p/") {
		t.Errorf("stored message holds the token: %q", msgs[0].Text)
	}
	if got := h.events.Names(); !slices.Equal(got, []string{"order.created", "order.advanced"}) {
		t.Errorf("events = %v", got)
	}
}

func TestRecordPayment_ConfirmsTheDepositOnceAndReplays(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	h.clock.Advance(48 * time.Hour) // paid on 12-03
	in := apporder.PaymentInput{Order: v.ID, Kind: order.PaymentDeposit, AmountCents: 50_000, Method: order.MethodPix, Version: v.Version, Idempotency: h.key()}
	paid, err := h.svc.RecordPayment(h.staff(t), in)
	if err != nil {
		t.Fatal(err)
	}
	if !paid.DepositPaid || paid.ReleaseDate == nil || *paid.ReleaseDate != testkit.Date(t, "2026-12-03") || paid.DeliveryDate != testkit.Date(t, "2026-12-23") {
		t.Errorf("after the deposit: paid %v release %v delivery %s", paid.DepositPaid, paid.ReleaseDate, paid.DeliveryDate)
	}
	want := []string{"order.created", "order.payment_recorded", "order.deposit_confirmed"}
	if got := h.events.Names(); !slices.Equal(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
	replay, err := h.svc.RecordPayment(h.staff(t), in)
	if err != nil || len(replay.Payments) != 1 || len(h.events.Names()) != 3 {
		t.Errorf("replay: err %v, %d payments, events %v", err, len(replay.Payments), h.events.Names())
	}
	if _, err := h.svc.RecordPayment(h.viewer(t), in); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("viewer: err = %v", err)
	}
	in.Idempotency = appshared.IdempotencyKey{}
	if _, err := h.svc.RecordPayment(h.staff(t), in); !errors.Is(err, apporder.ErrIdempotencyKeyRequired) {
		t.Errorf("no key: err = %v", err)
	}
}

func TestRelease_TheOwnersOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	if _, err := h.svc.Release(h.admin(t), apporder.VersionInput{Order: v.ID, Version: v.Version}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("admin: err = %v, want ErrForbidden", err)
	}
	got, err := h.svc.Release(h.owner(t), apporder.VersionInput{Order: v.ID, Version: v.Version})
	if err != nil || !got.Released {
		t.Fatalf("owner: %v released %v", err, got.Released)
	}
	if _, err := h.svc.Release(h.owner(t), apporder.VersionInput{Order: v.ID, Version: v.Version}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("stale version: err = %v", err)
	}
}

func TestVoidPayment_TheOwnersOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.pay(h.staff(t), t, h.create(h.staff(t), t, nil).Order, order.PaymentDeposit, 50_000)
	in := apporder.VoidPaymentInput{Order: v.ID, Payment: v.Payments[0].ID, Reason: "Pix não caiu", Version: v.Version}
	if _, err := h.svc.VoidPayment(h.admin(t), in); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("admin: err = %v", err)
	}
	got, err := h.svc.VoidPayment(h.owner(t), in)
	if err != nil || got.Payments[0].Voided == nil || got.Paid.Cents() != 0 {
		t.Errorf("owner: %v %+v", err, got.Payments)
	}
}

func TestChange_StaffOmittingTermsKeepsThem(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.admin(t), t, func(in *apporder.CreateInput) {
		in.Terms.DiscountCents, in.Terms.DepositPercent = ptr[int64](10_000), ptr(80)
	}).Order
	got, err := h.svc.Change(h.staff(t), apporder.ChangeInput{Order: v.ID, Version: v.Version,
		Lines: []apporder.LineRequest{{Item: h.banco, Quantity: 2}}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Order.Discount.Cents() != 10_000 || got.Order.Terms.DepositPercent != 80 || got.Order.Subtotal.Cents() != 200_000 {
		t.Errorf("after staff's change: discount %v deposit %d subtotal %v", got.Order.Discount, got.Order.Terms.DepositPercent, got.Order.Subtotal)
	}
}

func TestChange_StaffResettingDiscountIsForbidden(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.admin(t), t, func(in *apporder.CreateInput) { in.Terms.DiscountCents = ptr[int64](10_000) }).Order
	_, err := h.svc.Change(h.staff(t), apporder.ChangeInput{Order: v.ID, Version: v.Version,
		Lines: []apporder.LineRequest{h.bancoLine()}, Terms: apporder.TermsInput{Handover: order.HandoverPickup, DiscountCents: ptr[int64](0)}})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
}

func TestChange_PaidNeedsManageAndApprovalAndAnswersTheSummary(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.pay(h.staff(t), t, h.create(h.staff(t), t, nil).Order, order.PaymentDeposit, 50_000)
	in := apporder.ChangeInput{Order: v.ID, Version: v.Version, Lines: []apporder.LineRequest{{Item: h.banco, Quantity: 2}},
		Terms: apporder.TermsInput{Handover: order.HandoverPickup}}
	if _, err := h.svc.Change(h.staff(t), in); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("staff on a paid order: err = %v", err)
	}
	if _, err := h.svc.Change(h.admin(t), in); !errors.Is(err, order.ErrApprovalRequired) {
		t.Errorf("no approval: err = %v", err)
	}
	in.CustomerApproved = true
	got, err := h.svc.Change(h.admin(t), in)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Summary.Paid || got.Summary.NewTotal.Cents() != 200_000 || !strings.HasPrefix(got.WhatsAppURL, "https://wa.me/") {
		t.Errorf("summary %+v link %q", got.Summary, got.WhatsAppURL)
	}
	if got.Order.DepositDue.Cents() != 50_000 || got.Order.BalanceDue.Cents() != 150_000 {
		t.Errorf("deposit %v balance %v, want the confirmed deposit kept", got.Order.DepositDue, got.Order.BalanceDue)
	}
}

func TestChange_UnchangedLineKeepsItsPriceAndSnapshot(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	// The catalog raises the price; the line sent back as it was keeps the
	// price it was sold at (docs/architecture.md section 4.2).
	h.catalog.AddItem(fake.CatalogItem{ID: h.banco, Name: "Banco renomeado", Mode: catalog.ModeToOrder, Price: brl(t, 150_000), LeadDays: 30})
	got, err := h.svc.Change(h.staff(t), apporder.ChangeInput{Order: v.ID, Version: v.Version,
		Lines: []apporder.LineRequest{h.bancoLine()}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Order.Subtotal.Cents() != 100_000 || got.Order.Items[0].Name != "Banco de couro" || got.Order.Items[0].ID != v.Items[0].ID {
		t.Errorf("after the change: subtotal %v item %q", got.Order.Subtotal, got.Order.Items[0].Name)
	}
}

func TestChange_ChangedLineKeepsItemIDStatusAndSourcing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.admin(t), t, nil).Order
	moved, err := h.svc.UseBackupSupplier(h.admin(t), apporder.ItemVersionInput{Order: v.ID, Item: v.Items[0].ID, Version: v.Version})
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.svc.Change(h.staff(t), apporder.ChangeInput{Order: v.ID, Version: moved.Version,
		Lines: []apporder.LineRequest{{Item: h.banco, Quantity: 1, Options: []apporder.OptionChoice{h.red}}}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
	if err != nil {
		t.Fatal(err)
	}
	it := got.Order.Items[0]
	if it.ID != v.Items[0].ID || it.Supplier == nil || it.Supplier.Name != "Couros Rio" || len(it.Options) != 1 {
		t.Errorf("item = %+v, want the same item on the backup supplier, with the new option", it)
	}
}

func TestChangeDeliveryDate_AnswersTheLinkAndRecordsTheMessage(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	in := apporder.DateInput{Order: v.ID, Date: testkit.Date(t, "2026-12-30"), Reason: "Fornecedor atrasou", Version: v.Version}
	if _, err := h.svc.ChangeDeliveryDate(h.staff(t), in); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("staff: err = %v", err)
	}
	got, err := h.svc.ChangeDeliveryDate(h.admin(t), in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Order.DeliveryDate != in.Date || !got.Order.DateByHand || !strings.Contains(got.WhatsAppURL, "30%2F12%2F2026") {
		t.Errorf("date %s by hand %v link %q", got.Order.DeliveryDate, got.Order.DateByHand, got.WhatsAppURL)
	}
	if msgs := h.repo.Messages(); len(msgs) != 1 || msgs[0].Kind != apporder.MessageDateChanged || strings.Contains(msgs[0].Text, "/p/") {
		t.Errorf("messages = %+v", msgs)
	}
}

func TestSendTrackingLink_RotatesTheTokenAndStoresNoLink(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	created := h.create(h.staff(t), t, nil)
	before, _ := h.repo.Stored(h.store, created.Order.ID)
	got, err := h.svc.SendTrackingLink(h.staff(t), created.Order.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := h.repo.Stored(h.store, created.Order.ID)
	if got.TrackingURL == created.TrackingURL || after.Tracking == before.Tracking {
		t.Error("the link did not change")
	}
	token := got.TrackingURL[strings.LastIndex(got.TrackingURL, "/")+1:]
	msgs := h.repo.Messages()
	if !strings.Contains(got.WhatsAppURL, token) || len(msgs) != 1 || strings.Contains(msgs[0].Text+msgs[0].URL, token) {
		t.Errorf("link %q, stored %+v", got.WhatsAppURL, msgs)
	}
	if _, err := h.svc.SendTrackingLink(h.viewer(t), created.Order.ID); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("viewer: err = %v", err)
	}
}

func TestMovesAndCancel_PermissionsEventsAndVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	moved, err := h.svc.MoveItem(h.staff(t), apporder.MoveItemInput{Order: v.ID, Item: v.Items[0].ID, To: order.ItemSentToSupplier, Version: v.Version})
	if err != nil || moved.Items[0].Status != order.ItemSentToSupplier {
		t.Fatalf("move: %v", err)
	}
	if _, err := h.svc.UseBackupSupplier(h.staff(t), apporder.ItemVersionInput{Order: v.ID, Item: v.Items[0].ID, Version: moved.Version}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("staff backup switch: err = %v", err)
	}
	if _, err := h.svc.Cancel(h.staff(t), apporder.CancelInput{Order: v.ID, Reason: "x", Version: moved.Version}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("staff cancel: err = %v", err)
	}
	if _, err := h.svc.Cancel(h.admin(t), apporder.CancelInput{Order: v.ID, Reason: "x", Version: v.Version}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("stale cancel: err = %v", err)
	}
	got, err := h.svc.Cancel(h.admin(t), apporder.CancelInput{Order: v.ID, Reason: "Cliente desistiu", Version: moved.Version})
	if err != nil || got.CancelReason != "Cliente desistiu" {
		t.Fatalf("cancel: %v", err)
	}
	want := []string{"order.created", "order.item_moved", "order.cancelled"}
	if names := h.events.Names(); !slices.Equal(names, want) {
		t.Errorf("events = %v, want %v", names, want)
	}
	if hist := h.repo.Written(h.store, v.ID); len(hist) != 3 || hist[2].Kind != order.HistoryCancelled {
		t.Errorf("history = %+v", hist)
	}
}

func TestAdvance_GuardRefusalChangesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.staff(t), t, nil).Order
	_, err := h.svc.Advance(h.staff(t), apporder.AdvanceInput{Order: v.ID, To: "purchasing", Version: v.Version})
	var g *order.GuardError
	if !errors.As(err, &g) || g.Guard != order.GuardDepositUnpaid {
		t.Errorf("err = %v, want deposit_unpaid", err)
	}
	if stored, _ := h.repo.Stored(h.store, v.ID); stored.Version != v.Version || len(h.events.Names()) != 1 {
		t.Error("a refused move was saved")
	}
}

// Costs and margins reach the owner only, on reads and on every write
// answer (ADR 0030, Amendments).
func TestViews_CostsAndMarginsForTheOwnerOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(h.owner(t), t, nil).Order
	if v.Cost == nil || v.Cost.Cents() != 40_000 || v.Margin == nil || v.Items[0].Cost == nil {
		t.Errorf("owner's create answer: cost %v margin %v", v.Cost, v.Margin)
	}
	for name, ctx := range map[string]context.Context{"admin": h.admin(t), "staff": h.staff(t), "viewer": h.viewer(t)} {
		got, err := h.svc.Get(ctx, v.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Cost != nil || got.Margin != nil || got.Items[0].Cost != nil {
			t.Errorf("%s reads costs: %v %v %v", name, got.Cost, got.Margin, got.Items[0].Cost)
		}
	}
	answer := h.pay(h.staff(t), t, v, order.PaymentDeposit, 50_000)
	if answer.Cost != nil || answer.Items[0].Cost != nil {
		t.Error("staff's payment answer carries costs")
	}
	if answer.Items[0].Supplier == nil || answer.Items[0].Backup == nil {
		t.Error("staff should see the supplier and the backup switch")
	}
}

// TestTrackingView_HasNoInternalField walks every field name of the client
// view, recursively, against what clients must never see (docs/architecture.md
// section 2, rule 8).
func TestTrackingView_HasNoInternalField(t *testing.T) {
	t.Parallel()
	refused := []string{"cost", "margin", "supplier", "leaddays", "atrisk", "alert", "expected", "recordedby", "history", "token", "phone", "reason"}
	var walk func(reflect.Type, string)
	walk = func(rt reflect.Type, path string) {
		switch rt.Kind() {
		case reflect.Pointer, reflect.Slice:
			walk(rt.Elem(), path)
		case reflect.Struct:
			if rt.PkgPath() == "github.com/bergsantana/encomendo/backend/internal/domain/shared" {
				return // Money and CivilDate
			}
			for i := range rt.NumField() {
				f := rt.Field(i)
				name := strings.ToLower(f.Name)
				for _, r := range refused {
					if strings.Contains(name, r) {
						t.Errorf("TrackingView%s.%s looks like internal data (%s)", path, f.Name, r)
					}
				}
				walk(f.Type, path+"."+f.Name)
			}
		}
	}
	walk(reflect.TypeOf(apporder.TrackingView{}), "")

	h := newHarness(t)
	v := h.create(h.owner(t), t, func(in *apporder.CreateInput) { in.Lines[0].Options = []apporder.OptionChoice{h.red} })
	rec, _ := h.repo.Stored(h.store, v.Order.ID)
	o, err := order.Restore(rec, testkit.Preset(t, testkit.PresetEncomendaMontagem))
	if err != nil {
		t.Fatal(err)
	}
	tv := apporder.NewTrackingView(o, "Alta RPM")
	b, _ := json.Marshal(tv)
	for _, leak := range []string{"Couros", "40000", "Pode atrasar"} {
		if strings.Contains(string(b), leak) {
			t.Errorf("tracking view JSON holds %q: %s", leak, b)
		}
	}
	if len(tv.Steps) != 7 || !tv.Steps[0].Current || tv.Steps[0].Label != "Pedido recebido" || tv.Items[0].Options[0] != "Cor da costura: Vermelha" {
		t.Errorf("tracking view = %+v", tv)
	}
}

func TestQuote_WritesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	q, err := h.svc.Quote(h.staff(t), apporder.QuoteInput{Lines: []apporder.LineRequest{{Item: h.banco, Quantity: 2, Options: []apporder.OptionChoice{h.red}}},
		Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
	if err != nil {
		t.Fatal(err)
	}
	// (1.000 + 50) × 2; 15 + 2 option days + 3 + 2.
	if q.Total.Cents() != 210_000 || q.DepositDue.Cents() != 105_000 || q.DeliveryDate != testkit.Date(t, "2026-12-23") {
		t.Errorf("quote = %+v", q)
	}
	if len(h.events.Names()) != 0 || len(h.repo.Messages()) != 0 {
		t.Error("a quote wrote something")
	}
	v := h.create(h.staff(t), t, nil).Order
	qc, err := h.svc.Quote(h.staff(t), apporder.QuoteInput{Order: v.ID, Lines: []apporder.LineRequest{{Item: h.banco, Quantity: 3}}, Terms: apporder.TermsInput{Handover: order.HandoverPickup}})
	if err != nil || qc.Total.Cents() != 300_000 {
		t.Errorf("change quote: %v %v", err, qc.Total)
	}
	if stored, _ := h.repo.Stored(h.store, v.ID); stored.Version != v.Version {
		t.Error("a change quote saved the order")
	}
}

func ptr[T any](v T) *T { return &v }
