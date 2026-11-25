package identity

import (
	"errors"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var (
	t0      = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	family  = FamilyID(shared.NewV7(t0.UnixMilli(), [10]byte{2}))
	tokenA  = RefreshTokenID(shared.NewV7(t0.UnixMilli(), [10]byte{3}))
	tokenB  = RefreshTokenID(shared.NewV7(t0.UnixMilli(), [10]byte{4}))
	storeID = shared.TenantID(shared.NewV7(t0.UnixMilli(), [10]byte{5}))
)

func freshToken(t *testing.T) RefreshToken {
	t.Helper()
	tok, err := NewRefreshToken(RefreshTokenSpec{
		ID: tokenA, Family: family, Tenant: storeID,
		Hash: shared.Secret{1}.Hash(), AccessJTI: shared.NewV7(t0.UnixMilli(), [10]byte{6}),
	}, t0)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestNewRefreshToken_Lasts30Days(t *testing.T) {
	t.Parallel()

	tok := freshToken(t)
	if want := t0.Add(30 * 24 * time.Hour); !tok.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", tok.ExpiresAt, want)
	}
	if !tok.CreatedAt.Equal(t0) || !tok.UsedAt.IsZero() || !tok.ReplacedBy.IsZero() || !tok.SupersededAt.IsZero() {
		t.Errorf("a new token is not fresh: %+v", tok)
	}
}

func TestNewRefreshToken_RefusesMissingParts(t *testing.T) {
	t.Parallel()

	ok := RefreshTokenSpec{ID: tokenA, Family: family, Hash: shared.Secret{1}.Hash(), AccessJTI: shared.NewV7(1, [10]byte{1})}
	for name, spec := range map[string]RefreshTokenSpec{
		"no id":     {Family: ok.Family, Hash: ok.Hash, AccessJTI: ok.AccessJTI},
		"no family": {ID: ok.ID, Hash: ok.Hash, AccessJTI: ok.AccessJTI},
		"no hash":   {ID: ok.ID, Family: ok.Family, AccessJTI: ok.AccessJTI},
		"no jti":    {ID: ok.ID, Family: ok.Family, Hash: ok.Hash},
	} {
		if _, err := NewRefreshToken(spec, t0); !errors.Is(err, ErrInvalidRefreshToken) {
			t.Errorf("%s: err = %v, want ErrInvalidRefreshToken", name, err)
		}
	}
	if _, err := NewRefreshToken(ok, t0); err != nil {
		t.Errorf("a store-less token is valid: %v", err)
	}
}

// used returns tok first used at at, rotated to tokenB.
func used(tok RefreshToken, at time.Time) RefreshToken {
	tok.UsedAt = at
	tok.ReplacedBy = tokenB
	return tok
}

func successorOf(tok RefreshToken) *RefreshToken {
	s := RefreshToken{ID: tok.ReplacedBy, Family: tok.Family, CreatedAt: tok.UsedAt, ExpiresAt: tok.UsedAt.Add(RefreshTokenLifetime)}
	return &s
}

// TestRefreshToken_UseAt checks the window of ADR 0013: a token presented
// again is a retry only within 2 minutes of its first use and while its
// successor is neither used nor superseded; anything else that was used is
// reuse, and an expired token is refused without revoking anything.
func TestRefreshToken_UseAt(t *testing.T) {
	t.Parallel()

	fresh := freshToken(t)
	usedAt := t0.Add(time.Hour)
	once := used(fresh, usedAt)
	successorUsed := successorOf(once)
	successorUsed.UsedAt = usedAt.Add(30 * time.Second)
	successorSuperseded := successorOf(once)
	successorSuperseded.SupersededAt = usedAt.Add(30 * time.Second)
	superseded := fresh
	superseded.SupersededAt = t0.Add(time.Minute)
	strayed := successorOf(once)
	strayed.ID = tokenA // not the token it was rotated to

	cases := []struct {
		name      string
		tok       RefreshToken
		successor *RefreshToken
		now       time.Time
		want      Use
	}{
		{"unused rotates", fresh, nil, t0.Add(time.Minute), UseRotate},
		{"unused, a second before expiry", fresh, nil, fresh.ExpiresAt.Add(-time.Second), UseRotate},
		{"expired is refused", fresh, nil, fresh.ExpiresAt, UseRefused},
		{"used and expired is refused, not reuse", once, successorOf(once), fresh.ExpiresAt.Add(time.Hour), UseRefused},
		{"retry at once", once, successorOf(once), usedAt, UseRetry},
		{"retry inside the window", once, successorOf(once), usedAt.Add(90 * time.Second), UseRetry},
		{"retry at exactly 2 minutes", once, successorOf(once), usedAt.Add(RefreshRetryWindow), UseRetry},
		{"2 minutes and a nanosecond is reuse", once, successorOf(once), usedAt.Add(RefreshRetryWindow + time.Nanosecond), UseReuse},
		{"successor already used is reuse", once, successorUsed, usedAt.Add(time.Minute), UseReuse},
		{"successor superseded is reuse", once, successorSuperseded, usedAt.Add(time.Minute), UseReuse},
		{"successor missing is reuse", once, nil, usedAt.Add(time.Minute), UseReuse},
		{"successor of another token is reuse", once, strayed, usedAt.Add(time.Minute), UseReuse},
		{"superseded token is reuse", superseded, nil, t0.Add(2 * time.Minute), UseReuse},
		{"superseded, even inside the window, is reuse", superseded, nil, t0.Add(61 * time.Second), UseReuse},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.tok.UseAt(c.successor, c.now); got != c.want {
				t.Errorf("UseAt = %v, want %v", got, c.want)
			}
		})
	}
}

