package order

import (
	"context"
	"errors"
	"fmt"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// ForPurchasing is what the purchasing context calls through
// adapter/inproc (ADR 0031). Purchasing depends on orders and orders never
// know purchasing, so this is a second driving port, not a port of
// app/order's own. Every method checks its own permission and runs in one
// WithinTx, which joins the caller's transaction through a savepoint, so a
// send or a receipt commits its lines and the order moves together.
type ForPurchasing interface {
	ToBuy(ctx context.Context) (ToBuyView, error)
	PurchaseOrders(ctx context.Context, ids []shared.OrderID) ([]PurchaseOrderView, error)
	MovePurchased(ctx context.Context, moves []PurchaseMoveInput) error
	RecordPromisedDates(ctx context.Context, dates []PromisedDateInput) error
	RecordPaidCosts(ctx context.Context, costs []PaidCostInput) error
}

// ToBuyView is "A comprar" as orders know it: the bought items still to
// buy of purchasable orders. Truncated is true when the store has more
// active orders than one read covers (the 500 of Hoje).
type ToBuyView struct {
	Items     []BoughtItemView
	Truncated bool
}

// BoughtItemView is one bought item still to buy. Its supplier is the one
// the item was sold with (or switched to, the backup), never the catalog's
// current one. Cost is the unit cost at order time, nil unless the caller
// sees costs; purchasing never stores it from here (it copies it in SQL).
type BoughtItemView struct {
	Order        shared.OrderID
	Number       order.Number
	Item         shared.OrderItemID
	CatalogItem  shared.ItemID
	Name         string
	Quantity     int
	Options      []order.ChosenOption
	Supplier     shared.SupplierID
	SupplierName string
	ArrivalLimit shared.CivilDate
	Cost         *shared.Money
}

// PurchaseOrderView is what purchasing needs of one order: whether it was
// cancelled or closed, its arrival limit and each of its items as they are
// now. It carries no cost.
type PurchaseOrderView struct {
	Order        shared.OrderID
	Number       order.Number
	Cancelled    bool
	Closed       bool // handed over or cancelled
	ArrivalLimit shared.CivilDate
	Items        []PurchaseItemView
}

// PurchaseItemView is one item of an order, bought or not, as it is now.
type PurchaseItemView struct {
	Item         shared.OrderItemID
	Bought       bool
	Status       order.ItemStatus
	Arrived      bool
	Name         string
	Quantity     int
	Options      []order.ChosenOption
	Expected     shared.CivilDate
	Promised     shared.CivilDate // zero when the supplier promised no date
	Alerts       []schedule.Alert
	Supplier     shared.SupplierID
	SupplierName string
}

// PurchaseMoveInput is one move purchasing makes on an order's bought item.
type PurchaseMoveInput struct {
	Order shared.OrderID
	Move  order.PurchaseMove
}

// PromisedDateInput is a supplier's promised date for one bought item.
type PromisedDateInput struct {
	Order shared.OrderID
	Item  shared.OrderItemID
	Date  shared.CivilDate
	Ref   string // the pedido de compra, "C-0012"
}

// PaidCostInput is the unit price paid for one bought item.
type PaidCostInput struct {
	Order shared.OrderID
	Item  shared.OrderItemID
	Cost  shared.Money
}

// ToBuy returns the bought items in to_buy of purchasable orders
// (order.Order.Purchasable with the store's "Liberar compras antes do
// sinal"), earliest delivery first. It needs purchase:write, or
// receiving:write, since staff receive items bought at the counter.
//
// Product rule: a purchase only goes out after the deposit is paid, unless
// the owner changed this (docs/product/design-document.md, System rules).
func (s *Service) ToBuy(ctx context.Context) (ToBuyView, error) {
	by, err := requireAny(ctx, identity.PermPurchaseWrite, identity.PermReceivingWrite)
	if err != nil {
		return ToBuyView{}, err
	}
	var out ToBuyView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		w, err := s.writeCtx(ctx, by)
		if err != nil {
			return err
		}
		rows, truncated, err := s.active(ctx, readCtx{by: by, facts: w.facts, now: w.now, today: w.today})
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
		out = ToBuyView{Items: boughtToBuy(orders, w.facts.PurchaseBeforeDeposit, identity.SeesCosts(by.Role())), Truncated: truncated}
		return nil
	})
	return out, err
}

