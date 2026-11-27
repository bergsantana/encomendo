//go:build integration

package rls_test

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

func ids(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	slices.Sort(out)
	return out, rows.Err()
}

func sorted(vs ...shared.TenantID) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.String())
	}
	slices.Sort(out)
	return out
}

// TestUserScope_ReadsOnlyTheUsersStores checks the store picker policies of
// ADR 0010: with no tenant set, a user sees their own memberships and the
// stores they belong to, and nothing of any other user or store.
func TestUserScope_ReadsOnlyTheUsersStores(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, appshared.Scope{UserID: w.both}, func(ctx context.Context, tx *sql.Tx) error {
		got, err := ids(ctx, tx, `SELECT tenant_id::text FROM memberships`)
		if err != nil {
			return err
		}
		if want := sorted(w.a, w.b); !slices.Equal(got, want) {
			t.Errorf("memberships in user scope = %v, want the user's own in A and B %v", got, want)
		}
		got, err = ids(ctx, tx, `SELECT id::text FROM tenants`)
		if err != nil {
			return err
		}
		if want := sorted(w.a, w.b); !slices.Equal(got, want) {
			t.Errorf("tenants in user scope = %v, want A and B %v", got, want)
		}
		for _, table := range []string{"tenant_settings", "invitations", "audit_log"} {
			if n, err := count(ctx, tx, "SELECT count(*) FROM "+table); err != nil || n != 0 {
				t.Errorf("%s in user scope: %d rows (err %v), want 0", table, n, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestUserScope_CannotWriteMembershipsOrStores: the user policies are SELECT
// only, so user-scoped code can never add itself to a store (ADR 0010).
func TestUserScope_CannotWriteMembershipsOrStores(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	user := appshared.Scope{UserID: w.both}

	for name, insert := range map[string]tenantTable{"memberships": tenantTables[1], "tenants": tenantTables[0]} {
		target := w.a
		if name == "tenants" {
			target = shared.TenantID(pgtest.NewID())
		}
		err := inScope(t, app, user, func(ctx context.Context, tx *sql.Tx) error {
			query, args := insert.insert(w, target)
			_, err := tx.ExecContext(ctx, query, args...)
			return err
		})
		if !refused(err) {
			t.Errorf("insert into %s in user scope: err = %v, want 42501", name, err)
		}
	}
	for _, query := range []string{
		`UPDATE memberships SET role = 'owner' WHERE user_id = $1`,
		`DELETE FROM memberships WHERE user_id = $1`,
		`UPDATE tenants SET name = 'Tomada' WHERE id IN (SELECT tenant_id FROM memberships WHERE user_id = $1)`,
	} {
		err := inScope(t, app, user, func(ctx context.Context, tx *sql.Tx) error {
			res, err := tx.ExecContext(ctx, query, w.both.String())
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n != 0 { // pgx always reports the count
				t.Errorf("%q in user scope changed %d rows, want 0", query, n)
			}
			return nil
		})
		if err != nil {
			t.Errorf("%q in user scope: %v", query, err)
		}
	}
}

// TestTenantScope_HidesTheUsersOtherStores: inside store A, the user policies
// are off, so a member of A and B sees only A (ADR 0010).
func TestTenantScope_HidesTheUsersOtherStores(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, appshared.Scope{TenantID: w.a, UserID: w.both}, func(ctx context.Context, tx *sql.Tx) error {
		got, err := ids(ctx, tx, `SELECT DISTINCT tenant_id::text FROM memberships`)
		if err != nil {
			return err
		}
		if want := sorted(w.a); !slices.Equal(got, want) {
			t.Errorf("memberships in store A = %v, want only A %v", got, want)
		}
		got, err = ids(ctx, tx, `SELECT id::text FROM tenants`)
		if err != nil {
			return err
		}
		if want := sorted(w.a); !slices.Equal(got, want) {
			t.Errorf("tenants in store A = %v, want only A %v", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestClientScope_SeesNoTenantRows: a client account sees nothing of any
// store but the orders linked to it (architecture 5.5, item 7), and the
// seeded orders are linked to no account. orders_tables_test.go covers the
// linked ones.
func TestClientScope_SeesNoTenantRows(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, appshared.Scope{ClientAccountID: w.account}, func(ctx context.Context, tx *sql.Tx) error {
		for _, tt := range tenantTables {
			if !tt.can("s") {
				continue
			}
			if n, err := count(ctx, tx, "SELECT count(*) FROM "+tt.name); err != nil || n != 0 {
				t.Errorf("%s as a client account: %d rows (err %v), want 0", tt.name, n, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestPooledConnection_ForgetsThePreviousTenant is the trap of ADR 0009: once
// a connection has set app.tenant_id, current_setting returns the empty
// string rather than NULL in later transactions. The helpers turn the empty
// string into NULL, so the next transaction on that connection sees no rows,
// whether it sets the settings to the empty string (as the TxManager does)
// or not at all.
func TestPooledConnection_ForgetsThePreviousTenant(t *testing.T) {
	w := seed(t)
	app := env.Open(t, pgtest.App)
	app.SetMaxOpenConns(1) // every transaction below gets the same connection
	ctx := t.Context()

	if err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		n, err := count(ctx, tx, `SELECT count(*) FROM memberships`)
		if err == nil && n == 0 {
			t.Error("store A sees no memberships; the case below would prove nothing")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := inScope(t, app, appshared.Scope{}, func(ctx context.Context, tx *sql.Tx) error {
		n, err := count(ctx, tx, `SELECT count(*) FROM memberships`)
		if err == nil && n != 0 {
			t.Errorf("zero scope after store A on one connection: %d memberships, want 0", n)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	tx, err := app.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }() // read only
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT current_setting('app.tenant_id', true)`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != "" {
		t.Fatalf("app.tenant_id on the reused connection = %q, want ''", raw)
	}
	if n, err := count(ctx, tx, `SELECT count(*) FROM tenants`); err != nil || n != 0 {
		t.Errorf("no settings at all on the reused connection: %d tenants (err %v), want 0", n, err)
	}
}
