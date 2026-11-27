package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// CatalogDeps is what the catalog handlers need.
type CatalogDeps struct {
	UseCases appcatalog.UseCases
	// Limiter counts the photo upload requests (ADR 0021).
	Limiter ratelimit.Limiter
	Log     *slog.Logger
}

// Catalog serves /api/v1/admin/catalog/*: categories, item fields, items,
// kits and their photos. The router puts authn with the backoffice audience
// and the suspended-store check in front, and the use cases authorize
// (catalog:read or catalog:write) and decide what the caller's role sees:
// costs and margins come in the views of the owner only (ADR 0025), and a
// cost key sent by any other role is a 403. A handler decodes, calls one
// use case, maps and answers.
type Catalog struct {
	uc      appcatalog.UseCases
	limiter ratelimit.Limiter
	log     *slog.Logger
}

// NewCatalog returns the catalog handlers, or an error when a dependency is
// missing.
func NewCatalog(d CatalogDeps) (*Catalog, error) {
	if d.UseCases == nil || d.Limiter == nil || d.Log == nil {
		return nil, errors.New("handler: the catalog handlers need use cases, a limiter and a logger")
	}
	return &Catalog{uc: d.UseCases, limiter: d.Limiter, log: d.Log}, nil
}

// ListCategories is GET /api/v1/admin/catalog/categories.
func (h *Catalog) ListCategories(w http.ResponseWriter, r *http.Request) {
	v, err := h.uc.ListCategories(r.Context())
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewList(v, dto.NewCategory) })
}

// CreateCategory is POST /api/v1/admin/catalog/categories: 201.
func (h *Catalog) CreateCategory(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r, h.log, dto.CategoryRequest.Input)
	if !ok {
		return
	}
	v, err := h.uc.CreateCategory(r.Context(), in)
	reply(w, r, h.log, http.StatusCreated, err, func() any { return dto.NewCategory(v) })
}

// UpdateCategory is PATCH /api/v1/admin/catalog/categories/{categoryId}.
func (h *Catalog) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.CategoryID](w, r, h.log, "categoryId")
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.UpdateCategoryRequest) (appcatalog.UpdateCategoryInput, error) { return req.Input(id) })
	if !ok {
		return
	}
	v, err := h.uc.UpdateCategory(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewCategory(v) })
}

// DeleteCategory is DELETE /api/v1/admin/catalog/categories/{categoryId}:
// 204, or 409 category_in_use.
func (h *Catalog) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.CategoryID](w, r, h.log, "categoryId")
	if !ok {
		return
	}
	reply(w, r, h.log, http.StatusNoContent, h.uc.DeleteCategory(r.Context(), id), nil)
}

// ListAttributes is GET /api/v1/admin/catalog/attributes.
func (h *Catalog) ListAttributes(w http.ResponseWriter, r *http.Request) {
	v, err := h.uc.ListAttributes(r.Context())
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewList(v, dto.NewAttribute) })
}

// CreateAttribute is POST /api/v1/admin/catalog/attributes: 201.
func (h *Catalog) CreateAttribute(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r, h.log, dto.AttributeRequest.Input)
	if !ok {
		return
	}
	v, err := h.uc.CreateAttribute(r.Context(), in)
	reply(w, r, h.log, http.StatusCreated, err, func() any { return dto.NewAttribute(v) })
}

// ReplaceAttribute is PUT /api/v1/admin/catalog/attributes/{attributeId}.
func (h *Catalog) ReplaceAttribute(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.AttributeID](w, r, h.log, "attributeId")
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, dto.AttributeRequest.Input)
	if !ok {
		return
	}
	v, err := h.uc.ReplaceAttribute(r.Context(), appcatalog.ReplaceAttributeInput{ID: id, Attribute: in})
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewAttribute(v) })
}

