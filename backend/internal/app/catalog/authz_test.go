package catalog_test

import (
	"context"
	"errors"
	"testing"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
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
	owner := catalog.ItemPhotos(it.ID)
	photo := h.confirmPhoto(t, owner, jpegBytes)
	staged := h.stage(t, owner, jpegBytes)
	return []useCase{
		{"RequestPhotoUpload", true, func(ctx context.Context) error {
			return ignore(h.svc.RequestPhotoUpload(ctx, appcatalog.PhotoUploadInput{Owner: owner, ContentType: catalog.ContentTypeJPEG}))
		}},
		{"ConfirmPhoto", true, func(ctx context.Context) error { return ignore(h.svc.ConfirmPhoto(ctx, owner, staged)) }},
		{"OrderPhotos", true, func(ctx context.Context) error {
			return ignore(h.svc.OrderPhotos(ctx, owner, []shared.PhotoID{photo.Photo.ID}))
		}},
		{"DeletePhoto", true, func(ctx context.Context) error { return h.svc.DeletePhoto(ctx, owner, photo.Photo.ID) }},
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
	// The subtests share the harness, so they run one after another.
	for name, c := range callers {
		t.Run(name, func(t *testing.T) {
			for _, uc := range cases {
				if c.writeOnly && !uc.write {
					continue
				}
				if err := uc.call(c.ctx); !errors.Is(err, c.want) {
					t.Errorf("%s: %v, want %v", uc.name, err, c.want)
				}
			}
		})
	}
	if got := len(h.tx.Scopes()); got != before {
		t.Errorf("%d transactions started for refused callers, want none", got-before)
	}
}

func TestUseCases_EveryRoleReads(t *testing.T) {
	t.Parallel()
	for _, role := range []shared.Role{shared.RoleViewer, shared.RoleStaff, shared.RoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			ctx := h.as(t, h.store1, role)
			for _, uc := range h.useCases(t) {
				if uc.write {
					continue
				}
				if err := uc.call(ctx); err != nil {
					t.Errorf("%s: %v", uc.name, err)
				}
			}
		})
	}
}

func TestUseCases_AdminWrites(t *testing.T) {
	t.Parallel()
	for _, uc := range newHarness(t).useCases(t) {
		if !uc.write {
			continue
		}
		t.Run(uc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t) // each write runs on rows of its own
			for _, c := range h.useCases(t) {
				if c.name == uc.name {
					if err := c.call(h.admin(t)); err != nil {
						t.Errorf("admin: %v", err)
					}
				}
			}
		})
	}
}
