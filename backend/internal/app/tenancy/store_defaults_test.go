package tenancy_test

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

func TestStoreDefaults_AStorePageScopeReadsThem(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	store, owner := h.newStore(t, "altarpm")
	next := tenancy.Editable{
		StockEnabled: true, ProductionEnabled: true, DepositPercent: 30, AssemblyDays: 4, SafetyMarginDays: 2,
		WhatsAppPhone: "5592991234567", TimeZone: tenancy.ZoneManaus,
	}
	if _, err := h.svc.UpdateSettings(as(t, owner, store, shared.RoleOwner), apptenancy.UpdateSettingsInput{Settings: next, Version: 1}); err != nil {
		t.Fatal(err)
	}
	storePage := appshared.WithScope(context.Background(), appshared.Scope{TenantID: store})

	got, err := h.svc.StoreDefaults(storePage)

	want := apptenancy.StoreDefaultsView{
		Currency: "BRL", AssemblyDays: 4, SafetyMarginDays: 2, StockEnabled: true, ProductionEnabled: true,
		DepositPercent: 30, TimeZone: tenancy.ZoneManaus, Slug: "altarpm", StoreName: "Loja altarpm",
		WhatsAppPhone: "5592991234567", Workflow: h.preset,
	}
	if err != nil || got != want {
		t.Errorf("StoreDefaults = %+v, %v; want %+v", got, err, want)
	}
	if last := h.tx.Scopes()[len(h.tx.Scopes())-1]; last != (appshared.Scope{TenantID: store}) {
		t.Errorf("read in scope %+v, want the store page's", last)
	}
	if staff, err := h.svc.StoreDefaults(as(t, owner, store, shared.RoleViewer)); err != nil || staff != want {
		t.Errorf("a staff scope = %+v, %v; want the same defaults", staff, err)
	}
}

func TestStoreDefaults_NeedsATenantInTheScope(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.newStore(t, "altarpm")
	before := len(h.tx.Scopes())
	clientArea := appshared.WithScope(context.Background(), appshared.Scope{ClientAccountID: shared.ClientAccountID(h.ids.NewID())})

	for name, ctx := range map[string]context.Context{"no scope": context.Background(), "client area": clientArea} {
		if _, err := h.svc.StoreDefaults(ctx); !errors.Is(err, authz.ErrTenantRequired) {
			t.Errorf("%s: %v, want ErrTenantRequired", name, err)
		}
	}
	if len(h.tx.Scopes()) != before {
		t.Error("a transaction started without a tenant")
	}
}

func TestStoreDefaults_AMissingRowIsABug(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	store, _ := h.newStore(t, "altarpm")
	ctx := appshared.WithScope(context.Background(), appshared.Scope{TenantID: store})

	h.store.Fail("SettingsOf", shared.ErrNotFound)
	if _, err := h.svc.StoreDefaults(ctx); err == nil || errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing row: %v, want a 500 that matches no sentinel", err)
	}
	h.store.Fail("SettingsOf", errBoom)
	if _, err := h.svc.StoreDefaults(ctx); !errors.Is(err, errBoom) {
		t.Errorf("a failed read: %v, want it passed through", err)
	}
}
