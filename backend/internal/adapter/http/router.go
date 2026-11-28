package http

import (
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/ratelimit"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Handlers are the handlers the router mounts. Each milestone adds its own.
// A nil group is not mounted.
type Handlers struct {
	Health     *handler.Health
	Auth       *handler.Auth
	Tenancy    *handler.Tenancy
	Catalog    *handler.Catalog
	Suppliers  *handler.Suppliers
	Workflows  *handler.Workflows
	Customers  *handler.Customers
	Orders     *handler.Orders
	Today      *handler.Today
	Finance    *handler.Finance
	Purchasing *handler.Purchasing
}

// hasAdmin reports whether any group of /api/v1/admin routes is set.
func (h Handlers) hasAdmin() bool {
	return h.Tenancy != nil || h.Catalog != nil || h.Suppliers != nil || h.Workflows != nil ||
		h.Customers != nil || h.Orders != nil || h.Today != nil || h.Finance != nil || h.Purchasing != nil
}

// RouterOptions configure the middleware stack.
type RouterOptions struct {
	HSTS bool
	// AuthGuard is the cross-origin check of /auth/* (middleware.CrossOrigin,
	// built in main because it validates the admin origin). Required when
	// Handlers.Auth is set.
	AuthGuard middleware.Middleware
	// RateLimits applies the per-IP rules (ADR 0021). Required when
	// Handlers.Auth is set.
	RateLimits *middleware.RateLimits
	// Authn checks the access token on /api/v1/admin/*. Required when any
	// admin handler group is set.
	Authn *middleware.Authn
	// Stores refuses a suspended store on every tenant-scoped admin route
	// (ADR 0022). Required when any admin handler group is set.
	Stores middleware.StoreChecker
}

// NewRouter returns the API handler: every route, wrapped in the middleware
// stack. Order matters: the request ID is set first so every log line has it,
// and Logging wraps Recover so a recovered panic is logged as a 500. It
// returns an error for /auth handlers without their cross-origin check or
// rate limits, which would mount routes that fail every request or none.
func NewRouter(log *slog.Logger, h Handlers, opts RouterOptions) (http.Handler, error) {
	if h.Auth != nil && (opts.AuthGuard == nil || opts.RateLimits == nil) {
		return nil, errors.New("router: the /auth routes need the cross-origin check and the rate limits")
	}
	if h.hasAdmin() && (opts.Authn == nil || opts.Stores == nil) {
		return nil, errors.New("router: the admin routes need authn and the suspended-store check")
	}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", h.Health.Live)
	mux.HandleFunc("GET /readyz", h.Health.Ready)
	if h.Auth != nil {
		mountAuth(mux, h.Auth, opts)
	}
	if h.hasAdmin() {
		mountAdmin(mux, h, opts.Authn, middleware.ActiveStore(opts.Stores, log))
	}

	return middleware.Chain(withProblemFallback(mux),
		middleware.RequestID,
		middleware.Logging(log),
		middleware.Recover(log),
		middleware.SecurityHeaders(opts.HSTS),
	), nil
}

// authRoute is one /auth operation and the per-IP rule in front of it, if
// any.
type authRoute struct {
	pattern string
	rule    *ratelimit.Rule
	handle  http.HandlerFunc
}

// authRoutes are the nine /auth operations of the contract. Switching and
// logout have no per-IP limit (ADR 0021).
func authRoutes(a *handler.Auth) []authRoute {
	return []authRoute{
		{"POST /auth/signup", &ratelimit.SignUpByIP, a.SignUp},
		{"POST /auth/login", &ratelimit.LogInByIP, a.LogIn},
		{"POST /auth/refresh", &ratelimit.RefreshByIP, a.Refresh},
		{"POST /auth/switch-tenant", nil, a.SwitchTenant},
		{"POST /auth/logout", nil, a.LogOut},
		{"POST /auth/password-reset", &ratelimit.PasswordResetByIP, a.RequestPasswordReset},
		{"POST /auth/password-reset/confirm", &ratelimit.PasswordResetConfirmByIP, a.ConfirmPasswordReset},
		{"POST /auth/invitations/preview", &ratelimit.InvitationPreviewByIP, a.PreviewInvitation},
		{"POST /auth/invitations/accept", &ratelimit.InvitationAcceptByIP, a.AcceptInvitation},
	}
}

// mountAuth mounts each /auth operation on its own method and path, so a
// wrong method answers 405 before any check runs. The order in front of each
// one matters: the cross-origin check first, so a cross-site page cannot
// spend its visitors' rate-limit budgets; then the deadline; then the
// per-IP limit, before the body is read (ADRs 0010, 0019 and 0021).
func mountAuth(mux *http.ServeMux, a *handler.Auth, opts RouterOptions) {
	for _, rt := range authRoutes(a) {
		mws := []middleware.Middleware{opts.AuthGuard, middleware.Deadline(middleware.AuthDeadline)}
		if rt.rule != nil {
			mws = append(mws, opts.RateLimits.ByIP(*rt.rule))
		}
		mux.Handle(rt.pattern, middleware.Chain(rt.handle, mws...))
	}
}

// adminRoutes are every /api/v1/admin operation of every handler group. A
// new group adds its routes here, never through a mount of its own, so each
// one gets authn and the suspended-store check (ADR 0022).
func adminRoutes(h Handlers) map[string]http.HandlerFunc {
	routes := map[string]http.HandlerFunc{}
	if h.Tenancy != nil {
		maps.Copy(routes, tenancyRoutes(h.Tenancy))
	}
	if h.Catalog != nil {
		maps.Copy(routes, catalogRoutes(h.Catalog))
	}
	if h.Suppliers != nil {
		maps.Copy(routes, supplierRoutes(h.Suppliers))
	}
	if h.Workflows != nil {
		routes["GET /api/v1/admin/status-workflows"] = h.Workflows.List
	}
	if h.Customers != nil {
		maps.Copy(routes, customerRoutes(h.Customers))
	}
	if h.Orders != nil {
		maps.Copy(routes, orderRoutes(h.Orders))
	}
	if h.Today != nil {
		routes["GET /api/v1/admin/today"] = h.Today.Get
	}
	if h.Finance != nil {
		routes["GET /api/v1/admin/finance/summary"] = h.Finance.Summary
	}
	if h.Purchasing != nil {
		maps.Copy(routes, purchasingRoutes(h.Purchasing))
	}
	return routes
}

func tenancyRoutes(t *handler.Tenancy) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"POST /api/v1/admin/tenants":                      t.CreateTenant,
		"GET /api/v1/admin/settings":                      t.Settings,
		"PUT /api/v1/admin/settings":                      t.UpdateSettings,
		"PUT /api/v1/admin/settings/status-workflow":      t.ChooseWorkflow,
		"GET /api/v1/admin/members":                       t.Members,
		"DELETE /api/v1/admin/members/{userId}":           t.RemoveMember,
		"PUT /api/v1/admin/members/{userId}/role":         t.ChangeRole,
		"GET /api/v1/admin/invitations":                   t.Invitations,
		"POST /api/v1/admin/invitations":                  t.InviteMember,
		"DELETE /api/v1/admin/invitations/{invitationId}": t.RevokeInvitation,
	}
}

