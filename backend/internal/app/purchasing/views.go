package purchasing

import (
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// These are staff views. Costs and prices paid are set only for the owner
// (identity.SeesCosts); no view here is ever sent to a client
// (docs/product/design-document.md, System rules).

// ChoiceView is an option choice of a bought item, as it was sold.
type ChoiceView struct{ Name, Label string }

// ToBuyItemView is one bought item still to buy.
type ToBuyItemView struct {
	Order        shared.OrderID
	OrderNumber  string
	OrderItem    shared.OrderItemID
	CatalogItem  shared.ItemID
	Name         string
	Quantity     int
	Choices      []ChoiceView
	ArrivalLimit shared.CivilDate
	// Replacement is "Reposição": the item was received defective and is
	// bought again.
	Replacement bool
	Cost        *shared.Money // unit cost at order time; the owner's only
}

// ToBuySupplierView is the items to buy from one supplier.
type ToBuySupplierView struct {
	Supplier shared.SupplierID
	Name     string
	Phone    string // digits for wa.me
	Items    []ToBuyItemView
}

// ToBuyView is "A comprar", by supplier name.
type ToBuyView struct {
	Suppliers []ToBuySupplierView
	Truncated bool // more active orders than one read covers
}

// SentView is a pedido de compra just sent: the supplier order, the wa.me
// link with the message, and the message for "Copiar mensagem". Nothing was
// sent by the server.
type SentView struct {
	SupplierOrder SupplierOrderView
	WhatsAppURL   string
	Text          string
}

// SupplierOrderSummary is one row of "Enviadas".
type SupplierOrderSummary struct {
	ID           shared.SupplierOrderID
	Number       purchasing.Number
	Supplier     shared.SupplierID
	SupplierName string
	SentAt       time.Time
	Promised     shared.CivilDate // zero when none was recorded
	Received     int              // lines received or found defective
	Total        int              // lines not cancelled
	State        purchasing.State
}

// ActorView is the staff member who did something.
type ActorView struct {
	ID   shared.UserID
	Name string
}

// PurchaseLineView is one line of a pedido de compra.
type PurchaseLineView struct {
	ID          shared.PurchaseLineID
	Order       shared.OrderID
	OrderNumber string
	OrderItem   shared.OrderItemID // zero once removed from the order
	CatalogItem shared.ItemID
	Name        string
	Quantity    int
	Choices     []ChoiceView
	Status      purchasing.LineStatus
	// Promised is the supplier's date for this line ("Prazo do fornecedor"),
	// its own or the order's; zero when none.
	Promised     shared.CivilDate
	Expected     shared.CivilDate // the order item's forecast; zero once removed
	ArrivalLimit shared.CivilDate // the order's "Precisa chegar até"; zero once removed
	Alerts       []schedule.Alert
	Notice       purchasing.NoticeReason // "" when the line is still needed
	CheckNote    string
	Cost         *shared.Money // the owner's only
	Paid         *shared.Money // the owner's only
}

// SupplierOrderView is a pedido de compra with its lines.
type SupplierOrderView struct {
	SupplierOrderSummary
	SentBy ActorView
	// WhatsAppURL opens the chat with the supplier, with no text.
	WhatsAppURL string
	Lines       []PurchaseLineView
	CanUndo     bool
	Version     int
}

// ToReceiveLineView is one line Receber itens can check. Line is zero for
// an item still in "A comprar", received at the counter by OrderItem.
type ToReceiveLineView struct {
	Line        shared.PurchaseLineID
	Order       shared.OrderID
	OrderNumber string
	OrderItem   shared.OrderItemID
	Name        string
	Quantity    int
	Choices     []ChoiceView
}

// ToReceiveGroupView is the open lines of one pedido de compra.
type ToReceiveGroupView struct {
	SupplierOrder shared.SupplierOrderID
	Number        purchasing.Number
	SupplierName  string
	Lines         []ToReceiveLineView
}

// ToReceiveView is what Receber itens lists. It carries no cost.
type ToReceiveView struct {
	Groups    []ToReceiveGroupView // oldest send first
	ToBuy     []ToReceiveLineView  // "Comprados sem pedido de compra"
	Truncated bool
}

// PhotoUploadView is where and how to upload one receiving photo. Key goes
// back in a receipt's photo keys.
type PhotoUploadView struct {
	Key       string
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// OrderRefView names an order.
type OrderRefView struct {
	Order  shared.OrderID
	Number string
}

// ReceiptEntryView is one checked line of a receipt.
type ReceiptEntryView struct {
	Line        shared.PurchaseLineID
	Order       shared.OrderID
	OrderNumber string
	OrderItem   shared.OrderItemID
	Name        string
	Check       purchasing.Check
}

// BackToBuyView is an item found defective, back in "A comprar".
type BackToBuyView struct {
	OrderItem shared.OrderItemID
	Name      string
}

// ReceiptView is a saved receipt.
type ReceiptView struct {
	ID              shared.ReceiptID
	ReceivedAt      time.Time
	Entries         []ReceiptEntryView
	CompletedOrders []OrderRefView // orders whose every item has now arrived
	BackToBuy       []BackToBuyView
}

// NoticeView is a sent line no longer needed ("Avisar o fornecedor").
type NoticeView struct {
	Line          shared.PurchaseLineID
	SupplierOrder shared.SupplierOrderID
	Number        purchasing.Number
	SupplierName  string
	Order         shared.OrderID
	OrderNumber   string
	Name          string
	Quantity      int
	Reason        purchasing.NoticeReason
	// WhatsAppURL asks the supplier to cancel; it never names the client.
	WhatsAppURL string
}

// OrderLineStatus is where a bought item stands in purchasing: "to_buy"
// while it waits in "A comprar", else its line's status.
type OrderLineStatus string

// OrderLineToBuy is the status of an item still in "A comprar".
const OrderLineToBuy OrderLineStatus = "to_buy"

// OrderLineView is what purchasing knows of one bought item of an order.
type OrderLineView struct {
	OrderItem    shared.OrderItemID
	SupplierName string
	Status       OrderLineStatus
	// The rest is set for an item that has a line.
	Line          shared.PurchaseLineID
	SupplierOrder shared.SupplierOrderID // zero for an item received at the counter
	Number        purchasing.Number      // zero for an item received at the counter
	Promised      shared.CivilDate
	// ChatURL is "Falar com o fornecedor": a wa.me link with a text that
	// never names the client. Set only for a caller with purchase:write.
	ChatURL string
}

// NeedKind says what a purchasing entry of Hoje asks.
type NeedKind string

// The kinds of need, in the order Hoje lists them.
const (
	NeedNotifySupplier NeedKind = "notify_supplier"
	NeedNoPromisedDate NeedKind = "no_promised_date"
	NeedToSend         NeedKind = "to_send"
)

// NeedView is one purchasing entry of "Precisa de você"; its fields follow
// Kind.
type NeedView struct {
	Kind          NeedKind
	Supplier      shared.SupplierID      // to_send
	SupplierName  string                 // every kind
	Count         int                    // to_send: entries in "A comprar"
	SupplierOrder shared.SupplierOrderID // no_promised_date, notify_supplier
	Number        purchasing.Number      // no_promised_date, notify_supplier
	Days          int                    // no_promised_date: days since it was sent
	Line          shared.PurchaseLineID  // notify_supplier
	ItemName      string                 // notify_supplier
}

// TodayView is Hoje's purchasing entries.
type TodayView struct {
	ToSend int // items in "A comprar"
	// Needs lists notices first, then pedidos de compra with no promised
	// date, then items to send.
	Needs []NeedView
}
