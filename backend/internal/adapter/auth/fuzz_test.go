package auth_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// FuzzAccessTokens_Verify: whatever the text, Verify accepts nothing but
// the exact token Issue signed. A second spelling of an accepted token (a
// changed base64 bit, a reordered header) would be a forgery the signature
// did not catch.
func FuzzAccessTokens_Verify(f *testing.F) {
	fx := newFixture(f)
	valid := fx.issue(f, fx.staff(f))
	parts := strings.Split(valid, ".")
	f.Add(valid)
	f.Add(parts[0] + "." + parts[1] + ".")
	f.Add(strings.ToUpper(valid))
	f.Add(valid[:len(valid)-1] + "B")
	f.Add(valid[:len(valid)-10] + "\n" + valid[len(valid)-10:])
	f.Add("eyJhbGciOiJub25lIn0." + parts[1] + ".")

	f.Fuzz(func(t *testing.T, raw string) {
		c, err := fx.tokens.Verify(context.Background(), raw, shared.AudienceBackoffice)
		if err == nil && raw != valid {
			t.Fatalf("Verify accepted %q as %+v", raw, c)
		}
	})
}
