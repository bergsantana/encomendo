package purchasing

import (
	"context"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// supplierOrderView builds the staff view of so for by, with the lines'
// order facts read now. version is the one the caller's next write needs.
// Costs and prices paid are set only for the owner.
func (s *Service) supplierOrderView(ctx context.Context, by shared.Principal, so *purchasing.SupplierOrder, version int) (SupplierOrderView, error) {
	lines := so.Lines()
	f, err := s.facts(ctx, ordersOf(lines)...)
	if err != nil {
		return SupplierOrderView{}, err
	}
	names, err := s.d.Repo.StaffNames(ctx, []shared.UserID{so.SentBy()})
	if err != nil {
		return SupplierOrderView{}, fmt.Errorf("staff names of supplier order %s: %w", so.ID(), err)
	}
	// The link has no text: it only opens the chat.
	link, err := s.d.Links.Link(so.SupplierPhone(), "")
	if err != nil {
		return SupplierOrderView{}, fmt.Errorf("wa.me link of supplier order %s: %w", so.ID(), err)
	}
	views := make([]PurchaseLineView, len(lines))
	for i, l := range lines {
		views[i] = lineView(so, l, f, owner(by))
	}
	return SupplierOrderView{
		SupplierOrderSummary: summaryOf(so),
		SentBy:               ActorView{ID: so.SentBy(), Name: names[so.SentBy()]},
		WhatsAppURL:          link, Lines: views, CanUndo: so.CanUndo(), Version: version,
	}, nil
}

// summaryOf is the row of "Enviadas" for so.
func summaryOf(so *purchasing.SupplierOrder) SupplierOrderSummary {
	sum := SupplierOrderSummary{
		ID: so.ID(), Number: so.Number(), Supplier: so.Supplier(), SupplierName: so.SupplierName(),
		SentAt: so.SentAt(), Promised: so.Promised(), State: so.State(),
	}
	for _, l := range so.Lines() {
		switch l.Status() {
		case purchasing.LineReceived, purchasing.LineDefective:
			sum.Received++
			sum.Total++
		case purchasing.LineCancelled:
		default:
			sum.Total++
		}
	}
	return sum
}

// lineView is one line of so with what the order says of its item now.
func lineView(so *purchasing.SupplierOrder, l purchasing.Line, f orderFacts, costs bool) PurchaseLineView {
	it := l.Item()
	of := f[it.Order]
	v := PurchaseLineView{
		ID: l.ID(), Order: it.Order, OrderNumber: of.Number, OrderItem: it.OrderItem, CatalogItem: it.CatalogItem,
		Name: it.Name, Quantity: it.Quantity, Choices: choiceViews(it.Choices), Status: l.Status(),
		Promised: so.PromisedFor(l), CheckNote: l.CheckNote(),
	}
	if item, ok := of.Item(it.OrderItem); ok && !it.OrderItem.IsZero() {
		v.Expected, v.ArrivalLimit, v.Alerts = item.Expected, of.ArrivalLimit, item.Alerts
	}
	if reason, ok := purchasing.NoticeOf(l, sideOf(l, of)); ok {
		v.Notice = reason
	}
	if costs {
		v.Cost, v.Paid = l.Cost(), l.Paid()
	}
	return v
}

// sideOf is what the order says now of the item l bought: whether the
// order was cancelled, and the item's quantity and choices, or that the
// order no longer has it.
func sideOf(l purchasing.Line, of OrderFacts) purchasing.OrderSide {
	side := purchasing.OrderSide{OrderCancelled: of.Cancelled}
	item, ok := of.Item(l.Item().OrderItem)
	if !ok || l.Item().OrderItem.IsZero() {
		side.ItemRemoved = true
		return side
	}
	side.Quantity, side.Choices = item.Quantity, item.Choices
	return side
}
