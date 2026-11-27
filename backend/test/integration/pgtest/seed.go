//go:build integration

package pgtest

import (
	"crypto/rand"
	"database/sql"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Seeder inserts rows as the superuser, past RLS, with fresh ids so tests
// can share one database and run in parallel.
type Seeder struct {
	t  testing.TB
	db *sql.DB
}

// Seeder returns a Seeder connected as the superuser.
func (e *Env) Seeder(t testing.TB) *Seeder {
	t.Helper()
	return &Seeder{t: t, db: e.Open(t, Superuser)}
}

// DB is the superuser pool, for checks behind RLS.
func (s *Seeder) DB() *sql.DB { return s.db }

func (s *Seeder) exec(query string, args ...any) {
	s.t.Helper()
	if _, err := s.db.ExecContext(s.t.Context(), query, args...); err != nil {
		s.t.Fatalf("seed: %v\n%s", err, query)
	}
}

// User inserts a staff user and returns its id.
func (s *Seeder) User() shared.UserID {
	s.t.Helper()
	id := NewID()
	s.exec(`INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, 'Pessoa de teste', 'x')`,
		id.String(), "u-"+id.String()+"@teste.encomendo.com.br")
	return shared.UserID(id)
}

// Tenant inserts a store with its settings (the default status workflow)
// and owner as its owner, and returns its id.
func (s *Seeder) Tenant(owner shared.UserID) shared.TenantID {
	s.t.Helper()
	id := NewID()
	s.exec(`INSERT INTO tenants (id, slug, name) VALUES ($1, $2, 'Loja de teste')`,
		id.String(), "loja-"+id.String()[24:])
	s.exec(`INSERT INTO tenant_settings (tenant_id, status_workflow_id, status_workflow_version)
		SELECT $1, id, version FROM status_workflows WHERE is_default`, id.String())
	tenant := shared.TenantID(id)
	s.Member(tenant, owner, shared.RoleOwner)
	return tenant
}

// Member adds user to tenant with role.
func (s *Seeder) Member(tenant shared.TenantID, user shared.UserID, role shared.Role) {
	s.t.Helper()
	s.exec(`INSERT INTO memberships (tenant_id, user_id, role) VALUES ($1, $2, $3)`,
		tenant.String(), user.String(), string(role))
}

// Invitation inserts a pending invitation to tenant, sent by invitedBy, and
// returns its id.
func (s *Seeder) Invitation(tenant shared.TenantID, invitedBy shared.UserID) shared.InvitationID {
	s.t.Helper()
	id := NewID()
	s.exec(`INSERT INTO invitations (tenant_id, id, email, role, token_hash, invited_by, expires_at)
		VALUES ($1, $2, $3, 'staff', $4, $5, $6)`,
		tenant.String(), id.String(), "c-"+id.String()+"@teste.encomendo.com.br",
		RandomHash(), invitedBy.String(), time.Now().Add(7*24*time.Hour))
	return shared.InvitationID(id)
}

// Event inserts an outbox event for tenant.
func (s *Seeder) Event(tenant shared.TenantID) {
	s.t.Helper()
	s.exec(`INSERT INTO outbox_events (tenant_id, name, payload) VALUES ($1, 'test.seeded', '{}')`, tenant.String())
}

// Supplier inserts a supplier of tenant and returns its id.
func (s *Seeder) Supplier(tenant shared.TenantID) shared.SupplierID {
	s.t.Helper()
	id := NewID()
	s.exec(`INSERT INTO suppliers (tenant_id, id, name, whatsapp_phone) VALUES ($1, $2, 'Fornecedor de teste', '5511987654321')`,
		tenant.String(), id.String())
	return shared.SupplierID(id)
}

// ClientAccount inserts a client account and returns its id.
func (s *Seeder) ClientAccount() shared.ClientAccountID {
	s.t.Helper()
	id := NewID()
	s.exec(`INSERT INTO client_accounts (id) VALUES ($1)`, id.String())
	return shared.ClientAccountID(id)
}

// RandomHash returns 32 random bytes, the size of every stored token hash.
func RandomHash() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24+)
	return b
}
