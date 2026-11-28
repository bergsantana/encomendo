package models

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Order maps orders.
//
// ClientAccountID is read-only (<-:false): encomendo_app has no INSERT or
// UPDATE grant on client_account_id (0013), and milestone 7 adds a reviewed
// way to link an account. TrackingTokenHash is the token's SHA-256, 32
// bytes; nothing logs it, because GORM's logger drops every value.
// DeliveryAddress is a jsonb snapshot with no foreign key (0013).
//
// Dates are time.Time at UTC midnight: the date columns come back that way
// through pgx, and the conversion to shared.CivilDate is in this file.
type Order struct {
	TenantID              string     `gorm:"column:tenant_id"`
	ID                    string     `gorm:"column:id;primaryKey"`
	Number                int        `gorm:"column:number"`
	CustomerID            string     `gorm:"column:customer_id"`
	WorkflowID            string     `gorm:"column:workflow_id"`
	WorkflowVersion       int        `gorm:"column:workflow_version"`
	StatusKey             string     `gorm:"column:status_key"`
	ReleaseDate           *time.Time `gorm:"column:release_date;type:date"`
	ReleasedBy            *string    `gorm:"column:released_by"`
	DepositConfirmedCents *int64     `gorm:"column:deposit_confirmed_cents"`
	DeliveryDate          time.Time  `gorm:"column:delivery_date;type:date"`
	PromiseChangedByHand  bool       `gorm:"column:promise_changed_by_hand"`
	AssemblyDays          int        `gorm:"column:assembly_days"`
	SafetyMarginDays      int        `gorm:"column:safety_margin_days"`
	DepositPercent        int        `gorm:"column:deposit_percent"`
	Currency              string     `gorm:"column:currency"`
	SubtotalCents         int64      `gorm:"column:subtotal_cents"`
	FreightCents          int64      `gorm:"column:freight_cents"`
	DiscountCents         int64      `gorm:"column:discount_cents"`
	Handover              string     `gorm:"column:handover"`
	DeliveryAddressID     *string    `gorm:"column:delivery_address_id"`
	DeliveryAddress       []byte     `gorm:"column:delivery_address;type:jsonb"`
	HandedOverAs          *string    `gorm:"column:handed_over_as"`
	HandedOverAt          *time.Time `gorm:"column:handed_over_at"`
	CancelledAt           *time.Time `gorm:"column:cancelled_at"`
	CancelReason          *string    `gorm:"column:cancel_reason"`
	TrackingTokenHash     []byte     `gorm:"column:tracking_token_hash"`
	ClientAccountID       *string    `gorm:"column:client_account_id;<-:false"`
	CreatedBy             *string    `gorm:"column:created_by"`
	Version               int        `gorm:"column:version"`
	CreatedAt             time.Time  `gorm:"column:created_at"`
}

// TableName returns orders.
func (Order) TableName() string { return "orders" }

// OrderUpdate returns the columns of orders an update may change, which are
// the ones 0013 grants, and nothing else: no id, tenant, number, customer,
// workflow, currency, creation fields or client account. The version is the
// new one.
func OrderUpdate(m Order, version int) map[string]any {
	return map[string]any{
		"status_key": m.StatusKey, "release_date": nilable(m.ReleaseDate), "released_by": nilable(m.ReleasedBy),
		"deposit_confirmed_cents": nilable(m.DepositConfirmedCents), "delivery_date": m.DeliveryDate,
		"promise_changed_by_hand": m.PromiseChangedByHand, "assembly_days": m.AssemblyDays,
		"safety_margin_days": m.SafetyMarginDays, "deposit_percent": m.DepositPercent,
		"subtotal_cents": m.SubtotalCents, "freight_cents": m.FreightCents, "discount_cents": m.DiscountCents,
		"handover": m.Handover, "delivery_address_id": nilable(m.DeliveryAddressID),
		"delivery_address": nilableBytes(m.DeliveryAddress), "handed_over_as": nilable(m.HandedOverAs),
		"handed_over_at": nilable(m.HandedOverAt), "cancelled_at": nilable(m.CancelledAt),
		"cancel_reason": nilable(m.CancelReason), "tracking_token_hash": m.TrackingTokenHash,
		"version": version, "updated_at": gorm.Expr("now()"),
	}
}

