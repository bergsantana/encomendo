package supplier_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	"github.com/bergsantana/encomendo/backend/internal/app/supplier/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

var (
	start   = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	errBoom = errors.New("postgres is down")
)

const phone = "5511987654321"

type harness struct {
	svc    *appsupplier.Service
	store  *fake.Store
	tx     *sharedfake.TxManager
	ids    *sharedfake.IDs
	store1 shared.TenantID
	store2 shared.TenantID
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ids := sharedfake.NewIDs(sharedfake.NewClock(start))
	h := &harness{
		store: fake.NewStore(), tx: sharedfake.NewTxManager(), ids: ids,
		store1: shared.TenantID(ids.NewID()), store2: shared.TenantID(ids.NewID()),
	}
	h.svc = appsupplier.NewService(appsupplier.Deps{Suppliers: h.store, Tx: h.tx, IDs: ids})
	return h
}

var _ appsupplier.UseCases = (*appsupplier.Service)(nil)

// as returns a context of a staff user acting in tenant with role, as the
// authn middleware builds it.
func (h *harness) as(t *testing.T, tenant shared.TenantID, role shared.Role) context.Context {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(shared.UserID(h.ids.NewID()), tenant, role)
	if err != nil {
		t.Fatal(err)
	}
	return authz.WithPrincipal(context.Background(), p)
}

func (h *harness) owner(t *testing.T) context.Context { return h.as(t, h.store1, shared.RoleOwner) }

