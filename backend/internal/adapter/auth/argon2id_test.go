package auth_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/test/contract/identityports"
)

// cheap returns argon2id costs low enough for tests that hash many times.
// The format and every check are the same at any cost.
func cheap() auth.Argon2Params { return auth.Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1} }

func newHasher(t *testing.T, p auth.Argon2Params) *auth.Hasher {
	t.Helper()
	h, err := auth.NewHasher(p, auth.DefaultMaxConcurrentHashes, auth.DefaultMaxHashWait)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestHasher_PassesThePasswordHasherContract(t *testing.T) {
	t.Parallel()
	identityports.RunPasswordHasherContract(t, func(t *testing.T) appidentity.PasswordHasher {
		return newHasher(t, cheap())
	})
}

// TestHasher_WithTheDefaultCosts_HashesAndVerifies runs the costs main uses
// once, so a bound that refused them would fail here and not at the first
// login.
func TestHasher_WithTheDefaultCosts_HashesAndVerifies(t *testing.T) {
	t.Parallel()
	h := newHasher(t, auth.DefaultArgon2Params())
	ctx := context.Background()

	hash, err := h.Hash(ctx, "segredo-forte")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(hash), "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Errorf("hash %q; want the default costs in it", hash)
	}
	if ok, err := h.Verify(ctx, hash, "segredo-forte"); !ok || err != nil {
		t.Errorf("Verify = %v, %v; want true", ok, err)
	}
}

func TestDefaultArgon2Params_AreTheSecondRecommendationOfRFC9106(t *testing.T) {
	t.Parallel()
	want := auth.Argon2Params{Memory: 64 * 1024, Iterations: 3, Parallelism: 4}
	if got := auth.DefaultArgon2Params(); got != want {
		t.Errorf("DefaultArgon2Params = %+v; want %+v", got, want)
	}
}

func TestHasher_WritesThePHCStringWithItsCosts(t *testing.T) {
	t.Parallel()
	h := newHasher(t, auth.Argon2Params{Memory: 128, Iterations: 2, Parallelism: 2})

	hash, err := h.Hash(context.Background(), "segredo-forte")
	if err != nil {
		t.Fatal(err)
	}
	// A 16-byte salt and a 32-byte key, in unpadded standard base64.
	phc := regexp.MustCompile(`^\$argon2id\$v=19\$m=128,t=2,p=2\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)
	if !phc.MatchString(string(hash)) {
		t.Errorf("hash %q; want the PHC string of argon2id", hash)
	}
}

// TestHasher_VerifiesAPHCStringItDidNotWrite: a hash written by the
// argon2 reference format, here built by hand, verifies; so do hashes made
// with other costs, which is what lets ADR 0019 raise the costs later.
func TestHasher_VerifiesAPHCStringItDidNotWrite(t *testing.T) {
	t.Parallel()
	b64 := base64.RawStdEncoding
	salt := []byte("somesalt-16bytes")
	key := argon2.IDKey([]byte("segredo-forte"), salt, 2, 256, 2, 24)
	hash := identity.PasswordHash(fmt.Sprintf("$argon2id$v=19$m=256,t=2,p=2$%s$%s", b64.EncodeToString(salt), b64.EncodeToString(key)))
	h := newHasher(t, cheap())

	for password, want := range map[string]bool{"segredo-forte": true, "segredo-fraco": false} {
		if ok, err := h.Verify(context.Background(), hash, password); ok != want || err != nil {
			t.Errorf("Verify(%q) = %v, %v; want %v", password, ok, err, want)
		}
	}
}

func TestHasher_RefusesUnreadableHashes(t *testing.T) {
	t.Parallel()
	h := newHasher(t, cheap())
	valid, err := h.Hash(context.Background(), "segredo-forte")
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Split(string(valid), "$") // "", argon2id, v=19, costs, salt, key
	b64 := func(n int) string { return base64.RawStdEncoding.EncodeToString(make([]byte, n)) }
	with := func(i int, value string) string {
		g := append([]string(nil), f...)
		g[i] = value
		return strings.Join(g, "$")
	}

	for name, hash := range map[string]string{
		"argon2i":                     with(1, "argon2i"),
		"version 16":                  with(2, "v=16"),
		"no version":                  strings.Join(append(f[:2:2], f[3:]...), "$"),
		"an extra cost":               with(3, "m=64,t=1,p=1,x=1"),
		"costs out of order":          with(3, "t=1,m=64,p=1"),
		"a leading zero":              with(3, "m=064,t=1,p=1"),
		"a plus sign":                 with(3, "m=+64,t=1,p=1"),
		"memory below eight per lane": with(3, "m=8,t=1,p=2"),
		"no passes":                   with(3, "m=64,t=0,p=1"),
		"no lanes":                    with(3, "m=64,t=1,p=0"),
		"four GiB of memory":          with(3, "m=4194304,t=1,p=1"),
		"more than the whole budget":  with(3, "m=1024,t=1,p=1"),
		"a thousand passes":           with(3, "m=64,t=1000,p=1"),
		"salt not base64":             with(4, "!!!!"),
		"salt of four bytes":          with(4, b64(4)),
		"key of eight bytes":          with(5, b64(8)),
		"key padded":                  with(5, f[5]+"="),
		"a line break in the key":     with(5, f[5][:10]+"\n"+f[5][10:]),
		"a line break in the salt":    with(4, f[4][:10]+"\r\n"+f[4][10:]),
		"key in the url alphabet":     with(5, "-"+f[5][1:]),
		"a trailing field":            string(valid) + "$x",
		"bcrypt":                      "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
		"the password itself":         "segredo-forte",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ok, err := h.Verify(context.Background(), identity.PasswordHash(hash), "segredo-forte")
			if ok || !errors.Is(err, auth.ErrUnreadableHash) {
				t.Fatalf("Verify = %v, %v; want false and ErrUnreadableHash", ok, err)
			}
			if strings.Contains(err.Error(), hash) || strings.Contains(err.Error(), "segredo-forte") {
				t.Errorf("the error %q carries the hash or the password", err)
			}
		})
	}
}

func TestNewHasher_RefusesCostsOutOfRange(t *testing.T) {
	t.Parallel()

	for name, p := range map[string]auth.Argon2Params{
		"no memory":                   {Memory: 0, Iterations: 1, Parallelism: 1},
		"no passes":                   {Memory: 64, Iterations: 0, Parallelism: 1},
		"no lanes":                    {Memory: 64, Iterations: 1, Parallelism: 0},
		"memory below eight per lane": {Memory: 8, Iterations: 1, Parallelism: 2},
		"four GiB of memory":          {Memory: 4 << 20, Iterations: 1, Parallelism: 1},
		"a thousand passes":           {Memory: 64, Iterations: 1000, Parallelism: 1},
		"a hundred lanes":             {Memory: 1024, Iterations: 1, Parallelism: 100},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if h, err := auth.NewHasher(p, 1, time.Second); err == nil || h != nil {
				t.Errorf("NewHasher(%+v) = %v, %v; want an error", p, h, err)
			}
		})
	}
	if h, err := auth.NewHasher(cheap(), 0, time.Second); err == nil || h != nil {
		t.Errorf("NewHasher with no slots = %v, %v; want an error", h, err)
	}
	if h, err := auth.NewHasher(cheap(), 1, 0); err == nil || h != nil {
		t.Errorf("NewHasher with no wait = %v, %v; want an error", h, err)
	}
}
