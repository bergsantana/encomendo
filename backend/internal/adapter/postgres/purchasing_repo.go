package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Constraints whose violation means more than "already exists" (0016).
const (
	openItemIndex     = "purchase_lines_open_item_idx"
	receiptLineKey    = "receipt_lines_tenant_id_line_id_key"
	receiptPhotoKey   = "receipt_photos_object_key_key"
	receiptPhotoPK    = "receipt_photos_pkey"
	purchaseLineCols  = "pl.tenant_id, pl.id, pl.supplier_order_id, pl.supplier_id, pl.order_id, pl.order_item_id, pl.catalog_item_id, pl.name, pl.quantity, pl.choices, pl.status, pl.promised_date, pl.check_note, pl.cancel_reason, pl.received_on, pl.position"
	purchaseLineJoins = `purchase_lines pl
		LEFT JOIN purchase_line_costs c ON c.tenant_id = pl.tenant_id AND c.line_id = pl.id
		LEFT JOIN supplier_orders so ON so.tenant_id = pl.tenant_id AND so.id = pl.supplier_order_id`
	lineReadColumns = purchaseLineCols + `, c.currency AS cost_currency, c.cost_cents, c.paid_cost_cents,
		so.number AS order_number, so.supplier_name AS order_supplier_name, so.supplier_phone AS order_supplier_phone,
		so.sent_at AS order_sent_at, so.promised_date AS order_promised_date`
)

// Purchasing implements apppurchasing.Repository on supplier_orders,
// purchase_lines, purchase_line_costs, receipts and their rows (0016). Every
// table is staff-only under RLS and has no client policy, and the repository
// refuses a scope without a staff user before any SQL: the foreign key's
// ON DELETE SET NULL on a line runs as the table owner and ignores RLS, so
// the Go check is the guard that a store page scope never reaches it
// (docs/plans/m5-code-design.md, section 2).
//
// A line's unit costs live in purchase_line_costs. They are copied from the
// order's staff-only sourcing in SQL when the line is written, never taken
// from the caller, so a line sent by an admin keeps the owner's cost. The
// repository returns them with the line; which role may see them is the
// use case's (identity.SeesCosts).
type Purchasing struct{}

var _ apppurchasing.Repository = (*Purchasing)(nil)

// NewPurchasing returns the purchasing repository. It makes no ids: a
// line, a supplier order and a receipt arrive with the ids their use case
// gave them.
func NewPurchasing() *Purchasing { return &Purchasing{} }

// begin returns the transaction's connection, refusing a scope that has no
// staff user.
func begin(ctx context.Context) (*gorm.DB, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	if !hasStaff(ctx) {
		return nil, errStaffScope
	}
	return db, nil
}

// wrapPurchasing maps the unique violations purchasing gives a name to:
// a second open line for an item is apppurchasing.ErrOpenLine, a line in two
// receipts is purchasing.ErrLineClosed, and a photo key already stored
// matches catalog.ErrPhotoNotUploaded as well as shared.ErrConflict (the
// upload was promoted by the first receipt and is gone). Any other error is
// wrap's.
func wrapPurchasing(doing string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case openItemIndex:
			return fmt.Errorf("%s: %w (%w)", doing, apppurchasing.ErrOpenLine, err)
		case receiptLineKey:
			return fmt.Errorf("%s: %w: %w", doing, purchasing.ErrLineClosed, err)
		case receiptPhotoKey, receiptPhotoPK:
			return fmt.Errorf("%s: %w: %w", doing, catalog.ErrPhotoNotUploaded, mapError(err))
		}
	}
	return wrap(doing, err)
}

// NextNumber implements apppurchasing.Repository. The counter row is made
// and incremented in one statement; the row stays locked until the
// transaction ends, so two concurrent sends get two numbers and a rolled
// back one gives its number back.
func (*Purchasing) NextNumber(ctx context.Context, tenant shared.TenantID) (purchasing.Number, error) {
	db, err := begin(ctx)
	if err != nil {
		return 0, err
	}
	var n []int
	err = db.Raw(`INSERT INTO purchase_counters (tenant_id, last_number) VALUES (?, 1)
		ON CONFLICT (tenant_id) DO UPDATE SET last_number = purchase_counters.last_number + 1, updated_at = now()
		RETURNING last_number`, tenant.String()).Scan(&n).Error
	if err != nil {
		return 0, wrap("take the next supplier order number", err)
	}
	if len(n) != 1 {
		return 0, fmt.Errorf("take the next supplier order number: no counter for store %s", tenant)
	}
	return purchasing.Number(n[0]), nil
}

