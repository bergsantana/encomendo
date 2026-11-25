//go:build integration

package postgres_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// phaseOrder is the order of workflow.Phase (ADR 0004). The workflow domain
// lands in milestone 4 and adds a test that loads every preset through
// validate.go (ADR 0006); until then this test holds the seed to the rules.
var phaseOrder = []string{"draft", "awaiting_deposit", "confirmed", "sourcing", "assembling", "ready", "handed_over"}

type seededStatus struct{ key, label, clientLabel, phase string }

// TestWorkflowSeed_FollowsThePhaseRules checks the launch presets of
// docs/architecture.md 4.2: each one has draft, awaiting_deposit, ready and
// handed_over, its phases never go backwards, every status has both labels,
// and exactly one preset is the default for new stores.
func TestWorkflowSeed_FollowsThePhaseRules(t *testing.T) {
	t.Parallel()
	db := env.Open(t, pgtest.App)
	ctx := t.Context()

	rows, err := db.QueryContext(ctx, `SELECT w.key, s.key, s.label, s.client_label, s.phase
		FROM status_workflows w JOIN workflow_statuses s ON s.workflow_id = w.id AND s.workflow_version = w.version
		ORDER BY w.key, s.position`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	presets := map[string][]seededStatus{}
	for rows.Next() {
		var preset string
		var s seededStatus
		if err := rows.Scan(&preset, &s.key, &s.label, &s.clientLabel, &s.phase); err != nil {
			t.Fatal(err)
		}
		presets[preset] = append(presets[preset], s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	wantPresets := []string{"encomenda_montagem", "orcamento_primeiro", "producao_propria", "pronta_entrega"}
	var got []string
	for k := range presets {
		got = append(got, k)
	}
	slices.Sort(got)
	if !slices.Equal(got, wantPresets) {
		t.Fatalf("presets = %v, want %v", got, wantPresets)
	}
	for preset, statuses := range presets {
		checkPreset(t, preset, statuses)
	}

	var defaults []string
	defaultRows, err := db.QueryContext(ctx, `SELECT key FROM status_workflows WHERE is_default`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = defaultRows.Close() }()
	for defaultRows.Next() {
		var k string
		if err := defaultRows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		defaults = append(defaults, k)
	}
	if !slices.Equal(defaults, []string{"encomenda_montagem"}) {
		t.Errorf("default presets = %v, want only encomenda_montagem (ADR 0006)", defaults)
	}
}

func checkPreset(t *testing.T, preset string, statuses []seededStatus) {
	t.Helper()
	last := -1
	phases := map[string]bool{}
	for _, s := range statuses {
		i := slices.Index(phaseOrder, s.phase)
		if i < last {
			t.Errorf("%s: %s (%s) comes after a later phase", preset, s.key, s.phase)
		}
		last = i
		phases[s.phase] = true
		if s.label == "" || s.clientLabel == "" {
			t.Errorf("%s: %s needs a label and a client label", preset, s.key)
		}
		// Clients never see suppliers, purchases or "Pode atrasar"
		// (docs/product/design-document.md, System rules).
		for _, internal := range []string{"fornecedor", "compra", "atras"} {
			if strings.Contains(strings.ToLower(s.clientLabel), internal) {
				t.Errorf("%s: client label %q of %s names internal work (%q)", preset, s.clientLabel, s.key, internal)
			}
		}
	}
	for _, required := range []string{"draft", "awaiting_deposit", "ready", "handed_over"} {
		if !phases[required] {
			t.Errorf("%s has no %s status (ADR 0004)", preset, required)
		}
	}
}
