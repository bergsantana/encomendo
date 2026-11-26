//go:build integration

package postgres_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/inproc"
	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	appidentity "github.com/bergsantana/encomendo/backend/internal/app/identity"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/test/contract/identityports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// The inproc adapters pass identity's contract for its two tenancy ports,
// on tenancy's real Lookup over the Postgres repositories (ADR 0011).

func tenancyHarness(t *testing.T) identityports.TenancyHarness {
	s := env.Seeder(t)
	lookup := apptenancy.NewLookup(apptenancy.Deps{
		Tenants: postgres.NewTenants(), Members: postgres.NewMembers(), Invitations: postgres.NewInvitations(),
		Events: postgres.NewOutbox(),
	})
	var mu sync.Mutex
	owners := map[shared.TenantID]shared.UserID{}
	return identityports.TenancyHarness{
		Memberships: inproc.NewIdentityMemberships(lookup),
		Invitations: inproc.NewIdentityInvitations(lookup),
		Tx:          postgres.NewTxManager(openApp(t)),
		NewEmail:    func(*testing.T) string { return "i-" + suffix() + "@teste.encomendo.com.br" },
		Now:         storeNow(),
		NewStore: func(t *testing.T, name string) appidentity.TenantSummary {
			owner := s.User()
			id := s.Tenant(owner)
			mu.Lock()
			owners[id] = owner
			mu.Unlock()
			var slug string
			if _, err := s.DB().ExecContext(t.Context(), `UPDATE tenants SET name = $1 WHERE id = $2`, name, id.String()); err != nil {
				t.Fatal(err)
			}
			if err := s.DB().QueryRowContext(t.Context(), `SELECT slug FROM tenants WHERE id = $1`, id.String()).Scan(&slug); err != nil {
				t.Fatal(err)
			}
			return appidentity.TenantSummary{ID: id, Name: name, Slug: slug}
		},
		NewUser: func(t *testing.T, email string) shared.UserID {
			id := pgtest.NewID()
			if _, err := s.DB().ExecContext(t.Context(),
				`INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, 'Pessoa de teste', 'x')`, id.String(), email); err != nil {
				t.Fatal(err)
			}
			return shared.UserID(id)
		},
		AddMember: func(_ *testing.T, store shared.TenantID, user shared.UserID, role shared.Role) {
			s.Member(store, user, role)
		},
		Invite: func(t *testing.T, store shared.TenantID, email string, role shared.Role, expires time.Time) shared.SecretHash {
			mu.Lock()
			owner := owners[store]
			mu.Unlock()
			if owner.IsZero() {
				t.Fatalf("store %v was not made by NewStore", store)
			}
			var hash shared.SecretHash
			copy(hash[:], pgtest.RandomHash())
			if _, err := s.DB().ExecContext(t.Context(),
				`INSERT INTO invitations (tenant_id, id, email, role, token_hash, invited_by, expires_at)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				store.String(), pgtest.NewID().String(), email, string(role), hash[:], owner.String(), expires); err != nil {
				t.Fatal(fmt.Errorf("seed invitation: %w", err))
			}
			return hash
		},
	}
}

func TestInprocMemberships_Contract(t *testing.T) {
	identityports.RunMembershipsContract(t, tenancyHarness)
}

func TestInprocInvitations_Contract(t *testing.T) {
	identityports.RunInvitationsContract(t, tenancyHarness)
}
