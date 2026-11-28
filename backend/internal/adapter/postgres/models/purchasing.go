package models

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// SupplierOrder maps supplier_orders (0016): a pedido de compra with the
// message it was sent with. created_at and updated_at are left to their
// defaults.
type SupplierOrder struct {
	TenantID      string     `gorm:"column:tenant_id"`
	ID            string     `gorm:"column:id;primaryKey"`
	Number        int        `gorm:"column:number"`
	SupplierID    string     `gorm:"column:supplier_id"`
	SupplierName  string     `gorm:"column:supplier_name"`
	SupplierPhone string     `gorm:"column:supplier_phone"`
	SentAt        time.Time  `gorm:"column:sent_at"`
	SentBy        string     `gorm:"column:sent_by"`
	PromisedDate  *time.Time `gorm:"column:promised_date;type:date"`
	InTransitAt   *time.Time `gorm:"column:in_transit_at"`
	UndoneAt      *time.Time `gorm:"column:undone_at"`
	UndoneBy      *string    `gorm:"column:undone_by"`
	MessageText   string     `gorm:"column:message_text"`
	MessageURL    string     `gorm:"column:message_url"`
	Version       int        `gorm:"column:version"`
}

// TableName returns supplier_orders.
func (SupplierOrder) TableName() string { return "supplier_orders" }

// SupplierOrderFromRecord returns the row of r with the message it was sent
// with.
func SupplierOrderFromRecord(r purchasing.Record, text, url string) SupplierOrder {
	return SupplierOrder{
		TenantID: r.Tenant.String(), ID: r.ID.String(), Number: int(r.Number), SupplierID: r.Supplier.String(),
		SupplierName: r.SupplierName, SupplierPhone: r.SupplierPhone, SentAt: r.SentAt.UTC(), SentBy: r.SentBy.String(),
		PromisedDate: dateOrNil(r.Promised), InTransitAt: timeOrNil(r.InTransitAt), UndoneAt: timeOrNil(r.UndoneAt),
		UndoneBy: optID(r.UndoneBy), MessageText: text, MessageURL: url, Version: r.Version,
	}
}

// SupplierOrderUpdate returns the columns of supplier_orders an update may
// change, which are the ones 0016 grants and nothing else: the supplier, the
// number, the sent facts and the message stay as they were sent. version is
// the new one.
func SupplierOrderUpdate(r purchasing.Record, version int) map[string]any {
	return map[string]any{
		"promised_date": nilable(dateOrNil(r.Promised)), "in_transit_at": nilable(timeOrNil(r.InTransitAt)),
		"undone_at": nilable(timeOrNil(r.UndoneAt)), "undone_by": nilable(optID(r.UndoneBy)),
		"version": version, "updated_at": gorm.Expr("now()"),
	}
}

// ToRecord rebuilds the supplier order's record from its lines, which must
// come in the order they were picked.
func (m SupplierOrder) ToRecord(lines []purchasing.LineRecord) (purchasing.Record, error) {
	r := purchasing.Record{
		Number: purchasing.Number(m.Number), SupplierName: m.SupplierName, SupplierPhone: m.SupplierPhone,
		SentAt: m.SentAt.UTC(), Promised: dateOf(m.PromisedDate), InTransitAt: zeroTime(m.InTransitAt),
		UndoneAt: zeroTime(m.UndoneAt), Lines: lines, Version: m.Version,
	}
	var err error
	if r.Tenant, err = parseID[shared.TenantID](m.TenantID); err != nil {
		return r, err
	}
	if r.ID, err = parseID[shared.SupplierOrderID](m.ID); err != nil {
		return r, err
	}
	if r.Supplier, err = parseID[shared.SupplierID](m.SupplierID); err != nil {
		return r, err
	}
	if r.SentBy, err = parseID[shared.UserID](m.SentBy); err != nil {
		return r, err
	}
	r.UndoneBy, err = parseOptID[shared.UserID](m.UndoneBy)
	return r, err
}

// dbClock is the value of a column the database stamps with its own clock at
// the moment of each statement (clock_timestamp()), not at the start of the
// transaction. A line's created_at carries the order lines were written in,
// which the table has no other column for; the writes of one transaction
// would all tie on now().
type dbClock string

// GormValue implements gorm.Valuer: the column takes clock_timestamp().
func (dbClock) GormValue(context.Context, *gorm.DB) clause.Expr {
	return clause.Expr{SQL: "clock_timestamp()"}
}

// PurchaseLine maps purchase_lines (0016). A line's costs are in
// purchase_line_costs, apart, and a counter line has no supplier order.
//
// The unit costs are never on this row; LineRead joins them.
type PurchaseLine struct {
	TenantID        string     `gorm:"column:tenant_id"`
	ID              string     `gorm:"column:id;primaryKey"`
	SupplierOrderID *string    `gorm:"column:supplier_order_id"`
	SupplierID      string     `gorm:"column:supplier_id"`
	OrderID         string     `gorm:"column:order_id"`
	OrderItemID     *string    `gorm:"column:order_item_id"`
	CatalogItemID   string     `gorm:"column:catalog_item_id"`
	Name            string     `gorm:"column:name"`
	Quantity        int        `gorm:"column:quantity"`
	Choices         []byte     `gorm:"column:choices;type:jsonb"`
	Status          string     `gorm:"column:status"`
	PromisedDate    *time.Time `gorm:"column:promised_date;type:date"`
	CheckNote       string     `gorm:"column:check_note"`
	CancelReason    *string    `gorm:"column:cancel_reason"`
	ReceivedOn      *time.Time `gorm:"column:received_on;type:date"`
	Position        int        `gorm:"column:position"`
	CreatedAt       dbClock    `gorm:"column:created_at;->:false;<-:create"`
}

