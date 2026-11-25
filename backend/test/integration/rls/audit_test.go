//go:build integration

package rls_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// auditRow is one audit_log row. The query selects by actor, so a row found
// at all was recorded with app.user_id.
type auditRow struct {
	table, action, actor string
	old, new             map[string]any
}

// TestAuditTrigger_RecordsChangesWithoutSecrets checks what audit_row writes
// (0005_rls_helpers): one row per change, under the store, with app.user_id
// as the actor, the old and new copies the action calls for, and no column
// ending in _hash in either copy.
func TestAuditTrigger_RecordsChangesWithoutSecrets(t *testing.T) {
	t.Parallel()
	w := seed(t)
	app := env.Open(t, pgtest.App)
	ctx := t.Context()

	tx, err := app.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }() // a no-op after the commit below
	invitation := pgtest.NewID().String()
	for _, step := range []struct {
		query string
		args  []any
	}{
		{setScopeSQL, []any{w.a.String(), w.ownerA.String(), ""}},
		{`INSERT INTO invitations (tenant_id, id, email, role, token_hash, invited_by, expires_at)
			VALUES ($1, $2, 'auditoria@teste.encomendo.com.br', 'staff', $3, $4, $5)`,
			[]any{w.a.String(), invitation, pgtest.RandomHash(), w.ownerA.String(), time.Now().Add(time.Hour)}},
		{`UPDATE invitations SET role = 'viewer' WHERE id = $1`, []any{invitation}},
		{`DELETE FROM memberships WHERE tenant_id = $1 AND user_id = $2`, []any{w.a.String(), w.both.String()}},
	} {
		if _, err := tx.ExecContext(ctx, step.query, step.args...); err != nil {
			t.Fatalf("%s: %v", step.query, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	got := auditRows(t, w)
	want := []struct{ table, action string }{
		{"invitations", "insert"}, {"invitations", "update"}, {"memberships", "delete"},
	}
	if len(got) != len(want) {
		t.Fatalf("audit rows by store A's owner = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, r := range got {
		if r.table != want[i].table || r.action != want[i].action {
			t.Errorf("audit row %d = %s %s, want %s %s", i, r.table, r.action, want[i].table, want[i].action)
		}
		if (r.old == nil) != (r.action == "insert") || (r.new == nil) != (r.action == "delete") {
			t.Errorf("audit row %d (%s): old %v, new %v; want the copies the action calls for", i, r.action, r.old, r.new)
		}
		for _, copied := range []map[string]any{r.old, r.new} {
			for key := range copied {
				if strings.HasSuffix(key, "_hash") {
					t.Errorf("audit row %d copies %s; hashes never reach the log", i, key)
				}
			}
		}
	}
}

// TestAuditTrigger_SkipsADeletedStoresCascade: deleting a store cascades to
// its team and settings, and the trigger must not try to log them under a
// store that no longer exists (the store's log goes with it).
func TestAuditTrigger_SkipsADeletedStoresCascade(t *testing.T) {
	t.Parallel()
	w := seed(t)
	db := env.Seeder(t).DB()
	if _, err := db.ExecContext(t.Context(), `DELETE FROM tenants WHERE id = $1`, w.c.String()); err != nil {
		t.Fatalf("delete a store with members, settings and invitations: %v", err)
	}
	var left int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM audit_log WHERE tenant_id = $1`, w.c.String()).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d audit rows left for a deleted store", left)
	}
}

func auditRows(t *testing.T, w world) []auditRow {
	t.Helper()
	rows, err := env.Seeder(t).DB().QueryContext(t.Context(), `SELECT table_name, action, actor_user_id::text,
			coalesce(old_row, 'null'), coalesce(new_row, 'null')
		FROM audit_log WHERE tenant_id = $1 AND actor_user_id = $2 ORDER BY id`, w.a.String(), w.ownerA.String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		var oldJSON, newJSON []byte
		if err := rows.Scan(&r.table, &r.action, &r.actor, &oldJSON, &newJSON); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(oldJSON, &r.old); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(newJSON, &r.new); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
