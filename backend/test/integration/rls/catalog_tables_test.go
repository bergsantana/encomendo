//go:build integration

package rls_test

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// The parent rows a catalog insert needs, as data-modifying CTEs for the
// tenant in $1. Each insert below is one statement, so it is refused or
// allowed as a whole: in another store's scope its first parent already
// breaks the policy, and its foreign keys are checked at the end of the
// statement, when every parent is there.
const (
	cteItem = `i AS (INSERT INTO items (tenant_id, id, name, sourcing_mode, price_cents, currency, ready_days)
		VALUES ($1::uuid, gen_random_uuid(), 'Banco de couro', 'to_order', 120000, 'BRL', 15) RETURNING tenant_id, id)`
	cteOtherItem = `j AS (INSERT INTO items (tenant_id, id, name, sourcing_mode, price_cents, currency, ready_days)
		VALUES ($1::uuid, gen_random_uuid(), 'Banco simples', 'to_order', 60000, 'BRL', 7) RETURNING tenant_id, id)`
	cteKit = `k AS (INSERT INTO kits (tenant_id, id, name, price_cents, currency)
		VALUES ($1::uuid, gen_random_uuid(), 'Kit Scrambler', 300000, 'BRL') RETURNING tenant_id, id)`
	cteValue = `a AS (INSERT INTO attributes (tenant_id, id, name, position)
		VALUES ($1::uuid, gen_random_uuid(), 'Serve em ' || gen_random_uuid(), 0) RETURNING tenant_id, id),
		v AS (INSERT INTO attribute_values (tenant_id, id, attribute_id, label, position)
		SELECT tenant_id, gen_random_uuid(), id, 'CB 500X', 0 FROM a RETURNING tenant_id, id)`
	cteSupplier = `s AS (INSERT INTO suppliers (tenant_id, id, name, whatsapp_phone)
		VALUES ($1::uuid, gen_random_uuid(), 'Couros Paulista', '5511987654321') RETURNING tenant_id, id)`
)

// insertSQL is an insert function for a statement whose only argument is
// the tenant.
func insertSQL(query string) func(world, shared.TenantID) (string, []any) {
	return func(_ world, tenant shared.TenantID) (string, []any) { return query, []any{tenant.String()} }
}

