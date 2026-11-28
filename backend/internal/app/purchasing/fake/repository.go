package fake

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// CostSource tells the order-time unit cost of an order item: what the
// Postgres repository reads from order_item_sourcing in SQL when it copies
// a line's cost. *OrderItems is one.
type CostSource interface {
	Cost(item shared.OrderItemID) (shared.Money, bool)
}

type orderKey struct {
	tenant shared.TenantID
	id     shared.SupplierOrderID
}

type orderRow struct {
	rec       purchasing.Record
	text, url string
}

type lineKey = shared.PurchaseLineID

// counterRow is a line received at the counter.
type counterRow struct {
	tenant shared.TenantID
	rec    purchasing.LineRecord
}

type receiptRow struct {
	tenant shared.TenantID
	id     shared.ReceiptID
	at     time.Time
	lines  []shared.PurchaseLineID
}

// Repository is an in-memory apppurchasing.Repository. Like adapter/postgres
// it refuses calls outside a fake transaction, shows rows only in a scope of
// their store, keeps one open line per order item, checks versions, and a
// failed transaction undoes its writes.
type Repository struct {
	mu       sync.Mutex
	orders   map[orderKey]orderRow
	counters map[shared.TenantID]purchasing.Number
	counter  map[lineKey]counterRow
	seq      map[lineKey]int // the order lines were written in
	nextSeq  int
	receipts map[shared.ReceiptID]receiptRow
	received map[lineKey]bool // lines in a receipt
	photos   map[string]bool
	names    map[shared.UserID]string
	costs    CostSource
	fail     error
}

var _ apppurchasing.Repository = (*Repository)(nil)

// NewRepository returns an empty repository.
func NewRepository() *Repository {
	return &Repository{
		orders: map[orderKey]orderRow{}, counters: map[shared.TenantID]purchasing.Number{},
		counter: map[lineKey]counterRow{}, seq: map[lineKey]int{}, receipts: map[shared.ReceiptID]receiptRow{},
		received: map[lineKey]bool{}, photos: map[string]bool{}, names: map[shared.UserID]string{},
	}
}

// LinkCosts gives the repository what Postgres joins in SQL: the order-time
// costs to copy into new lines. Without it lines get no cost.
func (r *Repository) LinkCosts(c CostSource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.costs = c
}

// NameUser sets the name StaffNames gives for a user.
func (r *Repository) NameUser(id shared.UserID, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names[id] = name
}

// Fail makes every later call return err; nil heals it.
func (r *Repository) Fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fail = err
}

// ForgetItem sets the order item of every line to the zero id, as the
// foreign key's ON DELETE SET NULL does when the order drops the item. It
// is not a repository method: the order context's delete causes it.
func (r *Repository) ForgetItem(item shared.OrderItemID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, row := range r.orders {
		for i := range row.rec.Lines {
			if row.rec.Lines[i].Item.OrderItem == item {
				row.rec.Lines[i].Item.OrderItem = shared.OrderItemID{}
			}
		}
		r.orders[k] = row
	}
	for k, row := range r.counter {
		if row.rec.Item.OrderItem == item {
			row.rec.Item.OrderItem = shared.OrderItemID{}
			r.counter[k] = row
		}
	}
}

// enter checks the call and returns with r.mu held on success.
func (r *Repository) enter(ctx context.Context, tenant shared.TenantID) error {
	if !sharedfake.InTx(ctx) {
		return appshared.ErrNoTransaction
	}
	r.mu.Lock()
	if r.fail != nil {
		r.mu.Unlock()
		return r.fail
	}
	if appshared.ScopeFrom(ctx).TenantID != tenant || appshared.ScopeFrom(ctx).UserID.IsZero() {
		r.mu.Unlock()
		return errors.New("fake: new row violates row-level security policy")
	}
	return nil
}

// NextNumber implements apppurchasing.Repository.
func (r *Repository) NextNumber(ctx context.Context, tenant shared.TenantID) (purchasing.Number, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return 0, err
	}
	defer r.mu.Unlock()
	sharedfake.KeepOnRollback(ctx, &r.mu, r.counters, tenant)
	r.counters[tenant]++
	return r.counters[tenant], nil
}

