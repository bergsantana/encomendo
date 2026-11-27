package handler

import (
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/catalog"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// RequestPhotoUpload is POST /api/v1/admin/catalog/photo-uploads: 201 with
// the signed upload. Each call from a caller who may write the catalog counts
// against the user's and the store's limits, because every URL is reusable
// for its whole life and storage is billed (ADR 0021, amended in step
// 3.10). A caller who may not write is not counted: the use case refuses
// the call, and counting it would let a viewer spend the store's budget.
func (h *Catalog) RequestPhotoUpload(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r, h.log, dto.PhotoUploadRequest.Input)
	if !ok {
		return
	}
	if !h.allowPhotoUpload(w, r) {
		return
	}
	v, err := h.uc.RequestPhotoUpload(r.Context(), in)
	reply(w, r, h.log, http.StatusCreated, err, func() any { return dto.NewPhotoUpload(v) })
}

// allowPhotoUpload counts one upload request under the per-user rule and
// then the per-store rule, so a refused user does not spend the store's
// budget, and answers 429 when either is used up.
func (h *Catalog) allowPhotoUpload(w http.ResponseWriter, r *http.Request) bool {
	p, err := authz.Require(r.Context(), identity.PermCatalogWrite)
	if err != nil {
		return true // the use case refuses the call
	}
	return h.allow(w, r, ratelimit.PhotoUploadByUser, p.UserID().String()) &&
		h.allow(w, r, ratelimit.PhotoUploadByStore, p.TenantID().String())
}

// allow counts one call under rule for value, and answers 429 when the
// rule is used up.
func (h *Catalog) allow(w http.ResponseWriter, r *http.Request, rule ratelimit.Rule, value string) bool {
	ok, wait := h.limiter.Allow(r.Context(), rule.Key(value), rule.Limit, rule.Window)
	if !ok {
		respond.RateLimited(w, r, wait)
	}
	return ok
}

// ConfirmItemPhoto is POST /api/v1/admin/catalog/items/{itemId}/photos.
func (h *Catalog) ConfirmItemPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.ItemID](w, r, h.log, "itemId")
	if !ok {
		return
	}
	h.confirmPhoto(w, r, catalog.ItemPhotos(id))
}

// ConfirmKitPhoto is POST /api/v1/admin/catalog/kits/{kitId}/photos.
func (h *Catalog) ConfirmKitPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.KitID](w, r, h.log, "kitId")
	if !ok {
		return
	}
	h.confirmPhoto(w, r, catalog.KitPhotos(id))
}

// confirmPhoto records the photo uploaded to the key: 201 for a new photo,
// 200 with the same photo when the key was already confirmed on this owner.
func (h *Catalog) confirmPhoto(w http.ResponseWriter, r *http.Request, owner catalog.PhotoOwner) {
	key, ok := decode(w, r, h.log, dto.ConfirmPhotoRequest.Input)
	if !ok {
		return
	}
	c, err := h.uc.ConfirmPhoto(r.Context(), owner, key)
	status := http.StatusCreated
	if c.AlreadyRecorded {
		status = http.StatusOK
	}
	reply(w, r, h.log, status, err, func() any { return dto.NewPhoto(c.Photo) })
}

// OrderItemPhotos is PUT /api/v1/admin/catalog/items/{itemId}/photos/order.
func (h *Catalog) OrderItemPhotos(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.ItemID](w, r, h.log, "itemId")
	if !ok {
		return
	}
	h.orderPhotos(w, r, catalog.ItemPhotos(id))
}

// OrderKitPhotos is PUT /api/v1/admin/catalog/kits/{kitId}/photos/order.
func (h *Catalog) OrderKitPhotos(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.KitID](w, r, h.log, "kitId")
	if !ok {
		return
	}
	h.orderPhotos(w, r, catalog.KitPhotos(id))
}

func (h *Catalog) orderPhotos(w http.ResponseWriter, r *http.Request, owner catalog.PhotoOwner) {
	ids, ok := decode(w, r, h.log, dto.OrderPhotosRequest.Input)
	if !ok {
		return
	}
	v, err := h.uc.OrderPhotos(r.Context(), owner, ids)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewPhotoList(v) })
}

// DeleteItemPhoto is DELETE
// /api/v1/admin/catalog/items/{itemId}/photos/{photoId}: 204.
func (h *Catalog) DeleteItemPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.ItemID](w, r, h.log, "itemId")
	if !ok {
		return
	}
	h.deletePhoto(w, r, catalog.ItemPhotos(id))
}

// DeleteKitPhoto is DELETE
// /api/v1/admin/catalog/kits/{kitId}/photos/{photoId}: 204.
func (h *Catalog) DeleteKitPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam[shared.KitID](w, r, h.log, "kitId")
	if !ok {
		return
	}
	h.deletePhoto(w, r, catalog.KitPhotos(id))
}

func (h *Catalog) deletePhoto(w http.ResponseWriter, r *http.Request, owner catalog.PhotoOwner) {
	photo, ok := idParam[shared.PhotoID](w, r, h.log, "photoId")
	if !ok {
		return
	}
	reply(w, r, h.log, http.StatusNoContent, h.uc.DeletePhoto(r.Context(), owner, photo), nil)
}
