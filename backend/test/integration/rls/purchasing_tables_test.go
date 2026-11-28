//go:build integration

package rls_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// The parent rows a purchasing insert needs, as data-modifying CTEs for the
// tenant in $1, after cteCustomer, cteOrder, cteLine and cteOrderItem (whose
// oi gains order_id here through cteOrderItemFull).
const (
	cteOrderItemFull = `oi AS (INSERT INTO order_items (tenant_id, id, order_id, line_id, position, snapshot,
			sourcing_mode, quantity, status, expected_date)
		SELECT tenant_id, gen_random_uuid(), order_id, id, 1, '{"name": "Banco de couro"}', 'to_order', 1,
			'sent_to_supplier', current_date + 15 FROM l RETURNING tenant_id, id, order_id)`
	cteSupplierOrder = `so AS (INSERT INTO supplier_orders (tenant_id, id, number, supplier_id, supplier_name,
			supplier_phone, sent_at, sent_by, message_text, message_url)
		SELECT tenant_id, gen_random_uuid(), 1 + floor(random() * 1000000000)::int, id, 'Couros Paulista',
			'5511991234567', now(), gen_random_uuid(), 'Olá, Couros Paulista!', 'https://wa.me/5511991234567?text=Ol%C3%A1'
		FROM s RETURNING tenant_id, id, supplier_id)`
	ctePurchaseLine = `pl AS (INSERT INTO purchase_lines (tenant_id, id, supplier_order_id, supplier_id, order_id,
			order_item_id, catalog_item_id, name, quantity, status, position)
		SELECT so.tenant_id, gen_random_uuid(), so.id, so.supplier_id, oi.order_id, oi.id, gen_random_uuid(),
			'Banco de couro', 1, 'sent', 0 FROM so, oi RETURNING tenant_id, id)`
	cteReceipt = `r AS (INSERT INTO receipts (tenant_id, id, received_by, received_at)
		VALUES ($1::uuid, gen_random_uuid(), gen_random_uuid(), now()) RETURNING tenant_id, id)`
	// purchaseChain is every parent of a purchase line.
	purchaseChain = `WITH ` + cteCustomer + `, ` + cteOrder + `, ` + cteLine + `, ` + cteOrderItemFull + `, ` +
		cteSupplier + `, ` + cteSupplierOrder + `, ` + ctePurchaseLine
)

// purchasingTables are the tables of 0016_purchasing, all staff-only
// (ADR 0024, ADR 0031). Receipts are append-only.
var purchasingTables = []tenantTable{
	{
		name: "purchase_counters", column: "tenant_id", grants: "siu", set: "last_number = 9", seeded: true, staffOnly: true,
		insert: insertSQL(`INSERT INTO purchase_counters (tenant_id) VALUES ($1::uuid)`),
	},
	{
		name: "supplier_orders", column: "tenant_id", grants: "siu", set: "promised_date = current_date + 9",
		seeded: true, staffOnly: true,
		insert: insertSQL(`WITH ` + cteSupplier + `, ` + cteSupplierOrder + ` SELECT 1`),
	},
	{
		name: "purchase_lines", column: "tenant_id", grants: "siu", set: "check_note = 'riscado'", seeded: true, staffOnly: true,
		insert: insertSQL(purchaseChain + ` SELECT 1`),
	},
	{
		name: "purchase_line_costs", column: "tenant_id", grants: "siu", set: "paid_cost_cents = 41000",
		seeded: true, staffOnly: true,
		insert: insertSQL(purchaseChain + `
			INSERT INTO purchase_line_costs (tenant_id, line_id, currency, cost_cents) SELECT tenant_id, id, 'BRL', 40000 FROM pl`),
	},
	{
		name: "receipts", column: "tenant_id", grants: "si", set: "received_at = now()", seeded: true, staffOnly: true,
		insert: insertSQL(`WITH ` + cteReceipt + ` SELECT 1`),
	},
	{
		name: "receipt_lines", column: "tenant_id", grants: "si", set: "check_result = 'defective'", seeded: true, staffOnly: true,
		insert: insertSQL(purchaseChain + `, ` + cteReceipt + `
			INSERT INTO receipt_lines (tenant_id, receipt_id, line_id, check_result) SELECT r.tenant_id, r.id, pl.id, 'ok' FROM r, pl`),
	},
	{
		name: "receipt_photos", column: "tenant_id", grants: "si", set: "size_bytes = 99", seeded: true, staffOnly: true,
		insert: insertSQL(`WITH ` + cteReceipt + `, p AS (SELECT gen_random_uuid() AS id)
			INSERT INTO receipt_photos (tenant_id, id, receipt_id, object_key, content_type, size_bytes)
			SELECT r.tenant_id, p.id, r.id, 't/' || r.tenant_id || '/receipts/' || p.id, 'image/jpeg', 10 FROM r, p`),
	},
}

