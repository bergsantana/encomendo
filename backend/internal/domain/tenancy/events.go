package tenancy

import "github.com/bergsantana/encomendo/backend/internal/domain/shared"

// Events of the tenancy context. The outbox stores each under the store of
// the transaction that published it, so none repeats the tenant id. They
// carry ids, never emails or names: those stay in their tables. The names
// are stable; consumers select on them.

// TenantCreated is published when a store is created, with its owner.
type TenantCreated struct {
	Owner shared.UserID `json:"owner"`
	Slug  string        `json:"slug"`
}

// EventName implements appshared.Event.
func (TenantCreated) EventName() string { return "tenancy.tenant_created" }

// MemberInvited is published when an invitation is sent, or sent again in
// place of a pending one.
type MemberInvited struct {
	InvitationID shared.InvitationID `json:"invitationId"`
	Role         shared.Role         `json:"role"`
	InvitedBy    shared.UserID       `json:"invitedBy"`
}

// EventName implements appshared.Event.
func (MemberInvited) EventName() string { return "tenancy.member_invited" }

// InvitationRevoked is published when a pending invitation is cancelled, by
// a person or because its sender lost the right to manage the team.
type InvitationRevoked struct {
	InvitationID shared.InvitationID `json:"invitationId"`
	By           shared.UserID       `json:"by"`
}

// EventName implements appshared.Event.
func (InvitationRevoked) EventName() string { return "tenancy.invitation_revoked" }

// MemberJoined is published when someone accepts an invitation.
type MemberJoined struct {
	UserID       shared.UserID       `json:"userId"`
	Role         shared.Role         `json:"role"`
	InvitationID shared.InvitationID `json:"invitationId"`
}

// EventName implements appshared.Event.
func (MemberJoined) EventName() string { return "tenancy.member_joined" }

// MemberRoleChanged is published when a member's role changes.
type MemberRoleChanged struct {
	UserID shared.UserID `json:"userId"`
	From   shared.Role   `json:"from"`
	To     shared.Role   `json:"to"`
	By     shared.UserID `json:"by"`
}

// EventName implements appshared.Event.
func (MemberRoleChanged) EventName() string { return "tenancy.member_role_changed" }

// MemberRemoved is published when someone leaves the team. Their account
// stays.
type MemberRemoved struct {
	UserID shared.UserID `json:"userId"`
	By     shared.UserID `json:"by"`
}

// EventName implements appshared.Event.
func (MemberRemoved) EventName() string { return "tenancy.member_removed" }

// SettingsUpdated is published when the owner saves the settings; Version
// is the new one.
type SettingsUpdated struct {
	Version int           `json:"version"`
	By      shared.UserID `json:"by"`
}

// EventName implements appshared.Event.
func (SettingsUpdated) EventName() string { return "tenancy.settings_updated" }
