package identity_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func (h *harness) requestReset(t *testing.T, email string) identity.ResetCode {
	t.Helper()
	before := len(h.mailer.Sent())
	if err := h.svc.RequestPasswordReset(context.Background(), appidentity.PasswordResetInput{Email: email}); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	sent := h.mailer.Sent()
	if len(sent) != before+1 {
		t.Fatalf("%d emails sent, want one more", len(sent)-before)
	}
	return sent[len(sent)-1].Code
}

func (h *harness) confirm(email string, code identity.ResetCode, password string) error {
	return h.svc.ConfirmPasswordReset(context.Background(), appidentity.ConfirmPasswordResetInput{
		Email: email, Code: code.String(), Password: password,
	})
}

func wrongCode(c identity.ResetCode) identity.ResetCode {
	if c == "000000" {
		return "000001"
	}
	return "000000"
}

func TestRequestPasswordReset_StoresOnlyAnHMACAndSendsTheCode(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	user := h.signUp(t, "ana@ex.com", "segredo-forte").Session.User.ID

	code := h.requestReset(t, "ANA@ex.com")

	sent := h.mailer.Sent()[0]
	if sent.To.String() != "ana@ex.com" || sent.Name.String() != "Ana Souza" {
		t.Errorf("email to %q for %q", sent.To, sent.Name)
	}
	codes := h.store.ResetCodes()
	if len(codes) != 1 {
		t.Fatalf("%d codes stored, want 1", len(codes))
	}
	c := codes[0]
	if c.User != user || c.Hash != identity.HashResetCode(resetKey, user, code) {
		t.Error("the stored code is not the HMAC of the emailed one under the server key")
	}
	if !c.ExpiresAt.Equal(start.Add(10 * time.Minute)) {
		t.Errorf("code expires %v, want in 10 minutes", c.ExpiresAt)
	}
}

func TestRequestPasswordReset_UnknownEmailLooksTheSame(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	err := h.svc.RequestPasswordReset(context.Background(), appidentity.PasswordResetInput{Email: "ninguem@ex.com"})

	if err != nil {
		t.Fatalf("err = %v; the answer must never tell who has an account", err)
	}
	if len(h.mailer.Sent()) != 0 || len(h.store.ResetCodes()) != 0 {
		t.Error("an unknown email got a code")
	}
}

func TestRequestPasswordReset_InvalidEmailAndFailures(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	if got := fieldsOf(t, h.svc.RequestPasswordReset(context.Background(), appidentity.PasswordResetInput{Email: "nope"})); !slices.Equal(got, []string{"email"}) {
		t.Errorf("fields = %v", got)
	}
	h.store.Fail("UserByEmail", errBoom)
	if err := h.svc.RequestPasswordReset(context.Background(), appidentity.PasswordResetInput{Email: "ana@ex.com"}); !errors.Is(err, errBoom) {
		t.Errorf("store failure: %v", err)
	}
	h.store.Fail("UserByEmail", nil)
	h.secrets.Fail(errBoom)
	if err := h.svc.RequestPasswordReset(context.Background(), appidentity.PasswordResetInput{Email: "ana@ex.com"}); !errors.Is(err, errBoom) {
		t.Errorf("secrets failure: %v", err)
	}
}

func TestConfirmPasswordReset_SetsThePasswordAndEndsEverySession(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	login, _ := h.session(t)
	other, err := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "ana@ex.com", Password: "segredo-forte"})
	if err != nil {
		t.Fatal(err)
	}
	code := h.requestReset(t, "ana@ex.com")
	h.clock.Advance(3 * time.Minute)
	before := len(h.store.Calls())

	if err := h.confirm("ana@ex.com", code, "nova-senha-forte"); err != nil {
		t.Fatal(err)
	}

	if len(h.live(t)) != 0 {
		t.Error("sessions survived the reset")
	}
	for _, s := range []appidentity.AuthResult{login, other} {
		if jti := h.storedToken(t, s.RefreshToken).AccessJTI; !slices.Contains(h.deny.JTIs(), jti) {
			t.Errorf("access token %v not denylisted", jti)
		}
	}
	if _, err := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "ana@ex.com", Password: "nova-senha-forte"}); err != nil {
		t.Errorf("login with the new password: %v", err)
	}
	if _, err := h.svc.Login(context.Background(), appidentity.LoginInput{Email: "ana@ex.com", Password: "segredo-forte"}); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Errorf("login with the old password: %v", err)
	}
	// ADR 0013: lock the account first, update its row, then the families.
	calls := h.store.Calls()[before:]
	if calls[0] != "LockUserByEmail" ||
		slices.Index(calls, "UpdatePassword") > slices.Index(calls, "RevokeUserFamilies") ||
		slices.Index(calls, "UpdatePassword") < 0 {
		t.Errorf("calls = %v; want LockUserByEmail first and UpdatePassword before RevokeUserFamilies", calls)
	}
	if err := h.confirm("ana@ex.com", code, "terceira-senha"); !errors.Is(err, identity.ErrInvalidCode) {
		t.Errorf("the code worked twice: %v", err)
	}
}

