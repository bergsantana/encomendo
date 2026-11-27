package catalog_test

import (
	"context"
	"errors"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// useCase is one use case called with the ids of rows that exist.
type useCase struct {
	name  string
	write bool
	call  func(ctx context.Context) error
}

func (h *harness) useCases(t *testing.T) []useCase {
	t.Helper()
	cat, err := h.svc.CreateCategory(h.owner(t), appcatalog.CreateCategoryInput{Name: "Bancos"})
	if err != nil {
		t.Fatal(err)
	}
	attr, err := h.svc.CreateAttribute(h.owner(t), appcatalog.AttributeInput{Name: "Serve em"})
	if err != nil {
		t.Fatal(err)
	}
	it := h.createItem(t, inHouseInput("Banco", 10000, 5))
	k := h.createKit(t, kitOf("Kit", 9000, one(it.ID)))
	free := h.createItem(t, inHouseInput("Avulso", 100, 1)) // in no kit, so it can be archived
	p := shared.PageRequest{}
	ignore := func(_ any, err error) error { return err }
	return []useCase{
		{"ListCategories", false, func(ctx context.Context) error { return ignore(h.svc.ListCategories(ctx)) }},
		{"CreateCategory", true, func(ctx context.Context) error {
			return ignore(h.svc.CreateCategory(ctx, appcatalog.CreateCategoryInput{Name: "Rodas"}))
		}},
		{"UpdateCategory", true, func(ctx context.Context) error {
			return ignore(h.svc.UpdateCategory(ctx, appcatalog.UpdateCategoryInput{ID: cat.ID, Name: ptr("Assentos")}))
		}},
		{"DeleteCategory", true, func(ctx context.Context) error { return h.svc.DeleteCategory(ctx, cat.ID) }},
		{"ListAttributes", false, func(ctx context.Context) error { return ignore(h.svc.ListAttributes(ctx)) }},
		{"CreateAttribute", true, func(ctx context.Context) error {
			return ignore(h.svc.CreateAttribute(ctx, appcatalog.AttributeInput{Name: "Material"}))
		}},
		{"ReplaceAttribute", true, func(ctx context.Context) error {
			return ignore(h.svc.ReplaceAttribute(ctx, appcatalog.ReplaceAttributeInput{ID: attr.ID, Attribute: appcatalog.AttributeInput{Name: "Moto"}}))
		}},
		{"DeleteAttribute", true, func(ctx context.Context) error { return h.svc.DeleteAttribute(ctx, attr.ID) }},
		{"ListItems", false, func(ctx context.Context) error { return ignore(h.svc.ListItems(ctx, appcatalog.ItemFilter{}, p)) }},
		{"GetItem", false, func(ctx context.Context) error { return ignore(h.svc.GetItem(ctx, it.ID)) }},
		{"CreateItem", true, func(ctx context.Context) error { return ignore(h.svc.CreateItem(ctx, inHouseInput("Capa", 100, 1))) }},
		{"ReplaceItem", true, func(ctx context.Context) error {
			return ignore(h.svc.ReplaceItem(ctx, appcatalog.ReplaceItemInput{ID: it.ID, Item: inHouseInput("Banco", 100, 1), Version: 1}))
		}},
		{"SetItemPublished", true, func(ctx context.Context) error {
			return ignore(h.svc.SetItemPublished(ctx, appcatalog.SetItemPublishedInput{ID: it.ID, Published: true, Version: 1}))
		}},
		{"ArchiveItem", true, func(ctx context.Context) error {
			return ignore(h.svc.ArchiveItem(ctx, appcatalog.ItemVersionInput{ID: free.ID, Version: 1}))
		}},
		{"RestoreItem", true, func(ctx context.Context) error {
			return ignore(h.svc.RestoreItem(ctx, appcatalog.ItemVersionInput{ID: free.ID, Version: 1}))
		}},
		{"ListKits", false, func(ctx context.Context) error { return ignore(h.svc.ListKits(ctx, appcatalog.KitFilter{}, p)) }},
		{"GetKit", false, func(ctx context.Context) error { return ignore(h.svc.GetKit(ctx, k.ID)) }},
		{"CreateKit", true, func(ctx context.Context) error { return ignore(h.svc.CreateKit(ctx, kitOf("Outro", 100, one(it.ID)))) }},
		{"ReplaceKit", true, func(ctx context.Context) error {
			return ignore(h.svc.ReplaceKit(ctx, appcatalog.ReplaceKitInput{ID: k.ID, Kit: kitOf("Kit", 100, one(it.ID)), Version: 1}))
		}},
		{"SetKitPublished", true, func(ctx context.Context) error {
			return ignore(h.svc.SetKitPublished(ctx, appcatalog.SetKitPublishedInput{ID: k.ID, Published: true, Version: 1}))
		}},
		{"ArchiveKit", true, func(ctx context.Context) error {
			return ignore(h.svc.ArchiveKit(ctx, appcatalog.KitVersionInput{ID: k.ID, Version: 1}))
		}},
		{"RestoreKit", true, func(ctx context.Context) error {
			return ignore(h.svc.RestoreKit(ctx, appcatalog.KitVersionInput{ID: k.ID, Version: 1}))
		}},
	}
}

func TestUseCases_RefuseCallersWithoutThePermission(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	cases := h.useCases(t)
	client, err := shared.NewClientPrincipal(shared.ClientAccountID(h.ids.NewID()))
	if err != nil {
		t.Fatal(err)
	}
	callers := map[string]struct {
		ctx       context.Context
		want      error
		writeOnly bool
	}{
		"no caller":                      {context.Background(), shared.ErrUnauthenticated, false},
		"a client account":               {authz.WithPrincipal(context.Background(), client), shared.ErrForbidden, false},
		"no store selected":              {h.as(t, shared.TenantID{}, ""), authz.ErrTenantRequired, false},
		"the store page with no account": {h.storePage(h.store1), shared.ErrUnauthenticated, false},
		"a viewer writing":               {h.viewer(t), shared.ErrForbidden, true},
		"a staff member writing":         {h.staff(t), shared.ErrForbidden, true},
	}
	before := len(h.tx.Scopes())
	for name, c := range callers {
		for _, uc := range cases {
			if c.writeOnly && !uc.write {
				continue
			}
			if err := uc.call(c.ctx); !errors.Is(err, c.want) {
				t.Errorf("%s calling %s: %v, want %v", name, uc.name, err, c.want)
			}
		}
	}
	if got := len(h.tx.Scopes()); got != before {
		t.Errorf("%d transactions started for refused callers, want none", got-before)
	}
}

func TestUseCases_EveryRoleReads(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, uc := range h.useCases(t) {
		if uc.write {
			continue
		}
		for _, ctx := range []context.Context{h.viewer(t), h.staff(t), h.admin(t)} {
			if err := uc.call(ctx); err != nil {
				t.Errorf("%s: %v", uc.name, err)
			}
		}
	}
}

func TestUseCases_AdminWrites(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, uc := range h.useCases(t) {
		if !uc.write || uc.name == "DeleteCategory" || uc.name == "DeleteAttribute" {
			continue
		}
		err := uc.call(h.admin(t))
		// Each case runs once on the same rows; a version moved on by an
		// earlier case is a conflict, which means the admin got past authz.
		if err != nil && !errors.Is(err, shared.ErrConflict) {
			t.Errorf("admin %s: %v", uc.name, err)
		}
	}
}
