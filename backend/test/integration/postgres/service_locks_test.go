//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	"github.com/bergsantana/encomendo/backend/internal/adapter/inproc"
	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	"github.com/bergsantana/encomendo/backend/internal/adapter/system"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/app/identity/fake"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The lock protocol of ADR 0013 end to end: the real identity service on
// the real repositories, with requests that overlap. The fakes' locks are
// one mutex; only Postgres shows that rotations, retries, logouts and
// resets of one login cannot leave a live token behind.

type idStack struct {
	svc    *appidentity.Service
	tokens *auth.AccessTokens
	deny   *fake.Denylist
	mailer *fake.Mailer
}

func newIdentityService(t *testing.T) idStack {
	t.Helper()
	db := openApp(t)
	clock := sharedfake.NewClock(time.Now().UTC().Truncate(time.Microsecond))
	ids := system.NewIDs()
	tenancyDeps := apptenancy.Deps{
		Tenants: postgres.NewTenants(), Members: postgres.NewMembers(), Invitations: postgres.NewInvitations(),
		Events: postgres.NewOutbox(),
	}
	lookup := apptenancy.NewLookup(tenancyDeps)
	hasher, err := auth.NewHasher(auth.Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1}, 4, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	key, err := auth.ParseKey(base64.StdEncoding.EncodeToString(seed))
	if err != nil {
		t.Fatal(err)
	}
	keys, err := auth.NewKeySet(key)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewAccessTokens(keys, "https://api.test", clock)
	if err != nil {
		t.Fatal(err)
	}
	deny, mailer := fake.NewDenylist(), fake.NewMailer()
	users := postgres.NewUsers()
	svc, err := appidentity.NewService(appidentity.Deps{
		Users: users, Refresh: postgres.NewRefreshTokens(), ResetCodes: postgres.NewResetCodes(),
		Memberships: inproc.NewIdentityMemberships(lookup), Invitations: inproc.NewIdentityInvitations(lookup),
		Hasher: hasher, Tokens: tokens, Denylist: deny, Mailer: mailer,
		Tx: postgres.NewTxManager(db), Clock: clock, IDs: ids, Secrets: auth.NewSecrets(),
		ResetCodeKey: bytes.Repeat([]byte("k"), appidentity.MinResetCodeKeyLength),
	})
	if err != nil {
		t.Fatal(err)
	}
	return idStack{svc: svc, tokens: tokens, deny: deny, mailer: mailer}
}

