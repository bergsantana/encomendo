package identity_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// invite stores a pending invitation of email to a new store and returns
// the link's token.
func (h *harness) invite(t *testing.T, email string, role shared.Role) (string, appidentity.TenantSummary) {
	t.Helper()
	st := h.store1("convite")
	secret, err := h.secrets.NewSecret(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	addr, err := identity.ParseEmail(email)
	if err != nil {
		t.Fatal(err)
	}
	h.invites.Add(secret.Hash(), appidentity.PendingInvitation{
		ID: shared.InvitationID(h.ids.NewID()), Tenant: st, Email: addr, Role: role, ExpiresAt: start.Add(7 * 24 * time.Hour),
	})
	return tenancy.NewInvitationToken(st.ID, secret).Reveal(), st
}

func (h *harness) accept(token, name, password string) (appidentity.AuthResult, error) {
	return h.svc.AcceptInvitation(context.Background(), appidentity.AcceptInvitationInput{Token: token, Name: name, Password: password})
}

func TestPreviewInvitation(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token, st := h.invite(t, "novo@ex.com", shared.RoleStaff)

	p, err := h.svc.PreviewInvitation(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}

	if p.Tenant != st || p.Email != "novo@ex.com" || p.Role != shared.RoleStaff || p.AccountExists {
		t.Errorf("preview = %+v", p)
	}
	// The scope is the tenant inside the token, and nothing else (ADR 0011).
	if got := h.tx.Scopes(); !slices.Equal(got, []appshared.Scope{{TenantID: st.ID}}) {
		t.Errorf("scopes = %+v", got)
	}

	h.signUp(t, "NOVO@ex.com", "segredo-forte")
	if p, _ := h.svc.PreviewInvitation(context.Background(), token); !p.AccountExists {
		t.Error("accountExists is false for an email with an account")
	}
}

func TestPreviewInvitation_BadLinksAreNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token, st := h.invite(t, "novo@ex.com", shared.RoleStaff)

	otherStore := tenancy.NewInvitationToken(shared.TenantID(h.ids.NewID()), shared.Secret{1}).Reveal()
	for name, tok := range map[string]string{
		"malformed":      "nope",
		"unknown secret": tenancy.NewInvitationToken(st.ID, shared.Secret{1}).Reveal(),
		"another store":  otherStore,
	} {
		if _, err := h.svc.PreviewInvitation(context.Background(), tok); !errors.Is(err, tenancy.ErrInvitationNotPending) {
			t.Errorf("%s: %v, want ErrInvitationNotPending", name, err)
		}
	}
	h.clock.Advance(7 * 24 * time.Hour)
	if _, err := h.svc.PreviewInvitation(context.Background(), token); !errors.Is(err, tenancy.ErrInvitationNotPending) {
		t.Errorf("expired: %v", err)
	}
}

func TestAcceptInvitation_NewAccount(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token, st := h.invite(t, "novo@ex.com", shared.RoleViewer)

	res, err := h.accept(token, "Bruno Lima", "senha-do-bruno")
	if err != nil {
		t.Fatal(err)
	}

	if res.Session.Tenant != st || res.Session.Role != shared.RoleViewer || res.Session.User.Name != "Bruno Lima" {
		t.Errorf("session = %+v", res.Session)
	}
	if p := h.lastClaims(t).Principal; p.TenantID() != st.ID || p.Role() != shared.RoleViewer {
		t.Errorf("access token for %v as %q", p.TenantID(), p.Role())
	}
	if j := h.invites.Joins(); len(j) != 1 || j[0].User != res.Session.User.ID || j[0].Email.String() != "novo@ex.com" {
		t.Errorf("joins = %+v", j)
	}
	if _, err := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "novo@ex.com", Password: "senha-do-bruno"}); err != nil {
		t.Errorf("the new account cannot log in: %v", err)
	}
	// Tenant scope to read the invitation, tenant scope to use it up, user
	// scope to build the session (ADR 0009, ADR 0011).
	want := []appshared.Scope{{TenantID: st.ID}, {TenantID: st.ID}, {UserID: res.Session.User.ID}}
	if got := h.tx.Scopes()[:3]; !slices.Equal(got, want) {
		t.Errorf("scopes = %+v, want %+v", got, want)
	}
}

func TestAcceptInvitation_NewAccountNeedsAValidNameAndPassword(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token, _ := h.invite(t, "novo@ex.com", shared.RoleStaff)

	_, err := h.accept(token, "", "curta")

	if got := fieldsOf(t, err); !slices.Equal(got, []string{"name", "password"}) {
		t.Errorf("fields = %v", got)
	}
	if len(h.invites.Joins()) != 0 || h.hasher.Hashes() != 0 {
		t.Error("invalid input joined the store or reached the hasher")
	}
}

func TestAcceptInvitation_ExistingAccount(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.signUp(t, "ana@ex.com", "segredo-forte").Session.User.ID
	token, st := h.invite(t, "Ana@Ex.com", shared.RoleAdmin)

	// A wrong password never joins: holding the link is not enough (ADR 0011).
	if _, err := h.accept(token, "", "errada"); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v, want ErrInvalidCredentials", err)
	}
	if len(h.invites.Joins()) != 0 {
		t.Fatal("a wrong password joined the store")
	}

	res, err := h.accept(token, "Nome Ignorado", "segredo-forte")
	if err != nil {
		t.Fatal(err)
	}
	if res.Session.User.ID != user || res.Session.User.Name != "Ana Souza" || res.Session.Tenant != st || res.Session.Role != shared.RoleAdmin {
		t.Errorf("session = %+v", res.Session)
	}
}

