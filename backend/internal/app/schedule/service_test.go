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
	h.order = testkit.NewOrder().WithItems(2).Build(t)
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

// The order was made on the 1st and is not released, so its dates count from
// today: on the 2nd in São Paulo they are stale. An item the supplier
// promised a date for keeps counting from that date.
func TestRecalculate_SavesRedate(t *testing.T) {
	t.Parallel()
	h := newHarness(t, "America/Sao_Paulo")
	ctx := h.as(t, h.order.TenantID(), shared.RoleStaff)
	items := h.order.Items()
	promised := testkit.Date(t, "2026-12-20")
	owner := testkit.Staff(t, h.order.TenantID(), shared.RoleOwner)
	today, now := testkit.Date(t, "2026-12-01"), time.Date(2026, 12, 1, 15, 0, 0, 0, time.UTC)
	move := order.PurchaseMove{Item: items[0].ID(), To: order.ItemSentToSupplier}
	if err := h.order.MovePurchased(move, today, now, owner); err != nil {
		t.Fatal(err)
	}
	if err := h.order.RecordPromisedDate(items[0].ID(), promised, "C-0001", today, now, owner); err != nil {
		t.Fatal(err)
	}
	promisedExpected := h.order.Items()[0].Expected()

	if err := h.svc.Recalculate(ctx, h.order.ID()); err != nil {
		t.Fatal(err)
	}
	if got := h.orders.Saves(); got != 1 {
		t.Fatalf("saves after a stale order = %d, want 1", got)
	}
	got := h.order.Items()
	if got[0].Expected() != promisedExpected {
		t.Errorf("promised item expected %s, want %s", got[0].Expected(), promisedExpected)
	}
	if want := testkit.Date(t, "2026-12-07"); got[1].Expected() != want {
		t.Errorf("unpromised item expected %s, want %s (5 lead days from the 2nd)", got[1].Expected(), want)
	}

	if err := h.svc.Recalculate(ctx, h.order.ID()); err != nil {
		t.Fatal(err)
	}
	if got := h.orders.Saves(); got != 1 {
		t.Errorf("saves after a current order = %d, want still 1", got)
	}
}
