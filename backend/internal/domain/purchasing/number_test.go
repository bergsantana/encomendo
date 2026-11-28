package purchasing_test

import (
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/purchasing"
)

func TestNumber_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		n    purchasing.Number
		want string
	}{{1, "C-0001"}, {12, "C-0012"}, {9999, "C-9999"}, {12345, "C-12345"}}
	for _, tc := range tests {
		if got := tc.n.String(); got != tc.want {
			t.Errorf("Number(%d).String() = %q, want %q", tc.n, got, tc.want)
		}
	}
	if purchasing.Number(0).Valid() || !purchasing.Number(1).Valid() {
		t.Error("Valid: 0 must be invalid and 1 valid")
	}
}
