package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/handler/dto"
	apporder "github.com/bergsantana/encomendo/backend/internal/app/order"
)

// TodayDeps is what the Hoje handler needs.
type TodayDeps struct {
	UseCases apporder.UseCases
	Log      *slog.Logger
}

// Today serves GET /api/v1/admin/today: Hoje. Its answer type has no field
// for a cost or a margin, so no role can get one from it.
type Today struct {
	uc  apporder.UseCases
	log *slog.Logger
}

// NewToday returns the Hoje handler, or an error when a dependency is
// missing.
func NewToday(d TodayDeps) (*Today, error) {
	if d.UseCases == nil || d.Log == nil {
		return nil, errors.New("handler: the Hoje handler needs use cases and a logger")
	}
	return &Today{uc: d.UseCases, log: d.Log}, nil
}

// Get is GET /api/v1/admin/today.
func (h *Today) Get(w http.ResponseWriter, r *http.Request) {
	v, err := h.uc.Today(r.Context())
	reply(w, r, h.log, http.StatusOK, err, func() any { return dto.NewToday(v) })
}
