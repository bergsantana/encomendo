---
name: new-migration
description: Add or change a Postgres table in Encomendo with goose SQL, tenant columns, composite foreign keys, row level security and the RLS test cases. Use for any schema change.
---

# New migration

Schema changes are where tenant isolation is won or lost. Follow every step.

## 1. Check ownership

Migrations have one owner per milestone (see "Working in parallel sessions" in the root `CLAUDE.md`). If you are not the owner, write the SQL you need in your PR description or a message and stop.

## 2. Create the file

- Path: `backend/internal/adapter/postgres/migrations/NNNN_<what>.sql`, where NNNN is the next number on `main`. Rebase first so the number is free.
- SQL only, never a Go migration. Both sections are required:

```sql
-- +goose Up
…

-- +goose Down
…
```

- Down must undo Up exactly. CI runs up → down → up.

## 3. Table conventions

For a tenant-scoped table:

```sql
CREATE TABLE purchase_lines (
    tenant_id     uuid        NOT NULL REFERENCES tenants (id),
    id            uuid        NOT NULL,           -- UUIDv7, generated in Go
    order_item_id uuid,                           -- null for a stock restock
    cost_cents bigint      NOT NULL CHECK (cost_cents >= 0),
    currency   char(3)     NOT NULL,
    version    int         NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),                                              -- target for composite FKs into this table
    FOREIGN KEY (tenant_id, order_item_id) REFERENCES order_items (tenant_id, id)  -- can't point at another tenant's row
);

CREATE INDEX purchase_lines_tenant_item_idx ON purchase_lines (tenant_id, order_item_id);

ALTER TABLE purchase_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_lines FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON purchase_lines
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

GRANT SELECT, INSERT, UPDATE, DELETE ON purchase_lines TO encomendo_app;
```

(The table above only illustrates the conventions. The real `purchase_lines` in `0011_purchasing.sql` follows `docs/architecture.md` section 7: supplier, expected and promised dates, status.)

Checklist:

- [ ] `tenant_id uuid NOT NULL` and every index starts with `tenant_id`
- [ ] `UNIQUE (tenant_id, id)` so other tables can reference this one with a composite key
- [ ] Composite foreign keys `(tenant_id, x_id)` for every reference to another tenant table
- [ ] The policy reads the settings only through the `0005_rls_helpers.sql` functions (`current_tenant_id()`, `current_user_id()`, `current_client_account_id()`), never `current_setting(…)::uuid`: on a pooled connection the setting can be `''`, and the cast would raise an error instead of matching no rows (ADR 0009)
- [ ] Money as `bigint` cents plus `char(3)` currency
- [ ] `version int` if the aggregate uses optimistic locking
- [ ] `created_at`, `updated_at`
- [ ] `ENABLE` **and** `FORCE ROW LEVEL SECURITY`, policy with `USING` **and** `WITH CHECK`
- [ ] Grants for `encomendo_app` only. Never grant to `PUBLIC`.
- [ ] If clients read the table through the client area, add the second `SELECT`-only policy on `app.client_account_id` (architecture section 5.5). Supplier, cost and purchasing tables never get one.
- [ ] Append-only tables (movements, history, audit) get no `UPDATE`/`DELETE` grant.
- [ ] A global table (like `templates` or `status_workflows`) has a comment saying why it is not tenant-scoped. Published workflow versions are immutable: a change is a new version row, never an `UPDATE`.

## 4. Persistence model

- Add or update the GORM struct in `adapter/postgres/models/<ctx>.go`, with `toDomain()` and `fromDomain()`.
- Never call `AutoMigrate`. The struct follows the SQL, not the other way round.

## 5. Tests

- `test/integration/rls/`: add the table to the RLS suite with all five cases: select, insert with the other tenant's id, update, delete, and no tenant set (expects zero rows). With a client policy, also: another client's rows (zero rows) and any write as a client (rejected).
- `test/integration/postgres/`: repository tests for the new queries, including the constraint errors mapped to domain errors.
- Run:

```bash
make test-integration
go run ./cmd/migrate down && go run ./cmd/migrate up
```

## 6. Review

Ask the `tenancy-security-reviewer` subagent to audit the migration and repository diff before you open the PR.