// TestPurchaseLines_OneOpenLinePerItem: an order item has at most one line
// sent or in transit, so two people sending it at once cannot both win
// (ADR 0031); a cancelled line does not count.
func TestPurchaseLines_OneOpenLinePerItem(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		var line string
		if err := tx.QueryRowContext(ctx, purchaseChain+` SELECT id FROM pl`, w.a.String()).Scan(&line); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO purchase_lines (tenant_id, id, supplier_order_id, supplier_id,
				order_id, order_item_id, catalog_item_id, name, quantity, status, position)
			SELECT tenant_id, gen_random_uuid(), supplier_order_id, supplier_id, order_id, order_item_id,
				catalog_item_id, name, quantity, 'in_transit', 1 FROM purchase_lines WHERE id = $1`, line)
		return err
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "purchase_lines_open_item_idx" {
		t.Errorf("a second open line: err = %v, want a unique violation on purchase_lines_open_item_idx", err)
	}
	err = inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		var line string
		if err := tx.QueryRowContext(ctx, purchaseChain+` SELECT id FROM pl`, w.a.String()).Scan(&line); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE purchase_lines SET status = 'cancelled', cancel_reason = 'undone' WHERE id = $1`, line); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO purchase_lines (tenant_id, id, supplier_order_id, supplier_id,
				order_id, order_item_id, catalog_item_id, name, quantity, status, position)
			SELECT tenant_id, gen_random_uuid(), supplier_order_id, supplier_id, order_id, order_item_id,
				catalog_item_id, name, quantity, 'sent', 1 FROM purchase_lines WHERE id = $1`, line)
		return err
	})
	if err != nil {
		t.Errorf("a new line after the old one was cancelled: %v", err)
	}
}

// TestPurchaseLine_KeepsRowWhenOrderItemRemoved: removing an order item in
// a change keeps its sent line, with a null item, its order and its own
// snapshot, so Compras can still flag it ("Avisar o fornecedor"). The
// referential action runs as the table owner, past RLS (security review of
// 5.1); the order repository's staff guard is what keeps a store page scope
// from deleting order items.
func TestPurchaseLine_KeepsRowWhenOrderItemRemoved(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		var line, item string
		if err := tx.QueryRowContext(ctx, purchaseChain+` SELECT pl.id, oi.id FROM pl, oi`, w.a.String()).Scan(&line, &item); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM order_items WHERE id = $1`, item); err != nil {
			return err
		}
		var gone bool
		var order, name string
		if err := tx.QueryRowContext(ctx, `SELECT order_item_id IS NULL, order_id::text, name FROM purchase_lines WHERE id = $1`,
			line).Scan(&gone, &order, &name); err != nil {
			return err
		}
		if !gone || order == "" || name != "Banco de couro" {
			t.Errorf("line after its item went: item null %v, order %q, name %q", gone, order, name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestPurchaseLine_ItemMustBelongToItsOrder: a line cannot name an item of
// another order, even of the same store.
func TestPurchaseLine_ItemMustBelongToItsOrder(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `WITH `+cteCustomer+`, `+cteOrder+`, `+cteLine+`, `+cteOrderItemFull+`, `+
			cteSupplier+`, `+cteSupplierOrder+`, other AS (SELECT id FROM orders WHERE tenant_id = $1::uuid LIMIT 1)
			INSERT INTO purchase_lines (tenant_id, id, supplier_order_id, supplier_id, order_id, order_item_id,
				catalog_item_id, name, quantity, status, position)
			SELECT so.tenant_id, gen_random_uuid(), so.id, so.supplier_id, other.id, oi.id, gen_random_uuid(),
				'Banco de couro', 1, 'sent', 0 FROM so, oi, other`, w.a.String())
		return err
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" || pgErr.ConstraintName != "purchase_lines_order_item_fkey" {
		t.Errorf("an item of another order: err = %v, want a violation of purchase_lines_order_item_fkey", err)
	}
}

