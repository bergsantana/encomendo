package purchasing

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// State is where a supplier order stands. The values are the contract's
// SupplierOrderState.
type State string

// The states.
const (
	// StateSent: a line is still sent, so nothing says it is on its way.
	StateSent State = "sent"
	// StateInTransit: every open line is in transit.
	StateInTransit State = "in_transit"
	// StateReceived: no line is open any more.
	StateReceived State = "received"
	// StateUndone: "Desfazer envio" cancelled it.
	StateUndone State = "undone"
)

// SupplierOrder is a "pedido de compra" (C-0012): the lines sent to one
// supplier at once. Its fields are unexported and change only through its
// methods.
type SupplierOrder struct {
	tenant                      shared.TenantID
	id                          shared.SupplierOrderID
	number                      Number
	supplier                    shared.SupplierID
	supplierName, supplierPhone string // as sent
	sentAt                      time.Time
	sentBy                      shared.UserID
	promised                    shared.CivilDate
	inTransitAt                 time.Time
	undoneAt                    time.Time
	undoneBy                    shared.UserID
	lines                       []Line
	version                     int
}

// SendSpec is what Send needs.
type SendSpec struct {
	Tenant                      shared.TenantID
	ID                          shared.SupplierOrderID
	Number                      Number
	Supplier                    shared.SupplierID
	SupplierName, SupplierPhone string
	Lines                       []LineSpec // one per picked item, each with its id and cost
	By                          shared.Principal
	Now                         time.Time
}

// LineSpec is one picked item with the id its line will have and its unit
// cost at order time (nil when unknown).
type LineSpec struct {
	ID   shared.PurchaseLineID
	Item Item
	Cost *shared.Money
}

// ItemDate is an order item with the day its supplier promised it.
type ItemDate struct {
	Order shared.OrderID
	Item  shared.OrderItemID
	Date  shared.CivilDate
}

// ItemPrice is an order item with the unit price paid for it.
type ItemPrice struct {
	Order shared.OrderID
	Item  shared.OrderItemID
	Price shared.Money
}

// Send makes a supplier order with every line sent. It returns
// ErrEmptyPick for no lines, ErrItemPickedTwice for one order item twice,
// and a ValidationError on "lines[i].name", "lines[i].quantity" or
// "lines[i].cost" for a bad name, quantity or cost.
func Send(s SendSpec) (*SupplierOrder, error) {
	if s.Tenant.IsZero() || s.ID.IsZero() || s.Supplier.IsZero() || s.By.UserID().IsZero() || s.Now.IsZero() || !s.Number.Valid() {
		return nil, fmt.Errorf("send supplier order: %w", errMissing)
	}
	if strings.TrimSpace(s.SupplierName) == "" {
		return nil, fieldError("supplierName", msgItemName)
	}
	lines, err := newLines(s)
	if err != nil {
		return nil, err
	}
	return &SupplierOrder{
		tenant: s.Tenant, id: s.ID, number: s.Number, supplier: s.Supplier,
		supplierName: strings.TrimSpace(s.SupplierName), supplierPhone: s.SupplierPhone,
		sentAt: s.Now, sentBy: s.By.UserID(), lines: lines, version: 1,
	}, nil
}

// newLines builds the sent lines of a pick, checking each one.
func newLines(s SendSpec) ([]Line, error) {
	if len(s.Lines) == 0 {
		return nil, refuse(ErrEmptyPick, "lines", msgNoLines)
	}
	lines := make([]Line, 0, len(s.Lines))
	seen := make(map[shared.OrderItemID]bool, len(s.Lines))
	for i, ls := range s.Lines {
		field := fmt.Sprintf("lines[%d]", i)
		l, err := newLine(ls, s.Supplier, s.ID, LineSent, field)
		if err != nil {
			return nil, err
		}
		if seen[ls.Item.OrderItem] {
			return nil, refuse(ErrItemPickedTwice, field+".orderItemId", msgPickedTwice)
		}
		seen[ls.Item.OrderItem] = true
		lines = append(lines, l)
	}
	return lines, nil
}

