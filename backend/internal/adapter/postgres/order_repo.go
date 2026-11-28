package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Orders implements apporder.Repository on orders and the tables an order is
// made of (0013). Three RLS tiers decide what a scope sees: the order, its
// lines, items and payments are the store's (tenant_isolation, and
// client_read for a client account); the sourcing and the messages are
// staff-only, so a scope with a tenant and no user reads and writes none;
// the history is written by any scope of the store and read by staff only.
//
// Every write names its columns, because encomendo_app may update only
// some. orders.client_account_id is never written (models.Order tags it
// read-only), and the tracking hash is bound like every value: GORM's logger
// drops values, so it never reaches a log.
type Orders struct {
	ids appshared.IDGenerator
}

var _ apporder.Repository = (*Orders)(nil)

// NewOrders returns the orders repository. ids gives history entries their
// ids: the generator's ids keep their order, so entries written in one
// transaction with the same time still read back in the order they were
// made.
func NewOrders(ids appshared.IDGenerator) *Orders { return &Orders{ids: ids} }

// NextNumber takes the store's next order number. The counter row is made on
// first use; the UPDATE locks it until the caller's transaction ends, so two
// concurrent orders get two numbers, and a rolled back order gives its
// number back.
func (*Orders) NextNumber(ctx context.Context, tenant shared.TenantID) (order.Number, error) {
	db, err := conn(ctx)
	if err != nil {
		return 0, err
	}
	err = db.Exec(`INSERT INTO order_counters (tenant_id) VALUES (?) ON CONFLICT DO NOTHING`, tenant.String()).Error
	if err != nil {
		return 0, wrap("make the order counter", err)
	}
	var n []int
	err = db.Raw(`UPDATE order_counters SET last_number = last_number + 1, updated_at = now()
		WHERE tenant_id = ? RETURNING last_number`, tenant.String()).Scan(&n).Error
	if err != nil {
		return 0, wrap("take the next order number", err)
	}
	if len(n) != 1 {
		return 0, fmt.Errorf("take the next order number: no counter for store %s", tenant)
	}
	return order.Number(n[0]), nil
}

// Insert stores a new order with its lines, items, sourcing, payments and
// pending history. A number the store already uses is an error matching
// shared.ErrConflict; a customer the store does not have, shared.ErrInvalid.
// It needs a staff scope, because the sourcing is staff-only (ADR 0027):
// the store page's own orders wait for milestone 7.
func (r *Orders) Insert(ctx context.Context, o *order.Order) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	if !hasStaff(ctx) {
		return errStaffScope
	}
	rows, err := models.OrderRowsFromRecord(o.Record())
	if err != nil {
		return fmt.Errorf("encode order %s: %w", o.ID(), err)
	}
	if err := db.Create(&rows.Order).Error; err != nil {
		return wrap("insert order", err)
	}
	if err := saveParts(db, rows); err != nil {
		return err
	}
	return r.appendHistory(db, o)
}

// Update stores o while the stored version is still o.Version(), and the next
// version, then replaces its lines and items as sets and writes its new
// payments, voids and pending history. It returns shared.ErrVersionConflict
// for a stale version and apporder.ErrNotFound when the order is gone or not
// the store's. Like Insert it needs a staff scope.
func (r *Orders) Update(ctx context.Context, o *order.Order) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	if !hasStaff(ctx) {
		return errStaffScope
	}
	rec := o.Record()
	rows, err := models.OrderRowsFromRecord(rec)
	if err != nil {
		return fmt.Errorf("encode order %s: %w", o.ID(), err)
	}
	res := db.Model(&models.Order{}).
		Where("tenant_id = ? AND id = ? AND version = ?", rows.Order.TenantID, rows.Order.ID, rec.Version).
		Updates(models.OrderUpdate(rows.Order, rec.Version+1))
	if res.Error != nil {
		return wrap("update order", res.Error)
	}
	if res.RowsAffected == 0 {
		return missingOrStale(db, "orders", o.TenantID(), rows.Order.ID, apporder.ErrNotFound)
	}
	if err := pruneParts(db, rows); err != nil {
		return err
	}
	if err := saveParts(db, rows); err != nil {
		return err
	}
	return r.appendHistory(db, o)
}

// pruneParts deletes the lines and items the order no longer has, so lines
// and items are replaced as a set by id. An item that moved to another line
// counts as gone and is inserted again. Deleting an item takes its sourcing
// with it (ON DELETE CASCADE), and deleting a line takes its items.
func pruneParts(db *gorm.DB, rows models.OrderRows) error {
	tenant, orderID := rows.Order.TenantID, rows.Order.ID
	pairs := make([][]any, 0, len(rows.Items))
	for _, it := range rows.Items {
		pairs = append(pairs, []any{it.ID, it.LineID})
	}
	items := db.Where("tenant_id = ? AND order_id = ?", tenant, orderID)
	if len(pairs) > 0 {
		items = items.Where("(id, line_id) NOT IN ?", pairs)
	}
	if err := items.Delete(&models.OrderItem{}).Error; err != nil {
		return wrap("delete removed order items", err)
	}
	lineIDs := make([]string, 0, len(rows.Lines))
	for _, l := range rows.Lines {
		lineIDs = append(lineIDs, l.ID)
	}
	lines := db.Where("tenant_id = ? AND order_id = ?", tenant, orderID)
	if len(lineIDs) > 0 {
		lines = lines.Where("id NOT IN ?", lineIDs)
	}
	return wrap("delete removed order lines", lines.Delete(&models.OrderLine{}).Error)
}