// boughtToBuy lists the bought items still to buy of the purchasable
// orders, with costs only for a caller who sees them.
func boughtToBuy(orders []*order.Order, beforeDeposit, seesCosts bool) []BoughtItemView {
	var out []BoughtItemView
	for _, o := range orders {
		if !o.Purchasable(beforeDeposit) {
			continue
		}
		for _, it := range o.Items() {
			if !it.Bought() || it.Status() != order.ItemToBuy {
				continue
			}
			snap, src := it.Snapshot(), it.Sourcing()
			v := BoughtItemView{
				Order: o.ID(), Number: o.Number(), Item: it.ID(), CatalogItem: snap.CatalogItem, Name: snap.Name,
				Quantity: snap.Quantity, Options: snap.Options, Supplier: src.Supplier, SupplierName: src.SupplierName,
				ArrivalLimit: o.ArrivalLimit(),
			}
			if seesCosts {
				v.Cost = src.Cost
			}
			out = append(out, v)
		}
	}
	return out
}

// PurchaseOrders returns each order of ids as purchasing needs it, in the
// order asked, each once (order:read). It carries no cost. An order the
// store does not have is an error matching shared.ErrNotFound.
func (s *Service) PurchaseOrders(ctx context.Context, ids []shared.OrderID) ([]PurchaseOrderView, error) {
	var out []PurchaseOrderView
	err := s.read(ctx, identity.PermOrderRead, func(ctx context.Context, r readCtx) error {
		for _, id := range unique(ids) {
			o, err := s.load(ctx, r.by.TenantID(), id)
			if err != nil {
				return err
			}
			out = append(out, purchaseOrderView(o, r.today))
		}
		return nil
	})
	return out, err
}

func purchaseOrderView(o *order.Order, today shared.CivilDate) PurchaseOrderView {
	v := PurchaseOrderView{
		Order: o.ID(), Number: o.Number(), Cancelled: o.Phase() == workflow.PhaseCancelled, Closed: o.Closed(),
		ArrivalLimit: o.ArrivalLimit(),
	}
	alerts := o.Alerts(today)
	for _, it := range o.Items() {
		snap, src := it.Snapshot(), it.Sourcing()
		promised, _ := it.Promised()
		v.Items = append(v.Items, PurchaseItemView{
			Item: it.ID(), Bought: it.Bought(), Status: it.Status(), Arrived: it.Arrived(), Name: snap.Name,
			Quantity: snap.Quantity, Options: snap.Options, Expected: it.Expected(), Promised: promised,
			Alerts: alerts[it.ID()], Supplier: src.Supplier, SupplierName: src.SupplierName,
		})
	}
	return v
}

// MovePurchased applies moves, grouped by order, each order loaded once and
// saved with the version it was loaded at: purchasing holds no order
// version, so a concurrent write to the same order is
// shared.ErrVersionConflict and the whole call fails. A move to received
// needs receiving:write (staff receive), keyed on the move asked for, so a
// defect that lands on to_buy is still a receipt; every other move needs
// purchase:write (owner, admin). It publishes order.item_moved for each
// move, and returns the domain's refusals (order.ErrOrderClosed,
// order.ErrItemNotFound, order.ErrInvalidItemMove) unchanged.
func (s *Service) MovePurchased(ctx context.Context, moves []PurchaseMoveInput) error {
	by, err := requirePurchaseMoves(ctx, moves)
	if err != nil {
		return err
	}
	orders := make([]shared.OrderID, len(moves))
	for i, m := range moves {
		orders[i] = m.Order
	}
	return s.eachOrder(ctx, by, orders, func(o *order.Order, w writeCtx) ([]appshared.Event, error) {
		var events []appshared.Event
		for _, m := range moves {
			if m.Order != o.ID() {
				continue
			}
			if err := o.MovePurchased(m.Move, w.today, w.now, w.by); err != nil {
				return nil, fmt.Errorf("move item %s of order %s: %w", m.Move.Item, o.ID(), err)
			}
			events = append(events, order.ItemMoved{Order: o.ID(), Item: m.Move.Item, To: statusOf(o, m.Move.Item)})
		}
		return events, nil
	})
}

