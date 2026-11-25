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

// errNoLongerManages means the caller's token still grants members:manage
// but their membership, read inside the transaction, no longer does.
var errNoLongerManages = fmt.Errorf("tenancy: the caller no longer manages the team: %w", shared.ErrForbidden)

// manager re-reads the caller's membership inside the transaction, FOR
// SHARE. Team management never trusts the role in the token, which can be
// 15 minutes old, so a demoted or removed admin loses it at once; the lock
// makes a concurrent demotion wait for this call to commit (ADR 0011).
func (s *Service) manager(ctx context.Context, by shared.Principal) error {
	m, err := s.members.LockMembershipOf(ctx, by.TenantID(), by.UserID())
	if errors.Is(err, shared.ErrNotFound) {
		return errNoLongerManages
	}
	if err != nil {
		return fmt.Errorf("read the caller's membership: %w", err)
	}
	if !m.ManagesTeam() {
		return errNoLongerManages
	}
	return nil
}

// Members returns one page of the store's team, oldest member first, with
// names and emails (members:manage, checked against the caller's current
// membership).
func (s *Service) Members(ctx context.Context, page shared.PageRequest) (shared.Page[MemberView], error) {
	by, err := authz.Require(ctx, identity.PermMembersManage)
	if err != nil {
		return shared.Page[MemberView]{}, err
	}
	var out shared.Page[MemberView]
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.manager(ctx, by); err != nil {
			return err
		}
		ms, err := s.members.Memberships(ctx, by.TenantID(), page)
		if err != nil {
			return fmt.Errorf("list members: %w", err)
		}
		views, err := s.memberViews(ctx, ms.Items)
		if err != nil {
			return err
		}
		out = shared.Page[MemberView]{Items: views, NextCursor: ms.NextCursor}
		return nil
	})
	return out, err
}

// memberViews joins memberships with their accounts. A member without an
// account breaks a foreign key, so it is a bug, not a 404.
func (s *Service) memberViews(ctx context.Context, ms []*tenancy.Membership) ([]MemberView, error) {
	ids := make([]shared.UserID, 0, len(ms))
	for _, m := range ms {
		ids = append(ids, m.UserID())
	}
	users, err := s.users.UsersByID(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("read members' accounts: %w", err)
	}
	byID := make(map[shared.UserID]UserSummary, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	views := make([]MemberView, 0, len(ms))
	for _, m := range ms {
		u, ok := byID[m.UserID()]
		if !ok {
			return nil, fmt.Errorf("tenancy: member %v of %v has no account", m.UserID(), m.TenantID())
		}
		views = append(views, MemberView{UserID: m.UserID(), Name: u.Name, Email: u.Email, Role: m.Role(), JoinedAt: m.JoinedAt()})
	}
	return views, nil
}

// ChangeRole gives a member another role (members:manage, checked against
// the caller's current membership). Demoting someone below members:manage
// also revokes the invitations they sent (ADR 0011). It returns a
// ValidationError for owner or an unknown role, tenancy.ErrOwnerFixed for
// the owner, and shared.ErrNotFound for someone not in the store.
func (s *Service) ChangeRole(ctx context.Context, in ChangeRoleInput) (MemberView, error) {
	by, err := authz.Require(ctx, identity.PermMembersManage)
	if err != nil {
		return MemberView{}, err
	}
	role, err := tenancy.ParseInvitableRole(in.Role)
	if err != nil {
		return MemberView{}, err
	}
	now := s.clock.Now()
	var view MemberView
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.manager(ctx, by); err != nil {
			return err
		}
		m, err := s.members.MembershipOf(ctx, by.TenantID(), in.UserID)
		if err != nil {
			return fmt.Errorf("read member %v: %w", in.UserID, err)
		}
		from, managed := m.Role(), m.ManagesTeam()
		if err := m.ChangeRole(role); err != nil {
			return err
		}
		if from != role {
			if err := s.saveRole(ctx, m, from, managed, by, now); err != nil {
				return err
			}
		}
		views, err := s.memberViews(ctx, []*tenancy.Membership{m})
		if err != nil {
			return err
		}
		view = views[0]
		return nil
	})
	return view, err
}

func (s *Service) saveRole(ctx context.Context, m *tenancy.Membership, from shared.Role, managed bool, by shared.Principal, now time.Time) error {
	if err := s.members.UpdateRole(ctx, m); err != nil {
		return fmt.Errorf("save role: %w", err)
	}
	if managed && !m.ManagesTeam() {
		if err := s.revokeSentBy(ctx, m.TenantID(), m.UserID(), by.UserID(), now); err != nil {
			return err
		}
	}
	return s.events.Publish(ctx, tenancy.MemberRoleChanged{UserID: m.UserID(), From: from, To: m.Role(), By: by.UserID()})
}

// revokeSentBy closes every open invitation sender sent, so nobody keeps a
// way back in after losing access (ADR 0011).
func (s *Service) revokeSentBy(ctx context.Context, tenant shared.TenantID, sender, by shared.UserID, now time.Time) error {
	ids, err := s.invitations.RevokeInvitationsBy(ctx, tenant, sender, now)
	if err != nil {
		return fmt.Errorf("revoke invitations of %v: %w", sender, err)
	}
	for _, id := range ids {
		if err := s.events.Publish(ctx, tenancy.InvitationRevoked{InvitationID: id, By: by}); err != nil {
			return err
		}
	}
	return nil
}

// RemoveMember takes someone out of the team (members:manage, checked
// against the caller's current membership). Their account stays, their
// sessions in the store end at the next refresh, and the invitations they
// sent are revoked (ADR 0011). It returns tenancy.ErrOwnerFixed for the
// owner and shared.ErrNotFound for someone not in the store.
func (s *Service) RemoveMember(ctx context.Context, user shared.UserID) error {
	by, err := authz.Require(ctx, identity.PermMembersManage)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.manager(ctx, by); err != nil {
			return err
		}
		m, err := s.members.MembershipOf(ctx, by.TenantID(), user)
		if err != nil {
			return fmt.Errorf("read member %v: %w", user, err)
		}
		if err := m.CheckRemovable(); err != nil {
			return err
		}
		if err := s.members.DeleteMembership(ctx, by.TenantID(), user); err != nil {
			return fmt.Errorf("remove member %v: %w", user, err)
		}
		if err := s.revokeSentBy(ctx, by.TenantID(), user, by.UserID(), now); err != nil {
			return err
		}
		return s.events.Publish(ctx, tenancy.MemberRemoved{UserID: user, By: by.UserID()})
	})
}
