package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	apppurchasing "github.com/bergsantana/encomendo/backend/internal/app/purchasing"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// PurchasingDeps is what the purchasing handlers need.
type PurchasingDeps struct {
	UseCases apppurchasing.UseCases
	// Limiter counts the receiving photo upload requests (ADR 0021).
	Limiter ratelimit.Limiter
	Log     *slog.Logger
}

// Purchasing serves /api/v1/admin/purchasing/*: Compras (A comprar,
// Enviadas, the pedido de compra) and Receber itens. The router puts authn
// and the suspended-store check in front, and the use cases authorize
// (purchase:write, receiving:write, order:read) and decide who sees a cost:
// only the owner's view has one (ADR 0031). A handler decodes, calls one use
// case, maps and answers. Nothing here sends a message: a wa.me link is only
// passed on for the user to open (CLAUDE.md, rule 8).
type Purchasing struct {
	uc      apppurchasing.UseCases
	limiter ratelimit.Limiter
	log     *slog.Logger
}

// NewPurchasing returns the purchasing handlers, or an error when a
// dependency is missing.
func NewPurchasing(d PurchasingDeps) (*Purchasing, error) {
	if d.UseCases == nil || d.Limiter == nil || d.Log == nil {
		return nil, errors.New("handler: the purchasing handlers need use cases, a limiter and a logger")
	}
	return &Purchasing{uc: d.UseCases, limiter: d.Limiter, log: d.Log}, nil
}

// supplierOrderID reads {supplierOrderId} from the path.
func (h *Purchasing) supplierOrderID(w http.ResponseWriter, r *http.Request) (shared.SupplierOrderID, bool) {
	return idParam[shared.SupplierOrderID](w, r, h.log, "supplierOrderId")
}

// ToBuy is GET /api/v1/admin/purchasing/to-buy.
func (h *Purchasing) ToBuy(w http.ResponseWriter, r *http.Request) {
	v, err := h.uc.ToBuy(r.Context())
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewToBuy(v, seesCosts(r)) })
}

// PreviewSupplierOrder is POST /api/v1/admin/purchasing/supplier-orders/preview:
// nothing is saved and no number is taken.
func (h *Purchasing) PreviewSupplierOrder(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r, h.log, dto.SupplierOrderRequest.Input)
	if !ok {
		return
	}
	text, err := h.uc.Preview(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.SupplierMessagePreview{Text: text} })
}

// SendSupplierOrder is POST /api/v1/admin/purchasing/supplier-orders: 201,
// also for a replay of the same Idempotency-Key and body. A missing key is
// 400 idempotency_key_required, after the permission check.
func (h *Purchasing) SendSupplierOrder(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeKeyed(w, r, h.log, dto.SupplierOrderRequest.SendInput)
	if !ok {
		return
	}
	v, err := h.uc.Send(r.Context(), in)
	reply(w, r, h.log, http.StatusCreated, err, func() any { return dto.NewSentSupplierOrder(v, seesCosts(r)) })
}

// ListSupplierOrders is GET /api/v1/admin/purchasing/supplier-orders: one
// page, newest first. state is open (the default) or all.
func (h *Purchasing) ListSupplierOrders(w http.ResponseWriter, r *http.Request) {
	open, page, err := readSupplierOrderQuery(r)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	p, err := h.uc.SupplierOrders(r.Context(), open, page)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewPage(p, dto.NewSupplierOrderSummary) })
}

// readSupplierOrderQuery reads the state filter and the page of the list.
func readSupplierOrderQuery(r *http.Request) (open bool, page shared.PageRequest, err error) {
	page, err = pageRequest(r)
	if err != nil {
		return false, page, err
	}
	switch r.URL.Query().Get("state") {
	case "", "open":
		return true, page, nil
	case "all":
		return false, page, nil
	default:
		return false, page, shared.NewValidationError(shared.FieldError{Field: "state", Message: "Use open ou all."})
	}
}

// GetSupplierOrder is GET /api/v1/admin/purchasing/supplier-orders/{supplierOrderId}.
func (h *Purchasing) GetSupplierOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := h.supplierOrderID(w, r)
	if !ok {
		return
	}
	v, err := h.uc.SupplierOrder(r.Context(), id)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewSupplierOrder(v, seesCosts(r)) })
}

// RecordPromisedDate is PUT
// /api/v1/admin/purchasing/supplier-orders/{supplierOrderId}/promised-date.
func (h *Purchasing) RecordPromisedDate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.supplierOrderID(w, r)
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.PromisedDateRequest) (apppurchasing.PromisedDateInput, error) { return req.Input(id) })
	if !ok {
		return
	}
	v, err := h.uc.RecordPromisedDate(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewSupplierOrder(v, seesCosts(r)) })
}

// MarkInTransit is POST
// /api/v1/admin/purchasing/supplier-orders/{supplierOrderId}/in-transit.
func (h *Purchasing) MarkInTransit(w http.ResponseWriter, r *http.Request) {
	h.versioned(w, r, h.uc.MarkInTransit)
}

// UndoSupplierOrder is POST
// /api/v1/admin/purchasing/supplier-orders/{supplierOrderId}/undo: once
// something was received it is 409 supplier_order_received.
func (h *Purchasing) UndoSupplierOrder(w http.ResponseWriter, r *http.Request) {
	h.versioned(w, r, h.uc.Undo)
}