// PeekNumber implements apppurchasing.Repository.
func (r *Repository) PeekNumber(ctx context.Context, tenant shared.TenantID) (purchasing.Number, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return 0, err
	}
	defer r.mu.Unlock()
	return r.counters[tenant] + 1, nil
}

// InsertSupplierOrder implements apppurchasing.Repository.
func (r *Repository) InsertSupplierOrder(ctx context.Context, o *purchasing.SupplierOrder, text, url string) error {
	if err := r.enter(ctx, o.Tenant()); err != nil {
		return err
	}
	defer r.mu.Unlock()
	rec := o.Record()
	if err := r.checkNew(rec); err != nil {
		return err
	}
	for i := range rec.Lines {
		rec.Lines[i].Cost = r.costOf(rec.Lines[i].Item.OrderItem)
	}
	k := orderKey{rec.Tenant, rec.ID}
	sharedfake.KeepOnRollback(ctx, &r.mu, r.orders, k)
	r.orders[k] = orderRow{rec: rec, text: text, url: url}
	for _, l := range rec.Lines {
		r.track(ctx, l.ID)
	}
	return nil
}

// checkNew refuses a supplier order whose id, number, line id or open item
// is taken. The caller holds the lock.
func (r *Repository) checkNew(rec purchasing.Record) error {
	for k, row := range r.orders {
		if k.id == rec.ID || (k.tenant == rec.Tenant && row.rec.Number == rec.Number) {
			return fmt.Errorf("fake: supplier order %s or number %s taken: %w", rec.ID, rec.Number, shared.ErrConflict)
		}
		for _, have := range row.rec.Lines {
			for _, l := range rec.Lines {
				if have.ID == l.ID {
					return fmt.Errorf("fake: line %s taken: %w", l.ID, shared.ErrConflict)
				}
				if k.tenant == rec.Tenant && have.Status.Open() && !l.Item.OrderItem.IsZero() && have.Item.OrderItem == l.Item.OrderItem {
					return fmt.Errorf("fake: item %s has an open line: %w", l.Item.OrderItem, shared.ErrConflict)
				}
			}
		}
	}
	return nil
}

// costOf copies the order-time cost of an item. The caller holds the lock.
func (r *Repository) costOf(item shared.OrderItemID) *shared.Money {
	if r.costs == nil {
		return nil
	}
	c, ok := r.costs.Cost(item)
	if !ok {
		return nil
	}
	return &c
}

// track gives a new line its place in the order lines were written. The
// caller holds the lock.
func (r *Repository) track(ctx context.Context, id lineKey) {
	sharedfake.KeepOnRollback(ctx, &r.mu, r.seq, id)
	r.nextSeq++
	r.seq[id] = r.nextSeq
}

// UpdateSupplierOrder implements apppurchasing.Repository.
func (r *Repository) UpdateSupplierOrder(ctx context.Context, o *purchasing.SupplierOrder) error {
	if err := r.enter(ctx, o.Tenant()); err != nil {
		return err
	}
	defer r.mu.Unlock()
	k := orderKey{o.Tenant(), o.ID()}
	row, ok := r.orders[k]
	if !ok {
		return purchasing.ErrSupplierOrderNotFound
	}
	if row.rec.Version != o.Version() {
		return shared.ErrVersionConflict
	}
	rec := o.Record()
	if len(rec.Lines) != len(row.rec.Lines) {
		return fmt.Errorf("fake: a supplier order's lines cannot change: %w", shared.ErrInvalid)
	}
	for i := range rec.Lines {
		if rec.Lines[i].ID != row.rec.Lines[i].ID {
			return fmt.Errorf("fake: a supplier order's lines cannot change: %w", shared.ErrInvalid)
		}
		rec.Lines[i].Cost = row.rec.Lines[i].Cost // the cost is never updated
	}
	rec.Version++
	sharedfake.KeepOnRollback(ctx, &r.mu, r.orders, k)
	r.orders[k] = orderRow{rec: rec, text: row.text, url: row.url}
	return nil
}

// SupplierOrder implements apppurchasing.Repository.
func (r *Repository) SupplierOrder(ctx context.Context, tenant shared.TenantID, id shared.SupplierOrderID) (purchasing.Record, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return purchasing.Record{}, err
	}
	defer r.mu.Unlock()
	row, ok := r.orders[orderKey{tenant, id}]
	if !ok {
		return purchasing.Record{}, purchasing.ErrSupplierOrderNotFound
	}
	return cloneRecord(row.rec), nil
}

