package purchasing

import (
	"context"
	"errors"
	"fmt"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// draft is a pick resolved against "A comprar": the items in the order
// picked, the supplier to send to and the store that sends.
type draft struct {
	items   []ToBuyItem
	contact SupplierContact
	store   string
}

// facts returns what the supplier texts may say, for the number n.
func (d draft) facts(n purchasing.Number) purchasing.MessageFacts {
	items := make([]purchasing.Item, len(d.items))
	for i, it := range d.items {
		items[i] = it.Item
	}
	return purchasing.MessageFacts{Store: d.store, Supplier: d.contact.Name, Number: n, Items: items}
}

// pickItems resolves in against "A comprar": every item must be there for
// the supplier, once. It returns ErrItemNotToBuy for an item that is not
// (sent by someone else first, its order cancelled, or another supplier's),
// purchasing.ErrEmptyPick for no item and purchasing.ErrItemPickedTwice for
// one picked twice.
func (s *Service) pickItems(ctx context.Context, in PickInput) ([]ToBuyItem, error) {
	if len(in.OrderItems) == 0 {
		return nil, refuse(purchasing.ErrEmptyPick, "orderItemIds", msgNoItems)
	}
	tb, err := s.d.Orders.ToBuy(ctx)
	if err != nil {
		return nil, fmt.Errorf("read items to buy: %w", err)
	}
	byItem := make(map[shared.OrderItemID]ToBuyItem, len(tb.Items))
	for _, it := range tb.Items {
		byItem[it.Item.OrderItem] = it
	}
	picked := make([]ToBuyItem, 0, len(in.OrderItems))
	seen := make(map[shared.OrderItemID]bool, len(in.OrderItems))
	for i, id := range in.OrderItems {
		it, ok := byItem[id]
		if !ok || it.Supplier != in.Supplier {
			return nil, fmt.Errorf("item %s: %w", id, ErrItemNotToBuy)
		}
		if seen[id] {
			return nil, refuse(purchasing.ErrItemPickedTwice, fmt.Sprintf("orderItemIds[%d]", i), msgPickedTwice)
		}
		seen[id] = true
		picked = append(picked, it)
	}
	return picked, nil
}

// draftOf resolves the pick and reads the supplier and the store.
func (s *Service) draftOf(ctx context.Context, in PickInput) (draft, error) {
	items, err := s.pickItems(ctx, in)
	if err != nil {
		return draft{}, err
	}
	contact, err := s.d.Suppliers.Contact(ctx, in.Supplier)
	if err != nil {
		return draft{}, fmt.Errorf("supplier %s: %w", in.Supplier, err)
	}
	store, err := s.d.Store.Name(ctx)
	if err != nil {
		return draft{}, fmt.Errorf("store name: %w", err)
	}
	return draft{items: items, contact: contact, store: store}, nil
}

// Preview returns the text a pedido de compra of in would send, with the
// number the store would give next, which is taken only by Send (purchase:write).
// It writes nothing. It returns ErrItemNotToBuy and the pick's refusals as
// Send does.
func (s *Service) Preview(ctx context.Context, in PickInput) (string, error) {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return "", err
	}
	var text string
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		d, err := s.draftOf(ctx, in)
		if err != nil {
			return err
		}
		n, err := s.d.Repo.PeekNumber(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("peek supplier order number: %w", err)
		}
		text = purchasing.OrderMessage(d.facts(n))
		return nil
	})
	return text, err
}

// Send makes the pedido de compra of the picked items (purchase:write): a
// supplier order with a line per item, the items moved to sent_to_supplier
// in the same transaction, and the wa.me link with the message, recorded
// with it. Nothing is sent by the server. The Idempotency-Key is checked
// after the permission; a replay answers the supplier order as it is now,
// with the message it went out with.
//
// Product rule: the message never carries the client's name, phone,
// address or order number (docs/product/design-document.md, System rules):
// it is built from purchasing.MessageFacts, which has no field for them.
//
// It returns ErrIdempotencyKeyRequired, appshared.ErrIdempotencyKeyReused,
// ErrItemNotToBuy when an item is not in "A comprar" for the supplier (also
// when another send took it first, and also for the same key sent twice at
// once: the second waits on the open-line index and finds the item taken; a
// retry after the first committed replays it), purchasing.ErrEmptyPick,
// purchasing.ErrItemPickedTwice, and the order context's refusals.
func (s *Service) Send(ctx context.Context, in SendInput) (SentView, error) {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return SentView{}, err
	}
	if in.Idempotency.IsZero() {
		return SentView{}, ErrIdempotencyKeyRequired
	}
	var out SentView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		replayed, ok, err := s.replaySend(ctx, by, in.Idempotency)
		if err != nil || ok {
			out = replayed
			return err
		}
		c, err := s.call(ctx, by)
		if err != nil {
			return err
		}
		out, err = s.send(ctx, c, in)
		return err
	})
	return out, err
}

