package testkit

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

var idSeq atomic.Int64

// NextID returns a fresh, valid id: tests that build several aggregates get
// distinct ids without a random source.
func NextID() shared.ID {
	return shared.NewV7(1_700_000_000_000+idSeq.Add(1), [10]byte{9})
}

// BRL returns cents in reais, failing t on error.
func BRL(t testing.TB, cents int64) shared.Money {
	t.Helper()
	m, err := shared.NewMoney(cents, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Date parses "YYYY-MM-DD", failing t on error.
func Date(t testing.TB, s string) shared.CivilDate {
	t.Helper()
	d, err := shared.ParseCivilDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// Staff returns a backoffice principal with role in tenant, failing t on
// error.
func Staff(t testing.TB, tenant shared.TenantID, role shared.Role) shared.Principal {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(shared.UserID(NextID()), tenant, role)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// ItemOpts describes one loose item for OrderBuilder.WithItem.
type ItemOpts struct {
	Name     string
	Mode     catalog.SourcingMode // default to_order
	Price    int64                // unit price in cents; default 100_000 (R$ 1.000,00)
	Quantity int                  // default 1
	LeadDays int
	Cost     *int64 // per unit, in cents; nil for unknown
	Backup   *order.BackupSource
	Options  []order.ChosenOption
}

// OrderBuilder builds an order.NewSpec with sensible defaults: the
// encomenda_montagem preset, today 2026-12-01, pickup, a 50% deposit, 3
// assembly days and 2 days of margin. Each With method changes one thing.
type OrderBuilder struct {
	spec    order.NewSpec
	wf      func(testing.TB) workflow.Workflow
	items   []ItemOpts
	lines   []order.LineSpec
	termsFn func(*order.Terms)
}

// NewOrder starts an order builder.
func NewOrder() *OrderBuilder {
	tenant := shared.TenantID(NextID())
	return &OrderBuilder{
		spec: order.NewSpec{
			Tenant: tenant, ID: shared.OrderID(NextID()), Number: 1, Customer: shared.CustomerID(NextID()),
			Tracking: shared.SecretHash{1},
			Now:      time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC),
		},
		wf: func(t testing.TB) workflow.Workflow { return Preset(t, PresetEncomendaMontagem) },
	}
}

// WithPreset uses the launch preset with key.
func (b *OrderBuilder) WithPreset(key string) *OrderBuilder {
	b.wf = func(t testing.TB) workflow.Workflow { return Preset(t, key) }
	return b
}

// WithItems adds n loose to_order items, each R$ 1.000,00 with 5 lead days.
func (b *OrderBuilder) WithItems(n int) *OrderBuilder {
	for range n {
		b.items = append(b.items, ItemOpts{LeadDays: 5})
	}
	return b
}

// WithItem adds one loose item.
func (b *OrderBuilder) WithItem(o ItemOpts) *OrderBuilder {
	b.items = append(b.items, o)
	return b
}

// WithLine adds a line spec as it is.
func (b *OrderBuilder) WithLine(l order.LineSpec) *OrderBuilder {
	b.lines = append(b.lines, l)
	return b
}

// WithTerms changes the terms after the defaults are set.
func (b *OrderBuilder) WithTerms(f func(*order.Terms)) *OrderBuilder {
	prev := b.termsFn
	b.termsFn = func(t *order.Terms) {
		if prev != nil {
			prev(t)
		}
		f(t)
	}
	return b
}

// By sets who makes the order.
func (b *OrderBuilder) By(p shared.Principal) *OrderBuilder {
	b.spec.By = p
	return b
}

// Spec returns the order.NewSpec the builder holds.
func (b *OrderBuilder) Spec(t testing.TB) order.NewSpec {
	t.Helper()
	s := b.spec
	s.Workflow = b.wf(t)
	s.Today = DateOf(s.Now)
	if s.By.IsZero() {
		s.By = Staff(t, s.Tenant, shared.RoleOwner)
	}
	s.Terms = order.Terms{
		Handover: order.HandoverPickup, Freight: BRL(t, 0), Discount: BRL(t, 0),
		DepositPercent: 50, AssemblyDays: 3, SafetyMarginDays: 2,
	}
	if b.termsFn != nil {
		b.termsFn(&s.Terms)
	}
	s.Lines = append(s.Lines, b.lines...)
	items := b.items
	if len(items) == 0 && len(b.lines) == 0 {
		items = []ItemOpts{{LeadDays: 5}}
	}
	for _, o := range items {
		s.Lines = append(s.Lines, LooseLine(t, o))
	}
	return s
}

// Build makes the order, failing t on error.
func (b *OrderBuilder) Build(t testing.TB) *order.Order {
	t.Helper()
	o, err := order.New(b.Spec(t))
	if err != nil {
		t.Fatalf("build order: %v", err)
	}
	return o
}

// LooseLine returns a line of one loose item, priced at unit price plus
// options times quantity.
func LooseLine(t testing.TB, o ItemOpts) order.LineSpec {
	t.Helper()
	o = withItemDefaults(o)
	unit := o.Price
	for _, c := range o.Options {
		unit += c.ExtraPrice.Cents()
	}
	return order.LineSpec{
		ID: shared.OrderLineID(NextID()), Price: BRL(t, unit*int64(o.Quantity)),
		Items: []order.ItemSpec{ItemSpec(t, o)},
	}
}

// ItemSpec returns an order.ItemSpec for o, with a fresh id.
func ItemSpec(t testing.TB, o ItemOpts) order.ItemSpec {
	t.Helper()
	o = withItemDefaults(o)
	src := order.Sourcing{Backup: o.Backup}
	if o.Mode != catalog.ModeInHouse {
		src.Supplier, src.SupplierName = shared.SupplierID(NextID()), "Fornecedor "+o.Name
	}
	if o.Cost != nil {
		c := BRL(t, *o.Cost)
		src.Cost = &c
	}
	return order.ItemSpec{
		ID: shared.OrderItemID(NextID()),
		Snapshot: order.ItemSnapshot{
			CatalogItem: shared.ItemID(NextID()), Name: o.Name, SourcingMode: o.Mode,
			UnitPrice: BRL(t, o.Price), Quantity: o.Quantity, Options: o.Options, LeadDays: o.LeadDays,
		},
		Sourcing: src,
	}
}

func withItemDefaults(o ItemOpts) ItemOpts {
	if o.Name == "" {
		o.Name = "Item"
	}
	if o.Mode == "" {
		o.Mode = catalog.ModeToOrder
	}
	if o.Price == 0 {
		o.Price = 100_000
	}
	if o.Quantity == 0 {
		o.Quantity = 1
	}
	return o
}

// DateOf returns the day t falls on in UTC; builders use UTC noon-ish
// times so the day is the same in every Brazilian zone.
func DateOf(t time.Time) shared.CivilDate { return shared.DateOf(t, time.UTC) }
