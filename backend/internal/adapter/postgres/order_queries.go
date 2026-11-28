package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres/models"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// ByID returns the stored order, or apporder.ErrNotFound. In a scope with no
// staff user (a store page scope, a client account) the sourcing comes back
// empty, because RLS shows those scopes none of it.
func (*Orders) ByID(ctx context.Context, tenant shared.TenantID, id shared.OrderID) (order.Record, error) {
	db, err := conn(ctx)
	if err != nil {
		return order.Record{}, err
	}
	var heads []models.Order
	err = db.Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Limit(1).Find(&heads).Error
	if err != nil {
		return order.Record{}, wrap("read order", err)
	}
	if len(heads) == 0 {
		return order.Record{}, apporder.ErrNotFound
	}
	recs, err := loadRecords(ctx, db, tenant, heads)
	if err != nil {
		return order.Record{}, err
	}
	return recs[0], nil
}

// loadRecords reads the lines, items, sourcing and payments of heads, all of
// one store, with one query per table, and joins them into records in the
// order of heads. The sourcing is read only in a staff scope.
func loadRecords(ctx context.Context, db *gorm.DB, tenant shared.TenantID, heads []models.Order) ([]order.Record, error) {
	if len(heads) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(heads))
	for _, h := range heads {
		ids = append(ids, h.ID)
	}
	var lines []models.OrderLine
	var items []models.OrderItem
	var payments []models.Payment
	var sourcing []models.OrderItemSourcing
	t := tenant.String()
	if err := db.Where("tenant_id = ? AND order_id IN ?", t, ids).Order("order_id, position").Find(&lines).Error; err != nil {
		return nil, wrap("read order lines", err)
	}
	if err := db.Where("tenant_id = ? AND order_id IN ?", t, ids).Order("order_id, position").Find(&items).Error; err != nil {
		return nil, wrap("read order items", err)
	}
	if err := db.Where("tenant_id = ? AND order_id IN ?", t, ids).Order("order_id, recorded_at, id").Find(&payments).Error; err != nil {
		return nil, wrap("read payments", err)
	}
	if hasStaff(ctx) {
		err := db.Where("tenant_id = ? AND order_item_id IN (SELECT id FROM order_items WHERE tenant_id = ? AND order_id IN ?)",
			t, t, ids).Find(&sourcing).Error
		if err != nil {
			return nil, wrap("read order item sourcing", err)
		}
	}
	rows := groupRows(heads, lines, items, sourcing, payments)
	out := make([]order.Record, 0, len(heads))
	for _, r := range rows {
		rec, err := r.ToRecord()
		if err != nil {
			// %s, not %w: a broken stored row is a bug (500), and the decode
			// errors match shared.ErrInvalid, which would answer it as bad input.
			return nil, fmt.Errorf("stored order %s is invalid: %s", r.Order.ID, err.Error())
		}
		out = append(out, rec)
	}
	return out, nil
}

// groupRows splits what the queries read by order, keeping the order of heads
// and each table's own order.
func groupRows(heads []models.Order, lines []models.OrderLine, items []models.OrderItem,
	sourcing []models.OrderItemSourcing, payments []models.Payment,
) []models.OrderRows {
	at := make(map[string]int, len(heads))
	out := make([]models.OrderRows, len(heads))
	for i, h := range heads {
		at[h.ID] = i
		out[i].Order = h
	}
	itemOrder := make(map[string]int, len(items))
	for _, l := range lines {
		out[at[l.OrderID]].Lines = append(out[at[l.OrderID]].Lines, l)
	}
	for _, it := range items {
		i := at[it.OrderID]
		itemOrder[it.ID] = i
		out[i].Items = append(out[i].Items, it)
	}
	for _, s := range sourcing {
		i := itemOrder[s.OrderItemID]
		out[i].Sourcing = append(out[i].Sourcing, s)
	}
	for _, p := range payments {
		out[at[p.OrderID]].Payments = append(out[at[p.OrderID]].Payments, p)
	}
	return out
}

