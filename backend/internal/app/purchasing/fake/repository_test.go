package fake_test

import (
	"context"
	"errors"
	"testing"
	"time"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/app/purchasing/fake"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// These pin the fake to the contract written on apppurchasing.Repository,
// until the contract suite of step 5.9 runs it against Postgres too.

type rig struct {
	t      *testing.T
	tx     *sharedfake.TxManager
	repo   *fake.Repository
	tenant shared.TenantID
	by     shared.Principal
	ids    *sharedfake.IDs
}

func newRig(t *testing.T) *rig {
	clock := sharedfake.NewClock(time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC))
	ids := sharedfake.NewIDs(clock)
	tenant := shared.TenantID(ids.NewID())
	return &rig{t: t, tx: sharedfake.NewTxManager(), repo: fake.NewRepository(), tenant: tenant, ids: ids,
		by: testkit.Staff(t, tenant, shared.RoleOwner)}
}

func (r *rig) in(fn func(ctx context.Context) error) error {
	return r.tx.WithinTx(authz.WithPrincipal(context.Background(), r.by), fn)
}

// supplierOrder builds a sent supplier order of the given order items.
func (r *rig) supplierOrder(number purchasing.Number, items ...shared.OrderItemID) *purchasing.SupplierOrder {
	var lines []purchasing.LineSpec
	order := shared.OrderID(r.ids.NewID())
	for _, it := range items {
		lines = append(lines, purchasing.LineSpec{ID: shared.PurchaseLineID(r.ids.NewID()), Item: purchasing.Item{
			Order: order, OrderItem: it, CatalogItem: shared.ItemID(r.ids.NewID()), Name: "Banco", Quantity: 1}})
	}
	so, err := purchasing.Send(purchasing.SendSpec{
		Tenant: r.tenant, ID: shared.SupplierOrderID(r.ids.NewID()), Number: number, Supplier: shared.SupplierID(r.ids.NewID()),
		SupplierName: "Couros", SupplierPhone: "5511987654321", Lines: lines, By: r.by, Now: time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return so
}

func TestRepository_NumbersNeverRepeatAndPeekTakesNothing(t *testing.T) {
	r := newRig(t)
	var got []purchasing.Number
	for range 2 {
		if err := r.in(func(ctx context.Context) error {
			peek, err := r.repo.PeekNumber(ctx, r.tenant)
			if err != nil {
				return err
			}
			n, err := r.repo.NextNumber(ctx, r.tenant)
			if peek != n {
				t.Errorf("peek = %v, next = %v", peek, n)
			}
			got = append(got, n)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got[0] != 1 || got[1] != 2 {
		t.Fatalf("numbers = %v", got)
	}
	// A rolled back transaction gives its number back.
	boom := errors.New("boom")
	if err := r.in(func(ctx context.Context) error {
		if _, err := r.repo.NextNumber(ctx, r.tenant); err != nil {
			return err
		}
		return boom
	}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	_ = r.in(func(ctx context.Context) error {
		if n, _ := r.repo.PeekNumber(ctx, r.tenant); n != 3 {
			t.Errorf("after a rollback the next number is %v, want 3", n)
		}
		return nil
	})
}

func TestRepository_OneOpenLinePerItemAndVersionCheck(t *testing.T) {
	r := newRig(t)
	item := shared.OrderItemID(r.ids.NewID())
	first := r.supplierOrder(1, item)
	if err := r.in(func(ctx context.Context) error { return r.repo.InsertSupplierOrder(ctx, first, "text", "url") }); err != nil {
		t.Fatal(err)
	}
	second := r.supplierOrder(2, item)
	err := r.in(func(ctx context.Context) error { return r.repo.InsertSupplierOrder(ctx, second, "text", "url") })
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("second open line for one item: %v, want a conflict", err)
	}
	if _, ok := r.repo.Stored(r.tenant, second.ID()); ok {
		t.Fatal("a refused insert stored the order")
	}
	// Once the first is undone, the item may be sent again.
	undo := r.load(first.ID())
	if _, err := undo.Undo(r.by, time.Date(2026, 12, 2, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := r.in(func(ctx context.Context) error { return r.repo.UpdateSupplierOrder(ctx, undo) }); err != nil {
		t.Fatal(err)
	}
	if err := r.in(func(ctx context.Context) error { return r.repo.InsertSupplierOrder(ctx, second, "text", "url") }); err != nil {
		t.Fatalf("after the undo: %v", err)
	}
	// A stale version is refused; the stored one moved on by one.
	if err := r.in(func(ctx context.Context) error { return r.repo.UpdateSupplierOrder(ctx, undo) }); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("stale update: %v", err)
	}
	if rec, _ := r.repo.Stored(r.tenant, first.ID()); rec.Version != 2 {
		t.Fatalf("version = %d", rec.Version)
	}
}

func (r *rig) load(id shared.SupplierOrderID) *purchasing.SupplierOrder {
	var so *purchasing.SupplierOrder
	err := r.in(func(ctx context.Context) error {
		rec, err := r.repo.SupplierOrder(ctx, r.tenant, id)
		if err != nil {
			return err
		}
		so, err = purchasing.Restore(rec)
		return err
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return so
}

func TestRepository_ScopeAndTransaction(t *testing.T) {
	r := newRig(t)
	// Outside a transaction.
	if _, err := r.repo.NextNumber(context.Background(), r.tenant); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Fatalf("outside a transaction: %v", err)
	}
	// Another store's scope sees and writes nothing.
	other := testkit.Staff(t, shared.TenantID(r.ids.NewID()), shared.RoleOwner)
	err := r.tx.WithinTx(authz.WithPrincipal(context.Background(), other), func(ctx context.Context) error {
		_, err := r.repo.NextNumber(ctx, r.tenant)
		return err
	})
	if err == nil {
		t.Fatal("another store's scope took a number")
	}
	// A failed repository fails every call.
	r.repo.Fail(errors.New("down"))
	if err := r.in(func(ctx context.Context) error { _, err := r.repo.PeekNumber(ctx, r.tenant); return err }); err == nil {
		t.Fatal("a failed repository answered")
	}
	r.repo.Fail(nil)
	var _ apppurchasing.Repository = r.repo
	if err := r.in(func(ctx context.Context) error {
		_, err := r.repo.SupplierOrderOfLine(ctx, r.tenant, shared.PurchaseLineID(r.ids.NewID()))
		if !errors.Is(err, purchasing.ErrLineNotFound) {
			t.Errorf("unknown line: %v", err)
		}
		_, err = r.repo.Receipt(ctx, r.tenant, shared.ReceiptID(r.ids.NewID()))
		if !errors.Is(err, apppurchasing.ErrReceiptNotFound) {
			t.Errorf("unknown receipt: %v", err)
		}
		_, err = r.repo.SupplierOrders(ctx, r.tenant, false, mustPage(t, "garbage|x", 5))
		if !errors.Is(err, shared.ErrInvalid) {
			t.Errorf("cursor: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func mustPage(t *testing.T, cursor string, limit int) shared.PageRequest {
	p, err := shared.NewPageRequest(cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
