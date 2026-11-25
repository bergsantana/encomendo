package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The "Esqueci a senha" limits of ADR 0011, also stated in the contract.
const (
	// ResetCodeLifetime is how long a code works.
	ResetCodeLifetime = 10 * time.Minute
	// MaxResetAttempts is how many wrong tries one code survives.
	MaxResetAttempts = 5
	// ResetBudget is how many wrong codes an account allows per
	// ResetBudgetWindow, across all its codes, so asking for new codes
	// never buys more guesses.
	ResetBudget       = 10
	ResetBudgetWindow = 24 * time.Hour
)

// ErrInvalidResetCode means a reset code record was built from missing
// parts. It is a bug, so it matches no sentinel; a wrong code typed by a
// person is ErrInvalidCode.
var ErrInvalidResetCode = errors.New("identity: invalid password reset code record")

// ResetCodeID identifies one emailed code.
type ResetCodeID shared.ID

// IsZero reports whether the id is unset.
func (id ResetCodeID) IsZero() bool { return shared.ID(id).IsZero() }

// ResetCode is the six-digit code emailed to reset a password.
type ResetCode string

// ParseResetCode checks that s is exactly six ASCII digits. It returns a
// ValidationError on the field "code" otherwise; a well-formed wrong code is
// for the use case to refuse with ErrInvalidCode.
func ParseResetCode(s string) (ResetCode, error) {
	if len(s) != 6 {
		return "", invalidCodeField()
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return "", invalidCodeField()
		}
	}
	return ResetCode(s), nil
}

func invalidCodeField() error {
	return shared.NewValidationError(shared.FieldError{Field: "code", Message: "Informe os 6 dígitos do código."})
}

// ResetCodeFrom turns random bytes into a code, leading zeros kept. The
// modulo bias over 64 bits is below one part in 10¹³.
func ResetCodeFrom(s shared.Secret) ResetCode {
	return ResetCode(fmt.Sprintf("%06d", binary.BigEndian.Uint64(s[:8])%1_000_000))
}

// Reveal returns the six digits, for the email. Only the mailer calls it.
func (c ResetCode) Reveal() string { return string(c) }

// String hides the code, so no %v, %s or log line prints it.
func (ResetCode) String() string { return shared.Redacted }

// GoString hides the code from %#v too.
func (ResetCode) GoString() string { return "identity.ResetCode(" + shared.Redacted + ")" }

// CodeHash is how a reset code is stored.
type CodeHash [32]byte

// HashResetCode returns HMAC-SHA256(key, user || code). A plain hash of six
// digits falls to a million guesses from a leaked table; the HMAC needs the
// server key too (ADR 0011). The user id in the input keeps one code from
// hashing alike for two accounts.
func HashResetCode(key []byte, user shared.UserID, code ResetCode) CodeHash {
	mac := hmac.New(sha256.New, key)
	mac.Write(user[:])
	mac.Write([]byte(code))
	return CodeHash(mac.Sum(nil))
}

// PasswordResetCode is one emailed code as stored. Only the newest code of
// an account counts; older rows stay for a day so the budget can sum their
// wrong attempts. The store counts attempts with one atomic update (ADR
// 0011), never by writing this value back.
type PasswordResetCode struct {
	ID        ResetCodeID
	User      shared.UserID
	Hash      CodeHash
	CreatedAt time.Time
	ExpiresAt time.Time
	Attempts  int       // wrong tries so far
	UsedAt    time.Time // zero until the code reset the password
}

// NewPasswordResetCode returns an unused code that expires
// ResetCodeLifetime after now. It returns ErrInvalidResetCode for a zero
// id, user or hash.
func NewPasswordResetCode(id ResetCodeID, user shared.UserID, hash CodeHash, now time.Time) (PasswordResetCode, error) {
	if id.IsZero() || user.IsZero() || hash == (CodeHash{}) {
		return PasswordResetCode{}, fmt.Errorf("new reset code of %v: %w", user, ErrInvalidResetCode)
	}
	return PasswordResetCode{ID: id, User: user, Hash: hash, CreatedAt: now, ExpiresAt: now.Add(ResetCodeLifetime)}, nil
}

// UsableAt reports whether the code can still be tried at now: unused, not
// expired, and fewer than MaxResetAttempts wrong tries.
func (c PasswordResetCode) UsableAt(now time.Time) bool {
	return c.UsedAt.IsZero() && now.Before(c.ExpiresAt) && c.Attempts < MaxResetAttempts
}

// Matches reports whether h is the code's hash, in constant time.
func (c PasswordResetCode) Matches(h CodeHash) bool {
	return subtle.ConstantTimeCompare(c.Hash[:], h[:]) == 1
}

// ResetBudgetSpent reports whether an account with wrong wrong attempts in
// the last ResetBudgetWindow may try no more codes. Anyone who knows the
// email can spend it; ADR 0011 accepts that, because logging in with the
// current password still works.
func ResetBudgetSpent(wrong int) bool { return wrong >= ResetBudget }
