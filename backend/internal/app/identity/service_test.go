package identity_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/app/identity/fake"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	start    = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	errBoom  = errors.New("postgres is down")
	resetKey = []byte("the reset code key, 32 bytes or more")
)

type harness struct {
	svc     *appidentity.Service
	store   *fake.Store
	hasher  *fake.Hasher
	tokens  *fake.Tokens
	deny    *fake.Denylist
	mailer  *fake.Mailer
	members *fake.Memberships
	invites *fake.Invitations
	tx      *sharedfake.TxManager
	clock   *sharedfake.Clock
	ids     *sharedfake.IDs
	secrets *sharedfake.Secrets
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	clock := sharedfake.NewClock(start)
	members := fake.NewMemberships()
	h := &harness{
		store: fake.NewStore(), hasher: fake.NewHasher(), tokens: fake.NewTokens(), deny: fake.NewDenylist(),
		mailer: fake.NewMailer(), members: members, invites: fake.NewInvitations(members),
		tx: sharedfake.NewTxManager(), clock: clock, ids: sharedfake.NewIDs(clock), secrets: sharedfake.NewSecrets(),
	}
	svc, err := appidentity.NewService(h.deps(resetKey))
	if err != nil {
		t.Fatal(err)
	}
	h.svc = svc
	return h
}

func (h *harness) deps(key []byte) appidentity.Deps {
	return appidentity.Deps{
		Users: h.store, Refresh: h.store, ResetCodes: h.store,
		Memberships: h.members, Invitations: h.invites,
		Hasher: h.hasher, Tokens: h.tokens, Denylist: h.deny, Mailer: h.mailer,
		Tx: h.tx, Clock: h.clock, IDs: h.ids, Secrets: h.secrets,
		ResetCodeKey: key,
	}
}

var _ appidentity.UseCases = (*appidentity.Service)(nil)

// TestNewService_RefusesAWeakResetKey: with a short or missing key the HMAC
// is a public function, and a leaked table of six-digit codes falls to a
// million guesses (ADR 0011).
func TestNewService_RefusesAWeakResetKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	for _, key := range [][]byte{nil, {}, make([]byte, appidentity.MinResetCodeKeyLength-1)} {
		if svc, err := appidentity.NewService(h.deps(key)); err == nil || svc != nil {
			t.Errorf("a %d-byte key: %v, %v; want an error", len(key), svc, err)
		}
	}
}

func TestAuthResult_NeverPrintsItsTokens(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	res := h.signUp(t, "ana@ex.com", "segredo-forte")

	for _, out := range []string{fmt.Sprint(res), fmt.Sprintf("%v %+v %#v %s", res, res, res, res)} {
		if strings.Contains(out, res.RefreshToken) || strings.Contains(out, res.AccessToken) {
			t.Errorf("printed a token: %s", out)
		}
	}
}

// signUp creates an account through the use case and returns its session.
func (h *harness) signUp(t *testing.T, email, password string) appidentity.AuthResult {
	t.Helper()
	res, err := h.svc.SignUp(context.Background(), appidentity.SignUpInput{Name: "Ana Souza", Email: email, Password: password})
	if err != nil {
		t.Fatalf("SignUp(%s): %v", email, err)
	}
	return res
}

func (h *harness) store1(name string) appidentity.TenantSummary {
	return appidentity.TenantSummary{ID: shared.TenantID(h.ids.NewID()), Name: name, Slug: "loja-" + name}
}

func hashOf(t *testing.T, token string) shared.SecretHash {
	t.Helper()
	s, err := shared.ParseSecret(token)
	if err != nil {
		t.Fatalf("refresh token %q is not a secret: %v", token, err)
	}
	return s.Hash()
}

func (h *harness) storedToken(t *testing.T, token string) identity.RefreshToken {
	t.Helper()
	tok, ok := h.store.RefreshTokenWithHash(hashOf(t, token))
	if !ok {
		t.Fatalf("no stored refresh token for %q", token)
	}
	return tok
}

