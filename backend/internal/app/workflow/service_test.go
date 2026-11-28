package workflow_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/internal/app/shared/authz"
	sharedfake "github.com/bergsantana/encomendo/backend/internal/app/shared/fake"
	appworkflow "github.com/bergsantana/encomendo/backend/internal/app/workflow"
	"github.com/bergsantana/encomendo/backend/internal/app/workflow/fake"
	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
	"github.com/bergsantana/encomendo/backend/internal/domain/workflow"
	"github.com/bergsantana/encomendo/backend/internal/testkit"
)

var errBoom = errors.New("postgres is down")

type harness struct {
	svc    *appworkflow.Service
	repo   *fake.Repository
	cache  *sharedfake.Cache
	clock  *sharedfake.Clock
	tx     *sharedfake.TxManager
	tenant shared.TenantID
	user   shared.UserID
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	clock := sharedfake.NewClock(time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC))
	ids := sharedfake.NewIDs(clock)
	h := &harness{
		repo: fake.NewRepository(), cache: sharedfake.NewCache(clock), clock: clock, tx: sharedfake.NewTxManager(),
		tenant: shared.TenantID(ids.NewID()), user: shared.UserID(ids.NewID()),
	}
	for _, w := range testkit.Presets(t) {
		h.repo.Add(w, w.Key() == testkit.PresetEncomendaMontagem)
	}
	h.svc = appworkflow.NewService(h.repo, h.tx, h.cache)
	return h
}

func (h *harness) as(t *testing.T, role shared.Role) context.Context {
	t.Helper()
	p, err := shared.NewBackofficePrincipal(h.user, h.tenant, role)
	if err != nil {
		t.Fatal(err)
	}
	return authz.WithPrincipal(context.Background(), p)
}

func TestPresets_EveryRoleMayRead(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, role := range []shared.Role{shared.RoleOwner, shared.RoleAdmin, shared.RoleStaff, shared.RoleViewer} {
		got, err := h.svc.Presets(h.as(t, role))
		if err != nil || len(got) != 4 {
			t.Errorf("%s: %d presets, %v", role, len(got), err)
		}
	}
}

func TestPresets_NeedsAStaffCaller(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	if _, err := h.svc.Presets(context.Background()); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("no caller: %v, want ErrUnauthenticated", err)
	}
	noStore, err := shared.NewBackofficePrincipal(h.user, shared.TenantID{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Presets(authz.WithPrincipal(context.Background(), noStore)); !errors.Is(err, authz.ErrTenantRequired) {
		t.Errorf("no store selected: %v, want ErrTenantRequired", err)
	}
	if h.repo.Calls("Published") != 0 {
		t.Error("the repository was read before authorization")
	}
}

func TestPresets_ListsTheLatestVersionOfEachPresetByKey(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	first := testkit.Preset(t, testkit.PresetProntaEntrega)
	v2, err := workflow.New(first.ID(), 2, first.Key(), "Pronta entrega 2", first.Statuses())
	if err != nil {
		t.Fatal(err)
	}
	h.repo.Add(v2, false)

	got, err := h.svc.Presets(h.as(t, shared.RoleViewer))
	if err != nil {
		t.Fatal(err)
	}

	var keys []string
	for _, v := range got {
		keys = append(keys, v.Key)
	}
	want := []string{"encomenda_montagem", "orcamento_primeiro", "producao_propria", "pronta_entrega"}
	if !slices.Equal(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	last := got[3]
	if last.Version != 2 || last.Name != "Pronta entrega 2" || len(last.Statuses) != 5 {
		t.Errorf("pronta_entrega = v%d %q with %d statuses, want its version 2", last.Version, last.Name, len(last.Statuses))
	}
	if s := got[0].Statuses[2]; s.Key != "purchasing" || s.Phase != workflow.PhaseSourcing || s.ClientLabel != "Pedido confirmado" {
		t.Errorf("third status of encomenda_montagem = %+v", s)
	}
}

func TestPresets_CacheHitSkipsPostgres(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.as(t, shared.RoleOwner)
	first, err := h.svc.Presets(ctx)
	if err != nil {
		t.Fatal(err)
	}

	second, err := h.svc.Presets(ctx)

	if err != nil || !slices.EqualFunc(first, second, equalViews) {
		t.Errorf("second call = %v, %v; want the first answer", second, err)
	}
	if n := h.repo.Calls("Published"); n != 1 {
		t.Errorf("Published called %d times, want 1 (the second call is a cache hit)", n)
	}
}

func TestPresets_CacheExpiresAfterTenMinutes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.as(t, shared.RoleOwner)
	if _, err := h.svc.Presets(ctx); err != nil {
		t.Fatal(err)
	}

	h.clock.Advance(9 * time.Minute)
	if _, err := h.svc.Presets(ctx); err != nil || h.repo.Calls("Published") != 1 {
		t.Fatalf("after 9 minutes: %v, %d reads, want a hit", err, h.repo.Calls("Published"))
	}
	h.clock.Advance(2 * time.Minute)
	if _, err := h.svc.Presets(ctx); err != nil || h.repo.Calls("Published") != 2 {
		t.Errorf("after 11 minutes: %v, %d reads, want a second read", err, h.repo.Calls("Published"))
	}
}

func TestPresets_RedisDownFallsBackToPostgres(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.cache.SetDown(true)

	got, err := h.svc.Presets(h.as(t, shared.RoleOwner))

	if err != nil || len(got) != 4 {
		t.Errorf("with the cache down: %d presets, %v; want all four from Postgres", len(got), err)
	}
}

func TestPresets_PostgresFailureIsReturned(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.repo.Fail("Published", errBoom)

	if _, err := h.svc.Presets(h.as(t, shared.RoleOwner)); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the repository's", err)
	}
}