// TestPurchaseLines_RulesOfTheRow: the CHECKs of a line, one statement
// each, each a check violation (23514), not a missing grant.
func TestPurchaseLines_RulesOfTheRow(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	for name, set := range map[string]string{
		"cancelled without a reason": "status = 'cancelled'",
		"a reason while open":        "cancel_reason = 'undone'",
		"received without a day":     "status = 'received'",
		"defective without a note":   "status = 'defective', received_on = current_date",
	} {
		err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
			var line string
			if err := tx.QueryRowContext(ctx, purchaseChain+` SELECT id FROM pl`, w.a.String()).Scan(&line); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `UPDATE purchase_lines SET `+set+` WHERE id = $1`, line)
			return err
		})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("%s: err = %v, want a check violation (23514)", name, err)
		}
	}
	counter := func(status, receivedOn string) error {
		return inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `WITH `+cteCustomer+`, `+cteOrder+`, `+cteLine+`, `+cteOrderItemFull+`, `+cteSupplier+`
				INSERT INTO purchase_lines (tenant_id, id, supplier_order_id, supplier_id, order_id, order_item_id,
					catalog_item_id, name, quantity, status, received_on, position)
				SELECT s.tenant_id, gen_random_uuid(), NULL, s.id, oi.order_id, oi.id, gen_random_uuid(),
					'Banco de couro', 1, $2, `+receivedOn+`, 0 FROM s, oi`, w.a.String(), status)
			return err
		})
	}
	var pgErr *pgconn.PgError
	if err := counter("sent", "NULL"); !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Errorf("an open line with no pedido de compra: err = %v, want a check violation", err)
	}
	if err := counter("received", "current_date"); err != nil {
		t.Errorf("a counter line received at once: %v", err)
	}
}

// TestPurchaseLines_ForeignKeysStayInTheStore: as store A, a line naming
// store B's pedido de compra, order and item, or supplier is refused by a
// composite key (foreign key checks bypass RLS, so only tenant_id in every
// key keeps them in the store), and a line's supplier must be its pedido de
// compra's. Each line is otherwise valid (received at once, so no open-line
// clash), so only the key can refuse it.
func TestPurchaseLines_ForeignKeysStayInTheStore(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	db := env.Seeder(t).DB()
	var bOrder, bItem, bSupplier, bSupplierOrder string
	if err := db.QueryRowContext(t.Context(), `SELECT order_id, order_item_id, supplier_id, supplier_order_id
		FROM purchase_lines WHERE tenant_id = $1 AND order_item_id IS NOT NULL AND supplier_order_id IS NOT NULL LIMIT 1`,
		w.b.String()).Scan(&bOrder, &bItem, &bSupplier, &bSupplierOrder); err != nil {
		t.Fatal(err)
	}
	type refs struct{ so, supplier, order, item string }
	for name, change := range map[string]func(own refs, otherSupplier string) refs{
		"B's pedido de compra":          func(r refs, _ string) refs { r.so = bSupplierOrder; return r },
		"B's order and item":            func(r refs, _ string) refs { r.order, r.item = bOrder, bItem; return r },
		"B's supplier, no pedido":       func(r refs, _ string) refs { r.so, r.supplier = "", bSupplier; return r },
		"not its pedido's own supplier": func(r refs, other string) refs { r.supplier = other; return r },
	} {
		err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
			var own refs
			if err := tx.QueryRowContext(ctx, purchaseChain+` SELECT so.id, so.supplier_id, oi.order_id, oi.id FROM so, oi`,
				w.a.String()).Scan(&own.so, &own.supplier, &own.order, &own.item); err != nil {
				return err
			}
			var other string
			if err := tx.QueryRowContext(ctx, `WITH `+cteSupplier+` SELECT id FROM s`, w.a.String()).Scan(&other); err != nil {
				return err
			}
			r := change(own, other)
			_, err := tx.ExecContext(ctx, `INSERT INTO purchase_lines (tenant_id, id, supplier_order_id, supplier_id,
					order_id, order_item_id, catalog_item_id, name, quantity, status, received_on, check_note, position)
				VALUES ($1, gen_random_uuid(), NULLIF($2, '')::uuid, $3, $4, $5, gen_random_uuid(), 'Banco de couro', 1,
					'received', current_date, '', 9)`,
				w.a.String(), r.so, r.supplier, r.order, r.item)
			return err
		})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
			t.Errorf("%s: err = %v, want a foreign key violation (23503)", name, err)
		}
	}
}

// TestReceiptPhotos_KeyMatchesItsRow: a receiving photo can only name the
// key the API issues for it, t/{its tenant}/receipts/{its id}.
func TestReceiptPhotos_KeyMatchesItsRow(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	for name, key := range map[string]string{
		"another store's key": "'t/' || $2 || '/receipts/' || p.id",
		"another photo's id":  "'t/' || r.tenant_id || '/receipts/' || gen_random_uuid()",
		"a catalog prefix":    "'t/' || r.tenant_id || '/items/' || p.id",
		"a trailing segment":  "'t/' || r.tenant_id || '/receipts/' || p.id || '/x'",
	} {
		err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `WITH `+cteReceipt+`, p AS (SELECT gen_random_uuid() AS id)
				INSERT INTO receipt_photos (tenant_id, id, receipt_id, object_key, content_type, size_bytes)
				SELECT r.tenant_id, p.id, r.id, `+key+`, 'image/png', 10 FROM r, p WHERE $2::uuid IS NOT NULL`,
				w.a.String(), w.b.String())
			return err
		})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("%s: err = %v, want a check violation (23514)", name, err)
		}
	}
}

