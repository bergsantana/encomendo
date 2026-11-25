-- Platform tables: the audit log and the outbox. Both are tenant-scoped and
-- append-only for encomendo_app; RLS and grants come in 0007_rls_policies.
-- idempotency_keys lands with its first user in milestone 5 (ADR 0014).
--
-- Their rows are written by a trigger or by the outbox adapter, not built as
-- aggregates in Go, so their ids come from an identity column rather than a
-- Go UUIDv7. The id also gives the order rows were written in.

-- +goose Up
CREATE TABLE audit_log (
    tenant_id     uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    table_name    text        NOT NULL,
    action        text        NOT NULL CHECK (action IN ('insert', 'update', 'delete')),
    -- app.user_id of the transaction; null for work no staff member did.
    -- Not a foreign key, so the log outlives the account.
    actor_user_id uuid,
    old_row       jsonb,
    new_row       jsonb,
    at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_tenant_at_idx ON audit_log (tenant_id, at DESC, id DESC);

-- An event exists if and only if the change that caused it committed
-- (app/shared EventPublisher). cmd/worker relays it in milestone 6, under
-- its own role; the index it needs comes with it.
CREATE TABLE outbox_events (
    tenant_id    uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         text        NOT NULL CHECK (name <> ''),
    payload      jsonb       NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);

CREATE INDEX outbox_events_tenant_idx ON outbox_events (tenant_id, id);

-- Team and settings changes are recorded for the owner (audit views in
-- milestone 8).
CREATE TRIGGER memberships_audit AFTER INSERT OR UPDATE OR DELETE ON memberships
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER invitations_audit AFTER INSERT OR UPDATE OR DELETE ON invitations
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER tenant_settings_audit AFTER INSERT OR UPDATE OR DELETE ON tenant_settings
    FOR EACH ROW EXECUTE FUNCTION audit_row();

-- +goose Down
DROP TRIGGER tenant_settings_audit ON tenant_settings;
DROP TRIGGER invitations_audit ON invitations;
DROP TRIGGER memberships_audit ON memberships;
DROP TABLE outbox_events;
DROP TABLE audit_log;