// SupplierOrderOfLine implements apppurchasing.Repository.
func (r *Repository) SupplierOrderOfLine(ctx context.Context, tenant shared.TenantID, line shared.PurchaseLineID) (purchasing.Record, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return purchasing.Record{}, err
	}
	defer r.mu.Unlock()
	for k, row := range r.orders {
		if k.tenant == tenant && slices.ContainsFunc(row.rec.Lines, func(l purchasing.LineRecord) bool { return l.ID == line }) {
			return cloneRecord(row.rec), nil
		}
	}
	return purchasing.Record{}, purchasing.ErrLineNotFound
}

// Message implements apppurchasing.Repository.
func (r *Repository) Message(ctx context.Context, tenant shared.TenantID, id shared.SupplierOrderID) (apppurchasing.SentMessage, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return apppurchasing.SentMessage{}, err
	}
	defer r.mu.Unlock()
	row, ok := r.orders[orderKey{tenant, id}]
	if !ok {
		return apppurchasing.SentMessage{}, purchasing.ErrSupplierOrderNotFound
	}
	return apppurchasing.SentMessage{Text: row.text, URL: row.url}, nil
}

// SupplierOrders implements apppurchasing.Repository. Its cursor is
// "{sent at, unix nanoseconds}|{id}".
func (r *Repository) SupplierOrders(ctx context.Context, tenant shared.TenantID, open bool, p shared.PageRequest) (shared.Page[purchasing.Record], error) {
	if err := r.enter(ctx, tenant); err != nil {
		return shared.Page[purchasing.Record]{}, err
	}
	defer r.mu.Unlock()
	after, err := parseCursor(p.Cursor())
	if err != nil {
		return shared.Page[purchasing.Record]{}, err
	}
	var rows []purchasing.Record
	for k, row := range r.orders {
		if k.tenant != tenant || (open && !hasOpenLine(row.rec)) {
			continue
		}
		if p.Cursor() != "" && !before(row.rec, after) {
			continue
		}
		rows = append(rows, cloneRecord(row.rec))
	}
	slices.SortFunc(rows, func(a, b purchasing.Record) int { return -cmpRecords(a, b) })
	var page shared.Page[purchasing.Record]
	if len(rows) > p.Limit() {
		rows = rows[:p.Limit()]
		last := rows[len(rows)-1]
		page.NextCursor = strconv.FormatInt(last.SentAt.UnixNano(), 10) + "|" + last.ID.String()
	}
	page.Items = rows
	return page, nil
}

type cursor struct {
	at int64
	id string
}

func parseCursor(c string) (cursor, error) {
	if c == "" {
		return cursor{}, nil
	}
	at, id, ok := strings.Cut(c, "|")
	n, err := strconv.ParseInt(at, 10, 64)
	if _, perr := shared.ParseSupplierOrderID(id); !ok || err != nil || perr != nil {
		return cursor{}, shared.NewValidationError(shared.FieldError{Field: "cursor", Message: "Cursor inválido."})
	}
	return cursor{at: n, id: id}, nil
}

// before reports whether rec sorts after c in the newest-first order.
func before(rec purchasing.Record, c cursor) bool {
	return cmp.Or(cmp.Compare(rec.SentAt.UnixNano(), c.at), strings.Compare(rec.ID.String(), c.id)) < 0
}

func cmpRecords(a, b purchasing.Record) int {
	return cmp.Or(a.SentAt.Compare(b.SentAt), strings.Compare(a.ID.String(), b.ID.String()))
}

func hasOpenLine(rec purchasing.Record) bool {
	return slices.ContainsFunc(rec.Lines, func(l purchasing.LineRecord) bool { return l.Status.Open() })
}

// OpenLines implements apppurchasing.Repository.
func (r *Repository) OpenLines(ctx context.Context, tenant shared.TenantID) ([]apppurchasing.StoredLine, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return nil, err
	}
	defer r.mu.Unlock()
	var recs []purchasing.Record
	for k, row := range r.orders {
		if k.tenant == tenant {
			recs = append(recs, row.rec)
		}
	}
	slices.SortFunc(recs, cmpRecords)
	var out []apppurchasing.StoredLine
	for _, rec := range recs {
		for _, l := range rec.Lines {
			if l.Status.Open() {
				out = append(out, storedLine(rec, l))
			}
		}
	}
	return out, nil
}

