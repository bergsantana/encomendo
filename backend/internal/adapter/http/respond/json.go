package respond

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// DefaultMaxBodyBytes caps request bodies unless a handler asks for more
// (uploads go through storage, never through JSON bodies).
const DefaultMaxBodyBytes int64 = 1 << 20 // 1 MiB

// Decode reads one JSON object from the request body into dst. It rejects
// bodies over maxBytes, unknown fields and trailing data, and returns a
// *shared.ValidationError so the caller can pass it straight to Error.
func Decode(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return shared.NewValidationError(shared.FieldError{
			Field: "body", Message: "O corpo deve conter um único objeto JSON.",
		})
	}
	return nil
}

func decodeError(err error) error {
	var maxErr *http.MaxBytesError
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	switch {
	case errors.As(err, &maxErr):
		return shared.NewValidationError(shared.FieldError{
			Field: "body", Message: fmt.Sprintf("O corpo passa do limite de %d bytes.", maxErr.Limit),
		})
	case errors.As(err, &typeErr):
		return shared.NewValidationError(shared.FieldError{
			Field: typeErr.Field, Message: "Tipo de valor inválido.",
		})
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return shared.NewValidationError(shared.FieldError{
			Field: "body", Message: "JSON inválido.",
		})
	default:
		// encoding/json reports unknown fields only as text: json: unknown field "x".
		if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			return shared.NewValidationError(shared.FieldError{
				Field: strings.Trim(name, `"`), Message: "Campo não reconhecido.",
			})
		}
		return shared.NewValidationError(shared.FieldError{Field: "body", Message: "JSON inválido."})
	}
}

// JSON writes v as a JSON response with the given status. An encoding error
// can only be logged: the status line has already been sent.
func JSON(w http.ResponseWriter, r *http.Request, log *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.ErrorContext(r.Context(), "encode response", "err", err)
	}
}
