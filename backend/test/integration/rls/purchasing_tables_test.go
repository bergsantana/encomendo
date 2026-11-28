//go:build integration

package rls_test

import (
	"context"
	"database/sql"
	"errors"
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
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Errorf("an item of another order: err = %v, want a foreign key violation (23503)", err)
	}
}

// TestPurchaseLines_RulesOfTheRow: the CHECKs of a line, as one statement
// each.
func TestPurchaseLines_RulesOfTheRow(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	for name, set := range map[string]string{
		"cancelled without a reason": "status = 'cancelled'",
		"a reason while open":        "cancel_reason = 'undone'",
		"received without a day":     "status = 'received'",
		"defective without a note":   "status = 'defective', received_on = current_date",
		"a counter line still open":  "supplier_order_id = NULL",
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
		if !errors.As(err, &pgErr) || (pgErr.Code != "23514" && pgErr.Code != "42501") {
			t.Errorf("%s: err = %v, want a check violation or no grant", name, err)
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