// OrderLine maps order_lines. Kit is the jsonb KitSnapshot, nil for a loose
// item.
type OrderLine struct {
	TenantID         string  `gorm:"column:tenant_id"`
	ID               string  `gorm:"column:id;primaryKey"`
	OrderID          string  `gorm:"column:order_id"`
	Position         int     `gorm:"column:position"`
	KitID            *string `gorm:"column:kit_id"`
	Kit              []byte  `gorm:"column:kit;type:jsonb"`
	PriceCents       int64   `gorm:"column:price_cents"`
	AssemblyDays     *int    `gorm:"column:assembly_days"`
	SafetyMarginDays *int    `gorm:"column:safety_margin_days"`
}

// TableName returns order_lines.
func (OrderLine) TableName() string { return "order_lines" }

// OrderItem maps order_items. Snapshot is the jsonb ItemSnapshot: what the
// client may see, never a supplier or a cost.
type OrderItem struct {
	TenantID     string     `gorm:"column:tenant_id"`
	ID           string     `gorm:"column:id;primaryKey"`
	OrderID      string     `gorm:"column:order_id"`
	LineID       string     `gorm:"column:line_id"`
	Position     int        `gorm:"column:position"`
	Snapshot     []byte     `gorm:"column:snapshot;type:jsonb"`
	SourcingMode string     `gorm:"column:sourcing_mode"`
	Quantity     int        `gorm:"column:quantity"`
	Status       string     `gorm:"column:status"`
	CheckResult  *string    `gorm:"column:check_result"`
	ExpectedDate time.Time  `gorm:"column:expected_date;type:date"`
	ArrivedOn    *time.Time `gorm:"column:arrived_on;type:date"`
}

// TableName returns order_items.
func (OrderItem) TableName() string { return "order_items" }

// OrderItemSourcing maps order_item_sourcing, the staff-only supplier and
// cost of an item at order time.
type OrderItemSourcing struct {
	TenantID           string  `gorm:"column:tenant_id;primaryKey"`
	OrderItemID        string  `gorm:"column:order_item_id;primaryKey"`
	SupplierID         *string `gorm:"column:supplier_id"`
	SupplierName       string  `gorm:"column:supplier_name"`
	CostCents          *int64  `gorm:"column:cost_cents"`
	BackupSupplierID   *string `gorm:"column:backup_supplier_id"`
	BackupSupplierName *string `gorm:"column:backup_supplier_name"`
	BackupLeadDays     *int    `gorm:"column:backup_lead_days"`
	BackupCostCents    *int64  `gorm:"column:backup_cost_cents"`
	// PromisedDate is the day the supplier promised the item and PaidCostCents
	// the unit price paid (0016): staff-only, so they live here and never on
	// order_items, which clients may read.
	PromisedDate  *time.Time `gorm:"column:promised_date;type:date"`
	PaidCostCents *int64     `gorm:"column:paid_cost_cents"`
}

// TableName returns order_item_sourcing.
func (OrderItemSourcing) TableName() string { return "order_item_sourcing" }

// Payment maps payments.
type Payment struct {
	TenantID    string     `gorm:"column:tenant_id"`
	ID          string     `gorm:"column:id;primaryKey"`
	OrderID     string     `gorm:"column:order_id"`
	Kind        string     `gorm:"column:kind"`
	AmountCents int64      `gorm:"column:amount_cents"`
	Currency    string     `gorm:"column:currency"`
	Method      string     `gorm:"column:method"`
	RecordedBy  *string    `gorm:"column:recorded_by"`
	RecordedAt  time.Time  `gorm:"column:recorded_at"`
	VoidedAt    *time.Time `gorm:"column:voided_at"`
	VoidedBy    *string    `gorm:"column:voided_by"`
	VoidReason  *string    `gorm:"column:void_reason"`
}

