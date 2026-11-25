package identity

import (
	"unicode/utf8"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// The password policy of docs/api/openapi.yaml (Password): 8 to 128
// characters, nothing else. Length beats composition rules, and the upper
// bound keeps argon2id's input small.
const (
	MinPasswordLength = 8
	MaxPasswordLength = 128
)

// PasswordHash is a password as the PasswordHasher stored it, an argon2id
// PHC string. The domain never reads it; it only keeps it with its user.
type PasswordHash string

// CheckNewPassword applies the password policy to a password being chosen
// (sign-up, reset, a new account through an invitation). It returns a
// ValidationError on the field "password".
func CheckNewPassword(p string) error {
	if n := utf8.RuneCountInString(p); n < MinPasswordLength || n > MaxPasswordLength {
		return shared.NewValidationError(shared.FieldError{
			Field:   "password",
			Message: "A senha precisa ter de 8 a 128 caracteres.",
		})
	}
	return nil
}

// CheckPresentedPassword bounds a password typed to log in. A wrong password
// is invalid_credentials, never a field error, so the only check is that the
// input is not longer than any password could be. It returns a
// ValidationError on the field "password" for more than 128 characters.
func CheckPresentedPassword(p string) error {
	if utf8.RuneCountInString(p) > MaxPasswordLength {
		return shared.NewValidationError(shared.FieldError{
			Field:   "password",
			Message: "A senha tem no máximo 128 caracteres.",
		})
	}
	return nil
}