// newLine checks one line spec and returns the line with status. The id of
// the supplier order is zero for a counter line.
func newLine(ls LineSpec, supplier shared.SupplierID, order shared.SupplierOrderID, status LineStatus, field string) (Line, error) {
	if ls.ID.IsZero() {
		return Line{}, fmt.Errorf("%s: line id: %w", field, errMissing)
	}
	if err := ls.Item.validate(field, true); err != nil {
		return Line{}, err
	}
	if ls.Cost != nil {
		if err := validateMoney(*ls.Cost, field+".cost"); err != nil {
			return Line{}, err
		}
	}
	return Line{
		id: ls.ID, supplierOrder: order, supplier: supplier, item: ls.Item.clone(),
		status: status, cost: copyMoney(ls.Cost),
	}, nil
}

// Tenant returns the store the supplier order belongs to.
func (o *SupplierOrder) Tenant() shared.TenantID { return o.tenant }

// ID returns the supplier order's id.
func (o *SupplierOrder) ID() shared.SupplierOrderID { return o.id }

// Number returns the per-store number, shown as C-0012.
func (o *SupplierOrder) Number() Number { return o.number }

// Supplier returns the supplier the order was sent to.
func (o *SupplierOrder) Supplier() shared.SupplierID { return o.supplier }

// SupplierName returns the supplier's name as it was when sent.
func (o *SupplierOrder) SupplierName() string { return o.supplierName }

// SupplierPhone returns the supplier's phone as it was when sent.
func (o *SupplierOrder) SupplierPhone() string { return o.supplierPhone }

// SentAt returns when the supplier order was sent.
func (o *SupplierOrder) SentAt() time.Time { return o.sentAt }

// SentBy returns who sent it.
func (o *SupplierOrder) SentBy() shared.UserID { return o.sentBy }

// Promised returns the date the supplier promised the whole order, or the
// zero date when none was recorded.
func (o *SupplierOrder) Promised() shared.CivilDate { return o.promised }

// InTransitAt returns when the order was marked in transit, or the zero time.
func (o *SupplierOrder) InTransitAt() time.Time { return o.inTransitAt }

// UndoneAt returns when "Desfazer envio" ran, or the zero time.
func (o *SupplierOrder) UndoneAt() time.Time { return o.undoneAt }

// UndoneBy returns who undid it, or the zero id.
func (o *SupplierOrder) UndoneBy() shared.UserID { return o.undoneBy }

// Version returns the version for optimistic locking; a new order is 1.
func (o *SupplierOrder) Version() int { return o.version }

// CheckVersion returns shared.ErrVersionConflict unless expected is the
// order's version.
func (o *SupplierOrder) CheckVersion(expected int) error {
	if o.version != expected {
		return shared.ErrVersionConflict
	}
	return nil
}

// Lines returns the lines in the order they were picked. The result is a
// copy.
func (o *SupplierOrder) Lines() []Line {
	out := make([]Line, len(o.lines))
	for i, l := range o.lines {
		l.item = l.item.clone()
		out[i] = l
	}
	return out
}

// Line returns the line with that id, and false when the order has none.
func (o *SupplierOrder) Line(id shared.PurchaseLineID) (Line, bool) {
	i := o.indexOf(id)
	if i < 0 {
		return Line{}, false
	}
	l := o.lines[i]
	l.item = l.item.clone()
	return l, true
}

// PromisedFor returns the day the supplier promised line l: its own date,
// or the order's when it has none; the zero date when neither was recorded.
func (o *SupplierOrder) PromisedFor(l Line) shared.CivilDate {
	if !l.promised.IsZero() {
		return l.promised
	}
	return o.promised
}

func (o *SupplierOrder) indexOf(id shared.PurchaseLineID) int {
	return slices.IndexFunc(o.lines, func(l Line) bool { return l.id == id })
}

