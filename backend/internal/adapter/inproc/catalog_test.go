package inproc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/inproc"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	supplierfake "github.com/bergsantana/encomendo/backend/internal/app/supplier/fake"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func staff(t *testing.T, user shared.UserID, tenant shared.TenantID, role shared.Role) context.Context {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(user, tenant, role)
	if err != nil {
		t.Fatal(err)
	}
	return authz.WithPrincipal(context.Background(), p)
}

func TestCatalogSuppliers_NamesTheCallersSuppliersOnly(t *testing.T) {
	t.Parallel()
	ids := sharedfake.NewIDs(sharedfake.NewClock(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)))
	svc := appsupplier.NewService(appsupplier.Deps{Suppliers: supplierfake.NewStore(), Tx: sharedfake.NewTxManager(), IDs: ids})
	mine, theirs := shared.TenantID(ids.NewID()), shared.TenantID(ids.NewID())
	owner := staff(t, shared.UserID(ids.NewID()), mine, shared.RoleOwner)
	a, err := svc.Create(owner, appsupplier.CreateInput{Name: "Couros Paulista", WhatsAppPhone: "5511987654321"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(staff(t, shared.UserID(ids.NewID()), theirs, shared.RoleOwner), appsupplier.CreateInput{Name: "Alheio", WhatsAppPhone: "5511987654321"})
	if err != nil {
		t.Fatal(err)
	}
	dir := inproc.NewCatalogSuppliers(svc)

	got, err := dir.Names(staff(t, shared.UserID(ids.NewID()), mine, shared.RoleViewer), []shared.SupplierID{a.ID, b.ID})

	if err != nil || len(got) != 1 || got[a.ID] != "Couros Paulista" {
		t.Errorf("Names = %v, %v; want only the caller's supplier", got, err)
	}
	if _, err := dir.Names(context.Background(), []shared.SupplierID{a.ID}); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("no caller: %v, want ErrUnauthenticated", err)
	}
}

type stubDefaults struct {
	view apptenancy.StoreDefaultsView
	err  error
}

func (s stubDefaults) StoreDefaults(context.Context) (apptenancy.StoreDefaultsView, error) {
	return s.view, s.err
}

func TestCatalogDefaults_TranslatesTenancysDefaults(t *testing.T) {
	t.Parallel()
	view := apptenancy.StoreDefaultsView{Currency: "BRL", AssemblyDays: 3, SafetyMarginDays: 2, StockEnabled: true}
	got, err := inproc.NewCatalogDefaults(stubDefaults{view: view}).StoreDefaults(context.Background())

	want := appcatalog.Defaults{Currency: "BRL", AssemblyDays: 3, SafetyMarginDays: 2, StockEnabled: true}
	if err != nil || got != want {
		t.Errorf("StoreDefaults = %+v, %v; want %+v", got, err, want)
	}
	if _, err := inproc.NewCatalogDefaults(stubDefaults{err: authz.ErrTenantRequired}).StoreDefaults(context.Background()); !errors.Is(err, authz.ErrTenantRequired) {
		t.Errorf("an error: %v, want it passed through", err)
	}
}

var _ inproc.TenancyDefaults = (*apptenancy.Service)(nil)
