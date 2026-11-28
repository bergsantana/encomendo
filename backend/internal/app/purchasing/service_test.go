package purchasing_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

func TestNewService_NamesTheMissingDependency(t *testing.T) {
	w := newWorld(t)
	for name, mutate := range map[string]func(*apppurchasing.Deps){
		"Repo": func(d *apppurchasing.Deps) { d.Repo = nil }, "Orders": func(d *apppurchasing.Deps) { d.Orders = nil },
		"Suppliers": func(d *apppurchasing.Deps) { d.Suppliers = nil }, "Store": func(d *apppurchasing.Deps) { d.Store = nil },
		"Links": func(d *apppurchasing.Deps) { d.Links = nil }, "Photos": func(d *apppurchasing.Deps) { d.Photos = nil },
		"Idempotency": func(d *apppurchasing.Deps) { d.Idempotency = nil }, "Tx": func(d *apppurchasing.Deps) { d.Tx = nil },
		"Clock": func(d *apppurchasing.Deps) { d.Clock = nil }, "IDs": func(d *apppurchasing.Deps) { d.IDs = nil },
	} {
		d := w.deps()
		mutate(&d)
		if _, err := apppurchasing.NewService(d); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("without %s: err = %v", name, err)
		}
	}
}

// useCase is one call of UseCases with who may make it.
type useCase struct {
	name    string
	allowed []shared.Role
	call    func(ctx context.Context) error
}

// useCasesOf returns every use case of w's service with a call that gets
// past the permission check, on ids that exist in w.
func useCasesOf(w *world, svc apppurchasing.UseCases, sent apppurchasing.SupplierOrderView, toBuy shared.OrderItemID, ord shared.OrderID) []useCase {
	line, so := sent.Lines[0].ID, sent.ID
	page, _ := shared.NewPageRequest("", 5)
	var (
		purchase  = []shared.Role{shared.RoleOwner, shared.RoleAdmin}
		ownerOnly = []shared.Role{shared.RoleOwner}
		receiving = []shared.Role{shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff}
		reading   = []shared.Role{shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer}
	)
	ver := apppurchasing.VersionInput{ID: so, Version: sent.Version}
	pick := apppurchasing.PickInput{Supplier: w.supplier.ID, OrderItems: []shared.OrderItemID{toBuy}}
	return []useCase{
		{"ToBuy", purchase, func(ctx context.Context) error { _, err := svc.ToBuy(ctx); return err }},
		{"Preview", purchase, func(ctx context.Context) error { _, err := svc.Preview(ctx, pick); return err }},
		{"Send", purchase, func(ctx context.Context) error {
			_, err := svc.Send(ctx, apppurchasing.SendInput{PickInput: pick, Idempotency: w.key()})
			return err
		}},
		{"SupplierOrders", purchase, func(ctx context.Context) error { _, err := svc.SupplierOrders(ctx, true, page); return err }},
		{"SupplierOrder", purchase, func(ctx context.Context) error { _, err := svc.SupplierOrder(ctx, so); return err }},
		{"RecordPromisedDate", purchase, func(ctx context.Context) error {
			_, err := svc.RecordPromisedDate(ctx, apppurchasing.PromisedDateInput{ID: so, Version: sent.Version, Date: w.today})
			return err
		}},
		{"MarkInTransit", purchase, func(ctx context.Context) error { _, err := svc.MarkInTransit(ctx, ver); return err }},
		{"Undo", purchase, func(ctx context.Context) error { _, err := svc.Undo(ctx, ver); return err }},
		{"RecordPaidPrices", ownerOnly, func(ctx context.Context) error {
			_, err := svc.RecordPaidPrices(ctx, apppurchasing.PaidPricesInput{ID: so, Version: sent.Version, Prices: []apppurchasing.LinePrice{{Line: line, Cents: 1}}})
			return err
		}},
		{"ToReceive", receiving, func(ctx context.Context) error { _, err := svc.ToReceive(ctx); return err }},
		{"RequestPhotoUpload", receiving, func(ctx context.Context) error { _, err := svc.RequestPhotoUpload(ctx, "image/png"); return err }},
		{"Receive", receiving, func(ctx context.Context) error {
			_, err := svc.Receive(ctx, apppurchasing.ReceiptInput{Entries: []apppurchasing.ReceiptEntryInput{entry(line, purchasing.CheckOK, "")}, Idempotency: w.key()})
			return err
		}},
		{"Notices", purchase, func(ctx context.Context) error { _, err := svc.Notices(ctx); return err }},
		{"ResolveNotice", purchase, func(ctx context.Context) error { return svc.ResolveNotice(ctx, line) }},
		{"OrderLines", reading, func(ctx context.Context) error { _, err := svc.OrderLines(ctx, ord); return err }},
		{"Today", purchase, func(ctx context.Context) error { _, err := svc.Today(ctx); return err }},
	}
}

