package identity

import (
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Errors a person can cause. Each matches the sentinel that gives its HTTP
// status; respond/problem.go gives each its own contract code.
var (
	// ErrInvalidCredentials means the email has no account or the password
	// is wrong; the two are never told apart (ADR 0010). It matches
	// shared.ErrUnauthenticated: 401 invalid_credentials, a form error that
	// never calls for a refresh (ADR 0013).
	ErrInvalidCredentials = fmt.Errorf("identity: wrong email or password: %w", shared.ErrUnauthenticated)
	// ErrEmailTaken means sign-up met an email that already has an account.
	// It matches shared.ErrConflict: 409 email_taken.
	ErrEmailTaken = fmt.Errorf("identity: email already has an account: %w", shared.ErrConflict)
	// ErrInvalidCode means a password reset code is wrong, expired, used up
	// or was never sent, or the account's budget is spent; the cases are
	// never told apart. It matches shared.ErrInvalid: 400 invalid_code.
	ErrInvalidCode = fmt.Errorf("identity: wrong, expired or used-up reset code: %w", shared.ErrInvalid)
)
