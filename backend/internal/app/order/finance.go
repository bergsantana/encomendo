package order

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// maxPeriodDays is the longest Financeiro period, both ends included
// (OpenAPI PeriodTo).
const maxPeriodDays = 366

// HandedOverCap is the most hand-overs the finance summary reads for one
// period; the repository stops there.
const HandedOverCap = 2000

// MoneyByKey is an amount received under one payment kind or method.
type MoneyByKey struct {
	Key   string // a PaymentKind or a PaymentMethod
	Total shared.Money
}

// ReceivedView is the money recorded in the period, voided payments left out.
type ReceivedView struct {
	Total    shared.Money
	ByKind   []MoneyByKey // by key; only kinds with payments
	ByMethod []MoneyByKey // by key; only methods with payments
}

// ToReceiveView is the balance still due on active orders, whatever the
// period.
type ToReceiveView struct {
	Total  shared.Money
	Orders []SummaryView // those with a balance, by delivery date
}

// RefundView is an order with "Valor a devolver".
type RefundView struct {
	Order  SummaryView
	Amount shared.Money
}

// MarginRow is an order handed over in the period. Cost and Margin are nil
// when an item's cost was unknown. Only the owner reads Finance, so the
// costs here follow identity.SeesCosts like every other view.
type MarginRow struct {
	Order        shared.OrderID
	Number       string
	CustomerName string
	HandedOverOn shared.CivilDate // in the store's time zone
	Total        shared.Money
	Cost         *shared.Money
	Margin       *catalog.Margin
}

// FinanceView is Financeiro for one period. Owner only.
type FinanceView struct {
	From, To     shared.CivilDate
	Received     ReceivedView
	ToReceive    ToReceiveView
	ToRefund     []RefundView
	Margins      []MarginRow
	MarginsTotal MarginsTotal
	// Truncated says a list hit its read cap (activeCap active orders,
	// HandedOverCap hand-overs): "A receber", "A devolver" or the margins
	// cover only part of the store, and the screen says so.
	Truncated bool
}

// MarginsTotal is the period's total of "Margem dos pedidos entregues",
// computed here so clients never add money up. Cost and Margin count only
// the orders whose every cost was known; Margin is nil when none was.
type MarginsTotal struct {
	Total  shared.Money // every order handed over in the period
	Cost   shared.Money // orders with a known cost
	Margin *catalog.Margin
}

// marginsTotal sums rows. The margin's basis points are over what those
// orders sold (cost + margin), as each row's are.
func marginsTotal(rows []MarginRow, currency string) (MarginsTotal, error) {
	zero, err := shared.ZeroMoney(currency)
	if err != nil {
		return MarginsTotal{}, err
	}
	out := MarginsTotal{Total: zero, Cost: zero}
	var margin, sold int64
	known := false
	for _, r := range rows {
		if out.Total, err = out.Total.Add(r.Total); err != nil {
			return MarginsTotal{}, err
		}
		if r.Cost == nil || r.Margin == nil {
			continue
		}
		if out.Cost, err = out.Cost.Add(*r.Cost); err != nil {
			return MarginsTotal{}, err
		}
		known = true
		margin += r.Margin.Cents
		sold += r.Cost.Cents() + r.Margin.Cents
	}
	if known && sold > 0 {
		out.Margin = &catalog.Margin{Cents: margin, BasisPoints: margin * 10000 / sold}
	}
	return out, nil
}

// checkPeriod returns a ValidationError unless from..to is a period of at
// most maxPeriodDays days, both included.
func checkPeriod(from, to shared.CivilDate) error {
	var bad []shared.FieldError
	if from.IsZero() {
		bad = append(bad, shared.FieldError{Field: "from", Message: "Escolha o primeiro dia do período."})
	}
	if to.IsZero() {
		bad = append(bad, shared.FieldError{Field: "to", Message: "Escolha o último dia do período."})
	}
	if len(bad) == 0 {
		switch days := from.DaysUntil(to) + 1; {
		case days < 1:
			bad = append(bad, shared.FieldError{Field: "from", Message: "O primeiro dia não pode ser depois do último."})
		case days > maxPeriodDays:
			bad = append(bad, shared.FieldError{Field: "to", Message: "Escolha um período de até 366 dias."})
		}
	}
	if len(bad) > 0 {
		return shared.NewValidationError(bad...)
	}
	return nil
}