// send is the write of Send, inside its transaction.
func (s *Service) send(ctx context.Context, c call, in SendInput) (SentView, error) {
	d, err := s.draftOf(ctx, in.PickInput)
	if err != nil {
		return SentView{}, err
	}
	n, err := s.d.Repo.NextNumber(ctx, c.by.TenantID())
	if err != nil {
		return SentView{}, fmt.Errorf("take supplier order number: %w", err)
	}
	so, err := s.newSupplierOrder(c, in.Supplier, n, d)
	if err != nil {
		return SentView{}, err
	}
	text := purchasing.OrderMessage(d.facts(n))
	url, err := s.d.Links.Link(d.contact.Phone, text)
	if err != nil {
		return SentView{}, fmt.Errorf("wa.me link to supplier %s: %w", in.Supplier, err)
	}
	// The repository copies each line's cost in SQL, so an admin's send
	// keeps the owner's cost (docs/plans/m5-code-design.md, section 4.1).
	if err := s.d.Repo.InsertSupplierOrder(ctx, so, text, url); err != nil {
		if errors.Is(err, ErrOpenLine) {
			// The unique open-line index: another send took an item first.
			return SentView{}, fmt.Errorf("save supplier order %s: %w", so.ID(), ErrItemNotToBuy)
		}
		return SentView{}, fmt.Errorf("save supplier order %s: %w", so.ID(), err)
	}
	if err := s.moveAll(ctx, so, order.ItemSentToSupplier); err != nil {
		return SentView{}, err
	}
	if err := s.d.Idempotency.Remember(ctx, in.Idempotency, so.ID().String(), idempotencyTTL); err != nil {
		return SentView{}, fmt.Errorf("remember idempotency key: %w", err)
	}
	// Read it back: the repository copied the costs the aggregate lacks.
	if so, err = s.loadSupplierOrder(ctx, c.by.TenantID(), so.ID()); err != nil {
		return SentView{}, err
	}
	v, err := s.supplierOrderView(ctx, c.by, so, so.Version())
	return SentView{SupplierOrder: v, WhatsAppURL: url, Text: text}, err
}

func (s *Service) newSupplierOrder(c call, supplier shared.SupplierID, n purchasing.Number, d draft) (*purchasing.SupplierOrder, error) {
	specs := make([]purchasing.LineSpec, len(d.items))
	for i, it := range d.items {
		specs[i] = purchasing.LineSpec{ID: shared.PurchaseLineID(s.d.IDs.NewID()), Item: it.Item}
	}
	return purchasing.Send(purchasing.SendSpec{
		Tenant: c.by.TenantID(), ID: shared.SupplierOrderID(s.d.IDs.NewID()), Number: n, Supplier: supplier,
		SupplierName: d.contact.Name, SupplierPhone: d.contact.Phone, Lines: specs, By: c.by, Now: c.now,
	})
}

// replaySend answers a key already used: the supplier order it made, as it
// is now, with its message. A key that made anything else was reused for
// another request.
func (s *Service) replaySend(ctx context.Context, by shared.Principal, k appshared.IdempotencyKey) (SentView, bool, error) {
	resource, ok, err := s.d.Idempotency.Recall(ctx, k)
	if err != nil || !ok {
		return SentView{}, false, err
	}
	id, err := shared.ParseSupplierOrderID(resource)
	if err != nil {
		return SentView{}, false, appshared.ErrIdempotencyKeyReused
	}
	so, err := s.loadSupplierOrder(ctx, by.TenantID(), id)
	if errors.Is(err, purchasing.ErrSupplierOrderNotFound) {
		return SentView{}, false, appshared.ErrIdempotencyKeyReused
	}
	if err != nil {
		return SentView{}, false, err
	}
	msg, err := s.d.Repo.Message(ctx, by.TenantID(), id)
	if err != nil {
		return SentView{}, false, fmt.Errorf("message of supplier order %s: %w", id, err)
	}
	v, err := s.supplierOrderView(ctx, by, so, so.Version())
	return SentView{SupplierOrder: v, WhatsAppURL: msg.URL, Text: msg.Text}, true, err
}

// moveAll moves the item of every line of so to status to, naming the
// supplier order. Items of orders that are closed, or that lost the item,
// are left where they are (see orderFacts.movable).
func (s *Service) moveAll(ctx context.Context, so *purchasing.SupplierOrder, to order.ItemStatus) error {
	lines := so.Lines()
	items := make([]shared.OrderItemID, len(lines))
	for i, l := range lines {
		items[i] = l.Item().OrderItem
	}
	return s.moveItems(ctx, so, items, to)
}

// moveItems moves items of so's lines to status to, in one go.
func (s *Service) moveItems(ctx context.Context, so *purchasing.SupplierOrder, items []shared.OrderItemID, to order.ItemStatus) error {
	lines := so.Lines()
	orderOf := make(map[shared.OrderItemID]shared.OrderID, len(lines))
	for _, l := range lines {
		it := l.Item()
		orderOf[it.OrderItem] = it.Order
	}
	f, err := s.facts(ctx, ordersOf(lines)...)
	if err != nil {
		return err
	}
	var moves []ItemMove
	for _, id := range items {
		if f.movable(orderOf[id], id) {
			moves = append(moves, ItemMove{Order: orderOf[id], Item: id, To: to, Ref: so.Number().String()})
		}
	}
	if len(moves) == 0 {
		return nil
	}
	if err := s.d.Orders.Move(ctx, moves); err != nil {
		return fmt.Errorf("move items of supplier order %s to %s: %w", so.ID(), to, err)
	}
	return nil
}
