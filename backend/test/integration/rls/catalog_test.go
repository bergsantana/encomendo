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

// extraPolicies is every policy besides tenant_isolation and staff_only. Each must be
// SELECT only and apply only with no tenant set (ADR 0010), so it can never
// widen what a store's own transactions see or let anyone write. The
// client_read policies of 0013_orders (architecture 5.5, item 7) were
// reviewed with step 4.7.
var extraPolicies = []string{
	"memberships.own_memberships", "order_items.client_read", "order_lines.client_read",
	"orders.client_read", "payments.client_read", "tenants.member_stores",
}

// TestPolicies_OnlyTheListedExtrasWidenReads: tenant_isolation checks writes
// with the same expression it reads with, and any other policy is a listed,
// SELECT-only, no-tenant one. A new policy fails here until it is reviewed.
func TestPolicies_OnlyTheListedExtrasWidenReads(t *testing.T) {
	t.Parallel()
	db := env.Seeder(t).DB()
	asymmetric := tableNames(t, db, `SELECT tablename FROM pg_policies
		WHERE schemaname = 'public' AND policyname = 'tenant_isolation'
		  AND (cmd <> 'ALL' OR permissive <> 'PERMISSIVE' OR qual IS DISTINCT FROM with_check)`)
	if len(asymmetric) != 0 {
		t.Errorf("tenant_isolation policies that are not FOR ALL with USING = WITH CHECK: %v", asymmetric)
	}
	// staff_only and the write-anywhere, staff-read pairs of audit_log and
	// order_status_history narrow reads; TestStaffOnlyPolicies,
	// TestAuditLogPolicies and TestOrderHistoryPolicies pin them.
	got := tableNames(t, db, `SELECT tablename || '.' || policyname FROM pg_policies
		WHERE schemaname = 'public' AND policyname NOT IN ('tenant_isolation', 'staff_only')
		  AND tablename NOT IN ('audit_log', 'order_status_history')`)
	if !slices.Equal(got, extraPolicies) {
		t.Errorf("extra policies = %v, want exactly the reviewed %v", got, extraPolicies)
	}
	loose := tableNames(t, db, `SELECT tablename || '.' || policyname || ' ' || cmd || ': ' || qual FROM pg_policies
		WHERE schemaname = 'public' AND policyname NOT IN ('tenant_isolation', 'staff_only')
		  AND tablename NOT IN ('audit_log', 'order_status_history')
		  AND (cmd <> 'SELECT' OR qual NOT LIKE '((current_tenant_id() IS NULL) AND %')`)
	if len(loose) != 0 {
		t.Errorf("extra policies that are not SELECT only behind current_tenant_id() IS NULL: %v", loose)
	}
}

// TestNothingIsGrantedToPublic: every privilege is granted to a named role,
// on tables and on our own functions (the extensions' functions excepted).
func TestNothingIsGrantedToPublic(t *testing.T) {
	t.Parallel()
	db := env.Seeder(t).DB()
	public := tableNames(t, db, `SELECT DISTINCT table_name FROM information_schema.role_table_grants
		WHERE table_schema = 'public' AND grantee = 'PUBLIC'`)
	if len(public) != 0 {
		t.Errorf("tables granted to PUBLIC: %v", public)
	}
	functions := tableNames(t, db, `SELECT p.proname FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public'
		  AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = p.oid AND d.deptype = 'e')
		  AND has_function_privilege('public', p.oid, 'EXECUTE')`)
	if len(functions) != 0 {
		t.Errorf("functions anyone may execute: %v; REVOKE ALL … FROM PUBLIC", functions)
	}
	definers := tableNames(t, db, `SELECT p.proname FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public' AND p.prosecdef
		  AND NOT EXISTS (SELECT 1 FROM unnest(p.proconfig) c WHERE c LIKE 'search_path=%')`)
	if len(definers) != 0 {
		t.Errorf("SECURITY DEFINER functions without a fixed search_path: %v", definers)
	}
}

// TestStore_CannotChangeItsAddressOrStatus: a store may rename itself, but
// its slug and its status are not the app's to change (column grants), so a
// suspended store cannot lift its own suspension.
func TestStore_CannotChangeItsAddressOrStatus(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	for _, set := range []string{"status = 'active'", "slug = 'outro-endereco'", "id = id"} {
		err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "UPDATE tenants SET "+set+" WHERE id = $1", w.a.String())
			return err
		})
		if !refused(err) {
			t.Errorf("store A setting %s on itself: err = %v, want 42501", set, err)
		}
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

func tableNames(t *testing.T, db *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query, args...)
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
