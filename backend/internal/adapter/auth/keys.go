package auth

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
)

// ErrInvalidKey means a key text is not a 32-byte Ed25519 seed, or a key set
// was built with a zero key. Its text never carries the key.
var ErrInvalidKey = errors.New("auth: invalid Ed25519 key")

// Key is an Ed25519 private key, built only from its seed. It prints as its
// kid, through fmt and slog alike, so a logged configuration never carries
// it (backend/CLAUDE.md, Logging). fmt cannot call String on a field it
// cannot reach, such as an unexported field of a config struct, and then
// prints what it finds by reflection; so the private key is held only by
// the signing func, which fmt prints as an address whatever the verb. The
// zero Key is no key.
type Key struct {
	m *keyMaterial
}

type keyMaterial struct {
	public ed25519.PublicKey
	kid    string
	sign   func(message []byte) []byte
}

// Public implements crypto.Signer.
func (m *keyMaterial) Public() crypto.PublicKey { return m.public }

// Sign implements crypto.Signer for Ed25519, which signs the message
// itself: opts must ask for no hash.
func (m *keyMaterial) Sign(_ io.Reader, message []byte, opts crypto.SignerOpts) ([]byte, error) {
	if opts.HashFunc() != crypto.Hash(0) {
		return nil, errors.New("auth: Ed25519 signs the message unhashed")
	}
	return m.sign(message), nil
}

// ParseKey reads an Ed25519 key from its 32-byte seed in padded standard
// base64, the form `openssl rand -base64 32` prints (ADR 0019). Whitespace
// around it is ignored. It returns ErrInvalidKey for anything else,
// including a line break inside, which the base64 decoder would skip, and a
// seed of one repeated byte, such as an all-A placeholder left in a config.
func ParseKey(text string) (Key, error) {
	text = strings.TrimSpace(text)
	seed, err := base64.StdEncoding.Strict().DecodeString(text)
	if err != nil || len(seed) != ed25519.SeedSize || base64.StdEncoding.EncodeToString(seed) != text ||
		strings.Count(string(seed), string(seed[:1])) == len(seed) {
		return Key{}, ErrInvalidKey
	}
	// Rebuilt from the seed, the public half always matches the private one:
	// signing with a mismatched public half can leak an Ed25519 key.
	private := ed25519.NewKeyFromSeed(seed)
	public, ok := private.Public().(ed25519.PublicKey)
	if !ok {
		return Key{}, ErrInvalidKey
	}
	sign := func(message []byte) []byte { return ed25519.Sign(private, message) }
	return Key{m: &keyMaterial{public: public, kid: thumbprint(public), sign: sign}}, nil
}

// KID returns the key's RFC 7638 thumbprint, the kid of the tokens it
// signs; empty for the zero Key. It is public and safe to log.
func (k Key) KID() string {
	if k.m == nil {
		return ""
	}
	return k.m.kid
}

// IsZero reports whether k is no key.
func (k Key) IsZero() bool { return k.m == nil }

// String shows the kid only.
func (k Key) String() string { return "auth.Key(" + k.KID() + ")" }

// GoString shows the kid only, for %#v.
func (k Key) GoString() string { return k.String() }

// LogValue shows the kid only, for slog.
func (k Key) LogValue() slog.Value { return slog.StringValue(k.String()) }

// KeySet is the key that signs access tokens and every key that verifies
// them, each named by its kid. It prints as its kids only. The zero KeySet
// has no keys.
type KeySet struct {
	signing   Key
	verifying map[string]ed25519.PublicKey
}

// NewKeySet returns a key set that signs with signing and verifies with it
// and with verifyOnly: the next key before a rotation, the previous one
// after it (ADR 0019). Only the public halves of verifyOnly are kept. Each
// key is named by its kid, so the configuration never names one and two
// instances always agree. It returns ErrInvalidKey for a zero key.
func NewKeySet(signing Key, verifyOnly ...Key) (KeySet, error) {
	if signing.IsZero() {
		return KeySet{}, ErrInvalidKey
	}
	k := KeySet{signing: signing, verifying: map[string]ed25519.PublicKey{}}
	for _, key := range append([]Key{signing}, verifyOnly...) {
		if key.IsZero() {
			return KeySet{}, ErrInvalidKey
		}
		k.verifying[key.m.kid] = key.m.public
	}
	return k, nil
}

// KID returns the kid of the signing key, the one new tokens carry; empty
// for the zero KeySet.
func (k KeySet) KID() string { return k.signing.KID() }

// KIDs returns every kid that verifies, sorted, for the startup log that
// shows a verify-only key left behind after a rotation (ADR 0019).
func (k KeySet) KIDs() []string {
	kids := make([]string, 0, len(k.verifying))
	for kid := range k.verifying {
		kids = append(kids, kid)
	}
	slices.Sort(kids)
	return kids
}

// String shows the kids only.
func (k KeySet) String() string {
	return "auth.KeySet{signing: " + k.KID() + ", verifying: [" + strings.Join(k.KIDs(), " ") + "]}"
}

// GoString shows the kids only, for %#v.
func (k KeySet) GoString() string { return k.String() }

// LogValue shows the kids only, for slog.
func (k KeySet) LogValue() slog.Value { return slog.StringValue(k.String()) }

// verifier returns the public key named kid.
func (k KeySet) verifier(kid string) (ed25519.PublicKey, bool) {
	key, ok := k.verifying[kid]
	return key, ok
}

// thumbprint is the RFC 7638 thumbprint of an Ed25519 public key: the
// SHA-256 of its JWK's required members in lexical order (RFC 8037 2),
// in unpadded URL-safe base64.
func thumbprint(key ed25519.PublicKey) string {
	jwk := `{"crv":"Ed25519","kty":"OKP","x":"` + base64.RawURLEncoding.EncodeToString(key) + `"}`
	sum := sha256.Sum256([]byte(jwk))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
