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

// The parent rows an order-table insert needs, as data-modifying CTEs for
// the tenant in $1 (see catalog_tables_test.go). Phones and order numbers
// are random in a space of 1e9, with about ten rows per store, so a
// collision on their unique keys is a one-in-a-hundred-million event.
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

// twoClients is the client-read fixture: the first account has an order in
// store A and one in store B, the second an order in store A. Each order
// has a line, an item, its sourcing, a payment, a history entry and a
// message. seed supplies the stores' other rows (customers, addresses,
// counters, idempotency keys), none linked to a client.
type twoClients struct {
	w             world
	first, second shared.ClientAccountID
	firstOrders   []string
	secondOrder   string
}

// seedClients builds the twoClients fixture.
func seedClients(t *testing.T) twoClients {
	t.Helper()
	w := seed(t)
	s := env.Seeder(t)
	c := twoClients{w: w, first: s.ClientAccount(), second: s.ClientAccount()}
	c.firstOrders = []string{linkedOrder(t, s.DB(), w.a, c.first), linkedOrder(t, s.DB(), w.b, c.first)}
	c.secondOrder = linkedOrder(t, s.DB(), w.a, c.second)
	return c
}

// linkedOrder writes, as the superuser, an order of tenant linked to
// account, with a row in each table an order owns (line, item, sourcing,
// payment, history, message), and returns its id.
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
	res, err := db.ExecContext(t.Context(), `UPDATE orders SET client_account_id = $1 WHERE id = $2`, account.String(), id)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("link order %s: %d rows, want 1", id, n)
	}
	return id
}

// asClient is the scope of a client area request: the account, no store.
func asClient(account shared.ClientAccountID) appshared.Scope {
	return appshared.Scope{ClientAccountID: account}
}