// TestOrderHistory_TakesThePromisedDateKind: 0016 widens the history kinds
// with the supplier's promised date.
func TestOrderHistory_TakesThePromisedDateKind(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `WITH `+cteCustomer+`, `+cteOrder+`
			INSERT INTO order_status_history (tenant_id, id, order_id, kind, at, detail)
			SELECT tenant_id, gen_random_uuid(), id, 'promised_date', now(), '{"ref": "C-0001"}' FROM o`, w.a.String())
		return err
	})
	if err != nil {
		t.Errorf("a promised_date entry: %v", err)
	}
}

// TestSourcing_PurchasingColumnsGranted: the app may write the supplier's
// promised date and the price paid on an item's sourcing (0013 grants
// UPDATE by column).
func TestSourcing_PurchasingColumnsGranted(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	err := inScope(t, app, storeA(w), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE order_item_sourcing SET promised_date = current_date + 3,
			paid_cost_cents = 41000 WHERE tenant_id = $1`, w.a.String())
		return err
	})
	if err != nil {
		t.Errorf("update the purchasing columns of sourcing: %v", err)
	}
}

// TestPurchasingTables_RefuseADirectInsert: a store page scope writing a
// purchasing row straight onto store A's own seeded parents is refused by
// that table's own staff_only policy, not by a parent's (security review of
// 5.5, as TestStaffOnlyTables_RefuseADirectInsert for the catalog).
func TestPurchasingTables_RefuseADirectInsert(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	db := env.Seeder(t).DB()
	var so, supplier, order, item, line, receipt string
	if err := db.QueryRowContext(t.Context(), `SELECT pl.supplier_order_id, pl.supplier_id, pl.order_id, pl.order_item_id, pl.id
		FROM purchase_lines pl WHERE pl.tenant_id = $1 AND pl.supplier_order_id IS NOT NULL AND pl.order_item_id IS NOT NULL LIMIT 1`,
		w.a.String()).Scan(&so, &supplier, &order, &item, &line); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT id FROM receipts WHERE tenant_id = $1 LIMIT 1`, w.a.String()).Scan(&receipt); err != nil {
		t.Fatal(err)
	}
	for table, q := range map[string]struct {
		sql  string
		args []any
	}{
		"purchase_counters": {`INSERT INTO purchase_counters (tenant_id) VALUES ($1)`, nil},
		"supplier_orders": {`INSERT INTO supplier_orders (tenant_id, id, number, supplier_id, supplier_name, supplier_phone,
			sent_at, sent_by, message_text, message_url) VALUES ($1, gen_random_uuid(), 999999, $2, 'Couros', '5511991234567',
			now(), gen_random_uuid(), 'Olá', 'https://wa.me/5511991234567')`, []any{supplier}},
		"purchase_lines": {`INSERT INTO purchase_lines (tenant_id, id, supplier_order_id, supplier_id, order_id, order_item_id,
			catalog_item_id, name, quantity, status, received_on, position) VALUES ($1, gen_random_uuid(), $2, $3, $4, $5,
			gen_random_uuid(), 'Banco', 1, 'received', current_date, 9)`, []any{so, supplier, order, item}},
		"purchase_line_costs": {`INSERT INTO purchase_line_costs (tenant_id, line_id, currency, cost_cents) VALUES ($1, $2, 'BRL', 1)`, []any{line}},
		"receipts":            {`INSERT INTO receipts (tenant_id, id, received_by, received_at) VALUES ($1, gen_random_uuid(), gen_random_uuid(), now())`, nil},
		"receipt_lines": {`INSERT INTO receipt_lines (tenant_id, receipt_id, line_id, check_result) VALUES ($1, $2, $3, 'ok')`,
			[]any{receipt, line}},
		"receipt_photos": {`INSERT INTO receipt_photos (tenant_id, id, receipt_id, object_key, content_type, size_bytes)
			SELECT $1::uuid, p.id, $2::uuid, 't/' || $1::text || '/receipts/' || p.id, 'image/png', 10 FROM (SELECT gen_random_uuid() AS id) p`,
			[]any{receipt}},
	} {
		err := inScope(t, app, storePage(w), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, q.sql, append([]any{w.a.String()}, q.args...)...)
			return err
		})
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" || !strings.Contains(pgErr.Message, `"`+table+`"`) {
			t.Errorf("direct insert into %s as a store page: err = %v, want 42501 from %s's own policy", table, err, table)
		}
	}
}