func (h *harness) lastClaims(t *testing.T) appidentity.AccessClaims {
	t.Helper()
	issued := h.tokens.Issued()
	if len(issued) == 0 {
		t.Fatal("no access token issued")
	}
	return issued[len(issued)-1]
}

func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	var ve *shared.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a ValidationError", err)
	}
	var names []string
	for _, f := range ve.Fields {
		names = append(names, f.Field)
	}
	return names
}

// ---- SignUp ----

func TestSignUp_CreatesAnAccountAndAStorelessSession(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	res := h.signUp(t, "  Ana@AltaRPM.com.br ", "segredo-forte")

	s := res.Session
	if s.HasTenant() || s.Role != "" || s.Permissions == nil || len(s.Permissions) != 0 || s.Memberships == nil || len(s.Memberships) != 0 {
		t.Errorf("session = %+v; want no store, and empty, non-nil permissions and memberships", s)
	}
	if s.User.Name != "Ana Souza" || s.User.Email != "Ana@AltaRPM.com.br" || s.User.ID.IsZero() {
		t.Errorf("user = %+v", s.User)
	}
	stored, ok := h.store.User(s.User.ID)
	if !ok || stored.PasswordHash == "segredo-forte" || stored.PasswordHash == "" {
		t.Fatalf("stored user %+v: want only a hash of the password", stored)
	}
	if !res.AccessExpiresAt.Equal(start.Add(15*time.Minute)) || !res.RefreshExpiresAt.Equal(start.Add(30*24*time.Hour)) {
		t.Errorf("expiries: access %v, refresh %v", res.AccessExpiresAt, res.RefreshExpiresAt)
	}
	tok := h.storedToken(t, res.RefreshToken)
	if !tok.Tenant.IsZero() || !tok.UsedAt.IsZero() {
		t.Errorf("stored refresh token = %+v; want unused and storeless", tok)
	}
	fam, ok := h.store.Family(tok.Family)
	if !ok || fam.User != s.User.ID || fam.Revoked() {
		t.Errorf("family = %+v", fam)
	}
	c := h.lastClaims(t)
	if c.Principal.UserID() != s.User.ID || c.Principal.HasTenant() || c.Principal.Audience() != shared.AudienceBackoffice {
		t.Errorf("access token principal = %+v", c.Principal)
	}
	if c.JTI != tok.AccessJTI || res.AccessToken != "access."+c.JTI.String() {
		t.Error("the refresh token does not record the jti of the access token issued with it")
	}
	if !c.IssuedAt.Equal(start) || !c.ExpiresAt.Equal(start.Add(15*time.Minute)) {
		t.Errorf("claims times %v → %v", c.IssuedAt, c.ExpiresAt)
	}
	if got := h.tx.Scopes(); len(got) != 1 || got[0] != (appshared.Scope{}) {
		t.Errorf("scopes = %+v; sign-up touches no tenant rows and needs none", got)
	}
}

func TestSignUp_ReportsEveryInvalidFieldBeforeHashing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	_, err := h.svc.SignUp(context.Background(), appidentity.SignUpInput{Name: " ", Email: "nope", Password: "curta"})

	if got := fieldsOf(t, err); !slices.Equal(got, []string{"name", "email", "password"}) {
		t.Errorf("fields = %v, want name, email, password", got)
	}
	if h.hasher.Hashes() != 0 || len(h.tx.Scopes()) != 0 {
		t.Error("invalid input still reached the hasher or the database")
	}
}

func TestSignUp_EmailTakenInAnyCase(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.signUp(t, "ana@altarpm.com.br", "segredo-forte")

	_, err := h.svc.SignUp(context.Background(), appidentity.SignUpInput{Name: "Outra", Email: "ANA@altarpm.com.br", Password: "outra-senha"})

	if !errors.Is(err, identity.ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
	if n := len(h.store.LiveTokens(start)); n != 1 {
		t.Errorf("%d live refresh tokens, want only the first account's", n)
	}
}

func TestSignUp_AFailedSessionLeavesNoAccount(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.store.Fail("InsertRefreshToken", errBoom)

	if _, err := h.svc.SignUp(context.Background(), appidentity.SignUpInput{Name: "Ana", Email: "ana@ex.com", Password: "segredo-forte"}); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}

	// The account and the session commit together or not at all: the
	// email is still free.
	h.store.Fail("InsertRefreshToken", nil)
	h.signUp(t, "ana@ex.com", "segredo-forte")
}

