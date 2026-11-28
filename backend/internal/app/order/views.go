package order

import (
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/schedule"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// StatusView is a status with its names for staff and clients.
type StatusView struct {
	Key, Label, ClientLabel string
	Phase                   workflow.Phase
}

// WorkflowView is the workflow an order was made with, at its version.
type WorkflowView struct {
	ID       shared.WorkflowID
	Version  int
	Key      string
	Name     string
	Statuses []StatusView
}

// CustomerRef is the order's customer as the order screen shows it.
type CustomerRef struct {
	ID          shared.CustomerID
	Name, Phone string
}

// SupplierView is an item's supplier: staff data, never in a client view.
type SupplierView struct {
	Name     string
	LeadDays int
}

// ItemView is one item of an order, for staff.
type ItemView struct {
	ID           shared.OrderItemID
	Line         shared.OrderLineID
	CatalogItem  shared.ItemID
	Name         string
	SourcingMode catalog.SourcingMode
	Quantity     int
	UnitPrice    shared.Money
	Options      []order.ChosenOption
	Status       order.ItemStatus
	Check        *order.Check
	Expected     shared.CivilDate
	ArrivedOn    *shared.CivilDate
	Arrived      bool
	Alerts       []schedule.Alert
	NextMoves    []order.ItemStatus
	Supplier     *SupplierView
	Backup       *SupplierView // while "Usar o fornecedor reserva" is possible
	Cost         *shared.Money // owner only
}

// LineView is one line of an order.
type LineView struct {
	ID    shared.OrderLineID
	Kit   *order.KitSnapshot
	Price shared.Money
	Items []shared.OrderItemID
}

// PaymentView is a payment with who recorded and voided it.
type PaymentView struct {
	ID         shared.PaymentID
	Kind       order.PaymentKind
	Amount     shared.Money
	Method     order.PaymentMethod
	RecordedBy shared.UserID
	RecordedAt time.Time
	Voided     *VoidView
}

// VoidView is who voided a payment, when and why.
type VoidView struct {
	By     shared.UserID
	At     time.Time
	Reason string
}

// TermsView is the order's terms as the order screen shows them.
type TermsView struct {
	Handover                       order.Handover
	Address                        *order.DeliveryAddress
	DepositPercent                 int
	AssemblyDays, SafetyMarginDays int
}

// View is an order as staff see it. Costs and margins are filled only
// for the owner (identity.SeesCosts); every other role gets nil. It is
// built only by toView, so no write answer can skip that rule (ADR 0030,
// Amendments).
type View struct {
	ID           shared.OrderID
	Number       string
	Customer     CustomerRef
	Status       StatusView
	Workflow     WorkflowView
	Moves        []StatusView
	Lines        []LineView
	Items        []ItemView
	Terms        TermsView
	Subtotal     shared.Money
	Freight      shared.Money
	Discount     shared.Money
	Total        shared.Money
	DepositDue   shared.Money
	Paid         shared.Money
	BalanceDue   shared.Money
	RefundDue    shared.Money
	DepositPaid  bool
	Released     bool
	Payments     []PaymentView
	ReleaseDate  *shared.CivilDate
	DeliveryDate shared.CivilDate
	DateByHand   bool
	ArrivalLimit shared.CivilDate
	Editable     order.Editability
	CancelReason string
	CreatedAt    time.Time
	Cost         *shared.Money   // owner only
	Margin       *catalog.Margin // owner only
	Version      int
}

// toView builds the staff view of o for by, today. It is the only builder
// of View: costs and margins follow identity.SeesCosts(by.Role()).
func toView(o *order.Order, c CustomerFacts, by shared.Principal, today shared.CivilDate) View {
	owner := identity.SeesCosts(by.Role())
	terms := o.Terms()
	v := View{
		ID: o.ID(), Number: o.Number().String(), Customer: CustomerRef{ID: c.ID, Name: c.Name, Phone: c.Phone},
		Status: statusView(o.Status()), Workflow: workflowView(o.Workflow()), Moves: moves(o),
		Terms: TermsView{
			Handover: terms.Handover, Address: terms.Address, DepositPercent: terms.DepositPercent,
			AssemblyDays: o.AssemblyDaysCounted(), SafetyMarginDays: terms.SafetyMarginDays,
		},
		Subtotal: o.Subtotal(), Freight: terms.Freight, Discount: terms.Discount, Total: total(o),
		DepositDue: o.DepositDue(), Paid: o.Paid(), BalanceDue: o.BalanceDue(), RefundDue: o.RefundDue(),
		DepositPaid: o.DepositPaid(), DeliveryDate: o.Promise(), DateByHand: o.PromiseChangedByHand(),
		ArrivalLimit: o.ArrivalLimit(), Editable: o.Editable(), CreatedAt: o.CreatedAt(), Version: o.Version(),
	}
	_, v.Released = o.Released()
	if d, ok := o.ReleaseDate(); ok {
		v.ReleaseDate = &d
	}
	v.CancelReason, _ = o.CancelReason()
	v.Lines = lineViews(o)
	v.Items = itemViews(o, today, owner)
	v.Payments = paymentViews(o)
	if owner {
		if cost, ok := o.Cost(); ok {
			v.Cost = &cost
		}
		if m, ok := o.Margin(); ok {
			v.Margin = &m
		}
	}
	return v
}

func statusView(s workflow.Status) StatusView {
	return StatusView{Key: string(s.Key), Label: s.Label, ClientLabel: s.ClientLabel, Phase: s.Phase}
}

func workflowView(w workflow.Workflow) WorkflowView {
	out := WorkflowView{ID: w.ID(), Version: w.Version(), Key: w.Key(), Name: w.Name()}
	for _, s := range w.Statuses() {
		out.Statuses = append(out.Statuses, statusView(s))
	}
	return out
}

// moves are the statuses Advance accepts now, cancelled left out: cancel
// is its own call.
func moves(o *order.Order) []StatusView {
	var out []StatusView
	for _, s := range o.Workflow().Moves(o.Status().Key) {
		if s.Phase != workflow.PhaseCancelled {
			out = append(out, statusView(s))
		}
	}
	return out
}

func lineViews(o *order.Order) []LineView {
	var out []LineView
	for _, l := range o.Lines() {
		lv := LineView{ID: l.ID(), Price: l.Price()}
		if k, ok := l.Kit(); ok {
			lv.Kit = &k
		}
		for _, i := range l.Items() {
			lv.Items = append(lv.Items, i.ID())
		}
		out = append(out, lv)
	}
	return out
}

func itemViews(o *order.Order, today shared.CivilDate, owner bool) []ItemView {
	alerts := o.Alerts(today)
	var out []ItemView
	for _, i := range o.Items() {
		snap, src := i.Snapshot(), i.Sourcing()
		v := ItemView{
			ID: i.ID(), Line: i.Line(), CatalogItem: snap.CatalogItem, Name: snap.Name, SourcingMode: snap.SourcingMode,
			Quantity: snap.Quantity, UnitPrice: snap.UnitPrice, Options: snap.Options, Status: i.Status(),
			Expected: i.Expected(), Arrived: i.Arrived(), Alerts: alerts[i.ID()], NextMoves: i.NextMoves(),
		}
		if c, ok := i.Check(); ok {
			v.Check = &c
		}
		if d, ok := i.ArrivedOn(); ok {
			v.ArrivedOn = &d
		}
		if !src.Supplier.IsZero() {
			v.Supplier = &SupplierView{Name: src.SupplierName, LeadDays: snap.LeadDays}
		}
		if b := src.Backup; b != nil && i.Status() == order.ItemToBuy && !o.Closed() {
			v.Backup = &SupplierView{Name: b.Name, LeadDays: b.LeadDays}
		}
		if owner {
			v.Cost = src.Cost
		}
		out = append(out, v)
	}
	return out
}

func paymentViews(o *order.Order) []PaymentView {
	var out []PaymentView
	for _, p := range o.Payments() {
		by, at := p.RecordedBy()
		v := PaymentView{ID: p.ID(), Kind: p.Kind(), Amount: p.Amount(), Method: p.Method(), RecordedBy: by, RecordedAt: at}
		if vby, vat, reason, ok := p.Void(); ok {
			v.Voided = &VoidView{By: vby, At: vat, Reason: reason}
		}
		out = append(out, v)
	}
	return out
}