// PeekNumber implements apppurchasing.Repository. It reads the counter and
// writes nothing.
func (*Purchasing) PeekNumber(ctx context.Context, tenant shared.TenantID) (purchasing.Number, error) {
	db, err := begin(ctx)
	if err != nil {
		return 0, err
	}
	var last []int
	err = db.Table("purchase_counters").Select("last_number").Where("tenant_id = ?", tenant.String()).Scan(&last).Error
	if err != nil {
		return 0, wrap("peek the next supplier order number", err)
	}
	if len(last) == 0 {
		return 1, nil
	}
	return purchasing.Number(last[0] + 1), nil
}

// InsertSupplierOrder implements apppurchasing.Repository. The costs are
// copied from the order items' sourcing by the same transaction.
func (*Purchasing) InsertSupplierOrder(ctx context.Context, o *purchasing.SupplierOrder, text, url string) error {
	db, err := begin(ctx)
	if err != nil {
		return err
	}
	rec := o.Record()
	head := models.SupplierOrderFromRecord(rec, text, url)
	if err := db.Create(&head).Error; err != nil {
		return wrapPurchasing("insert supplier order", err)
	}
	rows := make([]models.PurchaseLine, len(rec.Lines))
	for i, l := range rec.Lines {
		if rows[i], err = models.PurchaseLineFromRecord(o.Tenant(), l, i); err != nil {
			return err
		}
	}
	if err := db.Create(&rows).Error; err != nil {
		return wrapPurchasing("insert purchase lines", err)
	}
	return copyCosts(db, o.Tenant(), rows)
}

// copyCosts writes the costs row of each new line: the unit cost of its
// order item, read from the staff-only sourcing in SQL, in the order's
// currency. A line whose item has no cost gets a row with a null cost, so
// the price paid has a row to go to.
func copyCosts(db *gorm.DB, tenant shared.TenantID, lines []models.PurchaseLine) error {
	ids := make([]string, len(lines))
	for i, l := range lines {
		ids[i] = l.ID
	}
	res := db.Exec(`INSERT INTO purchase_line_costs (tenant_id, line_id, currency, cost_cents)
		SELECT pl.tenant_id, pl.id, o.currency, s.cost_cents
		FROM purchase_lines pl
		JOIN orders o ON o.tenant_id = pl.tenant_id AND o.id = pl.order_id
		LEFT JOIN order_item_sourcing s ON s.tenant_id = pl.tenant_id AND s.order_item_id = pl.order_item_id
		WHERE pl.tenant_id = ? AND pl.id IN ?`, tenant.String(), ids)
	if res.Error != nil {
		return wrap("copy the order costs to the lines", res.Error)
	}
	if res.RowsAffected != int64(len(ids)) {
		return fmt.Errorf("copy the order costs to the lines: %d of %d lines found: %w", res.RowsAffected, len(ids), shared.ErrInvalid)
	}
	return nil
}

// UpdateSupplierOrder implements apppurchasing.Repository. It writes the
// order's own columns under the version guard, then each line's changing
// columns and the price paid; the lines are the order's own, none added or
// removed.
func (*Purchasing) UpdateSupplierOrder(ctx context.Context, o *purchasing.SupplierOrder) error {
	db, err := begin(ctx)
	if err != nil {
		return err
	}
	rec := o.Record()
	res := db.Model(&models.SupplierOrder{}).
		Where("tenant_id = ? AND id = ? AND version = ?", rec.Tenant.String(), rec.ID.String(), rec.Version).
		Updates(models.SupplierOrderUpdate(rec, rec.Version+1))
	if res.Error != nil {
		return wrap("update supplier order", res.Error)
	}
	if res.RowsAffected == 0 {
		return missingOrStale(db, "supplier_orders", rec.Tenant, rec.ID.String(), purchasing.ErrSupplierOrderNotFound)
	}
	var stored int64
	err = db.Table("purchase_lines").Where("tenant_id = ? AND supplier_order_id = ?", rec.Tenant.String(), rec.ID.String()).
		Count(&stored).Error
	if err != nil {
		return wrap("count the lines of the supplier order", err)
	}
	if stored != int64(len(rec.Lines)) {
		return fmt.Errorf("update supplier order: a supplier order's lines cannot change: %w", shared.ErrInvalid)
	}
	for _, l := range rec.Lines {
		if err := updateLine(db, rec, l); err != nil {
			return err
		}
	}
	return nil
}

