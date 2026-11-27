package supplierports

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// Harness is the supplier repository with what the suite needs to use it.
type Harness struct {
	Suppliers appsupplier.Repository
	Tx        appshared.TxManager
	NewID     func() shared.ID
	// NewTenant returns a store that suppliers may belong to. The Postgres
	// harness seeds a tenants row; the fake needs none.
	NewTenant func(t *testing.T) shared.TenantID
}

// tenant is one store of the suite with the staff user that acts in it.
type tenant struct {
	id   shared.TenantID
	user shared.UserID
}

func (h Harness) newTenant(t *testing.T) tenant {
	t.Helper()
	return tenant{id: h.NewTenant(t), user: shared.UserID(h.NewID())}
}

func (tn tenant) scope() appshared.Scope {
	return appshared.Scope{TenantID: tn.id, UserID: tn.user}
}

func (h Harness) in(t *testing.T, s appshared.Scope, fn func(ctx context.Context) error) {
	t.Helper()
	if err := h.try(s, fn); err != nil {
		t.Fatal(err)
	}
}

func (h Harness) try(s appshared.Scope, fn func(ctx context.Context) error) error {
	return h.Tx.WithinTx(appshared.WithScope(context.Background(), s), fn)
}

// add stores a supplier of tn with a valid number.
func (h Harness) add(t *testing.T, tn tenant, name string) *supplier.Supplier {
	t.Helper()
	s, err := supplier.New(tn.id, shared.SupplierID(h.NewID()), name, "5511987654321")
	if err != nil {
		t.Fatal(err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error { return h.Suppliers.Insert(ctx, s) })
	return s
}

func page(t *testing.T, cursor string, limit int) shared.PageRequest {
	t.Helper()
	p, err := shared.NewPageRequest(cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func names(p shared.Page[*supplier.Supplier]) []string {
	out := make([]string, 0, len(p.Items))
	for _, s := range p.Items {
		out = append(out, s.Name())
	}
	return out
}

// RunRepositoryContract checks the supplier repository: reading back what
// was stored, optimistic updates, ordering and paging, the filter, ByIDs,
// and what each scope sees and may write (the staff_only policy of
// 0009_suppliers). Names in the suite share one letter case and are ASCII,
// so the order does not depend on a database collation.
func RunRepositoryContract(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Run("a supplier reads back as inserted", func(t *testing.T) { insertAndGet(t, newHarness(t)) })
	t.Run("an unknown supplier is not found", func(t *testing.T) { getUnknown(t, newHarness(t)) })
	t.Run("updates are optimistic", func(t *testing.T) { updateOptimistic(t, newHarness(t)) })
	t.Run("a duplicate id is a conflict", func(t *testing.T) { insertDuplicate(t, newHarness(t)) })
	t.Run("a failed transaction undoes an insert", func(t *testing.T) { rollbackInsert(t, newHarness(t)) })
	t.Run("a failed transaction undoes an update", func(t *testing.T) { rollbackUpdate(t, newHarness(t)) })
	t.Run("an unknown supplier cannot be updated", func(t *testing.T) { updateUnknown(t, newHarness(t)) })
	t.Run("the list orders by name, then id, and pages", func(t *testing.T) { listPages(t, newHarness(t)) })
	t.Run("the last page ends at an exact boundary", func(t *testing.T) { listExactBoundary(t, newHarness(t)) })
	t.Run("the list filters by name, ignoring case", func(t *testing.T) { listFilter(t, newHarness(t)) })
	t.Run("the filter is literal text", func(t *testing.T) { listFilterLiteral(t, newHarness(t)) })
	t.Run("a cursor the repository did not issue is invalid", func(t *testing.T) { listBadCursor(t, newHarness(t)) })
	t.Run("ByIDs leaves out a missing id", func(t *testing.T) { byIDsMissing(t, newHarness(t)) })
	t.Run("another store's suppliers are invisible", func(t *testing.T) { otherTenant(t, newHarness(t)) })
	t.Run("rows cannot be written into another store", func(t *testing.T) { writesStayInScope(t, newHarness(t)) })
	t.Run("a scope with no user sees and writes nothing", func(t *testing.T) { staffOnly(t, newHarness(t)) })
	t.Run("needs a transaction", func(t *testing.T) { needsTx(t, newHarness(t)) })
}

func insertAndGet(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	s := h.add(t, tn, "Couros Paulista")
	h.in(t, tn.scope(), func(ctx context.Context) error {
		got, err := h.Suppliers.ByID(ctx, tn.id, s.ID())
		if err != nil {
			t.Fatalf("ByID: %v", err)
		}
		if got.ID() != s.ID() || got.TenantID() != tn.id || got.Name() != "Couros Paulista" ||
			got.WhatsAppPhone() != "5511987654321" || got.Version() != 1 {
			t.Errorf("read back %+v, want what was inserted at version 1", got)
		}
		return nil
	})
}

func getUnknown(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	h.in(t, tn.scope(), func(ctx context.Context) error {
		_, err := h.Suppliers.ByID(ctx, tn.id, shared.SupplierID(h.NewID()))
		if !errors.Is(err, supplier.ErrNotFound) || !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("an unknown id: %v, want supplier.ErrNotFound", err)
		}
		return nil
	})
}

func updateOptimistic(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	s := h.add(t, tn, "Couros Paulista")
	h.in(t, tn.scope(), func(ctx context.Context) error {
		cur, err := h.Suppliers.ByID(ctx, tn.id, s.ID())
		if err != nil {
			return err
		}
		stale, err := h.Suppliers.ByID(ctx, tn.id, s.ID())
		if err != nil {
			return err
		}
		if err := cur.Rename("Couros do Norte"); err != nil {
			return err
		}
		if err := cur.ChangePhone("5592991234567"); err != nil {
			return err
		}
		if err := h.Suppliers.Update(ctx, cur); err != nil {
			t.Errorf("Update: %v", err)
		}
		got, err := h.Suppliers.ByID(ctx, tn.id, s.ID())
		if err != nil || got.Name() != "Couros do Norte" || got.WhatsAppPhone() != "5592991234567" || got.Version() != 2 {
			t.Errorf("after the update: %+v, %v; want the new values at version 2", got, err)
		}
		if err := stale.Rename("Outro nome"); err != nil {
			return err
		}
		if err := h.Suppliers.Update(ctx, stale); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("a stale update: %v, want ErrVersionConflict", err)
		}
		got, err = h.Suppliers.ByID(ctx, tn.id, s.ID())
		if err != nil || got.Name() != "Couros do Norte" || got.Version() != 2 {
			t.Errorf("after the stale update: %+v, %v; want it unchanged", got, err)
		}
		return nil
	})
}

func updateUnknown(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	ghost, err := supplier.New(tn.id, shared.SupplierID(h.NewID()), "Fantasma", "5511987654321")
	if err != nil {
		t.Fatal(err)
	}
	err = h.try(tn.scope(), func(ctx context.Context) error { return h.Suppliers.Update(ctx, ghost) })
	if !errors.Is(err, supplier.ErrNotFound) {
		t.Errorf("updating a supplier that was never inserted: %v, want ErrNotFound", err)
	}
}

func listPages(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	for _, name := range []string{"Delta", "Alfa", "Charlie", "Bravo", "Echo"} {
		h.add(t, tn, name)
	}
	twinA := h.add(t, tn, "Foxtrot")
	twinB := h.add(t, tn, "Foxtrot")
	wantTwins := []shared.SupplierID{twinA.ID(), twinB.ID()}
	slices.SortFunc(wantTwins, compareIDs)
	h.in(t, tn.scope(), func(ctx context.Context) error {
		all := h.walk(ctx, t, tn, 3)
		var got []string
		var twins []shared.SupplierID
		for _, s := range all {
			got = append(got, s.Name())
			if s.Name() == "Foxtrot" {
				twins = append(twins, s.ID())
			}
		}
		want := []string{"Alfa", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Foxtrot"}
		if !slices.Equal(got, want) {
			t.Errorf("names across pages = %v, want %v", got, want)
		}
		if !slices.Equal(twins, wantTwins) {
			t.Errorf("equal names = %v, want them ordered by id %v", twins, wantTwins)
		}
		return nil
	})
}

// walk lists every supplier of tn page by page, limit at a time, following
// the cursors, and fails when a page is too long or the cursors never end.
func (h Harness) walk(ctx context.Context, t *testing.T, tn tenant, limit int) []*supplier.Supplier {
	t.Helper()
	var all []*supplier.Supplier
	cursor := ""
	for range 20 {
		p, err := h.Suppliers.List(ctx, tn.id, appsupplier.ListFilter{}, page(t, cursor, limit))
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(p.Items) > limit {
			t.Errorf("a page of %d, want at most %d", len(p.Items), limit)
		}
		all = append(all, p.Items...)
		if !p.HasMore() {
			return all
		}
		cursor = p.NextCursor
	}
	t.Fatal("the cursors never ended")
	return nil
}

func compareIDs(a, b shared.SupplierID) int { return strings.Compare(a.String(), b.String()) }

var errRollback = errors.New("supplierports: the transaction fails on purpose")

func insertDuplicate(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	s := h.add(t, a, "Alfa")
	again, err := supplier.New(a.id, s.ID(), "Outra", "5511987654321")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.try(a.scope(), func(ctx context.Context) error { return h.Suppliers.Insert(ctx, again) }); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("the same id again: %v, want ErrConflict", err)
	}
	// The key sees every store's rows: another store reusing the id is the
	// same conflict, never a leak of whose it is.
	foreign, err := supplier.New(b.id, s.ID(), "Alheia", "5511987654321")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.try(b.scope(), func(ctx context.Context) error { return h.Suppliers.Insert(ctx, foreign) }); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("another store with the same id: %v, want ErrConflict", err)
	}
	h.in(t, a.scope(), func(ctx context.Context) error {
		got, err := h.Suppliers.ByID(ctx, a.id, s.ID())
		if err != nil || got.Name() != "Alfa" {
			t.Errorf("the original after the refusals: %+v, %v; want it untouched", got, err)
		}
		return nil
	})
}

