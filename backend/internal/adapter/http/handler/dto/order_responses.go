package dto

import (
	"time"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The order response types mirror the contract's staff schemas. Costs and
// margins are the owner's (ADR 0025): the use cases leave them nil for every
// other role, and a mapper takes sees, whether the caller is the owner, to
// write them (null when nothing is recorded) or leave the keys out of the
// JSON altogether. sees only ever adds a null: a role the use case did not
// give a cost has no cost to write.

// ChosenOption is the ChosenOption schema.
type ChosenOption struct {
	OptionID   shared.OptionID `json:"optionId"`
	ChoiceID   shared.ChoiceID `json:"choiceId"`
	Name       string          `json:"name"`
	Label      string          `json:"label"`
	ExtraPrice shared.Money    `json:"extraPrice"`
	ExtraDays  int             `json:"extraDays"`
}

func newChosenOptions(in []order.ChosenOption) []ChosenOption {
	out := make([]ChosenOption, 0, len(in))
	for _, o := range in {
		out = append(out, ChosenOption{
			OptionID: o.Option, ChoiceID: o.Choice, Name: o.Name, Label: o.Label, ExtraPrice: o.ExtraPrice, ExtraDays: o.ExtraDays,
		})
	}
	return out
}

// KitRef is the KitRef schema.
type KitRef struct {
	ID    shared.KitID `json:"id"`
	Name  string       `json:"name"`
	Swaps []KitSwap    `json:"swaps"`
}

// KitSwap is one swap of a kit line, with the item names as they were.
type KitSwap struct {
	RemovedItemID shared.ItemID `json:"removedItemId"`
	RemovedName   string        `json:"removedName"`
	AddedItemID   shared.ItemID `json:"addedItemId"`
	AddedName     string        `json:"addedName"`
}

func newKitRef(k *order.KitSnapshot) *KitRef {
	if k == nil {
		return nil
	}
	out := &KitRef{ID: k.Kit, Name: k.Name, Swaps: make([]KitSwap, 0, len(k.Swaps))}
	for _, s := range k.Swaps {
		out.Swaps = append(out.Swaps, KitSwap{
			RemovedItemID: s.Removed, RemovedName: s.RemovedName, AddedItemID: s.Added, AddedName: s.AddedName,
		})
	}
	return out
}

// OrderLine is the OrderLine schema.
type OrderLine struct {
	ID      shared.OrderLineID   `json:"id"`
	Kit     *KitRef              `json:"kit"`
	Price   shared.Money         `json:"price"`
	ItemIDs []shared.OrderItemID `json:"itemIds"`
}

// ItemSupplier is the ItemSupplier schema: staff data.
type ItemSupplier struct {
	Name     string `json:"name"`
	LeadDays int    `json:"leadDays"`
}

func newItemSupplier(s *apporder.SupplierView) *ItemSupplier {
	if s == nil {
		return nil
	}
	return &ItemSupplier{Name: s.Name, LeadDays: s.LeadDays}
}

// OrderItem is the OrderItem schema.
type OrderItem struct {
	ID             shared.OrderItemID   `json:"id"`
	LineID         shared.OrderLineID   `json:"lineId"`
	CatalogItemID  shared.ItemID        `json:"catalogItemId"`
	Name           string               `json:"name"`
	SourcingMode   catalog.SourcingMode `json:"sourcingMode"`
	Quantity       int                  `json:"quantity"`
	UnitPrice      shared.Money         `json:"unitPrice"`
	Options        []ChosenOption       `json:"options"`
	Status         order.ItemStatus     `json:"status"`
	Check          *order.Check         `json:"check"`
	ExpectedDate   shared.CivilDate     `json:"expectedDate"`
	ArrivedOn      *shared.CivilDate    `json:"arrivedOn"`
	Arrived        bool                 `json:"arrived"`
	Alerts         []string             `json:"alerts"`
	NextMoves      []order.ItemStatus   `json:"nextMoves"`
	Supplier       *ItemSupplier        `json:"supplier,omitempty"`
	BackupSupplier *ItemSupplier        `json:"backupSupplier,omitempty"`
	Cost           *Owned[shared.Money] `json:"cost,omitempty"`
}

func newOrderItem(i apporder.ItemView, sees bool) OrderItem {
	out := OrderItem{
		ID: i.ID, LineID: i.Line, CatalogItemID: i.CatalogItem, Name: i.Name, SourcingMode: i.SourcingMode,
		Quantity: i.Quantity, UnitPrice: i.UnitPrice, Options: newChosenOptions(i.Options), Status: i.Status,
		Check: i.Check, ExpectedDate: i.Expected, ArrivedOn: i.ArrivedOn, Arrived: i.Arrived,
		Alerts: make([]string, 0, len(i.Alerts)), NextMoves: append([]order.ItemStatus{}, i.NextMoves...),
		Supplier: newItemSupplier(i.Supplier), BackupSupplier: newItemSupplier(i.Backup),
	}
	for _, a := range i.Alerts {
		out.Alerts = append(out.Alerts, string(a))
	}
	if sees {
		out.Cost = Own(i.Cost)
	}
	return out
}

// Payment is the Payment schema.
type Payment struct {
	ID         shared.PaymentID    `json:"id"`
	Kind       order.PaymentKind   `json:"kind"`
	Amount     shared.Money        `json:"amount"`
	Method     order.PaymentMethod `json:"method"`
	RecordedBy shared.UserID       `json:"recordedBy"`
	RecordedAt time.Time           `json:"recordedAt"`
	Voided     *PaymentVoid        `json:"voided,omitempty"`
}

// PaymentVoid is who voided a payment, when and why.
type PaymentVoid struct {
	By     shared.UserID `json:"by"`
	At     time.Time     `json:"at"`
	Reason string        `json:"reason"`
}

func newPayments(in []apporder.PaymentView) []Payment {
	out := make([]Payment, 0, len(in))
	for _, p := range in {
		v := Payment{ID: p.ID, Kind: p.Kind, Amount: p.Amount, Method: p.Method, RecordedBy: p.RecordedBy, RecordedAt: p.RecordedAt.UTC()}
		if p.Voided != nil {
			v.Voided = &PaymentVoid{By: p.Voided.By, At: p.Voided.At.UTC(), Reason: p.Voided.Reason}
		}
		out = append(out, v)
	}
	return out
}

// OrderCustomer is the customer of an order, as the order screen shows it.
type OrderCustomer struct {
	ID    shared.CustomerID `json:"id"`
	Name  string            `json:"name"`
	Phone string            `json:"phone"`
}

// Order is the Order schema: an order as staff see it.
type Order struct {
	ID                  shared.OrderID       `json:"id"`
	Number              string               `json:"number"`
	Customer            OrderCustomer        `json:"customer"`
	Status              OrderStatus          `json:"status"`
	Workflow            StatusWorkflow       `json:"workflow"`
	Moves               []OrderStatus        `json:"moves"`
	Lines               []OrderLine          `json:"lines"`
	Items               []OrderItem          `json:"items"`
	Handover            order.Handover       `json:"handover"`
	Address             *Address             `json:"address"`
	Subtotal            shared.Money         `json:"subtotal"`
	Freight             shared.Money         `json:"freight"`
	Discount            shared.Money         `json:"discount"`
	Total               shared.Money         `json:"total"`
	DepositPercent      int                  `json:"depositPercent"`
	DepositDue          shared.Money         `json:"depositDue"`
	DepositPaid         bool                 `json:"depositPaid"`
	Paid                shared.Money         `json:"paid"`
	BalanceDue          shared.Money         `json:"balanceDue"`
	RefundDue           shared.Money         `json:"refundDue"`
	Released            bool                 `json:"released"`
	Payments            []Payment            `json:"payments"`
	ReleaseDate         *shared.CivilDate    `json:"releaseDate"`
	DeliveryDate        shared.CivilDate     `json:"deliveryDate"`
	DeliveryDateChanged bool                 `json:"deliveryDateChanged"`
	ArrivalLimit        shared.CivilDate     `json:"arrivalLimit"`
	AssemblyDays        int                  `json:"assemblyDays"`
	SafetyMarginDays    int                  `json:"safetyMarginDays"`
	Editable            order.Editability    `json:"editable"`
	CancelReason        string               `json:"cancelReason,omitempty"`
	CreatedAt           time.Time            `json:"createdAt"`
	Cost                *Owned[shared.Money] `json:"cost,omitempty"`
	Margin              *Owned[Margin]       `json:"margin,omitempty"`
	Version             int                  `json:"version"`
}

// NewOrder maps an order for a caller; sees is whether the caller is the
// owner, the only role that sees costs and margins.
func NewOrder(v apporder.View, sees bool) Order {
	out := Order{
		ID: v.ID, Number: v.Number, Customer: OrderCustomer{ID: v.Customer.ID, Name: v.Customer.Name, Phone: v.Customer.Phone},
		Status:   orderStatus(v.Status.Key, v.Status.Label, v.Status.ClientLabel, v.Status.Phase),
		Workflow: newOrderWorkflow(v.Workflow), Moves: newStatusViews(v.Moves),
		Handover: v.Terms.Handover, Address: newOrderAddress(v.Terms.Address),
		Subtotal: v.Subtotal, Freight: v.Freight, Discount: v.Discount, Total: v.Total,
		DepositPercent: v.Terms.DepositPercent, DepositDue: v.DepositDue, DepositPaid: v.DepositPaid,
		Paid: v.Paid, BalanceDue: v.BalanceDue, RefundDue: v.RefundDue, Released: v.Released,
		Payments: newPayments(v.Payments), ReleaseDate: v.ReleaseDate, DeliveryDate: v.DeliveryDate,
		DeliveryDateChanged: v.DateByHand, ArrivalLimit: v.ArrivalLimit,
		AssemblyDays: v.Terms.AssemblyDays, SafetyMarginDays: v.Terms.SafetyMarginDays,
		Editable: v.Editable, CancelReason: v.CancelReason, CreatedAt: v.CreatedAt.UTC(), Version: v.Version,
		Lines: make([]OrderLine, 0, len(v.Lines)), Items: make([]OrderItem, 0, len(v.Items)),
	}
	for _, l := range v.Lines {
		ids := append([]shared.OrderItemID{}, l.Items...)
		out.Lines = append(out.Lines, OrderLine{ID: l.ID, Kit: newKitRef(l.Kit), Price: l.Price, ItemIDs: ids})
	}
	for _, i := range v.Items {
		out.Items = append(out.Items, newOrderItem(i, sees))
	}
	if sees {
		out.Cost = Own(v.Cost)
		out.Margin = ownedMargin(v.Margin)
	}
	return out
}

func newStatusViews(in []apporder.StatusView) []OrderStatus {
	out := make([]OrderStatus, 0, len(in))
	for _, s := range in {
		out = append(out, orderStatus(s.Key, s.Label, s.ClientLabel, s.Phase))
	}
	return out
}

func newOrderWorkflow(w apporder.WorkflowView) StatusWorkflow {
	return StatusWorkflow{ID: w.ID, Version: w.Version, Key: w.Key, Name: w.Name, Statuses: newStatusViews(w.Statuses)}
}

// CreatedOrder is the CreatedOrder schema. The tracking link is in the
// answer to the call that made the order only.
type CreatedOrder struct {
	Order       Order  `json:"order"`
	TrackingURL string `json:"trackingUrl,omitempty"`
	WhatsAppURL string `json:"whatsappUrl,omitempty"`
}

// NewCreatedOrder maps the answer of POST /orders.
func NewCreatedOrder(v apporder.CreatedView, sees bool) CreatedOrder {
	return CreatedOrder{Order: NewOrder(v.Order, sees), TrackingURL: v.TrackingURL, WhatsAppURL: v.WhatsAppURL}
}

// OrderChange is the OrderChange schema.
type OrderChange struct {
	Paid            bool             `json:"paid"`
	OldTotal        shared.Money     `json:"oldTotal"`
	NewTotal        shared.Money     `json:"newTotal"`
	OldDeliveryDate shared.CivilDate `json:"oldDeliveryDate"`
	NewDeliveryDate shared.CivilDate `json:"newDeliveryDate"`
	RefundDue       shared.Money     `json:"refundDue"`
}

// ChangedOrder is the ChangedOrder schema.
type ChangedOrder struct {
	Order       Order       `json:"order"`
	Change      OrderChange `json:"change"`
	WhatsAppURL string      `json:"whatsappUrl,omitempty"`
}

// NewChangedOrder maps the answer of PUT /orders/{orderId}.
func NewChangedOrder(v apporder.ChangedView, sees bool) ChangedOrder {
	c := v.Summary
	return ChangedOrder{
		Order: NewOrder(v.Order, sees), WhatsAppURL: v.WhatsAppURL,
		Change: OrderChange{
			Paid: c.Paid, OldTotal: c.OldTotal, NewTotal: c.NewTotal,
			OldDeliveryDate: c.OldPromise, NewDeliveryDate: c.NewPromise, RefundDue: c.RefundDue,
		},
	}
}

// MessagedOrder is the MessagedOrder schema.
type MessagedOrder struct {
	Order       Order  `json:"order"`
	WhatsAppURL string `json:"whatsappUrl"`
}

// NewMessagedOrder maps the answer of PUT .../delivery-date.
func NewMessagedOrder(v apporder.MessagedView, sees bool) MessagedOrder {
	return MessagedOrder{Order: NewOrder(v.Order, sees), WhatsAppURL: v.WhatsAppURL}
}

// TrackingLink is the TrackingLink schema.
type TrackingLink struct {
	TrackingURL string `json:"trackingUrl"`
	WhatsAppURL string `json:"whatsappUrl"`
}

// NewTrackingLink maps the answer of POST .../tracking-link.
func NewTrackingLink(v apporder.LinkView) TrackingLink {
	return TrackingLink{TrackingURL: v.TrackingURL, WhatsAppURL: v.WhatsAppURL}
}

// Quote is the Quote schema: prices, deposit and dates, computed by the
// server.
type Quote struct {
	Lines            []QuoteLine      `json:"lines"`
	Subtotal         shared.Money     `json:"subtotal"`
	Freight          shared.Money     `json:"freight"`
	Discount         shared.Money     `json:"discount"`
	Total            shared.Money     `json:"total"`
	DepositPercent   int              `json:"depositPercent"`
	DepositDue       shared.Money     `json:"depositDue"`
	AssemblyDays     int              `json:"assemblyDays"`
	SafetyMarginDays int              `json:"safetyMarginDays"`
	DeliveryDate     shared.CivilDate `json:"deliveryDate"`
	ArrivalLimit     shared.CivilDate `json:"arrivalLimit"`
}

// QuoteLine is one line of a quote.
type QuoteLine struct {
	Kit   *KitRef      `json:"kit"`
	Price shared.Money `json:"price"`
	Items []QuoteItem  `json:"items"`
}

// QuoteItem is one item of a quote, with its expected date.
type QuoteItem struct {
	CatalogItemID shared.ItemID        `json:"catalogItemId"`
	Name          string               `json:"name"`
	SourcingMode  catalog.SourcingMode `json:"sourcingMode"`
	Quantity      int                  `json:"quantity"`
	UnitPrice     shared.Money         `json:"unitPrice"`
	Options       []ChosenOption       `json:"options"`
	ExpectedDate  shared.CivilDate     `json:"expectedDate"`
}

// NewQuote maps a quote.
func NewQuote(v apporder.QuoteView) Quote {
	out := Quote{
		Lines: make([]QuoteLine, 0, len(v.Lines)), Subtotal: v.Subtotal, Freight: v.Freight, Discount: v.Discount,
		Total: v.Total, DepositPercent: v.DepositPercent, DepositDue: v.DepositDue, AssemblyDays: v.AssemblyDays,
		SafetyMarginDays: v.SafetyMarginDays, DeliveryDate: v.DeliveryDate, ArrivalLimit: v.ArrivalLimit,
	}
	for _, l := range v.Lines {
		ql := QuoteLine{Kit: newKitRef(l.Kit), Price: l.Price, Items: make([]QuoteItem, 0, len(l.Items))}
		for _, i := range l.Items {
			ql.Items = append(ql.Items, QuoteItem{
				CatalogItemID: i.CatalogItem, Name: i.Name, SourcingMode: i.SourcingMode, Quantity: i.Quantity,
				UnitPrice: i.UnitPrice, Options: newChosenOptions(i.Options), ExpectedDate: i.Expected,
			})
		}
		out.Lines = append(out.Lines, ql)
	}
	return out
}

// OrderSummary is the OrderSummary schema: one order as a list line shows
// it. It has no cost or margin to carry, for any role.
type OrderSummary struct {
	ID           shared.OrderID   `json:"id"`
	Number       string           `json:"number"`
	Customer     SummaryCustomer  `json:"customer"`
	Status       OrderStatus      `json:"status"`
	ItemsReady   int              `json:"itemsReady"`
	ItemsTotal   int              `json:"itemsTotal"`
	DeliveryDate shared.CivilDate `json:"deliveryDate"`
	Total        shared.Money     `json:"total"`
	BalanceDue   shared.Money     `json:"balanceDue"`
	Alerts       []Alert          `json:"alerts"`
}

// SummaryCustomer is the customer of a list line: no phone.
type SummaryCustomer struct {
	ID   shared.CustomerID `json:"id"`
	Name string            `json:"name"`
}

// Alert is the Alert schema.
type Alert struct {
	Kind   string             `json:"kind"`
	ItemID shared.OrderItemID `json:"itemId"`
}

// NewOrderSummary maps a list line.
func NewOrderSummary(s apporder.SummaryView) OrderSummary {
	out := OrderSummary{
		ID: s.ID, Number: s.Number, Customer: SummaryCustomer{ID: s.Customer.ID, Name: s.Customer.Name},
		Status:     orderStatus(s.Status.Key, s.Status.Label, s.Status.ClientLabel, s.Status.Phase),
		ItemsReady: s.ItemsReady, ItemsTotal: s.ItemsTotal, DeliveryDate: s.DeliveryDate,
		Total: s.Total, BalanceDue: s.BalanceDue, Alerts: make([]Alert, 0, len(s.Alerts)),
	}
	for _, a := range s.Alerts {
		out.Alerts = append(out.Alerts, Alert{Kind: string(a.Kind), ItemID: a.Item})
	}
	return out
}

// Actor is the Actor schema.
type Actor struct {
	ID   shared.UserID `json:"id"`
	Name string        `json:"name"`
}

// HistoryEntry is the HistoryEntry schema: only the fields its kind uses
// are written.
type HistoryEntry struct {
	Kind             order.HistoryKind   `json:"kind"`
	At               time.Time           `json:"at"`
	By               *Actor              `json:"by"`
	From             *OrderStatus        `json:"from,omitempty"`
	To               *OrderStatus        `json:"to,omitempty"`
	Handover         order.Handover      `json:"handover,omitempty"`
	ItemID           *shared.OrderItemID `json:"itemId,omitempty"`
	ItemName         string              `json:"itemName,omitempty"`
	ItemFrom         order.ItemStatus    `json:"itemFrom,omitempty"`
	ItemTo           order.ItemStatus    `json:"itemTo,omitempty"`
	Check            order.Check         `json:"check,omitempty"`
	PaymentID        *shared.PaymentID   `json:"paymentId,omitempty"`
	PaymentKind      order.PaymentKind   `json:"paymentKind,omitempty"`
	PaymentMethod    order.PaymentMethod `json:"paymentMethod,omitempty"`
	Amount           *shared.Money       `json:"amount,omitempty"`
	OldDate          *shared.CivilDate   `json:"oldDate,omitempty"`
	NewDate          *shared.CivilDate   `json:"newDate,omitempty"`
	Reason           string              `json:"reason,omitempty"`
	CustomerApproved bool                `json:"customerApproved,omitempty"`
}

// NewHistoryEntry maps one event of an order's history.
func NewHistoryEntry(h apporder.HistoryView) HistoryEntry {
	out := HistoryEntry{
		Kind: h.Kind, At: h.At.UTC(), Handover: h.Handover, ItemName: h.ItemName, ItemFrom: h.ItemFrom, ItemTo: h.ItemTo,
		Check: h.Check, PaymentKind: h.PaymentKind, PaymentMethod: h.PaymentMethod, Amount: h.Amount,
		OldDate: datePtr(h.OldDate), NewDate: datePtr(h.NewDate), Reason: h.Reason, CustomerApproved: h.CustomerApproved,
	}
	if h.By != nil {
		out.By = &Actor{ID: h.By.ID, Name: h.By.Name}
	}
	if h.From != nil {
		s := orderStatus(h.From.Key, h.From.Label, h.From.ClientLabel, h.From.Phase)
		out.From = &s
	}
	if h.To != nil {
		s := orderStatus(h.To.Key, h.To.Label, h.To.ClientLabel, h.To.Phase)
		out.To = &s
	}
	if !h.Item.IsZero() {
		out.ItemID = &h.Item
	}
	if !h.Payment.IsZero() {
		out.PaymentID = &h.Payment
	}
	return out
}

// datePtr is d, or nil for no date.
func datePtr(d shared.CivilDate) *shared.CivilDate {
	if d.IsZero() {
		return nil
	}
	return &d
}