// LinesOfOrder implements apppurchasing.Repository.
func (r *Repository) LinesOfOrder(ctx context.Context, tenant shared.TenantID, id shared.OrderID) ([]apppurchasing.StoredLine, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return nil, err
	}
	defer r.mu.Unlock()
	var out []apppurchasing.StoredLine
	for k, row := range r.orders {
		for _, l := range row.rec.Lines {
			if k.tenant == tenant && l.Item.Order == id {
				out = append(out, storedLine(row.rec, l))
			}
		}
	}
	for _, row := range r.counter {
		if row.tenant == tenant && row.rec.Item.Order == id {
			out = append(out, apppurchasing.StoredLine{LineRecord: cloneLine(row.rec)})
		}
	}
	slices.SortFunc(out, func(a, b apppurchasing.StoredLine) int { return cmp.Compare(r.seq[a.ID], r.seq[b.ID]) })
	return out, nil
}

// DefectiveItems implements apppurchasing.Repository.
func (r *Repository) DefectiveItems(ctx context.Context, tenant shared.TenantID, items []shared.OrderItemID) (map[shared.OrderItemID]bool, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return nil, err
	}
	defer r.mu.Unlock()
	out := map[shared.OrderItemID]bool{}
	note := func(l purchasing.LineRecord) {
		if l.Status == purchasing.LineDefective && slices.Contains(items, l.Item.OrderItem) {
			out[l.Item.OrderItem] = true
		}
	}
	for k, row := range r.orders {
		if k.tenant == tenant {
			for _, l := range row.rec.Lines {
				note(l)
			}
		}
	}
	for _, row := range r.counter {
		if row.tenant == tenant {
			note(row.rec)
		}
	}
	return out, nil
}

// InsertCounterLine implements apppurchasing.Repository.
func (r *Repository) InsertCounterLine(ctx context.Context, tenant shared.TenantID, l purchasing.Line) error {
	if err := r.enter(ctx, tenant); err != nil {
		return err
	}
	defer r.mu.Unlock()
	rec := l.Record()
	if !rec.SupplierOrder.IsZero() || (rec.Status != purchasing.LineReceived && rec.Status != purchasing.LineDefective) {
		return fmt.Errorf("fake: a counter line is received or defective with no supplier order: %w", shared.ErrInvalid)
	}
	if r.lineTaken(rec.ID) {
		return fmt.Errorf("fake: line %s taken: %w", rec.ID, shared.ErrConflict)
	}
	rec.Cost = r.costOf(rec.Item.OrderItem)
	sharedfake.KeepOnRollback(ctx, &r.mu, r.counter, rec.ID)
	r.counter[rec.ID] = counterRow{tenant: tenant, rec: rec}
	r.track(ctx, rec.ID)
	return nil
}

func (r *Repository) lineTaken(id lineKey) bool {
	if _, ok := r.counter[id]; ok {
		return true
	}
	for _, row := range r.orders {
		if slices.ContainsFunc(row.rec.Lines, func(l purchasing.LineRecord) bool { return l.ID == id }) {
			return true
		}
	}
	return false
}

// lineRecord finds a stored line of tenant. The caller holds the lock.
func (r *Repository) lineRecord(tenant shared.TenantID, id lineKey) (purchasing.LineRecord, bool) {
	if row, ok := r.counter[id]; ok && row.tenant == tenant {
		return row.rec, true
	}
	for k, row := range r.orders {
		if k.tenant != tenant {
			continue
		}
		for _, l := range row.rec.Lines {
			if l.ID == id {
				return l, true
			}
		}
	}
	return purchasing.LineRecord{}, false
}

