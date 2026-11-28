package dto

import (
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// FinanceSummary is the FinanceSummary schema: Financeiro, the owner's. The
// use case refuses every other role (finance:read), so the cost and margin
// of a row are always written, null when unknown.
type FinanceSummary struct {
	Truncated    bool             `json:"truncated"`
	MarginsTotal MarginsTotal     `json:"marginsTotal"`
	From         shared.CivilDate `json:"from"`
	To           shared.CivilDate `json:"to"`
	Received     Received         `json:"received"`
	ToReceive    ToReceive        `json:"toReceive"`
	ToRefund     []Refund         `json:"toRefund"`
	Margins      []MarginRow      `json:"margins"`
}

// MarginsTotal is the period's total of the margins.
type MarginsTotal struct {
	Total  shared.Money `json:"total"`
	Cost   shared.Money `json:"cost"`
	Margin *Margin      `json:"margin"`
}

// MoneyByKey is the MoneyByKey schema.
type MoneyByKey struct {
	Key   string       `json:"key"`
	Total shared.Money `json:"total"`
}

// Received is what the store received in the period.
type Received struct {
	Total    shared.Money `json:"total"`
	ByKind   []MoneyByKey `json:"byKind"`
	ByMethod []MoneyByKey `json:"byMethod"`
}

// ToReceive is the balances still due on active orders: their total, how
// many they are and the orders themselves, by delivery date.
type ToReceive struct {
	Total  shared.Money   `json:"total"`
	Orders int            `json:"orders"`
	List   []OrderSummary `json:"list"`
}

func newToReceive(v apporder.ToReceiveView) ToReceive {
	out := ToReceive{Total: v.Total, Orders: len(v.Orders), List: make([]OrderSummary, 0, len(v.Orders))}
	for _, o := range v.Orders {
		out.List = append(out.List, NewOrderSummary(o))
	}
	return out
}

// Refund is an order with "Valor a devolver".
type Refund struct {
	Order  OrderSummary `json:"order"`
	Amount shared.Money `json:"amount"`
}

// MarginRow is one order handed over in the period.
type MarginRow struct {
	OrderID      shared.OrderID       `json:"orderId"`
	Number       string               `json:"number"`
	CustomerName string               `json:"customerName"`
	HandedOverOn shared.CivilDate     `json:"handedOverOn"`
	Total        shared.Money         `json:"total"`
	Cost         *Owned[shared.Money] `json:"cost,omitempty"`
	Margin       *Owned[Margin]       `json:"margin,omitempty"`
}

func moneyByKey(in []apporder.MoneyByKey) []MoneyByKey {
	out := make([]MoneyByKey, 0, len(in))
	for _, m := range in {
		out = append(out, MoneyByKey{Key: m.Key, Total: m.Total})
	}
	return out
}

// NewFinanceSummary maps Financeiro.
func NewFinanceSummary(v apporder.FinanceView) FinanceSummary {
	out := FinanceSummary{
		Truncated: v.Truncated, From: v.From, To: v.To,
		MarginsTotal: MarginsTotal{Total: v.MarginsTotal.Total, Cost: v.MarginsTotal.Cost},
		Received:     Received{Total: v.Received.Total, ByKind: moneyByKey(v.Received.ByKind), ByMethod: moneyByKey(v.Received.ByMethod)},
		ToReceive:    newToReceive(v.ToReceive),
		ToRefund:     make([]Refund, 0, len(v.ToRefund)), Margins: make([]MarginRow, 0, len(v.Margins)),
	}
	if m := v.MarginsTotal.Margin; m != nil {
		out.MarginsTotal.Margin = &Margin{Cents: m.Cents, BasisPoints: m.BasisPoints}
	}
	for _, r := range v.ToRefund {
		out.ToRefund = append(out.ToRefund, Refund{Order: NewOrderSummary(r.Order), Amount: r.Amount})
	}
	for _, m := range v.Margins {
		out.Margins = append(out.Margins, MarginRow{
			OrderID: m.Order, Number: m.Number, CustomerName: m.CustomerName, HandedOverOn: m.HandedOverOn, Total: m.Total,
			Cost: Own(m.Cost), Margin: ownedMargin(m.Margin),
		})
	}
	return out
}
