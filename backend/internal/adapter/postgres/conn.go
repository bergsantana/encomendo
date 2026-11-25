package postgres

import (
	"context"

	"gorm.io/gorm"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

// conn returns the transaction in ctx, bound to ctx. Repositories run only
// inside TxManager.WithinTx: outside it no scope is set and RLS would show
// nothing, so they fail loudly with appshared.ErrNoTransaction instead.
func conn(ctx context.Context) (*gorm.DB, error) {
	c, ok := ctx.Value(txKey{}).(*txConn)
	if !ok {
		return nil, appshared.ErrNoTransaction
	}
	return c.db.WithContext(ctx), nil
}
