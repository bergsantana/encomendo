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
)

// account stores a user directly, with no session, and returns its id.
func (h *harness) account(t *testing.T, email, password string) shared.UserID {
	t.Helper()
	hash, err := h.hasher.Hash(context.Background(), password)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := identity.ParsePersonName("Ana Souza")
	addr, err := identity.ParseEmail(email)
	if err != nil {
		t.Fatal(err)
	}
	u, err := identity.NewUser(shared.UserID(h.ids.NewID()), identity.Profile{Name: name, Email: addr}, hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.tx.WithinTx(context.Background(), func(ctx context.Context) error { return h.store.InsertUser(ctx, u) }); err != nil {
		t.Fatal(err)
	}
	return u.ID()
}

// session makes a staff member of one store and logs them in there; theirs
// is the only family.
func (h *harness) session(t *testing.T) (appidentity.AuthResult, appidentity.TenantSummary) {
	t.Helper()
	user := h.account(t, "ana@ex.com", "segredo-forte")
	st := h.store1("alta")
	h.members.Add(user, appidentity.Membership{Tenant: st, Role: shared.RoleStaff})
	res, err := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "ana@ex.com", Password: "segredo-forte"})
	if err != nil {
		t.Fatal(err)
	}
	return res, st
}

func (h *harness) refresh(token string) (appidentity.AuthResult, error) {
	return h.svc.Refresh(context.Background(), appidentity.RefreshInput{RefreshToken: token})
}

func (h *harness) live(t *testing.T) []identity.RefreshToken {
	t.Helper()
	return h.store.LiveTokens(h.clock.Now())
}

func TestRefresh_RotatesWithinTheFamily(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, st := h.session(t)
	h.clock.Advance(14 * time.Minute)

	res, err := h.refresh(login.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}

	old, next := h.storedToken(t, login.RefreshToken), h.storedToken(t, res.RefreshToken)
	if res.RefreshToken == login.RefreshToken || next.Family != old.Family {
		t.Fatal("refresh did not rotate within the family")
	}
	if !old.UsedAt.Equal(h.clock.Now()) || old.ReplacedBy != next.ID {
		t.Errorf("old token = %+v; want used now and replaced by the new one", old)
	}
	if next.Tenant != st.ID || res.Session.Tenant != st || res.Session.Role != shared.RoleStaff {
		t.Errorf("refresh moved the session to %+v as %q", res.Session.Tenant, res.Session.Role)
	}
	if live := h.live(t); len(live) != 1 || live[0].ID != next.ID {
		t.Errorf("live tokens = %+v, want only the new one", live)
	}
	if got := h.tx.Scopes(); got[len(got)-1] != (appshared.Scope{UserID: res.Session.User.ID}) {
		t.Errorf("the rotation ran in scope %+v, want the user's alone", got[len(got)-1])
	}
}

func TestRefresh_FollowsTheProtocolOrder(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, _ := h.session(t)
	before := len(h.store.Calls())

	if _, err := h.refresh(login.RefreshToken); err != nil {
		t.Fatal(err)
	}

	// ADR 0013: find, lock the family, read again, then change only with
	// conditional updates, inserting the successor before pointing at it.
	got := h.store.Calls()[before:]
	want := []string{
		"RefreshTokenByHash", "LockFamily", "RefreshTokenByID", "UserByID",
		"MarkRefreshTokenUsed", "InsertRefreshToken", "SetSuccessor",
	}
	if !slices.Equal(got, want) {
		t.Errorf("calls = %v\nwant    %v", got, want)
	}
}