// TableName returns payments.
func (Payment) TableName() string { return "payments" }

// OrderRows is every row one order is stored as. Sourcing has one row per
// item.
type OrderRows struct {
	Order    Order
	Lines    []OrderLine
	Items    []OrderItem
	Sourcing []OrderItemSourcing
	Payments []Payment
}

// OrderRowsFromRecord returns the rows of r. Items are numbered from 1
// across the order, in line order, so reading them by position gives the
// order back.
func OrderRowsFromRecord(r order.Record) (OrderRows, error) {
	head, err := orderFromRecord(r)
	if err != nil {
		return OrderRows{}, err
	}
	rows := OrderRows{Order: head}
	tenant, orderID := r.Tenant.String(), r.ID.String()
	for li, l := range r.Lines {
		line, err := lineFromRecord(tenant, orderID, li+1, l)
		if err != nil {
			return OrderRows{}, err
		}
		rows.Lines = append(rows.Lines, line)
		for _, it := range l.Items {
			item, err := itemFromRecord(tenant, orderID, line.ID, len(rows.Items)+1, it)
			if err != nil {
				return OrderRows{}, err
			}
			rows.Items = append(rows.Items, item)
			rows.Sourcing = append(rows.Sourcing, sourcingFromRecord(tenant, it))
		}
	}
	for _, p := range r.Payments {
		rows.Payments = append(rows.Payments, paymentFromRecord(tenant, orderID, head.Currency, p))
	}
	return rows, nil
}

func orderFromRecord(r order.Record) (Order, error) {
	var subtotal int64
	for _, l := range r.Lines {
		subtotal += l.Price.Cents()
	}
	m := Order{
		TenantID: r.Tenant.String(), ID: r.ID.String(), Number: int(r.Number), CustomerID: r.Customer.String(),
		WorkflowID: r.WorkflowID.String(), WorkflowVersion: r.WorkflowVersion, StatusKey: string(r.StatusKey),
		ReleaseDate: dateOrNil(r.ReleaseDate), ReleasedBy: optID(r.ReleasedBy),
		DepositConfirmedCents: r.DepositConfirmedCents, DeliveryDate: dateValue(r.Promise),
		PromiseChangedByHand: r.PromiseChangedByHand, AssemblyDays: r.Terms.AssemblyDays,
		SafetyMarginDays: r.Terms.SafetyMarginDays, DepositPercent: r.Terms.DepositPercent,
		Currency: r.Terms.Freight.Currency(), SubtotalCents: subtotal, FreightCents: r.Terms.Freight.Cents(),
		DiscountCents: r.Terms.Discount.Cents(), Handover: string(r.Terms.Handover),
		HandedOverAs: optString(string(r.HandedOverAs)), HandedOverAt: timeOrNil(r.HandedOverAt),
		CancelledAt: timeOrNil(r.CancelledAt), CancelReason: optString(r.CancelReason),
		TrackingTokenHash: r.Tracking[:], CreatedBy: optID(r.CreatedBy), Version: r.Version,
		CreatedAt: r.CreatedAt.UTC(),
	}
	if a := r.Terms.Address; a != nil {
		raw, err := marshalAddress(*a)
		if err != nil {
			return Order{}, err
		}
		m.DeliveryAddressID, m.DeliveryAddress = optID(a.ID), raw
	}
	return m, nil
}

func lineFromRecord(tenant, orderID string, position int, l order.LineRecord) (OrderLine, error) {
	m := OrderLine{
		TenantID: tenant, ID: l.ID.String(), OrderID: orderID, Position: position, PriceCents: l.Price.Cents(),
		AssemblyDays: l.AssemblyDays, SafetyMarginDays: l.SafetyMarginDays,
	}
	if l.Kit != nil {
		raw, err := marshalKit(*l.Kit)
		if err != nil {
			return OrderLine{}, err
		}
		m.KitID, m.Kit = optID(l.Kit.Kit), raw
	}
	return m, nil
}

