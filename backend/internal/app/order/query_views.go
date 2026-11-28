package order

import (
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// CustomerBrief is a customer as a list line shows it: no phone.
type CustomerBrief struct {
	ID   shared.CustomerID
	Name string
}

// AlertView is one alert of an order's item: "Pode atrasar" (at_risk) or
// "Atrasada" (late). Staff data, never in a client view.
type AlertView struct {
	Kind schedule.Alert
	Item shared.OrderItemID
}

// SummaryView is one order as a list line shows it. It has no field for a
// cost or a margin, so no role can get one from a list, Hoje or the
// finance lists; it is built only by toSummary.
type SummaryView struct {
	ID           shared.OrderID
	Number       string
	Customer     CustomerBrief
	Status       StatusView
	ItemsReady   int // items that arrived
	ItemsTotal   int
	DeliveryDate shared.CivilDate
	Total        shared.Money
	BalanceDue   shared.Money
	Alerts       []AlertView // today's; none once handed over or cancelled
}

// toSummary builds o's list line. customerName is the customer's name.
func toSummary(o *order.Order, customerName string, today shared.CivilDate) SummaryView {
	v := SummaryView{
		ID: o.ID(), Number: o.Number().String(), Customer: CustomerBrief{ID: o.Customer(), Name: customerName},
		Status: statusView(o.Status()), DeliveryDate: o.Promise(), Total: total(o), BalanceDue: o.BalanceDue(),
	}
	alerts := o.Alerts(today)
	for _, i := range o.Items() {
		v.ItemsTotal++
		if i.Arrived() {
			v.ItemsReady++
		}
		for _, a := range alerts[i.ID()] {
			v.Alerts = append(v.Alerts, AlertView{Kind: a, Item: i.ID()})
		}
	}
	return v
}

// ActorView is the staff member who did something.
type ActorView struct {
	ID   shared.UserID
	Name string
}

// HistoryView is one event of an order's history. Only the fields its kind
// uses are set; a zero value means "not part of this event".
type HistoryView struct {
	Kind order.HistoryKind
	At   time.Time
	By   *ActorView // nil for an order placed on the store page

	From, To *StatusView
	Handover order.Handover

	Item             shared.OrderItemID
	ItemName         string // empty once the item was removed from the order
	ItemFrom, ItemTo order.ItemStatus
	Check            order.Check

	Payment       shared.PaymentID
	PaymentKind   order.PaymentKind
	PaymentMethod order.PaymentMethod
	Amount        *shared.Money

	OldDate, NewDate shared.CivilDate
	Reason           string
	CustomerApproved bool

	// Ref is the pedido de compra ("C-0012") of a purchasing move or a
	// promised date; staff data, never in a client view (ADR 0031).
	Ref string
}

// toHistoryView maps a stored history row; itemName is the name of the
// item the entry is about, when the order still holds it.
func toHistoryView(row HistoryRow, itemName string) HistoryView {
	e := row.Entry
	v := HistoryView{
		Kind: e.Kind, At: e.At, Handover: e.Handover,
		Item: e.Item, ItemName: itemName, ItemFrom: e.ItemFrom, ItemTo: e.ItemTo, Check: e.Check,
		Payment: e.Payment, PaymentKind: e.PaymentKind, PaymentMethod: e.PaymentMethod,
		OldDate: e.OldDate, NewDate: e.NewDate, Reason: e.Reason, CustomerApproved: e.CustomerApproved, Ref: e.Ref,
	}
	if !e.By.IsZero() {
		v.By = &ActorView{ID: e.By, Name: row.ByName}
	}
	if e.From.Key != "" {
		s := statusView(e.From)
		v.From = &s
	}
	if e.To.Key != "" {
		s := statusView(e.To)
		v.To = &s
	}
	if e.Amount != (shared.Money{}) {
		a := e.Amount
		v.Amount = &a
	}
	return v
}
