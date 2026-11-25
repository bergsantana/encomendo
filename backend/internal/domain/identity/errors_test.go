package identity

import (
	"errors"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// TestErrors_MatchTheirSentinels pins what each error maps to, because the
// API answers with the sentinel's status and these codes are contract
// (docs/api/openapi.yaml).
func TestErrors_MatchTheirSentinels(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		err, sentinel error
	}{
		{ErrInvalidCredentials, shared.ErrUnauthenticated}, // 401 invalid_credentials
		{ErrEmailTaken, shared.ErrConflict},                // 409 email_taken
		{ErrInvalidCode, shared.ErrInvalid},                // 400 invalid_code
	} {
		if !errors.Is(c.err, c.sentinel) {
			t.Errorf("%v does not match %v", c.err, c.sentinel)
		}
	}
	// Programmer errors stay 500s.
	for _, bug := range []error{ErrInvalidUser, ErrInvalidRefreshToken, ErrInvalidResetCode, ErrInvalidClientAccount} {
		for _, s := range []error{shared.ErrNotFound, shared.ErrConflict, shared.ErrForbidden, shared.ErrInvalid, shared.ErrUnauthenticated} {
			if errors.Is(bug, s) {
				t.Errorf("%v matches %v", bug, s)
			}
		}
	}
}
