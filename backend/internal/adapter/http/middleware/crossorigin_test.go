package middleware_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/http/middleware"
	"github.com/bergsantana/encomendo/backend/internal/adapter/http/respond"
)

const adminOrigin = "https://admin.encomendo.com.br"

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestCrossOrigin_NeedsABrowserOrigin(t *testing.T) {
	t.Parallel()

	good := []string{adminOrigin, "http://localhost:8080", "http://[::1]:5173"}
	bad := []string{
		"",
		"admin.encomendo.com.br",
		"https://admin.encomendo.com.br/",
		"https://admin.encomendo.com.br/login",
		"https://Admin.encomendo.com.br",
		"HTTPS://admin.encomendo.com.br",
		"https://admin.encomendo.com.br:443",
		"http://localhost:80",
		"https://admin.encomendo.com.br:",
		"https://admin.encomendo.com.br?x=1",
		"https://user@admin.encomendo.com.br",
		"ftp://admin.encomendo.com.br",
		"null",
	}
	for _, origin := range good {
		if _, err := middleware.CrossOrigin(origin, discard()); err != nil {
			t.Errorf("CrossOrigin(%q): %v", origin, err)
		}
	}
	for _, origin := range bad {
		if _, err := middleware.CrossOrigin(origin, discard()); err == nil {
			t.Errorf("CrossOrigin(%q) accepted an origin no browser sends", origin)
		}
	}
}

func TestCrossOrigin(t *testing.T) {
	t.Parallel()

	const json = "application/json"
	tests := []struct {
		name     string
		headers  map[string][]string
		wantPass bool
		wantCode string
	}{
		// ADR 0013: OkHttp and Ktor send neither header.
		{name: "native client", headers: map[string][]string{"Content-Type": {"application/json; charset=utf-8"}}, wantPass: true},
		{name: "the admin app", headers: map[string][]string{
			"Origin": {adminOrigin}, "Sec-Fetch-Site": {"same-origin"}, "Content-Type": {json},
		}, wantPass: true},
		{name: "the admin app, Origin only", headers: map[string][]string{"Origin": {adminOrigin}, "Content-Type": {json}}, wantPass: true},
		{name: "user-initiated", headers: map[string][]string{"Sec-Fetch-Site": {"none"}, "Content-Type": {json}}, wantPass: true},
		{name: "media type in upper case", headers: map[string][]string{"Content-Type": {"Application/JSON"}}, wantPass: true},

		{name: "a store page", headers: map[string][]string{
			"Origin": {"https://loja.encomendo.com.br"}, "Sec-Fetch-Site": {"same-site"}, "Content-Type": {json},
		}, wantCode: respond.CodeForbidden},
		{name: "another site", headers: map[string][]string{"Origin": {"https://evil.example"}, "Content-Type": {json}}, wantCode: respond.CodeForbidden},
		{name: "origin null", headers: map[string][]string{"Origin": {"null"}, "Content-Type": {json}}, wantCode: respond.CodeForbidden},
		{name: "the admin origin over http", headers: map[string][]string{"Origin": {"http://admin.encomendo.com.br"}, "Content-Type": {json}}, wantCode: respond.CodeForbidden},
		{name: "two Origin headers", headers: map[string][]string{"Origin": {adminOrigin, "https://evil.example"}, "Content-Type": {json}}, wantCode: respond.CodeForbidden},
		{name: "same-site without Origin", headers: map[string][]string{"Sec-Fetch-Site": {"same-site"}, "Content-Type": {json}}, wantCode: respond.CodeForbidden},
		{name: "cross-site without Origin", headers: map[string][]string{"Sec-Fetch-Site": {"cross-site"}, "Content-Type": {json}}, wantCode: respond.CodeForbidden},
		{name: "two Sec-Fetch-Site headers", headers: map[string][]string{"Sec-Fetch-Site": {"same-origin", "cross-site"}, "Content-Type": {json}}, wantCode: respond.CodeForbidden},
		{name: "an unknown Sec-Fetch-Site", headers: map[string][]string{"Sec-Fetch-Site": {"same-planet"}, "Content-Type": {json}}, wantCode: respond.CodeForbidden},
		{name: "the admin Origin but cross-site", headers: map[string][]string{
			"Origin": {adminOrigin}, "Sec-Fetch-Site": {"cross-site"}, "Content-Type": {json},
		}, wantCode: respond.CodeForbidden},
		{name: "a cross-site form is refused as cross-site", headers: map[string][]string{
			"Origin": {"https://evil.example"}, "Content-Type": {"application/x-www-form-urlencoded"},
		}, wantCode: respond.CodeForbidden},

		{name: "no Content-Type", headers: map[string][]string{}, wantCode: respond.CodeInvalid},
		{name: "text/plain", headers: map[string][]string{"Content-Type": {"text/plain"}}, wantCode: respond.CodeInvalid},
		{name: "a form", headers: map[string][]string{"Content-Type": {"application/x-www-form-urlencoded"}}, wantCode: respond.CodeInvalid},
		{name: "multipart", headers: map[string][]string{"Content-Type": {"multipart/form-data; boundary=x"}}, wantCode: respond.CodeInvalid},
		{name: "another JSON type", headers: map[string][]string{"Content-Type": {"application/merge-patch+json"}}, wantCode: respond.CodeInvalid},
		{name: "a malformed media type", headers: map[string][]string{"Content-Type": {"application/json; charset"}}, wantCode: respond.CodeInvalid},
		{name: "two Content-Type headers", headers: map[string][]string{"Content-Type": {json, "text/plain"}}, wantCode: respond.CodeInvalid},
	}
	guard, err := middleware.CrossOrigin(adminOrigin, discard())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reached := false
			h := guard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
			r := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{"refreshTransport":"body"}`))
			for name, values := range tt.headers {
				for _, v := range values {
					r.Header.Add(name, v)
				}
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)

			if reached != tt.wantPass {
				t.Fatalf("reached the handler = %v, want %v (answer %d %s)", reached, tt.wantPass, rec.Code, rec.Body.String())
			}
			if tt.wantPass {
				return
			}
			p := problem(t, rec)
			if p.Code != tt.wantCode {
				t.Errorf("code = %s, want %s", p.Code, tt.wantCode)
			}
			if tt.wantCode == respond.CodeInvalid && (rec.Code != http.StatusBadRequest || len(p.Errors) != 1 || p.Errors[0].Field != "Content-Type") {
				t.Errorf("answer = %d %+v, want 400 naming Content-Type", rec.Code, p)
			}
			if tt.wantCode == respond.CodeForbidden && rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", rec.Code)
			}
		})
	}
}
