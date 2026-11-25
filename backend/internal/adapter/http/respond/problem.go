package respond

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Stable problem codes. Clients branch on these, never on titles or details.
const (
	CodeInvalid          = "invalid"
	CodeNotFound         = "not_found"
	CodeConflict         = "conflict"
	CodeForbidden        = "forbidden"
	CodeInternal         = "internal"
	CodeNotReady         = "not_ready"
	CodeMethodNotAllowed = "method_not_allowed"
)

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
func Error(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	p := problemFor(err)
	if p.Status >= http.StatusInternalServerError {
		log.ErrorContext(r.Context(), "request failed", "err", err)
	}
	p.Instance = r.URL.Path
	Write(w, p)
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

func problemFor(err error) Problem {
	var ve *shared.ValidationError
	switch {
	case errors.As(err, &ve):
		p := Problem{Status: http.StatusBadRequest, Code: CodeInvalid, Title: "Dados inválidos"}
		for _, f := range ve.Fields {
			p.Errors = append(p.Errors, FieldIssue{Field: f.Field, Message: f.Message})
		}
		return p
	case errors.Is(err, shared.ErrInvalid):
		return Problem{Status: http.StatusBadRequest, Code: CodeInvalid, Title: "Dados inválidos"}
	case errors.Is(err, shared.ErrNotFound):
		return Problem{Status: http.StatusNotFound, Code: CodeNotFound, Title: "Não encontrado"}
	case errors.Is(err, shared.ErrConflict):
		return Problem{Status: http.StatusConflict, Code: CodeConflict, Title: "Conflito"}
	case errors.Is(err, shared.ErrForbidden):
		return Problem{Status: http.StatusForbidden, Code: CodeForbidden, Title: "Acesso negado"}
	default:
		return Problem{Status: http.StatusInternalServerError, Code: CodeInternal, Title: "Erro interno"}
	}
}
