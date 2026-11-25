package postgres

import (
	"context"

	"gorm.io/gorm"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

// conn returns the transaction in ctx, bound to ctx. Repositories run only
// inside TxManager.WithinTx: outside it no scope is set and RLS would show
// nothing, so they fail loudly with appshared.ErrNoTransaction instead.
//
// UPDATE is granted per column (0002, 0007), so an update names its columns,
// as in Select("role", "version", "updated_at") or a map. Save() and
// Select("*") also write id and tenant_id, which encomendo_app may not
// change: Postgres refuses them (42501) and the call becomes a 500.
func conn(ctx context.Context) (*gorm.DB, error) {
	c, ok := ctx.Value(txKey{}).(*txConn)
	if !ok {
		return nil, appshared.ErrNoTransaction
	}
	return c.db.WithContext(ctx), nil
}
