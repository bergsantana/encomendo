//go:build integration

package rls_test

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// TestEveryTenantTableIsListed keeps tenantTables complete: every table with
// a tenant_id column, plus tenants itself, must be in it, so a new tenant
// table cannot skip the five cases.
func TestEveryTenantTableIsListed(t *testing.T) {
	t.Parallel()
	db := env.Seeder(t).DB()
	got := tableNames(t, db, `SELECT table_name FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name = 'tenant_id'
		  AND table_name NOT IN ('refresh_tokens')
		UNION SELECT 'tenants'`)
	var want []string
	for _, tt := range tenantTables {
		want = append(want, tt.name)
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("tables with a tenant_id = %v, but the RLS suite covers %v", got, want)
	}
}

// TestEveryTenantTable_ForcesRLSThroughTheHelpers guards migrations to come:
// every tenant table has RLS enabled and forced (the owner is bound too),
// and no policy reads a setting except through the 0005 helpers, because a
// direct cast of the empty string to uuid fails instead of matching
// nothing (ADR 0009).
func TestEveryTenantTable_ForcesRLSThroughTheHelpers(t *testing.T) {
	t.Parallel()
	db := env.Seeder(t).DB()
	loose := tableNames(t, db, `SELECT c.relname FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r'
		  AND (c.relname = 'tenants' OR EXISTS (
		      SELECT 1 FROM pg_attribute a WHERE a.attrelid = c.oid AND a.attname = 'tenant_id' AND NOT a.attisdropped))
		  AND c.relname <> 'refresh_tokens'
		  AND NOT (c.relrowsecurity AND c.relforcerowsecurity)`)
	if len(loose) != 0 {
		t.Errorf("tenant tables without ENABLE and FORCE ROW LEVEL SECURITY: %v", loose)
	}
	direct := tableNames(t, db, `SELECT tablename || '.' || policyname FROM pg_policies
		WHERE schemaname = 'public'
		  AND (coalesce(qual, '') LIKE '%current_setting%' OR coalesce(with_check, '') LIKE '%current_setting%')`)
	if len(direct) != 0 {
		t.Errorf("policies that read current_setting directly: %v", direct)
	}
}

// TestNothingIsGrantedToPublic: every privilege is granted to a named role.
func TestNothingIsGrantedToPublic(t *testing.T) {
	t.Parallel()
	db := env.Seeder(t).DB()
	public := tableNames(t, db, `SELECT DISTINCT table_name FROM information_schema.role_table_grants
		WHERE table_schema = 'public' AND grantee = 'PUBLIC'`)
	if len(public) != 0 {
		t.Errorf("tables granted to PUBLIC: %v", public)
	}
}

// TestRefreshTokensHaveNoPolicy documents the one global table with a
// tenant_id: refresh tokens are found by hash before any scope exists
// (ADR 0010), so RLS cannot guard them. Only app/identity reads them.
func TestRefreshTokensHaveNoPolicy(t *testing.T) {
	t.Parallel()
	db := env.Seeder(t).DB()
	var rls bool
	if err := db.QueryRowContext(t.Context(),
		`SELECT relrowsecurity FROM pg_class WHERE relname = 'refresh_tokens'`).Scan(&rls); err != nil {
		t.Fatal(err)
	}
	if rls {
		t.Error("refresh_tokens has RLS; login and refresh look tokens up before any tenant is known")
	}
}

// TestWorkflowPresets_ReadOnlyToTheApp: the global status workflow tables are
// readable in every scope and writable in none (ADR 0006).
func TestWorkflowPresets_ReadOnlyToTheApp(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	for name, scope := range map[string]appshared.Scope{
		"no scope": {}, "store A": storeA(w), "user": {UserID: w.both}, "client": {ClientAccountID: w.account},
	} {
		err := inScope(t, app, scope, func(ctx context.Context, tx *sql.Tx) error {
			for _, table := range []string{"status_workflows", "workflow_statuses"} {
				if n, err := count(ctx, tx, "SELECT count(*) FROM "+table); err != nil || n == 0 {
					t.Errorf("%s in %s: %d rows (err %v), want the presets", table, name, n, err)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`INSERT INTO status_workflows (id, version, key, name) VALUES ('01a20ada-2c00-7000-8000-0000000000ff', 1, 'nova', 'Nova')`,
		`UPDATE workflow_statuses SET label = 'Trocado'`,
		`DELETE FROM workflow_statuses`,
		`UPDATE status_workflows SET is_default = NOT is_default`,
	} {
		err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, query)
			return err
		})
		if !refused(err) {
			t.Errorf("%q as the app: err = %v, want 42501", query, err)
		}
	}
}

func tableNames(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}