func TestRefresh_ReadsTheRoleAgain(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, st := h.session(t)
	user := login.Session.User.ID

	h.members.Add(user, appidentity.Membership{Tenant: st, Role: shared.RoleViewer})
	demoted, err := h.refresh(login.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if demoted.Session.Role != shared.RoleViewer || h.lastClaims(t).Principal.Role() != shared.RoleViewer {
		t.Errorf("after a role change the session says %q", demoted.Session.Role)
	}

	// A removed member's next refresh has no store (ADR 0010).
	h.members.Remove(user, st.ID)
	removed, err := h.refresh(demoted.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Session.HasTenant() || h.lastClaims(t).Principal.HasTenant() || !h.storedToken(t, removed.RefreshToken).Tenant.IsZero() {
		t.Errorf("a removed member kept store %+v", removed.Session.Tenant)
	}
}

// TestRefresh_RetryWindow walks the edges of ADR 0013 through the use case.
func TestRefresh_RetryWindow(t *testing.T) {
	t.Parallel()

	t.Run("a retry inside the window rotates again and leaves one live token", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		login, _ := h.session(t)
		lost, err := h.refresh(login.RefreshToken) // the answer never reached the client
		if err != nil {
			t.Fatal(err)
		}
		h.clock.Advance(90 * time.Second)

		retried, err := h.refresh(login.RefreshToken)
		if err != nil {
			t.Fatalf("retry: %v", err)
		}
		if lostTok := h.storedToken(t, lost.RefreshToken); lostTok.SupersededAt.IsZero() {
			t.Error("the lost successor was not superseded")
		}
		retriedTok := h.storedToken(t, retried.RefreshToken)
		if live := h.live(t); len(live) != 1 || live[0].ID != retriedTok.ID {
			t.Errorf("live tokens = %d, want only the retry's", len(live))
		}
		old := h.storedToken(t, login.RefreshToken)
		if old.ReplacedBy != retriedTok.ID || !old.UsedAt.Equal(start) {
			t.Errorf("presented token = %+v; want first use kept and pointing at the retry's token", old)
		}
	})

	t.Run("repeated retries never extend the window", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		login, _ := h.session(t)
		for range 3 {
			if _, err := h.refresh(login.RefreshToken); err != nil {
				t.Fatalf("retry at %v: %v", h.clock.Now().Sub(start), err)
			}
			h.clock.Advance(40 * time.Second)
		}
		// 120 s after the first use: still inside.
		if _, err := h.refresh(login.RefreshToken); err != nil {
			t.Fatalf("retry at exactly 2 minutes: %v", err)
		}
		h.clock.Advance(time.Second)
		if _, err := h.refresh(login.RefreshToken); !errors.Is(err, shared.ErrUnauthenticated) {
			t.Errorf("retry after 2 minutes: %v, want ErrUnauthenticated", err)
		}
		if live := h.live(t); len(live) != 0 {
			t.Errorf("%d live tokens after reuse, want none", len(live))
		}
	})

	t.Run("after 2 minutes presenting it again is reuse and revokes the family", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		login, _ := h.session(t)
		next, _ := h.refresh(login.RefreshToken)
		h.clock.Advance(2*time.Minute + time.Second)

		_, err := h.refresh(login.RefreshToken)

		assertRevoked(t, h, err, login, next)
	})

	t.Run("once the successor was used, presenting it again is reuse", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		login, _ := h.session(t)
		next, _ := h.refresh(login.RefreshToken)
		h.clock.Advance(10 * time.Second)
		third, err := h.refresh(next.RefreshToken)
		if err != nil {
			t.Fatal(err)
		}
		h.clock.Advance(10 * time.Second)

		_, err = h.refresh(login.RefreshToken)

		assertRevoked(t, h, err, login, next, third)
	})

	t.Run("the successor a retry superseded is reuse, and kills the retry's token too", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		login, _ := h.session(t)
		lost, _ := h.refresh(login.RefreshToken)
		retried, err := h.refresh(login.RefreshToken)
		if err != nil {
			t.Fatal(err)
		}

		_, err = h.refresh(lost.RefreshToken)

		assertRevoked(t, h, err, login, lost, retried)
	})
}

// assertRevoked checks a reuse: 401, the family revoked, nothing live, and
// the access tokens of every session given denylisted for their lifetime.
func assertRevoked(t *testing.T, h *harness, err error, sessions ...appidentity.AuthResult) {
	t.Helper()
	if !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
	fam, _ := h.store.Family(h.storedToken(t, sessions[0].RefreshToken).Family)
	if !fam.Revoked() {
		t.Error("the family is still live")
	}
	if live := h.live(t); len(live) != 0 {
		t.Errorf("%d live tokens after reuse, want none", len(live))
	}
	denied := h.deny.JTIs()
	for _, s := range sessions {
		if jti := h.storedToken(t, s.RefreshToken).AccessJTI; !slices.Contains(denied, jti) {
			t.Errorf("access token %v not denylisted", jti)
		}
	}
	last := h.deny.Denied()[len(h.deny.Denied())-1]
	if want := h.clock.Now().Add(identity.DenylistWindow); !last.Until.Equal(want) {
		t.Errorf("denied until %v, want %v", last.Until, want)
	}
}

