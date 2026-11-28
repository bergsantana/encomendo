package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// TenancyDeps is what the store and team handlers need.
type TenancyDeps struct {
	UseCases   apptenancy.UseCases
	Limiter    ratelimit.Limiter
	Pseudonyms ratelimit.Pseudonyms
	Log        *slog.Logger
}

// Tenancy serves /api/v1/admin/{tenants,settings,members,invitations}. The
// router puts authn with the backoffice audience in front, so every call
// has a principal; the use cases authorize, a store-less session getting
// 403 tenant_required everywhere but creating a store.
type Tenancy struct {
	uc         apptenancy.UseCases
	limiter    ratelimit.Limiter
	pseudonyms ratelimit.Pseudonyms
	log        *slog.Logger
}

// NewTenancy returns the store and team handlers, or an error when a
// dependency is missing.
func NewTenancy(d TenancyDeps) (*Tenancy, error) {
	if d.UseCases == nil || d.Limiter == nil || d.Log == nil || d.Pseudonyms.IsZero() {
		return nil, errors.New("handler: the tenancy handlers need use cases, a limiter, pseudonyms and a logger")
	}
	return &Tenancy{uc: d.UseCases, limiter: d.Limiter, pseudonyms: d.Pseudonyms, log: d.Log}, nil
}

// CreateTenant is POST /api/v1/admin/tenants: 201 with the store. Each
// attempt counts against the user's daily cap, a taken address included,
// because the handler cannot know the address is taken before trying
// (ADR 0021, amended in step 2.4).
func (h *Tenancy) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var in dto.CreateTenantRequest
	if err := respond.Decode(w, r, &in, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	if p, ok := authz.PrincipalFrom(r.Context()); ok && !h.allow(w, r, ratelimit.CreateTenantByUser, p.UserID().String()) {
		return
	}
	t, err := h.uc.CreateTenant(r.Context(), apptenancy.CreateTenantInput{Name: in.Name, Slug: in.Slug})
	h.answer(w, r, http.StatusCreated, err, func() any { return dto.NewTenant(t) })
}

// Settings is GET /api/v1/admin/settings.
func (h *Tenancy) Settings(w http.ResponseWriter, r *http.Request) {
	s, err := h.uc.Settings(r.Context())
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewTenantSettings(s) })
}

// UpdateSettings is PUT /api/v1/admin/settings: every field and the version
// read; a stale version is 409 version_conflict.
func (h *Tenancy) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var in dto.UpdateSettingsRequest
	if err := respond.Decode(w, r, &in, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	input, err := in.Input()
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	s, err := h.uc.UpdateSettings(r.Context(), input)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewTenantSettings(s) })
}

// ChooseWorkflow is PUT /api/v1/admin/settings/status-workflow: new orders
// take the chosen preset and orders already made keep theirs (ADR 0004). An
// unknown preset is 404 workflow_not_found and a stale settings version is
// 409 version_conflict.
func (h *Tenancy) ChooseWorkflow(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r, h.log, dto.ChooseWorkflowRequest.Input)
	if !ok {
		return
	}
	s, err := h.uc.ChooseStatusWorkflow(r.Context(), in)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewTenantSettings(s) })
}

// Members is GET /api/v1/admin/members: one page, oldest member first. A
// cursor the server did not issue is 400.
func (h *Tenancy) Members(w http.ResponseWriter, r *http.Request) {
	page, err := pageRequest(r)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	p, err := h.uc.Members(r.Context(), page)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewPage(p, dto.NewMember) })
}

// RemoveMember is DELETE /api/v1/admin/members/{userId}: 204.
func (h *Tenancy) RemoveMember(w http.ResponseWriter, r *http.Request) {
	user, err := pathID(r, "userId")
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	h.answer(w, r, http.StatusNoContent, h.uc.RemoveMember(r.Context(), shared.UserID(user)), nil)
}

// ChangeRole is PUT /api/v1/admin/members/{userId}/role.
func (h *Tenancy) ChangeRole(w http.ResponseWriter, r *http.Request) {
	user, err := pathID(r, "userId")
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	var in dto.ChangeRoleRequest
	if err := respond.Decode(w, r, &in, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	m, err := h.uc.ChangeRole(r.Context(), apptenancy.ChangeRoleInput{UserID: shared.UserID(user), Role: in.Role})
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewMember(m) })
}

