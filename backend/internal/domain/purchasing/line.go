package purchasing

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// maxNoteRunes is the longest defect note, in characters.
const maxNoteRunes = 500

// LineStatus is where a purchase line stands. The values are the
// contract's PurchaseLineStatus.
type LineStatus string

// The line statuses.
const (
	// LineSent: the supplier order was sent and nothing came back yet.
	LineSent LineStatus = "sent"
	// LineInTransit: the supplier said it is on its way.
	LineInTransit LineStatus = "in_transit"
	// LineReceived: it arrived whole and was checked "Tudo certo".
	LineReceived LineStatus = "received"
	// LineDefective: it arrived and was checked "Com defeito"; the note says why.
	LineDefective LineStatus = "defective"
	// LineCancelled: the line is no longer needed (see CancelReason).
	LineCancelled LineStatus = "cancelled"
)

// Open reports whether the line still waits for the supplier: sent or in
// transit. Only open lines are received, redated or become notices.
func (s LineStatus) Open() bool { return s == LineSent || s == LineInTransit }

// Valid reports whether s is one of the five statuses.
func (s LineStatus) Valid() bool {
	switch s {
	case LineSent, LineInTransit, LineReceived, LineDefective, LineCancelled:
		return true
	default:
		return false
	}
}

// CancelReason says why a line was cancelled.
type CancelReason string

// The reasons.
const (
	// CancelUndone: the whole supplier order was undone ("Desfazer envio").
	CancelUndone CancelReason = "undone"
	// CancelResolved: the line was a notice and the owner marked it resolved.
	CancelResolved CancelReason = "resolved"
)

// Check is the result of looking at what arrived. Its values equal
// order.Check's; the inproc adapter maps one to the other so the two
// domains do not import each other (ADR 0031).
type Check string

// The checks, worded "Tudo certo" and "Com defeito" on screen.
const (
	CheckOK        Check = "ok"
	CheckDefective Check = "defective"
)

// Valid reports whether c is ok or defective.
func (c Check) Valid() bool { return c == CheckOK || c == CheckDefective }

// Choice is an option choice as the order item was sold. Options are store
// choices, never free text (ADR 0024), so they carry no client data.
type Choice struct {
	Option   shared.OptionID
	ChoiceID shared.ChoiceID
	// Name is the option's name ("Cor") and Label the choice's ("Preto"),
	// as they were when the item was sold.
	Name, Label string
}

// Item is what a line buys, copied from the order item when it is sent. It
// has no field for the client's name, phone, address or order number, so no
// caller can put one in a supplier message.
type Item struct {
	Order       shared.OrderID
	OrderItem   shared.OrderItemID // zero once removed from the order
	CatalogItem shared.ItemID
	Name        string
	Quantity    int // 1..99
	Choices     []Choice
}

// validate checks an item for a new line. A stored line may have lost its
// order item, so only a new one needs it.
func (it Item) validate(field string, needOrderItem bool) error {
	if it.Order.IsZero() || it.CatalogItem.IsZero() || (needOrderItem && it.OrderItem.IsZero()) {
		return fmt.Errorf("item %q: %w", it.Name, errMissing)
	}
	if strings.TrimSpace(it.Name) == "" {
		return fieldError(field+".name", msgItemName)
	}
	if it.Quantity < 1 || it.Quantity > 99 {
		return fieldError(field+".quantity", msgQuantity)
	}
	return nil
}

func (it Item) clone() Item {
	it.Choices = slices.Clone(it.Choices)
	return it
}

// choiceSet returns the (option, choice) pairs of cs as a sorted, comparable
// key, so two item states with the same choices in any order are equal.
func choiceSet(cs []Choice) string {
	keys := make([]string, len(cs))
	for i, c := range cs {
		keys[i] = c.Option.String() + "/" + c.ChoiceID.String()
	}
	slices.Sort(keys)
	return strings.Join(slices.Compact(keys), ",")
}

// Line is one purchase line: one order item bought from one supplier. Its
// fields are unexported; a line changes only through its supplier order,
// or is born received at the counter (CounterLine).
type Line struct {
	id            shared.PurchaseLineID
	supplierOrder shared.SupplierOrderID // zero for a counter receipt
	supplier      shared.SupplierID
	item          Item
	status        LineStatus
	promised      shared.CivilDate // the line's own date; zero = the supplier order's
	cost, paid    *shared.Money    // unit; owner only
	checkNote     string
	cancelReason  CancelReason
	receivedOn    shared.CivilDate
}

// ID returns the line's id.
func (l Line) ID() shared.PurchaseLineID { return l.id }

// SupplierOrder returns the supplier order the line is on; zero for a line
// received at the counter.
func (l Line) SupplierOrder() shared.SupplierOrderID { return l.supplierOrder }

// Supplier returns the supplier the item was bought from.
func (l Line) Supplier() shared.SupplierID { return l.supplier }

