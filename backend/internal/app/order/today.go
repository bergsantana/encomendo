package order

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// Hoje's thresholds, in whole days of the store's calendar: an order needs
// the team once it has waited for the deposit for more than
// awaitingDepositDays, or been ready for more than readyDays
// (docs/product/design-document.md, Hoje).
const (
	awaitingDepositDays = 3
	readyDays           = 2
	dueSoonDays         = 7  // "Entregas nos próximos 7 dias": today through today + 7
	upcomingLimit       = 20 // "Próximas entregas"
	// activeCap bounds how many active orders Hoje and Financeiro read. The
	// repository returns the earliest deliveries first, so a store with
	// more active orders than this loses the latest ones from the counts.
	activeCap = 500
)

// NeedKind says why an order is in "Precisa de você".
type NeedKind string

// The kinds, in the order Today lists them: most urgent first.
const (
	NeedLate               NeedKind = "late"                  // an item is "Atrasada"
	NeedAtRisk             NeedKind = "at_risk"               // an item "Pode atrasar"
	NeedAwaitingDeposit    NeedKind = "awaiting_deposit"      // waiting for the deposit for more than 3 days
	NeedReadyNotHandedOver NeedKind = "ready_not_handed_over" // ready for more than 2 days
	NeedRefundDue          NeedKind = "refund_due"            // money to give back
)

var needRank = map[NeedKind]int{
	NeedLate: 0, NeedAtRisk: 1, NeedAwaitingDeposit: 2, NeedReadyNotHandedOver: 3, NeedRefundDue: 4,
}

// NeedView is one entry of "Precisa de você".
type NeedView struct {
	Kind  NeedKind
	Order SummaryView
	// Item names the item with late and at_risk.
	Item string
	// Days is the days an item is late (late), the days its expected date
	// is past the arrival limit (at_risk), the days in the status
	// (awaiting_deposit, ready_not_handed_over), and 0 for refund_due.
	Days int
	// Amount is the amount to give back, with refund_due.
	Amount *shared.Money
}

// TodayCounts are the summary cards of Hoje.
type TodayCounts struct {
	Active          int // orders neither handed over nor cancelled
	AwaitingDeposit int // in the awaiting_deposit phase
	WithAlert       int // with at least one alert today
	DueIn7Days      int // delivery date from today through today + 7
}

// TodayView is the Hoje screen. It carries no cost or margin.
type TodayView struct {
	Today    shared.CivilDate // the store's day
	Counts   TodayCounts
	NeedsYou []NeedView
	Upcoming []SummaryView // by delivery date, at most upcomingLimit
	// Truncated says the store has more active orders than Hoje reads
	// (activeCap): counts and needs cover only the earliest deliveries.
	Truncated bool
}

// need is an entry of "Precisa de você" before its order is summarized.
type need struct {
	kind   NeedKind
	order  *order.Order
	item   string
	days   int
	amount *shared.Money
}

// Today builds Hoje from the store's active orders, for the store's day
// (order:read). Orders are read once, never cached.
func (s *Service) Today(ctx context.Context) (TodayView, error) {
	var out TodayView
	err := s.read(ctx, identity.PermOrderRead, func(ctx context.Context, r readCtx) error {
		rows, truncated, err := s.active(ctx, r)
		if err != nil {
			return err
		}
		recs := make([]order.Record, len(rows))
		for i, row := range rows {
			recs[i] = row.Record
		}
		orders, err := s.restoreAll(ctx, recs)
		if err != nil {
			return err
		}
		out = TodayView{Today: r.today, Truncated: truncated}
		var needs []need
		for i, o := range orders {
			needs = append(needs, countAndNeeds(&out.Counts, o, rows[i].StatusSince, r)...)
		}
		return s.summarizeToday(ctx, &out, orders, needs, r.today)
	})
	return out, err
}

