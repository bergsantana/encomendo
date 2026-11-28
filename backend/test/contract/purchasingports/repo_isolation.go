package purchasingports

import (
	"context"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var isolationCases = []repoCase{
	{"another store reads and writes none of a store's purchasing", otherStoreSeesNothing},
	{"a scope with no staff user is refused on every method", storePageScopeRefused},
	{"StaffNames gives the names of the store's members only", staffNamesOfTheStore},
}

func otherStoreSeesNothing(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	b := h.item(t, w)
	sent := h.send(t, w.s, w.supplier, epoch, b)
	other := h.NewStore(t)
	h.in(t, other, func(ctx context.Context) error {
		if _, err := h.Repo.SupplierOrder(ctx, other.Tenant, sent.ID()); err == nil {
			t.Errorf("another store read a supplier order")
		}
		if _, err := h.Repo.SupplierOrderOfLine(ctx, other.Tenant, sent.Lines()[0].ID()); err == nil {
			t.Errorf("another store found a line")
		}
		if got, err := h.Repo.OpenLines(ctx, other.Tenant); err != nil || len(got) != 0 {
			t.Errorf("OpenLines of another store = %d, %v, want none", len(got), err)
		}
		if got, err := h.Repo.LinesOfOrder(ctx, other.Tenant, b.Order); err != nil || len(got) != 0 {
			t.Errorf("LinesOfOrder of another store = %d, %v, want none", len(got), err)
		}
		page, err := h.Repo.SupplierOrders(ctx, other.Tenant, false, shared.PageRequest{})
		if err != nil || len(page.Items) != 0 {
			t.Errorf("SupplierOrders of another store = %d, %v, want none", len(page.Items), err)
		}
		if got, err := h.Repo.DefectiveItems(ctx, other.Tenant, []shared.OrderItemID{b.Item}); err != nil || len(got) != 0 {
			t.Errorf("DefectiveItems of another store = %v, %v", got, err)
		}
		return nil
	})
	// A scope of another store cannot name this store's tenant.
	err := h.try(other.Scope(), func(ctx context.Context) error {
		_, err := h.Repo.SupplierOrder(ctx, w.s.Tenant, sent.ID())
		return err
	})
	if err == nil {
		t.Errorf("a scope of another store read this store's supplier order by naming its tenant")
	}
	err = h.try(other.Scope(), func(ctx context.Context) error {
		_, err := h.Repo.NextNumber(ctx, w.s.Tenant)
		return err
	})
	if err == nil {
		t.Errorf("a scope of another store took this store's next number")
	}
}

// A store page scope has a tenant and no user. Every table is staff-only and
// the repository refuses before any SQL, because a referential action would
// bypass RLS (0016).
func storePageScopeRefused(t *testing.T, h RepositoryHarness) {
	w := h.world(t)
	b := h.item(t, w)
	sent := h.send(t, w.s, w.supplier, epoch, b)
	page := appshared.Scope{TenantID: w.s.Tenant}
	so := h.newOrder(t, w.s, w.supplier, 5, epoch, h.item(t, w))
	line, err := purchasing.CounterLine(purchasing.CounterSpec{
		ID: shared.PurchaseLineID(h.NewID()), Supplier: w.supplier, Item: h.item(t, w).item("Banco", 1),
		Check: purchasing.CheckOK, Today: today(),
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]func(ctx context.Context) error{
		"NextNumber": func(ctx context.Context) error { _, err := h.Repo.NextNumber(ctx, w.s.Tenant); return err },
		"PeekNumber": func(ctx context.Context) error { _, err := h.Repo.PeekNumber(ctx, w.s.Tenant); return err },
		"InsertSupplierOrder": func(ctx context.Context) error {
			return h.Repo.InsertSupplierOrder(ctx, so, sentText, sentURL)
		},
		"UpdateSupplierOrder": func(ctx context.Context) error { return h.Repo.UpdateSupplierOrder(ctx, so) },
		"SupplierOrder": func(ctx context.Context) error {
			_, err := h.Repo.SupplierOrder(ctx, w.s.Tenant, sent.ID())
			return err
		},
		"Message":           func(ctx context.Context) error { _, err := h.Repo.Message(ctx, w.s.Tenant, sent.ID()); return err },
		"OpenLines":         func(ctx context.Context) error { _, err := h.Repo.OpenLines(ctx, w.s.Tenant); return err },
		"LinesOfOrder":      func(ctx context.Context) error { _, err := h.Repo.LinesOfOrder(ctx, w.s.Tenant, b.Order); return err },
		"InsertCounterLine": func(ctx context.Context) error { return h.Repo.InsertCounterLine(ctx, w.s.Tenant, line) },
		"StaffNames": func(ctx context.Context) error {
			_, err := h.Repo.StaffNames(ctx, w.s.Tenant, []shared.UserID{w.s.User})
			return err
		},
	}
	for name, call := range calls {
		if err := h.try(page, call); err == nil {
			t.Errorf("%s in a scope with no staff user succeeded, want it refused", name)
		}
	}
	h.in(t, w.s, func(ctx context.Context) error {
		if n, err := h.Repo.PeekNumber(ctx, w.s.Tenant); err != nil || n != 2 {
			t.Errorf("next number after refused calls = %v, %v, want 2 (only the one send took one)", n, err)
		}
		return nil
	})
}

func staffNamesOfTheStore(t *testing.T, h RepositoryHarness) {
	a, b := h.NewStore(t), h.NewStore(t)
	ask := []shared.UserID{a.User, b.User, shared.UserID(h.NewID())}
	h.in(t, a, func(ctx context.Context) error {
		got, err := h.Repo.StaffNames(ctx, a.Tenant, ask)
		if err != nil {
			return err
		}
		if len(got) != 1 || got[a.User] != "Ana Souza" {
			t.Errorf("StaffNames = %v, want only this store's member, Ana Souza (a user of another store and an unknown one are absent)", got)
		}
		if none, err := h.Repo.StaffNames(ctx, a.Tenant, nil); err != nil || len(none) != 0 {
			t.Errorf("StaffNames of no one = %v, %v", none, err)
		}
		return nil
	})
}
