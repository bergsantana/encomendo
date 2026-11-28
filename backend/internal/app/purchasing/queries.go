package purchasing

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// supplierGroup is the items to buy from one supplier.
type supplierGroup struct {
	id    shared.SupplierID
	name  string
	items []ToBuyItem
}

// groupBySupplier groups items by supplier, suppliers by name (then id),
// each keeping the order the order context gave its items: earliest
// delivery first.
func groupBySupplier(items []ToBuyItem) []supplierGroup {
	var groups []supplierGroup
	at := map[shared.SupplierID]int{}
	for _, it := range items {
		i, ok := at[it.Supplier]
		if !ok {
			i = len(groups)
			at[it.Supplier] = i
			groups = append(groups, supplierGroup{id: it.Supplier, name: it.SupplierName})
		}
		groups[i].items = append(groups[i].items, it)
	}
	slices.SortStableFunc(groups, func(a, b supplierGroup) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name)),
			strings.Compare(a.id.String(), b.id.String()),
		)
	})
	return groups
}

// ToBuy returns "A comprar", the bought items still to buy by supplier
// (purchase:write). An item received defective shows as a replacement
// ("Reposição"). The unit cost is set only for the owner.
func (s *Service) ToBuy(ctx context.Context) (ToBuyView, error) {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return ToBuyView{}, err
	}
	var out ToBuyView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		tb, err := s.d.Orders.ToBuy(ctx)
		if err != nil {
			return fmt.Errorf("read items to buy: %w", err)
		}
		ids := make([]shared.OrderItemID, len(tb.Items))
		for i, it := range tb.Items {
			ids[i] = it.Item.OrderItem
		}
		defective, err := s.d.Repo.DefectiveItems(ctx, by.TenantID(), ids)
		if err != nil {
			return fmt.Errorf("read defective items: %w", err)
		}
		out = ToBuyView{Truncated: tb.Truncated}
		for _, g := range groupBySupplier(tb.Items) {
			sv, err := s.supplierToBuy(ctx, g, defective, owner(by))
			if err != nil {
				return err
			}
			out.Suppliers = append(out.Suppliers, sv)
		}
		return nil
	})
	return out, err
}

func (s *Service) supplierToBuy(ctx context.Context, g supplierGroup, defective map[shared.OrderItemID]bool, costs bool) (ToBuySupplierView, error) {
	contact, err := s.d.Suppliers.Contact(ctx, g.id)
	if err != nil {
		return ToBuySupplierView{}, fmt.Errorf("supplier %s: %w", g.id, err)
	}
	sv := ToBuySupplierView{Supplier: g.id, Name: g.name, Phone: contact.Phone}
	for _, it := range g.items {
		v := ToBuyItemView{
			Order: it.Item.Order, OrderNumber: it.Number, OrderItem: it.Item.OrderItem, CatalogItem: it.Item.CatalogItem,
			Name: it.Item.Name, Quantity: it.Item.Quantity, Choices: choiceViews(it.Item.Choices),
			ArrivalLimit: it.ArrivalLimit, Replacement: defective[it.Item.OrderItem],
		}
		if costs {
			v.Cost = it.Cost
		}
		sv.Items = append(sv.Items, v)
	}
	return sv, nil
}

// SupplierOrders lists the store's pedidos de compra, newest first
// (purchase:write); with open, only those with a line still sent or in
// transit. It returns a ValidationError for a cursor the repository did not
// issue.
func (s *Service) SupplierOrders(ctx context.Context, open bool, p shared.PageRequest) (shared.Page[SupplierOrderSummary], error) {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return shared.Page[SupplierOrderSummary]{}, err
	}
	var out shared.Page[SupplierOrderSummary]
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		page, err := s.d.Repo.SupplierOrders(ctx, by.TenantID(), open, p)
		if err != nil {
			return fmt.Errorf("list supplier orders: %w", err)
		}
		out.NextCursor = page.NextCursor
		for _, rec := range page.Items {
			so, err := restore(rec)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, summaryOf(so))
		}
		return nil
	})
	return out, err
}

// SupplierOrder returns one pedido de compra with its lines
// (purchase:write). Costs and prices paid are set only for the owner. It
// returns purchasing.ErrSupplierOrderNotFound.
func (s *Service) SupplierOrder(ctx context.Context, id shared.SupplierOrderID) (SupplierOrderView, error) {
	by, err := authz.Require(ctx, identity.PermPurchaseWrite)
	if err != nil {
		return SupplierOrderView{}, err
	}
	var out SupplierOrderView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		so, err := s.loadSupplierOrder(ctx, by.TenantID(), id)
		if err != nil {
			return err
		}
		out, err = s.supplierOrderView(ctx, by, so, so.Version())
		return err
	})
	return out, err
}

