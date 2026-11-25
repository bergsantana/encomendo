//go:build integration

package postgres_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bergsantana/encomendo/backend/internal/adapter/postgres"
	appshared "github.com/bergsantana/encomendo/backend/internal/app/shared"
	"github.com/bergsantana/encomendo/backend/test/contract/sharedports"
	"github.com/bergsantana/encomendo/backend/test/integration/pgtest"
)

// The same suites run against the fakes in test/contract (ADR 0009).

func TestTxManager_Contract(t *testing.T) {
	db := openApp(t)
	sharedports.RunTxManagerContract(t, func(*testing.T) appshared.TxManager {
		return postgres.NewTxManager(db)
	})
}

func TestOutbox_Contract(t *testing.T) {
	db := openApp(t)
	seeder := env.Seeder(t)
	sharedports.RunEventPublisherContract(t, func(t *testing.T) sharedports.EventsHarness {
		tenant := seeder.Tenant(seeder.User())
		return sharedports.EventsHarness{
			Tx:        postgres.NewTxManager(db),
			Publisher: postgres.NewOutbox(),
			TenantID:  tenant,
			Committed: func(t *testing.T) []sharedports.StoredEvent {
				t.Helper()
				rows, err := seeder.DB().QueryContext(t.Context(),
					`SELECT name, payload FROM outbox_events WHERE tenant_id = $1 ORDER BY id`, tenant.String())
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = rows.Close() }()
				var out []sharedports.StoredEvent
				for rows.Next() {
					var e sharedports.StoredEvent
					var payload []byte
					if err := rows.Scan(&e.Name, &payload); err != nil {
						t.Fatal(err)
					}
					e.Payload = json.RawMessage(payload)
					out = append(out, e)
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				return out
			},
		}
	})
}

// TestOutbox_AppCannotReadTheOutbox: the API only appends events; the worker
// relays them in milestone 6, under its own role.
func TestOutbox_AppCannotReadTheOutbox(t *testing.T) {
	t.Parallel()
	app := env.Open(t, pgtest.App)
	_, err := app.ExecContext(t.Context(), `SELECT 1 FROM outbox_events LIMIT 1`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Errorf("encomendo_app reading outbox_events: err = %v, want permission denied (42501)", err)
	}
}