func rollbackInsert(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	s, err := supplier.New(tn.id, shared.SupplierID(h.NewID()), "Alfa", "5511987654321")
	if err != nil {
		t.Fatal(err)
	}
	err = h.try(tn.scope(), func(ctx context.Context) error {
		if err := h.Suppliers.Insert(ctx, s); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("the transaction = %v, want the error it returned", err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		if _, err := h.Suppliers.ByID(ctx, tn.id, s.ID()); !errors.Is(err, supplier.ErrNotFound) {
			t.Errorf("after the rollback: %v, want ErrNotFound", err)
		}
		return nil
	})
}

func rollbackUpdate(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	s := h.add(t, tn, "Alfa")
	err := h.try(tn.scope(), func(ctx context.Context) error {
		cur, err := h.Suppliers.ByID(ctx, tn.id, s.ID())
		if err != nil {
			return err
		}
		if err := cur.Rename("Renomeado"); err != nil {
			return err
		}
		if err := cur.ChangePhone("5592991234567"); err != nil {
			return err
		}
		if err := h.Suppliers.Update(ctx, cur); err != nil {
			return err
		}
		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("the transaction = %v, want the error it returned", err)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		got, err := h.Suppliers.ByID(ctx, tn.id, s.ID())
		if err != nil || got.Name() != "Alfa" || got.WhatsAppPhone() != "5511987654321" || got.Version() != 1 {
			t.Errorf("after the rollback: %+v, %v; want the old values at version 1", got, err)
		}
		return nil
	})
}

func listExactBoundary(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	for _, name := range []string{"Alfa", "Bravo", "Charlie", "Delta"} {
		h.add(t, tn, name)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		first, err := h.Suppliers.List(ctx, tn.id, appsupplier.ListFilter{}, page(t, "", 2))
		if err != nil || !first.HasMore() {
			t.Fatalf("first page = %v, %v; want it to have more", names(first), err)
		}
		last, err := h.Suppliers.List(ctx, tn.id, appsupplier.ListFilter{}, page(t, first.NextCursor, 2))
		if err != nil || !slices.Equal(names(last), []string{"Charlie", "Delta"}) || last.NextCursor != "" {
			t.Errorf("second page = %v, cursor %q, %v; want Charlie and Delta and no cursor", names(last), last.NextCursor, err)
		}
		whole, err := h.Suppliers.List(ctx, tn.id, appsupplier.ListFilter{}, page(t, "", 4))
		if err != nil || len(whole.Items) != 4 || whole.NextCursor != "" {
			t.Errorf("a page of exactly all rows = %v, cursor %q, %v; want no cursor", names(whole), whole.NextCursor, err)
		}
		return nil
	})
}