// Invitations is GET /api/v1/admin/invitations: pending ones, newest first.
func (h *Tenancy) Invitations(w http.ResponseWriter, r *http.Request) {
	page, err := pageRequest(r)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	p, err := h.uc.Invitations(r.Context(), page)
	h.answer(w, r, http.StatusOK, err, func() any { return dto.NewPage(p, dto.NewInvitation) })
}

// InviteMember is POST /api/v1/admin/invitations: 201 with the invitation,
// never its link. The daily caps of ADR 0021 count only calls that could
// send mail: from a caller whose token may manage the team, with an email
// and a role the use case accepts. The use case checks the current role
// again; counting a viewer's refused calls, or a typo, would spend the
// store's budget for nothing.
func (h *Tenancy) InviteMember(w http.ResponseWriter, r *http.Request) {
	var in dto.InviteMemberRequest
	if err := respond.Decode(w, r, &in, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	if !h.allowInvite(w, r, in) {
		return
	}
	inv, err := h.uc.InviteMember(r.Context(), apptenancy.InviteMemberInput{Email: in.Email, Role: in.Role})
	h.answer(w, r, http.StatusCreated, err, func() any { return dto.NewInvitation(inv) })
}

// RevokeInvitation is DELETE /api/v1/admin/invitations/{invitationId}: 204.
func (h *Tenancy) RevokeInvitation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "invitationId")
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	h.answer(w, r, http.StatusNoContent, h.uc.RevokeInvitation(r.Context(), shared.InvitationID(id)), nil)
}

// allowInvite applies the three invitation caps: per inviting user, per
// store and email, and per store, so a refused per-user count stops before
// the store's budget is touched.
func (h *Tenancy) allowInvite(w http.ResponseWriter, r *http.Request, in dto.InviteMemberRequest) bool {
	p, err := authz.Require(r.Context(), identity.PermMembersManage)
	if err != nil {
		return true // the use case refuses the call
	}
	email, err := identity.ParseEmail(in.Email)
	if err != nil {
		return true // the use case answers 400 and sends nothing
	}
	if _, err := tenancy.ParseInvitableRole(in.Role); err != nil {
		return true
	}
	return h.allow(w, r, ratelimit.InviteByUser, p.UserID().String()) &&
		h.allow(w, r, ratelimit.InviteByStoreEmail, h.pseudonyms.StoreEmail(p.TenantID(), email)) &&
		h.allow(w, r, ratelimit.InviteByStore, p.TenantID().String())
}

// allow counts one call under rule for value, and answers 429 when the
// rule is used up.
func (h *Tenancy) allow(w http.ResponseWriter, r *http.Request, rule ratelimit.Rule, value string) bool {
	ok, wait := h.limiter.Allow(r.Context(), rule.Key(value), rule.Limit, rule.Window)
	if !ok {
		respond.RateLimited(w, r, wait)
	}
	return ok
}

// answer writes err, or status with body() when body is not nil.
func (h *Tenancy) answer(w http.ResponseWriter, r *http.Request, status int, err error, body func() any) {
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	if body == nil {
		w.WriteHeader(status)
		return
	}
	respond.JSON(w, r, h.log, status, body())
}

// pageRequest reads ?cursor= and ?limit=. A limit that is not a number is
// a ValidationError, like one out of range.
func pageRequest(r *http.Request) (shared.PageRequest, error) {
	q := r.URL.Query()
	limit := 0
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			return shared.PageRequest{}, shared.NewValidationError(shared.FieldError{Field: "limit", Message: "Informe um número."})
		}
		limit = n
	}
	return shared.NewPageRequest(q.Get("cursor"), limit)
}

// pathID reads an id from the path. One that is not an id names nothing,
// so it is 404 not_found, as the contract lists for these paths.
func pathID(r *http.Request, name string) (shared.ID, error) {
	id, err := shared.ParseID(r.PathValue(name))
	if err != nil {
		return shared.ID{}, shared.ErrNotFound
	}
	return id, nil
}