// active reads up to activeCap active orders, earliest delivery first, and
// reports whether the store has more: one row past the cap is read to know.
func (s *Service) active(ctx context.Context, r readCtx) ([]ActiveOrder, bool, error) {
	rows, err := s.d.Repo.Active(ctx, r.by.TenantID(), activeCap+1)
	if err != nil {
		return nil, false, fmt.Errorf("active orders: %w", err)
	}
	if len(rows) > activeCap {
		return rows[:activeCap], true, nil
	}
	return rows, false, nil
}

// countAndNeeds adds o to the counts and returns what it needs from the team.
func countAndNeeds(c *TodayCounts, o *order.Order, since time.Time, r readCtx) []need {
	c.Active++
	if o.Phase() == workflow.PhaseAwaitingDeposit {
		c.AwaitingDeposit++
	}
	if d := r.today.DaysUntil(o.Promise()); d >= 0 && d <= dueSoonDays {
		c.DueIn7Days++
	}
	if len(o.Alerts(r.today)) > 0 {
		c.WithAlert++
	}
	return needsOf(o, since, r)
}

// needsOf returns why o needs the team today.
func needsOf(o *order.Order, since time.Time, r readCtx) []need {
	out := alertNeeds(o, r.today)
	inStatus := shared.DateOf(since, r.facts.Location).DaysUntil(r.today)
	switch o.Phase() {
	case workflow.PhaseAwaitingDeposit:
		if inStatus > awaitingDepositDays {
			out = append(out, need{kind: NeedAwaitingDeposit, order: o, days: inStatus})
		}
	case workflow.PhaseReady:
		if inStatus > readyDays {
			out = append(out, need{kind: NeedReadyNotHandedOver, order: o, days: inStatus})
		}
	}
	if refund := o.RefundDue(); !refund.IsZero() {
		out = append(out, need{kind: NeedRefundDue, order: o, amount: &refund})
	}
	return out
}

// alertNeeds returns one need per alerting item. An item both late and at
// risk is listed once, as late, the more urgent of the two.
func alertNeeds(o *order.Order, today shared.CivilDate) []need {
	alerts := o.Alerts(today)
	var out []need
	for _, i := range o.Items() {
		switch {
		case slices.Contains(alerts[i.ID()], schedule.AlertLate):
			days := i.Expected().DaysUntil(today)
			out = append(out, need{kind: NeedLate, order: o, item: i.Snapshot().Name, days: days})
		case slices.Contains(alerts[i.ID()], schedule.AlertAtRisk):
			days := o.ArrivalLimit().DaysUntil(i.Expected())
			out = append(out, need{kind: NeedAtRisk, order: o, item: i.Snapshot().Name, days: days})
		}
	}
	return out
}

// summarizeToday fills the needs, most urgent first, and the upcoming
// deliveries.
func (s *Service) summarizeToday(ctx context.Context, out *TodayView, orders []*order.Order, needs []need, today shared.CivilDate) error {
	// Stable: within a kind and a number of days the repository's order
	// (delivery date, id) stays.
	slices.SortStableFunc(needs, func(a, b need) int {
		return cmp.Or(cmp.Compare(needRank[a.kind], needRank[b.kind]), cmp.Compare(b.days, a.days))
	})
	names := newCustomerNames(s.d.Customers)
	for _, n := range needs {
		sum, err := names.summary(ctx, n.order, today)
		if err != nil {
			return err
		}
		out.NeedsYou = append(out.NeedsYou, NeedView{Kind: n.kind, Order: sum, Item: n.item, Days: n.days, Amount: n.amount})
	}
	upcoming := slices.Clone(orders)
	slices.SortStableFunc(upcoming, func(a, b *order.Order) int {
		return cmp.Or(b.Promise().DaysUntil(a.Promise()), cmp.Compare(a.Number(), b.Number()))
	})
	for _, o := range upcoming[:min(upcomingLimit, len(upcoming))] {
		sum, err := names.summary(ctx, o, today)
		if err != nil {
			return err
		}
		out.Upcoming = append(out.Upcoming, sum)
	}
	return nil
}