// RecordPromisedDates records each supplier's promised date (purchase:write)
// and redates the items; the promise to the client never moves (ADR 0030).
// It returns the domain's refusals unchanged (order.ErrDateInPast,
// order.ErrInvalidItemMove for an item not sent, order.ErrOrderClosed).
func (s *Service) RecordPromisedDates(ctx context.Context, dates []PromisedDateInput) error {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return err
	}
	orders := make([]shared.OrderID, len(dates))
	for i, d := range dates {
		orders[i] = d.Order
	}
	return s.eachOrder(ctx, by, orders, func(o *order.Order, w writeCtx) ([]appshared.Event, error) {
		for _, d := range dates {
			if d.Order != o.ID() {
				continue
			}
			if err := o.RecordPromisedDate(d.Item, d.Date, d.Ref, w.today, w.now, w.by); err != nil {
				return nil, fmt.Errorf("promised date of item %s of order %s: %w", d.Item, o.ID(), err)
			}
		}
		return nil, nil
	})
}

// RecordPaidCosts records the unit price paid for each item. Only the owner
// may (purchase:write and identity.SeesCosts, ADR 0025): anyone else is
// shared.ErrForbidden. It works on closed orders too, since margins are
// read after handover.
func (s *Service) RecordPaidCosts(ctx context.Context, costs []PaidCostInput) error {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return err
	}
	if !identity.SeesCosts(by.Role()) {
		return fmt.Errorf("role %s may not record a price paid: %w", by.Role(), shared.ErrForbidden)
	}
	orders := make([]shared.OrderID, len(costs))
	for i, c := range costs {
		orders[i] = c.Order
	}
	return s.eachOrder(ctx, by, orders, func(o *order.Order, _ writeCtx) ([]appshared.Event, error) {
		for _, c := range costs {
			if c.Order != o.ID() {
				continue
			}
			if err := o.RecordPaidCost(c.Item, c.Cost); err != nil {
				return nil, fmt.Errorf("paid cost of item %s of order %s: %w", c.Item, o.ID(), err)
			}
		}
		return nil, nil
	})
}

// eachOrder loads each order of ids once, applies change and saves it with
// its events, all in one transaction that joins the caller's.
func (s *Service) eachOrder(ctx context.Context, by shared.Principal, ids []shared.OrderID, change func(*order.Order, writeCtx) ([]appshared.Event, error)) error {
	return s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		w, err := s.writeCtx(ctx, by)
		if err != nil {
			return err
		}
		for _, id := range unique(ids) {
			o, err := s.load(ctx, by.TenantID(), id)
			if err != nil {
				return err
			}
			events, err := change(o, w)
			if err != nil {
				return err
			}
			if err := s.save(ctx, o, events); err != nil {
				return err
			}
		}
		return nil
	})
}

// requirePurchaseMoves authorizes every move and returns the caller.
func requirePurchaseMoves(ctx context.Context, moves []PurchaseMoveInput) (shared.Principal, error) {
	need := map[identity.Permission]bool{}
	for _, m := range moves {
		if m.Move.To == order.ItemReceived {
			need[identity.PermReceivingWrite] = true
		} else {
			need[identity.PermPurchaseWrite] = true
		}
	}
	var by shared.Principal
	for _, perm := range []identity.Permission{identity.PermReceivingWrite, identity.PermPurchaseWrite} {
		if !need[perm] {
			continue
		}
		p, err := authz.Require(ctx, perm)
		if err != nil {
			return shared.Principal{}, err
		}
		by = p
	}
	if by.IsZero() {
		return authz.Require(ctx, identity.PermReceivingWrite) // no moves: still a caller who may receive
	}
	return by, nil
}

// requireAny returns the caller when they hold one of perms, and the
// refusal of the last one otherwise. A refusal about the caller rather
// than the permission (no principal, no store, a scope that changed) comes
// back at once, since no other permission can fix it.
func requireAny(ctx context.Context, perms ...identity.Permission) (shared.Principal, error) {
	var last error
	for _, perm := range perms {
		by, err := authz.Require(ctx, perm)
		if err == nil {
			return by, nil
		}
		if !errors.Is(err, shared.ErrForbidden) || errors.Is(err, authz.ErrScopeMismatch) || errors.Is(err, authz.ErrTenantRequired) {
			return shared.Principal{}, err
		}
		last = err
	}
	return shared.Principal{}, last
}

// unique returns ids each once, in first-seen order.
func unique[T comparable](ids []T) []T {
	seen := make(map[T]bool, len(ids))
	out := make([]T, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// statusOf returns where item id of o stands now.
func statusOf(o *order.Order, id shared.OrderItemID) order.ItemStatus {
	for _, i := range o.Items() {
		if i.ID() == id {
			return i.Status()
		}
	}
	return ""
}