// TestRefreshToken_RepeatedRetriesNeverExtendTheWindow walks the lost
// answer case of ADR 0013: each retry supersedes the unused successor and
// points the token at a new one, but the window counts from the first use.
func TestRefreshToken_RepeatedRetriesNeverExtendTheWindow(t *testing.T) {
	t.Parallel()

	first := t0.Add(time.Hour)
	tok := used(freshToken(t), first)
	for _, after := range []time.Duration{30 * time.Second, 70 * time.Second, 110 * time.Second} {
		now := first.Add(after)
		if got := tok.UseAt(successorOf(tok), now); got != UseRetry {
			t.Fatalf("retry %v after first use = %v, want UseRetry", after, got)
		}
		tok = tok.RetriedTo(RefreshTokenID(shared.NewV7(now.UnixMilli(), [10]byte{9})))
		if !tok.UsedAt.Equal(first) {
			t.Fatalf("a retry moved UsedAt to %v; the window must count from the first use", tok.UsedAt)
		}
	}
	if got := tok.UseAt(successorOf(tok), first.Add(121*time.Second)); got != UseReuse {
		t.Errorf("after 2 minutes, retried token = %v, want UseReuse", got)
	}
}

func TestUse_String(t *testing.T) {
	t.Parallel()

	for use, want := range map[Use]string{UseRefused: "refused", UseRotate: "rotate", UseRetry: "retry", UseReuse: "reuse", Use(99): "Use(99)"} {
		if got := use.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(use), got, want)
		}
	}
}

func TestRefreshFamily(t *testing.T) {
	t.Parallel()

	f, err := NewRefreshFamily(family, userID)
	if err != nil {
		t.Fatal(err)
	}
	if f.Revoked() {
		t.Error("a new family is revoked")
	}
	f.RevokedAt = t0
	if !f.Revoked() {
		t.Error("a family with RevokedAt is live")
	}
	if _, err := NewRefreshFamily(FamilyID{}, userID); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("zero id: %v", err)
	}
	if _, err := NewRefreshFamily(family, shared.UserID{}); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Errorf("zero user: %v", err)
	}
}

func TestDenylistWindow_CoversLifetimeAndLeeway(t *testing.T) {
	t.Parallel()

	// Revoking a family denylists the access tokens issued with it in this
	// window, so none of them outlives the revoke (ADR 0013).
	if got, want := DenylistWindow, 15*time.Minute+30*time.Second; got != want {
		t.Errorf("DenylistWindow = %v, want %v", got, want)
	}
	if AccessTokenLifetime != 15*time.Minute || RefreshRetryWindow != 2*time.Minute {
		t.Errorf("lifetimes changed: access %v, retry window %v", AccessTokenLifetime, RefreshRetryWindow)
	}
}
