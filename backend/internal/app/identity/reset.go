package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// RequestPasswordReset stores a new six-digit code for the account with this
// email and has it emailed off the request path ("Esqueci a senha", ADR
// 0011). It returns nil whether or not the account exists, so the answer
// never tells; a ValidationError for a malformed email; and store or
// randomness failures. A failure to send is the mailer's to log.
func (s *Service) RequestPasswordReset(ctx context.Context, in PasswordResetInput) error {
	email, err := identity.ParseEmail(in.Email)
	if err != nil {
		return err
	}
	secret, err := s.secrets.NewSecret(ctx)
	if err != nil {
		return fmt.Errorf("draw reset code: %w", err)
	}
	code := identity.ResetCodeFrom(secret)
	now := s.clock.Now()
	var to *identity.User
	err = s.tx.WithinTx(publicScope(ctx), func(ctx context.Context) error {
		u, err := s.users.UserByEmail(ctx, email)
		if errors.Is(err, shared.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find user: %w", err)
		}
		c, err := identity.NewPasswordResetCode(identity.ResetCodeID(s.ids.NewID()), u.ID(), identity.HashResetCode(s.resetKey, u.ID(), code), now)
		if err != nil {
			return err
		}
		if err := s.resetCodes.InsertResetCode(ctx, c); err != nil {
			return fmt.Errorf("store reset code: %w", err)
		}
		to = u
		return nil
	})
	if err != nil || to == nil {
		return err
	}
	s.mailer.SendResetCode(ctx, ResetCodeEmail{To: to.Email(), Name: to.Name(), Code: code})
	return nil
}

// ConfirmPasswordReset sets a new password with the emailed code and
// revokes every refresh family of the account, denylisting their recent
// access tokens (ADR 0011). It returns a ValidationError for a malformed
// email or code or a password that breaks the policy, before any attempt is
// spent, and identity.ErrInvalidCode for an unknown email or a code that is
// wrong, expired, used up or not the newest, or when the account's daily
// budget of wrong codes is spent. A wrong code counts even though the call
// fails.
func (s *Service) ConfirmPasswordReset(ctx context.Context, in ConfirmPasswordResetInput) error {
	email, emailErr := identity.ParseEmail(in.Email)
	code, codeErr := identity.ParseResetCode(in.Code)
	if err := shared.MergeValidation(emailErr, codeErr, identity.CheckNewPassword(in.Password)); err != nil {
		return err
	}
	// argon2id before the transaction, so it never holds the account's lock
	// (ADR 0013); an unknown email costs the same.
	hash, err := s.hasher.Hash(ctx, in.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	now := s.clock.Now()
	var jtis []shared.ID
	var done bool
	err = s.tx.WithinTx(publicScope(ctx), func(ctx context.Context) error {
		var err error
		done, jtis, err = s.resetLocked(ctx, email, code, hash, now)
		return err
	})
	if err != nil {
		return err
	}
	if !done {
		return identity.ErrInvalidCode
	}
	if len(jtis) > 0 {
		s.denylist.Deny(ctx, jtis, now.Add(identity.DenylistWindow))
	}
	return nil
}

// resetLocked does the reset under the account's row lock. Every refusal
// returns false with a nil error, so the transaction commits and a wrong
// try stays counted.
func (s *Service) resetLocked(ctx context.Context, email identity.Email, code identity.ResetCode, hash identity.PasswordHash, now time.Time) (bool, []shared.ID, error) {
	u, err := s.users.LockUserByEmail(ctx, email)
	if errors.Is(err, shared.ErrNotFound) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("lock user: %w", err)
	}
	c, ok, err := s.usableCode(ctx, u.ID(), now)
	if err != nil || !ok {
		return false, nil, err
	}
	if !c.Matches(identity.HashResetCode(s.resetKey, u.ID(), code)) {
		if _, err := s.resetCodes.AddWrongResetAttempt(ctx, c.ID); err != nil {
			return false, nil, fmt.Errorf("count wrong reset code: %w", err)
		}
		return false, nil, nil
	}
	used, err := s.resetCodes.MarkResetCodeUsed(ctx, c.ID, now)
	if err != nil {
		return false, nil, fmt.Errorf("use reset code: %w", err)
	}
	if !used {
		return false, nil, nil
	}
	return s.setPassword(ctx, u, hash, now)
}

// usableCode returns the account's newest code if it may still be tried:
// the daily budget is not spent and the code is usable at now.
func (s *Service) usableCode(ctx context.Context, user shared.UserID, now time.Time) (identity.PasswordResetCode, bool, error) {
	wrong, err := s.resetCodes.WrongResetAttemptsSince(ctx, user, now.Add(-identity.ResetBudgetWindow))
	if err != nil {
		return identity.PasswordResetCode{}, false, fmt.Errorf("count wrong reset codes: %w", err)
	}
	if identity.ResetBudgetSpent(wrong) {
		return identity.PasswordResetCode{}, false, nil
	}
	c, err := s.resetCodes.NewestResetCode(ctx, user)
	if errors.Is(err, shared.ErrNotFound) {
		return identity.PasswordResetCode{}, false, nil
	}
	if err != nil {
		return identity.PasswordResetCode{}, false, fmt.Errorf("read reset code: %w", err)
	}
	return c, c.UsableAt(now), nil
}

// setPassword updates the account's row, then revokes its families, in that
// order (ADR 0013).
func (s *Service) setPassword(ctx context.Context, u *identity.User, hash identity.PasswordHash, now time.Time) (bool, []shared.ID, error) {
	if err := u.ChangePassword(hash); err != nil {
		return false, nil, err
	}
	if err := s.users.UpdatePassword(ctx, u); err != nil {
		return false, nil, fmt.Errorf("update password: %w", err)
	}
	jtis, err := s.refresh.RevokeUserFamilies(ctx, u.ID(), now, now.Add(-identity.DenylistWindow))
	if err != nil {
		return false, nil, fmt.Errorf("revoke sessions: %w", err)
	}
	return true, jtis, nil
}
