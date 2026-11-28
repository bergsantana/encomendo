package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// FinanceDeps is what the Financeiro handler needs.
type FinanceDeps struct {
	UseCases apporder.UseCases
	Log      *slog.Logger
}

// Finance serves GET /api/v1/admin/finance/summary: Financeiro, the
// owner's (finance:read, checked by the use case).
type Finance struct {
	uc  apporder.UseCases
	log *slog.Logger
}

// NewFinance returns the Financeiro handler, or an error when a dependency
// is missing.
func NewFinance(d FinanceDeps) (*Finance, error) {
	if d.UseCases == nil || d.Log == nil {
		return nil, errors.New("handler: the Financeiro handler needs use cases and a logger")
	}
	return &Finance{uc: d.UseCases, log: d.Log}, nil
}

// Summary is GET /api/v1/admin/finance/summary?from=&to=, both days in the
// store's time zone and both required.
func (h *Finance) Summary(w http.ResponseWriter, r *http.Request) {
	from, to, err := readPeriod(r)
	if err != nil {
		respond.Error(w, r, h.log, err)
		return
	}
	v, err := h.uc.Finance(r.Context(), from, to)
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewFinanceSummary(v) })
}

// readPeriod reads the from and to days. Both are required, so a missing or
// malformed one is a 400 naming it. The use case checks the order and the
// length of the period.
func readPeriod(r *http.Request) (from, to shared.CivilDate, err error) {
	var issues []shared.FieldError
	read := func(name string) shared.CivilDate {
		raw := r.URL.Query().Get(name)
		if raw == "" {
			issues = append(issues, shared.FieldError{Field: name, Message: "Obrigatório."})
			return shared.CivilDate{}
		}
		d, perr := shared.ParseCivilDate(raw)
		if perr != nil {
			issues = append(issues, shared.FieldError{Field: name, Message: "Use uma data no formato AAAA-MM-DD."})
		}
		return d
	}
	from, to = read("from"), read("to")
	if len(issues) > 0 {
		return from, to, shared.NewValidationError(issues...)
	}
	return from, to, nil
}