// catalogRoutes are the 29 catalog operations of the contract.
func catalogRoutes(c *handler.Catalog) map[string]http.HandlerFunc {
	const base = "/api/v1/admin/catalog"
	return map[string]http.HandlerFunc{
		"GET " + base + "/categories":                         c.ListCategories,
		"POST " + base + "/categories":                        c.CreateCategory,
		"PATCH " + base + "/categories/{categoryId}":          c.UpdateCategory,
		"DELETE " + base + "/categories/{categoryId}":         c.DeleteCategory,
		"GET " + base + "/attributes":                         c.ListAttributes,
		"POST " + base + "/attributes":                        c.CreateAttribute,
		"PUT " + base + "/attributes/{attributeId}":           c.ReplaceAttribute,
		"DELETE " + base + "/attributes/{attributeId}":        c.DeleteAttribute,
		"GET " + base + "/items":                              c.ListItems,
		"POST " + base + "/items":                             c.CreateItem,
		"GET " + base + "/items/{itemId}":                     c.GetItem,
		"PUT " + base + "/items/{itemId}":                     c.ReplaceItem,
		"PUT " + base + "/items/{itemId}/published":           c.SetItemPublished,
		"POST " + base + "/items/{itemId}/archive":            c.ArchiveItem,
		"POST " + base + "/items/{itemId}/restore":            c.RestoreItem,
		"POST " + base + "/items/{itemId}/photos":             c.ConfirmItemPhoto,
		"PUT " + base + "/items/{itemId}/photos/order":        c.OrderItemPhotos,
		"DELETE " + base + "/items/{itemId}/photos/{photoId}": c.DeleteItemPhoto,
		"GET " + base + "/kits":                               c.ListKits,
		"POST " + base + "/kits":                              c.CreateKit,
		"GET " + base + "/kits/{kitId}":                       c.GetKit,
		"PUT " + base + "/kits/{kitId}":                       c.ReplaceKit,
		"PUT " + base + "/kits/{kitId}/published":             c.SetKitPublished,
		"POST " + base + "/kits/{kitId}/archive":              c.ArchiveKit,
		"POST " + base + "/kits/{kitId}/restore":              c.RestoreKit,
		"POST " + base + "/kits/{kitId}/photos":               c.ConfirmKitPhoto,
		"PUT " + base + "/kits/{kitId}/photos/order":          c.OrderKitPhotos,
		"DELETE " + base + "/kits/{kitId}/photos/{photoId}":   c.DeleteKitPhoto,
		"POST " + base + "/photo-uploads":                     c.RequestPhotoUpload,
	}
}

