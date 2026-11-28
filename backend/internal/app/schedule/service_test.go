package schedule_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appschedule "github.com/bergsantana/encomendo/backend/internal/app/schedule"
	"github.com/bergsantana/encomendo/backend/internal/app/schedule/fake"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

type harness struct {
	svc    *appschedule.Service
	orders *fake.Orders
	clock  *sharedfake.Clock
	order  *order.Order
}

func newHarness(t *testing.T, zone string) *harness {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{orders: fake.NewOrders(), clock: sharedfake.NewClock(time.Date(2026, 12, 2, 3, 30, 0, 0, time.UTC))}
	h.svc, err = appschedule.NewService(fake.NewStore(loc), h.orders, sharedfake.NewTxManager(), h.clock)
	if err != nil {
		t.Fatal(err)
	}
	h.order = testkit.NewOrder().Build(t)
	h.orders.Add(h.order)
	return h
}

func (h *harness) as(t *testing.T, tenant shared.TenantID, role shared.Role) context.Context {
	t.Helper()
	p := testkit.Staff(t, tenant, role)
	return authz.WithPrincipal(context.Background(), p)
}

func TestNewService_NamesTheMissingPort(t *testing.T) {
	t.Parallel()
	tx, clock := sharedfake.NewTxManager(), sharedfake.NewClock(time.Now())
	store, orders := fake.NewStore(time.UTC), fake.NewOrders()
	for want, build := range map[string]func() error{
		"Store":  func() error { _, err := appschedule.NewService(nil, orders, tx, clock); return err },
		"Orders": func() error { _, err := appschedule.NewService(store, nil, tx, clock); return err },
		"Tx":     func() error { _, err := appschedule.NewService(store, orders, nil, clock); return err },
		"Clock":  func() error { _, err := appschedule.NewService(store, orders, tx, nil); return err },
	} {
		if err := build(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %s: err = %v", want, err)
		}
	}
}

// At 03:30 UTC it is already the 2nd in São Paulo (UTC-3) and still the 1st
// in Manaus (UTC-4).
func TestToday_IsTheStoresDay(t *testing.T) {
	t.Parallel()
	for zone, want := range map[string]string{"America/Sao_Paulo": "2026-12-02", "America/Manaus": "2026-12-01"} {
		h := newHarness(t, zone)
		day, loc, err := h.svc.Today(h.as(t, h.order.TenantID(), shared.RoleViewer))
		if err != nil || day != testkit.Date(t, want) || loc.String() != zone {
			t.Errorf("%s: today = %s in %v, %v; want %s", zone, day, loc, err, want)
		}
	}
}

func TestToday_NeedsACaller(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "America/Sao_Paulo")
	if _, _, err := h.svc.Today(context.Background()); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("err = %v", err)
	}
}

func TestRecalculate_NeedsOrderWrite(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "America/Sao_Paulo")
	tenant, id := h.order.TenantID(), h.order.ID()
	if err := h.svc.Recalculate(h.as(t, tenant, shared.RoleViewer), id); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("viewer: err = %v, want forbidden", err)
	}
	for _, role := range []shared.Role{shared.RoleStaff, shared.RoleAdmin, shared.RoleOwner} {
		if err := h.svc.Recalculate(h.as(t, tenant, role), id); err != nil {
			t.Errorf("%s: %v", role, err)
		}
	}
	if err := h.svc.Recalculate(context.Background(), id); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("no caller: err = %v", err)
	}
}

func TestRecalculate_UnknownOrAnotherStoresOrderIsNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "America/Sao_Paulo")
	tenant := h.order.TenantID()
	if err := h.svc.Recalculate(h.as(t, tenant, shared.RoleStaff), shared.OrderID(testkit.NextID())); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("unknown: err = %v", err)
	}
	other := shared.TenantID(testkit.NextID())
	if err := h.svc.Recalculate(h.as(t, other, shared.RoleOwner), h.order.ID()); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("another store's order: err = %v", err)
	}
}

func TestRecalculate_ChangesNothingUntilMilestone5(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "America/Sao_Paulo")
	ctx := h.as(t, h.order.TenantID(), shared.RoleStaff)
	before := h.order.Record()
	if err := h.svc.Recalculate(ctx, h.order.ID()); err != nil {
		t.Fatal(err)
	}
	if after := h.order.Record(); after.Version != before.Version || after.Promise != before.Promise {
		t.Errorf("order changed: %+v -> %+v", before, after)
	}
}
