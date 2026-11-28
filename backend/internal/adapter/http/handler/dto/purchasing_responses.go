package dto

import (
	"time"

	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The purchasing response types mirror the contract's `purchasing` schemas.
// They are staff data: no type here is ever reachable from a client view
// (TestPurchasingDTOs_NotInClientTypes). A cost and a price paid are the
// owner's (ADR 0025, ADR 0031): the use cases leave them nil for every other
// role, and each mapper takes sees, whether the caller is the owner, and
// writes them only then, so a view built wrongly still never leaks one.

// PurchaseChoice is the PurchaseChoice schema.
type PurchaseChoice struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

func newChoices(in []apppurchasing.ChoiceView) []PurchaseChoice {
	out := make([]PurchaseChoice, 0, len(in))
	for _, c := range in {
		out = append(out, PurchaseChoice{Name: c.Name, Label: c.Label})
	}
	return out
}

// dateOrNull is a calendar day, or null for the zero day.
func dateOrNull(d shared.CivilDate) *shared.CivilDate {
	if d.IsZero() {
		return nil
	}
	return &d
}

// ToBuyItem is the ToBuyItem schema.
type ToBuyItem struct {
	OrderID       shared.OrderID     `json:"orderId"`
	OrderNumber   string             `json:"orderNumber"`
	OrderItemID   shared.OrderItemID `json:"orderItemId"`
	CatalogItemID shared.ItemID      `json:"catalogItemId"`
	Name          string             `json:"name"`
	Quantity      int                `json:"quantity"`
	Choices       []PurchaseChoice   `json:"choices"`
	ArrivalLimit  shared.CivilDate   `json:"arrivalLimit"`
	Replacement   bool               `json:"replacement"`
	Cost          *shared.Money      `json:"cost,omitempty"`
}

// ToBuySupplier is the ToBuySupplier schema.
type ToBuySupplier struct {
	SupplierID    shared.SupplierID `json:"supplierId"`
	SupplierName  string            `json:"supplierName"`
	WhatsAppPhone string            `json:"whatsappPhone,omitempty"`
	Items         []ToBuyItem       `json:"items"`
}

// ToBuy is the ToBuy schema: "A comprar".
type ToBuy struct {
	Suppliers []ToBuySupplier `json:"suppliers"`
	Truncated bool            `json:"truncated"`
}

// NewToBuy maps "A comprar".
func NewToBuy(v apppurchasing.ToBuyView, sees bool) ToBuy {
	out := ToBuy{Suppliers: make([]ToBuySupplier, 0, len(v.Suppliers)), Truncated: v.Truncated}
	for _, s := range v.Suppliers {
		row := ToBuySupplier{SupplierID: s.Supplier, SupplierName: s.Name, WhatsAppPhone: s.Phone, Items: make([]ToBuyItem, 0, len(s.Items))}
		for _, i := range s.Items {
			it := ToBuyItem{
				OrderID: i.Order, OrderNumber: i.OrderNumber, OrderItemID: i.OrderItem, CatalogItemID: i.CatalogItem, Name: i.Name,
				Quantity: i.Quantity, Choices: newChoices(i.Choices), ArrivalLimit: i.ArrivalLimit, Replacement: i.Replacement,
			}
			if sees {
				it.Cost = i.Cost
			}
			row.Items = append(row.Items, it)
		}
		out.Suppliers = append(out.Suppliers, row)
	}
	return out
}

// SupplierMessagePreview is the SupplierMessagePreview schema.
type SupplierMessagePreview struct {
	Text string `json:"text"`
}

// PurchaseLine is the PurchaseLine schema.
type PurchaseLine struct {
	ID            shared.PurchaseLineID    `json:"id"`
	OrderID       shared.OrderID           `json:"orderId"`
	OrderNumber   string                   `json:"orderNumber"`
	OrderItemID   *shared.OrderItemID      `json:"orderItemId"`
	CatalogItemID shared.ItemID            `json:"catalogItemId"`
	Name          string                   `json:"name"`
	Quantity      int                      `json:"quantity"`
	Choices       []PurchaseChoice         `json:"choices"`
	Status        purchasing.LineStatus    `json:"status"`
	PromisedDate  *shared.CivilDate        `json:"promisedDate"`
	ExpectedDate  *shared.CivilDate        `json:"expectedDate"`
	ArrivalLimit  *shared.CivilDate        `json:"arrivalLimit"`
	Alerts        []string                 `json:"alerts"`
	Notice        *purchasing.NoticeReason `json:"notice"`
	CheckNote     string                   `json:"checkNote"`
	Cost          *shared.Money            `json:"cost,omitempty"`
	PaidCost      *Owned[shared.Money]     `json:"paidCost,omitempty"`
}

func newPurchaseLine(l apppurchasing.PurchaseLineView, sees bool) PurchaseLine {
	out := PurchaseLine{
		ID: l.ID, OrderID: l.Order, OrderNumber: l.OrderNumber, CatalogItemID: l.CatalogItem, Name: l.Name, Quantity: l.Quantity,
		Choices: newChoices(l.Choices), Status: l.Status, PromisedDate: dateOrNull(l.Promised), ExpectedDate: dateOrNull(l.Expected),
		ArrivalLimit: dateOrNull(l.ArrivalLimit), Alerts: make([]string, 0, len(l.Alerts)), CheckNote: l.CheckNote,
	}
	if !l.OrderItem.IsZero() {
		out.OrderItemID = &l.OrderItem
	}
	if l.Notice != "" {
		out.Notice = &l.Notice
	}
	for _, a := range l.Alerts {
		out.Alerts = append(out.Alerts, string(a))
	}
	if sees {
		out.Cost = l.Cost
		out.PaidCost = Own(l.Paid)
	}
	return out
}

// SupplierOrderSummary is the SupplierOrderSummary schema: a row of
// "Enviadas".
type SupplierOrderSummary struct {
	ID           shared.SupplierOrderID `json:"id"`
	Number       string                 `json:"number"`
	SupplierID   shared.SupplierID      `json:"supplierId"`
	SupplierName string                 `json:"supplierName"`
	SentAt       time.Time              `json:"sentAt"`
	PromisedDate *shared.CivilDate      `json:"promisedDate"`
	Received     int                    `json:"received"`
	Total        int                    `json:"total"`
	State        purchasing.State       `json:"state"`
}

// NewSupplierOrderSummary maps a row of "Enviadas".
func NewSupplierOrderSummary(s apppurchasing.SupplierOrderSummary) SupplierOrderSummary {
	return SupplierOrderSummary{
		ID: s.ID, Number: s.Number.String(), SupplierID: s.Supplier, SupplierName: s.SupplierName, SentAt: s.SentAt.UTC(),
		PromisedDate: dateOrNull(s.Promised), Received: s.Received, Total: s.Total, State: s.State,
	}
}

// SupplierOrder is the SupplierOrder schema: a pedido de compra with its
// lines. The summary's fields sit at the top level, as the contract's allOf
// has them.
type SupplierOrder struct {
	SupplierOrderSummary
	SentBy      Actor          `json:"sentBy"`
	WhatsAppURL string         `json:"whatsappUrl"`
	Lines       []PurchaseLine `json:"lines"`
	CanUndo     bool           `json:"canUndo"`
	Version     int            `json:"version"`
}

// NewSupplierOrder maps a pedido de compra; sees is whether the caller is
// the owner, who alone gets a line's cost and price paid.
func NewSupplierOrder(v apppurchasing.SupplierOrderView, sees bool) SupplierOrder {
	out := SupplierOrder{
		SupplierOrderSummary: NewSupplierOrderSummary(v.SupplierOrderSummary),
		SentBy:               Actor{ID: v.SentBy.ID, Name: v.SentBy.Name}, WhatsAppURL: v.WhatsAppURL,
		Lines: make([]PurchaseLine, 0, len(v.Lines)), CanUndo: v.CanUndo, Version: v.Version,
	}
	for _, l := range v.Lines {
		out.Lines = append(out.Lines, newPurchaseLine(l, sees))
	}
	return out
}

// SentSupplierOrder is the SentSupplierOrder schema.
type SentSupplierOrder struct {
	SupplierOrder SupplierOrder `json:"supplierOrder"`
	WhatsAppURL   string        `json:"whatsappUrl"`
	Text          string        `json:"text"`
}

// NewSentSupplierOrder maps a send.
func NewSentSupplierOrder(v apppurchasing.SentView, sees bool) SentSupplierOrder {
	return SentSupplierOrder{SupplierOrder: NewSupplierOrder(v.SupplierOrder, sees), WhatsAppURL: v.WhatsAppURL, Text: v.Text}
}

// ToReceiveLine is the ToReceiveLine schema.
type ToReceiveLine struct {
	LineID      *shared.PurchaseLineID `json:"lineId,omitempty"`
	OrderID     shared.OrderID         `json:"orderId"`
	OrderNumber string                 `json:"orderNumber"`
	OrderItemID shared.OrderItemID     `json:"orderItemId"`
	Name        string                 `json:"name"`
	Quantity    int                    `json:"quantity"`
	Choices     []PurchaseChoice       `json:"choices"`
}

func newToReceiveLines(in []apppurchasing.ToReceiveLineView) []ToReceiveLine {
	out := make([]ToReceiveLine, 0, len(in))
	for _, l := range in {
		row := ToReceiveLine{
			OrderID: l.Order, OrderNumber: l.OrderNumber, OrderItemID: l.OrderItem, Name: l.Name, Quantity: l.Quantity, Choices: newChoices(l.Choices),
		}
		if !l.Line.IsZero() {
			row.LineID = &l.Line
		}
		out = append(out, row)
	}
	return out
}

// ToReceiveGroup is the ToReceiveGroup schema.
type ToReceiveGroup struct {
	SupplierOrderID shared.SupplierOrderID `json:"supplierOrderId"`
	Number          string                 `json:"number"`
	SupplierName    string                 `json:"supplierName"`
	Lines           []ToReceiveLine        `json:"lines"`
}

// PurchaseToReceive is the ToReceive schema. It has no cost field, for any role.
type PurchaseToReceive struct {
	Groups    []ToReceiveGroup `json:"groups"`
	ToBuy     []ToReceiveLine  `json:"toBuy"`
	Truncated bool             `json:"truncated"`
}

// NewToReceive maps what Receber itens lists.
func NewToReceive(v apppurchasing.ToReceiveView) PurchaseToReceive {
	out := PurchaseToReceive{Groups: make([]ToReceiveGroup, 0, len(v.Groups)), ToBuy: newToReceiveLines(v.ToBuy), Truncated: v.Truncated}
	for _, g := range v.Groups {
		out.Groups = append(out.Groups, ToReceiveGroup{
			SupplierOrderID: g.SupplierOrder, Number: g.Number.String(), SupplierName: g.SupplierName, Lines: newToReceiveLines(g.Lines),
		})
	}
	return out
}

// NewReceiptPhotoUpload maps a signed upload of a receiving photo (the
// PhotoUpload schema). Headers is never null.
func NewReceiptPhotoUpload(u apppurchasing.PhotoUploadView) PhotoUpload {
	headers := u.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	return PhotoUpload{Key: u.Key, UploadURL: u.URL, Method: u.Method, Headers: headers, ExpiresAt: u.ExpiresAt.UTC()}
}

// OrderRef is the OrderRef schema.
type OrderRef struct {
	OrderID     shared.OrderID `json:"orderId"`
	OrderNumber string         `json:"orderNumber"`
}

// ReceiptEntry is one checked line of the Receipt schema.
type ReceiptEntry struct {
	LineID      shared.PurchaseLineID `json:"lineId"`
	OrderID     shared.OrderID        `json:"orderId"`
	OrderNumber string                `json:"orderNumber"`
	OrderItemID shared.OrderItemID    `json:"orderItemId"`
	Name        string                `json:"name"`
	Check       purchasing.Check      `json:"check"`
}

// BackToBuy is an item found defective, back in "A comprar".
type BackToBuy struct {
	OrderItemID shared.OrderItemID `json:"orderItemId"`
	Name        string             `json:"name"`
}

// Receipt is the Receipt schema.
type Receipt struct {
	ID              shared.ReceiptID `json:"id"`
	ReceivedAt      time.Time        `json:"receivedAt"`
	Entries         []ReceiptEntry   `json:"entries"`
	CompletedOrders []OrderRef       `json:"completedOrders"`
	BackToBuy       []BackToBuy      `json:"backToBuy"`
}

// NewReceipt maps a saved receipt. It has no cost field, for any role.
func NewReceipt(v apppurchasing.ReceiptView) Receipt {
	out := Receipt{
		ID: v.ID, ReceivedAt: v.ReceivedAt.UTC(), Entries: make([]ReceiptEntry, 0, len(v.Entries)),
		CompletedOrders: make([]OrderRef, 0, len(v.CompletedOrders)), BackToBuy: make([]BackToBuy, 0, len(v.BackToBuy)),
	}
	for _, e := range v.Entries {
		out.Entries = append(out.Entries, ReceiptEntry{
			LineID: e.Line, OrderID: e.Order, OrderNumber: e.OrderNumber, OrderItemID: e.OrderItem, Name: e.Name, Check: e.Check,
		})
	}
	for _, o := range v.CompletedOrders {
		out.CompletedOrders = append(out.CompletedOrders, OrderRef{OrderID: o.Order, OrderNumber: o.Number})
	}
	for _, b := range v.BackToBuy {
		out.BackToBuy = append(out.BackToBuy, BackToBuy{OrderItemID: b.OrderItem, Name: b.Name})
	}
	return out
}

// Notice is the Notice schema: "Avisar o fornecedor". Its link never names
// the client.
type Notice struct {
	LineID          shared.PurchaseLineID   `json:"lineId"`
	SupplierOrderID shared.SupplierOrderID  `json:"supplierOrderId"`
	Number          string                  `json:"number"`
	SupplierName    string                  `json:"supplierName"`
	OrderID         shared.OrderID          `json:"orderId"`
	OrderNumber     string                  `json:"orderNumber"`
	Name            string                  `json:"name"`
	Quantity        int                     `json:"quantity"`
	Reason          purchasing.NoticeReason `json:"reason"`
	WhatsAppURL     string                  `json:"whatsappUrl"`
}

// NewNotice maps a notice.
func NewNotice(n apppurchasing.NoticeView) Notice {
	return Notice{
		LineID: n.Line, SupplierOrderID: n.SupplierOrder, Number: n.Number.String(), SupplierName: n.SupplierName,
		OrderID: n.Order, OrderNumber: n.OrderNumber, Name: n.Name, Quantity: n.Quantity, Reason: n.Reason, WhatsAppURL: n.WhatsAppURL,
	}
}

// OrderPurchaseToBuy is the OrderPurchaseToBuy schema: the item is in "A
// comprar".
type OrderPurchaseToBuy struct {
	OrderItemID     shared.OrderItemID `json:"orderItemId"`
	SupplierName    string             `json:"supplierName"`
	Status          string             `json:"status"`
	SupplierChatURL string             `json:"supplierChatUrl,omitempty"`
}

// OrderPurchaseLine is the OrderPurchaseLine schema: the item's open line,
// or else its latest.
type OrderPurchaseLine struct {
	OrderItemID     shared.OrderItemID      `json:"orderItemId"`
	SupplierName    string                  `json:"supplierName"`
	Status          string                  `json:"status"`
	LineID          shared.PurchaseLineID   `json:"lineId"`
	SupplierOrderID *shared.SupplierOrderID `json:"supplierOrderId,omitempty"`
	Number          *string                 `json:"number"`
	PromisedDate    *shared.CivilDate       `json:"promisedDate"`
	SupplierChatURL string                  `json:"supplierChatUrl,omitempty"`
}

// NewOrderPurchases maps what purchasing knows of an order's bought items.
// Each entry is the schema its status names.
func NewOrderPurchases(in []apppurchasing.OrderLineView) List[any] {
	out := List[any]{Items: make([]any, 0, len(in))}
	for _, v := range in {
		if v.Status == apppurchasing.OrderLineToBuy {
			out.Items = append(out.Items, OrderPurchaseToBuy{
				OrderItemID: v.OrderItem, SupplierName: v.SupplierName, Status: string(v.Status), SupplierChatURL: v.ChatURL,
			})
			continue
		}
		row := OrderPurchaseLine{
			OrderItemID: v.OrderItem, SupplierName: v.SupplierName, Status: string(v.Status), LineID: v.Line,
			PromisedDate: dateOrNull(v.Promised), SupplierChatURL: v.ChatURL,
		}
		if !v.SupplierOrder.IsZero() {
			row.SupplierOrderID = &v.SupplierOrder
			n := v.Number.String()
			row.Number = &n
		}
		out.Items = append(out.Items, row)
	}
	return out
}

// ToSendNeed is the ToSendNeed schema.
type ToSendNeed struct {
	Kind         apppurchasing.NeedKind `json:"kind"`
	SupplierID   shared.SupplierID      `json:"supplierId"`
	SupplierName string                 `json:"supplierName"`
	Count        int                    `json:"count"`
}

// NoPromisedDateNeed is the NoPromisedDateNeed schema.
type NoPromisedDateNeed struct {
	Kind            apppurchasing.NeedKind `json:"kind"`
	SupplierOrderID shared.SupplierOrderID `json:"supplierOrderId"`
	Number          string                 `json:"number"`
	SupplierName    string                 `json:"supplierName"`
	Days            int                    `json:"days"`
}

// NotifySupplierNeed is the NotifySupplierNeed schema.
type NotifySupplierNeed struct {
	Kind            apppurchasing.NeedKind `json:"kind"`
	LineID          shared.PurchaseLineID  `json:"lineId"`
	SupplierOrderID shared.SupplierOrderID `json:"supplierOrderId"`
	Number          string                 `json:"number"`
	SupplierName    string                 `json:"supplierName"`
	ItemName        string                 `json:"itemName"`
}

// PurchasingToday is the PurchasingToday schema: Hoje's purchasing entries.
type PurchasingToday struct {
	ToSend int   `json:"toSend"`
	Needs  []any `json:"needs"`
}

// NewPurchasingToday maps Hoje's purchasing entries; each need is the
// schema its kind names.
func NewPurchasingToday(v apppurchasing.TodayView) PurchasingToday {
	out := PurchasingToday{ToSend: v.ToSend, Needs: make([]any, 0, len(v.Needs))}
	for _, n := range v.Needs {
		switch n.Kind {
		case apppurchasing.NeedToSend:
			out.Needs = append(out.Needs, ToSendNeed{Kind: n.Kind, SupplierID: n.Supplier, SupplierName: n.SupplierName, Count: n.Count})
		case apppurchasing.NeedNoPromisedDate:
			out.Needs = append(out.Needs, NoPromisedDateNeed{
				Kind: n.Kind, SupplierOrderID: n.SupplierOrder, Number: n.Number.String(), SupplierName: n.SupplierName, Days: n.Days,
			})
		case apppurchasing.NeedNotifySupplier:
			out.Needs = append(out.Needs, NotifySupplierNeed{
				Kind: n.Kind, LineID: n.Line, SupplierOrderID: n.SupplierOrder, Number: n.Number.String(),
				SupplierName: n.SupplierName, ItemName: n.ItemName,
			})
		}
	}
	return out
}

// SupplierItemUse is the SupplierItemUse schema: one item a supplier
// supplies. Cost is the owner's.
type SupplierItemUse struct {
	ItemID   shared.ItemID `json:"itemId"`
	ItemName string        `json:"itemName"`
	Role     string        `json:"role"`
	LeadDays int           `json:"leadDays"`
	Cost     *shared.Money `json:"cost,omitempty"`
}

// NewSupplierItemUses maps the items of a supplier; sees is whether the
// caller is the owner.
func NewSupplierItemUses(in []appcatalog.SupplierItemUseView, sees bool) List[SupplierItemUse] {
	return NewList(in, func(v appcatalog.SupplierItemUseView) SupplierItemUse {
		out := SupplierItemUse{ItemID: v.Item, ItemName: v.Name, Role: string(v.Role), LeadDays: v.LeadDays}
		if sees {
			out.Cost = v.Cost
		}
		return out
	})
}