// supplierRoutes are the five supplier operations of the contract.
func supplierRoutes(s *handler.Suppliers) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"GET /api/v1/admin/suppliers":                    s.List,
		"POST /api/v1/admin/suppliers":                   s.Create,
		"GET /api/v1/admin/suppliers/{supplierId}":       s.Get,
		"PUT /api/v1/admin/suppliers/{supplierId}":       s.Replace,
		"GET /api/v1/admin/suppliers/{supplierId}/items": s.Items,
	}
}

// purchasingRoutes are the sixteen purchasing operations of the contract.
// POST /supplier-orders/preview is its own path: the mux prefers the
// literal word to /supplier-orders/{supplierOrderId}, and an id is never a
// word, as with /orders/quote.
func purchasingRoutes(p *handler.Purchasing) map[string]http.HandlerFunc {
	const base = "/api/v1/admin/purchasing"
	return map[string]http.HandlerFunc{
		"GET " + base + "/to-buy":                                          p.ToBuy,
		"POST " + base + "/supplier-orders/preview":                        p.PreviewSupplierOrder,
		"GET " + base + "/supplier-orders":                                 p.ListSupplierOrders,
		"POST " + base + "/supplier-orders":                                p.SendSupplierOrder,
		"GET " + base + "/supplier-orders/{supplierOrderId}":               p.GetSupplierOrder,
		"PUT " + base + "/supplier-orders/{supplierOrderId}/promised-date": p.RecordPromisedDate,
		"POST " + base + "/supplier-orders/{supplierOrderId}/in-transit":   p.MarkInTransit,
		"POST " + base + "/supplier-orders/{supplierOrderId}/undo":         p.UndoSupplierOrder,
		"PUT " + base + "/supplier-orders/{supplierOrderId}/paid-prices":   p.RecordPaidPrices,
		"GET " + base + "/to-receive":                                      p.ToReceive,
		"POST " + base + "/receipt-photo-uploads":                          p.RequestReceiptPhotoUpload,
		"POST " + base + "/receipts":                                       p.RecordReceipt,
		"GET " + base + "/notices":                                         p.ListNotices,
		"POST " + base + "/notices/{lineId}/resolve":                       p.ResolveNotice,
		"GET " + base + "/orders/{orderId}/lines":                          p.OrderPurchases,
		"GET " + base + "/today":                                           p.Today,
	}
}