func listFilterLiteral(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	for _, name := range []string{"Alfa", "AxB", "A_B", "100% Couro"} {
		h.add(t, tn, name)
	}
	cases := []struct {
		query string
		want  []string
	}{
		{"%", []string{"100% Couro"}},
		{"_", []string{"A_B"}},
		{"A_B", []string{"A_B"}},
		{"%%", []string{}},
		{"__", []string{}},
		{"a%b", []string{}},
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		for _, c := range cases {
			p, err := h.Suppliers.List(ctx, tn.id, appsupplier.ListFilter{Query: c.query}, page(t, "", 10))
			if err != nil || !slices.Equal(names(p), c.want) {
				t.Errorf("filter %q = %v, %v; want %v", c.query, names(p), err, c.want)
			}
		}
		return nil
	})
}

func listFilter(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	for _, name := range []string{"Couros Paulista", "Ferragens Norte", "Tecidos Couro Fino"} {
		h.add(t, tn, name)
	}
	h.in(t, tn.scope(), func(ctx context.Context) error {
		p, err := h.Suppliers.List(ctx, tn.id, appsupplier.ListFilter{Query: "COURO"}, page(t, "", 10))
		if err != nil || !slices.Equal(names(p), []string{"Couros Paulista", "Tecidos Couro Fino"}) {
			t.Errorf("filter COURO = %v, %v; want the two leather suppliers", names(p), err)
		}
		p, err = h.Suppliers.List(ctx, tn.id, appsupplier.ListFilter{Query: "zzz"}, page(t, "", 10))
		if err != nil || len(p.Items) != 0 || p.HasMore() {
			t.Errorf("filter zzz = %v, %v; want an empty page", names(p), err)
		}
		return nil
	})
}

