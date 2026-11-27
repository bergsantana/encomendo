package handler

import (
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// reply writes err as a problem, or status with body() when body is not
// nil, or just status when it is.
func reply(w http.ResponseWriter, r *http.Request, log *slog.Logger, status int, err error, body func() any) {
	if err != nil {
		respond.Error(w, r, log, err)
		return
	}
	if body == nil {
		w.WriteHeader(status)
		return
	}
	respond.JSON(w, r, log, status, body())
}

// decode reads the request body into an R, refusing unknown fields, then
// turns it into the use case's input with input. On any failure it has
// already answered 400 and returns false.
func decode[R, I any](w http.ResponseWriter, r *http.Request, log *slog.Logger, input func(R) (I, error)) (I, bool) {
	var (
		req  R
		zero I
	)
	if err := respond.Decode(w, r, &req, respond.DefaultMaxBodyBytes); err != nil {
		respond.Error(w, r, log, err)
		return zero, false
	}
	in, err := input(req)
	if err != nil {
		respond.Error(w, r, log, err)
		return zero, false
	}
	return in, true
}

// idParam reads the id named name from the path. One that is not an id
// names nothing, so it answers 404 not_found and returns false.
func idParam[T ~[16]byte](w http.ResponseWriter, r *http.Request, log *slog.Logger, name string) (T, bool) {
	id, err := pathID(r, name)
	if err != nil {
		respond.Error(w, r, log, err)
		return T{}, false
	}
	return T(id), true
}

// listQuery reads the query parameters every catalog list shares: the page,
// the search text and, with archived, includeArchived. A malformed one is
// 400 invalid.
type listQuery struct {
	page            shared.PageRequest
	text            string
	includeArchived bool
}

const maxSearchLength = 100

func readListQuery(r *http.Request) (listQuery, error) {
	page, err := pageRequest(r)
	if err != nil {
		return listQuery{}, err
	}
	q := r.URL.Query()
	out := listQuery{page: page, text: q.Get("q")}
	if len([]rune(out.text)) > maxSearchLength {
		return listQuery{}, shared.NewValidationError(shared.FieldError{Field: "q", Message: "Use até 100 caracteres."})
	}
	switch q.Get("includeArchived") {
	case "", "false":
	case "true":
		out.includeArchived = true
	default:
		return listQuery{}, shared.NewValidationError(shared.FieldError{Field: "includeArchived", Message: "Use true ou false."})
	}
	return out, nil
}