// liveTokens counts the tokens of the user's families that can still be
// presented: unused, unsuperseded, unexpired, in a family not revoked.
func liveTokens(t *testing.T, user shared.UserID) int {
	t.Helper()
	var n int
	err := env.Seeder(t).DB().QueryRowContext(t.Context(), `
		SELECT count(*) FROM refresh_tokens t JOIN refresh_families f ON f.id = t.family_id
		WHERE f.user_id = $1 AND f.revoked_at IS NULL AND t.used_at IS NULL AND t.superseded_at IS NULL AND t.expires_at > now()`,
		user.String()).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func liveFamilies(t *testing.T, user shared.UserID) int {
	t.Helper()
	var n int
	if err := env.Seeder(t).DB().QueryRowContext(t.Context(),
		`SELECT count(*) FROM refresh_families WHERE user_id = $1 AND revoked_at IS NULL`, user.String()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func signUp(t *testing.T, svc *appidentity.Service) appidentity.AuthResult {
	t.Helper()
	res, err := svc.SignUp(context.Background(), appidentity.SignUpInput{
		Name: "Ana Souza", Email: "ana-" + suffix() + "@teste.encomendo.com.br", Password: "segredo-forte",
	})
	if err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	return res
}

type outcome struct {
	res appidentity.AuthResult
	err error
}

// race runs every call at the same moment and returns what each did.
func race(calls ...func() outcome) []outcome {
	out := make([]outcome, len(calls))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			out[i] = call()
		}()
	}
	close(start)
	wg.Wait()
	return out
}

// TestRefresh_TwoPresentationsOfOneTokenGiveOneRotationAndOneRetry: the
// second, arriving inside the retry window while the first successor is
// unused, supersedes it. Never two fresh successors (ADR 0013).
func TestRefresh_TwoPresentationsOfOneTokenGiveOneRotationAndOneRetry(t *testing.T) {
	t.Parallel()
	st := newIdentityService(t)
	svc, deny := st.svc, st.deny

	for range 8 {
		first := signUp(t, svc)
		user := first.Session.User.ID
		refresh := func() outcome {
			res, err := svc.Refresh(context.Background(), appidentity.RefreshInput{RefreshToken: first.RefreshToken})
			return outcome{res, err}
		}

		got := race(refresh, refresh)

		for i, o := range got {
			if o.err != nil {
				t.Fatalf("presentation %d: %v; want a rotation or a retry, never a refusal", i, o.err)
			}
		}
		if n := liveTokens(t, user); n != 1 {
			t.Fatalf("%d live tokens after two presentations; want exactly one", n)
		}
		if got[0].res.RefreshToken == got[1].res.RefreshToken {
			t.Error("both presentations got the same refresh token")
		}
		if n := len(deny.JTIs()); n != 0 {
			t.Errorf("a retry denylisted %d access tokens; want none", n)
		}
	}
}

// TestRefresh_APresentedSupersededSuccessorRevokesTheFamily: the token a
// retry replaced is presented later, which is reuse: the retry's own token
// dies with the family (ADR 0013).
func TestRefresh_APresentedSupersededSuccessorRevokesTheFamily(t *testing.T) {
	t.Parallel()
	svc := newIdentityService(t).svc
	first := signUp(t, svc)
	user := first.Session.User.ID
	lost, err := svc.Refresh(context.Background(), appidentity.RefreshInput{RefreshToken: first.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}
	retried, err := svc.Refresh(context.Background(), appidentity.RefreshInput{RefreshToken: first.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Refresh(context.Background(), appidentity.RefreshInput{RefreshToken: lost.RefreshToken}); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("presenting the superseded token: %v; want ErrUnauthenticated", err)
	}
	if _, err := svc.Refresh(context.Background(), appidentity.RefreshInput{RefreshToken: retried.RefreshToken}); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("the retry's token after the reuse: %v; want it dead with its family", err)
	}
	if n := liveFamilies(t, user); n != 0 {
		t.Errorf("%d live families after reuse; want none", n)
	}
}

// TestLogout_DuringARotationLeavesNoLiveToken: whichever of the two gets the
// family lock first, the family ends revoked, and a rotation that won has
// its access token denylisted too.
func TestLogout_DuringARotationLeavesNoLiveToken(t *testing.T) {
	t.Parallel()
	st := newIdentityService(t)
	svc, deny := st.svc, st.deny

	for range 8 {
		first := signUp(t, svc)
		user := first.Session.User.ID
		refresh := func() outcome {
			res, err := svc.Refresh(context.Background(), appidentity.RefreshInput{RefreshToken: first.RefreshToken})
			return outcome{res, err}
		}
		logout := func() outcome {
			return outcome{err: svc.Logout(context.Background(), appidentity.LogoutInput{RefreshToken: first.RefreshToken})}
		}

		got := race(refresh, logout)

		if got[1].err != nil {
			t.Fatalf("Logout: %v", got[1].err)
		}
		if n := liveFamilies(t, user); n != 0 {
			t.Fatalf("%d live families after a logout racing a refresh; want none", n)
		}
		if got[0].err != nil && !errors.Is(got[0].err, shared.ErrUnauthenticated) {
			t.Fatalf("the refresh that lost to the logout: %v; want ErrUnauthenticated, not a failure", got[0].err)
		}
		if got[0].err == nil {
			claims, err := st.tokens.Verify(context.Background(), got[0].res.AccessToken, shared.AudienceBackoffice)
			if err != nil {
				t.Fatalf("the refresh's access token: %v", err)
			}
			if !slices.Contains(deny.JTIs(), claims.JTI) {
				t.Errorf("the refresh that won returned an access token (jti %v) the logout did not denylist", claims.JTI)
			}
		}
	}
}

// TestPasswordReset_DuringALoginLeavesNoLiveFamily: a login whose lock comes
// first is revoked by the reset; one that comes after sees the new hash and
// fails (ADR 0013).
func TestPasswordReset_DuringALoginLeavesNoLiveFamily(t *testing.T) {
	t.Parallel()
	st := newIdentityService(t)
	svc, mailer := st.svc, st.mailer

	for range 6 {
		first := signUp(t, svc)
		email := first.Session.User.Email
		user := first.Session.User.ID
		if err := svc.RequestPasswordReset(context.Background(), appidentity.PasswordResetInput{Email: email}); err != nil {
			t.Fatal(err)
		}
		sent := mailer.Sent()
		code := sent[len(sent)-1].Code.Reveal()
		login := func() outcome {
			res, err := svc.Login(context.Background(), appidentity.LoginInput{Email: email, Password: "segredo-forte"})
			return outcome{res, err}
		}
		reset := func() outcome {
			return outcome{err: svc.ConfirmPasswordReset(context.Background(),
				appidentity.ConfirmPasswordResetInput{Email: email, Code: code, Password: "outra-senha-forte"})}
		}

		got := race(login, reset)

		if got[1].err != nil {
			t.Fatalf("ConfirmPasswordReset: %v", got[1].err)
		}
		if got[0].err != nil && !errors.Is(got[0].err, identity.ErrInvalidCredentials) {
			t.Fatalf("Login: %v; want success or ErrInvalidCredentials", got[0].err)
		}
		if n := liveFamilies(t, user); n != 0 {
			t.Fatalf("%d live families after a reset racing a login (login err: %v); want none", n, got[0].err)
		}
	}
}