func updateLine(db *gorm.DB, rec purchasing.Record, l purchasing.LineRecord) error {
	res := db.Model(&models.PurchaseLine{}).
		Where("tenant_id = ? AND id = ? AND supplier_order_id = ?", rec.Tenant.String(), l.ID.String(), rec.ID.String()).
		Updates(models.PurchaseLineUpdate(l))
	if res.Error != nil {
		return wrap("update purchase line", res.Error)
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("update purchase line %s: not a line of the supplier order: %w", l.ID, shared.ErrInvalid)
	}
	if l.Paid == nil {
		return nil
	}
	err := db.Table("purchase_line_costs").
		Where("tenant_id = ? AND line_id = ? AND paid_cost_cents IS DISTINCT FROM ?", rec.Tenant.String(), l.ID.String(), l.Paid.Cents()).
		Updates(map[string]any{"paid_cost_cents": l.Paid.Cents(), "updated_at": gorm.Expr("now()")}).Error
	return wrap("record the price paid", err)
}

// lineSelect is the query every read of lines starts from: the line with
// its costs and its supplier order's facts.
func lineSelect(db *gorm.DB) *gorm.DB {
	return db.Table(purchaseLineJoins).Select(lineReadColumns)
}

func recordsOf(rows []models.LineRead) ([]purchasing.LineRecord, error) {
	out := make([]purchasing.LineRecord, len(rows))
	for i, r := range rows {
		rec, err := r.ToRecord()
		if err != nil {
			return nil, fmt.Errorf("stored purchase line %s is invalid: %s", r.ID, err.Error())
		}
		out[i] = rec
	}
	return out, nil
}

func storedOf(rows []models.LineRead) ([]apppurchasing.StoredLine, error) {
	recs, err := recordsOf(rows)
	if err != nil {
		return nil, err
	}
	out := make([]apppurchasing.StoredLine, len(rows))
	for i, r := range rows {
		name, phone := r.SupplierFacts()
		out[i] = apppurchasing.StoredLine{
			LineRecord: recs[i], Number: r.Number(), SupplierName: name, SupplierPhone: phone,
			SentAt: r.SentAt(), OrderPromised: r.Promised(),
		}
	}
	return out, nil
}

// loadOrders rebuilds the records of heads, in their order, each with its
// lines in the order they were picked.
func loadOrders(db *gorm.DB, tenant shared.TenantID, heads []models.SupplierOrder) ([]purchasing.Record, error) {
	if len(heads) == 0 {
		return nil, nil
	}
	ids := make([]string, len(heads))
	for i, h := range heads {
		ids[i] = h.ID
	}
	var rows []models.LineRead
	err := lineSelect(db).Where("pl.tenant_id = ? AND pl.supplier_order_id IN ?", tenant.String(), ids).
		Order("pl.supplier_order_id, pl.position").Scan(&rows).Error
	if err != nil {
		return nil, wrap("read purchase lines", err)
	}
	recs, err := recordsOf(rows)
	if err != nil {
		return nil, err
	}
	byOrder := make(map[shared.SupplierOrderID][]purchasing.LineRecord, len(heads))
	for _, r := range recs {
		byOrder[r.SupplierOrder] = append(byOrder[r.SupplierOrder], r)
	}
	out := make([]purchasing.Record, len(heads))
	for i, h := range heads {
		id, err := shared.ParseSupplierOrderID(h.ID)
		if err != nil {
			return nil, fmt.Errorf("stored supplier order id is invalid: %s", err.Error())
		}
		if out[i], err = h.ToRecord(byOrder[id]); err != nil {
			return nil, fmt.Errorf("stored supplier order %s is invalid: %s", h.ID, err.Error())
		}
	}
	return out, nil
}

