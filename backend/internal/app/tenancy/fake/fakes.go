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
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
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

type presetKey struct {
	id      shared.WorkflowID
	version int
}

// Workflows is a WorkflowPresets over a few presets, one of them the
// default. Like the real port it refuses calls outside a fake transaction.
type Workflows struct {
	mu      sync.Mutex
	def     tenancy.WorkflowRef
	presets map[presetKey]preset
	fail    error
}

type preset struct {
	ref      tenancy.WorkflowRef
	statuses []apptenancy.WorkflowStatus
}

// NewWorkflows returns presets whose default is ref, with a short list of
// statuses.
func NewWorkflows(ref tenancy.WorkflowRef) *Workflows {
	w := &Workflows{def: ref, presets: map[presetKey]preset{}}
	w.Add(ref, []apptenancy.WorkflowStatus{
		{Key: "draft", Label: "Rascunho", ClientLabel: "Pedido recebido", Phase: workflow.PhaseDraft},
		{Key: "delivered", Label: "Entregue", ClientLabel: "Entregue", Phase: workflow.PhaseHandedOver},
	})
	return w
}

// Add publishes a preset version with its statuses.
func (w *Workflows) Add(ref tenancy.WorkflowRef, statuses []apptenancy.WorkflowStatus) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.presets[presetKey{ref.ID, ref.Version}] = preset{ref: ref, statuses: statuses}
}

// Remove unpublishes a preset version, to test a store that points at one
// that is gone.
func (w *Workflows) Remove(ref tenancy.WorkflowRef) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.presets, presetKey{ref.ID, ref.Version})
}

// Fail makes every later call return err; nil heals it.
func (w *Workflows) Fail(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.fail = err
}

func (w *Workflows) enter(ctx context.Context) error {
	if !sharedfake.InTx(ctx) {
		return appshared.ErrNoTransaction
	}
	return w.fail
}

// DefaultWorkflow implements WorkflowPresets.
func (w *Workflows) DefaultWorkflow(ctx context.Context) (tenancy.WorkflowRef, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.enter(ctx); err != nil {
		return tenancy.WorkflowRef{}, err
	}
	return w.def, nil
}

// Exists implements WorkflowPresets.
func (w *Workflows) Exists(ctx context.Context, id shared.WorkflowID, version int) (tenancy.WorkflowRef, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.enter(ctx); err != nil {
		return tenancy.WorkflowRef{}, err
	}
	p, ok := w.presets[presetKey{id, version}]
	if !ok {
		return tenancy.WorkflowRef{}, shared.ErrNotFound
	}
	return p.ref, nil
}

// Statuses implements WorkflowPresets.
func (w *Workflows) Statuses(ctx context.Context, id shared.WorkflowID, version int) ([]apptenancy.WorkflowStatus, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.enter(ctx); err != nil {
		return nil, err
	}
	p, ok := w.presets[presetKey{id, version}]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return slices.Clone(p.statuses), nil
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
