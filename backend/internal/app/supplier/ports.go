package supplier

import (
	"context"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// UseCases is the driving port: what the supplier handlers call, and what
// adapter/inproc wraps so the catalog can read supplier names (Names).
type UseCases interface {
	Create(ctx context.Context, in CreateInput) (SupplierView, error)
	Replace(ctx context.Context, in ReplaceInput) (SupplierView, error)
	Get(ctx context.Context, id shared.SupplierID) (SupplierView, error)
	List(ctx context.Context, filter ListFilter, page shared.PageRequest) (shared.Page[SupplierView], error)
	Names(ctx context.Context, ids []shared.SupplierID) (map[shared.SupplierID]string, error)
}

// Repository is the driven port that stores suppliers (0009_suppliers).
//
// The suppliers table is staff-only under RLS: a method sees only the rows
// of the tenant in the transaction's scope, and only when the scope also has
// a user. With another tenant's id, or with no user, it finds nothing and
// writes nothing. The methods take the tenant explicitly too, so the SQL
// filters on it as well. Every method runs inside TxManager.WithinTx and
// returns appshared.ErrNoTransaction outside it. A missing supplier is
// supplier.ErrNotFound; other failures pass through unchanged.
type Repository interface {
	// Insert stores a new supplier at its version.
	Insert(ctx context.Context, s *supplier.Supplier) error
	// Update stores s's name and number while the stored version is still
	// s.Version(), and stores the next version. It returns
	// shared.ErrVersionConflict for a stale version and
	// supplier.ErrNotFound when the supplier is gone.
	Update(ctx context.Context, s *supplier.Supplier) error
	// ByID returns the supplier.
	ByID(ctx context.Context, tenant shared.TenantID, id shared.SupplierID) (*supplier.Supplier, error)
	// List returns one page of the store's suppliers whose name contains
	// filter.Query, ignoring case, ordered by name and then id. The cursor
	// is opaque: it marks the last row of the previous page, and one the
	// repository did not issue is a ValidationError on "cursor".
	List(ctx context.Context, tenant shared.TenantID, filter ListFilter, page shared.PageRequest) (shared.Page[*supplier.Supplier], error)
	// ByIDs returns the suppliers with these ids that exist, in any order.
	// A missing id is not an error; it is simply absent.
	ByIDs(ctx context.Context, tenant shared.TenantID, ids []shared.SupplierID) ([]*supplier.Supplier, error)
}
