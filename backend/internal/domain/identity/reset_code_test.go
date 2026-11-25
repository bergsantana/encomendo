package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

func TestParseResetCode(t *testing.T) {
	t.Parallel()

	if c, err := ParseResetCode("048213"); err != nil || c.Reveal() != "048213" {
		t.Errorf("ParseResetCode(048213) = %q, %v", c, err)
	}
	for _, bad := range []string{"", "12345", "1234567", "12a456", " 123456", "١٢٣٤٥٦"} {
		_, err := ParseResetCode(bad)
		var ve *shared.ValidationError
		if !errors.As(err, &ve) || ve.Fields[0].Field != "code" {
			t.Errorf("ParseResetCode(%q) = %v, want a ValidationError on code", bad, err)
		}
	}
}

func TestResetCodeFrom_IsSixDigitsAndSpreads(t *testing.T) {
	t.Parallel()

	seen := map[ResetCode]bool{}
	for i := range 200 {
		var s shared.Secret
		s[0], s[1], s[7] = byte(i), byte(i*31), byte(i*17)
		code := ResetCodeFrom(s)
		if _, err := ParseResetCode(code.Reveal()); err != nil {
			t.Fatalf("ResetCodeFrom gave %q: %v", code, err)
		}
		seen[code] = true
	}
	if len(seen) < 190 {
		t.Errorf("200 secrets gave only %d distinct codes", len(seen))
	}
	if got := ResetCodeFrom(shared.Secret{}); got != "000000" {
		t.Errorf("the zero secret gave %q, want the leading zeros kept", got)
	}
}

func TestHashResetCode_IsAnHMACBoundToTheUser(t *testing.T) {
	t.Parallel()

	key := []byte("a server key of at least 32 bytes!")
	code := ResetCode("482913")
	other := shared.UserID(shared.NewV7(2, [10]byte{2}))

	mac := hmac.New(sha256.New, key)
	mac.Write(userID[:])
	mac.Write([]byte("482913"))
	if got, want := HashResetCode(key, userID, code), CodeHash(mac.Sum(nil)); got != want {
		t.Errorf("HashResetCode = %x, want HMAC-SHA256(key, user || code) = %x", got, want)
	}
	if HashResetCode(key, other, code) == HashResetCode(key, userID, code) {
		t.Error("the same code hashes the same for two users")
	}
	if HashResetCode([]byte("another key, also 32 bytes long!!"), userID, code) == HashResetCode(key, userID, code) {
		t.Error("the key does not change the hash")
	}
}

func TestNewPasswordResetCode(t *testing.T) {
	t.Parallel()

	id := ResetCodeID(shared.NewV7(t0.UnixMilli(), [10]byte{7}))
	c, err := NewPasswordResetCode(id, userID, CodeHash{1}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if !c.ExpiresAt.Equal(t0.Add(10*time.Minute)) || c.Attempts != 0 || !c.UsedAt.IsZero() {
		t.Errorf("new code = %+v; want 10 minutes, no attempts, unused", c)
	}
	if _, err := NewPasswordResetCode(ResetCodeID{}, userID, CodeHash{1}, t0); !errors.Is(err, ErrInvalidResetCode) {
		t.Errorf("zero id: %v", err)
	}
	if _, err := NewPasswordResetCode(id, shared.UserID{}, CodeHash{1}, t0); !errors.Is(err, ErrInvalidResetCode) {
		t.Errorf("zero user: %v", err)
	}
	if _, err := NewPasswordResetCode(id, userID, CodeHash{}, t0); !errors.Is(err, ErrInvalidResetCode) {
		t.Errorf("zero hash: %v", err)
	}
}

func TestPasswordResetCode_UsableAt(t *testing.T) {
	t.Parallel()

	c, _ := NewPasswordResetCode(ResetCodeID(shared.NewV7(1, [10]byte{1})), userID, CodeHash{1}, t0)
	tired := c
	tired.Attempts = MaxResetAttempts
	almost := c
	almost.Attempts = MaxResetAttempts - 1
	spent := c
	spent.UsedAt = t0.Add(time.Minute)

	for name, tc := range map[string]struct {
		code PasswordResetCode
		now  time.Time
		want bool
	}{
		"fresh":                  {c, t0, true},
		"a second before expiry": {c, c.ExpiresAt.Add(-time.Second), true},
		"at expiry":              {c, c.ExpiresAt, false},
		"four wrong attempts":    {almost, t0, true},
		"five wrong attempts":    {tired, t0, false},
		"used":                   {spent, t0.Add(2 * time.Minute), false},
	} {
		if got := tc.code.UsableAt(tc.now); got != tc.want {
			t.Errorf("%s: UsableAt = %v, want %v", name, got, tc.want)
		}
	}
}

func TestResetCode_NeverPrintsItsValue(t *testing.T) {
	t.Parallel()

	c := ResetCode("482913")
	out := fmt.Sprintf("%s %v %+v %#v %q", c, c, struct{ Code ResetCode }{c}, c, c)
	if strings.Contains(out, "482913") || !strings.Contains(out, shared.Redacted) {
		t.Errorf("printed %q", out)
	}
}

func TestPasswordResetCode_Matches(t *testing.T) {
	t.Parallel()

	c := PasswordResetCode{Hash: CodeHash{1, 2, 3}}
	if !c.Matches(CodeHash{1, 2, 3}) || c.Matches(CodeHash{1, 2, 4}) {
		t.Error("Matches compares the wrong thing")
	}
}

func TestResetBudget(t *testing.T) {
	t.Parallel()

	// Ten wrong codes per account per day, however many codes it asked
	// for, so new codes never buy more guesses (ADR 0011).
	if ResetBudgetSpent(ResetBudget-1) || !ResetBudgetSpent(ResetBudget) || !ResetBudgetSpent(ResetBudget+3) {
		t.Errorf("budget of %d wrong codes is not enforced at the edge", ResetBudget)
	}
	if ResetBudget != 10 || ResetBudgetWindow != 24*time.Hour || MaxResetAttempts != 5 || ResetCodeLifetime != 10*time.Minute {
		t.Error("the reset limits changed; they are part of ADR 0011 and the contract")
	}
}
