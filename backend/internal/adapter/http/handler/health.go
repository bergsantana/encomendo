package handler

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
)

// Check is one dependency the API needs to serve traffic, such as Postgres.
type Check struct {
	Name string
	Run  func(ctx context.Context) error
}

// Health serves the liveness and readiness probes.
type Health struct {
	checks  []Check
	timeout time.Duration
	log     *slog.Logger
}

// NewHealth returns a Health that runs checks for readiness, each bounded by
// timeout.
func NewHealth(log *slog.Logger, timeout time.Duration, checks ...Check) *Health {
	return &Health{checks: checks, timeout: timeout, log: log}
}

type statusResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// Live answers GET /healthz: the process is up and serving HTTP. It never
// touches dependencies, so a database outage does not get the process killed.
func (h *Health) Live(w http.ResponseWriter, r *http.Request) {
	respond.JSON(w, r, h.log, http.StatusOK, statusResponse{Status: "ok"})
}

// Ready answers GET /readyz: every dependency responded within the timeout.
// Failing checks are named in the response; their errors are only logged,
// because they can carry hostnames and other internal detail.
func (h *Health) Ready(w http.ResponseWriter, r *http.Request) {
	results := h.runChecks(r.Context())

	var failed []string
	for name, err := range results {
		if err != nil {
			failed = append(failed, name)
			h.log.WarnContext(r.Context(), "readiness check failed", "check", name, "err", err)
		}
	}
	if len(failed) > 0 {
		sort.Strings(failed)
		respond.Write(w, respond.Problem{
			Status: http.StatusServiceUnavailable,
			Code:   respond.CodeNotReady,
			Title:  "Serviço indisponível",
			Detail: "failing checks: " + strings.Join(failed, ", "),
		})
		return
	}

	checks := make(map[string]string, len(results))
	for name := range results {
		checks[name] = "ok"
	}
	respond.JSON(w, r, h.log, http.StatusOK, statusResponse{Status: "ok", Checks: checks})
}

// runChecks runs every check at once so one slow dependency does not add up
// with the others.
func (h *Health) runChecks(ctx context.Context) map[string]error {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results = make(map[string]error, len(h.checks))
	)
	for _, c := range h.checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := c.Run(ctx)
			mu.Lock()
			results[c.Name] = err
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results
}