func TestConfirmPasswordReset_WrongCodesCount(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.signUp(t, "ana@ex.com", "segredo-forte")
	code := h.requestReset(t, "ana@ex.com")

	for i := range identity.MaxResetAttempts {
		if err := h.confirm("ana@ex.com", wrongCode(code), "nova-senha-forte"); !errors.Is(err, identity.ErrInvalidCode) {
			t.Fatalf("wrong try %d: %v, want ErrInvalidCode", i+1, err)
		}
	}

	// The attempts committed although the call failed, and five used the
	// code up: even the right code is refused now (ADR 0011).
	if got := h.store.ResetCodes()[0].Attempts; got != identity.MaxResetAttempts {
		t.Errorf("attempts = %d, want %d", got, identity.MaxResetAttempts)
	}
	if err := h.confirm("ana@ex.com", code, "nova-senha-forte"); !errors.Is(err, identity.ErrInvalidCode) {
		t.Errorf("the right code after 5 wrong tries: %v, want ErrInvalidCode", err)
	}
}

func TestConfirmPasswordReset_NewCodesBuyNoMoreGuesses(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.signUp(t, "ana@ex.com", "segredo-forte")
	for range identity.ResetBudget / identity.MaxResetAttempts {
		code := h.requestReset(t, "ana@ex.com")
		for range identity.MaxResetAttempts {
			_ = h.confirm("ana@ex.com", wrongCode(code), "nova-senha-forte") // each wrong try is the point
		}
		h.clock.Advance(time.Minute)
	}
	fresh := h.requestReset(t, "ana@ex.com")

	if err := h.confirm("ana@ex.com", fresh, "nova-senha-forte"); !errors.Is(err, identity.ErrInvalidCode) {
		t.Errorf("a fresh code with the budget spent: %v, want ErrInvalidCode", err)
	}
	// A day later the budget is back.
	h.clock.Advance(identity.ResetBudgetWindow)
	again := h.requestReset(t, "ana@ex.com")
	if err := h.confirm("ana@ex.com", again, "nova-senha-forte"); err != nil {
		t.Errorf("a day later: %v", err)
	}
}

func TestConfirmPasswordReset_RefusesAlike(t *testing.T) {
	t.Parallel()

	cases := map[string]func(t *testing.T, h *harness) (string, identity.ResetCode){
		"unknown email": func(t *testing.T, h *harness) (string, identity.ResetCode) {
			return "ninguem@ex.com", h.requestReset(t, "ana@ex.com")
		},
		"never sent": func(*testing.T, *harness) (string, identity.ResetCode) { return "ana@ex.com", "123456" },
		"expired": func(t *testing.T, h *harness) (string, identity.ResetCode) {
			code := h.requestReset(t, "ana@ex.com")
			h.clock.Advance(identity.ResetCodeLifetime)
			return "ana@ex.com", code
		},
		"an older code": func(t *testing.T, h *harness) (string, identity.ResetCode) {
			old := h.requestReset(t, "ana@ex.com")
			h.clock.Advance(time.Second)
			if newer := h.requestReset(t, "ana@ex.com"); newer == old {
				t.Skip("the two codes happen to match")
			}
			return "ana@ex.com", old
		},
	}
	for name, setUp := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.signUp(t, "ana@ex.com", "segredo-forte")
			email, code := setUp(t, h)
			if err := h.confirm(email, code, "nova-senha-forte"); !errors.Is(err, identity.ErrInvalidCode) {
				t.Errorf("err = %v, want ErrInvalidCode", err)
			}
			if len(h.live(t)) != 1 {
				t.Error("a refused reset ended the session")
			}
		})
	}
}

func TestConfirmPasswordReset_BadInputSpendsNoAttempt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.signUp(t, "ana@ex.com", "segredo-forte")
	h.requestReset(t, "ana@ex.com")
	hashes := h.hasher.Hashes()

	err := h.svc.ConfirmPasswordReset(context.Background(), appidentity.ConfirmPasswordResetInput{Email: "nope", Code: "12a", Password: "curta"})

	if got := fieldsOf(t, err); !slices.Equal(got, []string{"email", "code", "password"}) {
		t.Errorf("fields = %v", got)
	}
	if h.store.ResetCodes()[0].Attempts != 0 || h.hasher.Hashes() != hashes {
		t.Error("invalid input spent an attempt or a hash")
	}
}

func TestConfirmPasswordReset_FailuresAreNotInvalidCode(t *testing.T) {
	t.Parallel()

	for _, method := range []string{"LockUserByEmail", "WrongResetAttemptsSince", "NewestResetCode", "AddWrongResetAttempt", "MarkResetCodeUsed", "UpdatePassword", "RevokeUserFamilies"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			h.signUp(t, "ana@ex.com", "segredo-forte")
			code := h.requestReset(t, "ana@ex.com")
			if method == "AddWrongResetAttempt" {
				code = wrongCode(code)
			}
			h.store.Fail(method, errBoom)
			if err := h.confirm("ana@ex.com", code, "nova-senha-forte"); !errors.Is(err, errBoom) || errors.Is(err, shared.ErrInvalid) {
				t.Errorf("err = %v, want the failure, not a 400", err)
			}
		})
	}
	t.Run("hasher", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.hasher.Fail(errBoom)
		if err := h.confirm("ana@ex.com", "123456", "nova-senha-forte"); !errors.Is(err, errBoom) {
			t.Errorf("err = %v", err)
		}
	})
}