// dayStart returns the instant the civil day d begins in loc.
func dayStart(d shared.CivilDate, loc *time.Location) (time.Time, error) {
	t, err := time.ParseInLocation(time.DateOnly, d.String(), loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("start of %s: %w", d, err)
	}
	return t, nil
}

// Finance returns the owner's money summary for from..to, both days
// included, in the store's time zone (finance:read, the owner's). It refuses
// a missing day, from after to, and more than 366 days with a
// shared.ValidationError.
func (s *Service) Finance(ctx context.Context, from, to shared.CivilDate) (FinanceView, error) {
	var out FinanceView
	err := s.read(ctx, identity.PermFinanceRead, func(ctx context.Context, r readCtx) error {
		if err := checkPeriod(from, to); err != nil {
			return err
		}
		start, err := dayStart(from, r.facts.Location)
		if err != nil {
			return err
		}
		end, err := dayStart(to.AddDays(1), r.facts.Location)
		if err != nil {
			return err
		}
		out = FinanceView{From: from, To: to}
		names := newCustomerNames(s.d.Customers)
		if out.Received, err = s.received(ctx, r, start, end); err != nil {
			return err
		}
		if err = s.owed(ctx, &out, names, r); err != nil {
			return err
		}
		if out.Margins, err = s.margins(ctx, names, r, start, end); err != nil {
			return err
		}
		out.Truncated = out.Truncated || len(out.Margins) >= HandedOverCap
		out.MarginsTotal, err = marginsTotal(out.Margins, r.facts.Currency)
		return err
	})
	return out, err
}

// received sums the payments recorded in [start, end) that were not voided.
func (s *Service) received(ctx context.Context, r readCtx, start, end time.Time) (ReceivedView, error) {
	rows, err := s.d.Repo.Payments(ctx, r.by.TenantID(), start, end)
	if err != nil {
		return ReceivedView{}, fmt.Errorf("payments: %w", err)
	}
	zero, err := shared.ZeroMoney(r.facts.Currency)
	if err != nil {
		return ReceivedView{}, fmt.Errorf("store currency: %w", err)
	}
	out := ReceivedView{Total: zero}
	byKind, byMethod := map[string]shared.Money{}, map[string]shared.Money{}
	for _, p := range rows {
		if p.Voided {
			continue
		}
		if out.Total, err = out.Total.Add(p.Amount); err != nil {
			return ReceivedView{}, fmt.Errorf("sum payments: %w", err)
		}
		if err := addTo(byKind, string(p.Kind), p.Amount, zero); err != nil {
			return ReceivedView{}, err
		}
		if err := addTo(byMethod, string(p.Method), p.Amount, zero); err != nil {
			return ReceivedView{}, err
		}
	}
	out.ByKind, out.ByMethod = sortedMoney(byKind), sortedMoney(byMethod)
	return out, nil
}

func addTo(m map[string]shared.Money, key string, amount, zero shared.Money) error {
	cur, ok := m[key]
	if !ok {
		cur = zero
	}
	sum, err := cur.Add(amount)
	if err != nil {
		return fmt.Errorf("sum %s payments: %w", key, err)
	}
	m[key] = sum
	return nil
}

func sortedMoney(m map[string]shared.Money) []MoneyByKey {
	out := make([]MoneyByKey, 0, len(m))
	for k, v := range m {
		out = append(out, MoneyByKey{Key: k, Total: v})
	}
	slices.SortFunc(out, func(a, b MoneyByKey) int { return cmp.Compare(a.Key, b.Key) })
	return out
}