func TestService_EveryUseCaseChecksItsPermission(t *testing.T) {
	w := newWorld(t)
	s := sendTwo(w, w.owner)
	_, toBuy := w.order(240, bought{name: "Pedal", qty: 1})
	calls := useCasesOf(w, w.svc, s.view, toBuy[0], s.order1)
	if len(calls) != 16 {
		t.Fatalf("%d use cases in the table, want the 16 of UseCases", len(calls))
	}
	client, err := shared.NewClientPrincipal(shared.ClientAccountID(w.ids.NewID()))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			for _, who := range []shared.Principal{w.admin, w.staff, w.viewer, w.owner} {
				err := c.call(w.ctx(who))
				allowed := roleIn(who.Role(), c.allowed)
				if allowed && errors.Is(err, shared.ErrForbidden) {
					t.Errorf("%s was refused: %v", who.Role(), err)
				}
				if !allowed && !errors.Is(err, shared.ErrForbidden) {
					t.Errorf("%s was not refused: %v", who.Role(), err)
				}
			}
			if err := c.call(w.ctx(client)); !errors.Is(err, shared.ErrForbidden) {
				t.Errorf("a client account: %v, want ErrForbidden", err)
			}
			if err := c.call(context.Background()); !errors.Is(err, shared.ErrUnauthenticated) {
				t.Errorf("no caller: %v, want ErrUnauthenticated", err)
			}
		})
	}
	// Another store's principal never reaches this store's rows: the scope
	// the middleware set is the caller's own.
	other := testkit.Staff(t, shared.TenantID(w.ids.NewID()), shared.RoleOwner)
	ctx := authz.WithPrincipal(context.Background(), other)
	if _, err := w.svc.SupplierOrder(ctx, s.view.ID); !errors.Is(err, purchasing.ErrSupplierOrderNotFound) {
		t.Fatalf("another store's owner: %v", err)
	}
}

func roleIn(r shared.Role, roles []shared.Role) bool {
	for _, x := range roles {
		if x == r {
			return true
		}
	}
	return false
}

func TestToBuy_GroupsBySupplierMarksReplacementAndTruncation(t *testing.T) {
	w := newWorld(t)
	zeta := apppurchasing.SupplierContact{ID: shared.SupplierID(w.ids.NewID()), Name: "zeta Couros", Phone: "5511911112222"}
	alfa := apppurchasing.SupplierContact{ID: shared.SupplierID(w.ids.NewID()), Name: "Alfa Peças", Phone: "5521933334444"}
	w.suppliers.Add(zeta)
	w.suppliers.Add(alfa)
	_, a := w.orderOf(zeta, 231, bought{name: "Banco", qty: 1})
	_, b := w.orderOf(alfa, 232, bought{name: "Pedal", qty: 1}, bought{name: "Guidão", qty: 3, choices: []purchasing.Choice{preto}})
	_, c := w.orderOf(zeta, 233, bought{name: "Capa", qty: 1})
	w.items.SetTruncated(true)

	v, err := w.svc.ToBuy(w.ctx(w.admin))
	if err != nil {
		t.Fatal(err)
	}
	if !v.Truncated || len(v.Suppliers) != 2 || v.Suppliers[0].Name != "Alfa Peças" || v.Suppliers[1].Name != "zeta Couros" {
		t.Fatalf("suppliers = %+v", v.Suppliers)
	}
	if got := v.Suppliers[0]; got.Phone != "5521933334444" || got.Supplier != alfa.ID || len(got.Items) != 2 || got.Items[0].OrderItem != b[0] {
		t.Fatalf("alfa = %+v", got)
	}
	g := v.Suppliers[0].Items[1]
	if g.Quantity != 3 || g.OrderNumber != "P-0232" || len(g.Choices) != 1 || g.Choices[0].Name != "Cor" || g.ArrivalLimit != w.today.AddDays(17) || g.Replacement {
		t.Fatalf("item = %+v", g)
	}
	if got := v.Suppliers[1].Items; len(got) != 2 || got[0].OrderItem != a[0] || got[1].OrderItem != c[0] {
		t.Fatalf("zeta items = %+v", got)
	}
}

