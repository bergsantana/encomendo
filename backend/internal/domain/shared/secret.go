package shared

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// ErrMalformedSecret means a string is not a secret in the form Secret.String
// writes. It matches no sentinel on purpose: a malformed refresh token is a
// 401, a malformed invitation link a 404, so each caller maps it.
var ErrMalformedSecret = errors.New("malformed secret")

// Secret is 32 random bytes handed to a client once, as an opaque token:
// refresh tokens, the random part of invitation links, order tracking
// tokens. The server stores only its Hash (docs/architecture.md 5.1, 5.2).
// It is built from bytes the caller drew from a SecretGenerator, so tests can
// fix it, as NewV7 does for ids.
type Secret [32]byte

// SecretHash is the SHA-256 of a Secret, the only form that is stored.
// A plain hash is enough: 256 random bits cannot be guessed, unlike a
// six-digit code, which is stored as an HMAC instead.
type SecretHash [32]byte

// Hash returns the SHA-256 of the secret's bytes.
func (s Secret) Hash() SecretHash { return sha256.Sum256(s[:]) }

// Reveal returns the secret as 43 characters of unpadded URL-safe base64,
// fit for a cookie, a JSON body or a URL fragment. Call it only where the
// secret is handed to its owner.
func (s Secret) Reveal() string { return base64.RawURLEncoding.EncodeToString(s[:]) }

// Redacted is what every secret type prints as through fmt, so no %v, %s
// or log line ever shows one (backend/CLAUDE.md, Logging).
const Redacted = "[redacted]"

// String hides the secret; Reveal gives the value.
func (Secret) String() string { return Redacted }

// GoString hides the secret from %#v too.
func (Secret) GoString() string { return "shared.Secret(" + Redacted + ")" }

// ParseSecret reads what Reveal writes, and only that: it returns
// ErrMalformedSecret for any other length, alphabet or padding, and for a
// non-canonical last character, so every secret has exactly one spelling.
func ParseSecret(text string) (Secret, error) {
	var s Secret
	if len(text) != base64.RawURLEncoding.EncodedLen(len(s)) {
		return Secret{}, ErrMalformedSecret
	}
	n, err := base64.RawURLEncoding.Strict().Decode(s[:], []byte(text))
	if err != nil || n != len(s) {
		return Secret{}, ErrMalformedSecret
	}
	return s, nil
}
