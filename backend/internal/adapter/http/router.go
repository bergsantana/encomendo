package http

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
)

// Handlers are the handlers the router mounts. Each milestone adds its own.
type Handlers struct {
	Health *handler.Health
}

// RouterOptions configure the middleware stack.
type RouterOptions struct {
	HSTS bool
}

// NewRouter returns the API handler: every route, wrapped in the middleware
// stack. Order matters: the request ID is set first so every log line has it,
// and Logging wraps Recover so a recovered panic is logged as a 500.
func NewRouter(log *slog.Logger, h Handlers, opts RouterOptions) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", h.Health.Live)
	mux.HandleFunc("GET /readyz", h.Health.Ready)

	return middleware.Chain(withProblemFallback(mux),
		middleware.RequestID,
		middleware.Logging(log),
		middleware.Recover(log),
		middleware.SecurityHeaders(opts.HSTS),
	)
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
