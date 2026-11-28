package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appcustomer "github.com/bergsantana/encomendo/backend/internal/app/customer"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// CustomersDeps is what the customer handlers need.
type CustomersDeps struct {
	UseCases appcustomer.UseCases
	Log      *slog.Logger
}

// Customers serves /api/v1/admin/customers. The use cases authorize
// (order:read to look, order:write to add, customer:write to replace); a
// customer belongs to one store and never carries another's data.
type Customers struct {
	uc  appcustomer.UseCases
	log *slog.Logger
}

// NewCustomers returns the customer handlers, or an error when a dependency
// is missing.
func NewCustomers(d CustomersDeps) (*Customers, error) {
	if d.UseCases == nil || d.Log == nil {
		return nil, errors.New("handler: the customer handlers need use cases and a logger")
	}
	return &Customers{uc: d.UseCases, log: d.Log}, nil
}

// List is GET /api/v1/admin/customers.
func (h *Customers) List(w http.ResponseWriter, r *http.Request) {
	q, err := readListQuery(r, false)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	p, err := h.uc.List(r.Context(), appcustomer.ListFilter{Query: q.text}, q.page)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewPage(p, dto.NewCustomerSummary) })
}

// Create is POST /api/v1/admin/customers: 201, or 409 phone_taken.
func (h *Customers) Create(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r, h.log, dto.CustomerRequest.Input)
	if !ok {
		return
	}
	v, err := h.uc.Create(r.Context(), in)
	reply(w, r, h.log, http.StatusCreated, err, func() any { return dto.NewCustomer(v) })
}

// Get is GET /api/v1/admin/customers/{customerId}.
func (h *Customers) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.CustomerID](w, r, h.log, "customerId")
	if !ok {
		return
	}
	v, err := h.uc.Get(r.Context(), id)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewCustomer(v) })
}

// Update is PUT /api/v1/admin/customers/{customerId}: a stale version is
// 409 version_conflict.
func (h *Customers) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.CustomerID](w, r, h.log, "customerId")
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.ReplaceCustomerRequest) (appcustomer.UpdateInput, error) { return req.Input(id) })
	if !ok {
		return
	}
	v, err := h.uc.Update(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewCustomer(v) })
}
