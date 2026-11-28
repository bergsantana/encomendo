package purchasingports

import (
	"context"
	"reflect"
	"testing"
	"time"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Store is one store of the suite with a staff user, a member of it, whom
// StaffNames knows as "Ana Souza".
type Store struct {
	Tenant shared.TenantID
	User   shared.UserID
}

// Scope returns the staff scope of the store.
func (s Store) Scope() appshared.Scope {
	return appshared.Scope{TenantID: s.Tenant, UserID: s.User}
}

// Bought is an order of a store with one bought item still to buy.
type Bought struct {
	Order   shared.OrderID
	Item    shared.OrderItemID
	Catalog shared.ItemID
}

// RepositoryHarness is the purchasing repository with what the suite needs
// to use it. Orders and their items are seeded by the harness, because
// Postgres has foreign keys from a line to its order and item, and the cost
// of a line is copied from the order item's sourcing.
type RepositoryHarness struct {
	Repo apppurchasing.Repository
	Tx   appshared.TxManager
	// NewID returns a fresh id.
	NewID func() shared.ID
	// NewStore returns a new store with a staff member named "Ana Souza".
	NewStore func(t *testing.T) Store
	// NewSupplier returns a supplier of the store.
	NewSupplier func(t *testing.T, tenant shared.TenantID) shared.SupplierID
	// NewItem returns an order of the store with one bought item, whose
	// order-time unit cost is cost cents (BRL), or unknown for nil.
	NewItem func(t *testing.T, tenant shared.TenantID, cost *int64) Bought
	// RemoveItem removes the item from its order, as a change to the order
	// does; the lines that held it keep their rows, with no order item.
	RemoveItem func(t *testing.T, tenant shared.TenantID, b Bought)
}

type repoCase struct {
	name string
	run  func(*testing.T, RepositoryHarness)
}

// RunRepositoryContract checks apppurchasing.Repository: the number
// counter, supplier orders with their lines and costs, the reads Compras
// and the order screens use, counter lines, receipts, and isolation between
// stores. Every case runs on a harness of its own.
func RunRepositoryContract(t *testing.T, newHarness func(t *testing.T) RepositoryHarness) {
	t.Helper()
	groups := []struct {
		name  string
		cases []repoCase
	}{
		{"numbers", numberCases},
		{"supplier orders", supplierOrderCases},
		{"lines", lineCases},
		{"receipts", receiptCases},
		{"isolation", isolationCases},
	}
	for _, g := range groups {
		t.Run(g.name, func(t *testing.T) {
			for _, c := range g.cases {
				t.Run(c.name, func(t *testing.T) { c.run(t, newHarness(t)) })
			}
		})
	}
}

// epoch is the suite's "now": whole microseconds in UTC, as Postgres keeps
// them, so a stored time reads back equal.
var epoch = time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)

func (h RepositoryHarness) try(s appshared.Scope, fn func(ctx context.Context) error) error {
	return h.Tx.WithinTx(appshared.WithScope(context.Background(), s), fn)
}

func (h RepositoryHarness) in(t *testing.T, s Store, fn func(ctx context.Context) error) {
	t.Helper()
	if err := h.try(s.Scope(), fn); err != nil {
		t.Fatal(err)
	}
}

func (h RepositoryHarness) principal(t *testing.T, s Store) shared.Principal {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(s.User, s.Tenant, shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func today() shared.CivilDate { return shared.DateOf(epoch, time.UTC) }

func brl(t *testing.T, cents int64) shared.Money {
	t.Helper()
	m, err := shared.NewMoney(cents, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func ptr[T any](v T) *T { return &v }

func sameRecord(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s differs\n got: %+v\nwant: %+v", what, got, want)
	}
}

// item is what a line buys, of b.
func (b Bought) item(name string, quantity int) purchasing.Item {
	return purchasing.Item{Order: b.Order, OrderItem: b.Item, CatalogItem: b.Catalog, Name: name, Quantity: quantity}
}

// newOrder builds the supplier order of items sent at the given time under
// number, one line each, in the order given.
func (h RepositoryHarness) newOrder(t *testing.T, s Store, supplier shared.SupplierID, number purchasing.Number, at time.Time, items ...Bought) *purchasing.SupplierOrder {
	t.Helper()
	lines := make([]purchasing.LineSpec, len(items))
	for i, b := range items {
		lines[i] = purchasing.LineSpec{ID: shared.PurchaseLineID(h.NewID()), Item: b.item("Banco de couro", 1+i)}
	}
	so, err := purchasing.Send(purchasing.SendSpec{
		Tenant: s.Tenant, ID: shared.SupplierOrderID(h.NewID()), Number: number, Supplier: supplier,
		SupplierName: "Couros Paulista", SupplierPhone: "5511987654321", Lines: lines, By: h.principal(t, s), Now: at,
	})
	if err != nil {
		t.Fatalf("build supplier order: %v", err)
	}
	return so
}

const (
	sentText = "Olá, tudo bem? Preciso de 1 Banco de couro."
	sentURL  = "https://wa.me/5511987654321?text=Ol%C3%A1"
)

// send takes the store's next number and stores a supplier order sent at the
// given time, in one transaction, as a send does.
func (h RepositoryHarness) send(t *testing.T, s Store, supplier shared.SupplierID, at time.Time, items ...Bought) *purchasing.SupplierOrder {
	t.Helper()
	var so *purchasing.SupplierOrder
	h.in(t, s, func(ctx context.Context) error {
		n, err := h.Repo.NextNumber(ctx, s.Tenant)
		if err != nil {
			return err
		}
		so = h.newOrder(t, s, supplier, n, at, items...)
		return h.Repo.InsertSupplierOrder(ctx, so, sentText, sentURL)
	})
	return so
}

// stored reads a supplier order back.
func (h RepositoryHarness) stored(t *testing.T, s Store, id shared.SupplierOrderID) purchasing.Record {
	t.Helper()
	var r purchasing.Record
	h.in(t, s, func(ctx context.Context) error {
		var err error
		r, err = h.Repo.SupplierOrder(ctx, s.Tenant, id)
		return err
	})
	return r
}

// restore rebuilds the stored supplier order, as the app layer does.
func (h RepositoryHarness) restore(t *testing.T, s Store, id shared.SupplierOrderID) *purchasing.SupplierOrder {
	t.Helper()
	so, err := purchasing.Restore(h.stored(t, s, id))
	if err != nil {
		t.Fatal(err)
	}
	return so
}

// update stores so inside a transaction of the store.
func (h RepositoryHarness) update(s Store, so *purchasing.SupplierOrder) error {
	return h.try(s.Scope(), func(ctx context.Context) error { return h.Repo.UpdateSupplierOrder(ctx, so) })
}

func (h RepositoryHarness) mustUpdate(t *testing.T, s Store, so *purchasing.SupplierOrder) {
	t.Helper()
	if err := h.update(s, so); err != nil {
		t.Fatal(err)
	}
}

// world is a store with a supplier, for the cases that need nothing more.
type world struct {
	s        Store
	supplier shared.SupplierID
}

func (h RepositoryHarness) world(t *testing.T) world {
	t.Helper()
	s := h.NewStore(t)
	return world{s: s, supplier: h.NewSupplier(t, s.Tenant)}
}

func (h RepositoryHarness) item(t *testing.T, w world) Bought {
	t.Helper()
	return h.NewItem(t, w.s.Tenant, ptr(int64(4000)))
}
