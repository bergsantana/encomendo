package auth_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/adapter/auth"
)

func seed(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i*7 + 3)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// The development key is refused outside development wherever it appears,
// and there is no fallback key (ADR 0019).
func TestLoadKeys(t *testing.T) {
	t.Parallel()

	own := seed(t)
	tests := []struct {
		name     string
		signing  string
		verify   []string
		allowDev bool
		wantErr  string
	}{
		{name: "a seed of one's own", signing: own},
		{name: "a rotation", signing: own, verify: []string{auth.DevSigningSeed}, allowDev: true},
		{name: "no signing seed", signing: "", wantErr: "no fallback key"},
		{name: "a bad signing seed", signing: "not base64", wantErr: "signing seed"},
		{name: "a bad verify-only seed", signing: own, verify: []string{"short"}, wantErr: "verify-only seed 1"},
		{name: "the dev key in development", signing: auth.DevSigningSeed, allowDev: true},
		{name: "the dev key as signing key in production", signing: auth.DevSigningSeed, wantErr: "development signing key"},
		{name: "the dev key as verify-only key in production", signing: own, verify: []string{auth.DevSigningSeed}, wantErr: "development signing key"},
		{name: "the dev key with spaces around it", signing: "  " + auth.DevSigningSeed + "\n", wantErr: "development signing key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			keys, err := auth.LoadKeys(tt.signing, tt.verify, tt.allowDev)
			if tt.wantErr == "" {
				if err != nil || len(keys.KIDs()) != 1+len(tt.verify) {
					t.Fatalf("LoadKeys = %v, %v", keys, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("LoadKeys error = %v, want one about %q", err, tt.wantErr)
			}
		})
	}
}
