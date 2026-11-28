package order

import (
	"context"
	"fmt"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ForPurchasing is what the purchasing context calls through
// adapter/inproc (ADR 0031). Purchasing depends on orders and orders never
// know purchasing, so this is a second driving port, not a port of
// app/order's own. Every method checks its own permission and runs in one
// WithinTx, which joins the caller's transaction through a savepoint, so a
// send or a receipt commits its lines and the order moves together.
type ForPurchasing interface {
	MovePurchased(ctx context.Context, moves []PurchaseMoveInput) error
}

// PurchaseMoveInput is one move purchasing makes on an order's bought item.
type PurchaseMoveInput struct {
	Order shared.OrderID
	Move  order.PurchaseMove
}

// MovePurchased applies moves, grouped by order, each order loaded once and
// saved with the version it was loaded at: purchasing holds no order
// version, so a concurrent write to the same order is
// shared.ErrVersionConflict and the whole call fails. A move to received
// needs receiving:write (staff receive); every other move needs
// purchase:write (owner, admin). It publishes order.item_moved for each
// move, and returns the domain's refusals (order.ErrOrderClosed,
// order.ErrItemNotFound, order.ErrInvalidItemMove) unchanged.
func (s *Service) MovePurchased(ctx context.Context, moves []PurchaseMoveInput) error {
	by, err := requirePurchaseMoves(ctx, moves)
	if err != nil {
		return err
	}
	return s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		w, err := s.writeCtx(ctx, by)
		if err != nil {
			return err
		}
		for _, id := range ordersOf(moves) {
			if err := s.movePurchased(ctx, id, moves, w); err != nil {
				return err
			}
		}
		return nil
	})
}

// movePurchased applies the moves of order id and saves it.
func (s *Service) movePurchased(ctx context.Context, id shared.OrderID, moves []PurchaseMoveInput, w writeCtx) error {
	o, err := s.load(ctx, w.by.TenantID(), id)
	if err != nil {
		return err
	}
	var events []appshared.Event
	for _, m := range moves {
		if m.Order != id {
			continue
		}
		if err := o.MovePurchased(m.Move, w.today, w.now, w.by); err != nil {
			return fmt.Errorf("move item %s of order %s: %w", m.Move.Item, id, err)
		}
		events = append(events, order.ItemMoved{Order: id, Item: m.Move.Item, To: statusOf(o, m.Move.Item)})
	}
	return s.save(ctx, o, events)
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

// ordersOf returns the orders of moves, each once, in first-seen order.
func ordersOf(moves []PurchaseMoveInput) []shared.OrderID {
	seen := map[shared.OrderID]bool{}
	var out []shared.OrderID
	for _, m := range moves {
		if !seen[m.Order] {
			seen[m.Order] = true
			out = append(out, m.Order)
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
