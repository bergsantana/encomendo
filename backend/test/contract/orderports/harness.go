package orderports

import (
	"context"
	"testing"
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Harness is the order repository with what the suite needs to use it.
type Harness struct {
	Orders apporder.Repository
	Tx     appshared.TxManager
	NewID  func() shared.ID
	// NewTenant returns a store orders may belong to. The Postgres harness
	// seeds a tenants row; the fake needs none.
	NewTenant func(t *testing.T) shared.TenantID
	// Workflow is a published status workflow every order of the suite is
	// made on: the Postgres harness reads one the migrations seeded, the
	// fake gets a preset. Orders restore against it, so it must be the one
	// the repository can find by id and version.
	Workflow workflow.Workflow
	// NewCustomer returns a customer of the store with that name. Orders
	// need one: Postgres has a foreign key, and a name search reads it.
	NewCustomer func(t *testing.T, tenant shared.TenantID, name string) shared.CustomerID
	// NewUser returns a user with that name, whom History names.
	NewUser func(t *testing.T, name string) shared.UserID
}

// store is one store of the suite with the staff user that acts in it.
type store struct {
	id       shared.TenantID
	user     shared.UserID
	customer shared.CustomerID
}

func (h Harness) newStore(t *testing.T) store {
	t.Helper()
	id := h.NewTenant(t)
	return store{id: id, user: h.NewUser(t, "Ana Souza"), customer: h.NewCustomer(t, id, "Cliente Padrão")}
}

func (s store) scope() appshared.Scope {
	return appshared.Scope{TenantID: s.id, UserID: s.user}
}

func (s store) principal(t *testing.T) shared.Principal {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(s.user, s.id, shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (h Harness) try(s appshared.Scope, fn func(ctx context.Context) error) error {
	return h.Tx.WithinTx(appshared.WithScope(context.Background(), s), fn)
}

func (h Harness) in(t *testing.T, s store, fn func(ctx context.Context) error) {
	t.Helper()
	if err := h.try(s.scope(), fn); err != nil {
		t.Fatal(err)
	}
}

func page(t *testing.T, cursor string, limit int) shared.PageRequest {
	t.Helper()
	p, err := shared.NewPageRequest(cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// epoch is the suite's "now": whole microseconds in UTC, as Postgres keeps
// them, so a stored time reads back equal.
var epoch = time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)

func brl(t *testing.T, cents int64) shared.Money {
	t.Helper()
	m, err := shared.NewMoney(cents, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func date(t *testing.T, s string) shared.CivilDate {
	t.Helper()
	d, err := shared.ParseCivilDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func today(now time.Time) shared.CivilDate { return shared.DateOf(now, time.UTC) }

// status returns the first status of the workflow in phase.
func (h Harness) status(t *testing.T, p workflow.Phase) workflow.Status {
	t.Helper()
	for _, s := range h.Workflow.Statuses() {
		if s.Phase == p {
			return s
		}
	}
	t.Fatalf("the workflow has no status in phase %s", p)
	return workflow.Status{}
}

// build is an order under construction. Its fields are what a test varies;
// everything else is a fixed, valid default.
type build struct {
	customer  shared.CustomerID
	number    order.Number
	lead      int
	now       time.Time
	anonymous bool // made with no user, as a store page order is (milestone 7)
	rich      bool // a kit with swaps, options, a backup supplier, delivery
}

// buildOpt changes a build.
type buildOpt func(*build)

func withCustomer(id shared.CustomerID) buildOpt { return func(b *build) { b.customer = id } }
func withLead(days int) buildOpt                 { return func(b *build) { b.lead = days } }
func anonymous() buildOpt                        { return func(b *build) { b.anonymous = true } }
func rich() buildOpt                             { return func(b *build) { b.rich = true } }

// itemSpec returns an item of the given mode with a supplier, a cost and,
// when backup is set, a backup supplier.
func (h Harness) itemSpec(t *testing.T, mode catalog.SourcingMode, name string, lead int, backup bool) order.ItemSpec {
	t.Helper()
	cost := brl(t, 6000)
	src := order.Sourcing{Cost: &cost}
	if mode != catalog.ModeInHouse {
		src.Supplier, src.SupplierName = shared.SupplierID(h.NewID()), "Fornecedor "+name
	} else {
		src.Cost = nil
	}
	if backup {
		bc := brl(t, 6500)
		src.Backup = &order.BackupSource{Supplier: shared.SupplierID(h.NewID()), Name: "Reserva " + name, LeadDays: lead + 4, Cost: &bc}
	}
	return order.ItemSpec{
		ID: shared.OrderItemID(h.NewID()),
		Snapshot: order.ItemSnapshot{
			CatalogItem: shared.ItemID(h.NewID()), Name: name, SourcingMode: mode,
			UnitPrice: brl(t, 10000), Quantity: 1, LeadDays: lead,
		},
		Sourcing: src,
	}
}

func (h Harness) looseLine(t *testing.T, it order.ItemSpec) order.LineSpec {
	t.Helper()
	return order.LineSpec{
		ID:    shared.OrderLineID(h.NewID()),
		Price: brl(t, it.Snapshot.UnitPrice.Cents()*int64(it.Snapshot.Quantity)), Items: []order.ItemSpec{it},
	}
}

// richLines returns a kit line with a swap, one loose item with a chosen
// option and a backup supplier, and one in-house item.
func (h Harness) richLines(t *testing.T, lead int) []order.LineSpec {
	t.Helper()
	removed, added := shared.ItemID(h.NewID()), shared.ItemID(h.NewID())
	kitItem := h.itemSpec(t, catalog.ModeToOrder, "Peça do kit", lead, false)
	kitItem.Snapshot.CatalogItem = added
	kit := order.LineSpec{
		ID: shared.OrderLineID(h.NewID()), Price: brl(t, 45000), Items: []order.ItemSpec{kitItem},
		Kit: &order.KitSnapshot{
			Kit: shared.KitID(h.NewID()), Name: "Kit Iniciante",
			Swaps: []order.SwapSnapshot{{Removed: removed, Added: added, RemovedName: "Peça original", AddedName: "Peça do kit"}},
		},
		AssemblyDays: intp(2), SafetyMarginDays: intp(1),
	}
	opt := h.itemSpec(t, catalog.ModeToOrder, "Banco com opção", lead+1, true)
	opt.Snapshot.Quantity = 2
	opt.Snapshot.Options = []order.ChosenOption{{
		Option: shared.OptionID(h.NewID()), Choice: shared.ChoiceID(h.NewID()), Name: "Cor da costura", Label: "Vermelha",
		ExtraPrice: brl(t, 1500), ExtraDays: 2,
	}}
	loose := h.looseLine(t, opt)
	loose.Price = brl(t, (10000+1500)*2)
	made := h.looseLine(t, h.itemSpec(t, catalog.ModeInHouse, "Peça própria", 3, false))
	return []order.LineSpec{kit, loose, made}
}

func intp(n int) *int { return &n }

// newOrder builds an order of the store with the number n.
func (h Harness) newOrder(t *testing.T, s store, n order.Number, opts ...buildOpt) *order.Order {
	t.Helper()
	b := build{customer: s.customer, number: n, lead: 5, now: epoch}
	for _, o := range opts {
		o(&b)
	}
	by := s.principal(t)
	if b.anonymous {
		by = shared.Principal{}
	}
	id := shared.OrderID(h.NewID())
	var tracking shared.SecretHash
	copy(tracking[:16], id[:])
	copy(tracking[16:], id[:])
	terms := order.Terms{
		Handover: order.HandoverPickup, Freight: brl(t, 0), Discount: brl(t, 0),
		DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2,
	}
	lines := []order.LineSpec{h.looseLine(t, h.itemSpec(t, catalog.ModeToOrder, "Peça avulsa", b.lead, false))}
	if b.rich {
		lines = h.richLines(t, b.lead)
		terms.Handover, terms.Freight, terms.Discount = order.HandoverDelivery, brl(t, 2500), brl(t, 1000)
		terms.Address = &order.DeliveryAddress{
			ID: shared.AddressID(h.NewID()), Label: "Casa", CEP: "69000000", Street: "Rua das Flores", Number: "10",
			Complement: "Apto 2", District: "Centro", City: "Manaus", State: "AM",
		}
	}
	o, err := order.New(order.NewSpec{
		Tenant: s.id, ID: id, Number: n, Customer: b.customer, Workflow: h.Workflow, Lines: lines, Terms: terms,
		Tracking: tracking, By: by, Now: b.now, Today: today(b.now),
	})
	if err != nil {
		t.Fatalf("build order: %v", err)
	}
	return o
}

// insert takes the store's next number and stores a new order with it, in
// one transaction, as Create does.
func (h Harness) insert(t *testing.T, s store, opts ...buildOpt) *order.Order {
	t.Helper()
	var o *order.Order
	h.in(t, s, func(ctx context.Context) error {
		n, err := h.Orders.NextNumber(ctx, s.id)
		if err != nil {
			return err
		}
		o = h.newOrder(t, s, n, opts...)
		return h.Orders.Insert(ctx, o)
	})
	return o
}

// insertAs takes the store's next number and stores a new order with it
// after change edits its record, restored on the workflow: the way the suite
// puts an order in a state without walking there. A nil change stores the
// order as built.
func (h Harness) insertAs(t *testing.T, s store, change func(*order.Record), opts ...buildOpt) *order.Order {
	t.Helper()
	var out *order.Order
	h.in(t, s, func(ctx context.Context) error {
		n, err := h.Orders.NextNumber(ctx, s.id)
		if err != nil {
			return err
		}
		r := h.newOrder(t, s, n, opts...).Record()
		if change != nil {
			change(&r)
		}
		if out, err = order.Restore(r, h.Workflow); err != nil {
			return err
		}
		return h.Orders.Insert(ctx, out)
	})
	return out
}

// load reads an order back as the app layer does: a record restored on its
// workflow.
func (h Harness) load(t *testing.T, s store, id shared.OrderID) *order.Order {
	t.Helper()
	var o *order.Order
	h.in(t, s, func(ctx context.Context) error {
		r, err := h.Orders.ByID(ctx, s.id, id)
		if err != nil {
			return err
		}
		o, err = order.Restore(r, h.Workflow)
		return err
	})
	return o
}

func (h Harness) record(t *testing.T, s store, id shared.OrderID) order.Record {
	t.Helper()
	var r order.Record
	h.in(t, s, func(ctx context.Context) error {
		var err error
		r, err = h.Orders.ByID(ctx, s.id, id)
		return err
	})
	return r
}

func (h Harness) update(s store, o *order.Order) error {
	return h.try(s.scope(), func(ctx context.Context) error { return h.Orders.Update(ctx, o) })
}

func (h Harness) mustUpdate(t *testing.T, s store, o *order.Order) {
	t.Helper()
	if err := h.update(s, o); err != nil {
		t.Fatal(err)
	}
}