// List returns one page of the store's orders that match f, by (delivery
// date, id), with the cursor of the next page when more rows exist. f.Alert
// is not applied: it is the service's.
func (*Orders) List(ctx context.Context, tenant shared.TenantID, f apporder.ListFilter, p shared.PageRequest) (shared.Page[order.Record], error) {
	db, err := conn(ctx)
	if err != nil {
		return shared.Page[order.Record]{}, err
	}
	q := narrowOrders(db.Model(&models.Order{}).Where("orders.tenant_id = ?", tenant.String()), f)
	if cursor := p.Cursor(); cursor != "" {
		after, err := parseOrderCursor(cursor)
		if err != nil {
			return shared.Page[order.Record]{}, err
		}
		q = q.Where("(orders.delivery_date, orders.id) > (?, ?)", after.date, after.id)
	}
	limit := p.Limit()
	var heads []models.Order
	if err := q.Order("orders.delivery_date, orders.id").Limit(limit + 1).Find(&heads).Error; err != nil {
		return shared.Page[order.Record]{}, wrap("list orders", err)
	}
	var next string
	if len(heads) > limit {
		heads = heads[:limit]
		last := heads[limit-1]
		next = orderPosition{date: last.DeliveryDate, id: last.ID}.cursor()
	}
	recs, err := loadRecords(ctx, db, tenant, heads)
	if err != nil {
		return shared.Page[order.Record]{}, err
	}
	return shared.Page[order.Record]{Items: recs, NextCursor: next}, nil
}

// narrowOrders adds f's conditions. A cancelled order's status is the
// implicit "cancelled" one, whatever status_key holds (it keeps the last
// real status), so each status and phase condition names cancelled_at first.
// Conditions are subqueries rather than joins, so a page never repeats an
// order and the select stays the order's own columns.
func narrowOrders(q *gorm.DB, f apporder.ListFilter) *gorm.DB {
	switch f.Scope {
	case apporder.ScopeActive:
		q = q.Where("orders.cancelled_at IS NULL AND orders.handed_over_at IS NULL")
	case apporder.ScopeHandedOver:
		q = q.Where("orders.handed_over_at IS NOT NULL")
	case apporder.ScopeCancelled:
		q = q.Where("orders.cancelled_at IS NOT NULL")
	case apporder.ScopeAll:
	}
	if !f.Customer.IsZero() {
		q = q.Where("orders.customer_id = ?", f.Customer.String())
	}
	switch {
	case f.Status == workflow.Cancelled.Key:
		q = q.Where("orders.cancelled_at IS NOT NULL")
	case f.Status != "":
		q = q.Where("orders.cancelled_at IS NULL AND orders.status_key = ?", string(f.Status))
	}
	switch {
	case f.Phase == workflow.PhaseCancelled:
		q = q.Where("orders.cancelled_at IS NOT NULL")
	case f.Phase != "":
		q = q.Where(`orders.cancelled_at IS NULL AND EXISTS (SELECT 1 FROM workflow_statuses ws
			WHERE ws.workflow_id = orders.workflow_id AND ws.workflow_version = orders.workflow_version
			AND ws.key = orders.status_key AND ws.phase = ?)`, string(f.Phase))
	}
	return narrowByQuery(q, f.Query)
}

// narrowByQuery keeps the order with that number, or the orders whose
// customer's name starts with the text, ignoring case and taking % and _
// literally.
func narrowByQuery(q *gorm.DB, query string) *gorm.DB {
	if query == "" {
		return q
	}
	if n, ok := apporder.NumberQuery(query); ok {
		return q.Where("orders.number = ?", int(n))
	}
	return q.Where(`EXISTS (SELECT 1 FROM customers c WHERE c.tenant_id = orders.tenant_id
		AND c.id = orders.customer_id AND lower(c.name) LIKE lower(?))`, likeEscaper.Replace(query)+"%")
}

// orderPosition is where a delivery-date page ends: the date and id of its
// last order. The cursor holds both, so it needs no lookup and survives the
// order changing or leaving the filter.
type orderPosition struct {
	date time.Time
	id   string
}

