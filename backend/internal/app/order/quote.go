package order

import (
	"context"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// QuoteView is what an order would be: prices, deposit and dates, all
// computed by the server, with nothing saved.
//
// RefundDue is what the changed order would owe back, zero for a new order.
// DaysAfterDeposit counts calendar days from the day the deadline counts
// from to DeliveryDate: today for a new order and for a change to an order
// not yet released, the order's release date for a change to a released one
// ("N dias depois do sinal"). Cost and Margin are the owner's: they are nil
// for every other role, and for the owner too when a cost is unknown.
type QuoteView struct {
	Lines                          []QuoteLine
	Subtotal, Freight, Discount    shared.Money
	Total, DepositDue, RefundDue   shared.Money
	DepositPercent                 int
	AssemblyDays, SafetyMarginDays int
	DeliveryDate, ArrivalLimit     shared.CivilDate
	DaysAfterDeposit               int
	Cost                           *shared.Money   // owner only
	Margin                         *catalog.Margin // owner only
}

// QuoteLine is one line of a quote.
type QuoteLine struct {
	Kit   *order.KitSnapshot
	Price shared.Money
	Items []QuoteItem
}

// QuoteItem is one item of a quote, with its expected date.
type QuoteItem struct {
	CatalogItem  shared.ItemID
	Name         string
	SourcingMode catalog.SourcingMode
	Quantity     int
	UnitPrice    shared.Money
	Options      []order.ChosenOption
	Expected     shared.CivilDate
}

// Quote answers what Create would make of in (order:write; terms other than
// the defaults need order:manage), or, with in.Order set, what Change would
// make of that order. Nothing is written. It returns the refusals Create or
// Change would.
func (s *Service) Quote(ctx context.Context, in QuoteInput) (QuoteView, error) {
	by, err := authz.Require(ctx, identity.PermOrderWrite)
	if err != nil {
		return QuoteView{}, err
	}
	var out QuoteView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		w, err := s.writeCtx(ctx, by)
		if err != nil {
			return err
		}
		w.preview = true
		var o *order.Order
		if in.Order.IsZero() {
			o, err = s.quoteNew(ctx, in, w)
		} else {
			o, err = s.quoteChange(ctx, in, w)
		}
		if err != nil {
			return err
		}
		out = quoteView(o, w)
		return nil
	})
	return out, err
}

func (s *Service) quoteNew(ctx context.Context, in QuoteInput, w writeCtx) (*order.Order, error) {
	wf, err := s.d.Workflows.Current(ctx)
	if err != nil {
		return nil, fmt.Errorf("current workflow: %w", err)
	}
	lines, err := s.newLines(ctx, in.Lines)
	if err != nil {
		return nil, err
	}
	// A quote has no customer yet: a delivery address only has to be named.
	c := CustomerFacts{ID: shared.CustomerID(s.d.IDs.NewID())}
	if !in.Terms.Address.IsZero() {
		c.Addresses = []order.DeliveryAddress{{ID: in.Terms.Address}}
	}
	in.Terms, c, err = s.withNewAddress(ctx, in.Terms, c, w)
	if err != nil {
		return nil, err
	}
	terms, err := s.newTerms(w.by, w.facts, lines, in.Terms, c)
	if err != nil {
		return nil, err
	}
	return order.New(order.NewSpec{
		Tenant: w.by.TenantID(), ID: shared.OrderID(s.d.IDs.NewID()), Number: 1, Customer: c.ID,
		Workflow: wf, Lines: lines, Terms: terms, Tracking: shared.SecretHash{0x9}, // never saved
		By: w.by, Now: w.now, Today: w.today,
	})
}

func (s *Service) quoteChange(ctx context.Context, in QuoteInput, w writeCtx) (*order.Order, error) {
	o, err := s.load(ctx, w.by.TenantID(), in.Order)
	if err != nil {
		return nil, err
	}
	if _, err := s.applyChange(ctx, o, ChangeInput{Lines: in.Lines, Terms: in.Terms, CustomerApproved: true}, w); err != nil {
		return nil, err
	}
	return o, nil
}

// quoteView maps the in-memory order o, as the call w would leave it. Cost
// and margin follow identity.SeesCosts, as on every order view.
func quoteView(o *order.Order, w writeCtx) QuoteView {
	t := o.Terms()
	v := QuoteView{
		Subtotal: o.Subtotal(), Freight: t.Freight, Discount: t.Discount, Total: total(o), DepositDue: o.DepositDue(),
		RefundDue: o.RefundDue(), DepositPercent: t.DepositPercent, AssemblyDays: o.AssemblyDaysCounted(),
		SafetyMarginDays: t.SafetyMarginDays, DeliveryDate: o.Promise(), ArrivalLimit: o.ArrivalLimit(),
	}
	from := w.today
	if released, ok := o.ReleaseDate(); ok {
		from = released
	}
	v.DaysAfterDeposit = from.DaysUntil(o.Promise())
	if identity.SeesCosts(w.by.Role()) {
		if cost, ok := o.Cost(); ok {
			v.Cost = &cost
		}
		if m, ok := o.Margin(); ok {
			v.Margin = &m
		}
	}
	for _, l := range o.Lines() {
		ql := QuoteLine{Price: l.Price()}
		if k, ok := l.Kit(); ok {
			ql.Kit = &k
		}
		for _, i := range l.Items() {
			snap := i.Snapshot()
			ql.Items = append(ql.Items, QuoteItem{
				CatalogItem: snap.CatalogItem, Name: snap.Name, SourcingMode: snap.SourcingMode,
				Quantity: snap.Quantity, UnitPrice: snap.UnitPrice, Options: snap.Options, Expected: i.Expected(),
			})
		}
		v.Lines = append(v.Lines, ql)
	}
	return v
}
