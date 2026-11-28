package schedule

import (
	"context"
	"errors"
	"fmt"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
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

// Recalculate is the entry point that re-derives the expected dates of one
// order (order:write); today it only checks access, see below. It returns
// an error matching shared.ErrNotFound when the store has no such order.
//
// Until milestone 5 records supplier promised dates, an item's expected
// date is a function of the release date, its lead days and its options,
// and every write that changes one of them (Change, Release, the deposit,
// MoveItem, UseBackupSupplier) already re-derives the dates in the same
// transaction. So there is nothing stale to repair yet, and the domain has
// no operation that re-dates an order on its own: Recalculate checks the
// caller and the order and saves nothing. Milestone 5 adds the domain
// operation and has this method save the order with it, in the same
// transaction (docs/plans/m4-code-design.md section 3.7).
func (s *Service) Recalculate(ctx context.Context, id shared.OrderID) error {
	by, err := authz.Require(ctx, identity.PermOrderWrite)
	if err != nil {
		return err
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.orders.ByID(ctx, by.TenantID(), id); err != nil {
			return fmt.Errorf("load order %s: %w", id, err)
		}
		return nil
	})
}
