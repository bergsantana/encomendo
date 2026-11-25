package tenancy

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var invitationID = shared.InvitationID(shared.NewV7(t0.UnixMilli(), [10]byte{4}))

func mustEmail(t *testing.T, s string) identity.Email {
	t.Helper()
	e, err := identity.ParseEmail(s)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func spec(t *testing.T) InvitationSpec {
	t.Helper()
	return InvitationSpec{
		ID: invitationID, Tenant: tenantID, Email: mustEmail(t, "novo@altarpm.com.br"),
		Role: shared.RoleStaff, Hash: shared.Secret{7}.Hash(), InvitedBy: ownerID,
	}
}

func TestNewInvitation_LastsSevenDays(t *testing.T) {
	t.Parallel()

	inv, err := NewInvitation(spec(t), t0)
	if err != nil {
		t.Fatal(err)
	}
	if !inv.ExpiresAt().Equal(t0.Add(7*24*time.Hour)) || !inv.CreatedAt().Equal(t0) {
		t.Errorf("created %v, expires %v; want 7 days", inv.CreatedAt(), inv.ExpiresAt())
	}
	if inv.ID() != invitationID || inv.TenantID() != tenantID || inv.Email().String() != "novo@altarpm.com.br" ||
		inv.Role() != shared.RoleStaff || inv.Hash() != (shared.Secret{7}.Hash()) || inv.InvitedBy() != ownerID {
		t.Errorf("invitation lost a field: %+v", inv)
	}
	if !inv.PendingAt(t0) {
		t.Error("a new invitation is not pending")
	}
}

func TestNewInvitation_Refuses(t *testing.T) {
	t.Parallel()

	owner := spec(t)
	owner.Role = shared.RoleOwner
	var ve *shared.ValidationError
	if _, err := NewInvitation(owner, t0); !errors.As(err, &ve) || ve.Fields[0].Field != "role" {
		t.Errorf("owner invitation: %v, want a ValidationError on role", err)
	}
	for name, mutate := range map[string]func(*InvitationSpec){
		"zero id":      func(s *InvitationSpec) { s.ID = shared.InvitationID{} },
		"zero tenant":  func(s *InvitationSpec) { s.Tenant = shared.TenantID{} },
		"zero email":   func(s *InvitationSpec) { s.Email = identity.Email{} },
		"zero hash":    func(s *InvitationSpec) { s.Hash = shared.SecretHash{} },
		"zero inviter": func(s *InvitationSpec) { s.InvitedBy = shared.UserID{} },
	} {
		s := spec(t)
		mutate(&s)
		if _, err := NewInvitation(s, t0); !errors.Is(err, ErrInvalidInvitation) {
			t.Errorf("%s: %v, want ErrInvalidInvitation", name, err)
		}
	}
}

func TestInvitation_PendingAt(t *testing.T) {
	t.Parallel()

	inv, _ := NewInvitation(spec(t), t0)
	if !inv.PendingAt(inv.ExpiresAt().Add(-time.Second)) || inv.PendingAt(inv.ExpiresAt()) {
		t.Error("an invitation is pending until, and not at, its expiry")
	}

	accepted, _ := NewInvitation(spec(t), t0)
	if err := accepted.Accept(t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if accepted.PendingAt(t0.Add(2*time.Hour)) || !accepted.AcceptedAt().Equal(t0.Add(time.Hour)) {
		t.Error("an accepted invitation is still pending")
	}

	revoked, _ := NewInvitation(spec(t), t0)
	if err := revoked.Revoke(t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if revoked.PendingAt(t0.Add(2*time.Hour)) || !revoked.RevokedAt().Equal(t0.Add(time.Hour)) {
		t.Error("a revoked invitation is still pending")
	}
}

func TestInvitation_WorksOnce(t *testing.T) {
	t.Parallel()

	// Accepted, revoked and expired invitations all answer the same: 404.
	inv, _ := NewInvitation(spec(t), t0)
	_ = inv.Accept(t0.Add(time.Minute)) // the first use works; checked above
	if err := inv.Accept(t0.Add(2 * time.Minute)); !errors.Is(err, ErrInvitationNotPending) {
		t.Errorf("second Accept: %v, want ErrInvitationNotPending", err)
	}
	if err := inv.Revoke(t0.Add(2 * time.Minute)); !errors.Is(err, ErrInvitationNotPending) {
		t.Errorf("Revoke after Accept: %v, want ErrInvitationNotPending", err)
	}
	expired, _ := NewInvitation(spec(t), t0)
	if err := expired.Accept(t0.Add(InvitationLifetime)); !errors.Is(err, ErrInvitationNotPending) {
		t.Errorf("Accept at expiry: %v, want ErrInvitationNotPending", err)
	}
	if !errors.Is(ErrInvitationNotPending, shared.ErrNotFound) {
		t.Error("ErrInvitationNotPending must be a 404")
	}
}

func TestRestoreInvitation(t *testing.T) {
	t.Parallel()

	rec := InvitationRecord{
		ID: invitationID, Tenant: tenantID, Email: "novo@altarpm.com.br", Role: shared.RoleViewer,
		Hash: shared.Secret{1}.Hash(), InvitedBy: ownerID, CreatedAt: t0, ExpiresAt: t0.Add(time.Hour),
		RevokedAt: t0.Add(time.Minute),
	}
	inv, err := RestoreInvitation(rec)
	if err != nil {
		t.Fatal(err)
	}
	if inv.PendingAt(t0.Add(2*time.Minute)) || inv.Role() != shared.RoleViewer {
		t.Errorf("restored invitation: pending %v, role %q", inv.PendingAt(t0.Add(2*time.Minute)), inv.Role())
	}
	both := rec
	both.AcceptedAt = t0.Add(time.Minute)
	badEmail := rec
	badEmail.Email = "nope"
	owner := rec
	owner.Role = shared.RoleOwner
	noTenant := rec
	noTenant.Tenant = shared.TenantID{}
	for name, r := range map[string]InvitationRecord{
		"accepted and revoked": both, "bad email": badEmail, "owner": owner, "zero tenant": noTenant,
	} {
		if _, err := RestoreInvitation(r); !errors.Is(err, ErrInvalidInvitation) {
			t.Errorf("%s: %v, want ErrInvalidInvitation", name, err)
		}
	}
}

func TestInvitationToken_RoundTrips(t *testing.T) {
	t.Parallel()

	secret := shared.Secret{1, 2, 3, 4}
	tok := NewInvitationToken(tenantID, secret)
	text := tok.Reveal()
	if len(text) != 64 || strings.ContainsAny(text, "+/=.") {
		t.Fatalf("token %q: want 64 URL-safe characters", text)
	}
	back, err := ParseInvitationToken(text)
	if err != nil {
		t.Fatal(err)
	}
	// The tenant travels in the token, so accepting can scope its
	// transaction before reading anything; only the secret's hash is stored
	// (ADR 0011).
	if back.TenantID() != tenantID || back.Hash() != secret.Hash() {
		t.Errorf("parsed tenant %v, hash %x", back.TenantID(), back.Hash())
	}
}

func TestParseInvitationToken_RefusesAsNotFound(t *testing.T) {
	t.Parallel()

	valid := NewInvitationToken(tenantID, shared.Secret{9}).Reveal()
	zeroTenant := NewInvitationToken(shared.TenantID{}, shared.Secret{9}).Reveal()
	for name, in := range map[string]string{
		"empty":               "",
		"too short":           valid[:63],
		"too long":            valid + "A",
		"bad alphabet":        strings.Repeat("*", 64),
		"zero tenant":         zeroTenant,
		"refresh token shape": shared.Secret{1}.Reveal(),
	} {
		if _, err := ParseInvitationToken(in); !errors.Is(err, ErrInvitationNotPending) {
			t.Errorf("%s: %v, want ErrInvitationNotPending (the same 404 as any bad link)", name, err)
		}
	}
}

func TestInvitationToken_NeverPrintsItsValue(t *testing.T) {
	t.Parallel()

	tok := NewInvitationToken(tenantID, shared.Secret{4, 5})
	out := fmt.Sprintf("%s %v %+v %#v", tok, tok, struct{ Link InvitationToken }{tok}, tok)
	if strings.Contains(out, tok.Reveal()) || !strings.Contains(out, shared.Redacted) {
		t.Errorf("printed %q", out)
	}
}

func FuzzParseInvitationToken(f *testing.F) {
	f.Add(NewInvitationToken(tenantID, shared.Secret{5}).Reveal())
	f.Add("garbage")
	f.Fuzz(func(t *testing.T, in string) {
		tok, err := ParseInvitationToken(in)
		if err != nil {
			return
		}
		if tok.Reveal() != in || tok.TenantID().IsZero() {
			t.Errorf("accepted %q, which reads back as %q with tenant %v", in, tok.Reveal(), tok.TenantID())
		}
	})
}