// create adds a supplier through the use case as the owner.
func (h *harness) create(t *testing.T, name string) appsupplier.View {
	t.Helper()
	v, err := h.svc.Create(h.owner(t), appsupplier.CreateInput{Name: name, WhatsAppPhone: phone})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustPage(t *testing.T, cursor string, limit int) shared.PageRequest {
	t.Helper()
	p, err := shared.NewPageRequest(cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %v, want a ValidationError", err)
	}
	var out []string
	for _, f := range ve.Fields {
		if f.Message == "" {
			t.Errorf("field %q has no pt-BR message", f.Field)
		}
		out = append(out, f.Field)
	}
	return out
}

func TestCreate(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.owner(t)

	v, err := h.svc.Create(ctx, appsupplier.CreateInput{Name: "  Couros Paulista ", WhatsAppPhone: phone})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if v.ID.IsZero() || v.Name != "Couros Paulista" || v.WhatsAppPhone != phone || v.Version != 1 {
		t.Errorf("view = %+v, want the trimmed name, the number and version 1", v)
	}
	got, err := h.svc.Get(ctx, v.ID)
	if err != nil || got != v {
		t.Errorf("Get = %+v, %v; want the created view", got, err)
	}
	if scopes := h.tx.Scopes(); len(scopes) == 0 || scopes[0].TenantID != h.store1 {
		t.Errorf("scopes = %+v, want the transaction in the caller's store", scopes)
	}
}

func TestCreate_PassesValidationThroughWithFieldNames(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	_, err := h.svc.Create(h.owner(t), appsupplier.CreateInput{Name: "  ", WhatsAppPhone: "123"})
	if got := fieldsOf(t, err); !slices.Equal(got, []string{"name", "whatsappPhone"}) {
		t.Errorf("fields = %v, want name and whatsappPhone", got)
	}
	page, err := h.svc.List(h.owner(t), appsupplier.ListFilter{}, shared.PageRequest{})
	if err != nil || len(page.Items) != 0 {
		t.Errorf("after the refusal: %+v, %v; want no supplier stored", page, err)
	}
}

func TestReplace(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.owner(t)
	v := h.create(t, "Couros Paulista")

	got, err := h.svc.Replace(ctx, appsupplier.ReplaceInput{ID: v.ID, Name: "Couros do Norte", WhatsAppPhone: "5592991234567", Version: 1})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	want := appsupplier.View{ID: v.ID, Name: "Couros do Norte", WhatsAppPhone: "5592991234567", Version: 2}
	if got != want {
		t.Errorf("view = %+v, want %+v", got, want)
	}
	if read, err := h.svc.Get(ctx, v.ID); err != nil || read != want {
		t.Errorf("Get = %+v, %v; want the replaced supplier at version 2", read, err)
	}
}

func TestReplace_Refuses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.owner(t)
	v := h.create(t, "Couros Paulista")
	if _, err := h.svc.Replace(ctx, appsupplier.ReplaceInput{ID: v.ID, Name: "Segundo", WhatsAppPhone: phone, Version: 1}); err != nil {
		t.Fatal(err)
	}

	t.Run("a stale version", func(t *testing.T) {
		_, err := h.svc.Replace(ctx, appsupplier.ReplaceInput{ID: v.ID, Name: "Terceiro", WhatsAppPhone: phone, Version: 1})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("error = %v, want ErrVersionConflict", err)
		}
		if read, _ := h.svc.Get(ctx, v.ID); read.Name != "Segundo" || read.Version != 2 {
			t.Errorf("stored = %+v, want it unchanged", read)
		}
	})
	t.Run("a stale version wins over invalid values", func(t *testing.T) {
		_, err := h.svc.Replace(ctx, appsupplier.ReplaceInput{ID: v.ID, Name: "", WhatsAppPhone: "1", Version: 1})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("error = %v, want ErrVersionConflict", err)
		}
	})
	t.Run("invalid values, with every field named", func(t *testing.T) {
		_, err := h.svc.Replace(ctx, appsupplier.ReplaceInput{ID: v.ID, Name: " ", WhatsAppPhone: "1", Version: 2})
		if got := fieldsOf(t, err); !slices.Equal(got, []string{"name", "whatsappPhone"}) {
			t.Errorf("fields = %v, want name and whatsappPhone", got)
		}
		if read, _ := h.svc.Get(ctx, v.ID); read.Name != "Segundo" || read.WhatsAppPhone != phone || read.Version != 2 {
			t.Errorf("stored = %+v, want it unchanged", read)
		}
	})
	t.Run("an unknown supplier", func(t *testing.T) {
		_, err := h.svc.Replace(ctx, appsupplier.ReplaceInput{ID: shared.SupplierID(h.ids.NewID()), Name: "X", WhatsAppPhone: phone, Version: 1})
		if !errors.Is(err, supplier.ErrNotFound) || !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("error = %v, want supplier.ErrNotFound", err)
		}
	})
	t.Run("another store's supplier", func(t *testing.T) {
		_, err := h.svc.Replace(h.as(t, h.store2, shared.RoleOwner), appsupplier.ReplaceInput{ID: v.ID, Name: "X", WhatsAppPhone: phone, Version: 2})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("error = %v, want not found", err)
		}
	})
	t.Run("a failed save", func(t *testing.T) {
		h.store.Fail("Update", errBoom)
		defer h.store.Fail("Update", nil)
		_, err := h.svc.Replace(ctx, appsupplier.ReplaceInput{ID: v.ID, Name: "Quarto", WhatsAppPhone: phone, Version: 2})
		if !errors.Is(err, errBoom) {
			t.Errorf("error = %v, want the repository error", err)
		}
	})
}

func TestGet_NotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	mine := h.create(t, "Alfa")

	if _, err := h.svc.Get(h.owner(t), shared.SupplierID(h.ids.NewID())); !errors.Is(err, supplier.ErrNotFound) {
		t.Errorf("unknown id: %v, want supplier.ErrNotFound", err)
	}
	if _, err := h.svc.Get(h.as(t, h.store2, shared.RoleOwner), mine.ID); !errors.Is(err, supplier.ErrNotFound) {
		t.Errorf("another store's id: %v, want supplier.ErrNotFound", err)
	}
}