func (o *SupplierOrder) undone() bool { return !o.undoneAt.IsZero() }

// State returns where the order stands: undone after "Desfazer envio",
// sent while a line is still sent, in transit once every open line is,
// and received when no line is open.
func (o *SupplierOrder) State() State {
	if o.undone() {
		return StateUndone
	}
	open, sent := 0, 0
	for _, l := range o.lines {
		if l.status.Open() {
			open++
		}
		if l.status == LineSent {
			sent++
		}
	}
	switch {
	case open == 0:
		return StateReceived
	case sent > 0:
		return StateSent
	default:
		return StateInTransit
	}
}

// CanUndo reports whether "Desfazer envio" is allowed: the order is not
// undone and no line was received or found defective.
func (o *SupplierOrder) CanUndo() bool {
	if o.undone() {
		return false
	}
	return !slices.ContainsFunc(o.lines, func(l Line) bool {
		return l.status == LineReceived || l.status == LineDefective
	})
}

// Undo cancels every open line ("undone") and returns their order items,
// to go back to to_buy; an item removed from its order is left out. It
// returns ErrUndone when already undone and ErrSupplierOrderReceived once a
// line was received or found defective. A line already cancelled stays as
// it was.
func (o *SupplierOrder) Undo(by shared.Principal, now time.Time) ([]shared.OrderItemID, error) {
	if o.undone() {
		return nil, ErrUndone
	}
	if !o.CanUndo() {
		return nil, ErrSupplierOrderReceived
	}
	if by.UserID().IsZero() || now.IsZero() {
		return nil, fmt.Errorf("undo supplier order %v: %w", o.id, errMissing)
	}
	var items []shared.OrderItemID
	for i := range o.lines {
		l := &o.lines[i]
		if !l.status.Open() {
			continue
		}
		l.cancel(CancelUndone)
		if !l.item.OrderItem.IsZero() {
			items = append(items, l.item.OrderItem)
		}
	}
	o.undoneAt, o.undoneBy = now, by.UserID()
	return items, nil
}

// MarkInTransit moves every sent line to in_transit and returns their
// order items (one removed from its order is left out). It returns
// ErrUndone for an undone order and ErrNothingOpen when no line is sent.
func (o *SupplierOrder) MarkInTransit(now time.Time) ([]shared.OrderItemID, error) {
	if o.undone() {
		return nil, ErrUndone
	}
	if now.IsZero() {
		return nil, fmt.Errorf("mark supplier order %v in transit: %w", o.id, errMissing)
	}
	var items []shared.OrderItemID
	moved := false
	for i := range o.lines {
		l := &o.lines[i]
		if l.status != LineSent {
			continue
		}
		l.status, moved = LineInTransit, true
		if !l.item.OrderItem.IsZero() {
			items = append(items, l.item.OrderItem)
		}
	}
	if !moved {
		return nil, ErrNothingOpen
	}
	o.inTransitAt = now
	return items, nil
}

// RecordPromisedDate sets the date the supplier promised the order, and the
// own date of each line in overrides, for open lines only; an open line
// without an override follows the order's date again, so overrides is the
// full set. It returns a ValidationError on "date" for a zero date, and
// ErrDateInPast for a day before today (on "date" or "lines[i].date", i
// being the line's place in the order); ErrLineNotFound for a line not on
// the order and ErrLineClosed for an override on a closed line. Nothing
// changes on error. It returns each open line's order item with its
// effective date; an item removed from its order is left out.
func (o *SupplierOrder) RecordPromisedDate(date shared.CivilDate, overrides map[shared.PurchaseLineID]shared.CivilDate, today shared.CivilDate) ([]ItemDate, error) {
	if today.IsZero() {
		return nil, fmt.Errorf("record promised date: today: %w", errMissing)
	}
	if err := o.checkDates(date, overrides, today); err != nil {
		return nil, err
	}
	o.promised = date
	var out []ItemDate
	for i := range o.lines {
		l := &o.lines[i]
		if !l.status.Open() {
			continue
		}
		l.promised = overrides[l.id]
		if !l.item.OrderItem.IsZero() {
			out = append(out, ItemDate{Order: l.item.Order, Item: l.item.OrderItem, Date: o.PromisedFor(*l)})
		}
	}
	return out, nil
}

