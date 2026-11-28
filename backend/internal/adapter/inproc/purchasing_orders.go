package inproc

import (
	"context"
	"fmt"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// PurchasingOrders implements apppurchasing.OrderItems on top of app/order's
// ForPurchasing (ADR 0031). It only translates: every permission and every
// rule is the order context's, and the caller's context (its principal,
// scope and transaction) goes through unchanged, so purchasing gains no
// power of its own over orders.
type PurchasingOrders struct {
	orders apporder.ForPurchasing
}

var _ apppurchasing.OrderItems = (*PurchasingOrders)(nil)

// NewPurchasingOrders returns the adapter over orders.
func NewPurchasingOrders(orders apporder.ForPurchasing) *PurchasingOrders {
	return &PurchasingOrders{orders: orders}
}

// ToBuy implements apppurchasing.OrderItems.
func (a *PurchasingOrders) ToBuy(ctx context.Context) (apppurchasing.ToBuyItems, error) {
	v, err := a.orders.ToBuy(ctx)
	if err != nil {
		return apppurchasing.ToBuyItems{}, err
	}
	out := apppurchasing.ToBuyItems{Truncated: v.Truncated, Items: make([]apppurchasing.ToBuyItem, len(v.Items))}
	for i, it := range v.Items {
		out.Items[i] = apppurchasing.ToBuyItem{
			Item: purchasing.Item{
				Order: it.Order, OrderItem: it.Item, CatalogItem: it.CatalogItem, Name: it.Name,
				Quantity: it.Quantity, Choices: choicesOf(it.Options),
			},
			Number: it.Number.String(), Supplier: it.Supplier, SupplierName: it.SupplierName,
			ArrivalLimit: it.ArrivalLimit, Cost: it.Cost,
		}
	}
	return out, nil
}

// Orders implements apppurchasing.OrderItems.
func (a *PurchasingOrders) Orders(ctx context.Context, ids []shared.OrderID) ([]apppurchasing.OrderFacts, error) {
	views, err := a.orders.PurchaseOrders(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]apppurchasing.OrderFacts, len(views))
	for i, v := range views {
		f := apppurchasing.OrderFacts{
			Order: v.Order, Number: v.Number.String(), Cancelled: v.Cancelled, Closed: v.Closed,
			ArrivalLimit: v.ArrivalLimit, Items: make([]apppurchasing.OrderItemFacts, len(v.Items)),
		}
		for j, it := range v.Items {
			f.Items[j] = apppurchasing.OrderItemFacts{
				Item: it.Item, Bought: it.Bought, Status: it.Status, Arrived: it.Arrived, Name: it.Name,
				Quantity: it.Quantity, Choices: choicesOf(it.Options), Expected: it.Expected, Promised: it.Promised,
				Alerts: it.Alerts, Supplier: it.Supplier, SupplierName: it.SupplierName,
			}
		}
		out[i] = f
	}
	return out, nil
}

// Move implements apppurchasing.OrderItems.
func (a *PurchasingOrders) Move(ctx context.Context, moves []apppurchasing.ItemMove) error {
	in := make([]apporder.PurchaseMoveInput, len(moves))
	for i, m := range moves {
		check, err := checkOf(m.Check)
		if err != nil {
			return err
		}
		in[i] = apporder.PurchaseMoveInput{Order: m.Order, Move: order.PurchaseMove{
			Item: m.Item, To: m.To, Check: check, Note: m.Note, Ref: m.Ref,
		}}
	}
	return a.orders.MovePurchased(ctx, in)
}

// RecordPromisedDates implements apppurchasing.OrderItems.
func (a *PurchasingOrders) RecordPromisedDates(ctx context.Context, dates []apppurchasing.ItemPromise) error {
	in := make([]apporder.PromisedDateInput, len(dates))
	for i, d := range dates {
		in[i] = apporder.PromisedDateInput{Order: d.Order, Item: d.Item, Date: d.Date, Ref: d.Ref}
	}
	return a.orders.RecordPromisedDates(ctx, in)
}

// RecordPaidCosts implements apppurchasing.OrderItems.
func (a *PurchasingOrders) RecordPaidCosts(ctx context.Context, costs []apppurchasing.ItemCost) error {
	in := make([]apporder.PaidCostInput, len(costs))
	for i, c := range costs {
		in[i] = apporder.PaidCostInput{Order: c.Order, Item: c.Item, Cost: c.Cost}
	}
	return a.orders.RecordPaidCosts(ctx, in)
}

// choicesOf keeps what a supplier may read of the client's choices: the
// option's name and the choice's label, never their prices or days.
func choicesOf(opts []order.ChosenOption) []purchasing.Choice {
	if len(opts) == 0 {
		return nil
	}
	out := make([]purchasing.Choice, len(opts))
	for i, o := range opts {
		out[i] = purchasing.Choice{Option: o.Option, ChoiceID: o.Choice, Name: o.Name, Label: o.Label}
	}
	return out
}

// checkOf maps a purchasing check to the order's; the two share values.
func checkOf(c *purchasing.Check) (*order.Check, error) {
	if c == nil {
		return nil, nil
	}
	switch *c {
	case purchasing.CheckOK:
		ok := order.CheckOK
		return &ok, nil
	case purchasing.CheckDefective:
		bad := order.CheckDefective
		return &bad, nil
	default:
		return nil, fmt.Errorf("check %q: %w", *c, order.ErrInvalidItemMove)
	}
}
