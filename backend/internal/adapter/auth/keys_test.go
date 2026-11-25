package auth_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
)

// The Ed25519 key of RFC 8037, appendix A, with its RFC 7638 thumbprint
// (A.3): a published vector, so the kid is checked against something this
// package did not compute.
const (
	rfc8037Seed       = "nWGxne_9WmC6hEr0kuwsxERJxWl7MmkZcDusAxyuf2A"
	rfc8037Thumbprint = "kPrK_qmxVWaYVA9wwBF6Iuo3vVzz7TxHCTwXBygrS4k"
)

// rfc8037Key returns the RFC 8037 seed as configuration holds it: standard
// base64 with padding, as `openssl rand -base64 32` prints (ADR 0019).
func rfc8037Key(t *testing.T) string {
	t.Helper()
	seed, err := base64.RawURLEncoding.DecodeString(rfc8037Seed)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(seed)
}

func TestParseKey_NamesTheKeyOfRFC8037ByItsThumbprint(t *testing.T) {
	t.Parallel()

	key, err := auth.ParseKey(" " + rfc8037Key(t) + "\n")
	if err != nil {
		t.Fatalf("ParseKey: %v", err)
	}
	if key.KID() != rfc8037Thumbprint {
		t.Errorf("KID = %q; want the RFC 7638 thumbprint %q", key.KID(), rfc8037Thumbprint)
	}
	if got := keySet(t, key).KID(); got != rfc8037Thumbprint {
		t.Errorf("KeySet.KID = %q; want %q", got, rfc8037Thumbprint)
	}
}

func TestParseKey_RefusesAnythingButThirtyTwoBytesOfBase64(t *testing.T) {
	t.Parallel()
	b64 := func(n int) string { return base64.StdEncoding.EncodeToString(make([]byte, n)) }
	valid := rfc8037Key(t)

	for name, text := range map[string]string{
		"empty":               "",
		"not base64":          "not a key at all!",
		"31 bytes":            b64(31),
		"33 bytes":            b64(33),
		"a whole private key": b64(ed25519.PrivateKeySize),
		"url alphabet":        rfc8037Seed,
		"no padding":          strings.TrimRight(valid, "="),
		"a line break inside": valid[:20] + "\n" + valid[20:],
		"a zero placeholder":  b64(ed25519.SeedSize),
		"one repeated byte":   base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x41}, ed25519.SeedSize)),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			key, err := auth.ParseKey(text)
			if !errors.Is(err, auth.ErrInvalidKey) || !key.IsZero() {
				t.Fatalf("ParseKey = %v, %v; want ErrInvalidKey", key, err)
			}
			if text != "" && strings.Contains(err.Error(), text) {
				t.Errorf("the error %q carries the key text", err)
			}
		})
	}
}

func TestNewKeySet_RefusesAZeroKey(t *testing.T) {
	t.Parallel()
	good, _ := newKey(t)

	if _, err := auth.NewKeySet(auth.Key{}); !errors.Is(err, auth.ErrInvalidKey) {
		t.Errorf("no signing key: %v; want ErrInvalidKey", err)
	}
	if _, err := auth.NewKeySet(good, auth.Key{}); !errors.Is(err, auth.ErrInvalidKey) {
		t.Errorf("a zero verify-only key: %v; want ErrInvalidKey", err)
	}
}

func TestKeySet_ListsEveryKidThatVerifiesOnce(t *testing.T) {
	t.Parallel()
	signing, _ := newKey(t)
	next, _ := newKey(t)

	want := []string{signing.KID(), next.KID()}
	slices.Sort(want)
	if got := keySet(t, signing, next, signing).KIDs(); !slices.Equal(got, want) {
		t.Errorf("KIDs = %v; want %v", got, want)
	}
}

// TestKeys_NeverPrintTheirSeed: a logged configuration or a wrapped startup
// error must not carry a signing key, through any verb or slog handler, and
// not from an unexported field either (backend/CLAUDE.md, Logging). Whoever
// held the seed could sign owner tokens for every store.
func TestKeys_NeverPrintTheirSeed(t *testing.T) {
	t.Parallel()
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	key, err := auth.ParseKey(base64.StdEncoding.EncodeToString(seed))
	if err != nil {
		t.Fatal(err)
	}
	keys := keySet(t, key)
	tokens := accessTokens(t, keys, issuer, sharedfake.NewClock(start))
	private := ed25519.NewKeyFromSeed(seed)
	leaks := []string{
		base64.StdEncoding.EncodeToString(seed), base64.RawURLEncoding.EncodeToString(seed),
		hex.EncodeToString(seed), strings.Trim(fmt.Sprintf("%d", seed), "[]")[:40], string(seed),
		base64.StdEncoding.EncodeToString(private), base64.RawURLEncoding.EncodeToString(private),
	}
	type config struct {
		Exported   auth.Key
		unexported auth.Key
		set        auth.KeySet
		tokens     *auth.AccessTokens
	}
	values := []any{key, keys, tokens, config{key, key, keys, tokens}, &config{key, key, keys, tokens}}

	var out bytes.Buffer
	for _, v := range values {
		fmt.Fprintf(&out, "%v %+v %#v %s\n", v, v, v, v)
	}
	for _, h := range []slog.Handler{slog.NewTextHandler(&out, nil), slog.NewJSONHandler(&out, nil)} {
		slog.New(h).Info("config", "key", key, "keys", keys, "tokens", tokens, "config", values[3])
	}
	for _, leak := range leaks {
		if strings.Contains(out.String(), leak) {
			t.Fatalf("the seed got out (%q) in:\n%s", leak, out.String())
		}
	}
	if !strings.Contains(out.String(), key.KID()) {
		t.Errorf("the kid, which is safe to show, is missing from:\n%s", out.String())
	}
}

// newKey returns a fresh key, and its raw private key for forging tokens.
func newKey(t testing.TB) (auth.Key, ed25519.PrivateKey) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	key, err := auth.ParseKey(base64.StdEncoding.EncodeToString(seed))
	if err != nil {
		t.Fatal(err)
	}
	return key, ed25519.NewKeyFromSeed(seed)
}