// SupplierOrder implements apppurchasing.Repository.
func (*Purchasing) SupplierOrder(ctx context.Context, tenant shared.TenantID, id shared.SupplierOrderID) (purchasing.Record, error) {
	db, err := begin(ctx)
	if err != nil {
		return purchasing.Record{}, err
	}
	var heads []models.SupplierOrder
	err = db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Limit(1).Find(&heads).Error
	if err != nil {
		return purchasing.Record{}, wrap("read supplier order", err)
	}
	if len(heads) == 0 {
		return purchasing.Record{}, purchasing.ErrSupplierOrderNotFound
	}
	recs, err := loadOrders(db, tenant, heads)
	if err != nil {
		return purchasing.Record{}, err
	}
	return recs[0], nil
}

// SupplierOrderOfLine implements apppurchasing.Repository.
func (r *Purchasing) SupplierOrderOfLine(ctx context.Context, tenant shared.TenantID, line shared.PurchaseLineID) (purchasing.Record, error) {
	db, err := begin(ctx)
	if err != nil {
		return purchasing.Record{}, err
	}
	var owner []string
	err = db.Table("purchase_lines").Select("supplier_order_id::text").
		Where("tenant_id = ? AND id = ? AND supplier_order_id IS NOT NULL", tenant.String(), line.String()).Scan(&owner).Error
	if err != nil {
		return purchasing.Record{}, wrap("find the supplier order of a line", err)
	}
	if len(owner) == 0 {
		return purchasing.Record{}, purchasing.ErrLineNotFound
	}
	id, err := shared.ParseSupplierOrderID(owner[0])
	if err != nil {
		return purchasing.Record{}, fmt.Errorf("stored supplier order id is invalid: %s", err.Error())
	}
	return r.SupplierOrder(ctx, tenant, id)
}

// Message implements apppurchasing.Repository.
func (*Purchasing) Message(ctx context.Context, tenant shared.TenantID, id shared.SupplierOrderID) (apppurchasing.SentMessage, error) {
	db, err := begin(ctx)
	if err != nil {
		return apppurchasing.SentMessage{}, err
	}
	var rows []struct {
		Text string `gorm:"column:message_text"`
		URL  string `gorm:"column:message_url"`
	}
	err = db.Table("supplier_orders").Select("message_text, message_url").
		Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Scan(&rows).Error
	if err != nil {
		return apppurchasing.SentMessage{}, wrap("read supplier order message", err)
	}
	if len(rows) == 0 {
		return apppurchasing.SentMessage{}, purchasing.ErrSupplierOrderNotFound
	}
	return apppurchasing.SentMessage{Text: rows[0].Text, URL: rows[0].URL}, nil
}

// sentPosition is where a page of supplier orders ends: the sent time and
// id of its last order. The time is kept to the microsecond, as the column
// is.
type sentPosition struct {
	at time.Time
	id string
}

func (p sentPosition) cursor() string {
	text := p.at.UTC().Format(time.RFC3339Nano) + "|" + p.id
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

// parseSentCursor reads what sentPosition.cursor wrote; anything else is
// errBadCursor.
func parseSentCursor(c string) (sentPosition, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return sentPosition{}, errBadCursor()
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return sentPosition{}, errBadCursor()
	}
	t, terr := time.Parse(time.RFC3339Nano, at)
	parsed, perr := shared.ParseID(id)
	if terr != nil || perr != nil || parsed.String() != id {
		return sentPosition{}, errBadCursor()
	}
	return sentPosition{at: t.UTC(), id: id}, nil
}