// firstCursor returns the NextCursor of tn's first page of one supplier.
func (h Harness) firstCursor(t *testing.T, tn tenant) string {
	t.Helper()
	var cursor string
	h.in(t, tn.scope(), func(ctx context.Context) error {
		p, err := h.Suppliers.List(ctx, tn.id, appsupplier.ListFilter{}, page(t, "", 1))
		if err != nil || !p.HasMore() {
			t.Fatalf("first page = %v, %v; want it to have more", names(p), err)
		}
		cursor = p.NextCursor
		return nil
	})
	return cursor
}

func listBadCursor(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	for _, name := range []string{"Alfa", "Bravo", "Charlie"} {
		h.add(t, a, name)
	}
	h.add(t, b, "Delta")
	h.add(t, b, "Eco")
	real := h.firstCursor(t, a)
	first := "A" // a real cursor changed in its first character names another id
	if real[:1] == first {
		first = "B"
	}
	cursors := map[string]string{
		"gibberish":              "not-a-cursor",
		"a truncated cursor":     real[:len(real)-4],
		"a tampered cursor":      first + real[1:],
		"another store's cursor": h.firstCursor(t, b),
	}
	h.in(t, a.scope(), func(ctx context.Context) error {
		for name, c := range cursors {
			if _, err := h.Suppliers.List(ctx, a.id, appsupplier.ListFilter{}, page(t, c, 10)); !errors.Is(err, shared.ErrInvalid) {
				t.Errorf("%s: %v, want ErrInvalid", name, err)
			}
		}
		return nil
	})
}

func byIDsMissing(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	a := h.add(t, tn, "Alfa")
	b := h.add(t, tn, "Bravo")
	missing := shared.SupplierID(h.NewID())
	h.in(t, tn.scope(), func(ctx context.Context) error {
		got, err := h.Suppliers.ByIDs(ctx, tn.id, []shared.SupplierID{b.ID(), missing, a.ID(), a.ID()})
		if err != nil {
			t.Fatalf("ByIDs: %v", err)
		}
		found := map[shared.SupplierID]string{}
		for _, s := range got {
			found[s.ID()] = s.Name()
		}
		if len(got) != 2 || found[a.ID()] != "Alfa" || found[b.ID()] != "Bravo" {
			t.Errorf("ByIDs = %v, want Alfa and Bravo once each and the missing id left out", found)
		}
		none, err := h.Suppliers.ByIDs(ctx, tn.id, nil)
		if err != nil || len(none) != 0 {
			t.Errorf("ByIDs of no ids = %v, %v; want none", none, err)
		}
		return nil
	})
}

