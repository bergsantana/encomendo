package respond

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// Stable problem codes. Clients branch on these, never on titles or details.
// docs/api/openapi.yaml lists every one in the Problem schema.
const (
	CodeInvalid          = "invalid"
	CodeNotFound         = "not_found"
	CodeConflict         = "conflict"
	CodeForbidden        = "forbidden"
	CodeInternal         = "internal"
	CodeNotReady         = "not_ready"
	CodeMethodNotAllowed = "method_not_allowed"

	// The codes of staff sessions (ADR 0010) and of the team (ADR 0011).
	CodeUnauthenticated    = "unauthenticated"
	CodeInvalidCredentials = "invalid_credentials" //nolint:gosec // G101: a problem code, not a credential
	CodeTenantRequired     = "tenant_required"
	CodeVersionConflict    = "version_conflict"
	CodeRateLimited        = "rate_limited"
	CodeEmailTaken         = "email_taken"
	CodeSlugTaken          = "slug_taken"
	CodeAlreadyMember      = "already_member"
	CodeInvalidCode        = "invalid_code"
	CodeEmailUnavailable   = "email_unavailable"
)

// StatusClientClosedRequest is the status logged for a request whose client
// went away before the answer, nginx's 499. Nobody reads it; it keeps such
// requests out of the server-failure counts (ADR 0019).
const StatusClientClosedRequest = 499

// RetryAfterUnavailable is the wait every 503 not_ready asks for: long
// enough for a busy password hasher to free a slot (ADR 0019), short enough
// that a person retrying does not notice.
const RetryAfterUnavailable = 5 * time.Second

// HeaderRetryAfter carries the wait, in whole seconds, on 429 and 503.
const HeaderRetryAfter = "Retry-After"

// Problem is an RFC 9457 problem details body with Encomendo's stable code.
type Problem struct {
	Type     string       `json:"type"`
	Title    string       `json:"title"`
	Status   int          `json:"status"`
	Code     string       `json:"code"`
	Detail   string       `json:"detail,omitempty"`
	Instance string       `json:"instance,omitempty"`
	Errors   []FieldIssue `json:"errors,omitempty"`
}

// FieldIssue is one invalid field in a 400 response.
type FieldIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error writes the problem response for err. Known domain errors keep their
// meaning; anything else becomes a 500 whose detail is never sent to the
// client, only logged.
//
// A request whose client has gone (its context is canceled) comes first,
// whatever err matches, and gets StatusClientClosedRequest and no body: the
// Postgres TxManager reports a commit that failed after the client left as
// both the context's error and the commit's, and that is the client
// leaving, not a conflict (ADR 0019). Only an error that is the
// cancellation itself is logged at debug. Any other is logged as usual, so
// a failure on work that outlives the request, such as a logout's revoke,
// still reaches an operator.
//
// Every 401 carries WWW-Authenticate: Bearer, and every 503 not_ready
// carries Retry-After, which the apps show (ADR 0013).
func Error(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	ctx := r.Context()
	p := problemFor(err)
	if errors.Is(ctx.Err(), context.Canceled) {
		if errors.Is(err, context.Canceled) {
			log.DebugContext(ctx, "request canceled by the client", "err", err)
		} else {
			logProblem(ctx, log, p, err)
		}
		w.WriteHeader(StatusClientClosedRequest)
		return
	}
	logProblem(ctx, log, p, err)
	switch p.Code {
	case CodeUnauthenticated, CodeInvalidCredentials:
		w.Header().Set("WWW-Authenticate", "Bearer")
	case CodeNotReady:
		setRetryAfter(w, RetryAfterUnavailable)
	}
	p.Instance = r.URL.Path
	Write(w, p)
}

// RateLimited answers 429 rate_limited after a limit refused the request.
// Retry-After is wait in whole seconds, rounded up and at least 1, because
// every 429 must carry it and the apps show it (ADR 0013).
func RateLimited(w http.ResponseWriter, r *http.Request, wait time.Duration) {
	setRetryAfter(w, wait)
	Write(w, Problem{
		Status: http.StatusTooManyRequests, Code: CodeRateLimited, Title: "Muitas tentativas.", Instance: r.URL.Path,
	})
}

