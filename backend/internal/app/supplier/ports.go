package supplier

import (
	"context"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// UseCases is the driving port: what the supplier handlers call, and what
// adapter/inproc wraps so the catalog can read supplier names (Names).
type UseCases interface {
	Create(ctx context.Context, in CreateInput) (View, error)
	Replace(ctx context.Context, in ReplaceInput) (View, error)
	Get(ctx context.Context, id shared.SupplierID) (View, error)
	List(ctx context.Context, filter ListFilter, page shared.PageRequest) (shared.Page[View], error)
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
	// Insert stores a new supplier at its version. It returns an error
	// matching shared.ErrConflict when a supplier, of any store, already has
	// the id.
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
	// is opaque and holds the id of the last row of the previous page; the
	// repository finds that row's place in the order by id. A cursor whose
	// id does not parse, or names no supplier visible in the scope, is a
	// ValidationError on "cursor". Query is literal text: "%" and "_" match
	// only themselves. The last page has no NextCursor, also when it ends
	// exactly at a page boundary.
	List(ctx context.Context, tenant shared.TenantID, filter ListFilter, page shared.PageRequest) (shared.Page[*supplier.Supplier], error)
	// ByIDs returns the suppliers with these ids that exist, in any order.
	// A missing id is not an error; it is simply absent.
	ByIDs(ctx context.Context, tenant shared.TenantID, ids []shared.SupplierID) ([]*supplier.Supplier, error)
}