// ToReceive lists what Receber itens can check (receiving:write): the
// open lines by pedido de compra, oldest send first, then the items still
// in "A comprar", which can be received as bought at the counter. It
// carries no cost.
func (s *Service) ToReceive(ctx context.Context) (ToReceiveView, error) {
	by, err := authz.Require(ctx, identity.PermReceivingWrite)
	if err != nil {
		return ToReceiveView{}, err
	}
	var out ToReceiveView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		open, err := s.d.Repo.OpenLines(ctx, by.TenantID())
		if err != nil {
			return fmt.Errorf("read open lines: %w", err)
		}
		tb, err := s.d.Orders.ToBuy(ctx)
		if err != nil {
			return fmt.Errorf("read items to buy: %w", err)
		}
		ids := make([]shared.OrderID, len(open))
		for i, l := range open {
			ids[i] = l.Item.Order
		}
		f, err := s.facts(ctx, ids...)
		if err != nil {
			return err
		}
		out = ToReceiveView{Groups: receiveGroups(open, f), Truncated: tb.Truncated}
		for _, it := range tb.Items {
			out.ToBuy = append(out.ToBuy, ToReceiveLineView{
				Order: it.Item.Order, OrderNumber: it.Number, OrderItem: it.Item.OrderItem,
				Name: it.Item.Name, Quantity: it.Item.Quantity, Choices: choiceViews(it.Item.Choices),
			})
		}
		return nil
	})
	return out, err
}

// receiveGroups groups open lines by supplier order, in the order the
// repository gave them (oldest send first).
func receiveGroups(open []StoredLine, f orderFacts) []ToReceiveGroupView {
	var groups []ToReceiveGroupView
	at := map[shared.SupplierOrderID]int{}
	for _, l := range open {
		i, ok := at[l.SupplierOrder]
		if !ok {
			i = len(groups)
			at[l.SupplierOrder] = i
			groups = append(groups, ToReceiveGroupView{SupplierOrder: l.SupplierOrder, Number: l.Number, SupplierName: l.SupplierName})
		}
		groups[i].Lines = append(groups[i].Lines, ToReceiveLineView{
			Line: l.ID, Order: l.Item.Order, OrderNumber: f[l.Item.Order].Number, OrderItem: l.Item.OrderItem,
			Name: l.Item.Name, Quantity: l.Item.Quantity, Choices: choiceViews(l.Item.Choices),
		})
	}
	return groups
}

// OrderLines returns what purchasing knows of the bought items of order id
// (order:read), so the order screen can show each item's pedido de compra
// and promised date. An item still in "A comprar" shows as that; any other
// shows its open line, or else its latest. With purchase:write each item
// also carries "Falar com o fornecedor", a wa.me link whose text never names
// the client (docs/product/design-document.md, System rules); other roles
// get no link. It returns an error matching shared.ErrNotFound for an
// unknown order.
func (s *Service) OrderLines(ctx context.Context, id shared.OrderID) ([]OrderLineView, error) {
	by, err := authz.Require(ctx, identity.PermOrderRead)
	if err != nil {
		return nil, err
	}
	var out []OrderLineView
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		f, err := s.facts(ctx, id)
		if err != nil {
			return err
		}
		lines, err := s.d.Repo.LinesOfOrder(ctx, by.TenantID(), id)
		if err != nil {
			return fmt.Errorf("read lines of order %s: %w", id, err)
		}
		chat := identity.Allows(by.Role(), identity.PermPurchaseWrite)
		talk := talker{s: s, chat: chat}
		for _, it := range f[id].Items {
			if !it.Bought {
				continue
			}
			v, ok, err := talk.orderLine(ctx, it, lineOfItem(lines, it))
			if err != nil {
				return err
			}
			if ok {
				out = append(out, v)
			}
		}
		return nil
	})
	return out, err
}

// lineOfItem picks the line that speaks for an order item: its open line,
// else its latest one that was not cancelled (lines come oldest first).
func lineOfItem(lines []StoredLine, it OrderItemFacts) *StoredLine {
	var latest *StoredLine
	for i := range lines {
		l := &lines[i]
		if l.Item.OrderItem != it.Item || l.Status == purchasing.LineCancelled {
			continue
		}
		if l.Status.Open() {
			return l
		}
		latest = l
	}
	return latest
}
