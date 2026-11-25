//go:build integration

// Package rls_test proves tenant isolation in the database itself (ADR 0002):
// connected as encomendo_app, with the settings the TxManager sets, no
// transaction can read or change another tenant's rows, and a transaction
// with no tenant sees none. It runs SQL directly, so it checks the policies
// and grants, not the Go code above them.
package rls_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

var env *pgtest.Env

func TestMain(m *testing.M) {
	os.Exit(pgtest.Run(m, &env))
}

// setScopeSQL is what the Postgres TxManager runs at the start of every
// transaction (ADR 0009).
const setScopeSQL = `SELECT set_config('app.tenant_id', $1, true),
	set_config('app.user_id', $2, true),
	set_config('app.client_account_id', $3, true)`

type id interface {
	IsZero() bool
	String() string
}

// setting is how the TxManager writes an id: the empty string when unset.
func setting(v id) string {
	if v.IsZero() {
		return ""
	}
	return v.String()
}

// inScope runs fn as encomendo_app in a transaction with scope s, then rolls
// back, so no case leaves rows behind.
func inScope(t *testing.T, db *sql.DB, s appshared.Scope, fn func(ctx context.Context, tx *sql.Tx) error) error {
	t.Helper()
	ctx := t.Context()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }() // every case rolls back on purpose
	if _, err := tx.ExecContext(ctx, setScopeSQL,
		setting(s.TenantID), setting(s.UserID), setting(s.ClientAccountID)); err != nil {
		t.Fatalf("set scope: %v", err)
	}
	return fn(ctx, tx)
}

// refused reports whether err is Postgres refusing the statement for
// privileges: a missing grant or a row that fails a policy's WITH CHECK.
func refused(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42501"
}

func count(ctx context.Context, tx *sql.Tx, query string, args ...any) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}