func itemFromRecord(tenant, orderID, lineID string, position int, it order.ItemRecord) (OrderItem, error) {
	if it.Expected.IsZero() {
		return OrderItem{}, fmt.Errorf("order item %s has no expected date", it.ID)
	}
	raw, err := marshalItemSnapshot(it.Snapshot)
	if err != nil {
		return OrderItem{}, err
	}
	m := OrderItem{
		TenantID: tenant, ID: it.ID.String(), OrderID: orderID, LineID: lineID, Position: position, Snapshot: raw,
		SourcingMode: string(it.Snapshot.SourcingMode), Quantity: it.Snapshot.Quantity, Status: string(it.Status),
		ExpectedDate: dateValue(it.Expected), ArrivedOn: dateOrNil(it.ArrivedOn),
	}
	if it.Check != nil {
		c := string(*it.Check)
		m.CheckResult = &c
	}
	return m, nil
}

func sourcingFromRecord(tenant string, it order.ItemRecord) OrderItemSourcing {
	s := it.Sourcing
	m := OrderItemSourcing{
		TenantID: tenant, OrderItemID: it.ID.String(), SupplierID: optID(s.Supplier), SupplierName: s.SupplierName,
		CostCents: centsOrNil(s.Cost), PaidCostCents: centsOrNil(s.PaidCost), PromisedDate: dateOrNil(it.Promised),
	}
	if b := s.Backup; b != nil {
		name, lead := b.Name, b.LeadDays
		m.BackupSupplierID, m.BackupSupplierName, m.BackupLeadDays = optID(b.Supplier), &name, &lead
		m.BackupCostCents = centsOrNil(b.Cost)
	}
	return m
}

func paymentFromRecord(tenant, orderID, currency string, p order.PaymentRecord) Payment {
	return Payment{
		TenantID: tenant, ID: p.ID.String(), OrderID: orderID, Kind: string(p.Kind), AmountCents: p.Amount.Cents(),
		Currency: currency, Method: string(p.Method), RecordedBy: optID(p.By), RecordedAt: p.At.UTC(),
		VoidedAt: timeOrNil(p.VoidedAt), VoidedBy: optID(p.VoidedBy), VoidReason: optString(p.VoidReason),
	}
}

// ToRecord rebuilds the order's record. Lines and items must come in
// position order. Sourcing may be empty (a scope with no staff user reads
// none): the items then carry the zero order.Sourcing.
func (rows OrderRows) ToRecord() (order.Record, error) {
	r, err := rows.Order.toRecord()
	if err != nil {
		return order.Record{}, err
	}
	cur := rows.Order.Currency
	sourcing := make(map[string]OrderItemSourcing, len(rows.Sourcing))
	for _, s := range rows.Sourcing {
		sourcing[s.OrderItemID] = s
	}
	byLine := make(map[string]int, len(rows.Lines))
	for _, l := range rows.Lines {
		lr, err := l.toRecord(cur)
		if err != nil {
			return order.Record{}, err
		}
		byLine[l.ID] = len(r.Lines)
		r.Lines = append(r.Lines, lr)
	}
	for _, it := range rows.Items {
		idx, ok := byLine[it.LineID]
		if !ok {
			return order.Record{}, fmt.Errorf("order item %s belongs to line %s, which the order lacks", it.ID, it.LineID)
		}
		ir, err := it.toRecord(cur, sourcing[it.ID])
		if err != nil {
			return order.Record{}, err
		}
		r.Lines[idx].Items = append(r.Lines[idx].Items, ir)
	}
	for _, p := range rows.Payments {
		pr, err := p.toRecord()
		if err != nil {
			return order.Record{}, err
		}
		r.Payments = append(r.Payments, pr)
	}
	return r, nil
}

