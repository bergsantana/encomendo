//go:build e2e

// Package e2e_test checks milestone 2's "Done when" items against the built
// stack (make stack-up, or the CI e2e job): a Go client calling the API as
// a native client does, with Mailpit for email and a database superuser for
// the one thing only the platform does, suspending a store.
package e2e_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver for the owner connection
	goredis "github.com/redis/go-redis/v9"
)

// env returns the variable or its default: the CI stack's addresses.
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var (
	apiURL     = env("E2E_API_URL", "http://localhost:8081")
	mailpitURL = env("E2E_MAILPIT_URL", "http://localhost:8025")
	// The superuser of the local stack: tenants is under FORCE ROW LEVEL
	// SECURITY, so even its owner changes nothing without a tenant set.
	platformDBURL = env("E2E_PLATFORM_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/encomendo?sslmode=disable")
	redisURL      = env("E2E_REDIS_URL", "redis://localhost:6379/0")
	// The stack's issuer: the config default in development.
	issuer = env("E2E_ISSUER", "encomendo-development")
)

func TestMain(m *testing.M) {
	if err := clearRateLimits(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: clear rate limits:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// clearRateLimits drops every rate-limit count, so the per-IP limits of a
// local stack do not refuse a second run: every call here comes from one
// address (ADR 0021). E2E_REDIS_URL must only ever point at a throwaway
// stack's Redis, never a shared one.
func clearRateLimits() error {
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return err
	}
	rdb := goredis.NewClient(opts)
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	iter := rdb.Scan(ctx, 0, "g:rl:*", 500).Iterator()
	for key := range iter.Vals(ctx) {
		if err := rdb.Del(ctx, key).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

var run = fmt.Sprintf("%d", time.Now().UnixNano())

// client bounds every call, so a hung stack fails a test instead of
// waiting for the go test timeout.
var client = &http.Client{Timeout: 15 * time.Second}

var seq atomic.Int64

// unique returns a name no earlier run used.
func unique(prefix string) string {
	return fmt.Sprintf("%s-%s-%d", prefix, run, seq.Add(1))
}

// answer is one API answer.
type answer struct {
	Status int
	Body   map[string]any
	Header http.Header
}

func (a answer) str(key string) string {
	s, _ := a.Body[key].(string)
	return s
}

// call sends method path with body (nil for none) and token (empty for
// none), as a native client: no Origin, no Sec-Fetch-Site.
func call(t *testing.T, method, path string, body any, token string) answer {
	t.Helper()
	return callWith(t, method, path, body, token, nil)
}

// callWith is call with more request headers, such as an Idempotency-Key.
func callWith(t *testing.T, method, path string, body any, token string, headers map[string]string) answer {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, apiURL+path, buf)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	out := answer{Status: res.StatusCode, Header: res.Header}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out.Body) // a non-JSON body leaves Body nil
	}
	return out
}

// must fails t unless a has status.
func must(t *testing.T, a answer, status int, what string) answer {
	t.Helper()
	if a.Status != status {
		t.Fatalf("%s: answer %d %v, want %d", what, a.Status, a.Body, status)
	}
	return a
}

// session is a signed-in staff member, with the body transport.
type session struct {
	email, password string
	access, refresh string
	userID          string
	store           string
}

// signUp creates an account and returns its session, without a store.
func signUp(t *testing.T, name string) *session {
	t.Helper()
	s := &session{email: unique("e2e") + "@teste.encomendo.local", password: "segredo-forte-1"}
	a := must(t, call(t, "POST", "/auth/signup", map[string]any{
		"name": name, "email": s.email, "password": s.password, "refreshTransport": "body",
	}, ""), 201, "sign-up")
	s.take(a)
	user, _ := a.Body["user"].(map[string]any)
	s.userID, _ = user["id"].(string)
	return s
}

func (s *session) take(a answer) {
	s.access, s.refresh = a.str("accessToken"), a.str("refreshToken")
}

// newStore creates a store and switches the session into it.
func (s *session) newStore(t *testing.T, name string) string {
	t.Helper()
	a := must(t, call(t, "POST", "/api/v1/admin/tenants", map[string]any{"name": name, "slug": unique("loja")}, s.access), 201, "create store")
	id := a.str("id")
	s.switchTo(t, id)
	return id
}

func (s *session) switchTo(t *testing.T, store string) {
	t.Helper()
	s.take(must(t, call(t, "POST", "/auth/switch-tenant", map[string]any{"tenantId": store, "refreshToken": s.refresh}, ""), 200, "switch"))
}

var linkToken = regexp.MustCompile(`/convite#token=([A-Za-z0-9_-]+)`)

// invitationLink waits for the invitation email to to reach Mailpit and
// returns the token in its link.
func invitationLink(t *testing.T, to string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if token := findInvitation(t, to); token != "" {
			return token
		}
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatalf("no invitation email for %s in Mailpit", to)
	return ""
}

func findInvitation(t *testing.T, to string) string {
	t.Helper()
	var list struct {
		Messages []struct{ ID string } `json:"messages"`
	}
	getJSON(t, mailpitURL+"/api/v1/search?query="+strings.ReplaceAll("to:"+to, "@", "%40"), &list)
	for _, m := range list.Messages {
		var msg struct{ Text string }
		getJSON(t, mailpitURL+"/api/v1/message/"+m.ID, &msg)
		if match := linkToken.FindStringSubmatch(msg.Text); match != nil {
			return match[1]
		}
	}
	return ""
}

func getJSON(t *testing.T, url string, dst any) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
}

// suspend suspends a store as the platform does, by SQL outside the API
// (ADR 0022). It fails unless exactly that store changed.
func suspend(t *testing.T, store string) {
	t.Helper()
	db, err := sql.Open("pgx", platformDBURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	res, err := db.ExecContext(t.Context(), `UPDATE tenants SET status = 'suspended' WHERE id = $1`, store)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("suspending %s changed %d rows (%v), want 1", store, n, err)
	}
}
