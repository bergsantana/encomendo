package auth_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/app/identity/fake"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The refresh-token security tests of docs/architecture.md section 9, run
// through app/identity on this package's real tokens, hasher and secrets,
// with fake stores. app/identity's own tests cover every rotation path on
// fakes; these check what only the real adapters can show: that the jti the
// service denies is the one inside the signed token, and that a retried
// rotation hands out tokens that verify (ADR 0013).

// sessions is app/identity on the real auth adapters.
type sessions struct {
	f    *fixture
	deny *fake.Denylist
	svc  *appidentity.Service
}

func newSessions(t *testing.T) *sessions {
	t.Helper()
	f := newFixture(t)
	members, store, deny := fake.NewMemberships(), fake.NewStore(), fake.NewDenylist()
	svc, err := appidentity.NewService(appidentity.Deps{
		Users: store, Refresh: store, ResetCodes: store,
		Memberships: members, Invitations: fake.NewInvitations(members),
		Hasher: newHasher(t, cheap()), Tokens: f.tokens, Denylist: deny, Mailer: fake.NewMailer(),
		Tx: sharedfake.NewTxManager(), Clock: f.clock, IDs: f.ids, Secrets: auth.NewSecrets(),
		ResetCodeKey: bytes.Repeat([]byte("k"), appidentity.MinResetCodeKeyLength),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &sessions{f: f, deny: deny, svc: svc}
}

func (s *sessions) signUp(t *testing.T) appidentity.AuthResult {
	t.Helper()
	res, err := s.svc.SignUp(context.Background(), appidentity.SignUpInput{
		Name: "Ana", Email: "ana@ex.com", Password: "segredo-forte",
	})
	if err != nil {
		t.Fatalf("SignUp: %v", err)
	}
	return res
}

func (s *sessions) refresh(t *testing.T, token string) (appidentity.AuthResult, error) {
	t.Helper()
	return s.svc.Refresh(context.Background(), appidentity.RefreshInput{RefreshToken: token})
}

// jti verifies res's access token and returns its jti, checking that the
// token speaks for res's user.
func (s *sessions) jti(t *testing.T, res appidentity.AuthResult) shared.ID {
	t.Helper()
	c, err := s.f.tokens.Verify(context.Background(), res.AccessToken, shared.AudienceBackoffice)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if c.Principal.UserID() != res.Session.User.ID || !c.ExpiresAt.Equal(res.AccessExpiresAt) {
		t.Errorf("the token says %+v; the session says user %v until %v", c, res.Session.User.ID, res.AccessExpiresAt)
	}
	return c.JTI
}

func (s *sessions) denied() []shared.ID {
	var out []shared.ID
	for _, d := range s.deny.Denied() {
		out = append(out, d.JTIs...)
	}
	return out
}

func TestRefresh_ReuseRevokesTheFamilyAndDeniesItsSignedTokens(t *testing.T) {
	t.Parallel()
	s := newSessions(t)
	first := s.signUp(t)
	second, err := s.refresh(t, first.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	s.f.clock.Advance(identity.RefreshRetryWindow + time.Second)

	if _, err := s.refresh(t, first.RefreshToken); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("reusing the spent token: %v; want ErrUnauthenticated", err)
	}
	if _, err := s.refresh(t, second.RefreshToken); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("the live token after the reuse: %v; want the family revoked", err)
	}
	denied := s.denied()
	for name, res := range map[string]appidentity.AuthResult{"first": first, "second": second} {
		if jti := s.jti(t, res); !slices.Contains(denied, jti) {
			t.Errorf("the %s access token (jti %v) is not denied; denied %v", name, jti, denied)
		}
	}
}

func TestRefresh_RetryInsideTheWindowHandsOutTokensThatWork(t *testing.T) {
	t.Parallel()
	s := newSessions(t)
	first := s.signUp(t)
	lost, err := s.refresh(t, first.RefreshToken) // the answer the client never got
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	s.f.clock.Advance(identity.RefreshRetryWindow - time.Second)

	retried, err := s.refresh(t, first.RefreshToken)
	if err != nil {
		t.Fatalf("the retry: %v; want a new pair", err)
	}
	if retried.RefreshToken == lost.RefreshToken || s.jti(t, retried) == s.jti(t, lost) {
		t.Error("the retry handed out the lost pair again")
	}
	if _, err := s.refresh(t, retried.RefreshToken); err != nil {
		t.Errorf("refreshing with the retried token: %v", err)
	}
	if denied := s.denied(); len(denied) != 0 {
		t.Errorf("a retry denied %v; want nothing denied", denied)
	}
}
