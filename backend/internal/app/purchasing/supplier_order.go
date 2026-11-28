package purchasing

import (
	"context"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// effect is what a supplier order change does to the order context once
// the supplier order is saved, in the same transaction.
type effect func(ctx context.Context) error

// change is one write on a supplier order the caller read at version: the
// domain step, which may refuse and returns its effect on the orders.
type change struct {
	id      shared.SupplierOrderID
	version int
	apply   func(ctx context.Context, so *purchasing.SupplierOrder, c call) (effect, error)
}

// change runs ch for by in one transaction: it loads the supplier order at
// the caller's version, lets the domain act, saves it, applies the effect
// on the orders, and answers the supplier order as saved.
func (s *Service) change(ctx context.Context, by shared.Principal, ch change) (SupplierOrderView, error) {
	var out SupplierOrderView
	err := s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := s.call(ctx, by)
		if err != nil {
			return err
		}
		so, err := s.loadSupplierOrder(ctx, by.TenantID(), ch.id)
		if err != nil {
			return err
		}
		if err := so.CheckVersion(ch.version); err != nil {
			return err
		}
		apply, err := ch.apply(ctx, so, c)
		if err != nil {
			return err
		}
		if err := s.d.Repo.UpdateSupplierOrder(ctx, so); err != nil {
			return fmt.Errorf("save supplier order %s: %w", so.ID(), err)
		}
		if apply != nil {
			if err := apply(ctx); err != nil {
				return err
			}
		}
		// The repository stored the next version.
		out, err = s.supplierOrderView(ctx, by, so, so.Version()+1)
		return err
	})
	return out, err
}

// Undo is "Desfazer envio" (purchase:write): every open line is cancelled
// and its item goes back to "A comprar". The items of a cancelled order or
// of a removed item are not moved. It returns purchasing.ErrUndone,
// purchasing.ErrSupplierOrderReceived once a line was received or found
// defective, shared.ErrVersionConflict and purchasing.ErrSupplierOrderNotFound.
func (s *Service) Undo(ctx context.Context, in VersionInput) (SupplierOrderView, error) {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return SupplierOrderView{}, err
	}
	return s.change(ctx, by, change{id: in.ID, version: in.Version, apply: func(_ context.Context, so *purchasing.SupplierOrder, c call) (effect, error) {
		items, err := so.Undo(c.by, c.now)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context) error { return s.moveItems(ctx, so, items, order.ItemToBuy) }, nil
	}})
}

// MarkInTransit marks every sent line in transit and moves their items
// (purchase:write). It returns purchasing.ErrNothingOpen when no line is
// sent, purchasing.ErrUndone, shared.ErrVersionConflict and
// purchasing.ErrSupplierOrderNotFound.
func (s *Service) MarkInTransit(ctx context.Context, in VersionInput) (SupplierOrderView, error) {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return SupplierOrderView{}, err
	}
	return s.change(ctx, by, change{id: in.ID, version: in.Version, apply: func(_ context.Context, so *purchasing.SupplierOrder, c call) (effect, error) {
		items, err := so.MarkInTransit(c.now)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context) error { return s.moveItems(ctx, so, items, order.ItemInTransit) }, nil
	}})
}

// RecordPromisedDate records the supplier's promised date of the order,
// with a different date for some lines (purchase:write). It redates the
// items still on an open order, which raises "Pode atrasar" or "Atrasada"
// when the date is late; the promise to the client never moves (ADR 0030).
// It returns purchasing.ErrDateInPast and the other refusals of
// purchasing.SupplierOrder.RecordPromisedDate, shared.ErrVersionConflict
// and purchasing.ErrSupplierOrderNotFound.
func (s *Service) RecordPromisedDate(ctx context.Context, in PromisedDateInput) (SupplierOrderView, error) {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return SupplierOrderView{}, err
	}
	overrides := make(map[shared.PurchaseLineID]shared.CivilDate, len(in.Lines))
	for i, l := range in.Lines {
		if _, dup := overrides[l.Line]; dup {
			return SupplierOrderView{}, invalid(fmt.Sprintf("lines[%d].lineId", i), msgLineRepeated)
		}
		overrides[l.Line] = l.Date
	}
	return s.change(ctx, by, change{id: in.ID, version: in.Version, apply: func(_ context.Context, so *purchasing.SupplierOrder, c call) (effect, error) {
		dates, err := so.RecordPromisedDate(in.Date, overrides, c.today)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context) error { return s.redate(ctx, so, dates) }, nil
	}})
}