// InsertReceipt implements apppurchasing.Repository.
func (r *Repository) InsertReceipt(ctx context.Context, rc *purchasing.Receipt) error {
	if err := r.enter(ctx, rc.Tenant()); err != nil {
		return err
	}
	defer r.mu.Unlock()
	if _, ok := r.receipts[rc.ID()]; ok {
		return fmt.Errorf("fake: receipt %s exists: %w", rc.ID(), shared.ErrConflict)
	}
	row := receiptRow{tenant: rc.Tenant(), id: rc.ID(), at: rc.At()}
	var keys []string
	for _, e := range rc.Entries() {
		l, ok := r.lineRecord(rc.Tenant(), e.Line)
		if !ok || (l.Status != purchasing.LineReceived && l.Status != purchasing.LineDefective) {
			return fmt.Errorf("fake: line %s is not received: %w", e.Line, shared.ErrInvalid)
		}
		if r.received[e.Line] {
			return fmt.Errorf("fake: line %s: %w", e.Line, purchasing.ErrLineClosed)
		}
		row.lines = append(row.lines, e.Line)
		for _, p := range e.Photos {
			keys = append(keys, p.Key)
		}
	}
	for _, p := range rc.Invoice() {
		keys = append(keys, p.Key)
	}
	for _, k := range keys {
		if r.photos[k] {
			return fmt.Errorf("fake: photo %s taken: %w", k, shared.ErrConflict)
		}
	}
	sharedfake.KeepOnRollback(ctx, &r.mu, r.receipts, rc.ID())
	r.receipts[rc.ID()] = row
	for _, id := range row.lines {
		sharedfake.KeepOnRollback(ctx, &r.mu, r.received, id)
		r.received[id] = true
	}
	for _, k := range keys {
		sharedfake.KeepOnRollback(ctx, &r.mu, r.photos, k)
		r.photos[k] = true
	}
	return nil
}

// Receipt implements apppurchasing.Repository: its lines as they are now.
func (r *Repository) Receipt(ctx context.Context, tenant shared.TenantID, id shared.ReceiptID) (apppurchasing.StoredReceipt, error) {
	if err := r.enter(ctx, tenant); err != nil {
		return apppurchasing.StoredReceipt{}, err
	}
	defer r.mu.Unlock()
	row, ok := r.receipts[id]
	if !ok || row.tenant != tenant {
		return apppurchasing.StoredReceipt{}, apppurchasing.ErrReceiptNotFound
	}
	out := apppurchasing.StoredReceipt{ID: id, At: row.at}
	for _, lid := range row.lines {
		l, _ := r.lineRecord(tenant, lid)
		out.Lines = append(out.Lines, cloneLine(l))
	}
	return out, nil
}

// StaffNames implements apppurchasing.Repository, with the names set by
// NameUser.
func (r *Repository) StaffNames(ctx context.Context, ids []shared.UserID) (map[shared.UserID]string, error) {
	if !sharedfake.InTx(ctx) {
		return nil, appshared.ErrNoTransaction
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return nil, r.fail
	}
	out := map[shared.UserID]string{}
	for _, id := range ids {
		if n, ok := r.names[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

// Stored returns the committed record of a supplier order, for assertions.
func (r *Repository) Stored(tenant shared.TenantID, id shared.SupplierOrderID) (purchasing.Record, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.orders[orderKey{tenant, id}]
	return cloneRecord(row.rec), ok
}

// CounterLines returns the lines received at the counter, for assertions.
func (r *Repository) CounterLines() []purchasing.LineRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []purchasing.LineRecord
	for _, row := range r.counter {
		out = append(out, cloneLine(row.rec))
	}
	slices.SortFunc(out, func(a, b purchasing.LineRecord) int { return cmp.Compare(r.seq[a.ID], r.seq[b.ID]) })
	return out
}

// Receipts returns how many receipts are stored, for assertions.
func (r *Repository) Receipts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.receipts)
}

func storedLine(rec purchasing.Record, l purchasing.LineRecord) apppurchasing.StoredLine {
	return apppurchasing.StoredLine{
		LineRecord: cloneLine(l), Number: rec.Number, SupplierName: rec.SupplierName,
		SupplierPhone: rec.SupplierPhone, SentAt: rec.SentAt, OrderPromised: rec.Promised,
	}
}

func cloneLine(l purchasing.LineRecord) purchasing.LineRecord {
	l.Item.Choices = slices.Clone(l.Item.Choices)
	if l.Cost != nil {
		c := *l.Cost
		l.Cost = &c
	}
	if l.Paid != nil {
		p := *l.Paid
		l.Paid = &p
	}
	return l
}

func cloneRecord(rec purchasing.Record) purchasing.Record {
	lines := make([]purchasing.LineRecord, len(rec.Lines))
	for i, l := range rec.Lines {
		lines[i] = cloneLine(l)
	}
	rec.Lines = lines
	return rec
}
