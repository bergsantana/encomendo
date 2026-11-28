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

// notice is an open line that is no longer needed, with the order facts
// that say why.
type notice struct {
	line        StoredLine
	reason      purchasing.NoticeReason
	orderNumber string
}

// findNotices reads every open line and the orders behind them, and keeps
// those the order no longer needs: its order was cancelled, its item was
// removed, or its quantity or choices changed (ADR 0031). Nothing is
// stored: a notice is found when it is read.
func (s *Service) findNotices(ctx context.Context, tenant shared.TenantID) ([]notice, error) {
	open, err := s.d.Repo.OpenLines(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("read open lines: %w", err)
	}
	return s.noticesAmong(ctx, open)
}

// noticesAmong keeps the lines of open that the orders no longer need.
func (s *Service) noticesAmong(ctx context.Context, open []StoredLine) ([]notice, error) {
	ids := make([]shared.OrderID, len(open))
	for i, l := range open {
		ids[i] = l.Item.Order
	}
	f, err := s.facts(ctx, ids...)
	if err != nil {
		return nil, err
	}
	var out []notice
	for _, sl := range open {
		l, err := purchasing.RestoreLine(sl.LineRecord)
		if err != nil {
			return nil, fmt.Errorf("restore line %s: %w", sl.ID, err)
		}
		if reason, ok := purchasing.NoticeOf(l, sideOf(l, f[sl.Item.Order])); ok {
			out = append(out, notice{line: sl, reason: reason, orderNumber: f[sl.Item.Order].Number})
		}
	}
	return out, nil
}

// Notices lists the sent lines the store no longer needs, each with a
// wa.me link that asks the supplier to cancel (purchase:write). The text
// never names the client (docs/product/design-document.md, System rules).
func (s *Service) Notices(ctx context.Context) ([]NoticeView, error) {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return nil, err
	}
	var out []NoticeView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		found, err := s.findNotices(ctx, by.TenantID())
		if err != nil {
			return err
		}
		if len(found) == 0 {
			return nil
		}
		store, err := s.d.Store.Name(ctx)
		if err != nil {
			return fmt.Errorf("store name: %w", err)
		}
		for _, n := range found {
			v, err := s.noticeView(n, store)
			if err != nil {
				return err
			}
			out = append(out, v)
		}
		return nil
	})
	return out, err
}

func (s *Service) noticeView(n notice, store string) (NoticeView, error) {
	l := n.line
	text := purchasing.CancelMessage(purchasing.MessageFacts{
		Store: store, Supplier: l.SupplierName, Number: l.Number, Items: []purchasing.Item{l.Item},
	})
	url, err := s.d.Links.Link(l.SupplierPhone, text)
	if err != nil {
		return NoticeView{}, fmt.Errorf("wa.me link of notice %s: %w", l.ID, err)
	}
	return NoticeView{
		Line: l.ID, SupplierOrder: l.SupplierOrder, Number: l.Number, SupplierName: l.SupplierName,
		Order: l.Item.Order, OrderNumber: n.orderNumber, Name: l.Item.Name, Quantity: l.Item.Quantity,
		Reason: n.reason, WhatsAppURL: url,
	}, nil
}

// ResolveNotice is "Marcar como resolvido" (purchase:write): it cancels a
// sent line the store no longer needs. When the reason is a changed
// quantity or choices, the item also goes back to "A comprar", so it is
// bought again as it is now. It returns purchasing.ErrLineClosed when the
// line is not open or is no longer a notice, and purchasing.ErrLineNotFound
// for a line the store does not have.
func (s *Service) ResolveNotice(ctx context.Context, line shared.PurchaseLineID) error {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return err
	}
	return s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		rec, err := s.d.Repo.SupplierOrderOfLine(ctx, by.TenantID(), line)
		if err != nil {
			return fmt.Errorf("find supplier order of line %s: %w", line, err)
		}
		so, err := restore(rec)
		if err != nil {
			return err
		}
		l, ok := so.Line(line)
		if !ok {
			return fmt.Errorf("line %s: %w", line, purchasing.ErrLineNotFound)
		}
		f, err := s.facts(ctx, l.Item().Order)
		if err != nil {
			return err
		}
		reason, isNotice := purchasing.NoticeOf(l, sideOf(l, f[l.Item().Order]))
		if !isNotice {
			return fmt.Errorf("line %s is still needed: %w", line, purchasing.ErrLineClosed)
		}
		if _, err := so.Resolve(line); err != nil {
			return err
		}
		if err := s.d.Repo.UpdateSupplierOrder(ctx, so); err != nil {
			return fmt.Errorf("save supplier order %s: %w", so.ID(), err)
		}
		if reason != purchasing.NoticeItemChanged {
			return nil
		}
		return s.moveItems(ctx, so, []shared.OrderItemID{l.Item().OrderItem}, order.ItemToBuy)
	})
}

// talker builds "Falar com o fornecedor" links for one call, reading the
// store's name and each supplier once.
type talker struct {
	s        *Service
	chat     bool // the caller has purchase:write
	store    string
	contacts map[shared.SupplierID]SupplierContact
}

// orderLine is the view of one bought item with the line that speaks for it
// (nil when it has none), and false when purchasing has nothing to say of
// the item: it is past "A comprar" with no line.
func (t *talker) orderLine(ctx context.Context, it OrderItemFacts, l *StoredLine) (OrderLineView, bool, error) {
	v := OrderLineView{OrderItem: it.Item, SupplierName: it.SupplierName, Status: OrderLineToBuy}
	supplier := it.Supplier
	talk := purchasing.Item{Name: it.Name, Quantity: it.Quantity, Choices: it.Choices}
	var number purchasing.Number
	if it.Status != order.ItemToBuy {
		if l == nil {
			return OrderLineView{}, false, nil
		}
		v.Status, v.Line, v.SupplierOrder, v.Number = OrderLineStatus(l.Status), l.ID, l.SupplierOrder, l.Number
		v.Promised = l.EffectivePromised()
		if l.SupplierName != "" {
			v.SupplierName = l.SupplierName
		}
		supplier, talk, number = l.Supplier, l.Item, l.Number
	}
	if !t.chat {
		return v, true, nil
	}
	url, err := t.link(ctx, supplier, number, talk)
	v.ChatURL = url
	return v, true, err
}

// link is the wa.me link to the supplier about item, on the supplier order
// number (zero when nothing was sent yet).
func (t *talker) link(ctx context.Context, supplier shared.SupplierID, number purchasing.Number, item purchasing.Item) (string, error) {
	if t.contacts == nil {
		t.contacts = map[shared.SupplierID]SupplierContact{}
		store, err := t.s.d.Store.Name(ctx)
		if err != nil {
			return "", fmt.Errorf("store name: %w", err)
		}
		t.store = store
	}
	c, ok := t.contacts[supplier]
	if !ok {
		var err error
		if c, err = t.s.d.Suppliers.Contact(ctx, supplier); err != nil {
			return "", fmt.Errorf("supplier %s: %w", supplier, err)
		}
		t.contacts[supplier] = c
	}
	text := purchasing.TalkMessage(purchasing.MessageFacts{Store: t.store, Supplier: c.Name, Number: number, Items: []purchasing.Item{item}})
	url, err := t.s.d.Links.Link(c.Phone, text)
	if err != nil {
		return "", fmt.Errorf("wa.me link to supplier %s: %w", supplier, err)
	}
	return url, nil
}
