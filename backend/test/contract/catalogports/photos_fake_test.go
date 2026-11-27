package catalogports

import (
	"context"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/app/catalog/fake"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestFakePhotos_Contract(t *testing.T) {
	RunPhotosContract(t, func(*testing.T) PhotosHarness {
		ids := sharedfake.NewIDs(sharedfake.NewClock(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)))
		store := fake.NewStore()
		tx := sharedfake.NewTxManager()
		seed := func(t *testing.T, tenant shared.TenantID, fn func(ctx context.Context) error) {
			t.Helper()
			scope := appshared.Scope{TenantID: tenant, UserID: shared.UserID(ids.NewID())}
			if err := tx.WithinTx(appshared.WithScope(context.Background(), scope), fn); err != nil {
				t.Fatal(err)
			}
		}
		return PhotosHarness{
			Photos: store.Photos, Tx: tx, NewID: ids.NewID,
			NewTenant: func(*testing.T) shared.TenantID { return shared.TenantID(ids.NewID()) },
			NewItem: func(t *testing.T, tenant shared.TenantID) shared.ItemID {
				it := bareItem(t, tenant, shared.ItemID(ids.NewID()))
				seed(t, tenant, func(ctx context.Context) error { return store.Items.Insert(ctx, it) })
				return it.ID()
			},
			NewKit: func(t *testing.T, tenant shared.TenantID) shared.KitID {
				it := bareItem(t, tenant, shared.ItemID(ids.NewID()))
				k := bareKit(t, tenant, shared.KitID(ids.NewID()), it.ID())
				seed(t, tenant, func(ctx context.Context) error {
					if err := store.Items.Insert(ctx, it); err != nil {
						return err
					}
					return store.Kits.Insert(ctx, k)
				})
				return k.ID()
			},
		}
	})
}

// bareItem is an in-house item with nothing but what it needs.
func bareItem(t *testing.T, tenant shared.TenantID, id shared.ItemID) *catalog.Item {
	t.Helper()
	it, err := catalog.NewItem(tenant, id, catalog.ItemInput{
		ItemDetails: catalog.ItemDetails{Name: "Banco", Price: brl(t, 100)},
		Sourcing:    catalog.SourcingInput{Mode: catalog.ModeInHouse, ProductionDays: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

// bareKit is a kit of one item.
func bareKit(t *testing.T, tenant shared.TenantID, id shared.KitID, item shared.ItemID) *catalog.Kit {
	t.Helper()
	k, err := catalog.NewKit(tenant, id, catalog.KitInput{
		Name: "Kit", Price: brl(t, 100), SwapMode: catalog.SwapLoseDiscount,
		Items: []catalog.KitItem{{Item: item, Quantity: 1}},
	}, catalog.ItemLookup{item: catalog.ItemFacts{Price: brl(t, 100)}})
	if err != nil {
		t.Fatal(err)
	}
	return k
}