// DeleteAttribute is DELETE /api/v1/admin/catalog/attributes/{attributeId}:
// 204.
func (h *Catalog) DeleteAttribute(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.AttributeID](w, r, h.log, "attributeId")
	if !ok {
		return
	}
	reply(w, r, h.log, http.StatusNoContent, h.uc.DeleteAttribute(r.Context(), id), nil)
}

// ListItems is GET /api/v1/admin/catalog/items.
func (h *Catalog) ListItems(w http.ResponseWriter, r *http.Request) {
	q, err := readListQuery(r, true)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	filter := appcatalog.ItemFilter{Query: q.text, IncludeArchived: q.includeArchived}
	if raw := r.URL.Query().Get("categoryId"); raw != "" {
		id, err := shared.ParseID(raw)
		if err != nil {
			respond.Error(w, r, h.log, shared.NewValidationError(shared.FieldError{Field: "categoryId", Message: "Identificador inválido."}))
			return
		}
		filter.Category = shared.CategoryID(id)
	}
	p, err := h.uc.ListItems(r.Context(), filter, q.page)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewPage(p, dto.NewItemSummary) })
}

// CreateItem is POST /api/v1/admin/catalog/items: 201. A cost key from any
// role but the owner is 403 forbidden.
func (h *Catalog) CreateItem(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r, h.log, dto.ItemRequest.Input)
	if !ok {
		return
	}
	v, err := h.uc.CreateItem(r.Context(), in)
	reply(w, r, h.log, http.StatusCreated, err, func() any { return dto.NewItem(v) })
}

// GetItem is GET /api/v1/admin/catalog/items/{itemId}.
func (h *Catalog) GetItem(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.ItemID](w, r, h.log, "itemId")
	if !ok {
		return
	}
	v, err := h.uc.GetItem(r.Context(), id)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewItem(v) })
}

// ReplaceItem is PUT /api/v1/admin/catalog/items/{itemId}: every field and
// the version read; a stale one is 409 version_conflict.
func (h *Catalog) ReplaceItem(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.ItemID](w, r, h.log, "itemId")
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.ReplaceItemRequest) (appcatalog.ReplaceItemInput, error) { return req.Input(id) })
	if !ok {
		return
	}
	v, err := h.uc.ReplaceItem(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewItem(v) })
}

// SetItemPublished is PUT /api/v1/admin/catalog/items/{itemId}/published.
func (h *Catalog) SetItemPublished(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.ItemID](w, r, h.log, "itemId")
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.SetPublishedRequest) (appcatalog.SetItemPublishedInput, error) {
		published, version, err := req.Input()
		return appcatalog.SetItemPublishedInput{ID: id, Published: published, Version: version}, err
	})
	if !ok {
		return
	}
	v, err := h.uc.SetItemPublished(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewItem(v) })
}

// ArchiveItem is POST /api/v1/admin/catalog/items/{itemId}/archive: 409
// item_in_kit, with the kits in references, while a kit holds the item.
func (h *Catalog) ArchiveItem(w http.ResponseWriter, r *http.Request) {
	in, ok := h.itemVersion(w, r)
	if !ok {
		return
	}
	v, err := h.uc.ArchiveItem(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewItem(v) })
}

// RestoreItem is POST /api/v1/admin/catalog/items/{itemId}/restore.
func (h *Catalog) RestoreItem(w http.ResponseWriter, r *http.Request) {
	in, ok := h.itemVersion(w, r)
	if !ok {
		return
	}
	v, err := h.uc.RestoreItem(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewItem(v) })
}

// itemVersion reads the item's id from the path and the version from the
// body, for archive and restore.
func (h *Catalog) itemVersion(w http.ResponseWriter, r *http.Request) (appcatalog.ItemVersionInput, bool) {
	id, ok := idParam[shared.ItemID](w, r, h.log, "itemId")
	if !ok {
		return appcatalog.ItemVersionInput{}, false
	}
	return decode(w, r, h.log, func(req dto.VersionRequest) (appcatalog.ItemVersionInput, error) {
		v, err := req.Input()
		return appcatalog.ItemVersionInput{ID: id, Version: v}, err
	})
}
