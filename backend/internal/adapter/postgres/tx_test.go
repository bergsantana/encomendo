package postgres

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// These cases never reach the database, so they run without Postgres; the
// rest of the TxManager is tested in tx_integration_test.go.

var (
	unitTenant  = shared.TenantID(shared.NewV7(1_700_000_000_000, [10]byte{1}))
	unitUser    = shared.UserID(shared.NewV7(1_700_000_000_000, [10]byte{2}))
	unitAccount = shared.ClientAccountID(shared.NewV7(1_700_000_000_000, [10]byte{3}))
)

func TestTxManager_RefusesBeforeTouchingTheDatabase(t *testing.T) {
	tx := &TxManager{} // no database: any query would panic
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	mixed := appshared.WithScope(context.Background(), appshared.Scope{TenantID: unitTenant, ClientAccountID: unitAccount})
	outer := context.WithValue(appshared.WithScope(context.Background(), appshared.Scope{TenantID: unitTenant}),
		txKey{}, &txConn{scope: appshared.Scope{TenantID: unitTenant}})
	nestedOther := appshared.WithScope(outer, appshared.Scope{UserID: unitUser})

	for name, c := range map[string]struct {
		ctx  context.Context
		want error
	}{
		"canceled context":           {canceled, context.Canceled},
		"client mixed with a tenant": {mixed, appshared.ErrInvalidScope},
		"nested with another scope":  {nestedOther, appshared.ErrInvalidScope},
	} {
		called := false
		err := tx.WithinTx(c.ctx, func(context.Context) error {
			called = true
			return nil
		})
		if !errors.Is(err, c.want) || called {
			t.Errorf("%s: WithinTx = %v, called %v; want %v and no call", name, err, called, c.want)
		}
	}
}

func TestConn_NeedsATransaction(t *testing.T) {
	if _, err := conn(context.Background()); !errors.Is(err, appshared.ErrNoTransaction) {
		t.Errorf("conn outside WithinTx = %v, want ErrNoTransaction", err)
	}
}

func TestSetting_IsEmptyForAnUnsetID(t *testing.T) {
	if got := setting(shared.TenantID{}); got != "" {
		t.Errorf("setting(zero) = %q, want ''", got)
	}
	if got := setting(unitTenant); got != unitTenant.String() {
		t.Errorf("setting(%v) = %q, want the canonical form", unitTenant, got)
	}
}