// owed fills what is still to receive and to refund from the active orders.
func (s *Service) owed(ctx context.Context, out *FinanceView, names *customerNames, r readCtx) error {
	rows, truncated, err := s.active(ctx, r)
	if err != nil {
		return err
	}
	out.Truncated = truncated
	recs := make([]order.Record, len(rows))
	for i, row := range rows {
		recs[i] = row.Record
	}
	orders, err := s.restoreAll(ctx, recs)
	if err != nil {
		return err
	}
	if out.ToReceive.Total, err = shared.ZeroMoney(r.facts.Currency); err != nil {
		return fmt.Errorf("store currency: %w", err)
	}
	for _, o := range orders {
		if bal := o.BalanceDue(); !bal.IsZero() {
			if out.ToReceive.Total, err = out.ToReceive.Total.Add(bal); err != nil {
				return fmt.Errorf("sum balances: %w", err)
			}
			sum, err := names.summary(ctx, o, r.today)
			if err != nil {
				return err
			}
			out.ToReceive.Orders = append(out.ToReceive.Orders, sum)
		}
		if refund := o.RefundDue(); !refund.IsZero() {
			sum, err := names.summary(ctx, o, r.today)
			if err != nil {
				return err
			}
			out.ToRefund = append(out.ToRefund, RefundView{Order: sum, Amount: refund})
		}
	}
	return nil
}

// margins lists the orders handed over in [start, end) with their cost and
// margin where every item's cost was known.
func (s *Service) margins(ctx context.Context, names *customerNames, r readCtx, start, end time.Time) ([]MarginRow, error) {
	recs, err := s.d.Repo.HandedOver(ctx, r.by.TenantID(), start, end)
	if err != nil {
		return nil, fmt.Errorf("handed over orders: %w", err)
	}
	orders, err := s.restoreAll(ctx, recs)
	if err != nil {
		return nil, err
	}
	var out []MarginRow
	for _, o := range orders {
		name, err := names.of(ctx, o.Customer())
		if err != nil {
			return nil, err
		}
		_, at, _ := o.HandedOver() // HandedOver lists only orders handed over
		row := MarginRow{
			Order: o.ID(), Number: o.Number().String(), CustomerName: name,
			HandedOverOn: shared.DateOf(at, r.facts.Location), Total: total(o),
		}
		if identity.SeesCosts(r.by.Role()) {
			if cost, ok := realCost(o); ok {
				row.Cost = &cost
				if m, ok := marginAfter(o, cost); ok {
					row.Margin = &m
				}
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// realCost is the order's cost for a margin: each item's unit cost is the
// price actually paid (Sourcing.PaidCost) when purchasing recorded one, else
// the cost known at order time (Sourcing.Cost), times its quantity. It
// returns false when an item has neither, so a margin is never guessed.
// order.Order.Cost counts the order-time cost only, which is what a quote
// shows before anything is bought (ADR 0031).
func realCost(o *order.Order) (shared.Money, bool) {
	currency := o.Terms().Freight.Currency()
	var cents int64
	for _, it := range o.Items() {
		unit := it.Sourcing().PaidCost
		if unit == nil {
			unit = it.Sourcing().Cost
		}
		if unit == nil {
			return shared.Money{}, false
		}
		cents += unit.Cents() * int64(it.Snapshot().Quantity)
	}
	cost, err := shared.NewMoney(cents, currency)
	return cost, err == nil
}

// marginAfter is what the order leaves after cost, over what was sold: the
// total less the freight, which only passes through.
func marginAfter(o *order.Order, cost shared.Money) (catalog.Margin, bool) {
	sold, err := shared.NewMoney(total(o).Cents()-o.Terms().Freight.Cents(), cost.Currency())
	if err != nil {
		return catalog.Margin{}, false
	}
	return catalog.MarginOf(sold, cost)
}