func TestSignUp_FailuresStayServerErrors(t *testing.T) {
	t.Parallel()

	for name, breakIt := range map[string]func(h *harness){
		"hasher":  func(h *harness) { h.hasher.Fail(errBoom) },
		"secrets": func(h *harness) { h.secrets.Fail(errBoom) },
		"insert":  func(h *harness) { h.store.Fail("InsertUser", errBoom) },
		"family":  func(h *harness) { h.store.Fail("CreateFamily", errBoom) },
		"issuer":  func(h *harness) { h.tokens.Fail(errBoom) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			breakIt(h)
			_, err := h.svc.SignUp(context.Background(), appidentity.SignUpInput{Name: "Ana", Email: "ana@ex.com", Password: "segredo-forte"})
			if !errors.Is(err, errBoom) || errors.Is(err, shared.ErrInvalid) || errors.Is(err, shared.ErrConflict) {
				t.Errorf("err = %v, want the failure itself", err)
			}
		})
	}
}

// ---- Login ----

func TestLogin_ChoosesTheStore(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		stores int
		hint   int // index into the stores, -1 for none, -2 for a store the user is not in
		want   int // index of the selected store, -1 for none
	}{
		"no stores":                     {0, -1, -1},
		"the only store":                {1, -1, 0},
		"several, no hint":              {3, -1, -1},
		"several, a hint":               {3, 2, 2},
		"a hint the user is not in":     {3, -2, -1},
		"one store, a hint for another": {1, -2, 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			user := h.signUp(t, "ana@ex.com", "segredo-forte").Session.User.ID
			var stores []appidentity.TenantSummary
			for i := range c.stores {
				st := h.store1(string(rune('a' + i)))
				stores = append(stores, st)
				h.members.Add(user, appidentity.Membership{Tenant: st, Role: shared.RoleAdmin})
			}
			in := appidentity.LoginInput{Email: "ana@ex.com", Password: "segredo-forte"}
			switch {
			case c.hint >= 0:
				in.TenantID = stores[c.hint].ID
			case c.hint == -2:
				in.TenantID = shared.TenantID(h.ids.NewID())
			}

			res, err := h.svc.Login(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}

			if len(res.Session.Memberships) != c.stores {
				t.Errorf("%d memberships, want %d", len(res.Session.Memberships), c.stores)
			}
			if c.want < 0 {
				if res.Session.HasTenant() || h.lastClaims(t).Principal.HasTenant() || len(res.Session.Permissions) != 0 {
					t.Errorf("session selects %+v, want none", res.Session.Tenant)
				}
				return
			}
			if res.Session.Tenant != stores[c.want] || res.Session.Role != shared.RoleAdmin {
				t.Errorf("session selects %+v as %q, want %+v as admin", res.Session.Tenant, res.Session.Role, stores[c.want])
			}
			if !slices.Equal(res.Session.Permissions, identity.PermissionsOf(shared.RoleAdmin)) {
				t.Errorf("permissions = %v", res.Session.Permissions)
			}
			p := h.lastClaims(t).Principal
			if p.TenantID() != stores[c.want].ID || p.Role() != shared.RoleAdmin {
				t.Errorf("access token for %v as %q", p.TenantID(), p.Role())
			}
			if tok := h.storedToken(t, res.RefreshToken); tok.Tenant != stores[c.want].ID {
				t.Errorf("refresh token keeps store %v, want %v", tok.Tenant, stores[c.want].ID)
			}
		})
	}
}

