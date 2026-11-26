package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
)

func TestDeadline_EndsTheRequestContext(t *testing.T) {
	t.Parallel()

	var deadline time.Time
	var ok bool
	h := middleware.Deadline(time.Hour)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		deadline, ok = r.Context().Deadline()
	}))
	before := time.Now()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/auth/login", nil))

	if !ok || deadline.Before(before.Add(time.Hour)) || deadline.After(time.Now().Add(time.Hour)) {
		t.Errorf("deadline = %v (set %v), want an hour from the request", deadline, ok)
	}
}

// The deadline's context stays a child of the request's: a client that
// leaves still stops the work, so a hash for nobody gives its slot back,
// and values such as the request ID pass through.
func TestDeadline_KeepsTheRequestContext(t *testing.T) {
	t.Parallel()

	type key struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "request-id"))
	defer cancel()
	var value any
	var stopped error
	h := middleware.Deadline(time.Hour)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		value = r.Context().Value(key{})
		cancel()
		// Bounded, so a context cut off from the request fails the test
		// instead of hanging it for the hour-long deadline.
		select {
		case <-r.Context().Done():
			stopped = r.Context().Err()
		case <-time.After(5 * time.Second):
		}
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(parent, http.MethodPost, "/auth/login", nil))

	if value != "request-id" {
		t.Errorf("context value = %v, want it passed through", value)
	}
	if !errors.Is(stopped, context.Canceled) {
		t.Errorf("after the client left, the context ended with %v, want context.Canceled", stopped)
	}
}

// Work stopped by the deadline answers 503 not_ready with Retry-After, the
// answer the apps retry, never a 500 or a 401 (ADR 0019).
func TestDeadline_StoppedWorkAnswersNotReady(t *testing.T) {
	t.Parallel()

	h := middleware.Deadline(time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		respond.Error(w, r, discard(), r.Context().Err())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/login", nil))

	if rec.Code != http.StatusServiceUnavailable || problem(t, rec).Code != respond.CodeNotReady {
		t.Errorf("answer = %d %s, want 503 not_ready", rec.Code, rec.Body.String())
	}
	if rec.Header().Get(respond.HeaderRetryAfter) == "" {
		t.Error("no Retry-After")
	}
}

// AuthDeadline leaves a busy hasher time to answer ErrBusy first, and stays
// under the Android app's 30-second request timeout (ADRs 0012 and 0019).
// One hash at the default costs takes well under a second; the margin below
// is twice that.
func TestAuthDeadline_LeavesRoomForTheHasher(t *testing.T) {
	t.Parallel()

	const oneHashMargin = 2 * time.Second
	if middleware.AuthDeadline < auth.DefaultMaxHashWait+oneHashMargin {
		t.Errorf("AuthDeadline %v leaves under %v after the hasher's %v wait",
			middleware.AuthDeadline, oneHashMargin, auth.DefaultMaxHashWait)
	}
	if middleware.AuthDeadline >= 30*time.Second {
		t.Errorf("AuthDeadline %v reaches the Android request timeout", middleware.AuthDeadline)
	}
}