// catalogTables are the tables of 0008_catalog and 0009_suppliers.
var catalogTables = []tenantTable{
	{
		name: "categories", column: "tenant_id", grants: "siud", set: "position = 9", seeded: true,
		insert: insertSQL(`INSERT INTO categories (tenant_id, id, name, position)
			VALUES ($1::uuid, gen_random_uuid(), 'Bancos ' || gen_random_uuid(), 0)`),
	},
	{
		name: "attributes", column: "tenant_id", grants: "siud", set: "position = 9", seeded: true,
		insert: insertSQL(`INSERT INTO attributes (tenant_id, id, name, position)
			VALUES ($1::uuid, gen_random_uuid(), 'Serve em ' || gen_random_uuid(), 0)`),
	},
	{
		name: "attribute_values", column: "tenant_id", grants: "siud", set: "position = 9", seeded: true,
		insert: insertSQL(`WITH a AS (INSERT INTO attributes (tenant_id, id, name, position)
			VALUES ($1::uuid, gen_random_uuid(), 'Serve em ' || gen_random_uuid(), 0) RETURNING tenant_id, id)
			INSERT INTO attribute_values (tenant_id, id, attribute_id, label, position)
			SELECT tenant_id, gen_random_uuid(), id, 'CB 500X', 0 FROM a`),
	},
	{
		name: "items", column: "tenant_id", grants: "siu", set: "price_cents = 2000", seeded: true,
		insert: insertSQL(`INSERT INTO items (tenant_id, id, name, sourcing_mode, price_cents, currency, ready_days)
			VALUES ($1::uuid, gen_random_uuid(), 'Banco de couro', 'to_order', 120000, 'BRL', 15)`),
	},
	{
		name: "item_attribute_values", column: "tenant_id", grants: "sid", set: "value_id = value_id", seeded: true,
		insert: insertSQL(`WITH ` + cteItem + `, ` + cteValue + `
			INSERT INTO item_attribute_values (tenant_id, item_id, value_id) SELECT i.tenant_id, i.id, v.id FROM i, v`),
	},
	{
		name: "customization_options", column: "tenant_id", grants: "siud", set: "position = 9", seeded: true,
		insert: insertSQL(`WITH ` + cteItem + `
			INSERT INTO customization_options (tenant_id, id, item_id, name, required, position, choices)
			SELECT tenant_id, gen_random_uuid(), id, 'Cor da costura', true, 0,
			'[{"id": "01936d1e-8b5a-7c2e-9f3a-000000000001", "label": "Vermelha", "extraPriceCents": 0, "extraDays": 0}]'
			FROM i`),
	},
	{
		name: "item_costs", column: "tenant_id", grants: "siud", set: "cost_cents = 5", seeded: true, staffOnly: true,
		insert: insertSQL(`WITH ` + cteItem + `
			INSERT INTO item_costs (tenant_id, item_id, cost_cents, currency) SELECT tenant_id, id, 40000, 'BRL' FROM i`),
	},
	{
		name: "kits", column: "tenant_id", grants: "siu", set: "price_cents = 2000", seeded: true,
		insert: insertSQL(`INSERT INTO kits (tenant_id, id, name, price_cents, currency)
			VALUES ($1::uuid, gen_random_uuid(), 'Kit Scrambler', 300000, 'BRL')`),
	},
	{
		name: "kit_items", column: "tenant_id", grants: "siud", set: "quantity = 2", seeded: true,
		insert: insertSQL(`WITH ` + cteKit + `, ` + cteItem + `
			INSERT INTO kit_items (tenant_id, kit_id, item_id, quantity, swappable, position)
			SELECT k.tenant_id, k.id, i.id, 1, true, 0 FROM k, i`),
	},
	{
		name: "kit_item_alternatives", column: "tenant_id", grants: "sid", set: "item_id = item_id", seeded: true,
		insert: insertSQL(`WITH ` + cteKit + `, ` + cteItem + `, ` + cteOtherItem + `,
			ki AS (INSERT INTO kit_items (tenant_id, kit_id, item_id, quantity, swappable, position)
			SELECT k.tenant_id, k.id, i.id, 1, true, 0 FROM k, i RETURNING tenant_id, kit_id, item_id)
			INSERT INTO kit_item_alternatives (tenant_id, kit_id, item_id, alternative_item_id)
			SELECT ki.tenant_id, ki.kit_id, ki.item_id, j.id FROM ki, j`),
	},
	{
		name: "kit_attribute_values", column: "tenant_id", grants: "sid", set: "value_id = value_id", seeded: true,
		insert: insertSQL(`WITH ` + cteKit + `, ` + cteValue + `
			INSERT INTO kit_attribute_values (tenant_id, kit_id, value_id) SELECT k.tenant_id, k.id, v.id FROM k, v`),
	},
	{
		name: "photos", column: "tenant_id", grants: "siud", set: "position = 1", seeded: true,
		insert: insertSQL(`WITH ` + cteItem + `, p AS (SELECT gen_random_uuid() AS id)
			INSERT INTO photos (tenant_id, id, item_id, object_key, content_type, size_bytes, position)
			SELECT i.tenant_id, p.id, i.id, 't/' || i.tenant_id || '/items/' || i.id || '/' || p.id, 'image/jpeg', 2048, 0
			FROM i, p`),
	},
	{
		name: "suppliers", column: "tenant_id", grants: "siu", set: "name = 'Outro fornecedor'", seeded: true, staffOnly: true,
		insert: insertSQL(`INSERT INTO suppliers (tenant_id, id, name, whatsapp_phone)
			VALUES ($1::uuid, gen_random_uuid(), 'Couros Paulista', '5511987654321')`),
	},
	{
		name: "supplier_items", column: "tenant_id", grants: "siud", set: "lead_days = 9", seeded: true, staffOnly: true,
		insert: insertSQL(`WITH ` + cteItem + `, ` + cteSupplier + `
			INSERT INTO supplier_items (tenant_id, item_id, supplier_id, role, lead_days, cost_cents, currency)
			SELECT i.tenant_id, i.id, s.id, 'main', 15, 40000, 'BRL' FROM i, s`),
	},
}

// staffOnlyTables names the tables under the staff_only policy.
func staffOnlyTables() []string {
	var out []string
	for _, tt := range tenantTables {
		if tt.staffOnly {
			out = append(out, tt.name)
		}
	}
	slices.Sort(out)
	return out
}

// storePage is the scope of a store page request (milestone 7): its tenant
// and no user (docs/architecture.md section 5.5, item 5).
func storePage(w world) appshared.Scope { return appshared.Scope{TenantID: w.a} }