func TestLogin_ReadsStoresInAUserOnlyScope(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.signUp(t, "ana@ex.com", "segredo-forte").Session.User.ID

	if _, err := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "ana@ex.com", Password: "segredo-forte"}); err != nil {
		t.Fatal(err)
	}

	// The store picker policies apply only with no tenant set (ADR 0010).
	if got := h.tx.Scopes(); !slices.Contains(got, appshared.Scope{UserID: user}) {
		t.Errorf("scopes = %+v, want one scoped to the user alone", got)
	}
}

func TestLogin_WrongPasswordAndUnknownEmailLookTheSame(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.signUp(t, "ana@ex.com", "segredo-forte")
	before := h.hasher.Verifies()

	_, wrong := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "ana@ex.com", Password: "errada"})
	_, unknown := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "ninguem@ex.com", Password: "errada"})

	for name, err := range map[string]error{"wrong password": wrong, "unknown email": unknown} {
		if !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Errorf("%s: err = %v, want ErrInvalidCredentials", name, err)
		}
	}
	// The unknown email still runs one argon2id check, against no hash, so
	// its timing tells nothing (ADR 0010).
	if got := h.hasher.Verifies() - before; got != 2 {
		t.Errorf("%d password checks, want one per attempt", got)
	}
	if n := len(h.store.LiveTokens(start)); n != 1 {
		t.Errorf("%d live tokens, want only the sign-up's", n)
	}
}

func TestLogin_RefusesWhenThePasswordChangedMeanwhile(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.signUp(t, "ana@ex.com", "segredo-forte").Session.User.ID
	// A password reset commits between the unlocked argon2id check and the
	// session's transaction (ADR 0013).
	h.store.OnCall("PasswordHashForShare", func() { resetPasswordBehindTheScenes(t, h, user) })

	_, err := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "ana@ex.com", Password: "segredo-forte"})

	if !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if n := len(h.store.LiveTokens(start)); n != 1 {
		t.Errorf("%d live tokens; the stale login must not create a family", n)
	}
}

// resetPasswordBehindTheScenes changes the stored hash as a committed reset
// would, once.
func resetPasswordBehindTheScenes(t *testing.T, h *harness, user shared.UserID) {
	t.Helper()
	h.store.OnCall("PasswordHashForShare", nil)
	err := h.tx.WithinTx(context.Background(), func(ctx context.Context) error {
		u, err := h.store.UserByID(ctx, user)
		if err != nil {
			return err
		}
		if err := u.ChangePassword("fake-argon2id$99$nova-senha"); err != nil {
			return err
		}
		return h.store.UpdatePassword(ctx, u)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLogin_ChecksInputBounds(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	_, badEmail := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "nope", Password: "x"})
	_, longPassword := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "a@ex.com", Password: string(make([]byte, 129))})

	if got := fieldsOf(t, badEmail); !slices.Equal(got, []string{"email"}) {
		t.Errorf("bad email: fields %v", got)
	}
	if got := fieldsOf(t, longPassword); !slices.Equal(got, []string{"password"}) {
		t.Errorf("long password: fields %v", got)
	}
}

func TestLogin_FailuresAreNeverInvalidCredentials(t *testing.T) {
	t.Parallel()

	// An outage must not log anyone out or look like a wrong password
	// (ADR 0013).
	for name, breakIt := range map[string]func(h *harness){
		"user lookup": func(h *harness) { h.store.Fail("UserByEmail", errBoom) },
		"hasher":      func(h *harness) { h.hasher.Fail(errBoom) },
		"hash check":  func(h *harness) { h.store.Fail("PasswordHashForShare", errBoom) },
		"memberships": func(h *harness) { h.members.Fail(errBoom) },
		"family":      func(h *harness) { h.store.Fail("CreateFamily", errBoom) },
		"token":       func(h *harness) { h.store.Fail("InsertRefreshToken", errBoom) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.signUp(t, "ana@ex.com", "segredo-forte")
			breakIt(h)
			_, err := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "ana@ex.com", Password: "segredo-forte"})
			if !errors.Is(err, errBoom) || errors.Is(err, shared.ErrUnauthenticated) {
				t.Errorf("err = %v, want the failure, never a 401", err)
			}
		})
	}
}
