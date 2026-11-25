package tenancy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// Invitations returns one page of the invitations pending now, newest
// first (members:manage).
func (s *Service) Invitations(ctx context.Context, page shared.PageRequest) (shared.Page[InvitationView], error) {
	by, err := authz.Require(ctx, identity.PermMembersManage)
	if err != nil {
		return shared.Page[InvitationView]{}, err
	}
	now := s.clock.Now()
	var out shared.Page[InvitationView]
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := s.invitations.PendingInvitations(ctx, by.TenantID(), now, page)
		if err != nil {
			return fmt.Errorf("list invitations: %w", err)
		}
		out = shared.Page[InvitationView]{Items: make([]InvitationView, 0, len(p.Items)), NextCursor: p.NextCursor}
		for _, inv := range p.Items {
			out.Items = append(out.Items, invitationView(inv))
		}
		return nil
	})
	return out, err
}

// InviteMember invites an email to the team with any role but owner
// (members:manage, checked against the caller's current membership; ADR
// 0011). An open invitation for the same email, expired or not, is revoked
// and replaced: that is "Reenviar convite". The link goes only into the
// email.
//
// It returns a ValidationError for the email or role,
// tenancy.ErrAlreadyMember when the email's account is in the team, and
// ErrEmailUnavailable when the invitation was saved but the email could not
// be sent.
func (s *Service) InviteMember(ctx context.Context, in InviteMemberInput) (InvitationView, error) {
	by, err := authz.Require(ctx, identity.PermMembersManage)
	if err != nil {
		return InvitationView{}, err
	}
	email, emailErr := identity.ParseEmail(in.Email)
	role, roleErr := tenancy.ParseInvitableRole(in.Role)
	if err := shared.MergeValidation(emailErr, roleErr); err != nil {
		return InvitationView{}, err
	}
	secret, err := s.secrets.NewSecret(ctx)
	if err != nil {
		return InvitationView{}, fmt.Errorf("draw invitation token: %w", err)
	}
	token := tenancy.NewInvitationToken(by.TenantID(), secret)
	spec := tenancy.InvitationSpec{
		ID: shared.InvitationID(s.ids.NewID()), Tenant: by.TenantID(), Email: email, Role: role,
		Hash: token.Hash(), InvitedBy: by.UserID(),
	}
	now := s.clock.Now()
	var inv *tenancy.Invitation
	var mail InvitationEmail
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		inv, mail, err = s.invite(ctx, by, spec, token, now)
		return err
	})
	if err != nil {
		return InvitationView{}, err
	}
	// The invitation stays when the email fails; inviting again replaces it.
	if err := s.mailer.SendInvitation(ctx, mail); err != nil {
		return InvitationView{}, fmt.Errorf("%w: %w", ErrEmailUnavailable, err)
	}
	return invitationView(inv), nil
}

func (s *Service) invite(ctx context.Context, by shared.Principal, spec tenancy.InvitationSpec, token tenancy.InvitationToken, now time.Time) (*tenancy.Invitation, InvitationEmail, error) {
	if err := s.manager(ctx, by); err != nil {
		return nil, InvitationEmail{}, err
	}
	if err := s.checkNotMember(ctx, spec.Tenant, spec.Email); err != nil {
		return nil, InvitationEmail{}, err
	}
	if err := s.closeOpenInvitation(ctx, by, spec.Email, now); err != nil {
		return nil, InvitationEmail{}, err
	}
	inv, err := tenancy.NewInvitation(spec, now)
	if err != nil {
		return nil, InvitationEmail{}, err
	}
	if err := s.invitations.InsertInvitation(ctx, inv); err != nil {
		return nil, InvitationEmail{}, fmt.Errorf("insert invitation: %w", err)
	}
	mail, err := s.invitationEmail(ctx, inv, token)
	if err != nil {
		return nil, InvitationEmail{}, err
	}
	err = s.events.Publish(ctx, tenancy.MemberInvited{InvitationID: inv.ID(), Role: inv.Role(), InvitedBy: by.UserID()})
	return inv, mail, err
}

// checkNotMember returns tenancy.ErrAlreadyMember when email's account
// belongs to the store.
func (s *Service) checkNotMember(ctx context.Context, tenant shared.TenantID, email identity.Email) error {
	u, err := s.users.UserByEmail(ctx, email)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find invited account: %w", err)
	}
	_, err = s.members.MembershipOf(ctx, tenant, u.ID)
	switch {
	case err == nil:
		return tenancy.ErrAlreadyMember
	case errors.Is(err, shared.ErrNotFound):
		return nil
	default:
		return fmt.Errorf("read invited account's membership: %w", err)
	}
}

// closeOpenInvitation revokes the email's open invitation, expired or not,
// which the one-open-invitation key would otherwise refuse to replace.
func (s *Service) closeOpenInvitation(ctx context.Context, by shared.Principal, email identity.Email, now time.Time) error {
	old, err := s.invitations.OpenInvitationByEmail(ctx, by.TenantID(), email)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find open invitation: %w", err)
	}
	if _, err := s.invitations.MarkInvitationRevoked(ctx, by.TenantID(), old.ID(), now); err != nil {
		return fmt.Errorf("replace invitation: %w", err)
	}
	return s.events.Publish(ctx, tenancy.InvitationRevoked{InvitationID: old.ID(), By: by.UserID()})
}

func (s *Service) invitationEmail(ctx context.Context, inv *tenancy.Invitation, token tenancy.InvitationToken) (InvitationEmail, error) {
	store, err := s.tenants.TenantByID(ctx, inv.TenantID())
	if err != nil {
		return InvitationEmail{}, fmt.Errorf("read store: %w", err)
	}
	senders, err := s.users.UsersByID(ctx, []shared.UserID{inv.InvitedBy()})
	if err != nil {
		return InvitationEmail{}, fmt.Errorf("read sender: %w", err)
	}
	var sender string
	if len(senders) == 1 {
		sender = senders[0].Name
	}
	return InvitationEmail{
		To: inv.Email(), Store: store.Name().String(), InvitedBy: sender, Role: inv.Role(),
		Token: token, ExpiresAt: inv.ExpiresAt(),
	}, nil
}

// RevokeInvitation cancels a pending invitation, so its link stops working
// (members:manage, checked against the caller's current membership). It
// returns an error matching shared.ErrNotFound for an unknown, expired,
// accepted or revoked invitation, or one of another store.
func (s *Service) RevokeInvitation(ctx context.Context, id shared.InvitationID) error {
	by, err := authz.Require(ctx, identity.PermMembersManage)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.manager(ctx, by); err != nil {
			return err
		}
		inv, err := s.invitations.InvitationByID(ctx, by.TenantID(), id)
		if err != nil {
			return fmt.Errorf("read invitation %v: %w", id, err)
		}
		if err := inv.Revoke(now); err != nil {
			return err
		}
		revoked, err := s.invitations.MarkInvitationRevoked(ctx, by.TenantID(), id, now)
		if err != nil {
			return fmt.Errorf("revoke invitation %v: %w", id, err)
		}
		if !revoked {
			return tenancy.ErrInvitationNotPending
		}
		return s.events.Publish(ctx, tenancy.InvitationRevoked{InvitationID: id, By: by.UserID()})
	})
}