// saveParts upserts the lines, items and sourcing, then writes the payments
// the table lacks and the voids it has not seen. Each upsert may touch only
// a row of this order: an id held by another order matches no row, which
// would drop the write silently, so it is refused instead.
func saveParts(db *gorm.DB, rows models.OrderRows) error {
	if err := upsertRows(db, "order lines", rows.Lines, "order_lines", "order_id",
		"position", "kit_id", "kit", "price_cents", "assembly_days", "safety_margin_days"); err != nil {
		return err
	}
	if err := upsertRows(db, "order items", rows.Items, "order_items", "line_id",
		"position", "snapshot", "sourcing_mode", "quantity", "status", "check_result", "expected_date", "arrived_on"); err != nil {
		return err
	}
	if err := upsertSourcing(db, rows.Sourcing); err != nil {
		return err
	}
	return savePayments(db, rows)
}

// upsertRows inserts rows or updates the ones whose id exists with the same
// guard column (the order of a line, the line of an item), and fails when
// one exists with another.
func upsertRows[M any](db *gorm.DB, what string, rows []M, table, guard string, columns ...string) error {
	if len(rows) == 0 {
		return nil
	}
	res := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		Where:   clause.Where{Exprs: []clause.Expression{gorm.Expr(table + "." + guard + " = excluded." + guard)}},
		DoUpdates: append(
			clause.AssignmentColumns(columns),
			clause.Assignment{Column: clause.Column{Name: "updated_at"}, Value: gorm.Expr("now()")},
		),
	}).Create(&rows)
	if res.Error != nil {
		return wrap("save "+what, res.Error)
	}
	if res.RowsAffected != int64(len(rows)) {
		return fmt.Errorf("save %s: an id belongs to another order: %w", what, shared.ErrInvalid)
	}
	return nil
}

func upsertSourcing(db *gorm.DB, rows []models.OrderItemSourcing) error {
	if len(rows) == 0 {
		return nil
	}
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "order_item_id"}},
		DoUpdates: append(
			clause.AssignmentColumns([]string{
				"supplier_id", "supplier_name", "cost_cents", "backup_supplier_id", "backup_supplier_name",
				"backup_lead_days", "backup_cost_cents",
			}),
			clause.Assignment{Column: clause.Column{Name: "updated_at"}, Value: gorm.Expr("now()")},
		),
	}).Create(&rows).Error
	return wrap("save order item sourcing", err)
}

// storedPayment is what savePayments reads of the payments already stored.
type storedPayment struct {
	ID     string `gorm:"column:id"`
	Voided bool   `gorm:"column:voided"`
}

// savePayments inserts the payments the order has and the table lacks, and
// records the voids the table has not seen. Payments are never deleted, and
// a void is never undone: the domain forbids it and the update names only a
// payment still not voided.
func savePayments(db *gorm.DB, rows models.OrderRows) error {
	if len(rows.Payments) == 0 {
		return nil
	}
	var stored []storedPayment
	err := db.Table("payments").Select("id, voided_at IS NOT NULL AS voided").
		Where("tenant_id = ? AND order_id = ?", rows.Order.TenantID, rows.Order.ID).Scan(&stored).Error
	if err != nil {
		return wrap("read stored payments", err)
	}
	known := make(map[string]bool, len(stored))
	for _, s := range stored {
		known[s.ID] = s.Voided
	}
	var fresh []models.Payment
	for _, p := range rows.Payments {
		voided, ok := known[p.ID]
		switch {
		case !ok:
			fresh = append(fresh, p)
		case !voided && p.VoidedAt != nil:
			err := db.Model(&models.Payment{}).Where("tenant_id = ? AND id = ? AND voided_at IS NULL", p.TenantID, p.ID).
				Updates(map[string]any{
					"voided_at": *p.VoidedAt, "voided_by": nilableString(p.VoidedBy), "void_reason": nilableString(p.VoidReason),
					"updated_at": gorm.Expr("now()"),
				}).Error
			if err != nil {
				return wrap("void payment", err)
			}
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	return wrap("insert payments", db.Create(&fresh).Error)
}

func nilableString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// appendHistory writes the order's pending history, oldest first, in one
// insert. The insert returns nothing: GORM asks for no column back when the
// model has no default-valued field, and RETURNING would need the history's
// read policy, which a store page scope lacks (0013).
func (r *Orders) appendHistory(db *gorm.DB, o *order.Order) error {
	pending := o.PendingHistory()
	if len(pending) == 0 {
		return nil
	}
	rows := make([]models.OrderHistory, 0, len(pending))
	for _, e := range pending {
		row, err := models.OrderHistoryFromEntry(o.TenantID(), r.ids.NewID(), o.ID(), e)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	return wrap("insert order history", db.Create(&rows).Error)
}

// AddMessage records a wa.me message for an order of the scope's store. The
// messages are staff-only, so it needs a staff scope.
func (*Orders) AddMessage(ctx context.Context, m apporder.Message) error {
	db, err := conn(ctx)
	if err != nil {
		return err
	}
	if !hasStaff(ctx) {
		return errStaffScope
	}
	row := models.OrderMessage{
		TenantID: txScope(ctx).TenantID.String(), ID: m.ID.String(), OrderID: m.Order.String(), Kind: string(m.Kind),
		ToPhone: m.ToPhone, Text: m.Text, URL: m.URL, At: m.At.UTC(),
	}
	if !m.By.IsZero() {
		by := m.By.String()
		row.ByUser = &by
	}
	return wrap("insert order message", db.Create(&row).Error)
}
