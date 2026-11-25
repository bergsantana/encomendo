package identityports

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	"github.com/bergsantana/encomendo/backend/internal/app/identity/fake"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

var start = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

func fakeUsers(*testing.T) UsersHarness {
	ids := sharedfake.NewIDs(sharedfake.NewClock(start))
	var n atomic.Int64
	return UsersHarness{
		Users:    fake.NewStore(),
		Tx:       sharedfake.NewTxManager(),
		NewEmail: func(*testing.T) string { return fmt.Sprintf("pessoa%d@ex.com", n.Add(1)) },
		NewID:    ids.NewID,
	}
}

func fakeRefresh(t *testing.T) RefreshHarness {
	h := fakeUsers(t)
	store := fake.NewStore()
	h.Users = store
	return RefreshHarness{UsersHarness: h, Refresh: store, ResetCodes: store, Now: start, Tenant: shared.TenantID(h.NewID())}
}

func TestFakeUsers_Contract(t *testing.T) { RunUsersContract(t, fakeUsers) }

func TestFakeRefreshTokens_Contract(t *testing.T) { RunRefreshTokensContract(t, fakeRefresh) }

func TestFakeResetCodes_Contract(t *testing.T) { RunResetCodesContract(t, fakeRefresh) }

func TestFakeHasher_Contract(t *testing.T) {
	RunPasswordHasherContract(t, func(*testing.T) appidentity.PasswordHasher { return fake.NewHasher() })
}

func fakeTenancy(*testing.T) TenancyHarness {
	ids := sharedfake.NewIDs(sharedfake.NewClock(start))
	members := fake.NewMemberships()
	invites := fake.NewInvitations(members)
	stores := map[shared.TenantID]appidentity.TenantSummary{}
	var n atomic.Int64
	return TenancyHarness{
		NewEmail:    func(*testing.T) string { return fmt.Sprintf("convidado%d@ex.com", n.Add(1)) },
		Memberships: members, Invitations: invites, Tx: sharedfake.NewTxManager(), Now: start,
		NewStore: func(_ *testing.T, name string) appidentity.TenantSummary {
			s := appidentity.TenantSummary{ID: shared.TenantID(ids.NewID()), Name: name, Slug: "loja-" + ids.NewID().String()[30:]}
			stores[s.ID] = s
			return s
		},
		NewUser: func(*testing.T, string) shared.UserID { return shared.UserID(ids.NewID()) },
		AddMember: func(_ *testing.T, store shared.TenantID, user shared.UserID, role shared.Role) {
			members.Add(user, appidentity.Membership{Tenant: stores[store], Role: role})
			invites.AddMember(store, user)
		},
		Invite: func(t *testing.T, store shared.TenantID, email string, role shared.Role, expires time.Time) shared.SecretHash {
			addr, err := identity.ParseEmail(email)
			if err != nil {
				t.Fatal(err)
			}
			var secret shared.Secret
			id := ids.NewID()
			copy(secret[:], id[:])
			invites.Add(secret.Hash(), appidentity.PendingInvitation{
				ID: shared.InvitationID(ids.NewID()), Tenant: stores[store], Email: addr, Role: role, ExpiresAt: expires,
			})
			return secret.Hash()
		},
	}
}

func TestFakeMemberships_Contract(t *testing.T) { RunMembershipsContract(t, fakeTenancy) }

func TestFakeInvitations_Contract(t *testing.T) { RunInvitationsContract(t, fakeTenancy) }