// Write sends p as application/problem+json.
func Write(w http.ResponseWriter, p Problem) {
	if p.Type == "" {
		p.Type = "about:blank"
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	// The status line is already sent; a failed write means the client left.
	_ = json.NewEncoder(w).Encode(p)
}

// setRetryAfter writes d in whole seconds, rounded up, and never below 1:
// "Retry-After: 0" would invite an immediate retry loop.
func setRetryAfter(w http.ResponseWriter, d time.Duration) {
	secs := d / time.Second
	if d%time.Second > 0 {
		secs++
	}
	w.Header().Set(HeaderRetryAfter, strconv.FormatInt(max(int64(secs), 1), 10))
}

// logProblem logs what an operator must see: every 5xx with its error, and
// a scope mismatch. ErrScopeMismatch stays a 403 because a client can cause
// it (a staff token sent to a store host), but it can also mean a
// middleware runs in the wrong order, so it is logged at warn (ADR 0009).
func logProblem(ctx context.Context, log *slog.Logger, p Problem, err error) {
	switch {
	case p.Status == http.StatusInternalServerError:
		log.ErrorContext(ctx, "request failed", "err", err)
	case p.Status == http.StatusServiceUnavailable:
		log.WarnContext(ctx, "request could not be served now", "err", err)
	case errors.Is(err, authz.ErrScopeMismatch):
		log.WarnContext(ctx, "request scope does not match the caller", "err", err)
	}
}

// problemFor maps err, most specific match first. The order matters where
// one sentinel matches another: ErrInvalidCredentials matches
// ErrUnauthenticated, ErrTenantRequired matches ErrForbidden, and a commit
// that failed after the request's deadline matches both
// context.DeadlineExceeded and the commit's error, where the deadline wins.
// An outage comes before any refusal, because a failure is never a 401
// (ADR 0013). A cancellation the client did not cause is the server's own
// doing: a 500.
func problemFor(err error) Problem {
	switch {
	case errors.Is(err, context.Canceled):
		return Problem{Status: http.StatusInternalServerError, Code: CodeInternal, Title: "Erro interno"}
	case errors.Is(err, apptenancy.ErrEmailUnavailable):
		return Problem{
			Status: http.StatusServiceUnavailable, Code: CodeEmailUnavailable,
			Title: `O convite foi salvo, mas o e-mail não saiu. Use "Reenviar" para tentar de novo.`,
		}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, shared.ErrUnavailable):
		return Problem{
			Status: http.StatusServiceUnavailable, Code: CodeNotReady,
			Title: "O servidor não respondeu. Tente de novo em instantes.",
		}
	case errors.Is(err, identity.ErrInvalidCredentials):
		return Problem{Status: http.StatusUnauthorized, Code: CodeInvalidCredentials, Title: "E-mail ou senha incorretos."}
	case errors.Is(err, shared.ErrUnauthenticated):
		return Problem{Status: http.StatusUnauthorized, Code: CodeUnauthenticated, Title: "Sessão inválida. Entre de novo."}
	case errors.Is(err, authz.ErrTenantRequired):
		return Problem{Status: http.StatusForbidden, Code: CodeTenantRequired, Title: "Escolha a loja."}
	case errors.Is(err, shared.ErrForbidden):
		return Problem{Status: http.StatusForbidden, Code: CodeForbidden, Title: "Acesso negado"}
	case errors.Is(err, shared.ErrConflict):
		return conflictFor(err)
	case errors.Is(err, shared.ErrInvalid):
		return invalidFor(err)
	case errors.Is(err, shared.ErrNotFound):
		return Problem{Status: http.StatusNotFound, Code: CodeNotFound, Title: "Não encontrado"}
	default:
		return Problem{Status: http.StatusInternalServerError, Code: CodeInternal, Title: "Erro interno"}
	}
}

// conflictFor maps an error matching shared.ErrConflict. Titles use the
// wording of docs/product/design-document.md, which the apps fall back to.
func conflictFor(err error) Problem {
	p := Problem{Status: http.StatusConflict, Code: CodeConflict, Title: "Conflito"}
	switch {
	case errors.Is(err, shared.ErrVersionConflict):
		p.Code, p.Title = CodeVersionConflict, "Alguém salvou antes de você. Recarregue e faça a mudança de novo."
	case errors.Is(err, identity.ErrEmailTaken):
		p.Code, p.Title = CodeEmailTaken, "Já existe uma conta com este e-mail."
	case errors.Is(err, tenancy.ErrSlugTaken):
		p.Code, p.Title = CodeSlugTaken, "Este link já está em uso. Escolha outro."
	case errors.Is(err, tenancy.ErrAlreadyMember):
		p.Code, p.Title = CodeAlreadyMember, "Esta pessoa já faz parte da equipe."
	}
	return p
}

// invalidFor maps an error matching shared.ErrInvalid, listing the fields
// of a *shared.ValidationError.
func invalidFor(err error) Problem {
	if errors.Is(err, identity.ErrInvalidCode) {
		return Problem{
			Status: http.StatusBadRequest, Code: CodeInvalidCode,
			Title: "Código errado ou vencido. Confira o e-mail ou peça um novo código.",
		}
	}
	p := Problem{Status: http.StatusBadRequest, Code: CodeInvalid, Title: "Dados inválidos"}
	var ve *shared.ValidationError
	if errors.As(err, &ve) {
		for _, f := range ve.Fields {
			p.Errors = append(p.Errors, FieldIssue{Field: f.Field, Message: f.Message})
		}
	}
	return p
}