func TestList_OrdersByNameAndPages(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, name := range []string{"Delta", "Alfa", "Charlie", "Bravo", "Echo"} {
		h.create(t, name)
	}
	ctx := h.owner(t)

	first, err := h.svc.List(ctx, appsupplier.ListFilter{}, mustPage(t, "", 2))
	if err != nil || !first.HasMore() || len(first.Items) != 2 || first.Items[0].Name != "Alfa" || first.Items[1].Name != "Bravo" {
		t.Fatalf("first page = %+v, %v; want Alfa and Bravo, with more", first, err)
	}
	second, err := h.svc.List(ctx, appsupplier.ListFilter{}, mustPage(t, first.NextCursor, 2))
	if err != nil || !second.HasMore() || len(second.Items) != 2 || second.Items[0].Name != "Charlie" || second.Items[1].Name != "Delta" {
		t.Fatalf("second page = %+v, %v; want Charlie and Delta, with more", second, err)
	}
	last, err := h.svc.List(ctx, appsupplier.ListFilter{}, mustPage(t, second.NextCursor, 2))
	if err != nil || last.HasMore() || len(last.Items) != 1 || last.Items[0].Name != "Echo" {
		t.Errorf("last page = %+v, %v; want Echo, last", last, err)
	}
	if _, err := h.svc.List(ctx, appsupplier.ListFilter{}, mustPage(t, "not-a-cursor", 2)); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("a cursor it did not issue: %v, want ErrInvalid", err)
	}
}

func TestList_FiltersByNameAndSeesOnlyTheCallersStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.create(t, "Couros Paulista")
	h.create(t, "Ferragens Norte")
	if _, err := h.svc.Create(h.as(t, h.store2, shared.RoleOwner), appsupplier.CreateInput{Name: "Couros Rivais", WhatsAppPhone: phone}); err != nil {
		t.Fatal(err)
	}

	page, err := h.svc.List(h.owner(t), appsupplier.ListFilter{Query: " COUROS "}, shared.PageRequest{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Name != "Couros Paulista" {
		t.Errorf("page = %+v, %v; want only the store's own Couros Paulista", page, err)
	}
}

func TestList_FailsWithTheRepositoryError(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.store.Fail("List", errBoom)

	if _, err := h.svc.List(h.owner(t), appsupplier.ListFilter{}, shared.PageRequest{}); !errors.Is(err, errBoom) {
		t.Errorf("error = %v, want the repository error", err)
	}
}

func TestNames(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a, b := h.create(t, "Alfa"), h.create(t, "Bravo")
	foreign, err := h.svc.Create(h.as(t, h.store2, shared.RoleOwner), appsupplier.CreateInput{Name: "Alheia", WhatsAppPhone: phone})
	if err != nil {
		t.Fatal(err)
	}
	missing := shared.SupplierID(h.ids.NewID())

	got, err := h.svc.Names(h.owner(t), []shared.SupplierID{a.ID, missing, b.ID, foreign.ID, a.ID})
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	want := map[shared.SupplierID]string{a.ID: "Alfa", b.ID: "Bravo"}
	if len(got) != len(want) || got[a.ID] != "Alfa" || got[b.ID] != "Bravo" {
		t.Errorf("names = %v, want %v: a missing id and another store's are absent", got, want)
	}

	t.Run("no ids", func(t *testing.T) {
		got, err := h.svc.Names(h.owner(t), nil)
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("names = %v, %v; want an empty map", got, err)
		}
	})
	t.Run("a failed read", func(t *testing.T) {
		h.store.Fail("ByIDs", errBoom)
		defer h.store.Fail("ByIDs", nil)
		if _, err := h.svc.Names(h.owner(t), []shared.SupplierID{a.ID}); !errors.Is(err, errBoom) {
			t.Errorf("error = %v, want the repository error", err)
		}
	})
}

