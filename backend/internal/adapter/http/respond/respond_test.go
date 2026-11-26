package respond

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type input struct {
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

func TestDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		maxBytes  int64
		wantErr   bool
		wantField string
	}{
		{name: "valid object", body: `{"name":"Banco de couro","qty":1}`, maxBytes: DefaultMaxBodyBytes},
		{name: "unknown field", body: `{"name":"x","price":10}`, maxBytes: DefaultMaxBodyBytes, wantErr: true, wantField: "price"},
		{name: "wrong type", body: `{"qty":"one"}`, maxBytes: DefaultMaxBodyBytes, wantErr: true, wantField: "qty"},
		{name: "malformed", body: `{"name":`, maxBytes: DefaultMaxBodyBytes, wantErr: true, wantField: "body"},
		{name: "empty", body: ``, maxBytes: DefaultMaxBodyBytes, wantErr: true, wantField: "body"},
		{name: "trailing data", body: `{"name":"x"}{"name":"y"}`, maxBytes: DefaultMaxBodyBytes, wantErr: true, wantField: "body"},
		{name: "too large", body: `{"name":"` + strings.Repeat("a", 64) + `"}`, maxBytes: 16, wantErr: true, wantField: "body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			var dst input
			err := Decode(httptest.NewRecorder(), r, &dst, tt.maxBytes)

			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Decode: %v", err)
				}
				return
			}
			var ve *shared.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Decode error = %v, want *shared.ValidationError", err)
			}
			if ve.Fields[0].Field != tt.wantField {
				t.Errorf("field = %q, want %q", ve.Fields[0].Field, tt.wantField)
			}
		})
	}
}

func TestError_MapsDomainErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"validation", shared.NewValidationError(shared.FieldError{Field: "phone", Message: "Obrigatório."}), 400, CodeInvalid},
		{"invalid", fmt.Errorf("parse: %w", shared.ErrInvalid), 400, CodeInvalid},
		{"not found", fmt.Errorf("load order: %w", shared.ErrNotFound), 404, CodeNotFound},
		{"conflict", shared.ErrConflict, 409, CodeConflict},
		{"forbidden", shared.ErrForbidden, 403, CodeForbidden},
		{"unknown", errors.New("connection refused"), 500, CodeInternal},
		{"unavailable", fmt.Errorf("hash password: %w", shared.ErrUnavailable), 503, CodeNotReady},
		{"deadline", fmt.Errorf("load user: %w", context.DeadlineExceeded), 503, CodeNotReady},
		{"commit conflict after the deadline", fmt.Errorf("transaction: %w: %w", context.DeadlineExceeded, shared.ErrConflict), 503, CodeNotReady},
		// A failure is never a 401 (ADR 0013), even wrapped with a refusal.
		{"refusal and outage", fmt.Errorf("%w: %w", shared.ErrUnauthenticated, shared.ErrUnavailable), 503, CodeNotReady},
		{"refusal and deadline", fmt.Errorf("%w: %w", shared.ErrUnauthenticated, context.DeadlineExceeded), 503, CodeNotReady},
		{"email unavailable", fmt.Errorf("invite: %w", apptenancy.ErrEmailUnavailable), 503, CodeEmailUnavailable},
		{"unauthenticated", shared.ErrUnauthenticated, 401, CodeUnauthenticated},
		{"invalid principal", shared.ErrInvalidPrincipal, 401, CodeUnauthenticated},
		{"invalid credentials", identity.ErrInvalidCredentials, 401, CodeInvalidCredentials},
		{"tenant required", authz.ErrTenantRequired, 403, CodeTenantRequired},
		{"tenant suspended", fmt.Errorf("switch: %w", tenancy.ErrTenantSuspended), 403, CodeTenantSuspended},
		{"scope mismatch", authz.ErrScopeMismatch, 403, CodeForbidden},
		{"owner fixed", tenancy.ErrOwnerFixed, 403, CodeForbidden},
		{"version conflict", fmt.Errorf("save settings: %w", shared.ErrVersionConflict), 409, CodeVersionConflict},
		{"email taken", identity.ErrEmailTaken, 409, CodeEmailTaken},
		{"slug taken", tenancy.ErrSlugTaken, 409, CodeSlugTaken},
		{"already member", tenancy.ErrAlreadyMember, 409, CodeAlreadyMember},
		{"invalid code", identity.ErrInvalidCode, 400, CodeInvalidCode},
		{"invitation not pending", tenancy.ErrInvitationNotPending, 404, CodeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			Error(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders/1", nil), discardLogger(), tt.err)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q", ct)
			}
			var p Problem
			if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if p.Code != tt.wantCode || p.Status != tt.wantStatus || p.Instance != "/api/v1/admin/orders/1" {
				t.Errorf("problem = %+v", p)
			}
		})
	}
}

