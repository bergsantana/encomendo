//go:build integration

package postgres_test

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
	"github.com/bergsantana/encomendo/backend/test/contract/identityports"
	"github.com/bergsantana/encomendo/backend/test/contract/tenancyports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// The repository contract suites, the same ones the fakes pass in
// test/contract (backend/CLAUDE.md, Testing). Emails and slugs carry a
// fresh suffix, so the suites share one database and run twice.

// storeNow is a recent time at Postgres's precision, which the suites
// compare exactly.
func storeNow() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

var uniq atomic.Int64

func suffix() string { return fmt.Sprintf("%s-%d", pgtest.NewID().String(), uniq.Add(1)) }

func hex(id shared.ID) string { return strings.ReplaceAll(id.String(), "-", "") }

func usersHarness(t *testing.T) identityports.UsersHarness {
	return identityports.UsersHarness{
		Users:    postgres.NewUsers(),
		Tx:       postgres.NewTxManager(openApp(t)),
		NewEmail: func(*testing.T) string { return "u-" + suffix() + "@teste.encomendo.com.br" },
		NewID:    pgtest.NewID,
	}
}

func refreshHarness(t *testing.T) identityports.RefreshHarness {
	s := env.Seeder(t)
	return identityports.RefreshHarness{
		UsersHarness: usersHarness(t),
		Refresh:      postgres.NewRefreshTokens(),
		ResetCodes:   postgres.NewResetCodes(),
		Now:          storeNow(),
		Tenant:       s.Tenant(s.User()),
	}
}

func TestUsers_Contract(t *testing.T) { identityports.RunUsersContract(t, usersHarness) }

func TestRefreshTokens_Contract(t *testing.T) {
	identityports.RunRefreshTokensContract(t, refreshHarness)
}

func TestResetCodes_Contract(t *testing.T) { identityports.RunResetCodesContract(t, refreshHarness) }

func TestTenancyRepositories_Contract(t *testing.T) {
	db := openApp(t)
	tx := postgres.NewTxManager(db)
	s := env.Seeder(t)
	var id string
	wf := tenancy.WorkflowRef{}
	if err := s.DB().QueryRowContext(t.Context(),
		`SELECT id, version, name FROM status_workflows WHERE is_default`).Scan(&id, &wf.Version, &wf.Name); err != nil {
		t.Fatal(err)
	}
	parsed, err := shared.ParseWorkflowID(id)
	if err != nil {
		t.Fatal(err)
	}
	wf.ID = parsed
	tenancyports.RunRepositoryContract(t, func(*testing.T) tenancyports.Harness {
		return tenancyports.Harness{
			Tenants: postgres.NewTenants(), Members: postgres.NewMembers(), Invitations: postgres.NewInvitations(),
			Tx: tx, NewID: pgtest.NewID,
			NewUser:  func(*testing.T) shared.UserID { return s.User() },
			NewSlug:  func(*testing.T) string { return fmt.Sprintf("loja-%s%d", hex(pgtest.NewID())[:20], uniq.Add(1)) },
			Workflow: wf, Now: storeNow(),
		}
	})
}
