package fake

import (
	"context"
	"slices"
	"sync"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/identity"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
)

// Users is a UserDirectory over an in-memory list.
type Users struct {
	mu   sync.Mutex
	byID map[shared.UserID]apptenancy.UserSummary
	fail error
}

// NewUsers returns a directory with no accounts.
func NewUsers() *Users { return &Users{byID: map[shared.UserID]apptenancy.UserSummary{}} }

// Add stores an account.
func (u *Users) Add(s apptenancy.UserSummary) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.byID[s.ID] = s
}

// Fail makes every later call return err; nil heals it.
func (u *Users) Fail(err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.fail = err
}

func (u *Users) enter(ctx context.Context) error {
	if !sharedfake.InTx(ctx) {
		return appshared.ErrNoTransaction
	}
	return u.fail
}

// UserByEmail implements UserDirectory.
func (u *Users) UserByEmail(ctx context.Context, email identity.Email) (apptenancy.UserSummary, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.enter(ctx); err != nil {
		return apptenancy.UserSummary{}, err
	}
	for _, s := range u.byID {
		if e, err := identity.ParseEmail(s.Email); err == nil && e.Equal(email) {
			return s, nil
		}
	}
	return apptenancy.UserSummary{}, shared.ErrNotFound
}

// UsersByID implements UserDirectory.
func (u *Users) UsersByID(ctx context.Context, ids []shared.UserID) ([]apptenancy.UserSummary, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := u.enter(ctx); err != nil {
		return nil, err
	}
	var out []apptenancy.UserSummary
	for _, id := range ids {
		if s, ok := u.byID[id]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

// Workflows is a WorkflowPresets with one default preset.
type Workflows struct {
	mu   sync.Mutex
	ref  tenancy.WorkflowRef
	fail error
}

// NewWorkflows returns presets whose default is ref.
func NewWorkflows(ref tenancy.WorkflowRef) *Workflows { return &Workflows{ref: ref} }

// Fail makes every later call return err; nil heals it.
func (w *Workflows) Fail(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fail = err
}

// DefaultWorkflow implements WorkflowPresets.
func (w *Workflows) DefaultWorkflow(ctx context.Context) (tenancy.WorkflowRef, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !sharedfake.InTx(ctx) {
		return tenancy.WorkflowRef{}, appshared.ErrNoTransaction
	}
	if w.fail != nil {
		return tenancy.WorkflowRef{}, w.fail
	}
	return w.ref, nil
}

// Mailer is an InvitationMailer that keeps what it sent.
type Mailer struct {
	mu   sync.Mutex
	sent []apptenancy.InvitationEmail
	fail error
}

// NewMailer returns a mailer with nothing sent.
func NewMailer() *Mailer { return &Mailer{} }

// Fail makes every later send return err; nil heals it.
func (m *Mailer) Fail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fail = err
}

// SendInvitation implements InvitationMailer.
func (m *Mailer) SendInvitation(_ context.Context, e apptenancy.InvitationEmail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	m.sent = append(m.sent, e)
	return nil
}

// Sent returns every email sent, oldest first.
func (m *Mailer) Sent() []apptenancy.InvitationEmail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.sent)
}