func TestError_ListsFieldIssues(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	err := shared.NewValidationError(shared.FieldError{Field: "phone", Message: "Informe um WhatsApp válido."})
	Error(w, httptest.NewRequest(http.MethodPost, "/", nil), discardLogger(), err)

	var p Problem
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Errors) != 1 || p.Errors[0].Field != "phone" {
		t.Errorf("errors = %+v", p.Errors)
	}
}

func TestError_HidesInternalDetail(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	Error(w, httptest.NewRequest(http.MethodGet, "/", nil), discardLogger(), errors.New("password=secret"))

	if strings.Contains(w.Body.String(), "secret") {
		t.Errorf("internal error detail leaked: %s", w.Body.String())
	}
}

func TestJSON_WritesBody(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	JSON(w, httptest.NewRequest(http.MethodGet, "/", nil), discardLogger(), http.StatusCreated, map[string]string{"status": "ok"})

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"status":"ok"}` {
		t.Errorf("body = %s", got)
	}
}

// logLines decodes the JSON log lines in buf.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("log line is not JSON: %s", line)
		}
		out = append(out, entry)
	}
	return out
}

func debugLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestError_CanceledRequestIsTheClientLeaving(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantLevel string // the one level logged
	}{
		{name: "the cancellation itself", err: fmt.Errorf("load user: %w", context.Canceled), wantLevel: "DEBUG"},
		// The TxManager reports both; the cancellation wins (ADR 0019).
		{name: "a commit conflict after the client left", err: fmt.Errorf("transaction: %w: %w", context.Canceled, shared.ErrConflict), wantLevel: "DEBUG"},
		// Work on a detached context, such as a logout's revoke, can still
		// fail; an operator must see it.
		{name: "a real failure after the client left", err: errors.New("revoke family: connection reset"), wantLevel: "ERROR"},
		{name: "an outage after the client left", err: fmt.Errorf("read denylist: %w", shared.ErrUnavailable), wantLevel: "WARN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			var logs bytes.Buffer
			w := httptest.NewRecorder()
			Error(w, httptest.NewRequestWithContext(ctx, http.MethodPost, "/auth/logout", nil), debugLogger(&logs), tt.err)

			if w.Code != StatusClientClosedRequest {
				t.Errorf("status = %d, want %d", w.Code, StatusClientClosedRequest)
			}
			if w.Body.Len() != 0 {
				t.Errorf("body = %q, want none: nobody reads it", w.Body.String())
			}
			lines := logLines(t, &logs)
			if len(lines) != 1 || lines[0]["level"] != tt.wantLevel {
				t.Errorf("logged %v, want one %s line", lines, tt.wantLevel)
			}
		})
	}
}

// A cancellation while the client is still connected is the server's own
// doing, such as code that canceled its own context: a logged 500.
func TestError_CancellationTheClientDidNotCauseIs500(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	w := httptest.NewRecorder()
	Error(w, httptest.NewRequest(http.MethodPost, "/auth/login", nil), debugLogger(&logs), fmt.Errorf("hash: %w", context.Canceled))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if lines := logLines(t, &logs); len(lines) != 1 || lines[0]["level"] != "ERROR" {
		t.Errorf("logged %v, want one error line", lines)
	}
}

func TestError_Headers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		header    string
		wantValue string
	}{
		{"401 asks for a bearer token", shared.ErrUnauthenticated, "WWW-Authenticate", "Bearer"},
		{"401 invalid_credentials too", identity.ErrInvalidCredentials, "WWW-Authenticate", "Bearer"},
		{"403 does not", shared.ErrForbidden, "WWW-Authenticate", ""},
		{"503 not_ready says when to retry", shared.ErrUnavailable, HeaderRetryAfter, "5"},
		{"503 after the deadline too", context.DeadlineExceeded, HeaderRetryAfter, "5"},
		{"500 does not", errors.New("bug"), HeaderRetryAfter, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			Error(w, httptest.NewRequest(http.MethodGet, "/", nil), discardLogger(), tt.err)

			if got := w.Header().Get(tt.header); got != tt.wantValue {
				t.Errorf("%s = %q, want %q", tt.header, got, tt.wantValue)
			}
		})
	}
}

func TestError_LogsWhatAnOperatorMustSee(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantLevel string // "" means nothing is logged
	}{
		{"500 at error", errors.New("bug"), "ERROR"},
		{"503 at warn", fmt.Errorf("read denylist: %w", shared.ErrUnavailable), "WARN"},
		{"scope mismatch at warn", authz.ErrScopeMismatch, "WARN"},
		{"401 not at all", shared.ErrUnauthenticated, ""},
		{"other 403 not at all", authz.ErrTenantRequired, ""},
		{"400 not at all", shared.ErrInvalid, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer
			Error(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), debugLogger(&logs), tt.err)

			lines := logLines(t, &logs)
			if tt.wantLevel == "" {
				if len(lines) != 0 {
					t.Errorf("logged %v, want nothing", lines)
				}
				return
			}
			if len(lines) != 1 || lines[0]["level"] != tt.wantLevel || lines[0]["err"] != tt.err.Error() {
				t.Errorf("logged %v, want one %s line with the error", lines, tt.wantLevel)
			}
		})
	}
}

func TestRateLimited_RetryAfterInWholeSecondsAtLeastOne(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wait time.Duration
		want string
	}{
		{-time.Second, "1"},
		{0, "1"},
		{time.Millisecond, "1"},
		{time.Second, "1"},
		{1500 * time.Millisecond, "2"},
		{30 * time.Second, "30"},
		{10 * time.Minute, "600"},
	}
	for _, tt := range tests {
		t.Run(tt.wait.String(), func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			RateLimited(w, httptest.NewRequest(http.MethodPost, "/auth/login", nil), tt.wait)

			if got := w.Header().Get(HeaderRetryAfter); got != tt.want {
				t.Errorf("Retry-After = %q, want %q", got, tt.want)
			}
			var p Problem
			if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusTooManyRequests || p.Code != CodeRateLimited || p.Instance != "/auth/login" {
				t.Errorf("status = %d, problem = %+v", w.Code, p)
			}
		})
	}
}

// TestCodes_AreInTheContract keeps every code this package sends in the
// Problem schema of docs/api/openapi.yaml, because clients branch on them.
func TestCodes_AreInTheContract(t *testing.T) {
	t.Parallel()

	spec, err := os.ReadFile("../../../../../docs/api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	enum := regexp.MustCompile(`(?s)Stable machine-readable code\..*?enum:\n((?:\s+- [a-z_]+\n)+)`).FindSubmatch(spec)
	if enum == nil {
		t.Fatal("the Problem code enum is not where this test looks for it")
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(enum[1])), "\n") {
		listed[strings.TrimPrefix(strings.TrimSpace(line), "- ")] = true
	}
	codes := []string{
		CodeInvalid, CodeNotFound, CodeConflict, CodeForbidden, CodeInternal, CodeNotReady, CodeMethodNotAllowed,
		CodeUnauthenticated, CodeInvalidCredentials, CodeTenantRequired, CodeVersionConflict, CodeRateLimited,
		CodeEmailTaken, CodeSlugTaken, CodeAlreadyMember, CodeInvalidCode, CodeEmailUnavailable, CodeTenantSuspended,
	}
	for _, code := range codes {
		if !listed[code] {
			t.Errorf("code %q is not in the contract's Problem enum", code)
		}
	}
}