// orderOf is the column through which each client-read table reaches its
// order.
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
// store as one of the first's, is invisible to the first, while the second
// client reads exactly that order.
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
	err = inScope(t, app, asClient(c.second), func(ctx context.Context, tx *sql.Tx) error {
		for _, table := range clientReadTables {
			total, err := count(ctx, tx, "SELECT count(*) FROM "+table)
			if err != nil {
				return err
			}
			own, err := count(ctx, tx, "SELECT count(*) FROM "+table+" WHERE "+orderOf[table]+" = $1", c.secondOrder)
			if err != nil {
				return err
			}
			if total != 1 || own != 1 {
				t.Errorf("%s as the second client: %d rows, %d of its order; want exactly its 1", table, total, own)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestClientRead_RefusesADirectInsertOnItsOwnOrder: a client scope writing
// straight onto its own order (whose parents it can read) is refused by the
// target table's own policy, not by a parent's (the trap of
// TestStaffOnlyTables_RefuseADirectInsert). Linking a new order to the
// client is refused before that, by the column grants
// (TestStore_CannotLinkAClientOrMoveAnOrder).
func TestClientRead_RefusesADirectInsertOnItsOwnOrder(t *testing.T) {
	t.Parallel()
	c := seedClients(t)
	app := env.Open(t, pgtest.App)
	own := c.firstOrders[0]
	inserts := map[string]string{
		"orders": `INSERT INTO orders (tenant_id, id, number, customer_id, workflow_id, workflow_version, status_key,
			delivery_date, assembly_days, safety_margin_days, deposit_percent, currency, subtotal_cents, handover,
			tracking_token_hash)
			SELECT tenant_id, gen_random_uuid(), number + 1, customer_id, workflow_id, workflow_version, status_key,
			delivery_date, 3, 2, 50, 'BRL', 100000, 'pickup', gen_random_bytes(32)
			FROM orders WHERE id = $1`,
		"order_lines": `INSERT INTO order_lines (tenant_id, id, order_id, position, price_cents)
			SELECT tenant_id, gen_random_uuid(), id, 2, 100 FROM orders WHERE id = $1`,
		"order_items": `INSERT INTO order_items (tenant_id, id, order_id, line_id, position, snapshot, sourcing_mode,
			quantity, status, expected_date)
			SELECT tenant_id, gen_random_uuid(), order_id, id, 2, '{}', 'to_order', 1, 'to_buy', current_date
			FROM order_lines WHERE order_id = $1`,
		"payments": `INSERT INTO payments (tenant_id, id, order_id, kind, amount_cents, currency, method, recorded_at)
			SELECT tenant_id, gen_random_uuid(), id, 'balance', 100, 'BRL', 'pix', now() FROM orders WHERE id = $1`,
	}
	for _, table := range clientReadTables {
		err := inScope(t, app, asClient(c.first), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, inserts[table], own)
			return err
		})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" || !strings.Contains(pgErr.Message, `"`+table+`"`) {
			t.Errorf("direct insert into %s as its client: err = %v, want 42501 from %s's own policy", table, err, table)
		}
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
			// Where encomendo_app has no UPDATE or DELETE grant (orders and
			// payments have no DELETE), the grant refuses before RLS; the
			// policy is what refuses the rest.
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
		for _, tt := range orderTables {
			if slices.Contains(clientReadTables, tt.name) || !tt.can("s") {
				continue
			}
			table := tt.name
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
// its own store's rows and nothing of the other store's orders, though the
// client has one there (the tenant guard of 5.5, item 7).
func TestClientRead_OffInEveryStoreScope(t *testing.T) {
	t.Parallel()
	c := seedClients(t)
	app := env.Open(t, pgtest.App)
	for _, store := range []struct {
		tenant shared.TenantID
		owner  shared.UserID
	}{{c.w.a, c.w.ownerA}, {c.w.b, c.w.ownerB}, {c.w.a, shared.UserID{}}} {
		// The last is milestone 7's shape: a store page with a logged-in
		// client, a tenant and a client account but no staff user.
		scope := appshared.Scope{TenantID: store.tenant, UserID: store.owner, ClientAccountID: c.first}
		err := inScope(t, app, scope, func(ctx context.Context, tx *sql.Tx) error {
			for _, table := range clientReadTables {
				others, err := count(ctx, tx, "SELECT count(*) FROM "+table+" WHERE tenant_id <> $1", store.tenant.String())
				if err != nil {
					return err
				}
				own, err := count(ctx, tx, "SELECT count(*) FROM "+table)
				if err != nil {
					return err
				}
				if others != 0 || own == 0 {
					t.Errorf("%s in store %s's scope with a client set: %d rows of other stores (want 0), %d own (want some)",
						table, store.tenant, others, own)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestOrderHistoryPolicies pins the pair 0013 gives order_status_history, the
// same as audit_log's (0010): inserts checked against the tenant alone,
// reads needing a staff user too, nothing else.
func TestOrderHistoryPolicies(t *testing.T) {
	t.Parallel()
	db := env.Seeder(t).DB()
	got := tableNames(t, db, `SELECT policyname || ' ' || cmd || ' ' || coalesce(qual, '-') || ' ' || coalesce(with_check, '-')
		FROM pg_policies WHERE schemaname = 'public' AND tablename = 'order_status_history'`)
	want := []string{
		"history_write INSERT - (tenant_id = current_tenant_id())",
		"staff_read SELECT " + staffOnlyQual + " -",
	}
	if !slices.Equal(got, want) {
		t.Errorf("order_status_history policies = %v, want %v", got, want)
	}
}

// TestOrderHistory_StorePageWritesButCannotRead: a store page scope (tenant,
// no user) writes its order's "created" entry, as a store page order does in
// milestone 7, but reads no entry of the store: they hold staff ids and
// free-text reasons (security review of 4.7).
func TestOrderHistory_StorePageWritesButCannotRead(t *testing.T) {
	t.Parallel()
	c := seedClients(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, storePage(c.w), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO order_status_history (tenant_id, id, order_id, kind, at, detail)
			VALUES ($1, gen_random_uuid(), $2, 'created', now(), '{}')`, c.w.a.String(), c.firstOrders[0]); err != nil {
			return err
		}
		n, err := count(ctx, tx, "SELECT count(*) FROM order_status_history")
		if err == nil && n != 0 {
			t.Errorf("order_status_history as a store page: %d rows, want 0", n)
		}
		return err
	})
	if err != nil {
		t.Fatalf("store page writing its history entry: %v", err)
	}
	err = inScope(t, app, storeA(c.w), func(ctx context.Context, tx *sql.Tx) error {
		n, err := count(ctx, tx, "SELECT count(*) FROM order_status_history")
		if err == nil && n == 0 {
			t.Error("order_status_history as store A's owner: 0 rows, want the store's history")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestStore_CannotLinkAClientOrMoveAnOrder: the column grants keep a store
// from linking an order or customer to a client account (milestone 7 adds a
// reviewed path) and from rewriting what an order is: its number, customer
// and workflow (security review of 4.7).
func TestStore_CannotLinkAClientOrMoveAnOrder(t *testing.T) {
	t.Parallel()
	c := seedClients(t)
	app := env.Open(t, pgtest.App)
	own := c.firstOrders[0]
	statements := map[string][]any{
		`INSERT INTO customers (tenant_id, id, name, whatsapp_phone, client_account_id)
			VALUES ($1, gen_random_uuid(), 'Maria', '5592981234567', $2)`: {c.w.a.String(), c.second.String()},
		`INSERT INTO orders (tenant_id, id, number, customer_id, workflow_id, workflow_version, status_key,
			delivery_date, assembly_days, safety_margin_days, deposit_percent, currency, subtotal_cents, handover,
			tracking_token_hash, client_account_id)
			SELECT tenant_id, gen_random_uuid(), number + 7, customer_id, workflow_id, workflow_version, status_key,
			delivery_date, 3, 2, 50, 'BRL', 100000, 'pickup', gen_random_bytes(32), $2::uuid
			FROM orders WHERE id = $1`: {own, c.second.String()},
		`UPDATE orders SET client_account_id = $2 WHERE id = $1`:                                         {own, c.second.String()},
		`UPDATE customers SET client_account_id = $2 WHERE tenant_id = $1`:                               {c.w.a.String(), c.second.String()},
		`UPDATE orders SET number = number + 1 WHERE id = $1`:                                            {own},
		`UPDATE orders SET customer_id = customer_id WHERE id = $1`:                                      {own},
		`UPDATE orders SET workflow_id = workflow_id, workflow_version = workflow_version WHERE id = $1`: {own},
		`UPDATE orders SET created_by = NULL WHERE id = $1`:                                              {own},
	}
	for query, args := range statements {
		err := inScope(t, app, storeA(c.w), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, query, args...)
			return err
		})
		if !refused(err) {
			t.Errorf("%s as store A: err = %v, want 42501", strings.Join(strings.Fields(query), " ")[:60], err)
		}
	}
}

// TestAuditTriggers_OnlyOnTheListedTables pins which tables are audited.
// audit_log is read by every staff role, viewers included, so a trigger on a
// table with costs or suppliers (item_costs, supplier_items,
// order_item_sourcing) or on the WhatsApp messages would leak them there.
func TestAuditTriggers_OnlyOnTheListedTables(t *testing.T) {
	t.Parallel()
	db := env.Seeder(t).DB()
	got := tableNames(t, db, `SELECT DISTINCT c.relname FROM pg_trigger g
		JOIN pg_class c ON c.oid = g.tgrelid JOIN pg_proc p ON p.oid = g.tgfoid
		WHERE p.proname = 'audit_row' AND NOT g.tgisinternal`)
	want := []string{
		"attributes", "categories", "customer_addresses", "customers", "invitations", "items", "kits",
		"memberships", "order_items", "order_lines", "orders", "payments", "tenant_settings",
	}
	if !slices.Equal(got, want) {
		t.Errorf("audited tables = %v, want %v", got, want)
	}
}

// TestAuditTrigger_OrdersLeaveTheTrackingHashOut: the order's audit rows
// never copy tracking_token_hash (audit_row drops every *_hash column).
func TestAuditTrigger_OrdersLeaveTheTrackingHashOut(t *testing.T) {
	t.Parallel()
	c := seedClients(t)
	db := env.Seeder(t).DB()
	var leaked int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM audit_log
		WHERE table_name = 'orders' AND (new_row ? 'tracking_token_hash' OR old_row ? 'tracking_token_hash')`).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	var audited int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM audit_log
		WHERE table_name = 'orders' AND new_row ->> 'id' = $1`, c.firstOrders[0]).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 || audited == 0 {
		t.Errorf("orders audit rows with the token hash = %d (want 0); rows for a seeded order = %d (want some)", leaked, audited)
	}
}
