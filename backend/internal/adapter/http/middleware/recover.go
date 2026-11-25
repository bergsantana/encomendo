package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
)

// Recover turns a panic in a handler into a logged 500 problem response, so
// one bad request never takes the process down. http.ErrAbortHandler is
// re-raised because net/http uses it to abort a response on purpose.
func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec)
				}
				log.ErrorContext(r.Context(), "panic in handler",
					"panic", fmt.Sprint(rec),
					"request_id", RequestIDFrom(r.Context()),
					"stack", string(debug.Stack()),
				)
				respond.Write(w, respond.Problem{
					Status: http.StatusInternalServerError,
					Code:   respond.CodeInternal,
					Title:  "Erro interno",
				})
			}()
			next.ServeHTTP(w, r)
		})
	}
}
