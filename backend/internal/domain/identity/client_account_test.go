package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var accountID = shared.ClientAccountID(shared.NewV7(1_700_000_000_000, [10]byte{8}))

func TestNewClientAccount(t *testing.T) {
	t.Parallel()

	// Client accounts are passwordless and may start without a name
	// (docs/architecture.md 5.3).
	a, err := NewClientAccount(accountID, "")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID() != accountID || a.Name() != "" || a.Version() != 1 {
		t.Errorf("account = %v %q v%d", a.ID(), a.Name(), a.Version())
	}
	named, err := NewClientAccount(accountID, "  Bruno  ")
	if err != nil || named.Name() != "Bruno" {
		t.Errorf("named account = %q, %v", named.Name(), err)
	}
	if _, err := NewClientAccount(accountID, strings.Repeat("b", 101)); !errors.Is(err, shared.ErrInvalid) {
		t.Errorf("101-character name: %v, want ErrInvalid", err)
	}
	if _, err := NewClientAccount(shared.ClientAccountID{}, ""); !errors.Is(err, ErrInvalidClientAccount) {
		t.Errorf("zero id: %v, want ErrInvalidClientAccount", err)
	}
}

func TestClientAccount_Rename(t *testing.T) {
	t.Parallel()

	a, _ := NewClientAccount(accountID, "Bruno")
	if err := a.Rename("Bruno Lima"); err != nil || a.Name() != "Bruno Lima" {
		t.Errorf("Rename = %q, %v", a.Name(), err)
	}
	if err := a.Rename("  "); err != nil || a.Name() != "" {
		t.Errorf("clearing the name = %q, %v", a.Name(), err)
	}
	if err := a.Rename("x\ny"); !errors.Is(err, shared.ErrInvalid) || a.Name() != "" {
		t.Errorf("bad name: %v, name now %q", err, a.Name())
	}
}

func TestClientIdentities(t *testing.T) {
	t.Parallel()

	// One way to log in: a verified email or a Google sub, never both
	// (0002_identity: num_nonnulls = 1).
	byEmail, err := NewEmailIdentity(accountID, mustEmail(t, "cliente@ex.com"))
	if err != nil {
		t.Fatal(err)
	}
	if byEmail.Kind() != ByEmail || byEmail.Email().String() != "cliente@ex.com" || byEmail.GoogleSub() != "" {
		t.Errorf("email identity = %+v", byEmail)
	}
	byGoogle, err := NewGoogleIdentity(accountID, "1098765432")
	if err != nil {
		t.Fatal(err)
	}
	if byGoogle.Kind() != ByGoogle || byGoogle.GoogleSub() != "1098765432" || !byGoogle.Email().IsZero() {
		t.Errorf("google identity = %+v", byGoogle)
	}
	if byGoogle.Account() != accountID {
		t.Error("identity lost its account")
	}

	if _, err := NewEmailIdentity(accountID, Email{}); !errors.Is(err, ErrInvalidClientAccount) {
		t.Errorf("zero email: %v", err)
	}
	for _, sub := range []string{"", strings.Repeat("1", 256), "12 34"} {
		if _, err := NewGoogleIdentity(accountID, sub); !errors.Is(err, ErrInvalidClientAccount) {
			t.Errorf("sub %q: %v", sub, err)
		}
	}
	if _, err := NewGoogleIdentity(shared.ClientAccountID{}, "1"); !errors.Is(err, ErrInvalidClientAccount) {
		t.Errorf("zero account: %v", err)
	}
}