// TableName returns purchase_lines.
func (PurchaseLine) TableName() string { return "purchase_lines" }

type lineChoiceJSON struct {
	Option string `json:"option"`
	Choice string `json:"choice"`
	Name   string `json:"name"`
	Label  string `json:"label"`
}

func marshalChoices(cs []purchasing.Choice) ([]byte, error) {
	out := make([]lineChoiceJSON, len(cs))
	for i, c := range cs {
		out[i] = lineChoiceJSON{Option: c.Option.String(), Choice: c.ChoiceID.String(), Name: c.Name, Label: c.Label}
	}
	return json.Marshal(out)
}

func unmarshalChoices(raw []byte) ([]purchasing.Choice, error) {
	var in []lineChoiceJSON
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]purchasing.Choice, len(in))
	for i, c := range in {
		opt, err := parseID[shared.OptionID](c.Option)
		if err != nil {
			return nil, err
		}
		ch, err := parseID[shared.ChoiceID](c.Choice)
		if err != nil {
			return nil, err
		}
		out[i] = purchasing.Choice{Option: opt, ChoiceID: ch, Name: c.Name, Label: c.Label}
	}
	return out, nil
}

// PurchaseLineFromRecord returns the row of l, the position-th line (from 0)
// of its supplier order. A line's unit costs are not part of it.
func PurchaseLineFromRecord(tenant shared.TenantID, l purchasing.LineRecord, position int) (PurchaseLine, error) {
	choices, err := marshalChoices(l.Item.Choices)
	if err != nil {
		return PurchaseLine{}, fmt.Errorf("encode choices of line %s: %w", l.ID, err)
	}
	return PurchaseLine{
		TenantID: tenant.String(), ID: l.ID.String(), SupplierOrderID: optID(l.SupplierOrder),
		SupplierID: l.Supplier.String(), OrderID: l.Item.Order.String(), OrderItemID: optID(l.Item.OrderItem),
		CatalogItemID: l.Item.CatalogItem.String(), Name: l.Item.Name, Quantity: l.Item.Quantity, Choices: choices,
		Status: string(l.Status), PromisedDate: dateOrNil(l.Promised), CheckNote: l.CheckNote,
		CancelReason: optString(string(l.CancelReason)), ReceivedOn: dateOrNil(l.ReceivedOn), Position: position,
	}, nil
}

// PurchaseLineUpdate returns the columns of purchase_lines an update may
// change, which are the ones 0016 grants and nothing else: the order, item,
// supplier and snapshot of a line never change.
func PurchaseLineUpdate(l purchasing.LineRecord) map[string]any {
	return map[string]any{
		"status": string(l.Status), "promised_date": nilable(dateOrNil(l.Promised)), "check_note": l.CheckNote,
		"cancel_reason": nilable(optString(string(l.CancelReason))), "received_on": nilable(dateOrNil(l.ReceivedOn)),
		"updated_at": gorm.Expr("now()"),
	}
}

// LineRead is a purchase line as read: the row, its costs from
// purchase_line_costs, and the facts of its supplier order, all null for
// what the line lacks (a counter line has no supplier order).
type LineRead struct {
	PurchaseLine  `gorm:"embedded"`
	CostCurrency  *string    `gorm:"column:cost_currency"`
	CostCents     *int64     `gorm:"column:cost_cents"`
	PaidCostCents *int64     `gorm:"column:paid_cost_cents"`
	OrderNumber   *int       `gorm:"column:order_number"`
	OrderSupplier *string    `gorm:"column:order_supplier_name"`
	OrderPhone    *string    `gorm:"column:order_supplier_phone"`
	OrderSentAt   *time.Time `gorm:"column:order_sent_at"`
	OrderPromised *time.Time `gorm:"column:order_promised_date;type:date"`
}

