package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
)

// OrdersDeps is what the order handlers need.
type OrdersDeps struct {
	UseCases apporder.UseCases
	Log      *slog.Logger
}

// Orders serves /api/v1/admin/orders. The router puts authn and the
// suspended-store check in front, and the use cases authorize and decide
// who sees a cost: only the owner's view has one (ADR 0025). A handler
// decodes, calls one use case, maps and answers. Nothing here sends a
// message: a wa.me link is only passed on for the user to open.
type Orders struct {
	uc  apporder.UseCases
	log *slog.Logger
}

// NewOrders returns the order handlers, or an error when a dependency is
// missing.
func NewOrders(d OrdersDeps) (*Orders, error) {
	if d.UseCases == nil || d.Log == nil {
		return nil, errors.New("handler: the order handlers need use cases and a logger")
	}
	return &Orders{uc: d.UseCases, log: d.Log}, nil
}

// answer writes err, or status with body() when err is nil. An archived item
// or kit met while pricing is item_unavailable here, where the catalog
// answers item_archived.
func (h *Orders) answer(w http.ResponseWriter, r *http.Request, status int, err error, body func() any) {
	reply(w, r, h.log, status, respond.ItemUnavailable(err), body)
}

// seesCosts reports whether the caller is the owner, who is the only role
// an order answer writes `cost` and `margin` for (ADR 0025). The use case
// has already left them out of every other role's view.
func seesCosts(r *http.Request) bool {
	p, ok := authz.PrincipalFrom(r.Context())
	return ok && identity.SeesCosts(p.Role())
}

// orderID reads {orderId} from the path.
func (h *Orders) orderID(w http.ResponseWriter, r *http.Request) (shared.OrderID, bool) {
	return idParam[shared.OrderID](w, r, h.log, "orderId")
}

// Quote is POST /api/v1/admin/orders/quote: nothing is saved.
func (h *Orders) Quote(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r, h.log, dto.QuoteRequest.Input)
	if !ok {
		return
	}
	v, err := h.uc.Quote(r.Context(), in)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewQuote(v, seesCosts(r)) })
}

// Create is POST /api/v1/admin/orders: 201, also for a replay of the same
// Idempotency-Key and body.
func (h *Orders) Create(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r, h.log, respond.DefaultMaxBodyBytes)
	if !ok {
		return
	}
	key, err := idempotencyKey(r, body)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.CreateOrderRequest) (apporder.CreateInput, error) { return req.Input(key) })
	if !ok {
		return
	}
	v, err := h.uc.Create(r.Context(), in)
	h.answer(w, r, http.StatusCreated, err, func() any { return dto.NewCreatedOrder(v, seesCosts(r)) })
}

// List is GET /api/v1/admin/orders: one page by delivery date.
func (h *Orders) List(w http.ResponseWriter, r *http.Request) {
	f, page, err := readOrderFilter(r)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	p, err := h.uc.List(r.Context(), f, page)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewPage(p, dto.NewOrderSummary) })
}

// Get is GET /api/v1/admin/orders/{orderId}.
func (h *Orders) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	v, err := h.uc.Get(r.Context(), id)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewOrder(v, seesCosts(r)) })
}

// Change is PUT /api/v1/admin/orders/{orderId}.
func (h *Orders) Change(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.ChangeOrderRequest) (apporder.ChangeInput, error) { return req.Input(id) })
	if !ok {
		return
	}
	v, err := h.uc.Change(r.Context(), in)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewChangedOrder(v, seesCosts(r)) })
}

// Advance is POST /api/v1/admin/orders/{orderId}/advance: a guard that does
// not hold is 409 guard_failed, naming it.
func (h *Orders) Advance(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.AdvanceRequest) (apporder.AdvanceInput, error) { return req.Input(id) })
	if !ok {
		return
	}
	v, err := h.uc.Advance(r.Context(), in)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewOrder(v, seesCosts(r)) })
}

// Release is POST /api/v1/admin/orders/{orderId}/release.
func (h *Orders) Release(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.VersionRequest) (apporder.VersionInput, error) {
		v, err := req.Input()
		return apporder.VersionInput{Order: id, Version: v}, err
	})
	if !ok {
		return
	}
	v, err := h.uc.Release(r.Context(), in)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewOrder(v, seesCosts(r)) })
}

// MoveItem is POST /api/v1/admin/orders/{orderId}/items/{itemId}/move.
func (h *Orders) MoveItem(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	item, ok := idParam[shared.OrderItemID](w, r, h.log, "itemId")
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.MoveItemRequest) (apporder.MoveItemInput, error) { return req.Input(id, item) })
	if !ok {
		return
	}
	v, err := h.uc.MoveItem(r.Context(), in)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewOrder(v, seesCosts(r)) })
}