func (m Order) toRecord() (order.Record, error) {
	var r order.Record
	var err error
	if r.Tenant, err = parseID[shared.TenantID](m.TenantID); err != nil {
		return r, err
	}
	if r.ID, err = parseID[shared.OrderID](m.ID); err != nil {
		return r, err
	}
	if r.Customer, err = parseID[shared.CustomerID](m.CustomerID); err != nil {
		return r, err
	}
	if r.WorkflowID, err = parseID[shared.WorkflowID](m.WorkflowID); err != nil {
		return r, err
	}
	r.Number, r.WorkflowVersion, r.StatusKey = order.Number(m.Number), m.WorkflowVersion, workflow.StatusKey(m.StatusKey)
	if r.Terms, err = m.terms(); err != nil {
		return r, err
	}
	if r.ReleasedBy, err = parseOptID[shared.UserID](m.ReleasedBy); err != nil {
		return r, err
	}
	if r.CreatedBy, err = parseOptID[shared.UserID](m.CreatedBy); err != nil {
		return r, err
	}
	r.ReleaseDate, r.Promise = dateOf(m.ReleaseDate), dateOfValue(m.DeliveryDate)
	r.DepositConfirmedCents, r.PromiseChangedByHand = m.DepositConfirmedCents, m.PromiseChangedByHand
	if m.HandedOverAs != nil {
		r.HandedOverAs = order.Handover(*m.HandedOverAs)
	}
	r.HandedOverAt, r.CancelledAt = zeroTime(m.HandedOverAt), zeroTime(m.CancelledAt)
	if m.CancelReason != nil {
		r.CancelReason = *m.CancelReason
	}
	if len(m.TrackingTokenHash) != len(r.Tracking) {
		return r, fmt.Errorf("order %s: tracking hash has %d bytes", m.ID, len(m.TrackingTokenHash))
	}
	copy(r.Tracking[:], m.TrackingTokenHash)
	r.CreatedAt, r.Version = m.CreatedAt.UTC(), m.Version
	return r, nil
}

func (m Order) terms() (order.Terms, error) {
	t := order.Terms{
		Handover: order.Handover(m.Handover), DepositPercent: m.DepositPercent,
		AssemblyDays: m.AssemblyDays, SafetyMarginDays: m.SafetyMarginDays,
	}
	var err error
	if t.Freight, err = shared.NewMoney(m.FreightCents, m.Currency); err != nil {
		return t, fmt.Errorf("order %s freight: %w", m.ID, err)
	}
	if t.Discount, err = shared.NewMoney(m.DiscountCents, m.Currency); err != nil {
		return t, fmt.Errorf("order %s discount: %w", m.ID, err)
	}
	if m.DeliveryAddress != nil {
		if m.DeliveryAddressID == nil {
			return t, fmt.Errorf("order %s has an address snapshot with no id", m.ID)
		}
		if t.Address, err = unmarshalAddress(*m.DeliveryAddressID, m.DeliveryAddress); err != nil {
			return t, fmt.Errorf("order %s address: %w", m.ID, err)
		}
	}
	return t, nil
}

func (m OrderLine) toRecord(currency string) (order.LineRecord, error) {
	id, err := parseID[shared.OrderLineID](m.ID)
	if err != nil {
		return order.LineRecord{}, err
	}
	price, err := shared.NewMoney(m.PriceCents, currency)
	if err != nil {
		return order.LineRecord{}, fmt.Errorf("order line %s price: %w", m.ID, err)
	}
	l := order.LineRecord{ID: id, Price: price, AssemblyDays: m.AssemblyDays, SafetyMarginDays: m.SafetyMarginDays}
	if m.Kit != nil {
		if m.KitID == nil {
			return l, fmt.Errorf("order line %s has a kit snapshot with no kit id", m.ID)
		}
		if l.Kit, err = unmarshalKit(*m.KitID, m.Kit); err != nil {
			return l, fmt.Errorf("order line %s kit: %w", m.ID, err)
		}
	}
	return l, nil
}

