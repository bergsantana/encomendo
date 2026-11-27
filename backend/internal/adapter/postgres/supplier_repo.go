package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// Suppliers implements appsupplier.Repository on suppliers. The table is
// staff-only under RLS (0009): a scope with no user, or another store's
// scope, sees and writes nothing, whatever the query.
type Suppliers struct{}

// NewSuppliers returns the suppliers repository.
func NewSuppliers() *Suppliers { return &Suppliers{} }

// Insert stores a new supplier at its version. A taken id, of any store, is
// an error matching shared.ErrConflict.
func (*Suppliers) Insert(ctx context.Context, s *supplier.Supplier) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.SupplierFromDomain(s)
	return wrap("insert supplier", db.Create(&m).Error)
}

// Update stores the name and number while the stored version is still
// s.Version(), and the next version. It returns shared.ErrVersionConflict for
// a stale version and supplier.ErrNotFound when the supplier is gone or not
// visible to the scope.
func (*Suppliers) Update(ctx context.Context, s *supplier.Supplier) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	m := models.SupplierFromDomain(s)
	res := db.Model(&models.Supplier{}).
		Where("tenant_id = ? AND id = ? AND version = ?", m.TenantID, m.ID, s.Version()).
		Updates(map[string]any{
			"name": m.Name, "whatsapp_phone": m.WhatsAppPhone,
			"version": s.Version() + 1, "updated_at": gorm.Expr("now()"),
		})
	ok, err := affected(res, "update supplier")
	if err != nil || ok {
		return err
	}
	return missingOrStale(db, "suppliers", s.TenantID(), m.ID, supplier.ErrNotFound)
}

// ByID returns the supplier, or supplier.ErrNotFound.
func (*Suppliers) ByID(ctx context.Context, tenant shared.TenantID, id shared.SupplierID) (*supplier.Supplier, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var m models.Supplier
	err = db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, supplier.ErrNotFound
	}
	if err != nil {
		return nil, wrap("read supplier", err)
	}
	return m.ToDomain()
}

// List returns one page of the store's suppliers by lower(name), then id.
func (*Suppliers) List(ctx context.Context, tenant shared.TenantID, filter appsupplier.ListFilter, page shared.PageRequest) (shared.Page[*supplier.Supplier], error) {
	db, err := conn(ctx)
	if err != nil {
		return shared.Page[*supplier.Supplier]{}, err
	}
	rows, next, err := namePageRows(db, namePage{
		table: "suppliers", tenant: tenant, req: page,
		narrow: func(q *gorm.DB) *gorm.DB {
			if filter.Query == "" {
				return q
			}
			return q.Where("name ILIKE ?", containsPattern(filter.Query))
		},
	}, func(m models.Supplier) string { return m.ID })
	if err != nil {
		return shared.Page[*supplier.Supplier]{}, err
	}
	out := shared.Page[*supplier.Supplier]{Items: make([]*supplier.Supplier, 0, len(rows)), NextCursor: next}
	for _, m := range rows {
		s, err := m.ToDomain()
		if err != nil {
			return shared.Page[*supplier.Supplier]{}, err
		}
		out.Items = append(out.Items, s)
	}
	return out, nil
}

// ByIDs returns the suppliers with these ids that the scope sees; a missing
// id is simply absent.
func (*Suppliers) ByIDs(ctx context.Context, tenant shared.TenantID, ids []shared.SupplierID) ([]*supplier.Supplier, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []*supplier.Supplier{}, nil
	}
	strs := make([]string, 0, len(ids))
	for _, id := range ids {
		strs = append(strs, id.String())
	}
	var rows []models.Supplier
	if err := db.Where("tenant_id = ? AND id IN ?", tenant.String(), strs).Find(&rows).Error; err != nil {
		return nil, wrap("read suppliers", err)
	}
	out := make([]*supplier.Supplier, 0, len(rows))
	for _, m := range rows {
		s, err := m.ToDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
