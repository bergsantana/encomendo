package inproc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/internal/adapter/inproc"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	apptenancy "github.com/bergsantana/encomendo/backend/internal/app/tenancy"
	tenancyfake "github.com/bergsantana/encomendo/backend/internal/app/tenancy/fake"
	appworkflow "github.com/bergsantana/encomendo/backend/internal/app/workflow"
	workflowfake "github.com/bergsantana/encomendo/backend/internal/app/workflow/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/tenancy"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

// workflowRig wires tenancy and the order port to the real app/workflow
// service over its fakes, as cmd/api does over Postgres and Redis.
type workflowRig struct {
	source  *appworkflow.Service
	repo    *workflowfake.Repository
	tenancy *apptenancy.Service
	ids     *sharedfake.IDs
}

func newWorkflowRig(t *testing.T) *workflowRig {
	t.Helper()
	clock := sharedfake.NewClock(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC))
	tx := sharedfake.NewTxManager()
	r := &workflowRig{repo: workflowfake.NewRepository(), ids: sharedfake.NewIDs(clock)}
	for _, w := range testkit.Presets(t) {
		r.repo.Add(w, w.Key() == testkit.PresetEncomendaMontagem)
	}
	r.source = appworkflow.NewService(r.repo, tx, sharedfake.NewCache(clock))
	store := tenancyfake.NewStore()
	r.tenancy = apptenancy.NewService(apptenancy.Deps{
		Tenants: store, Members: store, Invitations: store, Users: tenancyfake.NewUsers(),
		Workflows: inproc.NewTenancyWorkflows(r.source), Mailer: tenancyfake.NewMailer(),
		Tx: tx, Clock: clock, IDs: r.ids, Secrets: sharedfake.NewSecrets(), Events: sharedfake.NewEvents(),
		Slugs: tenancy.NewSlugPolicy("painel"),
	})
	return r
}

func TestTenancyWorkflows_ReadThroughTheSource(t *testing.T) {
	t.Parallel()
	r := newWorkflowRig(t)
	port := inproc.NewTenancyWorkflows(r.source)
	pronta := testkit.Preset(t, testkit.PresetProntaEntrega)
	err := sharedfake.NewTxManager().WithinTx(context.Background(), func(ctx context.Context) error {
		def, err := port.DefaultWorkflow(ctx)
		if want := testkit.Preset(t, testkit.PresetEncomendaMontagem); err != nil || def.ID != want.ID() || def.Version != 1 || def.Name != want.Name() {
			t.Errorf("DefaultWorkflow = %+v, %v", def, err)
		}
		ref, err := port.Exists(ctx, pronta.ID(), 1)
		if err != nil || ref != (tenancy.WorkflowRef{ID: pronta.ID(), Version: 1, Name: "Pronta entrega"}) {
			t.Errorf("Exists = %+v, %v", ref, err)
		}
		if _, err := port.Exists(ctx, pronta.ID(), 2); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("Exists of an unpublished version: %v, want ErrNotFound", err)
		}
		statuses, err := port.Statuses(ctx, pronta.ID(), 1)
		if err != nil || len(statuses) != 5 || statuses[2].Key != "picking" || statuses[2].Phase != workflow.PhaseSourcing {
			t.Errorf("Statuses = %+v, %v; want the five statuses in order", statuses, err)
		}
		if _, err := port.Statuses(ctx, shared.WorkflowID{}, 1); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("Statuses of the zero id: %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The owner's choice travels tenancy → inproc → app/workflow, and orders
// then read the chosen workflow for the store: the whole of step 4.8.
func TestOrderWorkflows_CurrentFollowsTheOwnersChoice(t *testing.T) {
	t.Parallel()
	r := newWorkflowRig(t)
	owner := shared.UserID(r.ids.NewID())
	p, err := shared.NewBackofficePrincipal(owner, shared.TenantID{}, "")
	if err != nil {
		t.Fatal(err)
	}
	created, err := r.tenancy.CreateTenant(authz.WithPrincipal(context.Background(), p), apptenancy.CreateTenantInput{Name: "Alta RPM", Slug: "altarpm"})
	if err != nil {
		t.Fatal(err)
	}
	ownerP, err := shared.NewBackofficePrincipal(owner, created.ID, shared.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	staffCtx := authz.WithPrincipal(context.Background(), ownerP)
	storePage := appshared.WithScope(context.Background(), appshared.Scope{TenantID: created.ID})
	orders := inproc.NewOrderWorkflows(r.tenancy, r.source)

	current, err := orders.Current(storePage)
	if err != nil || current.Key() != testkit.PresetEncomendaMontagem {
		t.Fatalf("Current of a new store = %q, %v; want the default preset", current.Key(), err)
	}
	pronta := testkit.Preset(t, testkit.PresetProntaEntrega)
	saved, err := r.tenancy.ChooseStatusWorkflow(staffCtx, apptenancy.ChooseWorkflowInput{Workflow: pronta.ID(), Version: 1, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Workflow.Statuses) != 5 || saved.Workflow.Ref.Name != "Pronta entrega" {
		t.Errorf("saved workflow = %+v", saved.Workflow)
	}

	current, err = orders.Current(storePage)
	if err != nil || current.Key() != testkit.PresetProntaEntrega {
		t.Errorf("Current after the choice = %q, %v; want pronta_entrega", current.Key(), err)
	}
	// An order made before keeps its own version, read by id.
	old, err := orders.Get(storePage, testkit.Preset(t, testkit.PresetEncomendaMontagem).ID(), 1)
	if err != nil || old.Key() != testkit.PresetEncomendaMontagem {
		t.Errorf("Get of the old version = %q, %v", old.Key(), err)
	}
	if _, err := orders.Get(storePage, pronta.ID(), 9); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("Get of an unpublished version: %v, want ErrNotFound", err)
	}
}

type defaultsStub struct {
	view apptenancy.StoreDefaultsView
	err  error
}

func (d defaultsStub) StoreDefaults(context.Context) (apptenancy.StoreDefaultsView, error) {
	return d.view, d.err
}

func TestOrderWorkflows_CurrentErrors(t *testing.T) {
	t.Parallel()
	r := newWorkflowRig(t)
	boom := errors.New("settings are down")

	_, err := inproc.NewOrderWorkflows(defaultsStub{err: authz.ErrTenantRequired}, r.source).Current(context.Background())
	if !errors.Is(err, authz.ErrTenantRequired) {
		t.Errorf("no tenant in the scope: %v, want ErrTenantRequired", err)
	}
	gone := tenancy.WorkflowRef{ID: shared.WorkflowID(r.ids.NewID()), Version: 1, Name: "Sumiu"}
	_, err = inproc.NewOrderWorkflows(defaultsStub{view: apptenancy.StoreDefaultsView{Workflow: gone}}, r.source).Current(context.Background())
	if err == nil || errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a store pointing at an unpublished version: %v, want an error that matches no sentinel (500)", err)
	}
	r.repo.Fail("ByVersion", boom)
	known := testkit.Preset(t, testkit.PresetProntaEntrega)
	view := apptenancy.StoreDefaultsView{Workflow: tenancy.WorkflowRef{ID: known.ID(), Version: 1}}
	_, err = inproc.NewOrderWorkflows(defaultsStub{view: view}, r.source).Current(context.Background())
	if !errors.Is(err, boom) {
		t.Errorf("a failing read: %v, want it passed through", err)
	}
}