func (p orderPosition) cursor() string {
	text := p.date.UTC().Format(time.DateOnly) + "." + p.id
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

// parseOrderCursor reads what orderPosition.cursor wrote; anything else is
// errBadCursor.
func parseOrderCursor(c string) (orderPosition, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return orderPosition{}, errBadCursor()
	}
	day, id, ok := strings.Cut(string(raw), ".")
	if !ok {
		return orderPosition{}, errBadCursor()
	}
	d, err := shared.ParseCivilDate(day)
	parsed, perr := shared.ParseID(id)
	if err != nil || perr != nil || parsed.String() != id {
		return orderPosition{}, errBadCursor()
	}
	t, err := time.Parse(time.DateOnly, d.String())
	if err != nil {
		return orderPosition{}, errBadCursor()
	}
	return orderPosition{date: t, id: id}, nil
}

// Active returns up to limit open orders by (delivery date, id), each with
// when it entered its current status: the time of the latest history entry
// that moved it into that status, else its creation. The history is staff-only,
// so outside a staff scope every order counts from its creation.
func (*Orders) Active(ctx context.Context, tenant shared.TenantID, limit int) ([]apporder.ActiveOrder, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, nil
	}
	var heads []models.Order
	err = db.Where("tenant_id = ? AND cancelled_at IS NULL AND handed_over_at IS NULL", tenant.String()).
		Order("delivery_date, id").Limit(limit).Find(&heads).Error
	if err != nil {
		return nil, wrap("list active orders", err)
	}
	recs, err := loadRecords(ctx, db, tenant, heads)
	if err != nil {
		return nil, err
	}
	since, err := statusSince(db, tenant, heads)
	if err != nil {
		return nil, err
	}
	out := make([]apporder.ActiveOrder, 0, len(recs))
	for _, rec := range recs {
		at, ok := since[rec.ID.String()]
		if !ok {
			at = rec.CreatedAt
		}
		out = append(out, apporder.ActiveOrder{Record: rec, StatusSince: at})
	}
	return out, nil
}

// statusSince finds, for each order, the time of its latest "status" history
// entry whose target is the order's current status key, in one query.
func statusSince(db *gorm.DB, tenant shared.TenantID, heads []models.Order) (map[string]time.Time, error) {
	ids := make([]string, 0, len(heads))
	for _, h := range heads {
		ids = append(ids, h.ID)
	}
	var rows []struct {
		OrderID string    `gorm:"column:order_id"`
		At      time.Time `gorm:"column:at"`
	}
	err := db.Raw(`SELECT DISTINCT ON (h.order_id) h.order_id, h.at
		FROM order_status_history h
		JOIN orders o ON o.tenant_id = h.tenant_id AND o.id = h.order_id
		WHERE h.tenant_id = ? AND h.order_id IN ? AND h.kind = 'status'
		  AND h.detail #>> '{to,key}' = o.status_key
		ORDER BY h.order_id, h.at DESC, h.id DESC`, tenant.String(), ids).Scan(&rows).Error
	if err != nil {
		return nil, wrap("read when orders entered their status", err)
	}
	out := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		out[r.OrderID] = r.At.UTC()
	}
	return out, nil
}

// History returns one page of an order's history, newest first by (at, id),
// with the name of whoever made each entry. It returns apporder.ErrNotFound
// when the store has no such order. The history is staff-only: another scope
// of the store gets an empty page.
func (*Orders) History(ctx context.Context, tenant shared.TenantID, id shared.OrderID, p shared.PageRequest) (shared.Page[apporder.HistoryRow], error) {
	db, err := conn(ctx)
	if err != nil {
		return shared.Page[apporder.HistoryRow]{}, err
	}
	var n int64
	err = db.Model(&models.Order{}).Where("tenant_id = ? AND id = ?", tenant.String(), id.String()).Count(&n).Error
	if err != nil {
		return shared.Page[apporder.HistoryRow]{}, wrap("check order", err)
	}
	if n == 0 {
		return shared.Page[apporder.HistoryRow]{}, apporder.ErrNotFound
	}
	q := db.Table("order_status_history AS h").
		Select("h.id, h.kind, h.by_user, h.at, h.detail, u.name AS by_name").
		Joins("LEFT JOIN users u ON u.id = h.by_user").
		Where("h.tenant_id = ? AND h.order_id = ?", tenant.String(), id.String())
	after, ok, err := parseCursor(p.Cursor())
	if err != nil {
		return shared.Page[apporder.HistoryRow]{}, err
	}
	if ok {
		q = q.Where("(h.at, h.id) < (?, ?)", after.time(), after.id)
	}
	limit := p.Limit()
	var rows []models.OrderHistoryRead
	if err := q.Order("h.at DESC, h.id DESC").Limit(limit + 1).Scan(&rows).Error; err != nil {
		return shared.Page[apporder.HistoryRow]{}, wrap("read order history", err)
	}
	var next string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next = positionOf(last.At, last.ID).cursor()
	}
	page := shared.Page[apporder.HistoryRow]{Items: make([]apporder.HistoryRow, 0, len(rows)), NextCursor: next}
	for _, r := range rows {
		e, err := r.ToEntry()
		if err != nil {
			// %s, not %w: see loadRecords.
			return shared.Page[apporder.HistoryRow]{}, fmt.Errorf("stored history entry is invalid: %s", err.Error())
		}
		row := apporder.HistoryRow{Entry: e}
		if r.ByName != nil {
			row.ByName = *r.ByName
		}
		page.Items = append(page.Items, row)
	}
	return page, nil
}