// SupplierOrders implements apppurchasing.Repository.
func (*Purchasing) SupplierOrders(ctx context.Context, tenant shared.TenantID, open bool, p shared.PageRequest) (shared.Page[purchasing.Record], error) {
	db, err := begin(ctx)
	if err != nil {
		return shared.Page[purchasing.Record]{}, err
	}
	q := db.Model(&models.SupplierOrder{}).Where("tenant_id = ?", tenant.String())
	if open {
		q = q.Where(`EXISTS (SELECT 1 FROM purchase_lines pl WHERE pl.tenant_id = supplier_orders.tenant_id
			AND pl.supplier_order_id = supplier_orders.id AND pl.status IN ('sent', 'in_transit'))`)
	}
	if cursor := p.Cursor(); cursor != "" {
		after, err := parseSentCursor(cursor)
		if err != nil {
			return shared.Page[purchasing.Record]{}, err
		}
		q = q.Where("(sent_at, id) < (?, ?)", after.at, after.id)
	}
	limit := p.Limit()
	var heads []models.SupplierOrder
	if err := q.Order("sent_at DESC, id DESC").Limit(limit + 1).Find(&heads).Error; err != nil {
		return shared.Page[purchasing.Record]{}, wrap("list supplier orders", err)
	}
	var next string
	if len(heads) > limit {
		heads = heads[:limit]
		last := heads[limit-1]
		next = sentPosition{at: last.SentAt, id: last.ID}.cursor()
	}
	recs, err := loadOrders(db, tenant, heads)
	if err != nil {
		return shared.Page[purchasing.Record]{}, err
	}
	return shared.Page[purchasing.Record]{Items: recs, NextCursor: next}, nil
}

// OpenLines implements apppurchasing.Repository.
func (*Purchasing) OpenLines(ctx context.Context, tenant shared.TenantID) ([]apppurchasing.StoredLine, error) {
	db, err := begin(ctx)
	if err != nil {
		return nil, err
	}
	var rows []models.LineRead
	err = lineSelect(db).Where("pl.tenant_id = ? AND pl.status IN ('sent', 'in_transit')", tenant.String()).
		Order("so.sent_at, so.id, pl.position").Scan(&rows).Error
	if err != nil {
		return nil, wrap("read open purchase lines", err)
	}
	return storedOf(rows)
}

// LinesOfOrder implements apppurchasing.Repository. Lines come in the order
// the database stamped them (created_at is the statement's clock), a batch
// by position.
func (*Purchasing) LinesOfOrder(ctx context.Context, tenant shared.TenantID, id shared.OrderID) ([]apppurchasing.StoredLine, error) {
	db, err := begin(ctx)
	if err != nil {
		return nil, err
	}
	var rows []models.LineRead
	err = lineSelect(db).Where("pl.tenant_id = ? AND pl.order_id = ?", tenant.String(), id.String()).
		Order("pl.created_at, pl.position, pl.id").Scan(&rows).Error
	if err != nil {
		return nil, wrap("read the purchase lines of an order", err)
	}
	return storedOf(rows)
}

// DefectiveItems implements apppurchasing.Repository.
func (*Purchasing) DefectiveItems(ctx context.Context, tenant shared.TenantID, items []shared.OrderItemID) (map[shared.OrderItemID]bool, error) {
	db, err := begin(ctx)
	if err != nil {
		return nil, err
	}
	out := map[shared.OrderItemID]bool{}
	if len(items) == 0 {
		return out, nil
	}
	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = it.String()
	}
	var found []string
	err = db.Table("purchase_lines").Distinct("order_item_id::text").
		Where("tenant_id = ? AND status = 'defective' AND order_item_id IN ?", tenant.String(), keys).Scan(&found).Error
	if err != nil {
		return nil, wrap("read defective items", err)
	}
	for _, f := range found {
		id, err := shared.ParseOrderItemID(f)
		if err != nil {
			return nil, fmt.Errorf("stored order item id is invalid: %s", err.Error())
		}
		out[id] = true
	}
	return out, nil
}

// InsertCounterLine implements apppurchasing.Repository. A counter line has
// no supplier order, so it takes position 0.
func (*Purchasing) InsertCounterLine(ctx context.Context, tenant shared.TenantID, l purchasing.Line) error {
	db, err := begin(ctx)
	if err != nil {
		return err
	}
	rec := l.Record()
	if !rec.SupplierOrder.IsZero() || (rec.Status != purchasing.LineReceived && rec.Status != purchasing.LineDefective) {
		return fmt.Errorf("insert counter line: a counter line is received or defective with no supplier order: %w", shared.ErrInvalid)
	}
	row, err := models.PurchaseLineFromRecord(tenant, rec, 0)
	if err != nil {
		return err
	}
	if err := db.Create(&row).Error; err != nil {
		return wrapPurchasing("insert counter line", err)
	}
	return copyCosts(db, tenant, []models.PurchaseLine{row})
}

