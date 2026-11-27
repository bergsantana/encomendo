//go:build integration

package rls_test

import (
	"context"
	"database/sql"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// The parent rows an order-table insert needs, as data-modifying CTEs for
// the tenant in $1 (see catalog_tables_test.go). Phones and numbers are
// random so repeated inserts in one store never collide on their unique
// keys.
const (
	cteCustomer = `c AS (INSERT INTO customers (tenant_id, id, name, whatsapp_phone)
		VALUES ($1::uuid, gen_random_uuid(), 'João Pereira',
			'5592' || lpad((floor(random() * 1000000000))::bigint::text, 9, '0')) RETURNING tenant_id, id)`
	cteOrder = `o AS (INSERT INTO orders (tenant_id, id, number, customer_id, workflow_id, workflow_version,
			status_key, delivery_date, assembly_days, safety_margin_days, deposit_percent, currency, subtotal_cents,
			handover, tracking_token_hash)
		SELECT c.tenant_id, gen_random_uuid(), 1 + floor(random() * 1000000000)::int, c.id, w.id, w.version,
			(SELECT key FROM workflow_statuses s WHERE s.workflow_id = w.id AND s.workflow_version = w.version
				AND s.position = 1), current_date + 20, 3, 2, 50, 'BRL', 100000, 'pickup', gen_random_bytes(32)
		FROM c, status_workflows w WHERE w.is_default RETURNING tenant_id, id)`
	cteLine = `l AS (INSERT INTO order_lines (tenant_id, id, order_id, position, price_cents)
		SELECT tenant_id, gen_random_uuid(), id, 1, 100000 FROM o RETURNING tenant_id, id, order_id)`
	cteOrderItem = `oi AS (INSERT INTO order_items (tenant_id, id, order_id, line_id, position, snapshot,
			sourcing_mode, quantity, status, expected_date)
		SELECT tenant_id, gen_random_uuid(), order_id, id, 1, '{"name": "Banco de couro"}', 'to_order', 1,
			'to_buy', current_date + 15 FROM l RETURNING tenant_id, id)`
)

// orderTables are the tables of 0012_customers to 0014_idempotency.
var orderTables = []tenantTable{
	{
		name: "customers", column: "tenant_id", grants: "siu", set: "notes = 'Prefere WhatsApp'", seeded: true,
		insert: insertSQL(`INSERT INTO customers (tenant_id, id, name, whatsapp_phone)
			VALUES ($1::uuid, gen_random_uuid(), 'João Pereira',
				'5592' || lpad((floor(random() * 1000000000))::bigint::text, 9, '0'))`),
	},
	{
		name: "customer_addresses", column: "tenant_id", grants: "siud", set: "city = 'Belém'", seeded: true,
		insert: insertSQL(`WITH ` + cteCustomer + `
			INSERT INTO customer_addresses (tenant_id, id, customer_id, position, label, cep, street, city, state)
			SELECT tenant_id, gen_random_uuid(), id, 1, 'Casa', '69057000', 'Rua A', 'Manaus', 'AM' FROM c`),
	},
	{
		name: "order_counters", column: "tenant_id", grants: "siu", set: "last_number = 9", seeded: true,
		insert: insertSQL(`INSERT INTO order_counters (tenant_id) VALUES ($1::uuid)`),
	},
	{
		name: "orders", column: "tenant_id", grants: "siu", set: "deposit_percent = 10", seeded: true,
		insert: insertSQL(`WITH ` + cteCustomer + `, ` + cteOrder + ` SELECT 1`),
	},
	{
		name: "order_lines", column: "tenant_id", grants: "siud", set: "position = 9", seeded: true,
		insert: insertSQL(`WITH ` + cteCustomer + `, ` + cteOrder + `, ` + cteLine + ` SELECT 1`),
	},
	{
		name: "order_items", column: "tenant_id", grants: "siud", set: "position = 9", seeded: true,
		insert: insertSQL(`WITH ` + cteCustomer + `, ` + cteOrder + `, ` + cteLine + `, ` + cteOrderItem + ` SELECT 1`),
	},
	{
		name: "order_item_sourcing", column: "tenant_id", grants: "siud", set: "supplier_name = 'Outro'",
		seeded: true, staffOnly: true,
		insert: insertSQL(`WITH ` + cteCustomer + `, ` + cteOrder + `, ` + cteLine + `, ` + cteOrderItem + `
			INSERT INTO order_item_sourcing (tenant_id, order_item_id, supplier_id, supplier_name, cost_cents)
			SELECT tenant_id, id, gen_random_uuid(), 'Couros Paulista', 40000 FROM oi`),
	},
	{
		name: "payments", column: "tenant_id", grants: "siu", set: "void_reason = 'Erro', voided_at = now()", seeded: true,
		insert: insertSQL(`WITH ` + cteCustomer + `, ` + cteOrder + `
			INSERT INTO payments (tenant_id, id, order_id, kind, amount_cents, currency, method, recorded_at)
			SELECT tenant_id, gen_random_uuid(), id, 'deposit', 50000, 'BRL', 'pix', now() FROM o`),
	},
	{
		name: "order_status_history", column: "tenant_id", grants: "si", set: "kind = 'date'", seeded: true,
		insert: insertSQL(`WITH ` + cteCustomer + `, ` + cteOrder + `
			INSERT INTO order_status_history (tenant_id, id, order_id, kind, at, detail)
			SELECT tenant_id, gen_random_uuid(), id, 'created', now(), '{}' FROM o`),
	},
	{
		name: "order_messages", column: "tenant_id", grants: "si", set: "text = 'Outro texto'", seeded: true, staffOnly: true,
		insert: insertSQL(`WITH ` + cteCustomer + `, ` + cteOrder + `
			INSERT INTO order_messages (tenant_id, id, order_id, kind, to_phone, text, url, at)
			SELECT tenant_id, gen_random_uuid(), id, 'date_changed', '5592991234567', 'Olá, João!',
				'https://wa.me/5592991234567?text=Ol%C3%A1', now() FROM o`),
	},
	{
		name: "idempotency_keys", column: "tenant_id", grants: "sid", set: "resource_id = resource_id", seeded: true,
		insert: insertSQL(`INSERT INTO idempotency_keys (tenant_id, key, request_hash, resource_id, expires_at)
			VALUES ($1::uuid, gen_random_uuid()::text, gen_random_bytes(32), gen_random_uuid(), now() + interval '1 day')`),
	},
}

// clientReadTables have the second, SELECT-only client_read policy
// (docs/architecture.md section 5.5, item 7).
var clientReadTables = []string{"orders", "order_lines", "order_items", "payments"}

// clients is two client accounts: one with an order in store A and one in
// store B, the other with an order in store A. Each order has a line, an
// item, its sourcing, a payment, a history entry and a message.
type clients struct {
	w             world
	first, second shared.ClientAccountID
	firstOrders   []string
	secondOrder   string
}

func seedClients(t *testing.T) clients {
	t.Helper()
	w := seed(t)
	s := env.Seeder(t)
	c := clients{w: w, first: s.ClientAccount(), second: s.ClientAccount()}
	c.firstOrders = []string{linkedOrder(t, s.DB(), w.a, c.first), linkedOrder(t, s.DB(), w.b, c.first)}
	c.secondOrder = linkedOrder(t, s.DB(), w.a, c.second)
	return c
}

// linkedOrder writes, as the superuser, an order of tenant linked to
// account, with one row in every order table, and returns its id.
func linkedOrder(t *testing.T, db *sql.DB, tenant shared.TenantID, account shared.ClientAccountID) string {
	t.Helper()
	var id string
	err := db.QueryRowContext(t.Context(), `WITH `+cteCustomer+`, `+cteOrder+`, `+cteLine+`, `+cteOrderItem+`,
		src AS (INSERT INTO order_item_sourcing (tenant_id, order_item_id, supplier_name, cost_cents)
			SELECT tenant_id, id, 'Couros Paulista', 40000 FROM oi),
		pay AS (INSERT INTO payments (tenant_id, id, order_id, kind, amount_cents, currency, method, recorded_at)
			SELECT tenant_id, gen_random_uuid(), id, 'deposit', 50000, 'BRL', 'pix', now() FROM o),
		his AS (INSERT INTO order_status_history (tenant_id, id, order_id, kind, at, detail)
			SELECT tenant_id, gen_random_uuid(), id, 'created', now(), '{}' FROM o),
		msg AS (INSERT INTO order_messages (tenant_id, id, order_id, kind, to_phone, text, url, at)
			SELECT tenant_id, gen_random_uuid(), id, 'created', '5592991234567', 'Olá', 'https://wa.me/5592991234567', now() FROM o)
		SELECT id FROM o`, tenant.String()).Scan(&id)
	if err != nil {
		t.Fatalf("seed a linked order: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE orders SET client_account_id = $1 WHERE id = $2`, account.String(), id); err != nil {
		t.Fatal(err)
	}
	return id
}

func asClient(account shared.ClientAccountID) appshared.Scope {
	return appshared.Scope{ClientAccountID: account}
}

// orderOf is how each client-read table reaches its order.
var orderOf = map[string]string{
	"orders": "id", "order_lines": "order_id", "order_items": "order_id", "payments": "order_id",
}

// TestClientRead_OwnOrdersAcrossStores: a client account reads the rows of
// its own linked orders, in both stores, and only those.
func TestClientRead_OwnOrdersAcrossStores(t *testing.T) {
	t.Parallel()
	c := seedClients(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, asClient(c.first), func(ctx context.Context, tx *sql.Tx) error {
		for _, table := range clientReadTables {
			total, err := count(ctx, tx, "SELECT count(*) FROM "+table)
			if err != nil {
				return err
			}
			own, err := count(ctx, tx, "SELECT count(*) FROM "+table+" WHERE "+orderOf[table]+" = ANY($1::uuid[])",
				"{"+c.firstOrders[0]+","+c.firstOrders[1]+"}")
			if err != nil {
				return err
			}
			if total != 2 || own != 2 {
				t.Errorf("%s as the first client: %d rows, %d of its orders; want its 2, one per store", table, total, own)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestClientRead_NotAnotherClients: the second client's order, in the same
// store as one of the first's, is invisible to the first.
func TestClientRead_NotAnotherClients(t *testing.T) {
	t.Parallel()
	c := seedClients(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, asClient(c.first), func(ctx context.Context, tx *sql.Tx) error {
		for _, table := range clientReadTables {
			n, err := count(ctx, tx, "SELECT count(*) FROM "+table+" WHERE "+orderOf[table]+" = $1", c.secondOrder)
			if err != nil {
				return err
			}
			if n != 0 {
				t.Errorf("%s as the first client: %d rows of the second client's order, want 0", table, n)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestClientRead_WritesRefused: the client policy is SELECT only, so a
// client scope can insert, change or delete nothing, not even its own rows.
func TestClientRead_WritesRefused(t *testing.T) {
	t.Parallel()
	c := seedClients(t)
	app := env.Open(t, pgtest.App)
	for _, tt := range orderTables {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := inScope(t, app, asClient(c.first), func(ctx context.Context, tx *sql.Tx) error {
				query, args := tt.insert(c.w, c.w.a)
				_, err := tx.ExecContext(ctx, query, args...)
				return err
			})
			if !refused(err) {
				t.Errorf("insert into %s as a client: err = %v, want 42501", tt.name, err)
			}
			for _, query := range []string{
				"UPDATE " + tt.name + " SET " + tt.set,
				"DELETE FROM " + tt.name,
			} {
				err := inScope(t, app, asClient(c.first), func(ctx context.Context, tx *sql.Tx) error {
					res, err := tx.ExecContext(ctx, query)
					if err != nil {
						return err
					}
					if n, _ := res.RowsAffected(); n != 0 {
						t.Errorf("%q as a client changed %d rows", query, n)
					}
					return nil
				})
				if err != nil && !refused(err) {
					t.Errorf("%q as a client: %v", query, err)
				}
			}
		})
	}
}

// TestClientRead_InternalTablesEmpty: what only staff may read stays out of
// the client area, even for the client's own orders: supplier and cost,
// messages, history, the store's customer records and its counters.
func TestClientRead_InternalTablesEmpty(t *testing.T) {
	t.Parallel()
	c := seedClients(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, asClient(c.first), func(ctx context.Context, tx *sql.Tx) error {
		for _, table := range []string{"order_item_sourcing", "order_messages", "order_status_history",
			"customers", "customer_addresses", "order_counters", "idempotency_keys"} {
			n, err := count(ctx, tx, "SELECT count(*) FROM "+table)
			if err != nil {
				return err
			}
			if n != 0 {
				t.Errorf("%s as a client: %d rows, want 0", table, n)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestClientRead_OffInEveryStoreScope: the client policy applies only with
// no tenant set, so a store scope with the client account also set reads
// nothing of another store's orders (the tenant guard of 5.5, item 7).
func TestClientRead_OffInEveryStoreScope(t *testing.T) {
	t.Parallel()
	c := seedClients(t)
	app := env.Open(t, pgtest.App)
	scope := appshared.Scope{TenantID: c.w.a, UserID: c.w.ownerA, ClientAccountID: c.first}
	err := inScope(t, app, scope, func(ctx context.Context, tx *sql.Tx) error {
		for _, table := range clientReadTables {
			n, err := count(ctx, tx, "SELECT count(*) FROM "+table+" WHERE tenant_id <> $1", c.w.a.String())
			if err != nil {
				return err
			}
			if n != 0 {
				t.Errorf("%s in store A's scope with a client set: %d rows of other stores, want 0", table, n)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