// versioned serves the two calls that take only the version read.
func (h *Purchasing) versioned(
	w http.ResponseWriter, r *http.Request, call func(ctx context.Context, in apppurchasing.VersionInput) (apppurchasing.SupplierOrderView, error),
) {
	id, ok := h.supplierOrderID(w, r)
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.VersionRequest) (apppurchasing.VersionInput, error) {
		return dto.SupplierOrderVersion(id, req)
	})
	if !ok {
		return
	}
	v, err := call(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewSupplierOrder(v, seesCosts(r)) })
}

// RecordPaidPrices is PUT
// /api/v1/admin/purchasing/supplier-orders/{supplierOrderId}/paid-prices:
// the owner's only, 403 for any other role.
func (h *Purchasing) RecordPaidPrices(w http.ResponseWriter, r *http.Request) {
	id, ok := h.supplierOrderID(w, r)
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.PaidPricesRequest) (apppurchasing.PaidPricesInput, error) { return req.Input(id) })
	if !ok {
		return
	}
	v, err := h.uc.RecordPaidPrices(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewSupplierOrder(v, seesCosts(r)) })
}

// ToReceive is GET /api/v1/admin/purchasing/to-receive.
func (h *Purchasing) ToReceive(w http.ResponseWriter, r *http.Request) {
	v, err := h.uc.ToReceive(r.Context())
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewToReceive(v) })
}

// RequestReceiptPhotoUpload is POST
// /api/v1/admin/purchasing/receipt-photo-uploads: 201 with the signed upload.
// It takes the catalog upload limit, per user and per store (ADR 0021), and
// only a caller who may receive counts against it.
func (h *Purchasing) RequestReceiptPhotoUpload(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r, h.log, dto.ReceiptPhotoUploadRequest.Input)
	if !ok {
		return
	}
	if !h.allowPhotoUpload(w, r) {
		return
	}
	v, err := h.uc.RequestPhotoUpload(r.Context(), in)
	reply(w, r, h.log, http.StatusCreated, err, func() any { return dto.NewReceiptPhotoUpload(v) })
}

// allowPhotoUpload counts one upload request under the per-user rule and
// then the per-store rule, as the catalog's does, so a refused user does
// not spend the store's budget. A caller who may not receive is not
// counted: the use case refuses the call.
func (h *Purchasing) allowPhotoUpload(w http.ResponseWriter, r *http.Request) bool {
	p, err := authz.Require(r.Context(), identity.PermReceivingWrite)
	if err != nil {
		return true // the use case refuses the call
	}
	return h.allow(w, r, ratelimit.PhotoUploadByUser, p.UserID().String()) &&
		h.allow(w, r, ratelimit.PhotoUploadByStore, p.TenantID().String())
}

// allow counts one call under rule for value, and answers 429 when the
// rule is used up.
func (h *Purchasing) allow(w http.ResponseWriter, r *http.Request, rule ratelimit.Rule, value string) bool {
	ok, wait := h.limiter.Allow(r.Context(), rule.Key(value), rule.Limit, rule.Window)
	if !ok {
		respond.RateLimited(w, r, wait)
	}
	return ok
}

// RecordReceipt is POST /api/v1/admin/purchasing/receipts: 201, also for a
// replay of the same Idempotency-Key and body.
func (h *Purchasing) RecordReceipt(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeKeyed(w, r, h.log, dto.ReceiptRequest.Input)
	if !ok {
		return
	}
	v, err := h.uc.Receive(r.Context(), in)
	reply(w, r, h.log, http.StatusCreated, err, func() any { return dto.NewReceipt(v) })
}

// ListNotices is GET /api/v1/admin/purchasing/notices.
func (h *Purchasing) ListNotices(w http.ResponseWriter, r *http.Request) {
	v, err := h.uc.Notices(r.Context())
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewList(v, dto.NewNotice) })
}

// ResolveNotice is POST /api/v1/admin/purchasing/notices/{lineId}/resolve:
// 204.
func (h *Purchasing) ResolveNotice(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.PurchaseLineID](w, r, h.log, "lineId")
	if !ok {
		return
	}
	reply(w, r, h.log, http.StatusNoContent, h.uc.ResolveNotice(r.Context(), id), nil)
}

// OrderPurchases is GET /api/v1/admin/purchasing/orders/{orderId}/lines.
func (h *Purchasing) OrderPurchases(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.OrderID](w, r, h.log, "orderId")
	if !ok {
		return
	}
	v, err := h.uc.OrderLines(r.Context(), id)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewOrderPurchases(v) })
}

// Today is GET /api/v1/admin/purchasing/today.
func (h *Purchasing) Today(w http.ResponseWriter, r *http.Request) {
	v, err := h.uc.Today(r.Context())
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewPurchasingToday(v) })
}

// decodeKeyed reads the body and the Idempotency-Key of a creating call,
// then turns them into the use case's input. A missing key comes through
// zero: the use case refuses it after checking the permission, so a role
// that may not make the call is never told about the header (ADR 0030). On
// any failure it has already answered and returns false.
func decodeKeyed[R, I any](
	w http.ResponseWriter, r *http.Request, log *slog.Logger, input func(R, appshared.IdempotencyKey) (I, error),
) (I, bool) {
	var zero I
	body, ok := readBody(w, r, log, respond.DefaultMaxBodyBytes)
	if !ok {
		return zero, false
	}
	key, err := idempotencyKey(r, body)
	if err != nil {
		respond.Error(w, r, log, err)
		return zero, false
	}
	return decode(w, r, log, func(req R) (I, error) { return input(req, key) })
}
