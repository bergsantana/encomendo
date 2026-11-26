package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// PreviewInvitation tells the app what an invitation link offers and
// whether its email already has an account. It reads only in the scope of
// the store inside the token. A malformed, unknown, expired, revoked or used
// link, or one whose sender can no longer manage the team, is
// tenancy.ErrInvitationNotPending (404), the same for each.
func (s *Service) PreviewInvitation(ctx context.Context, token string) (InvitationPreview, error) {
	tok, err := tenancy.ParseInvitationToken(token)
	if err != nil {
		return InvitationPreview{}, err
	}
	now := s.clock.Now()
	var p InvitationPreview
	err = s.tx.WithinTx(storeScope(ctx, tok.TenantID()), func(ctx context.Context) error {
		inv, user, err := s.readInvitation(ctx, tok, now)
		if err != nil {
			return err
		}
		p = InvitationPreview{
			Tenant: inv.Tenant, Email: inv.Email.String(), Role: inv.Role, ExpiresAt: inv.ExpiresAt, AccountExists: user != nil,
		}
		return nil
	})
	return p, err
}

// readInvitation reads the pending invitation and the account of its email,
// nil when there is none.
func (s *Service) readInvitation(ctx context.Context, tok tenancy.InvitationToken, now time.Time) (PendingInvitation, *identity.User, error) {
	inv, err := s.invitations.PendingInvitation(ctx, tok.TenantID(), tok.Hash(), now)
	if err != nil {
		return PendingInvitation{}, nil, fmt.Errorf("read invitation: %w", err)
	}
	if inv.Tenant.Suspended {
		// Joining writes into the store, so a suspended one refuses it.
		return PendingInvitation{}, nil, fmt.Errorf("read invitation: %w", tenancy.ErrTenantSuspended)
	}
	u, err := s.users.UserByEmail(ctx, inv.Email)
	if errors.Is(err, shared.ErrNotFound) {
		return inv, nil, nil
	}
	if err != nil {
		return PendingInvitation{}, nil, fmt.Errorf("find invited user: %w", err)
	}
	return inv, u, nil
}

// AcceptInvitation joins the invitation's store and starts a session there
// (ADR 0011). Without an account for the invited email, in.Name and
// in.Password create one; with one, in.Password must be its password, so
// holding a link never logs anyone in as someone else.
//
// It runs in three transactions. The first two are scoped to the store
// inside the token: one reads the invitation before argon2id runs outside
// any transaction (ADR 0013), the other uses it up and adds the membership.
// The last is scoped to the user and builds the session, re-checking the
// password hash FOR SHARE.
//
// It returns tenancy.ErrInvitationNotPending for a bad link, a
// ValidationError for a new account's name or password,
// identity.ErrInvalidCredentials for a wrong password,
// tenancy.ErrAlreadyMember when the account is already in the store (the
// invitation stays pending), and identity.ErrEmailTaken when someone signed
// up with the invited email in the meantime.
//
// The membership commits before the session is built. If the last
// transaction fails, the call fails, the link is used up, and the person is
// a member: they log in normally.
func (s *Service) AcceptInvitation(ctx context.Context, in AcceptInvitationInput) (AuthResult, error) {
	tok, err := tenancy.ParseInvitationToken(in.Token)
	if err != nil {
		return AuthResult{}, err
	}
	if err := identity.CheckPresentedPassword(in.Password); err != nil {
		return AuthResult{}, err
	}
	now := s.clock.Now()
	store := storeScope(ctx, tok.TenantID())
	var inv PendingInvitation
	var existing *identity.User
	err = s.tx.WithinTx(store, func(ctx context.Context) error {
		var err error
		inv, existing, err = s.readInvitation(ctx, tok, now)
		return err
	})
	if err != nil {
		return AuthResult{}, err
	}
	acct, err := s.account(ctx, existing, inv.Email, in)
	if err != nil {
		return AuthResult{}, err
	}
	fresh, err := s.drawToken(ctx)
	if err != nil {
		return AuthResult{}, err
	}
	if err := s.tx.WithinTx(store, func(ctx context.Context) error { return s.join(ctx, tok, acct, now) }); err != nil {
		return AuthResult{}, err
	}
	return s.startInvitedSession(ctx, tok.TenantID(), acct, fresh, now)
}

// account is who accepts: an existing account whose password was verified,
// or a new one to insert.
type account struct {
	user  *identity.User
	hash  identity.PasswordHash
	isNew bool
}

// account verifies the existing account's password, or validates and
// builds the new account, outside any transaction.
func (s *Service) account(ctx context.Context, existing *identity.User, email identity.Email, in AcceptInvitationInput) (account, error) {
	if existing != nil {
		hash, err := s.checkPassword(ctx, existing, in.Password)
		return account{user: existing, hash: hash}, err
	}
	name, nameErr := identity.ParsePersonName(in.Name)
	if err := shared.MergeValidation(nameErr, identity.CheckNewPassword(in.Password)); err != nil {
		return account{}, err
	}
	hash, err := s.hasher.Hash(ctx, in.Password)
	if err != nil {
		return account{}, fmt.Errorf("hash password: %w", err)
	}
	u, err := identity.NewUser(shared.UserID(s.ids.NewID()), identity.Profile{Name: name, Email: email}, hash)
	if err != nil {
		return account{}, err
	}
	return account{user: u, hash: hash, isNew: true}, nil
}

// join inserts a new account or re-checks an existing one's password, then
// uses the invitation up, in the store's scope.
func (s *Service) join(ctx context.Context, tok tenancy.InvitationToken, acct account, now time.Time) error {
	if acct.isNew {
		if err := s.insertUser(ctx, acct.user); err != nil {
			return err
		}
	} else if err := s.recheckPassword(ctx, acct.user.ID(), acct.hash); err != nil {
		return err
	}
	err := s.invitations.JoinByInvitation(ctx, Join{
		Tenant: tok.TenantID(), Hash: tok.Hash(), User: acct.user.ID(), Email: acct.user.Email(), At: now,
	})
	if err != nil {
		return fmt.Errorf("join store: %w", err)
	}
	return nil
}

// startInvitedSession starts the session in the store just joined, in the
// user's own scope.
func (s *Service) startInvitedSession(ctx context.Context, tenant shared.TenantID, acct account, fresh freshToken, now time.Time) (AuthResult, error) {
	var sess session
	err := s.tx.WithinTx(userScope(ctx, acct.user.ID()), func(ctx context.Context) error {
		if err := s.recheckPassword(ctx, acct.user.ID(), acct.hash); err != nil {
			return err
		}
		ms, err := s.memberships.MembershipsOf(ctx, acct.user.ID())
		if err != nil {
			return fmt.Errorf("read memberships: %w", err)
		}
		selected, _ := find(ms, tenant) // just joined; a concurrent removal leaves no store
		sess = session{user: acct.user, memberships: ms, selected: selected}
		return s.startLogin(ctx, acct.user.ID(), selected.Tenant.ID, fresh, now)
	})
	if err != nil {
		return AuthResult{}, err
	}
	return s.issue(ctx, sess, fresh, now)
}