// TestStaffOnlyTables runs the sixth case of ADR 0024 on every internal
// table: a store page scope reads zero rows and writes none, though store
// A's own rows exist (the five cases above find them with a staff user).
// Supplier names, lead times by supplier and costs never reach the store
// page, whatever its queries do.
func TestStaffOnlyTables(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	if len(staffOnlyTables()) == 0 {
		t.Fatal("no staff-only table listed")
	}
	for _, tt := range tenantTables {
		if !tt.staffOnly {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := inScope(t, app, storePage(w), func(ctx context.Context, tx *sql.Tx) error {
				n, err := count(ctx, tx, "SELECT count(*) FROM "+tt.name)
				if err == nil && n != 0 {
					t.Errorf("%s as a store page: %d rows, want 0", tt.name, n)
				}
				return err
			})
			if err != nil {
				t.Fatalf("select %s as a store page: %v", tt.name, err)
			}
			err = inScope(t, app, storePage(w), func(ctx context.Context, tx *sql.Tx) error {
				query, args := tt.insert(w, w.a)
				_, err := tx.ExecContext(ctx, query, args...)
				return err
			})
			if !refused(err) {
				t.Errorf("insert into %s as a store page: err = %v, want 42501", tt.name, err)
			}
			for _, query := range []string{
				"UPDATE " + tt.name + " SET " + tt.set + " WHERE tenant_id = $1",
				"DELETE FROM " + tt.name + " WHERE tenant_id = $1",
			} {
				err := inScope(t, app, storePage(w), func(ctx context.Context, tx *sql.Tx) error {
					res, err := tx.ExecContext(ctx, query, w.a.String())
					if err != nil {
						return err
					}
					if n, _ := res.RowsAffected(); n != 0 {
						t.Errorf("%q as a store page changed %d rows", query, n)
					}
					return nil
				})
				if err != nil && !refused(err) {
					t.Errorf("%q as a store page: %v", query, err)
				}
			}
		})
	}
}

