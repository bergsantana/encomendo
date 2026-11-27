package catalog

import (
	"context"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/supplier"
)

// OrderLineRequest is one line of an order as the client or the seller
// built it: a kit (Kit set, with its swaps and the options of its items) or
// a loose item (Item and Quantity).
type OrderLineRequest struct {
	Kit      shared.KitID  // zero for a loose item
	Item     shared.ItemID // the loose item; zero for a kit
	Quantity int           // loose items only: 1..99
	Swaps    []catalog.Swap
	Options  []OptionChoice // for the loose item, or for items of the kit
}

// OptionChoice is the choice made for one option of one catalog item.
type OptionChoice struct {
	Item   shared.ItemID
	Option shared.OptionID
	Choice shared.ChoiceID
}

// ChosenOption is an option choice resolved against the catalog: names and
// extras as they are now, for the order's snapshot.
type ChosenOption struct {
	Option     shared.OptionID
	Choice     shared.ChoiceID
	Name       string
	Label      string
	ExtraPrice shared.Money
	ExtraDays  int
}

// SupplierFact is one of an item's suppliers at order time: staff data.
type SupplierFact struct {
	ID       shared.SupplierID
	Name     string
	LeadDays int
	Cost     *shared.Money // per unit; nil when none is recorded
}

// OrderItem is one item of a resolved line: what the client may see, and,
// apart, the staff-only sourcing (suppliers and cost).
type OrderItem struct {
	Item         shared.ItemID
	Name         string
	SourcingMode catalog.SourcingMode
	UnitPrice    shared.Money // single-item price, options excluded
	Quantity     int
	ReadyDays    int
	Options      []ChosenOption

	MainSupplier   *SupplierFact // nil for in-house items
	BackupSupplier *SupplierFact
	Cost           *shared.Money // the in-house cost or the main supplier's cost; nil when unknown
}

// OrderKit is the kit a line was sold as, with its swaps named.
type OrderKit struct {
	ID    shared.KitID
	Name  string
	Swaps []OrderSwap
}

// OrderSwap is one swap of a kit line, with both items' names.
type OrderSwap struct {
	Removed, Added         shared.ItemID
	RemovedName, AddedName string
}

// OrderLine is a resolved line: its price, options included, and its items.
type OrderLine struct {
	Kit                            *OrderKit
	Price                          shared.Money
	Items                          []OrderItem
	AssemblyDays, SafetyMarginDays *int // the kit's own overrides
}

const maxLineQuantity = 99

