package purchasing

// NoticeReason says why an open line is no longer needed ("Avisar o
// fornecedor"). The values are the contract's notice reasons.
type NoticeReason string

// The reasons.
const (
	// NoticeOrderCancelled: the client's order was cancelled.
	NoticeOrderCancelled NoticeReason = "order_cancelled"
	// NoticeItemRemoved: the order no longer has the item.
	NoticeItemRemoved NoticeReason = "item_removed"
	// NoticeItemChanged: the order's item now has another quantity or
	// another set of option choices.
	NoticeItemChanged NoticeReason = "item_changed"
)

// OrderSide is what the order context says of a line's item now. The inproc
// adapter fills it from the order; purchasing never reads the order itself
// (ADR 0031).
type OrderSide struct {
	OrderCancelled bool
	ItemRemoved    bool // the order has no such item (or the line's item is zero)
	Quantity       int
	Choices        []Choice
}

// NoticeOf reports why an open line is no longer needed, and false when it
// still is, or when the line is not open: a closed line needs no notice.
// An order cancelled comes first, then an item removed (the line's own
// order item is zero once it was), then an item changed: its quantity
// differs, or its set of (option, choice) pairs does, in any order.
func NoticeOf(l Line, now OrderSide) (NoticeReason, bool) {
	if !l.status.Open() {
		return "", false
	}
	switch {
	case now.OrderCancelled:
		return NoticeOrderCancelled, true
	case now.ItemRemoved || l.item.OrderItem.IsZero():
		return NoticeItemRemoved, true
	case now.Quantity != l.item.Quantity || choiceSet(now.Choices) != choiceSet(l.item.Choices):
		return NoticeItemChanged, true
	default:
		return "", false
	}
}
