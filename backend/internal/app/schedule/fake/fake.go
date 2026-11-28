// Package fake holds in-memory versions of app/schedule's driven ports.
package fake

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	appschedule "github.com/bergsantana/encomendo/backend/internal/app/schedule"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Store is an appschedule.Store with a fixed time zone.
type Store struct{ loc *time.Location }

var _ appschedule.Store = Store{}

// NewStore returns a store in loc.
func NewStore(loc *time.Location) Store { return Store{loc: loc} }

// Location implements appschedule.Store.
func (s Store) Location(context.Context) (*time.Location, error) { return s.loc, nil }

type key struct {
	tenant shared.TenantID
	id     shared.OrderID
}

// Orders is an appschedule.Orders in memory. Like the real repository it
// refuses calls outside a fake transaction and shows a store only its own
// orders.
type Orders struct {
	mu    sync.Mutex
	rows  map[key]*order.Order
	saves int
}

var _ appschedule.Orders = (*Orders)(nil)

// NewOrders returns an empty set of orders.
func NewOrders() *Orders { return &Orders{rows: map[key]*order.Order{}} }

// Add stores o.
func (r *Orders) Add(o *order.Order) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[key{o.TenantID(), o.ID()}] = o
}

// ByID implements appschedule.Orders.
func (r *Orders) ByID(ctx context.Context, tenant shared.TenantID, id shared.OrderID) (*order.Order, error) {
	if !sharedfake.InTx(ctx) {
		return nil, appshared.ErrNoTransaction
	}
	if appshared.ScopeFrom(ctx).TenantID != tenant {
		return nil, errors.New("fake: new row violates row-level security policy")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.rows[key{tenant, id}]
	if !ok {
		return nil, fmt.Errorf("fake: order %s: %w", id, shared.ErrNotFound)
	}
	return o, nil
}

// Update implements appschedule.Orders. The order is shared by pointer, so
// the fake only checks the transaction and the store, and counts the save.
func (r *Orders) Update(ctx context.Context, o *order.Order) error {
	if !sharedfake.InTx(ctx) {
		return appshared.ErrNoTransaction
	}
	if appshared.ScopeFrom(ctx).TenantID != o.TenantID() {
		return errors.New("fake: new row violates row-level security policy")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[key{o.TenantID(), o.ID()}]; !ok {
		return fmt.Errorf("fake: order %s: %w", o.ID(), shared.ErrNotFound)
	}
	r.saves++
	return nil
}

// Saves returns how many times Update stored an order.
func (r *Orders) Saves() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saves
}