// Item returns what the line buys, as it was when sent. The result is a
// copy.
func (l Line) Item() Item { return l.item.clone() }

// Status returns where the line stands.
func (l Line) Status() LineStatus { return l.status }

// Promised returns the line's own promised date, or the zero date when it
// follows the supplier order's.
func (l Line) Promised() shared.CivilDate { return l.promised }

// Cost returns the unit cost at order time, or nil when unknown. Only the
// owner sees it; the caller decides.
func (l Line) Cost() *shared.Money { return copyMoney(l.cost) }

// Paid returns the unit price actually paid, or nil when none was recorded.
// Only the owner sees it.
func (l Line) Paid() *shared.Money { return copyMoney(l.paid) }

// CheckNote returns the note of a defect, or "" for any other line.
func (l Line) CheckNote() string { return l.checkNote }

// CancelReason returns why a cancelled line was cancelled, or "".
func (l Line) CancelReason() CancelReason { return l.cancelReason }

// ReceivedOn returns the day a received or defective line arrived, or the
// zero date.
func (l Line) ReceivedOn() shared.CivilDate { return l.receivedOn }

func copyMoney(m *shared.Money) *shared.Money {
	if m == nil {
		return nil
	}
	c := *m
	return &c
}

// validateMoney checks a cost or a price: zero or more, up to the bound.
func validateMoney(m shared.Money, field string) error {
	switch {
	case m.IsNegative():
		return fieldError(field, msgNegative)
	case m.Cents() > maxAmountCents:
		return fieldError(field, msgAmountTooHigh)
	default:
		return nil
	}
}

// resolveNote returns the note a check keeps: a defect keeps the trimmed
// note, which must be 1 to 500 characters; "Tudo certo" keeps none. The
// error is a ValidationError on field.
func resolveNote(check Check, note, field string) (string, error) {
	if check == CheckOK {
		return "", nil
	}
	note = strings.TrimSpace(note)
	if n := utf8.RuneCountInString(note); n < 1 || n > maxNoteRunes {
		return "", fieldError(field, msgDefectNote)
	}
	return note, nil
}

// checkedStatus is the status a check leaves a line in.
func checkedStatus(c Check) LineStatus {
	if c == CheckDefective {
		return LineDefective
	}
	return LineReceived
}

// receive closes an open line with its check. The caller has validated the
// check and the note.
func (l *Line) receive(check Check, note string, today shared.CivilDate) {
	l.status = checkedStatus(check)
	l.checkNote = note
	l.receivedOn = today
}

// cancel closes an open line without it arriving.
func (l *Line) cancel(reason CancelReason) {
	l.status = LineCancelled
	l.cancelReason = reason
}

// LineRecord is every field of a line, as repositories store it.
type LineRecord struct {
	ID            shared.PurchaseLineID
	SupplierOrder shared.SupplierOrderID
	Supplier      shared.SupplierID
	Item          Item
	Status        LineStatus
	Promised      shared.CivilDate
	Cost, Paid    *shared.Money
	CheckNote     string
	CancelReason  CancelReason
	ReceivedOn    shared.CivilDate
}

// Record returns every field of the line, for the repository to store.
func (l Line) Record() LineRecord {
	return LineRecord{
		ID: l.id, SupplierOrder: l.supplierOrder, Supplier: l.supplier, Item: l.item.clone(),
		Status: l.status, Promised: l.promised, Cost: copyMoney(l.cost), Paid: copyMoney(l.paid),
		CheckNote: l.checkNote, CancelReason: l.cancelReason, ReceivedOn: l.receivedOn,
	}
}

// RestoreLine rebuilds a stored line. It returns ErrInvalidRecord for a
// row that no rule could have written: a missing id, an unknown status, a
// cancelled line with no reason, a received one with no day, a defect with
// no note.
func RestoreLine(r LineRecord) (Line, error) {
	bad := func(why string) (Line, error) {
		return Line{}, fmt.Errorf("line %v: %s: %w", r.ID, why, ErrInvalidRecord)
	}
	switch {
	case r.ID.IsZero() || r.Supplier.IsZero():
		return bad("missing id or supplier")
	case r.Item.validate("item", false) != nil:
		return bad("invalid item")
	case !r.Status.Valid():
		return bad("unknown status " + string(r.Status))
	case r.Status == LineCancelled && r.CancelReason == "":
		return bad("cancelled without a reason")
	case (r.Status == LineReceived || r.Status == LineDefective) && r.ReceivedOn.IsZero():
		return bad("received without a day")
	case r.Status == LineDefective && r.CheckNote == "":
		return bad("defective without a note")
	}
	return Line{
		id: r.ID, supplierOrder: r.SupplierOrder, supplier: r.Supplier, item: r.Item.clone(),
		status: r.Status, promised: r.Promised, cost: copyMoney(r.Cost), paid: copyMoney(r.Paid),
		checkNote: r.CheckNote, cancelReason: r.CancelReason, receivedOn: r.ReceivedOn,
	}, nil
}