// TestStaffOnlyTables_RefuseADirectInsert: a store page scope writing an
// internal row straight onto store A's own seeded parents is refused by the
// internal table's own policy, not by a parent's (security review of 3.4).
func TestStaffOnlyTables_RefuseADirectInsert(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	db := env.Seeder(t).DB()
	var item, supplier string
	if err := db.QueryRowContext(t.Context(), `SELECT i.id FROM items i WHERE i.tenant_id = $1
		AND NOT EXISTS (SELECT 1 FROM item_costs c WHERE c.tenant_id = i.tenant_id AND c.item_id = i.id)
		AND NOT EXISTS (SELECT 1 FROM supplier_items s WHERE s.tenant_id = i.tenant_id AND s.item_id = i.id)
		LIMIT 1`, w.a.String()).Scan(&item); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM suppliers WHERE tenant_id = $1 LIMIT 1`, w.a.String()).Scan(&supplier); err != nil {
		t.Fatal(err)
	}
	for table, query := range map[string]string{
		"item_costs": `INSERT INTO item_costs (tenant_id, item_id, cost_cents, currency) VALUES ($1, $2, 100, 'BRL')`,
		"supplier_items": `INSERT INTO supplier_items (tenant_id, item_id, supplier_id, role, lead_days, currency)
			VALUES ($1, $2, $3, 'main', 3, 'BRL')`,
	} {
		err := inScope(t, app, storePage(w), func(ctx context.Context, tx *sql.Tx) error {
			args := []any{w.a.String(), item}
			if table == "supplier_items" {
				args = append(args, supplier)
			}
			_, err := tx.ExecContext(ctx, query, args...)
			return err
		})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" || !strings.Contains(pgErr.Message, `"`+table+`"`) {
			t.Errorf("direct insert into %s as a store page: err = %v, want 42501 from %s's own policy", table, err, table)
		}
	}
}

// TestAuditLog_StorePageWritesButCannotRead: a scope with a tenant and no
// user (store page, invitation preview) still writes the audit rows its
// changes trigger, but reads none, so drafts, roles and invitees' emails
// never reach it (0010_audit_log_staff_reads).
func TestAuditLog_StorePageWritesButCannotRead(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, storePage(w), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_log (tenant_id, table_name, action)
			VALUES ($1, 'items', 'insert')`, w.a.String()); err != nil {
			return err
		}
		n, err := count(ctx, tx, "SELECT count(*) FROM audit_log")
		if err == nil && n != 0 {
			t.Errorf("audit_log as a store page: %d rows, want 0", n)
		}
		return err
	})
	if err != nil {
		t.Fatalf("store page writing its audit row: %v", err)
	}
	err = inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		n, err := count(ctx, tx, "SELECT count(*) FROM audit_log")
		if err == nil && n == 0 {
			t.Error("audit_log as store A's owner: 0 rows, want the store's audit")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestAuditLogPolicies pins the pair 0010 gives audit_log: inserts checked
// against the tenant alone, reads needing a staff user too, nothing else.
func TestAuditLogPolicies(t *testing.T) {
	t.Parallel()
	db := env.Seeder(t).DB()
	got := tableNames(t, db, `SELECT policyname || ' ' || cmd || ' ' || coalesce(qual, '-') || ' ' || coalesce(with_check, '-')
		FROM pg_policies WHERE schemaname = 'public' AND tablename = 'audit_log' AND permissive = 'PERMISSIVE'`)
	want := []string{
		"audit_write INSERT - (tenant_id = current_tenant_id())",
		"staff_read SELECT " + staffOnlyQual + " -",
	}
	if !slices.Equal(got, want) {
		t.Errorf("audit_log policies = %v, want %v", got, want)
	}
	var restrictive int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM pg_policies
		WHERE tablename = 'audit_log' AND permissive <> 'PERMISSIVE'`).Scan(&restrictive); err != nil || restrictive != 0 {
		t.Errorf("restrictive audit_log policies: %d (err %v), want 0", restrictive, err)
	}
}

// staffOnlyQual is the one policy expression an internal table may have.
const staffOnlyQual = "((tenant_id = current_tenant_id()) AND (current_user_id() IS NOT NULL))"

// TestStaffOnlyPolicies: the internal tables, and only they, carry
// staff_only; it checks writes with the same expression it reads with; and
// they have no other policy, so no tenant_isolation and no client policy
// can widen them (ADR 0024; docs/architecture.md section 5.5, item 7).
func TestStaffOnlyPolicies(t *testing.T) {
	t.Parallel()
	db := env.Seeder(t).DB()
	got := tableNames(t, db, `SELECT tablename FROM pg_policies WHERE schemaname = 'public' AND policyname = 'staff_only'`)
	if want := staffOnlyTables(); !slices.Equal(got, want) {
		t.Errorf("tables with staff_only = %v, want the listed internal tables %v", got, want)
	}
	loose := tableNames(t, db, `SELECT tablename || ': ' || coalesce(qual, '') FROM pg_policies
		WHERE schemaname = 'public' AND policyname = 'staff_only'
		  AND (cmd <> 'ALL' OR permissive <> 'PERMISSIVE' OR qual IS DISTINCT FROM with_check OR qual <> $1)`,
		staffOnlyQual)
	if len(loose) != 0 {
		t.Errorf("staff_only policies not FOR ALL with USING = WITH CHECK = %s: %v", staffOnlyQual, loose)
	}
	others := tableNames(t, db, `SELECT tablename || '.' || policyname FROM pg_policies
		WHERE schemaname = 'public' AND policyname <> 'staff_only' AND tablename = ANY($1)`, staffOnlyTables())
	if len(others) != 0 {
		t.Errorf("internal tables with another policy: %v", others)
	}
}

// TestPhotos_KeyMatchesItsRow: a photo row can only name the object key the
// API issues for it, t/{its tenant}/{items|kits}/{its owner}/{its id}, so no
// row can point at another store's object (ADR 0025).
func TestPhotos_KeyMatchesItsRow(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	for name, key := range map[string]string{
		"another store's key":  "'t/' || $2 || '/items/' || i.id || '/' || p.id",
		"another owner's key":  "'t/' || i.tenant_id || '/items/' || gen_random_uuid() || '/' || p.id",
		"a kit's prefix":       "'t/' || i.tenant_id || '/kits/' || i.id || '/' || p.id",
		"another photo's id":   "'t/' || i.tenant_id || '/items/' || i.id || '/' || gen_random_uuid()",
		"a trailing segment":   "'t/' || i.tenant_id || '/items/' || i.id || '/' || p.id || '/x'",
		"a climb out of the t": "'t/' || i.tenant_id || '/items/' || i.id || '/../' || p.id",
	} {
		err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `WITH `+cteItem+`, p AS (SELECT gen_random_uuid() AS id)
				INSERT INTO photos (tenant_id, id, item_id, object_key, content_type, size_bytes, position)
				SELECT i.tenant_id, p.id, i.id, `+key+`, 'image/png', 10, 0 FROM i, p WHERE $2::uuid IS NOT NULL`,
				w.a.String(), w.b.String())
			return err
		})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("%s: err = %v, want a check violation (23514)", name, err)
		}
	}
}