func TestGet_ReturnsADomainWorkflowAndCachesIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	want := testkit.Preset(t, testkit.PresetProducaoPropria)

	got, err := h.svc.Get(context.Background(), want.ID(), 1)
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.svc.Get(context.Background(), want.ID(), 1)

	if err != nil || !slices.Equal(got.Statuses(), want.Statuses()) || !slices.Equal(again.Statuses(), want.Statuses()) {
		t.Errorf("Get = %+v, %+v, %v; want the seeded statuses", got.Statuses(), again.Statuses(), err)
	}
	if again.ID() != want.ID() || again.Version() != 1 || again.Key() != want.Key() || again.Name() != want.Name() {
		t.Errorf("cached workflow = %s v%d %s %q", again.ID(), again.Version(), again.Key(), again.Name())
	}
	if n := h.repo.Calls("ByVersion"); n != 1 {
		t.Errorf("ByVersion called %d times, want 1", n)
	}
	key, err := appshared.GlobalKey("workflow", want.ID().String(), "1")
	if err != nil {
		t.Fatal(err)
	}
	var stored appworkflow.View
	if hit, err := h.cache.Get(context.Background(), key, &stored); err != nil || !hit {
		t.Errorf("g:workflow:{id}:1 not cached: %v, %v", hit, err)
	}
}

func TestGet_UnknownVersionIsNotFoundAndNotCached(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	id := testkit.Preset(t, testkit.PresetProntaEntrega).ID()

	if _, err := h.svc.Get(context.Background(), id, 7); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("unknown version: %v, want ErrNotFound", err)
	}
	if _, err := h.svc.Get(context.Background(), shared.WorkflowID{}, 1); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("zero id: %v, want ErrNotFound", err)
	}
	// Publishing the version later makes it found at once.
	first := testkit.Preset(t, testkit.PresetProntaEntrega)
	v7, err := workflow.New(id, 7, first.Key(), first.Name(), first.Statuses())
	if err != nil {
		t.Fatal(err)
	}
	h.repo.Add(v7, false)
	if got, err := h.svc.Get(context.Background(), id, 7); err != nil || got.Version() != 7 {
		t.Errorf("after publishing: %v, %v", got.Version(), err)
	}
}

func TestGet_CacheDownFallsBackAndACorruptEntryIsAMiss(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	w := testkit.Preset(t, testkit.PresetEncomendaMontagem)
	h.cache.SetDown(true)
	if _, err := h.svc.Get(context.Background(), w.ID(), 1); err != nil {
		t.Fatalf("cache down: %v", err)
	}
	h.cache.SetDown(false)

	key, err := appshared.GlobalKey("workflow", w.ID().String(), "1")
	if err != nil {
		t.Fatal(err)
	}
	// An entry that decodes but no longer builds a workflow.
	if err := h.cache.Set(context.Background(), key, appworkflow.View{ID: w.ID(), Version: 1}, time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := h.svc.Get(context.Background(), w.ID(), 1)
	if err != nil || len(got.Statuses()) != 7 {
		t.Errorf("corrupt entry: %d statuses, %v; want it read from Postgres", len(got.Statuses()), err)
	}
}

func TestGet_PostgresFailureIsReturned(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.repo.Fail("ByVersion", errBoom)

	if _, err := h.svc.Get(context.Background(), testkit.Preset(t, testkit.PresetProntaEntrega).ID(), 1); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want the repository's", err)
	}
}

func TestDefault_IsTheSeededDefaultAndNeedsNoCaller(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	got, err := h.svc.Default(context.Background())

	if err != nil || got.Key() != testkit.PresetEncomendaMontagem {
		t.Errorf("Default = %q, %v; want encomenda_montagem", got.Key(), err)
	}
	h.repo.Fail("Default", errBoom)
	if _, err := h.svc.Default(context.Background()); !errors.Is(err, errBoom) {
		t.Errorf("failing repository: %v", err)
	}
}

func TestReads_JoinTheCallersTransaction(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	w := testkit.Preset(t, testkit.PresetEncomendaMontagem)

	err := h.tx.WithinTx(h.as(t, shared.RoleOwner), func(ctx context.Context) error {
		_, err := h.svc.Get(ctx, w.ID(), 1)
		return err
	})

	if err != nil {
		t.Errorf("a read inside a transaction: %v", err)
	}
	if got := len(h.tx.Scopes()); got != 2 {
		t.Errorf("%d transactions recorded, want the outer and the joined read", got)
	}
}

func equalViews(a, b appworkflow.View) bool {
	return a.ID == b.ID && a.Version == b.Version && a.Key == b.Key && a.Name == b.Name && slices.Equal(a.Statuses, b.Statuses)
}
