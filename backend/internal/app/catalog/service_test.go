package catalog_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/app/catalog/fake"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	start   = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	errBoom = errors.New("postgres is down")
)

var (
	_ appcatalog.UseCases     = (*appcatalog.Service)(nil)
	_ appcatalog.StoreQueries = (*appcatalog.Service)(nil)
)

type harness struct {
	svc       *appcatalog.Service
	store     *fake.Store
	suppliers *fake.Suppliers
	defaults  *fake.Defaults
	photos    *fake.Photos
	cache     *sharedfake.Cache
	tx        *sharedfake.TxManager
	clock     *sharedfake.Clock
	ids       *sharedfake.IDs
	// store1 is the store under test; store2 is another one.
	store1, store2 shared.TenantID
	// couros, ferragens and tecidos are suppliers of store1; alheio is
	// store2's.
	couros, ferragens, tecidos, alheio shared.SupplierID
	// refs, when set, gives refsInput a category and field values.
	refs func(in *appcatalog.ItemInput)
}

// refsInput is suppliedInput with the references refs sets.
func (h *harness) refsInput() appcatalog.ItemInput {
	in := h.suppliedInput("Banco")
	if h.refs != nil {
		h.refs(&in)
	}
	return in
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	clock := sharedfake.NewClock(start)
	ids := sharedfake.NewIDs(clock)
	h := &harness{
		store: fake.NewStore(), suppliers: fake.NewSuppliers(), defaults: fake.NewDefaults(), photos: fake.NewPhotos(clock),
		cache: sharedfake.NewCache(clock), tx: sharedfake.NewTxManager(), clock: clock, ids: ids,
		store1: shared.TenantID(ids.NewID()), store2: shared.TenantID(ids.NewID()),
	}
	h.couros, h.ferragens, h.tecidos, h.alheio = h.supplier(h.store1, "Couros Paulista"), h.supplier(h.store1, "Ferragens Norte"),
		h.supplier(h.store1, "Tecidos Finos"), h.supplier(h.store2, "Alheio")
	all := appcatalog.Defaults{Currency: "BRL", AssemblyDays: 3, SafetyMarginDays: 2, StockEnabled: true, ProductionEnabled: true}
	h.defaults.Set(h.store1, all)
	h.defaults.Set(h.store2, all)
	h.svc = appcatalog.NewService(appcatalog.Deps{
		Categories: h.store.Categories, Attributes: h.store.Attributes, Items: h.store.Items, Kits: h.store.Kits,
		Suppliers: h.suppliers, Defaults: h.defaults, Photos: h.photos,
		Cache: h.cache, Tx: h.tx, IDs: ids,
	})
	return h
}

func (h *harness) supplier(tenant shared.TenantID, name string) shared.SupplierID {
	id := shared.SupplierID(h.ids.NewID())
	h.suppliers.Add(tenant, id, name)
	h.store.AddSupplier(tenant, id)
	return id
}

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

func (h *harness) owner(t *testing.T) context.Context  { return h.as(t, h.store1, shared.RoleOwner) }
func (h *harness) admin(t *testing.T) context.Context  { return h.as(t, h.store1, shared.RoleAdmin) }
func (h *harness) staff(t *testing.T) context.Context  { return h.as(t, h.store1, shared.RoleStaff) }
func (h *harness) viewer(t *testing.T) context.Context { return h.as(t, h.store1, shared.RoleViewer) }

// storePage is the store page's context: the tenant from the subdomain and
// no principal.
func (h *harness) storePage(tenant shared.TenantID) context.Context {
	return appshared.WithScope(context.Background(), appshared.Scope{TenantID: tenant})
}

func brl(t *testing.T, cents int64) shared.Money {
	t.Helper()
	m, err := shared.NewMoney(cents, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func ptr[T any](v T) *T { return &v }

func mustPage(t *testing.T, cursor string, limit int) shared.PageRequest {
	t.Helper()
	p, err := shared.NewPageRequest(cursor, limit)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// fieldsOf returns the fields of a ValidationError, checking each has a
// pt-BR message.
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

// suppliedInput is an item bought from couros at 7 days, with a backup from
// ferragens at 9 days. Costs are left out.
func (h *harness) suppliedInput(name string) appcatalog.ItemInput {
	return appcatalog.ItemInput{
		Name: name, Description: "Couro legítimo", SourcingMode: catalog.ModeToOrder, PriceCents: 10000,
		MainSupplier:   &appcatalog.SupplierInput{SupplierID: h.couros, LeadDays: 7},
		BackupSupplier: &appcatalog.SupplierInput{SupplierID: h.ferragens, LeadDays: 9},
		Options: []appcatalog.OptionInput{{Name: "Cor da costura", Required: true, Choices: []appcatalog.ChoiceInput{
			{Label: "Vermelha", ExtraPriceCents: 500, ExtraDays: 2}, {Label: "Preta"},
		}}},
	}
}

// inHouseInput is an item made in house in days days, without a cost.
func inHouseInput(name string, priceCents int64, days int) appcatalog.ItemInput {
	return appcatalog.ItemInput{Name: name, SourcingMode: catalog.ModeInHouse, PriceCents: priceCents, ProductionDays: days}
}

// createItem adds an item as the owner.
func (h *harness) createItem(t *testing.T, in appcatalog.ItemInput) appcatalog.ItemView {
	t.Helper()
	v, err := h.svc.CreateItem(h.owner(t), in)
	if err != nil {
		t.Fatalf("CreateItem(%s): %v", in.Name, err)
	}
	return v
}

// createKit adds a kit as the owner.
func (h *harness) createKit(t *testing.T, in appcatalog.KitInput) appcatalog.KitView {
	t.Helper()
	v, err := h.svc.CreateKit(h.owner(t), in)
	if err != nil {
		t.Fatalf("CreateKit(%s): %v", in.Name, err)
	}
	return v
}

func kitOf(name string, priceCents int64, items ...catalog.KitItem) appcatalog.KitInput {
	return appcatalog.KitInput{Name: name, PriceCents: priceCents, Items: items}
}

func one(id shared.ItemID) catalog.KitItem { return catalog.KitItem{Item: id, Quantity: 1} }

// storeKey is the cache key of the store page catalog of tenant.
func storeKey(t *testing.T, tenant shared.TenantID) appshared.Key {
	t.Helper()
	k, err := appshared.TenantKey(tenant, "catalog", "store")
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// cached reports whether the store page catalog of tenant is in the cache.
func (h *harness) cached(t *testing.T, tenant shared.TenantID) bool {
	t.Helper()
	var c appcatalog.StoreCatalog
	hit, err := h.cache.Get(context.Background(), storeKey(t, tenant), &c)
	if err != nil {
		t.Fatal(err)
	}
	return hit
}

func sameMoney(a, b *shared.Money) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