func TestRefresh_TwoPresentationsAtOnceGiveOneRotationAndOneRetry(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, _ := h.session(t)
	var other appidentity.AuthResult
	// The other presentation's rotation commits between this call's first
	// read and its conditional update; the update changes nothing and the
	// call reads again (ADR 0013).
	h.store.OnCall("MarkRefreshTokenUsed", func() {
		h.store.OnCall("MarkRefreshTokenUsed", nil)
		var err error
		if other, err = h.refresh(login.RefreshToken); err != nil {
			t.Errorf("the other presentation: %v", err)
		}
	})

	res, err := h.refresh(login.RefreshToken)

	if err != nil {
		t.Fatalf("the second presentation: %v, want a retry", err)
	}
	if h.storedToken(t, other.RefreshToken).SupersededAt.IsZero() {
		t.Error("the other presentation's token was not superseded by the retry")
	}
	live := h.live(t)
	if len(live) != 1 || live[0].ID != h.storedToken(t, res.RefreshToken).ID {
		t.Errorf("%d live tokens, want exactly one: never two fresh successors", len(live))
	}
}

func TestRefresh_GivesUpAfterLosingTwice(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, _ := h.session(t)
	interfere := func(method string) {
		h.store.OnCall(method, func() {
			h.store.OnCall(method, nil)
			if _, err := h.refresh(login.RefreshToken); err != nil {
				t.Errorf("interfering call: %v", err)
			}
		})
	}
	interfere("MarkRefreshTokenUsed")
	interfere("SupersedeRefreshToken")

	_, err := h.refresh(login.RefreshToken)

	if !errors.Is(err, shared.ErrConflict) || errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("err = %v, want a retryable conflict, never a 401", err)
	}
}

func TestRefresh_RefusesWithoutRevoking(t *testing.T) {
	t.Parallel()

	cases := map[string]func(t *testing.T, h *harness, token string) string{
		"malformed": func(*testing.T, *harness, string) string { return "not-a-token" },
		"unknown":   func(*testing.T, *harness, string) string { return shared.Secret{42}.String() },
		"expired": func(_ *testing.T, h *harness, token string) string {
			h.clock.Advance(identity.RefreshTokenLifetime)
			return token
		},
	}
	for name, present := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			login, _ := h.session(t)
			_, err := h.refresh(present(t, h, login.RefreshToken))
			if !errors.Is(err, shared.ErrUnauthenticated) {
				t.Fatalf("err = %v, want ErrUnauthenticated", err)
			}
			fam, _ := h.store.Family(h.storedToken(t, login.RefreshToken).Family)
			if fam.Revoked() || len(h.deny.Denied()) != 0 {
				t.Error("a refused token revoked its family")
			}
		})
	}
}

func TestRefresh_RevokedFamily(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, _ := h.session(t)
	if err := h.svc.Logout(context.Background(), appidentity.LogoutInput{RefreshToken: login.RefreshToken}); err != nil {
		t.Fatal(err)
	}

	if _, err := h.refresh(login.RefreshToken); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("err = %v, want ErrUnauthenticated", err)
	}
}

// TestRefresh_FailuresAreNeverUnauthenticated keeps Android signed in during
// an outage: only a refused token is a 401 (ADR 0012, ADR 0013).
func TestRefresh_FailuresAreNeverUnauthenticated(t *testing.T) {
	t.Parallel()

	for name, breakIt := range map[string]func(h *harness){
		"first read":   func(h *harness) { h.store.Fail("RefreshTokenByHash", errBoom) },
		"family lock":  func(h *harness) { h.store.Fail("LockFamily", errBoom) },
		"second read":  func(h *harness) { h.store.Fail("RefreshTokenByID", errBoom) },
		"memberships":  func(h *harness) { h.members.Fail(errBoom) },
		"user":         func(h *harness) { h.store.Fail("UserByID", errBoom) },
		"mark used":    func(h *harness) { h.store.Fail("MarkRefreshTokenUsed", errBoom) },
		"insert":       func(h *harness) { h.store.Fail("InsertRefreshToken", errBoom) },
		"successor":    func(h *harness) { h.store.Fail("SetSuccessor", errBoom) },
		"secrets":      func(h *harness) { h.secrets.Fail(errBoom) },
		"token issuer": func(h *harness) { h.tokens.Fail(errBoom) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			login, _ := h.session(t)
			breakIt(h)
			_, err := h.refresh(login.RefreshToken)
			if !errors.Is(err, errBoom) || errors.Is(err, shared.ErrUnauthenticated) {
				t.Errorf("err = %v, want the failure, never a 401", err)
			}
		})
	}

	t.Run("revoking on reuse", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		login, _ := h.session(t)
		_, _ = h.refresh(login.RefreshToken) // the first use; the reuse below is what fails
		h.clock.Advance(time.Hour)
		h.store.Fail("RevokeFamily", errBoom)
		_, err := h.refresh(login.RefreshToken)
		if !errors.Is(err, errBoom) || errors.Is(err, shared.ErrUnauthenticated) {
			t.Errorf("err = %v, want the failure, never a 401", err)
		}
	})
}

