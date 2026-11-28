package dto

import (
	"fmt"

	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// The request types below mirror the contract's order request schemas, with
// the conventions of catalog_requests.go: a required key that is missing is
// reported by name, and values the domain owns (a handover, a payment kind
// or method, an item status) are passed through, so the domain's pt-BR
// message is the answer to a wrong one.

// SwapRequest is the Swap schema.
type SwapRequest struct {
	ItemID     *string `json:"itemId"`
	WithItemID *string `json:"withItemId"`
}

// OptionChoiceRequest is the OptionChoice schema.
type OptionChoiceRequest struct {
	ItemID   *string `json:"itemId"`
	OptionID *string `json:"optionId"`
	ChoiceID *string `json:"choiceId"`
}

// LineRequest is the LineRequest schema: a kit or a loose item. The use
// case checks that exactly one is set and the quantity fits.
type LineRequest struct {
	KitID    *string               `json:"kitId"`
	ItemID   *string               `json:"itemId"`
	Quantity int                   `json:"quantity"`
	Swaps    []SwapRequest         `json:"swaps"`
	Options  []OptionChoiceRequest `json:"options"`
}

func (r LineRequest) input(f *fields, path string) apporder.LineRequest {
	l := apporder.LineRequest{Quantity: r.Quantity}
	if r.KitID != nil {
		l.Kit = parseID[shared.KitID](f, *r.KitID, path+".kitId")
	}
	if r.ItemID != nil {
		l.Item = parseID[shared.ItemID](f, *r.ItemID, path+".itemId")
	}
	for i, s := range r.Swaps {
		sp := fmt.Sprintf("%s.swaps[%d]", path, i)
		l.Swaps = append(l.Swaps, catalog.Swap{
			Item: needID[shared.ItemID](f, s.ItemID, sp+".itemId"),
			With: needID[shared.ItemID](f, s.WithItemID, sp+".withItemId"),
		})
	}
	for i, o := range r.Options {
		op := fmt.Sprintf("%s.options[%d]", path, i)
		l.Options = append(l.Options, apporder.OptionChoice{
			Item:   needID[shared.ItemID](f, o.ItemID, op+".itemId"),
			Option: needID[shared.OptionID](f, o.OptionID, op+".optionId"),
			Choice: needID[shared.ChoiceID](f, o.ChoiceID, op+".choiceId"),
		})
	}
	return l
}

func linesInput(f *fields, lines []LineRequest) []apporder.LineRequest {
	out := make([]apporder.LineRequest, 0, len(lines))
	for i, l := range needList(f, lines, "lines") {
		out = append(out, l.input(f, fmt.Sprintf("lines[%d]", i)))
	}
	return out
}

// TermsRequest is the TermsInput schema. The handover is required; a
// freight left out is zero, also on a change (apporder.TermsInput).
type TermsRequest struct {
	Handover         *string `json:"handover"`
	AddressID        *string `json:"addressId"`
	FreightCents     *int64  `json:"freightCents"`
	DiscountCents    *int64  `json:"discountCents"`
	DepositPercent   *int    `json:"depositPercent"`
	AssemblyDays     *int    `json:"assemblyDays"`
	SafetyMarginDays *int    `json:"safetyMarginDays"`
}

func termsInput(f *fields, r *TermsRequest) apporder.TermsInput {
	if r == nil {
		f.add("terms", msgRequired)
		return apporder.TermsInput{}
	}
	t := apporder.TermsInput{
		Handover:     order.Handover(need(f, r.Handover, "terms.handover")),
		FreightCents: r.FreightCents, DiscountCents: r.DiscountCents, DepositPercent: r.DepositPercent,
		AssemblyDays: r.AssemblyDays, SafetyMarginDays: r.SafetyMarginDays,
	}
	if r.AddressID != nil {
		t.Address = parseID[shared.AddressID](f, *r.AddressID, "terms.addressId")
	}
	return t
}

// QuoteRequest is POST /orders/quote.
type QuoteRequest struct {
	Lines   []LineRequest `json:"lines"`
	Terms   *TermsRequest `json:"terms"`
	OrderID *string       `json:"orderId"`
}

// Input returns the use case's input, or a ValidationError.
func (r QuoteRequest) Input() (apporder.QuoteInput, error) {
	var f fields
	in := apporder.QuoteInput{Lines: linesInput(&f, r.Lines), Terms: termsInput(&f, r.Terms)}
	if r.OrderID != nil {
		in.Order = parseID[shared.OrderID](&f, *r.OrderID, "orderId")
	}
	return in, f.err()
}

// NewCustomerRequest is the NewCustomer schema.
type NewCustomerRequest struct {
	Name  *string `json:"name"`
	Phone *string `json:"phone"`
}

// CreateOrderRequest is POST /orders. The Idempotency-Key is a header the
// handler reads; it is added by WithKey.
type CreateOrderRequest struct {
	CustomerID     *string             `json:"customerId"`
	NewCustomer    *NewCustomerRequest `json:"newCustomer"`
	Lines          []LineRequest       `json:"lines"`
	Terms          *TermsRequest       `json:"terms"`
	SendToCustomer bool                `json:"sendToCustomer"`
}

// Input returns the use case's input with the idempotency key, or a
// ValidationError. Whether exactly one customer is given is the use case's
// to say.
func (r CreateOrderRequest) Input(key appshared.IdempotencyKey) (apporder.CreateInput, error) {
	var f fields
	in := apporder.CreateInput{
		Lines: linesInput(&f, r.Lines), Terms: termsInput(&f, r.Terms),
		SendToCustomer: r.SendToCustomer, Idempotency: key,
	}
	if r.CustomerID != nil {
		in.Customer = parseID[shared.CustomerID](&f, *r.CustomerID, "customerId")
	}
	if c := r.NewCustomer; c != nil {
		in.New = &apporder.NewCustomer{Name: need(&f, c.Name, "newCustomer.name"), Phone: need(&f, c.Phone, "newCustomer.phone")}
	}
	return in, f.err()
}

// ChangeOrderRequest is PUT /orders/{orderId}.
type ChangeOrderRequest struct {
	Lines            []LineRequest `json:"lines"`
	Terms            *TermsRequest `json:"terms"`
	CustomerApproved *bool         `json:"customerApproved"`
	Version          *int          `json:"version"`
}

// Input returns the use case's input for the order id, or a
// ValidationError.
func (r ChangeOrderRequest) Input(id shared.OrderID) (apporder.ChangeInput, error) {
	var f fields
	in := apporder.ChangeInput{
		Order: id, Lines: linesInput(&f, r.Lines), Terms: termsInput(&f, r.Terms),
		CustomerApproved: need(&f, r.CustomerApproved, "customerApproved"), Version: needVersion(&f, r.Version),
	}
	return in, f.err()
}

// AdvanceRequest is POST /orders/{orderId}/advance.
type AdvanceRequest struct {
	To       *string `json:"to"`
	Handover *string `json:"handover"`
	Version  *int    `json:"version"`
}

// Input returns the use case's input for the order id, or a
// ValidationError.
func (r AdvanceRequest) Input(id shared.OrderID) (apporder.AdvanceInput, error) {
	var f fields
	in := apporder.AdvanceInput{
		Order: id, To: workflow.StatusKey(need(&f, r.To, "to")), Version: needVersion(&f, r.Version),
	}
	if r.Handover != nil {
		h := order.Handover(*r.Handover)
		in.Handover = &h
	}
	return in, f.err()
}

// MoveItemRequest is POST /orders/{orderId}/items/{itemId}/move.
type MoveItemRequest struct {
	To      *string `json:"to"`
	Check   *string `json:"check"`
	Version *int    `json:"version"`
}

// Input returns the use case's input for the order item, or a
// ValidationError.
func (r MoveItemRequest) Input(id shared.OrderID, item shared.OrderItemID) (apporder.MoveItemInput, error) {
	var f fields
	in := apporder.MoveItemInput{
		Order: id, Item: item, To: order.ItemStatus(need(&f, r.To, "to")), Version: needVersion(&f, r.Version),
	}
	if r.Check != nil {
		c := order.Check(*r.Check)
		in.Check = &c
	}
	return in, f.err()
}

// PaymentRequest is POST /orders/{orderId}/payments.
type PaymentRequest struct {
	Kind        *string `json:"kind"`
	AmountCents *int64  `json:"amountCents"`
	Method      *string `json:"method"`
	Version     *int    `json:"version"`
}

// Input returns the use case's input with the idempotency key, or a
// ValidationError.
func (r PaymentRequest) Input(id shared.OrderID, key appshared.IdempotencyKey) (apporder.PaymentInput, error) {
	var f fields
	in := apporder.PaymentInput{
		Order: id, Kind: order.PaymentKind(need(&f, r.Kind, "kind")), AmountCents: need(&f, r.AmountCents, "amountCents"),
		Method: order.PaymentMethod(need(&f, r.Method, "method")), Version: needVersion(&f, r.Version), Idempotency: key,
	}
	return in, f.err()
}

// VoidPaymentRequest is POST /orders/{orderId}/payments/{paymentId}/void.
type VoidPaymentRequest struct {
	Reason  *string `json:"reason"`
	Version *int    `json:"version"`
}

// Input returns the use case's input, or a ValidationError.
func (r VoidPaymentRequest) Input(id shared.OrderID, payment shared.PaymentID) (apporder.VoidPaymentInput, error) {
	var f fields
	in := apporder.VoidPaymentInput{
		Order: id, Payment: payment, Reason: need(&f, r.Reason, "reason"), Version: needVersion(&f, r.Version),
	}
	return in, f.err()
}

// DeliveryDateRequest is PUT /orders/{orderId}/delivery-date.
type DeliveryDateRequest struct {
	Date    *string `json:"date"`
	Reason  string  `json:"reason"`
	Version *int    `json:"version"`
}

// Input returns the use case's input, or a ValidationError.
func (r DeliveryDateRequest) Input(id shared.OrderID) (apporder.DateInput, error) {
	var f fields
	in := apporder.DateInput{Order: id, Reason: r.Reason, Version: needVersion(&f, r.Version)}
	if raw := need(&f, r.Date, "date"); r.Date != nil {
		d, err := shared.ParseCivilDate(raw)
		if err != nil {
			f.add("date", "Use uma data no formato AAAA-MM-DD.")
		}
		in.Date = d
	}
	return in, f.err()
}

// CancelRequest is POST /orders/{orderId}/cancel.
type CancelRequest struct {
	Reason  *string `json:"reason"`
	Version *int    `json:"version"`
}

// Input returns the use case's input, or a ValidationError.
func (r CancelRequest) Input(id shared.OrderID) (apporder.CancelInput, error) {
	var f fields
	in := apporder.CancelInput{Order: id, Reason: need(&f, r.Reason, "reason"), Version: needVersion(&f, r.Version)}
	return in, f.err()
}
