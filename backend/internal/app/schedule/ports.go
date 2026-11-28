package schedule

import (
	"context"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Calendar is the driving port other contexts and the worker use to ask
// which day it is for the store the caller works in.
type Calendar interface {
	// Today returns the store's current day and its time zone.
	Today(ctx context.Context) (shared.CivilDate, *time.Location, error)
}

// Store gives the store's time zone. adapter/inproc implements it over the
// same tenancy settings app/order's Store port reads.
type Store interface {
	// Location returns the store's time zone.
	Location(ctx context.Context) (*time.Location, error)
}

// Orders reads the orders Recalculate works on. adapter/inproc implements
// it over app/order's repository and workflow source; this package never
// imports app/order.
type Orders interface {
	// ByID returns the store's order restored with the workflow it was made
	// with, or an error matching shared.ErrNotFound.
	ByID(ctx context.Context, tenant shared.TenantID, id shared.OrderID) (*order.Order, error)
}
