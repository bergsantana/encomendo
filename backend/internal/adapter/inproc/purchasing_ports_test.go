package inproc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/inproc"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	supplierfake "github.com/bergsantana/encomendo/backend/internal/app/supplier/fake"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

type fixedDefaults struct {
	v   apptenancy.StoreDefaultsView
	err error
}

func (f fixedDefaults) StoreDefaults(context.Context) (apptenancy.StoreDefaultsView, error) {
	return f.v, f.err
}

func TestPurchasingStore_NameAndTodayInTheStoresZone(t *testing.T) {
	t.Parallel()
	// 02:00 UTC on Dec 2 is still Dec 1 in Manaus (UTC−4).
	clock := sharedfake.NewClock(time.Date(2026, 12, 2, 2, 0, 0, 0, time.UTC))
	port := inproc.NewPurchasingStore(fixedDefaults{v: apptenancy.StoreDefaultsView{StoreName: "Alta RPM", TimeZone: tenancy.TimeZone("America/Manaus")}}, clock)
	if name, err := port.Name(context.Background()); err != nil || name != "Alta RPM" {
		t.Errorf("Name = %q, %v", name, err)
	}
	today, err := port.Today(context.Background())
	if err != nil || today.String() != "2026-12-01" {
		t.Errorf("Today = %s, %v; want 2026-12-01 in Manaus", today, err)
	}
	bad := inproc.NewPurchasingStore(fixedDefaults{err: shared.ErrNotFound}, clock)
	if _, err := bad.Today(context.Background()); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a failed read: err = %v", err)
	}
}

func TestPurchasingSuppliers_ContactFromTheSupplier(t *testing.T) {
	t.Parallel()
	ids := sharedfake.NewIDs(sharedfake.NewClock(time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)))
	tenant := shared.TenantID(ids.NewID())
	svc := appsupplier.NewService(appsupplier.Deps{Suppliers: supplierfake.NewStore(), Tx: sharedfake.NewTxManager(), IDs: ids})
	owner := staff(t, shared.UserID(ids.NewID()), tenant, shared.RoleOwner)
	made, err := svc.Create(owner, appsupplier.CreateInput{Name: "Couros Paulista", WhatsAppPhone: "5511987654321"})
	if err != nil {
		t.Fatal(err)
	}
	port := inproc.NewPurchasingSuppliers(svc)
	got, err := port.Contact(staff(t, shared.UserID(ids.NewID()), tenant, shared.RoleAdmin), made.ID)
	if err != nil || got.ID != made.ID || got.Name != "Couros Paulista" || got.Phone != "5511987654321" {
		t.Errorf("Contact = %+v, %v", got, err)
	}
	if _, err := port.Contact(owner, shared.SupplierID(ids.NewID())); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("unknown supplier: err = %v, want ErrNotFound", err)
	}
}