func (o *SupplierOrder) checkDates(date shared.CivilDate, overrides map[shared.PurchaseLineID]shared.CivilDate, today shared.CivilDate) error {
	if date.IsZero() {
		return fieldError("date", msgDateRequired)
	}
	if date.Before(today) {
		return refuse(ErrDateInPast, "date", msgDateFromToday)
	}
	for id := range overrides {
		if o.indexOf(id) < 0 {
			return fmt.Errorf("promised date of line %v: %w", id, ErrLineNotFound)
		}
	}
	for i, l := range o.lines {
		d, ok := overrides[l.id]
		switch {
		case !ok:
		case !l.status.Open():
			return fmt.Errorf("promised date of line %v: %w", l.id, ErrLineClosed)
		case d.IsZero():
			return fieldError(fmt.Sprintf("lines[%d].date", i), msgDateRequired)
		case d.Before(today):
			return refuse(ErrDateInPast, fmt.Sprintf("lines[%d].date", i), msgDateFromToday)
		}
	}
	return nil
}

// RecordPaidPrices sets the unit price paid on the lines in prices. It
// returns ErrLineNotFound for a line not on the order, ErrLineClosed for a
// cancelled line, and a ValidationError on "lines[i].paid" for a negative
// or too large price (i being the line's place in the order). Nothing
// changes on error. It returns each priced line's order item with its
// price, in the order's line order, for the order's margin; an item removed
// from its order is left out.
func (o *SupplierOrder) RecordPaidPrices(prices map[shared.PurchaseLineID]shared.Money) ([]ItemPrice, error) {
	for id := range prices {
		if o.indexOf(id) < 0 {
			return nil, fmt.Errorf("price of line %v: %w", id, ErrLineNotFound)
		}
	}
	for i, l := range o.lines {
		p, ok := prices[l.id]
		if !ok {
			continue
		}
		if l.status == LineCancelled {
			return nil, fmt.Errorf("price of line %v: %w", l.id, ErrLineClosed)
		}
		if err := validateMoney(p, fmt.Sprintf("lines[%d].paid", i)); err != nil {
			return nil, err
		}
	}
	var out []ItemPrice
	for i := range o.lines {
		l := &o.lines[i]
		p, ok := prices[l.id]
		if !ok {
			continue
		}
		l.paid = &p
		if !l.item.OrderItem.IsZero() {
			out = append(out, ItemPrice{Order: l.item.Order, Item: l.item.OrderItem, Price: p})
		}
	}
	return out, nil
}

// Receive checks one open line, received whole, on day today. It returns
// ErrLineNotFound for a line not on the order, ErrLineClosed when it is not
// open, and a ValidationError on "check" for a check other than ok or
// defective and on "note" for a defect with no note or one over 500
// characters. A defect keeps its (trimmed) note; "Tudo certo" keeps none.
// It returns the line as it is now.
func (o *SupplierOrder) Receive(line shared.PurchaseLineID, check Check, note string, today shared.CivilDate) (Line, error) {
	i := o.indexOf(line)
	if i < 0 {
		return Line{}, fmt.Errorf("receive line %v: %w", line, ErrLineNotFound)
	}
	l := &o.lines[i]
	if !l.status.Open() {
		return Line{}, fmt.Errorf("receive line %v: %w", line, ErrLineClosed)
	}
	if today.IsZero() {
		return Line{}, fmt.Errorf("receive line %v: today: %w", line, errMissing)
	}
	kept, err := checkResult(check, note, "")
	if err != nil {
		return Line{}, err
	}
	l.receive(check, kept, today)
	return o.lineAt(i), nil
}