func TestToday_NothingForStaff(t *testing.T) {
	w := newWorld(t)
	w.order(231, bought{name: "Banco", qty: 1})
	for _, who := range []shared.Principal{w.staff, w.viewer} {
		got, err := w.svc.Today(w.ctx(who))
		if !errors.Is(err, shared.ErrForbidden) || got.ToSend != 0 || len(got.Needs) != 0 {
			t.Fatalf("%s: %+v, %v", who.Role(), got, err)
		}
	}
}

func TestToday_NeedsInOrder(t *testing.T) {
	w := newWorld(t)
	alfa := apppurchasing.SupplierContact{ID: shared.SupplierID(w.ids.NewID()), Name: "Alfa", Phone: "5521933334444"}
	w.suppliers.Add(alfa)
	oCancelled, iCancelled := w.order(231, bought{name: "Banco", qty: 1})
	_, iWaiting := w.order(232, bought{name: "Guidão", qty: 1})
	waiting := w.send(w.owner, iWaiting[0]).SupplierOrder
	cancelled := w.send(w.owner, iCancelled[0]).SupplierOrder
	w.orderOf(alfa, 233, bought{name: "Pedal", qty: 1}, bought{name: "Capa", qty: 1})
	w.order(234, bought{name: "Retrovisor", qty: 1})
	w.items.Cancel(oCancelled)
	w.clock.Advance(4 * 24 * time.Hour)

	v, err := w.svc.Today(w.ctx(w.admin))
	if err != nil {
		t.Fatal(err)
	}
	if v.ToSend != 3 || len(v.Needs) != 4 {
		t.Fatalf("today = %+v", v)
	}
	n := v.Needs
	if n[0].Kind != apppurchasing.NeedNotifySupplier || n[0].Line != cancelled.Lines[0].ID || n[0].ItemName != "Banco" || n[0].SupplierName != w.supplier.Name || n[0].Number != 2 {
		t.Errorf("need 0 = %+v", n[0])
	}
	if n[1].Kind != apppurchasing.NeedNoPromisedDate || n[1].SupplierOrder != waiting.ID || n[1].Days != 4 || n[1].Number != 1 {
		t.Errorf("need 1 = %+v", n[1])
	}
	if n[2].Kind != apppurchasing.NeedToSend || n[2].Supplier != alfa.ID || n[2].Count != 2 || n[2].SupplierName != "Alfa" {
		t.Errorf("need 2 = %+v", n[2])
	}
	if n[3].Kind != apppurchasing.NeedToSend || n[3].Supplier != w.supplier.ID || n[3].Count != 1 {
		t.Errorf("need 3 = %+v", n[3])
	}
}

// Days count in the store's zone (UTC-3), not in UTC.
func TestToday_NoPromisedDateAfterTwoDays(t *testing.T) {
	w := newWorld(t)
	w.clock.Set(time.Date(2026, 12, 2, 1, 0, 0, 0, time.UTC)) // 22:00 on Dec 1 in the store
	_, i1 := w.order(231, bought{name: "Banco", qty: 1})
	_, i2 := w.order(232, bought{name: "Guidão", qty: 1})
	undated := w.send(w.owner, i1[0]).SupplierOrder
	dated := w.send(w.owner, i2[0]).SupplierOrder
	if _, err := w.svc.RecordPromisedDate(w.ctx(w.owner), apppurchasing.PromisedDateInput{ID: dated.ID, Version: 1, Date: w.today.AddDays(9)}); err != nil {
		t.Fatal(err)
	}
	needs := func() []apppurchasing.NeedView {
		v, err := w.svc.Today(w.ctx(w.owner))
		if err != nil {
			t.Fatal(err)
		}
		return v.Needs
	}

	w.clock.Set(time.Date(2026, 12, 4, 2, 0, 0, 0, time.UTC)) // 23:00 on Dec 3: 2 days
	if got := needs(); len(got) != 0 {
		t.Fatalf("after 2 days: %+v", got)
	}
	w.clock.Set(time.Date(2026, 12, 4, 12, 0, 0, 0, time.UTC)) // Dec 4 in the store: 3 days (2 in UTC)
	got := needs()
	if len(got) != 1 || got[0].Kind != apppurchasing.NeedNoPromisedDate || got[0].SupplierOrder != undated.ID || got[0].Days != 3 {
		t.Fatalf("after 3 days: %+v", got)
	}
	// Once the supplier promises a date, the need goes.
	if _, err := w.svc.RecordPromisedDate(w.ctx(w.owner), apppurchasing.PromisedDateInput{ID: undated.ID, Version: 1, Date: w.today.AddDays(9)}); err != nil {
		t.Fatal(err)
	}
	if got := needs(); len(got) != 0 {
		t.Fatalf("after the date: %+v", got)
	}
}
