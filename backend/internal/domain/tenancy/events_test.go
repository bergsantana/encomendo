package tenancy

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// eventNamer is the method the outbox needs (appshared.Event); the domain
// cannot import the app layer, so the test names it locally.
type eventNamer interface{ EventName() string }

func TestEvents_HaveStableNamesAndNoPersonalData(t *testing.T) {
	t.Parallel()

	events := map[string]eventNamer{
		"tenancy.tenant_created":      TenantCreated{Owner: ownerID, Slug: "altarpm"},
		"tenancy.member_invited":      MemberInvited{InvitationID: invitationID, Role: shared.RoleStaff, InvitedBy: ownerID},
		"tenancy.invitation_revoked":  InvitationRevoked{InvitationID: invitationID, By: ownerID},
		"tenancy.member_joined":       MemberJoined{UserID: staffID, Role: shared.RoleStaff, InvitationID: invitationID},
		"tenancy.member_role_changed": MemberRoleChanged{UserID: staffID, From: shared.RoleStaff, To: shared.RoleAdmin, By: ownerID},
		"tenancy.member_removed":      MemberRemoved{UserID: staffID, By: ownerID},
		"tenancy.settings_updated":    SettingsUpdated{Version: 2, By: ownerID},
	}
	for name, e := range events {
		if e.EventName() != name {
			t.Errorf("%T.EventName() = %q, want %q (stored events depend on it)", e, e.EventName(), name)
		}
		payload, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// The outbox keeps payloads; ids say who, emails and names stay in
		// their tables.
		if strings.Contains(string(payload), "@") {
			t.Errorf("%s carries an email: %s", name, payload)
		}
	}
}

func TestErrors_MatchTheirSentinels(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ err, sentinel error }{
		{ErrSlugTaken, shared.ErrConflict},            // 409 slug_taken
		{ErrAlreadyMember, shared.ErrConflict},        // 409 already_member
		{ErrOwnerFixed, shared.ErrForbidden},          // 403 forbidden
		{ErrInvitationNotPending, shared.ErrNotFound}, // 404 not_found
	} {
		if !errors.Is(c.err, c.sentinel) {
			t.Errorf("%v does not match %v", c.err, c.sentinel)
		}
	}
	for _, bug := range []error{ErrInvalidTenant, ErrInvalidMembership, ErrInvalidInvitation, ErrInvalidSettings} {
		for _, s := range []error{shared.ErrNotFound, shared.ErrConflict, shared.ErrForbidden, shared.ErrInvalid, shared.ErrUnauthenticated} {
			if errors.Is(bug, s) {
				t.Errorf("%v matches %v; a bug must stay a 500", bug, s)
			}
		}
	}
}