// checkResult validates the check and its note, and returns the note the
// line keeps. prefix is put before the field names.
func checkResult(check Check, note, prefix string) (string, error) {
	if !check.Valid() {
		return "", fieldError(prefix+"check", msgCheck)
	}
	return resolveNote(check, note, prefix+"note")
}

// Resolve cancels an open line that is a notice ("Marcar como resolvido"),
// with the reason "resolved". It returns ErrLineNotFound for a line not on
// the order and ErrLineClosed when it is not open. Whether the line really
// is a notice is for NoticeOf and the caller. It returns the line as it is
// now.
func (o *SupplierOrder) Resolve(line shared.PurchaseLineID) (Line, error) {
	i := o.indexOf(line)
	if i < 0 {
		return Line{}, fmt.Errorf("resolve line %v: %w", line, ErrLineNotFound)
	}
	l := &o.lines[i]
	if !l.status.Open() {
		return Line{}, fmt.Errorf("resolve line %v: %w", line, ErrLineClosed)
	}
	l.cancel(CancelResolved)
	return o.lineAt(i), nil
}

func (o *SupplierOrder) lineAt(i int) Line {
	l := o.lines[i]
	l.item = l.item.clone()
	return l
}

// Record is every field of a supplier order, as repositories store it.
type Record struct {
	Tenant                      shared.TenantID
	ID                          shared.SupplierOrderID
	Number                      Number
	Supplier                    shared.SupplierID
	SupplierName, SupplierPhone string
	SentAt                      time.Time
	SentBy                      shared.UserID
	Promised                    shared.CivilDate
	InTransitAt                 time.Time
	UndoneAt                    time.Time
	UndoneBy                    shared.UserID
	Lines                       []LineRecord
	Version                     int
}

// Record returns every field of the supplier order, for the repository to
// store.
func (o *SupplierOrder) Record() Record {
	lines := make([]LineRecord, len(o.lines))
	for i, l := range o.lines {
		lines[i] = l.Record()
	}
	return Record{
		Tenant: o.tenant, ID: o.id, Number: o.number, Supplier: o.supplier,
		SupplierName: o.supplierName, SupplierPhone: o.supplierPhone,
		SentAt: o.sentAt, SentBy: o.sentBy, Promised: o.promised,
		InTransitAt: o.inTransitAt, UndoneAt: o.undoneAt, UndoneBy: o.undoneBy,
		Lines: lines, Version: o.version,
	}
}

// Restore rebuilds a stored supplier order. It returns ErrInvalidRecord for
// a row that no rule could have written: a missing id, tenant, supplier or
// time, a number below 1, no line, a line of another supplier order or
// supplier, or a line that does not restore.
func Restore(r Record) (*SupplierOrder, error) {
	if r.Tenant.IsZero() || r.ID.IsZero() || r.Supplier.IsZero() || !r.Number.Valid() ||
		r.SentAt.IsZero() || r.SentBy.IsZero() || r.Version < 1 || len(r.Lines) == 0 {
		return nil, fmt.Errorf("supplier order %v: missing field: %w", r.ID, ErrInvalidRecord)
	}
	lines := make([]Line, len(r.Lines))
	for i, lr := range r.Lines {
		l, err := RestoreLine(lr)
		if err != nil {
			return nil, err
		}
		if l.supplierOrder != r.ID || l.supplier != r.Supplier {
			return nil, fmt.Errorf("supplier order %v: line %v belongs elsewhere: %w", r.ID, l.id, ErrInvalidRecord)
		}
		lines[i] = l
	}
	return &SupplierOrder{
		tenant: r.Tenant, id: r.ID, number: r.Number, supplier: r.Supplier,
		supplierName: r.SupplierName, supplierPhone: r.SupplierPhone,
		sentAt: r.SentAt, sentBy: r.SentBy, promised: r.Promised,
		inTransitAt: r.InTransitAt, undoneAt: r.UndoneAt, undoneBy: r.UndoneBy,
		lines: lines, version: r.Version,
	}, nil
}