func TestAcceptInvitation_AlreadyMember(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.signUp(t, "ana@ex.com", "segredo-forte").Session.User.ID
	token, st := h.invite(t, "ana@ex.com", shared.RoleStaff)
	h.invites.AddMember(st.ID, user)

	if _, err := h.accept(token, "", "segredo-forte"); !errors.Is(err, tenancy.ErrAlreadyMember) {
		t.Errorf("err = %v, want ErrAlreadyMember", err)
	}
	if _, err := h.svc.PreviewInvitation(context.Background(), token); err != nil {
		t.Errorf("the invitation was used up: %v", err)
	}
}

func TestAcceptInvitation_RefusesWhenThePasswordChangedMeanwhile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.signUp(t, "ana@ex.com", "segredo-forte").Session.User.ID
	token, _ := h.invite(t, "ana@ex.com", shared.RoleStaff)
	h.store.OnCall("PasswordHashForShare", func() { resetPasswordBehindTheScenes(t, h, user) })

	_, err := h.accept(token, "", "segredo-forte")

	if !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if len(h.invites.Joins()) != 0 {
		t.Error("a stale password joined the store")
	}
}

func TestAcceptInvitation_EmailTakenMeanwhile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	token, _ := h.invite(t, "novo@ex.com", shared.RoleStaff)
	// Someone signs up with the invited email between the read and the
	// insert.
	h.store.OnCall("InsertUser", func() {
		h.store.OnCall("InsertUser", nil)
		h.signUp(t, "novo@ex.com", "outra-senha-qualquer")
	})

	if _, err := h.accept(token, "Bruno", "senha-do-bruno"); !errors.Is(err, identity.ErrEmailTaken) {
		t.Errorf("err = %v, want ErrEmailTaken", err)
	}
}

func TestAcceptInvitation_BadLinkOrInput(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	if _, err := h.accept("nope", "Bruno", "senha-do-bruno"); !errors.Is(err, tenancy.ErrInvitationNotPending) {
		t.Errorf("malformed link: %v", err)
	}
	token, _ := h.invite(t, "novo@ex.com", shared.RoleStaff)
	if got := fieldsOf(t, func() error { _, err := h.accept(token, "Bruno", string(make([]byte, 129))); return err }()); !slices.Equal(got, []string{"password"}) {
		t.Errorf("long password: fields %v", got)
	}
}

func TestAcceptInvitation_FailuresStayServerErrors(t *testing.T) {
	t.Parallel()

	for name, breakIt := range map[string]func(h *harness){
		"invitation read": func(h *harness) { h.invites.Fail(errBoom) },
		"user lookup":     func(h *harness) { h.store.Fail("UserByEmail", errBoom) },
		"hasher":          func(h *harness) { h.hasher.Fail(errBoom) },
		"insert user":     func(h *harness) { h.store.Fail("InsertUser", errBoom) },
		"memberships":     func(h *harness) { h.members.Fail(errBoom) },
		"family":          func(h *harness) { h.store.Fail("CreateFamily", errBoom) },
		"secrets":         func(h *harness) { h.secrets.Fail(errBoom) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			token, _ := h.invite(t, "novo@ex.com", shared.RoleStaff)
			breakIt(h)
			_, err := h.accept(token, "Bruno", "senha-do-bruno")
			if !errors.Is(err, errBoom) || errors.Is(err, shared.ErrUnauthenticated) || errors.Is(err, shared.ErrNotFound) {
				t.Errorf("err = %v, want the failure itself", err)
			}
		})
	}
	t.Run("hash check", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.signUp(t, "ana@ex.com", "segredo-forte")
		token, _ := h.invite(t, "ana@ex.com", shared.RoleStaff)
		h.store.Fail("PasswordHashForShare", errBoom)
		if _, err := h.accept(token, "", "segredo-forte"); !errors.Is(err, errBoom) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestDirectory(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ana := h.signUp(t, "ana@ex.com", "segredo-forte").Session.User
	dir := appidentity.NewDirectory(h.store)

	err := h.tx.WithinTx(context.Background(), func(ctx context.Context) error {
		email, _ := identity.ParseEmail("ANA@ex.com")
		got, err := dir.UserByEmail(ctx, email)
		if err != nil || got != ana {
			t.Errorf("UserByEmail = %+v, %v; want %+v", got, err, ana)
		}
		unknown, _ := identity.ParseEmail("x@ex.com")
		if _, err := dir.UserByEmail(ctx, unknown); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("unknown email: %v", err)
		}
		users, err := dir.UsersByID(ctx, []shared.UserID{ana.ID, shared.UserID(h.ids.NewID())})
		if err != nil || len(users) != 1 || users[0] != ana {
			t.Errorf("UsersByID = %+v, %v", users, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	h.store.Fail("UsersByID", errBoom)
	_ = h.tx.WithinTx(context.Background(), func(ctx context.Context) error {
		if _, err := dir.UsersByID(ctx, []shared.UserID{ana.ID}); !errors.Is(err, errBoom) {
			t.Errorf("UsersByID failure: %v", err)
		}
		return nil
	})
}