// customerRoutes are the four customer operations of the contract.
func customerRoutes(c *handler.Customers) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"GET /api/v1/admin/customers":              c.List,
		"POST /api/v1/admin/customers":             c.Create,
		"GET /api/v1/admin/customers/{customerId}": c.Get,
		"PUT /api/v1/admin/customers/{customerId}": c.Update,
	}
}

// orderRoutes are the fifteen order operations of the contract. POST
// /orders/quote is its own path: the mux prefers the literal word to
// /orders/{orderId}, and an order id is never a word.
func orderRoutes(o *handler.Orders) map[string]http.HandlerFunc {
	const base = "/api/v1/admin/orders"
	return map[string]http.HandlerFunc{
		"POST " + base + "/quote":                                        o.Quote,
		"GET " + base:                                                    o.List,
		"POST " + base:                                                   o.Create,
		"GET " + base + "/{orderId}":                                     o.Get,
		"PUT " + base + "/{orderId}":                                     o.Change,
		"POST " + base + "/{orderId}/advance":                            o.Advance,
		"POST " + base + "/{orderId}/release":                            o.Release,
		"POST " + base + "/{orderId}/items/{itemId}/move":                o.MoveItem,
		"POST " + base + "/{orderId}/items/{itemId}/use-backup-supplier": o.UseBackupSupplier,
		"POST " + base + "/{orderId}/payments":                           o.RecordPayment,
		"POST " + base + "/{orderId}/payments/{paymentId}/void":          o.VoidPayment,
		"PUT " + base + "/{orderId}/delivery-date":                       o.ChangeDeliveryDate,
		"POST " + base + "/{orderId}/tracking-link":                      o.SendTrackingLink,
		"POST " + base + "/{orderId}/cancel":                             o.Cancel,
		"GET " + base + "/{orderId}/history":                             o.History,
	}
}

// worksInAStore reports whether an admin route acts in the selected store,
// so the suspended-store check runs on it. Creating a store works without
// one.
func worksInAStore(pattern string) bool { return pattern != "POST /api/v1/admin/tenants" }

// mountAdmin mounts every admin route behind authn for the backoffice
// audience: a client account's token is a 401 here, and so is none
// (docs/architecture.md section 5.1). Every route that works in a store
// then refuses a suspended one (ADR 0022). Bearer tokens are not sent by
// the browser on its own, so these routes need no cross-origin check.
func mountAdmin(mux *http.ServeMux, h Handlers, authn *middleware.Authn, activeStore middleware.Middleware) {
	staff := authn.Require(shared.AudienceBackoffice)
	for pattern, h := range adminRoutes(h) {
		mws := []middleware.Middleware{staff}
		if worksInAStore(pattern) {
			mws = append(mws, activeStore)
		}
		mux.Handle(pattern, middleware.Chain(h, mws...))
	}
}

// routeMethods are the methods checked when a request matches no route, to
// tell "no such path" (404) from "path exists, wrong method" (405).
var routeMethods = []string{
	http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
}

// withProblemFallback serves matched requests through mux and answers
// unmatched ones with problem+json instead of the mux's plain-text errors.
func withProblemFallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}

		var allowed []string
		for _, m := range routeMethods {
			probe := r.Clone(r.Context())
			probe.Method = m
			if _, pattern := mux.Handler(probe); pattern != "" {
				allowed = append(allowed, m)
			}
		}
		if len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			respond.Write(w, respond.Problem{
				Status: http.StatusMethodNotAllowed, Code: respond.CodeMethodNotAllowed,
				Title: "Método não permitido", Instance: r.URL.Path,
			})
			return
		}
		respond.Write(w, respond.Problem{
			Status: http.StatusNotFound, Code: respond.CodeNotFound,
			Title: "Não encontrado", Instance: r.URL.Path,
		})
	})
}