// InsertReceipt implements apppurchasing.Repository. Receipts are
// append-only: the receipt, then a row for each line it checked, then its
// photos.
func (*Purchasing) InsertReceipt(ctx context.Context, r *purchasing.Receipt) error {
	db, err := begin(ctx)
	if err != nil {
		return err
	}
	rows := models.ReceiptRowsFromDomain(r)
	if err := checkReceivedLines(db, r.Tenant(), rows.Lines); err != nil {
		return err
	}
	if err := db.Create(&rows.Receipt).Error; err != nil {
		return wrapPurchasing("insert receipt", err)
	}
	// One row at a time, so a failure names the line a receipt would close
	// twice (the entries' order is in created_at, set by the model).
	for i := range rows.Lines {
		if err := db.Create(&rows.Lines[i]).Error; err != nil {
			return wrapPurchasing("insert receipt line "+rows.Lines[i].LineID, err)
		}
	}
	if len(rows.Photos) == 0 {
		return nil
	}
	return wrapPurchasing("insert receipt photos", db.Create(&rows.Photos).Error)
}

// checkReceivedLines refuses a receipt of a line that is not stored received
// or defective, or that the store does not have.
func checkReceivedLines(db *gorm.DB, tenant shared.TenantID, lines []models.ReceiptLine) error {
	ids := make([]string, len(lines))
	for i, l := range lines {
		ids[i] = l.LineID
	}
	var n int64
	err := db.Table("purchase_lines").
		Where("tenant_id = ? AND id IN ? AND status IN ('received', 'defective')", tenant.String(), ids).Count(&n).Error
	if err != nil {
		return wrap("check the lines of a receipt", err)
	}
	if n != int64(len(ids)) {
		return fmt.Errorf("insert receipt: a line is not stored received or defective: %w", shared.ErrInvalid)
	}
	return nil
}

// Receipt implements apppurchasing.Repository: its lines as they are now,
// in the order of its entries.
func (*Purchasing) Receipt(ctx context.Context, tenant shared.TenantID, id shared.ReceiptID) (apppurchasing.StoredReceipt, error) {
	db, err := begin(ctx)
	if err != nil {
		return apppurchasing.StoredReceipt{}, err
	}
	var heads []models.Receipt
	err = db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Limit(1).Find(&heads).Error
	if err != nil {
		return apppurchasing.StoredReceipt{}, wrap("read receipt", err)
	}
	if len(heads) == 0 {
		return apppurchasing.StoredReceipt{}, apppurchasing.ErrReceiptNotFound
	}
	var rows []models.LineRead
	err = lineSelect(db).
		Joins("JOIN receipt_lines rl ON rl.tenant_id = pl.tenant_id AND rl.line_id = pl.id").
		Where("rl.tenant_id = ? AND rl.receipt_id = ?", tenant.String(), id.String()).
		Order("rl.created_at, pl.id").Scan(&rows).Error
	if err != nil {
		return apppurchasing.StoredReceipt{}, wrap("read the lines of a receipt", err)
	}
	lines, err := recordsOf(rows)
	if err != nil {
		return apppurchasing.StoredReceipt{}, err
	}
	return apppurchasing.StoredReceipt{ID: id, At: heads[0].ReceivedAt.UTC(), Lines: lines}, nil
}

// StaffNames implements apppurchasing.Repository: the names of the members
// of tenant among ids, joined through memberships so another store's users
// never come back, even by a known id. A user who is not a member is absent.
func (*Purchasing) StaffNames(ctx context.Context, tenant shared.TenantID, ids []shared.UserID) (map[shared.UserID]string, error) {
	db, err := begin(ctx)
	if err != nil {
		return nil, err
	}
	out := map[shared.UserID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = id.String()
	}
	var rows []struct {
		ID   string
		Name string
	}
	err = db.Table("users u").Select("u.id::text AS id, u.name").
		Joins("JOIN memberships m ON m.user_id = u.id").
		Where("m.tenant_id = ? AND u.id IN ?", tenant.String(), keys).Scan(&rows).Error
	if err != nil {
		return nil, wrap("read staff names", err)
	}
	for _, row := range rows {
		id, err := shared.ParseUserID(row.ID)
		if err != nil {
			return nil, fmt.Errorf("stored user id is invalid: %s", err.Error())
		}
		out[id] = row.Name
	}
	return out, nil
}