// ---- SwitchTenant ----

func TestSwitchTenant_MovesTheSessionToAnotherStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, _ := h.session(t)
	other := h.store1("beta")
	h.members.Add(login.Session.User.ID, appidentity.Membership{Tenant: other, Role: shared.RoleOwner})

	res, err := h.svc.SwitchTenant(context.Background(), appidentity.SwitchTenantInput{RefreshToken: login.RefreshToken, TenantID: other.ID})
	if err != nil {
		t.Fatal(err)
	}

	if res.Session.Tenant != other || res.Session.Role != shared.RoleOwner || h.lastClaims(t).Principal.TenantID() != other.ID {
		t.Errorf("switched to %+v as %q", res.Session.Tenant, res.Session.Role)
	}
	next := h.storedToken(t, res.RefreshToken)
	if next.Tenant != other.ID || h.storedToken(t, login.RefreshToken).ReplacedBy != next.ID {
		t.Error("switch did not rotate the token to the new store")
	}
}

func TestSwitchTenant_ChecksBeforeRotating(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, _ := h.session(t)

	_, notMember := h.svc.SwitchTenant(context.Background(), appidentity.SwitchTenantInput{
		RefreshToken: login.RefreshToken, TenantID: shared.TenantID(h.ids.NewID()),
	})
	_, noStore := h.svc.SwitchTenant(context.Background(), appidentity.SwitchTenantInput{RefreshToken: login.RefreshToken})

	if !errors.Is(notMember, shared.ErrForbidden) {
		t.Errorf("not a member: %v, want ErrForbidden", notMember)
	}
	if got := fieldsOf(t, noStore); !slices.Equal(got, []string{"tenantId"}) {
		t.Errorf("no store: fields %v", got)
	}
	// A 400 or 403 leaves the token as it was (ADR 0013).
	if tok := h.storedToken(t, login.RefreshToken); !tok.UsedAt.IsZero() {
		t.Error("a refused switch used the refresh token")
	}
	if _, err := h.refresh(login.RefreshToken); err != nil {
		t.Errorf("the token no longer works after a refused switch: %v", err)
	}
}

func TestSwitchTenant_ReuseStillRevokes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, st := h.session(t)
	next, _ := h.refresh(login.RefreshToken)
	h.clock.Advance(3 * time.Minute) // out of the window, within the access tokens' lifetime

	_, err := h.svc.SwitchTenant(context.Background(), appidentity.SwitchTenantInput{RefreshToken: login.RefreshToken, TenantID: st.ID})

	assertRevoked(t, h, err, login, next)
}

// ---- Logout ----

func TestLogout_RevokesTheFamilyAndDeniesTheAccessTokens(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, _ := h.session(t)
	next, _ := h.refresh(login.RefreshToken)
	presented := shared.NewV7(1, [10]byte{0xaa})

	err := h.svc.Logout(context.Background(), appidentity.LogoutInput{RefreshToken: next.RefreshToken, AccessJTI: presented})
	if err != nil {
		t.Fatal(err)
	}

	if len(h.live(t)) != 0 {
		t.Error("tokens still live after logout")
	}
	denied := h.deny.JTIs()
	for _, jti := range []shared.ID{presented, h.storedToken(t, login.RefreshToken).AccessJTI, h.storedToken(t, next.RefreshToken).AccessJTI} {
		if !slices.Contains(denied, jti) {
			t.Errorf("jti %v not denied", jti)
		}
	}
}

func TestLogout_AlwaysSucceedsForBadTokens(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	presented := shared.NewV7(1, [10]byte{0xbb})

	for _, token := range []string{"", "garbage", shared.Secret{7}.String()} {
		if err := h.svc.Logout(context.Background(), appidentity.LogoutInput{RefreshToken: token, AccessJTI: presented}); err != nil {
			t.Errorf("Logout(%q) = %v, want nil", token, err)
		}
	}
	if got := h.deny.JTIs(); len(got) != 3 || got[0] != presented {
		t.Errorf("denied %v, want the presented access token each time", got)
	}
	if err := h.svc.Logout(context.Background(), appidentity.LogoutInput{}); err != nil || len(h.deny.Denied()) != 3 {
		t.Errorf("empty logout: %v, %d deny calls; want nil and nothing denied", err, len(h.deny.Denied()))
	}
}

func TestLogout_ReportsStoreFailures(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, _ := h.session(t)
	h.store.Fail("RevokeFamily", errBoom)

	if err := h.svc.Logout(context.Background(), appidentity.LogoutInput{RefreshToken: login.RefreshToken}); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the failure", err)
	}
}
