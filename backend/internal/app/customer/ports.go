package customer

import (
	"context"

	"github.com/bergsantana/encomendo/backend/internal/domain/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// UseCases is the driving port: what the customer handlers call, and what
// adapter/inproc wraps for the order flow.
type UseCases interface {
	// List returns one page of the store's customers by name (order:read).
	List(ctx context.Context, f ListFilter, p shared.PageRequest) (shared.Page[Summary], error)
	// Get returns one customer (order:read).
	Get(ctx context.Context, id shared.CustomerID) (View, error)
	// ByPhone returns the customer with this WhatsApp number (order:read).
	ByPhone(ctx context.Context, phone string) (View, error)
	// Create adds a customer (order:write); ErrPhoneTaken for a number in use.
	Create(ctx context.Context, in CreateInput) (View, error)
	// Update replaces a customer (customer:write); ErrPhoneTaken for a
	// number another customer uses.
	Update(ctx context.Context, in UpdateInput) (View, error)
}

// Repository is the driven port that stores customers and their addresses
// (0012_customers).
//
// Both tables are under tenant_isolation: a method sees only the rows of the
// tenant in the transaction's scope. With another tenant's id it finds
// nothing and writes nothing. The methods take the tenant explicitly too, so
// the SQL filters on it as well. Every method runs inside TxManager.WithinTx
// and returns appshared.ErrNoTransaction outside it. A missing customer is
// customer.ErrNotFound; other failures pass through unchanged.
//
// A repository never writes the link to a client account: it reads it, and
// milestone 7 adds a reviewed way to set it.
type Repository interface {
	// Insert stores a new customer at its version, with its addresses in
	// order. It returns ErrPhoneTaken when another customer of the store has
	// the number, and an error matching shared.ErrConflict when a customer,
	// of any store, already has the id.
	Insert(ctx context.Context, c *customer.Customer) error
	// Update stores c's fields while the stored version is still
	// c.Version(), replaces its addresses as a set (an address whose id is
	// not in c is removed, one with a known id is updated, one with a new id
	// is added), and stores the next version. It returns
	// shared.ErrVersionConflict for a stale version, customer.ErrNotFound
	// when the customer is gone, ErrPhoneTaken when another customer has the
	// number, and an error matching shared.ErrInvalid when an address id
	// belongs to another customer.
	Update(ctx context.Context, c *customer.Customer) error
	// ByID returns the customer with its addresses in order.
	ByID(ctx context.Context, tenant shared.TenantID, id shared.CustomerID) (*customer.Customer, error)
	// ByPhone returns the store's customer with this number, or
	// customer.ErrNotFound.
	ByPhone(ctx context.Context, tenant shared.TenantID, phone string) (*customer.Customer, error)
	// List returns one page of the store's customers narrowed by f, ordered
	// by lower(name) and then id. The cursor is opaque and holds the id of
	// the last row of the previous page; the repository finds that row's
	// place in the order by id, so it still works after the row was renamed
	// or left the filter. A cursor whose id does not parse, or names no
	// customer visible in the scope, is a ValidationError on "cursor". The
	// last page has no NextCursor, also when it ends exactly at a page
	// boundary.
	List(ctx context.Context, tenant shared.TenantID, f ListFilter, p shared.PageRequest) (shared.Page[Summary], error)
}