func TestAuthorization(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	v := h.create(t, "Alfa")
	id := []shared.SupplierID{v.ID}

	writes := map[string]func(t *testing.T, ctx context.Context) error{
		"Create": func(_ *testing.T, ctx context.Context) error {
			_, err := h.svc.Create(ctx, appsupplier.CreateInput{Name: "Novo", WhatsAppPhone: phone})
			return err
		},
		"Replace": func(t *testing.T, ctx context.Context) error {
			fresh := h.create(t, "Fresca") // at version 1, so an allowed role succeeds
			_, err := h.svc.Replace(ctx, appsupplier.ReplaceInput{ID: fresh.ID, Name: "Novo", WhatsAppPhone: phone, Version: 1})
			return err
		},
	}
	reads := map[string]func(t *testing.T, ctx context.Context) error{
		"Get": func(_ *testing.T, ctx context.Context) error { _, err := h.svc.Get(ctx, v.ID); return err },
		"List": func(_ *testing.T, ctx context.Context) error {
			_, err := h.svc.List(ctx, appsupplier.ListFilter{}, shared.PageRequest{})
			return err
		},
		"Names": func(_ *testing.T, ctx context.Context) error { _, err := h.svc.Names(ctx, id); return err },
	}
	all := map[string]func(t *testing.T, ctx context.Context) error{}
	maps.Copy(all, writes)
	maps.Copy(all, reads)
	for name, call := range writes {
		for _, role := range []shared.Role{shared.RoleStaff, shared.RoleViewer} {
			t.Run(name+" is refused to "+string(role), func(t *testing.T) {
				if err := call(t, h.as(t, h.store1, role)); !errors.Is(err, shared.ErrForbidden) {
					t.Errorf("error = %v, want ErrForbidden", err)
				}
			})
		}
		for _, role := range []shared.Role{shared.RoleOwner, shared.RoleAdmin} {
			t.Run(name+" is allowed to "+string(role), func(t *testing.T) {
				if err := call(t, h.as(t, h.store1, role)); err != nil {
					t.Errorf("error = %v, want it allowed", err)
				}
			})
		}
	}
	for name, call := range reads {
		for _, role := range []shared.Role{shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer} {
			t.Run(name+" is allowed to "+string(role), func(t *testing.T) {
				if err := call(t, h.as(t, h.store1, role)); err != nil {
					t.Errorf("error = %v, want nil", err)
				}
			})
		}
	}
	t.Run("no caller", func(t *testing.T) {
		for name, call := range reads {
			if err := call(t, context.Background()); !errors.Is(err, shared.ErrUnauthenticated) {
				t.Errorf("%s with no caller: %v, want ErrUnauthenticated", name, err)
			}
		}
	})
	t.Run("no store selected", func(t *testing.T) {
		p, err := shared.NewBackofficePrincipal(shared.UserID(h.ids.NewID()), shared.TenantID{}, "")
		if err != nil {
			t.Fatal(err)
		}
		ctx := authz.WithPrincipal(context.Background(), p)
		for name, call := range all {
			if err := call(t, ctx); !errors.Is(err, authz.ErrTenantRequired) {
				t.Errorf("%s with no store: %v, want ErrTenantRequired", name, err)
			}
		}
	})
	t.Run("a scope that is not the caller's", func(t *testing.T) {
		other := appshared.Scope{TenantID: h.store2, UserID: shared.UserID(h.ids.NewID())}
		ctx := appshared.WithScope(h.as(t, h.store1, shared.RoleOwner), other)
		for name, call := range all {
			if err := call(t, ctx); !errors.Is(err, authz.ErrScopeMismatch) {
				t.Errorf("%s in another scope: %v, want ErrScopeMismatch", name, err)
			}
		}
	})
	t.Run("a client account", func(t *testing.T) {
		p, err := shared.NewClientPrincipal(shared.ClientAccountID(h.ids.NewID()))
		if err != nil {
			t.Fatal(err)
		}
		ctx := authz.WithPrincipal(context.Background(), p)
		for name, call := range reads {
			if err := call(t, ctx); !errors.Is(err, shared.ErrForbidden) {
				t.Errorf("%s as a client: %v, want ErrForbidden", name, err)
			}
		}
	})
}

func TestUseCases_AuthorizeBeforeTouchingTheRepository(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.store.Fail("Insert", errBoom)

	if _, err := h.svc.Create(h.as(t, h.store1, shared.RoleViewer), appsupplier.CreateInput{Name: "", WhatsAppPhone: ""}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("error = %v, want ErrForbidden before validation or storage", err)
	}
	if n := len(h.tx.Scopes()); n != 0 {
		t.Errorf("%d transactions opened for a refused caller, want none", n)
	}
}
