package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appsupplier "github.com/bergsantana/encomendo/backend/internal/app/supplier"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// SuppliersDeps is what the supplier handlers need.
type SuppliersDeps struct {
	UseCases appsupplier.UseCases
	Log      *slog.Logger
}

// Suppliers serves /api/v1/admin/suppliers. The use cases authorize
// (catalog:read or catalog:write, ADR 0025); a supplier carries no cost.
type Suppliers struct {
	uc  appsupplier.UseCases
	log *slog.Logger
}

// NewSuppliers returns the supplier handlers, or an error when a
// dependency is missing.
func NewSuppliers(d SuppliersDeps) (*Suppliers, error) {
	if d.UseCases == nil || d.Log == nil {
		return nil, errors.New("handler: the supplier handlers need use cases and a logger")
	}
	return &Suppliers{uc: d.UseCases, log: d.Log}, nil
}

// List is GET /api/v1/admin/suppliers.
func (h *Suppliers) List(w http.ResponseWriter, r *http.Request) {
	q, err := readListQuery(r)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	p, err := h.uc.List(r.Context(), appsupplier.ListFilter{Query: q.text}, q.page)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewPage(p, dto.NewSupplier) })
}

// Create is POST /api/v1/admin/suppliers: 201.
func (h *Suppliers) Create(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r, h.log, dto.SupplierRequest.Input)
	if !ok {
		return
	}
	v, err := h.uc.Create(r.Context(), in)
	reply(w, r, h.log, http.StatusCreated, err, func() any { return dto.NewSupplier(v) })
}

// Get is GET /api/v1/admin/suppliers/{supplierId}.
func (h *Suppliers) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.SupplierID](w, r, h.log, "supplierId")
	if !ok {
		return
	}
	v, err := h.uc.Get(r.Context(), id)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewSupplier(v) })
}

// Replace is PUT /api/v1/admin/suppliers/{supplierId}: a stale version is
// 409 version_conflict.
func (h *Suppliers) Replace(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.SupplierID](w, r, h.log, "supplierId")
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.ReplaceSupplierRequest) (appsupplier.ReplaceInput, error) { return req.Input(id) })
	if !ok {
		return
	}
	v, err := h.uc.Replace(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewSupplier(v) })
}
