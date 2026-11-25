package respond

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
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
