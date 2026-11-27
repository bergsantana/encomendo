package redis

import (
	"errors"
	"testing"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
)

func TestHasOwnerPrefix(t *testing.T) {
	t.Parallel()
	const id = "0189a3c0-7a3b-7000-8000-00000000000a"
	tests := []struct {
		key  string
		want bool
	}{
		{"t:" + id + ":catalog", true},
		{"t:" + id + ":catalog:item:1", true},
		{"g:workflow:presets", true},
		{"", false},
		{"g:", false},
		{"catalog:item:1", false},
		{"t:catalog:item:1", false},
		{"t:" + id, false},
		{"t:" + id + ":", false},
		{"t:not-a-uuid-not-a-uuid-not-a-uuid-xxxx:catalog", false},
		{"t:00000000-0000-0000-0000-000000000000:catalog", false},
		{"T:" + id + ":catalog", false},
		{"G:presets", false},
		{"auth:" + id, false},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			if got := hasOwnerPrefix(tc.key); got != tc.want {
				t.Errorf("hasOwnerPrefix(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

func TestCheckKey_RefusesTheZeroKey(t *testing.T) {
	t.Parallel()
	if err := checkKey(appshared.Key{}); !errors.Is(err, appshared.ErrInvalidCacheKey) {
		t.Errorf("checkKey(zero) = %v, want ErrInvalidCacheKey", err)
	}
}

func TestEscapeGlob(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"t:abc:catalog":  "t:abc:catalog",
		"a*b":            `a\*b`,
		"a?b[c]":         `a\?b\[c\]`,
		`back\slash`:     `back\\slash`,
		"g:presets.v-1_": "g:presets.v-1_",
	}
	for in, want := range tests {
		if got := escapeGlob(in); got != want {
			t.Errorf("escapeGlob(%q) = %q, want %q", in, got, want)
		}
	}
}
