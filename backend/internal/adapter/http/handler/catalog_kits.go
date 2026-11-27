package handler

import (
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appcatalog "github.com/bergsantana/encomendo/backend/internal/app/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// ListKits is GET /api/v1/admin/catalog/kits.
func (h *Catalog) ListKits(w http.ResponseWriter, r *http.Request) {
	q, err := readListQuery(r)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	p, err := h.uc.ListKits(r.Context(), appcatalog.KitFilter{Query: q.text, IncludeArchived: q.includeArchived}, q.page)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewPage(p, dto.NewKitSummary) })
}

// CreateKit is POST /api/v1/admin/catalog/kits: 201, or 409 item_archived
// with the archived items in references.
func (h *Catalog) CreateKit(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r, h.log, dto.KitRequest.Input)
	if !ok {
		return
	}
	v, err := h.uc.CreateKit(r.Context(), in)
	reply(w, r, h.log, http.StatusCreated, err, func() any { return dto.NewKit(v) })
}

// GetKit is GET /api/v1/admin/catalog/kits/{kitId}.
func (h *Catalog) GetKit(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.KitID](w, r, h.log, "kitId")
	if !ok {
		return
	}
	v, err := h.uc.GetKit(r.Context(), id)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewKit(v) })
}

// ReplaceKit is PUT /api/v1/admin/catalog/kits/{kitId}.
func (h *Catalog) ReplaceKit(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.KitID](w, r, h.log, "kitId")
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.ReplaceKitRequest) (appcatalog.ReplaceKitInput, error) { return req.Input(id) })
	if !ok {
		return
	}
	v, err := h.uc.ReplaceKit(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewKit(v) })
}

// SetKitPublished is PUT /api/v1/admin/catalog/kits/{kitId}/published.
func (h *Catalog) SetKitPublished(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.KitID](w, r, h.log, "kitId")
	if !ok {
		return
	}
	in, ok := decode(w, r, h.log, func(req dto.SetPublishedRequest) (appcatalog.SetKitPublishedInput, error) {
		published, version, err := req.Input()
		return appcatalog.SetKitPublishedInput{ID: id, Published: published, Version: version}, err
	})
	if !ok {
		return
	}
	v, err := h.uc.SetKitPublished(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewKit(v) })
}

// ArchiveKit is POST /api/v1/admin/catalog/kits/{kitId}/archive.
func (h *Catalog) ArchiveKit(w http.ResponseWriter, r *http.Request) {
	in, ok := h.kitVersion(w, r)
	if !ok {
		return
	}
	v, err := h.uc.ArchiveKit(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewKit(v) })
}

// RestoreKit is POST /api/v1/admin/catalog/kits/{kitId}/restore: 409
// item_archived while an item of the kit is archived.
func (h *Catalog) RestoreKit(w http.ResponseWriter, r *http.Request) {
	in, ok := h.kitVersion(w, r)
	if !ok {
		return
	}
	v, err := h.uc.RestoreKit(r.Context(), in)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewKit(v) })
}

// kitVersion reads the kit's id from the path and the version from the
// body, for archive and restore.
func (h *Catalog) kitVersion(w http.ResponseWriter, r *http.Request) (appcatalog.KitVersionInput, bool) {
	id, ok := idParam[shared.KitID](w, r, h.log, "kitId")
	if !ok {
		return appcatalog.KitVersionInput{}, false
	}
	return decode(w, r, h.log, func(req dto.VersionRequest) (appcatalog.KitVersionInput, error) {
		v, err := req.Input()
		return appcatalog.KitVersionInput{ID: id, Version: v}, err
	})
}
