//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/contract/catalogports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// photoOwners seeds the items and kits photos belong to as the superuser,
// so the photos suite does not wait for the catalog repositories (step
// 3.7).
type photoOwners struct{ s *pgtest.Seeder }

func (o photoOwners) item(t *testing.T, tenant shared.TenantID) shared.ItemID {
	t.Helper()
	id := pgtest.NewID()
	_, err := o.s.DB().ExecContext(t.Context(), `INSERT INTO items
		(tenant_id, id, name, sourcing_mode, price_cents, currency, production_days, ready_days)
		VALUES ($1, $2, 'Banco', 'in_house', 100, 'BRL', 1, 1)`, tenant.String(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	return shared.ItemID(id)
}

func (o photoOwners) kit(t *testing.T, tenant shared.TenantID) shared.KitID {
	t.Helper()
	id := pgtest.NewID()
	_, err := o.s.DB().ExecContext(t.Context(), `INSERT INTO kits (tenant_id, id, name, price_cents, currency)
		VALUES ($1, $2, 'Kit', 100, 'BRL')`, tenant.String(), id.String())
	if err != nil {
		t.Fatal(err)
	}
	// A kit holds at least one item, or it cannot be read back.
	_, err = o.s.DB().ExecContext(t.Context(), `INSERT INTO kit_items (tenant_id, kit_id, item_id, quantity, swappable, position)
		VALUES ($1, $2, $3, 1, false, 0)`, tenant.String(), id.String(), o.item(t, tenant).String())
	if err != nil {
		t.Fatal(err)
	}
	return shared.KitID(id)
}

func photosHarness(t *testing.T) catalogports.PhotosHarness {
	s := env.Seeder(t)
	owners := photoOwners{s}
	return catalogports.PhotosHarness{
		Photos: postgres.NewPhotos(), Tx: postgres.NewTxManager(openApp(t)), NewID: pgtest.NewID,
		NewTenant: func(*testing.T) shared.TenantID { return s.Tenant(s.User()) },
		NewItem:   owners.item,
		NewKit:    owners.kit,
	}
}

func TestPhotos_Contract(t *testing.T) { catalogports.RunPhotosContract(t, photosHarness) }

// Items.ByID and Kits.ByID read the photos the photo repository writes, in
// its order, and the owner's version does not move (plan 3.6).
func TestPhotos_OwnersReadThemWithoutAVersionChange(t *testing.T) {
	h := photosHarness(t)
	tenant := h.NewTenant(t)
	item, kit := h.NewItem(t, tenant), h.NewKit(t, tenant)
	scope := appshared.Scope{TenantID: tenant, UserID: shared.UserID(pgtest.NewID())}
	in := func(fn func(ctx context.Context) error) {
		t.Helper()
		if err := h.Tx.WithinTx(appshared.WithScope(context.Background(), scope), fn); err != nil {
			t.Fatal(err)
		}
	}
	var want []shared.PhotoID
	for _, owner := range []catalog.PhotoOwner{catalog.ItemPhotos(item), catalog.ItemPhotos(item), catalog.KitPhotos(kit)} {
		id := shared.PhotoID(pgtest.NewID())
		p, err := catalog.NewPhoto(id, catalog.PhotoKey(tenant, owner, id), catalog.ContentTypeWebP, 99)
		if err != nil {
			t.Fatal(err)
		}
		in(func(ctx context.Context) error { _, err := h.Photos.Insert(ctx, tenant, owner, p); return err })
		want = append(want, id)
	}
	in(func(ctx context.Context) error {
		return h.Photos.Reorder(ctx, tenant, catalog.ItemPhotos(item), []shared.PhotoID{want[1], want[0]})
	})
	in(func(ctx context.Context) error {
		it, err := postgres.NewItems().ByID(ctx, tenant, item)
		if err != nil {
			return err
		}
		if got := it.Photos(); len(got) != 2 || got[0].ID() != want[1] || got[1].ID() != want[0] || it.Version() != 1 {
			t.Errorf("item photos = %v at version %d, want %v at 1", got, it.Version(), []shared.PhotoID{want[1], want[0]})
		}
		k, err := postgres.NewKits().ByID(ctx, tenant, kit)
		if err != nil {
			return err
		}
		if got := k.Photos(); len(got) != 1 || got[0].ID() != want[2] || got[0].ContentType() != catalog.ContentTypeWebP || k.Version() != 1 {
			t.Errorf("kit photos = %v at version %d", got, k.Version())
		}
		return nil
	})
}

// Concurrent confirms on one item never record a 9th photo, nor two at one
// position: Insert serializes on the owner.
func TestPhotos_ConcurrentInsertsKeepTheLimit(t *testing.T) {
	h := photosHarness(t)
	tenant := h.NewTenant(t)
	owner := catalog.ItemPhotos(h.NewItem(t, tenant))
	scope := appshared.Scope{TenantID: tenant, UserID: shared.UserID(pgtest.NewID())}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for range 12 {
		wg.Go(func() {
			id := shared.PhotoID(pgtest.NewID())
			p, err := catalog.NewPhoto(id, catalog.PhotoKey(tenant, owner, id), catalog.ContentTypeJPEG, 10)
			if err != nil {
				errs <- err
				return
			}
			errs <- h.Tx.WithinTx(appshared.WithScope(context.Background(), scope), func(ctx context.Context) error {
				_, err := h.Photos.Insert(ctx, tenant, owner, p)
				return err
			})
		})
	}
	wg.Wait()
	close(errs)
	var ok, limited int
	for err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, catalog.ErrPhotoLimit):
			limited++
		default:
			t.Errorf("Insert: %v", err)
		}
	}
	if ok != 8 || limited != 4 {
		t.Errorf("%d recorded and %d refused, want 8 and 4", ok, limited)
	}
	var stored []appcatalog.StoredPhoto
	err := h.Tx.WithinTx(appshared.WithScope(context.Background(), scope), func(ctx context.Context) error {
		photos, err := h.Photos.ByOwner(ctx, tenant, owner)
		for _, p := range photos {
			sp, err := h.Photos.ByID(ctx, tenant, p.ID())
			if err != nil {
				return err
			}
			stored = append(stored, sp)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, sp := range stored {
		if sp.Position != i {
			t.Errorf("photo %d is at position %d", i, sp.Position)
		}
	}
}