// UseBackupSupplier is POST
// /api/v1/admin/orders/{orderId}/items/{itemId}/use-backup-supplier.
func (h *Orders) UseBackupSupplier(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	item, ok := idParam[shared.OrderItemID](w, r, h.log, "itemId")
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.VersionRequest) (apporder.ItemVersionInput, error) {
		v, err := req.Input()
		return apporder.ItemVersionInput{Order: id, Item: item, Version: v}, err
	})
	if !ok {
		return
	}
	v, err := h.uc.UseBackupSupplier(r.Context(), in)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewOrder(v, seesCosts(r)) })
}

// RecordPayment is POST /api/v1/admin/orders/{orderId}/payments: 201, also
// for a replay of the same Idempotency-Key and body.
func (h *Orders) RecordPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	body, ok := readBody(w, r, h.log, respond.DefaultMaxBodyBytes)
	if !ok {
		return
	}
	key, err := idempotencyKey(r, body)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.PaymentRequest) (apporder.PaymentInput, error) { return req.Input(id, key) })
	if !ok {
		return
	}
	v, err := h.uc.RecordPayment(r.Context(), in)
	h.answer(w, r, http.StatusCreated, err, func() any { return dto.NewOrder(v, seesCosts(r)) })
}

// VoidPayment is POST /api/v1/admin/orders/{orderId}/payments/{paymentId}/void.
func (h *Orders) VoidPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	payment, ok := idParam[shared.PaymentID](w, r, h.log, "paymentId")
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.VoidPaymentRequest) (apporder.VoidPaymentInput, error) { return req.Input(id, payment) })
	if !ok {
		return
	}
	v, err := h.uc.VoidPayment(r.Context(), in)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewOrder(v, seesCosts(r)) })
}

// ChangeDeliveryDate is PUT /api/v1/admin/orders/{orderId}/delivery-date:
// the answer carries a wa.me link, and nothing is sent.
func (h *Orders) ChangeDeliveryDate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.DeliveryDateRequest) (apporder.DateInput, error) { return req.Input(id) })
	if !ok {
		return
	}
	v, err := h.uc.ChangeDeliveryDate(r.Context(), in)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewMessagedOrder(v, seesCosts(r)) })
}

// SendTrackingLink is POST /api/v1/admin/orders/{orderId}/tracking-link.
func (h *Orders) SendTrackingLink(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	v, err := h.uc.SendTrackingLink(r.Context(), id)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewTrackingLink(v) })
}

// Cancel is POST /api/v1/admin/orders/{orderId}/cancel.
func (h *Orders) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.CancelRequest) (apporder.CancelInput, error) { return req.Input(id) })
	if !ok {
		return
	}
	v, err := h.uc.Cancel(r.Context(), in)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewOrder(v, seesCosts(r)) })
}

// History is GET /api/v1/admin/orders/{orderId}/history: newest first.
func (h *Orders) History(w http.ResponseWriter, r *http.Request) {
	id, ok := h.orderID(w, r)
	if !ok {
		return
	}
	page, err := pageRequest(r)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	p, err := h.uc.History(r.Context(), id, page)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewPage(p, dto.NewHistoryEntry) })
}

// readOrderFilter reads the order list's query: the page, status, phase,
// scope, customerId, q and alert. A value the use case owns (a phase, a
// scope) is passed on for it to refuse; one that is not even well formed is
// a 400 here.
func readOrderFilter(r *http.Request) (apporder.ListFilter, shared.PageRequest, error) {
	page, err := pageRequest(r)
	if err != nil {
		return apporder.ListFilter{}, shared.PageRequest{}, err
	}
	q := r.URL.Query()
	f := apporder.ListFilter{
		Status: workflow.StatusKey(q.Get("status")), Phase: workflow.Phase(q.Get("phase")),
		Scope: apporder.ListScope(q.Get("scope")), Query: q.Get("q"),
	}
	if len([]rune(f.Query)) > maxSearchLength {
		return f, page, shared.NewValidationError(shared.FieldError{Field: "q", Message: "Use até 100 caracteres."})
	}
	if raw := q.Get("customerId"); raw != "" {
		id, err := shared.ParseCustomerID(raw)
		if err != nil {
			return f, page, shared.NewValidationError(shared.FieldError{Field: "customerId", Message: "Identificador inválido."})
		}
		f.Customer = id
	}
	switch q.Get("alert") {
	case "", "false":
	case "true":
		f.Alert = true
	default:
		return f, page, shared.NewValidationError(shared.FieldError{Field: "alert", Message: "Use true ou false."})
	}
	return f, page, nil
}