// Payments returns the payments recorded in [from, to), voided ones included
// and marked, oldest first.
func (*Orders) Payments(ctx context.Context, tenant shared.TenantID, from, to time.Time) ([]apporder.PaymentRow, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		OrderID     string    `gorm:"column:order_id"`
		Kind        string    `gorm:"column:kind"`
		Method      string    `gorm:"column:method"`
		AmountCents int64     `gorm:"column:amount_cents"`
		Currency    string    `gorm:"column:currency"`
		RecordedAt  time.Time `gorm:"column:recorded_at"`
		Voided      bool      `gorm:"column:voided"`
	}
	err = db.Raw(`SELECT order_id, kind, method, amount_cents, currency, recorded_at, voided_at IS NOT NULL AS voided
		FROM payments
		WHERE tenant_id = ? AND recorded_at >= ? AND recorded_at < ?
		ORDER BY recorded_at, id`, tenant.String(), from.UTC(), to.UTC()).Scan(&rows).Error
	if err != nil {
		return nil, wrap("read payments", err)
	}
	out := make([]apporder.PaymentRow, 0, len(rows))
	for _, r := range rows {
		oid, err := shared.ParseOrderID(r.OrderID)
		if err != nil {
			return nil, fmt.Errorf("stored payment order id is invalid: %s", err.Error()) // a bug, never bad input
		}
		amount, err := shared.NewMoney(r.AmountCents, r.Currency)
		if err != nil {
			return nil, fmt.Errorf("stored payment amount is invalid: %s", err.Error()) // a bug, never bad input
		}
		out = append(out, apporder.PaymentRow{
			Order: oid, Kind: order.PaymentKind(r.Kind), Method: order.PaymentMethod(r.Method),
			Amount: amount, At: r.RecordedAt.UTC(), Voided: r.Voided,
		})
	}
	return out, nil
}

// HandedOver returns the orders handed over in [from, to) by hand-over time,
// at most apporder.HandedOverCap of them.
func (*Orders) HandedOver(ctx context.Context, tenant shared.TenantID, from, to time.Time) ([]order.Record, error) {
	db, err := conn(ctx)
	if err != nil {
		return nil, err
	}
	var heads []models.Order
	err = db.Where("tenant_id = ? AND handed_over_at >= ? AND handed_over_at < ?", tenant.String(), from.UTC(), to.UTC()).
		Order("handed_over_at, id").Limit(apporder.HandedOverCap).Find(&heads).Error
	if err != nil {
		return nil, wrap("list handed over orders", err)
	}
	return loadRecords(ctx, db, tenant, heads)
}

// StaffNames implements apporder.Repository: the users' names, read from
// users (a global table; only id and name are selected). A user that no
// longer exists is absent.
func (*Orders) StaffNames(ctx context.Context, ids []shared.UserID) (map[shared.UserID]string, error) {
	db, err := conn(ctx)
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
	if err := db.Table("users").Select("id::text AS id, name").Where("id IN ?", keys).Scan(&rows).Error; err != nil {
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