// ToRecord rebuilds the line's record.
func (m LineRead) ToRecord() (purchasing.LineRecord, error) {
	l := purchasing.LineRecord{
		Status: purchasing.LineStatus(m.Status), Promised: dateOf(m.PromisedDate), CheckNote: m.CheckNote,
		CancelReason: purchasing.CancelReason(deref(m.CancelReason)), ReceivedOn: dateOf(m.ReceivedOn),
	}
	l.Item = purchasing.Item{Name: m.Name, Quantity: m.Quantity}
	var err error
	if l.ID, err = parseID[shared.PurchaseLineID](m.ID); err != nil {
		return l, err
	}
	if l.SupplierOrder, err = parseOptID[shared.SupplierOrderID](m.SupplierOrderID); err != nil {
		return l, err
	}
	if l.Supplier, err = parseID[shared.SupplierID](m.SupplierID); err != nil {
		return l, err
	}
	if l.Item.Order, err = parseID[shared.OrderID](m.OrderID); err != nil {
		return l, err
	}
	if l.Item.OrderItem, err = parseOptID[shared.OrderItemID](m.OrderItemID); err != nil {
		return l, err
	}
	if l.Item.CatalogItem, err = parseID[shared.ItemID](m.CatalogItemID); err != nil {
		return l, err
	}
	if l.Item.Choices, err = unmarshalChoices(m.Choices); err != nil {
		return l, fmt.Errorf("line %s choices: %w", m.ID, err)
	}
	if m.CostCurrency != nil {
		if l.Cost, err = moneyOrNil(m.CostCents, *m.CostCurrency); err != nil {
			return l, err
		}
		l.Paid, err = moneyOrNil(m.PaidCostCents, *m.CostCurrency)
	}
	return l, err
}

// SentAt returns when the line's supplier order was sent, or the zero time.
func (m LineRead) SentAt() time.Time { return zeroTime(m.OrderSentAt) }

// Promised returns the supplier order's promised date, or the zero date.
func (m LineRead) Promised() shared.CivilDate { return dateOf(m.OrderPromised) }

// Number returns the line's supplier order number, or 0.
func (m LineRead) Number() purchasing.Number {
	if m.OrderNumber == nil {
		return 0
	}
	return purchasing.Number(*m.OrderNumber)
}

// SupplierFacts returns the supplier's name and phone as the line's order
// was sent, or empty strings.
func (m LineRead) SupplierFacts() (name, phone string) {
	return deref(m.OrderSupplier), deref(m.OrderPhone)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Receipt maps receipts (0016). The table is append-only.
type Receipt struct {
	TenantID   string    `gorm:"column:tenant_id"`
	ID         string    `gorm:"column:id;primaryKey"`
	ReceivedBy string    `gorm:"column:received_by"`
	ReceivedAt time.Time `gorm:"column:received_at"`
}

// TableName returns receipts.
func (Receipt) TableName() string { return "receipts" }

// ReceiptLine maps receipt_lines: one line a receipt checked. created_at
// carries the entry's place in the receipt (a microsecond after the one
// before), because the table has no position and a read must give the
// entries back in the order they were checked.
type ReceiptLine struct {
	TenantID    string    `gorm:"column:tenant_id"`
	ReceiptID   string    `gorm:"column:receipt_id"`
	LineID      string    `gorm:"column:line_id"`
	CheckResult string    `gorm:"column:check_result"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

// TableName returns receipt_lines.
func (ReceiptLine) TableName() string { return "receipt_lines" }

// ReceiptPhoto maps receipt_photos: an item photo (with a line) or an
// invoice photo (without).
type ReceiptPhoto struct {
	TenantID    string  `gorm:"column:tenant_id"`
	ID          string  `gorm:"column:id;primaryKey"`
	ReceiptID   string  `gorm:"column:receipt_id"`
	LineID      *string `gorm:"column:line_id"`
	ObjectKey   string  `gorm:"column:object_key"`
	ContentType string  `gorm:"column:content_type"`
	SizeBytes   int64   `gorm:"column:size_bytes"`
}

// TableName returns receipt_photos.
func (ReceiptPhoto) TableName() string { return "receipt_photos" }

// ReceiptRows is every row one receipt is stored as.
type ReceiptRows struct {
	Receipt Receipt
	Lines   []ReceiptLine
	Photos  []ReceiptPhoto
}

// ReceiptRowsFromDomain returns the rows of r: the receipt, a row for each
// entry in the order given, and a row for each photo of an entry or of the
// invoice.
func ReceiptRowsFromDomain(r *purchasing.Receipt) ReceiptRows {
	tenant, id := r.Tenant().String(), r.ID().String()
	rows := ReceiptRows{Receipt: Receipt{
		TenantID: tenant, ID: id, ReceivedBy: r.By().String(), ReceivedAt: r.At().UTC(),
	}}
	for i, e := range r.Entries() {
		line := e.Line.String()
		rows.Lines = append(rows.Lines, ReceiptLine{
			TenantID: tenant, ReceiptID: id, LineID: line, CheckResult: string(e.Check),
			CreatedAt: r.At().UTC().Add(time.Duration(i) * time.Microsecond),
		})
		for _, p := range e.Photos {
			rows.Photos = append(rows.Photos, photoRow(tenant, id, &line, p))
		}
	}
	for _, p := range r.Invoice() {
		rows.Photos = append(rows.Photos, photoRow(tenant, id, nil, p))
	}
	return rows
}

func photoRow(tenant, receipt string, line *string, p purchasing.Photo) ReceiptPhoto {
	return ReceiptPhoto{
		TenantID: tenant, ID: p.ID.String(), ReceiptID: receipt, LineID: line,
		ObjectKey: p.Key, ContentType: p.ContentType, SizeBytes: p.Size,
	}
}