func otherTenant(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	mine := h.add(t, a, "Alfa")
	theirs := h.add(t, b, "Bravo")
	h.in(t, a.scope(), func(ctx context.Context) error {
		if _, err := h.Suppliers.ByID(ctx, a.id, theirs.ID()); !errors.Is(err, supplier.ErrNotFound) {
			t.Errorf("reading B's supplier as A: %v, want ErrNotFound", err)
		}
		if _, err := h.Suppliers.ByID(ctx, b.id, theirs.ID()); !errors.Is(err, supplier.ErrNotFound) {
			t.Errorf("reading B's supplier as A, naming B: %v, want ErrNotFound", err)
		}
		p, err := h.Suppliers.List(ctx, a.id, appsupplier.ListFilter{}, page(t, "", 10))
		if err != nil || !slices.Equal(names(p), []string{"Alfa"}) {
			t.Errorf("A's list = %v, %v; want only Alfa", names(p), err)
		}
		p, err = h.Suppliers.List(ctx, b.id, appsupplier.ListFilter{}, page(t, "", 10))
		if err != nil || len(p.Items) != 0 {
			t.Errorf("B's list in A's scope = %v, %v; want none", names(p), err)
		}
		got, err := h.Suppliers.ByIDs(ctx, a.id, []shared.SupplierID{mine.ID(), theirs.ID()})
		if err != nil || len(got) != 1 || got[0].ID() != mine.ID() {
			t.Errorf("ByIDs = %v, %v; want only A's supplier", got, err)
		}
		other, err := supplier.Restore(supplier.Record{
			Tenant: b.id, ID: theirs.ID(), Name: "Invadido", Phone: "5511987654321", Version: 1,
		})
		if err != nil {
			return err
		}
		if err := h.Suppliers.Update(ctx, other); !errors.Is(err, supplier.ErrNotFound) {
			t.Errorf("A updating B's supplier: %v, want ErrNotFound", err)
		}
		return nil
	})
	h.in(t, b.scope(), func(ctx context.Context) error {
		got, err := h.Suppliers.ByID(ctx, b.id, theirs.ID())
		if err != nil || got.Name() != "Bravo" || got.Version() != 1 {
			t.Errorf("B's supplier after A's attempts: %+v, %v; want it untouched", got, err)
		}
		return nil
	})
}

func writesStayInScope(t *testing.T, h Harness) {
	a, b := h.newTenant(t), h.newTenant(t)
	intruder, err := supplier.New(b.id, shared.SupplierID(h.NewID()), "Intruso", "5511987654321")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.try(a.scope(), func(ctx context.Context) error { return h.Suppliers.Insert(ctx, intruder) }); err == nil {
		t.Error("store A inserted a supplier of store B")
	}
	h.in(t, b.scope(), func(ctx context.Context) error {
		p, err := h.Suppliers.List(ctx, b.id, appsupplier.ListFilter{}, page(t, "", 10))
		if err != nil || len(p.Items) != 0 {
			t.Errorf("B's list = %v, %v; want the refused insert absent", names(p), err)
		}
		return nil
	})
}

func staffOnly(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	s := h.add(t, tn, "Alfa")
	noUser := appshared.Scope{TenantID: tn.id}
	h.in(t, noUser, func(ctx context.Context) error {
		if _, err := h.Suppliers.ByID(ctx, tn.id, s.ID()); !errors.Is(err, supplier.ErrNotFound) {
			t.Errorf("ByID with no user: %v, want ErrNotFound", err)
		}
		p, err := h.Suppliers.List(ctx, tn.id, appsupplier.ListFilter{}, page(t, "", 10))
		if err != nil || len(p.Items) != 0 {
			t.Errorf("List with no user = %v, %v; want none", names(p), err)
		}
		got, err := h.Suppliers.ByIDs(ctx, tn.id, []shared.SupplierID{s.ID()})
		if err != nil || len(got) != 0 {
			t.Errorf("ByIDs with no user = %v, %v; want none", got, err)
		}
		cur, err := supplier.Restore(supplier.Record{Tenant: tn.id, ID: s.ID(), Name: "Mudado", Phone: "5592991234567", Version: 1})
		if err != nil {
			return err
		}
		if err := h.Suppliers.Update(ctx, cur); !errors.Is(err, supplier.ErrNotFound) {
			t.Errorf("Update with no user: %v, want ErrNotFound", err)
		}
		return nil
	})
	h.in(t, tn.scope(), func(ctx context.Context) error {
		got, err := h.Suppliers.ByID(ctx, tn.id, s.ID())
		if err != nil || got.Name() != "Alfa" || got.Version() != 1 {
			t.Errorf("the supplier after the refused update: %+v, %v; want it unchanged", got, err)
		}
		return nil
	})
	fresh, err := supplier.New(tn.id, shared.SupplierID(h.NewID()), "Novo", "5511987654321")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.try(noUser, func(ctx context.Context) error { return h.Suppliers.Insert(ctx, fresh) }); err == nil {
		t.Error("a scope with no user inserted a supplier")
	}
}

func needsTx(t *testing.T, h Harness) {
	tn := h.newTenant(t)
	if _, err := h.Suppliers.ByID(context.Background(), tn.id, shared.SupplierID(h.NewID())); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("outside a transaction: %v, want ErrNoTransaction", err)
	}
}