// redate gives the order context the promised dates of items still on an
// open order.
func (s *Service) redate(ctx context.Context, so *purchasing.SupplierOrder, dates []purchasing.ItemDate) error {
	ids := make([]shared.OrderID, len(dates))
	for i, d := range dates {
		ids[i] = d.Order
	}
	f, err := s.facts(ctx, ids...)
	if err != nil {
		return err
	}
	var promises []ItemPromise
	for _, d := range dates {
		if f.movable(d.Order, d.Item) {
			promises = append(promises, ItemPromise{Order: d.Order, Item: d.Item, Date: d.Date, Ref: so.Number().String()})
		}
	}
	if len(promises) == 0 {
		return nil
	}
	if err := s.d.Orders.RecordPromisedDates(ctx, promises); err != nil {
		return fmt.Errorf("record promised dates of supplier order %s: %w", so.ID(), err)
	}
	return nil
}

// RecordPaidPrices records the unit price paid per line (the owner only:
// purchase:write and identity.SeesCosts, else shared.ErrForbidden), and
// copies it to the order items for Financeiro's margin. Prices are in cents
// of the store's currency. It returns purchasing.ErrLineNotFound,
// purchasing.ErrLineClosed for a cancelled line, a ValidationError on
// "lines[i].paid", shared.ErrVersionConflict and
// purchasing.ErrSupplierOrderNotFound.
func (s *Service) RecordPaidPrices(ctx context.Context, in PaidPricesInput) (SupplierOrderView, error) {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return SupplierOrderView{}, err
	}
	if !owner(by) {
		return SupplierOrderView{}, fmt.Errorf("role %s may not record prices paid: %w", by.Role(), shared.ErrForbidden)
	}
	return s.change(ctx, by, change{id: in.ID, version: in.Version, apply: func(ctx context.Context, so *purchasing.SupplierOrder, _ call) (effect, error) {
		prices, err := s.pricesOf(ctx, in.Prices)
		if err != nil {
			return nil, err
		}
		paid, err := so.RecordPaidPrices(prices)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context) error { return s.copyPaid(ctx, paid) }, nil
	}})
}

// pricesOf turns bare cents into money of the store's currency.
func (s *Service) pricesOf(ctx context.Context, in []LinePrice) (map[shared.PurchaseLineID]shared.Money, error) {
	currency, err := s.d.Store.Currency(ctx)
	if err != nil {
		return nil, fmt.Errorf("store currency: %w", err)
	}
	out := make(map[shared.PurchaseLineID]shared.Money, len(in))
	for i, p := range in {
		if _, dup := out[p.Line]; dup {
			return nil, invalid(fmt.Sprintf("prices[%d].lineId", i), msgLineRepeated)
		}
		m, err := shared.NewMoney(p.Cents, currency)
		if err != nil {
			return nil, fmt.Errorf("price of line %s: %w", p.Line, err)
		}
		out[p.Line] = m
	}
	return out, nil
}

// copyPaid gives the order context the prices paid of items it still has.
func (s *Service) copyPaid(ctx context.Context, paid []purchasing.ItemPrice) error {
	ids := make([]shared.OrderID, len(paid))
	for i, p := range paid {
		ids[i] = p.Order
	}
	f, err := s.facts(ctx, ids...)
	if err != nil {
		return err
	}
	var costs []ItemCost
	for _, p := range paid {
		if f.has(p.Order, p.Item) {
			costs = append(costs, ItemCost{Order: p.Order, Item: p.Item, Cost: p.Price})
		}
	}
	if len(costs) == 0 {
		return nil
	}
	if err := s.d.Orders.RecordPaidCosts(ctx, costs); err != nil {
		return fmt.Errorf("record prices paid: %w", err)
	}
	return nil
}
