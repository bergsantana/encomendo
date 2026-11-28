package handler

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// headerIdempotencyKey is the header that makes a creating call safe to
// retry (ADR 0030).
const headerIdempotencyKey = "Idempotency-Key"

// maxIdempotencyKeyLength is the contract's limit on a key.
const maxIdempotencyKeyLength = 128

// readBody reads the request body, at most max bytes, and puts it back so
// respond.Decode can read it again. It answers 400 itself and returns false
// when the body is too large or unreadable.
func readBody(w http.ResponseWriter, r *http.Request, log *slog.Logger, max int64) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, max))
	if err != nil {
		var tooLarge *http.MaxBytesError
		msg := "JSON inválido."
		if errors.As(err, &tooLarge) {
			msg = "O corpo passa do limite permitido."
		}
		respond.Error(w, r, log, shared.NewValidationError(shared.FieldError{Field: "body", Message: msg}))
		return nil, false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, true
}

// idempotencyKey reads the Idempotency-Key header of r for a request whose
// body is body. A missing key comes back zero: the use case refuses it
// after checking the permission, so a role that may not make the call is
// never told about the header (ADR 0030). A key that is not 1 to 128
// printable ASCII characters is a 400.
//
// The hash covers who sent the request, the method, the path and the body,
// so the same key with another request is refused (422), and two staff
// members of one store who happen to send the same key and body never
// replay each other's order or payment (security review of 4.12 and 4.13).
func idempotencyKey(r *http.Request, body []byte) (appshared.IdempotencyKey, error) {
	key := r.Header.Get(headerIdempotencyKey)
	if key == "" {
		return appshared.IdempotencyKey{}, nil
	}
	if !validIdempotencyKey(key) {
		return appshared.IdempotencyKey{}, shared.NewValidationError(shared.FieldError{
			Field: headerIdempotencyKey, Message: "Use de 1 a 128 caracteres, sem espaços.",
		})
	}
	var user shared.UserID
	if p, ok := authz.PrincipalFrom(r.Context()); ok {
		user = p.UserID()
	}
	h := sha256.New()
	for _, part := range []string{user.String(), "\n", r.Method, "\n", r.URL.Path, "\n"} {
		h.Write([]byte(part))
	}
	h.Write(body)
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return appshared.IdempotencyKey{Key: key, RequestHash: sum}, nil
}

// validIdempotencyKey is the contract's pattern ^[!-~]{1,128}$.
func validIdempotencyKey(k string) bool {
	if len(k) == 0 || len(k) > maxIdempotencyKeyLength {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < '!' || k[i] > '~' {
			return false
		}
	}
	return true
}
