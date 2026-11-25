package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChain_FirstMiddlewareIsOutermost(t *testing.T) {
	t.Parallel()

	var order []string
	mark := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { order = append(order, "handler") }),
		mark("outer"), mark("inner"))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if got := strings.Join(order, ","); got != "outer,inner,handler" {
		t.Errorf("order = %s", got)
	}
}

func TestRequestID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		incoming string
		keep     bool
	}{
		{name: "keeps a safe incoming id", incoming: "abc-123_x.y", keep: true},
		{name: "generates when missing", incoming: ""},
		{name: "replaces unsafe characters", incoming: "bad\nid"},
		{name: "replaces overlong ids", incoming: strings.Repeat("a", 65)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var seen string
			h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				seen = RequestIDFrom(r.Context())
			}))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.incoming != "" {
				r.Header.Set(HeaderRequestID, tt.incoming)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			if seen == "" || w.Header().Get(HeaderRequestID) != seen {
				t.Fatalf("context id %q, header id %q", seen, w.Header().Get(HeaderRequestID))
			}
			if (seen == tt.incoming) != tt.keep {
				t.Errorf("kept incoming id = %v, want %v (got %q)", seen == tt.incoming, tt.keep, seen)
			}
		})
	}
}

func TestRecover_ReturnsProblemAndLogs(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	h := Recover(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(logs.String(), "panic in handler") || !strings.Contains(logs.String(), "boom") {
		t.Errorf("panic not logged: %s", logs.String())
	}
}

func TestRecover_ReraisesAbortHandler(t *testing.T) {
	t.Parallel()

	h := Recover(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		rec := recover()
		err, ok := rec.(error)
		if !ok || !errors.Is(err, http.ErrAbortHandler) {
			t.Errorf("recovered %v, want http.ErrAbortHandler", rec)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestLogging_LogsRoutePatternNotPath(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /p/{token}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hi"))
	})
	h := Chain(mux, RequestID, Logging(log))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/p/secret-token?x=1", nil))

	var line map[string]any
	if err := json.Unmarshal(logs.Bytes(), &line); err != nil {
		t.Fatalf("log is not JSON: %v (%s)", err, logs.String())
	}
	if line["route"] != "GET /p/{token}" {
		t.Errorf("route = %v", line["route"])
	}
	if line["status"] != float64(http.StatusTeapot) || line["bytes"] != float64(2) {
		t.Errorf("status/bytes = %v/%v", line["status"], line["bytes"])
	}
	if strings.Contains(logs.String(), "secret-token") {
		t.Errorf("token leaked into logs: %s", logs.String())
	}
}

func TestLogging_UnmatchedRoute(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	h := Logging(slog.New(slog.NewJSONHandler(&logs, nil)))(http.NotFoundHandler())

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nowhere", nil))

	if !strings.Contains(logs.String(), `"route":"unmatched"`) {
		t.Errorf("log = %s", logs.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	for _, hsts := range []bool{false, true} {
		w := httptest.NewRecorder()
		SecurityHeaders(hsts)(http.NotFoundHandler()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

		if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Frame-Options") != "DENY" {
			t.Errorf("hsts=%v: missing base headers: %v", hsts, w.Header())
		}
		if got := w.Header().Get("Strict-Transport-Security") != ""; got != hsts {
			t.Errorf("hsts=%v: Strict-Transport-Security present = %v", hsts, got)
		}
	}
}
