package order

import (
	"fmt"
	"slices"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// LineSpec is one line of a new or changed order, as the catalog priced it:
// a kit (Kit set) holding one item per kit item, or one loose item. The app
// layer fills the ids, reusing an item's id when it survives a change.
type LineSpec struct {
	ID    shared.OrderLineID
	Kit   *KitSnapshot // nil for a loose item
	Price shared.Money // the line's price, options included; the catalog computes it
	Items []ItemSpec   // a loose-item line has exactly one
	// AssemblyDays and SafetyMarginDays are the kit's own overrides, nil when
	// it has none. The app layer uses them to resolve Terms.
	AssemblyDays, SafetyMarginDays *int
}

// ItemSpec is one item of a line spec.
type ItemSpec struct {
	ID       shared.OrderItemID
	Snapshot ItemSnapshot
	Sourcing Sourcing
}

// Line is one line of an order: a kit sold at its kit price, or one loose
// item. Each of its items has its own status and date.
type Line struct {
	id                       shared.OrderLineID
	kit                      *KitSnapshot
	price                    shared.Money
	items                    []Item
	assemblyDays, marginDays *int
}

// ID returns the line's id.
func (l Line) ID() shared.OrderLineID { return l.id }

// Kit returns the kit the line was sold as, and false for a loose item.
func (l Line) Kit() (KitSnapshot, bool) {
	if l.kit == nil {
		return KitSnapshot{}, false
	}
	return cloneKit(*l.kit), true
}

// Price returns the line's price, options included.
func (l Line) Price() shared.Money { return l.price }

// Items returns the line's items, as a new slice.
func (l Line) Items() []Item { return slices.Clone(l.items) }

// KitDays returns the kit's own assembly and safety margin days, nil when
// it has none or the line is a loose item.
func (l Line) KitDays() (assembly, margin *int) {
	return clonePtr(l.assemblyDays), clonePtr(l.marginDays)
}

const (
	maxLines    = 50
	maxQuantity = 99
	maxLeadDays = 365
)

// buildLines checks specs and turns them into lines in currency. Items that
// keep their id from prev (a change) keep their status, check and arrival,
// when their sourcing mode is the same; every other item starts at its
// mode's initial status. Expected dates are set later, by redate.
func buildLines(specs []LineSpec, currency string, prev map[shared.OrderItemID]Item) ([]Line, error) {
	if len(specs) == 0 || len(specs) > maxLines {
		return nil, fieldError("lines", msgNoLines)
	}
	var errs []error
	seenLines := map[shared.OrderLineID]bool{}
	seenItems := map[shared.OrderItemID]bool{}
	lines := make([]Line, 0, len(specs))
	for i, s := range specs {
		if s.ID.IsZero() || seenLines[s.ID] {
			return nil, fmt.Errorf("line %d: missing or repeated id: %w", i, shared.ErrInvalid)
		}
		seenLines[s.ID] = true
		l, err := buildLine(fmt.Sprintf("lines[%d]", i), s, currency, prev, seenItems)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		lines = append(lines, l)
	}
	if err := shared.MergeValidation(errs...); err != nil {
		return nil, err
	}
	return lines, nil
}

func buildLine(path string, s LineSpec, currency string, prev map[shared.OrderItemID]Item, seen map[shared.OrderItemID]bool) (Line, error) {
	if len(s.Items) == 0 || (s.Kit == nil && len(s.Items) != 1) {
		return Line{}, fieldError(path, msgLineShape)
	}
	if s.Price.Currency() != currency || s.Price.IsNegative() {
		return Line{}, fieldError(path+".price", msgCurrency)
	}
	l := Line{id: s.ID, price: s.Price, assemblyDays: clonePtr(s.AssemblyDays), marginDays: clonePtr(s.SafetyMarginDays)}
	if s.Kit != nil {
		k := cloneKit(*s.Kit)
		l.kit = &k
	}
	var errs []error
	for j, is := range s.Items {
		if is.ID.IsZero() || seen[is.ID] {
			return Line{}, fmt.Errorf("%s item %d: missing or repeated id: %w", path, j, shared.ErrInvalid)
		}
		seen[is.ID] = true
		item, err := buildItem(fmt.Sprintf("%s.items[%d]", path, j), s.ID, is, currency)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		l.items = append(l.items, item.keepState(prev[is.ID]))
	}
	if err := shared.MergeValidation(errs...); err != nil {
		return Line{}, err
	}
	return l, nil
}

func buildItem(path string, line shared.OrderLineID, s ItemSpec, currency string) (Item, error) {
	snap := s.Snapshot
	var errs []error
	if strings.TrimSpace(snap.Name) == "" {
		errs = append(errs, fieldError(path+".name", msgItemName))
	}
	if !snap.SourcingMode.Valid() {
		errs = append(errs, fieldError(path+".sourcingMode", msgSourcingMode))
	}
	if snap.Quantity < 1 || snap.Quantity > maxQuantity {
		errs = append(errs, fieldError(path+".quantity", msgQuantity))
	}
	if snap.UnitPrice.Currency() != currency || snap.UnitPrice.IsNegative() {
		errs = append(errs, fieldError(path+".unitPrice", msgCurrency))
	}
	if snap.LeadDays < 0 || snap.LeadDays > maxLeadDays {
		errs = append(errs, fieldError(path+".leadDays", msgLeadDays))
	}
	for k, o := range snap.Options {
		if o.ExtraPrice.Currency() != currency || o.ExtraPrice.IsNegative() || o.ExtraDays < 0 || o.ExtraDays > maxLeadDays {
			errs = append(errs, fieldError(fmt.Sprintf("%s.options[%d]", path, k), msgCurrency))
		}
	}
	if err := shared.MergeValidation(errs...); err != nil {
		return Item{}, err
	}
	return Item{
		id: s.ID, line: line, snapshot: cloneSnapshot(snap), sourcing: cloneSourcing(s.Sourcing),
		status: InitialStatus(snap.SourcingMode),
	}, nil
}

func cloneKit(k KitSnapshot) KitSnapshot {
	k.Swaps = slices.Clone(k.Swaps)
	return k
}

func cloneSnapshot(s ItemSnapshot) ItemSnapshot {
	s.Options = slices.Clone(s.Options)
	return s
}

func cloneSourcing(s Sourcing) Sourcing {
	s.Cost = clonePtr(s.Cost)
	if s.Backup != nil {
		b := *s.Backup
		b.Cost = clonePtr(b.Cost)
		s.Backup = &b
	}
	return s
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
