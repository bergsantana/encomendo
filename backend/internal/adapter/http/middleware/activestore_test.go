package middleware_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

type checker struct{ err error }

func (c checker) CheckStore(context.Context) error { return c.err }

func TestActiveStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantPass   bool
		wantStatus int
		wantCode   string
	}{
		{name: "active store", wantPass: true, wantStatus: http.StatusOK},
		{name: "suspended store", err: tenancy.ErrTenantSuspended, wantStatus: http.StatusForbidden, wantCode: respond.CodeTenantSuspended},
		// A status that cannot be read fails the request, never passes it.
		{name: "status unreadable", err: fmt.Errorf("read store status: %w: %w", shared.ErrUnavailable, errors.New("postgres down")), wantStatus: http.StatusServiceUnavailable, wantCode: respond.CodeNotReady},
		{name: "a bug behind the check", err: errors.New("store row missing"), wantStatus: http.StatusInternalServerError, wantCode: respond.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reached := false
			h := middleware.ActiveStore(checker{err: tt.err}, discard())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil))

			if reached != tt.wantPass || rec.Code != tt.wantStatus {
				t.Fatalf("reached %v, answer %d; want %v and %d", reached, rec.Code, tt.wantPass, tt.wantStatus)
			}
			if tt.wantCode != "" && problem(t, rec).Code != tt.wantCode {
				t.Errorf("code = %s, want %s", problem(t, rec).Code, tt.wantCode)
			}
		})
	}
}
