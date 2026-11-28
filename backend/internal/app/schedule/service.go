package schedule

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Service implements Calendar and also offers Recalculate.
type Service struct {
	store  Store
	orders Orders
	tx     appshared.TxManager
	clock  appshared.Clock
}

var _ Calendar = (*Service)(nil)

// NewService returns a Service over its ports, or an error naming the
// first one that is missing.
func NewService(store Store, orders Orders, tx appshared.TxManager, clock appshared.Clock) (*Service, error) {
	switch {
	case store == nil:
		return nil, errors.New("schedule service: Store is required")
	case orders == nil:
		return nil, errors.New("schedule service: Orders is required")
	case tx == nil:
		return nil, errors.New("schedule service: Tx is required")
	case clock == nil:
		return nil, errors.New("schedule service: Clock is required")
	}
	return &Service{store: store, orders: orders, tx: tx, clock: clock}, nil
}

// Today returns the store's current day and time zone (order:read), found
// with the clock port in the store's zone: at 03:30 UTC it is already the
// next day in São Paulo and still the day before in Manaus.
func (s *Service) Today(ctx context.Context) (shared.CivilDate, *time.Location, error) {
	if _, err := authz.Require(ctx, identity.PermOrderRead); err != nil {
		return shared.CivilDate{}, nil, err
	}
	var (
		day shared.CivilDate
		loc *time.Location
	)
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if loc, err = s.store.Location(ctx); err != nil {
			return fmt.Errorf("store time zone: %w", err)
		}
		day = shared.DateOf(s.clock.Now(), loc)
		return nil
	})
	return day, loc, err
}

// Recalculate re-derives the expected dates of one order (order:write) and
// saves the order when any of them moved. It returns an error matching
// shared.ErrNotFound when the store has no such order.
//
// Every write that changes an item's timing already redates in its own
// transaction, so an order is usually current. What goes stale is the day
// itself: an order not yet released counts from today, and a time zone
// change moves today. Recalculate repairs that, taking each supplier's
// promised date where purchasing recorded one (ADR 0031). It saves nothing
// when no date moved, so a repeated call writes no version
// (docs/plans/m5-code-design.md section 3.3).
func (s *Service) Recalculate(ctx context.Context, id shared.OrderID) error {
	by, err := authz.Require(ctx, identity.PermOrderWrite)
	if err != nil {
		return err
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		o, err := s.orders.ByID(ctx, by.TenantID(), id)
		if err != nil {
			return fmt.Errorf("load order %s: %w", id, err)
		}
		loc, err := s.store.Location(ctx)
		if err != nil {
			return fmt.Errorf("store time zone: %w", err)
		}
		before := expectedDates(o)
		o.Redate(shared.DateOf(s.clock.Now(), loc))
		if slices.Equal(before, expectedDates(o)) {
			return nil
		}
		if err := s.orders.Update(ctx, o); err != nil {
			return fmt.Errorf("save order %s: %w", id, err)
		}
		return nil
	})
}

// expectedDates lists the expected date of every item, in order.
func expectedDates(o *order.Order) []shared.CivilDate {
	items := o.Items()
	dates := make([]shared.CivilDate, len(items))
	for i, it := range items {
		dates[i] = it.Expected()
	}
	return dates
}