// OrderLines resolves an order's lines against the catalog: each line's
// price (a kit by PriceWithSwaps in its own swap mode, plus the options'
// extras times the kit item's quantity; a loose item by (price + extras) ×
// quantity), and each item's snapshot and sourcing. It needs order:write,
// and reads costs whatever the caller's role: the order views decide who
// sees them (identity.SeesCosts). It is never cached, since it carries costs.
//
// It returns catalog.ErrItemNotFound or catalog.ErrKitNotFound for an
// unknown item or kit, an error matching catalog.ErrItemArchived for an
// archived item or kit, the kit swap errors of catalog.Kit.PriceWithSwaps,
// and a *shared.ValidationError for a bad quantity, a missing required
// option or a choice that is not its option's.
func (s *Service) OrderLines(ctx context.Context, reqs []OrderLineRequest) ([]OrderLine, error) {
	by, err := authz.Require(ctx, identity.PermOrderWrite)
	if err != nil {
		return nil, err
	}
	var out []OrderLine
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		out = make([]OrderLine, 0, len(reqs))
		for i, r := range reqs {
			line, err := s.orderLine(ctx, by.TenantID(), fmt.Sprintf("lines[%d]", i), r)
			if err != nil {
				return err
			}
			out = append(out, line)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) orderLine(ctx context.Context, tenant shared.TenantID, path string, r OrderLineRequest) (OrderLine, error) {
	switch {
	case !r.Kit.IsZero() && r.Item.IsZero():
		return s.kitLine(ctx, tenant, path, r)
	case r.Kit.IsZero() && !r.Item.IsZero():
		return s.looseLine(ctx, tenant, path, r)
	}
	return OrderLine{}, fieldErr(path, msgLineKitOrItem)
}

func (s *Service) looseLine(ctx context.Context, tenant shared.TenantID, path string, r OrderLineRequest) (OrderLine, error) {
	if r.Quantity < 1 || r.Quantity > maxLineQuantity || len(r.Swaps) > 0 {
		return OrderLine{}, fieldErr(path+".quantity", msgLineQuantity)
	}
	item, err := s.orderItem(ctx, tenant, path, r.Item, r.Quantity, r.Options)
	if err != nil {
		return OrderLine{}, err
	}
	unit, err := withExtras(item.UnitPrice, item.Options)
	if err != nil {
		return OrderLine{}, err
	}
	price, err := times(unit, r.Quantity)
	if err != nil {
		return OrderLine{}, err
	}
	return OrderLine{Price: price, Items: []OrderItem{item}}, nil
}

func (s *Service) kitLine(ctx context.Context, tenant shared.TenantID, path string, r OrderLineRequest) (OrderLine, error) {
	k, err := s.kits.ByID(ctx, tenant, r.Kit)
	if err != nil {
		return OrderLine{}, fmt.Errorf("kit %s: %w", r.Kit, err)
	}
	if k.Archived() {
		return OrderLine{}, fmt.Errorf("kit %s: %w", r.Kit, catalog.ErrItemArchived)
	}
	facts, err := s.kitFacts(ctx, tenant, k.Items())
	if err != nil {
		return OrderLine{}, err
	}
	price, err := k.PriceWithSwaps(domainLookup(facts), r.Swaps)
	if err != nil {
		return OrderLine{}, err
	}
	line := OrderLine{Kit: &OrderKit{ID: k.ID(), Name: k.Name()}}
	if d, ok := k.AssemblyDays(); ok {
		line.AssemblyDays = &d
	}
	if d, ok := k.SafetyMarginDays(); ok {
		line.SafetyMarginDays = &d
	}
	for j, ki := range k.Items() {
		id := swappedTo(ki.Item, r.Swaps)
		if id != ki.Item {
			line.Kit.Swaps = append(line.Kit.Swaps, OrderSwap{
				Removed: ki.Item, Added: id, RemovedName: facts[ki.Item].Name, AddedName: facts[id].Name,
			})
		}
		item, err := s.orderItem(ctx, tenant, fmt.Sprintf("%s.items[%d]", path, j), id, ki.Quantity, r.Options)
		if err != nil {
			return OrderLine{}, err
		}
		if price, err = addExtras(price, item.Options, ki.Quantity); err != nil {
			return OrderLine{}, err
		}
		line.Items = append(line.Items, item)
	}
	if err := unknownChoices(path, r.Options, line.Items); err != nil {
		return OrderLine{}, err
	}
	line.Price = price
	return line, nil
}

// orderItem loads one item for an order: archived items are refused, and
// the options chosen for it are resolved.
func (s *Service) orderItem(ctx context.Context, tenant shared.TenantID, path string, id shared.ItemID, qty int, choices []OptionChoice) (OrderItem, error) {
	it, err := s.items.ByID(ctx, tenant, id)
	if err != nil {
		return OrderItem{}, fmt.Errorf("item %s: %w", id, err)
	}
	if it.Archived() {
		return OrderItem{}, &catalog.ArchivedItemsError{Items: []shared.ItemID{id}}
	}
	options, err := chosenOptions(path, it, choices)
	if err != nil {
		return OrderItem{}, err
	}
	out := OrderItem{
		Item: id, Name: it.Name(), SourcingMode: it.SourcingMode(), UnitPrice: it.Price(),
		Quantity: qty, ReadyDays: it.ReadyDays(), Options: options, Cost: it.Cost(),
	}
	if err := s.supplierFacts(ctx, it, &out); err != nil {
		return OrderItem{}, err
	}
	return out, nil
}

// supplierFacts fills the item's main and backup suppliers with their
// names. A supplier the directory does not know keeps an empty name.
func (s *Service) supplierFacts(ctx context.Context, it *catalog.Item, out *OrderItem) error {
	main, hasMain := it.MainSupplier()
	backup, hasBackup := it.BackupSupplier()
	if !hasMain && !hasBackup {
		return nil
	}
	ids := []shared.SupplierID{main.SupplierID(), backup.SupplierID()}
	names, err := s.suppliers.Names(ctx, ids)
	if err != nil {
		return fmt.Errorf("supplier names: %w", err)
	}
	fact := func(l supplier.SupplierItem) *SupplierFact {
		return &SupplierFact{ID: l.SupplierID(), Name: names[l.SupplierID()], LeadDays: l.LeadDays(), Cost: l.Cost()}
	}
	if hasMain {
		out.MainSupplier = fact(main)
	}
	if hasBackup {
		out.BackupSupplier = fact(backup)
	}
	return nil
}

// chosenOptions resolves the choices made for item it's options: a required
// option needs one, a choice must be one of its option's, and an option
// gets at most one.
func chosenOptions(path string, it *catalog.Item, choices []OptionChoice) ([]ChosenOption, error) {
	var out []ChosenOption
	var errs []error
	for k, o := range it.Options() {
		picked := choicesFor(choices, it.ID(), o.ID())
		field := fmt.Sprintf("%s.options[%d]", path, k)
		switch {
		case len(picked) > 1:
			errs = append(errs, fieldErr(field, fmt.Sprintf(msgOptionOneChoice, o.Name())))
			continue
		case len(picked) == 0 && o.Required():
			errs = append(errs, fieldErr(field, fmt.Sprintf(msgOptionRequired, o.Name())))
			continue
		case len(picked) == 0:
			continue
		}
		c, ok := findChoice(o, picked[0])
		if !ok {
			errs = append(errs, fieldErr(field, fmt.Sprintf(msgOptionRequired, o.Name())))
			continue
		}
		out = append(out, ChosenOption{
			Option: o.ID(), Choice: c.ID(), Name: o.Name(), Label: c.Label(), ExtraPrice: c.ExtraPrice(), ExtraDays: c.ExtraDays(),
		})
	}
	return out, shared.MergeValidation(errs...)
}

func choicesFor(choices []OptionChoice, item shared.ItemID, option shared.OptionID) []shared.ChoiceID {
	var out []shared.ChoiceID
	for _, c := range choices {
		if c.Item == item && c.Option == option {
			out = append(out, c.Choice)
		}
	}
	return out
}

func findChoice(o catalog.CustomizationOption, id shared.ChoiceID) (catalog.Choice, bool) {
	for _, c := range o.Choices() {
		if c.ID() == id {
			return c, true
		}
	}
	return catalog.Choice{}, false
}

// unknownChoices refuses a choice for an item or option the line does not
// have, so nothing the client sent is silently dropped.
func unknownChoices(path string, choices []OptionChoice, items []OrderItem) error {
	for i, c := range choices {
		if !resolved(c, items) {
			return fieldErr(fmt.Sprintf("%s.options[%d]", path, i), msgOptionUnknown)
		}
	}
	return nil
}

func resolved(c OptionChoice, items []OrderItem) bool {
	for _, it := range items {
		if it.Item != c.Item {
			continue
		}
		for _, o := range it.Options {
			if o.Option == c.Option && o.Choice == c.Choice {
				return true
			}
		}
	}
	return false
}

// swappedTo returns the item that takes id's place after swaps, or id.
func swappedTo(id shared.ItemID, swaps []catalog.Swap) shared.ItemID {
	for _, sw := range swaps {
		if sw.Item == id {
			return sw.With
		}
	}
	return id
}

func domainLookup(facts map[shared.ItemID]ItemFacts) catalog.ItemLookup {
	out := make(catalog.ItemLookup, len(facts))
	for id, f := range facts {
		out[id] = f.Facts
	}
	return out
}

func withExtras(unit shared.Money, options []ChosenOption) (shared.Money, error) {
	return addExtras(unit, options, 1)
}

// addExtras returns m plus every option's extra price times qty.
func addExtras(m shared.Money, options []ChosenOption, qty int) (shared.Money, error) {
	for _, o := range options {
		extra, err := times(o.ExtraPrice, qty)
		if err != nil {
			return shared.Money{}, err
		}
		if m, err = m.Add(extra); err != nil {
			return shared.Money{}, err
		}
	}
	return m, nil
}

// times returns m × n. Catalog amounts are bounded (R$ 1.000.000.000,00) and
// n by 99, so the product fits; NewMoney still refuses a bad currency.
func times(m shared.Money, n int) (shared.Money, error) {
	return shared.NewMoney(m.Cents()*int64(n), m.Currency())
}

func fieldErr(field, message string) error {
	return shared.NewValidationError(shared.FieldError{Field: field, Message: message})
}

// pt-BR messages, from docs/product/design-document.md (Novo pedido).
const (
	msgLineKitOrItem   = "Escolha um kit ou um item."
	msgLineQuantity    = "Use uma quantidade de 1 a 99."
	msgOptionRequired  = "Escolha %s."
	msgOptionOneChoice = "Escolha só uma opção em %s."
	msgOptionUnknown   = "Esta opção não é de um item da linha."
)
