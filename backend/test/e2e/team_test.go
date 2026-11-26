//go:build e2e

package e2e_test

import (
	"regexp"
	"testing"
	"time"
)

// Sign-up, a store, login, and the whole team flow against the real stack:
// invite, read the link from Mailpit, accept, change the role, remove
// (docs/build-plan.md, milestone 2, Done when).
func TestTeam_FromSignUpToRemoval(t *testing.T) {
	t.Parallel()

	owner := signUp(t, "Ana Souza")
	store := owner.newStore(t, "Alta RPM")
	login := must(t, call(t, "POST", "/auth/login", map[string]any{
		"email": owner.email, "password": owner.password, "refreshTransport": "body",
	}, ""), 200, "login")
	if tenant, _ := login.Body["tenant"].(map[string]any); tenant["id"] != store || login.str("role") != "owner" {
		t.Fatalf("login selected %v as %q, want the only store as owner", login.Body["tenant"], login.str("role"))
	}

	email := unique("convidada") + "@teste.encomendo.local"
	must(t, call(t, "POST", "/api/v1/admin/invitations", map[string]any{"email": email, "role": "staff"}, owner.access), 201, "invite")
	token := invitationLink(t, email)

	preview := must(t, call(t, "POST", "/auth/invitations/preview", map[string]any{"token": token}, ""), 200, "preview")
	if preview.Body["accountExists"] != false || preview.str("role") != "staff" {
		t.Errorf("preview = %v", preview.Body)
	}
	joined := &session{email: email, password: "segredo-forte-2"}
	accepted := must(t, call(t, "POST", "/auth/invitations/accept", map[string]any{
		"token": token, "name": "Bia Lima", "password": joined.password, "refreshTransport": "body",
	}, ""), 200, "accept")
	joined.take(accepted)
	if tenant, _ := accepted.Body["tenant"].(map[string]any); tenant["id"] != store || accepted.str("role") != "staff" {
		t.Fatalf("accept selected %v as %q", accepted.Body["tenant"], accepted.str("role"))
	}
	// The link works once.
	must(t, call(t, "POST", "/auth/invitations/preview", map[string]any{"token": token}, ""), 404, "preview a used link")
	must(t, call(t, "POST", "/auth/invitations/accept", map[string]any{
		"token": token, "name": "Outra Pessoa", "password": "segredo-forte-3",
	}, ""), 404, "accept a used link")

	memberID := memberWithEmail(t, owner, email)
	changed := must(t, call(t, "PUT", "/api/v1/admin/members/"+memberID+"/role", map[string]any{"role": "admin"}, owner.access), 200, "change role")
	if changed.str("role") != "admin" {
		t.Errorf("changed role = %v", changed.Body)
	}
	// A refresh reads the role again (ADR 0010).
	refreshed := must(t, call(t, "POST", "/auth/refresh", map[string]any{"refreshToken": joined.refresh}, ""), 200, "refresh after the change")
	if refreshed.str("role") != "admin" {
		t.Errorf("the refreshed role = %q, want admin", refreshed.str("role"))
	}
	joined.take(refreshed)

	must(t, call(t, "DELETE", "/api/v1/admin/members/"+memberID, nil, owner.access), 204, "remove")
	after := must(t, call(t, "POST", "/auth/refresh", map[string]any{"refreshToken": joined.refresh}, ""), 200, "refresh after removal")
	if _, ok := after.Body["tenant"]; ok {
		t.Errorf("a removed member's session kept the store: %v", after.Body["tenant"])
	}
	joined.take(after)
	if a := call(t, "GET", "/api/v1/admin/settings", nil, joined.access); a.Status != 403 || a.str("code") != "tenant_required" {
		t.Errorf("a removed member's call = %d %v, want 403 tenant_required", a.Status, a.Body)
	}
}

// memberWithEmail returns the user id of the member with email, through
// the team list.
func memberWithEmail(t *testing.T, s *session, email string) string {
	t.Helper()
	a := must(t, call(t, "GET", "/api/v1/admin/members", nil, s.access), 200, "list members")
	items, _ := a.Body["items"].([]any)
	for _, it := range items {
		m := it.(map[string]any)
		if m["email"] == email {
			return m["userId"].(string)
		}
	}
	t.Fatalf("no member with %s in %v", email, items)
	return ""
}

// resetCode is the code on its own indented line of the email.
var resetCode = regexp.MustCompile(`(?m)^\s+(\d{6})\s*$`)

// Esqueci a senha: the code reaches Mailpit, sets a new password, and every
// old session ends (ADR 0011).
func TestPasswordReset(t *testing.T) {
	t.Parallel()

	s := signUp(t, "Caio Reis")
	must(t, call(t, "POST", "/auth/password-reset", map[string]any{"email": s.email}, ""), 202, "request a code")
	code := waitForCode(t, s.email)

	must(t, call(t, "POST", "/auth/password-reset/confirm", map[string]any{
		"email": s.email, "code": code, "password": "outra-senha-forte",
	}, ""), 204, "confirm")
	must(t, call(t, "POST", "/auth/refresh", map[string]any{"refreshToken": s.refresh}, ""), 401, "an old session after the reset")
	must(t, call(t, "POST", "/auth/login", map[string]any{"email": s.email, "password": "outra-senha-forte"}, ""), 200, "login with the new password")
	must(t, call(t, "POST", "/auth/login", map[string]any{"email": s.email, "password": s.password}, ""), 401, "login with the old password")
}

func waitForCode(t *testing.T, to string) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var list struct {
			Messages []struct{ ID string } `json:"messages"`
		}
		getJSON(t, mailpitURL+"/api/v1/search?query=to%3A"+to, &list)
		for _, m := range list.Messages {
			var msg struct{ Text string }
			getJSON(t, mailpitURL+"/api/v1/message/"+m.ID, &msg)
			if match := resetCode.FindStringSubmatch(msg.Text); match != nil {
				return match[1]
			}
		}
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatalf("no reset code for %s in Mailpit", to)
	return ""
}
