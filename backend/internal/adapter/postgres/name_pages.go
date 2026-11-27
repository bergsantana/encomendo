package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// errStaffScope is what a write that needs staff-only rows returns in a
// scope with no user. It matches no sentinel, so it is a logged 500: the
// caller used the wrong scope (ADR 0027).
var errStaffScope = errors.New("postgres: this write needs a staff scope (a tenant and a user)")

// hasStaff reports whether the transaction's scope has a user. RLS hides the
// staff-only tables (supplier_items, item_costs) from a scope with a tenant
// and no user, so a read would rebuild a wrong aggregate and a write could
// never drop the rows it replaces. The repositories ask this before any SQL
// (ADR 0027), and read the scope from the transaction (txScope), the one RLS
// uses, not from ctx.
func hasStaff(ctx context.Context) bool {
	return !txScope(ctx).UserID.IsZero()
}

// likeEscaper makes text literal for ILIKE, whose escape character is the
// backslash.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// containsPattern returns the ILIKE pattern that matches names containing
// query as literal text.
func containsPattern(query string) string {
	return "%" + likeEscaper.Replace(query) + "%"
}

// encodeNameCursor is the opaque cursor of a name-ordered page: the id of
// its last row. The row's name is looked up when the cursor is used, so
// the cursor resolves even after the row was renamed or left the filter.
func encodeNameCursor(id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id))
}

// decodeNameCursor reads what encodeNameCursor wrote. Anything else,
// including text that decodes to another spelling of an id, is errBadCursor.
func decodeNameCursor(cursor string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", errBadCursor()
	}
	id, err := shared.ParseID(string(raw))
	if err != nil || id.String() != string(raw) {
		return "", errBadCursor()
	}
	return id.String(), nil
}

// namePage is one keyset page of a tenant table ordered by lower(name), then
// id. Narrow adds the caller's filters to the page query.
type namePage struct {
	table  string
	tenant shared.TenantID
	req    shared.PageRequest
	narrow func(*gorm.DB) *gorm.DB
}

// namePageRows reads the rows of one page of p, and the cursor of the next
// page, empty on the last one. A cursor naming no row of the tenant that the
// scope sees is a ValidationError on "cursor". The cursor's row is found
// whatever the filter, so a row that left it still marks the place.
func namePageRows[M any](db *gorm.DB, p namePage, idOf func(M) string) ([]M, string, error) {
	q := db.Model(new(M)).Where("tenant_id = ?", p.tenant.String())
	if p.narrow != nil {
		q = p.narrow(q)
	}
	if cursor := p.req.Cursor(); cursor != "" {
		id, err := decodeNameCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		key, err := cursorKey(db, p.table, p.tenant, id)
		if err != nil {
			return nil, "", err
		}
		q = q.Where("(lower(name), id) > (?, ?)", key, id)
	}
	var rows []M
	limit := p.req.Limit()
	if err := q.Order("lower(name), id").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", wrap("list "+p.table, err)
	}
	if len(rows) <= limit {
		return rows, "", nil
	}
	rows = rows[:limit]
	return rows, encodeNameCursor(idOf(rows[limit-1])), nil
}

// cursorKey returns lower(name) of the row a cursor names.
func cursorKey(db *gorm.DB, table string, tenant shared.TenantID, id string) (string, error) {
	var keys []string
	err := db.Table(table).Where("tenant_id = ? AND id = ?", tenant.String(), id).
		Limit(1).Pluck("lower(name)", &keys).Error
	if err != nil {
		return "", wrap("read cursor row of "+table, err)
	}
	if len(keys) == 0 {
		return "", errBadCursor()
	}
	return keys[0], nil
}

// missingOrStale tells which of two causes made a conditional update of
// table match no row: the row is gone (or another store's, which RLS and the
// tenant filter make look the same), so notFound; or it is there at another
// version, so shared.ErrVersionConflict.
func missingOrStale(db *gorm.DB, table string, tenant shared.TenantID, id string, notFound error) error {
	var n int64
	err := db.Table(table).Where("tenant_id = ? AND id = ?", tenant.String(), id).Count(&n).Error
	if err != nil {
		return wrap("check "+table+" after a lost update", err)
	}
	if n == 0 {
		return notFound
	}
	return fmt.Errorf("%s changed since it was read: %w", table, shared.ErrVersionConflict)
}