func (m OrderItem) toRecord(currency string, s OrderItemSourcing) (order.ItemRecord, error) {
	id, err := parseID[shared.OrderItemID](m.ID)
	if err != nil {
		return order.ItemRecord{}, err
	}
	snap, err := unmarshalItemSnapshot(m.Snapshot, currency)
	if err != nil {
		return order.ItemRecord{}, fmt.Errorf("order item %s snapshot: %w", m.ID, err)
	}
	// The columns repeat two snapshot fields for checks and queries; the
	// snapshot is the record, and a disagreement is a corrupt row.
	if snap.SourcingMode != catalog.SourcingMode(m.SourcingMode) || snap.Quantity != m.Quantity {
		return order.ItemRecord{}, fmt.Errorf("order item %s: columns disagree with the snapshot", m.ID)
	}
	it := order.ItemRecord{
		ID: id, Snapshot: snap, Status: order.ItemStatus(m.Status),
		Expected: dateOfValue(m.ExpectedDate), ArrivedOn: dateOf(m.ArrivedOn),
	}
	if m.CheckResult != nil {
		c := order.Check(*m.CheckResult)
		it.Check = &c
	}
	if it.Sourcing, err = s.toDomain(currency); err != nil {
		return order.ItemRecord{}, fmt.Errorf("order item %s sourcing: %w", m.ID, err)
	}
	it.Promised = dateOf(s.PromisedDate)
	return it, nil
}

// toDomain rebuilds the sourcing; the zero row (none was read) is the zero
// order.Sourcing.
func (m OrderItemSourcing) toDomain(currency string) (order.Sourcing, error) {
	var s order.Sourcing
	if m.OrderItemID == "" {
		return s, nil
	}
	var err error
	if s.Supplier, err = parseOptID[shared.SupplierID](m.SupplierID); err != nil {
		return s, err
	}
	s.SupplierName = m.SupplierName
	if s.Cost, err = moneyOrNil(m.CostCents, currency); err != nil {
		return s, err
	}
	if s.PaidCost, err = moneyOrNil(m.PaidCostCents, currency); err != nil {
		return s, err
	}
	if m.BackupSupplierID == nil {
		return s, nil
	}
	b := &order.BackupSource{}
	if b.Supplier, err = parseOptID[shared.SupplierID](m.BackupSupplierID); err != nil {
		return s, err
	}
	if m.BackupSupplierName != nil {
		b.Name = *m.BackupSupplierName
	}
	if m.BackupLeadDays != nil {
		b.LeadDays = *m.BackupLeadDays
	}
	if b.Cost, err = moneyOrNil(m.BackupCostCents, currency); err != nil {
		return s, err
	}
	s.Backup = b
	return s, nil
}

func (m Payment) toRecord() (order.PaymentRecord, error) {
	id, err := parseID[shared.PaymentID](m.ID)
	if err != nil {
		return order.PaymentRecord{}, err
	}
	amount, err := shared.NewMoney(m.AmountCents, m.Currency)
	if err != nil {
		return order.PaymentRecord{}, fmt.Errorf("payment %s: %w", m.ID, err)
	}
	p := order.PaymentRecord{
		ID: id, Kind: order.PaymentKind(m.Kind), Amount: amount, Method: order.PaymentMethod(m.Method),
		At: m.RecordedAt.UTC(), VoidedAt: zeroTime(m.VoidedAt),
	}
	if p.By, err = parseOptID[shared.UserID](m.RecordedBy); err != nil {
		return p, err
	}
	if p.VoidedBy, err = parseOptID[shared.UserID](m.VoidedBy); err != nil {
		return p, err
	}
	if m.VoidReason != nil {
		p.VoidReason = *m.VoidReason
	}
	return p, nil
}
