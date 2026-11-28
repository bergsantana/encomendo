package dto

import (
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Today is the Today schema: Hoje. It carries no cost or margin, for any
// role: its views have no field for either.
type Today struct {
	Truncated bool             `json:"truncated"`
	Today     shared.CivilDate `json:"today"`
	Counts    TodayCounts      `json:"counts"`
	NeedsYou  []Need           `json:"needsYou"`
	Upcoming  []OrderSummary   `json:"upcoming"`
}

// TodayCounts are the counts at the top of Hoje.
type TodayCounts struct {
	Active          int `json:"active"`
	AwaitingDeposit int `json:"awaitingDeposit"`
	WithAlert       int `json:"withAlert"`
	DueIn7Days      int `json:"dueIn7Days"`
}

// Need is one entry of "Precisa de você".
type Need struct {
	Kind     apporder.NeedKind `json:"kind"`
	Order    OrderSummary      `json:"order"`
	ItemName string            `json:"itemName,omitempty"`
	Days     int               `json:"days,omitempty"`
	Amount   *shared.Money     `json:"amount,omitempty"`
}

// NewToday maps Hoje.
func NewToday(v apporder.TodayView) Today {
	out := Today{
		Truncated: v.Truncated, Today: v.Today,
		Counts:   TodayCounts{Active: v.Counts.Active, AwaitingDeposit: v.Counts.AwaitingDeposit, WithAlert: v.Counts.WithAlert, DueIn7Days: v.Counts.DueIn7Days},
		NeedsYou: make([]Need, 0, len(v.NeedsYou)), Upcoming: make([]OrderSummary, 0, len(v.Upcoming)),
	}
	for _, n := range v.NeedsYou {
		out.NeedsYou = append(out.NeedsYou, Need{
			Kind: n.Kind, Order: NewOrderSummary(n.Order), ItemName: n.Item, Days: n.Days, Amount: n.Amount,
		})
	}
	for _, s := range v.Upcoming {
		out.Upcoming = append(out.Upcoming, NewOrderSummary(s))
	}
	return out
}
