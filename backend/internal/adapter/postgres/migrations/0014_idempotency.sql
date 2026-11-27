-- Idempotency-Key storage for the POST endpoints that create resources
-- (milestone 4: orders and payments; ADR 0030). Each key remembers the id of
-- what its first call created, in the same transaction as that write, so a
-- retry is exactly-once. A replay answers that resource as it is now, not a
-- stored body. Keys live 24 hours; expired ones are ignored and may be
-- deleted.
--
-- tenant_isolation, no audit: the table holds only a key, a hash of the
-- request and an id.

-- +goose Up
CREATE TABLE idempotency_keys (
    tenant_id    uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    key          text        NOT NULL CHECK (char_length(key) BETWEEN 1 AND 128 AND key ~ '^[!-~]+$'),
    -- SHA-256 of the method, path and body, so the same key with another
    -- request is refused (idempotency_key_reused).
    request_hash bytea       NOT NULL CHECK (octet_length(request_hash) = 32),
    resource_id  uuid        NOT NULL,
    expires_at   timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, key),
    CHECK (expires_at > created_at)
);
CREATE INDEX idempotency_keys_tenant_expiry_idx ON idempotency_keys (tenant_id, expires_at);

ALTER TABLE idempotency_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE idempotency_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON idempotency_keys
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

-- DELETE lets a recall replace an expired key; a live key is never updated.
GRANT SELECT, INSERT, DELETE ON idempotency_keys TO encomendo_app;

-- +goose Down
DROP TABLE idempotency_keys;
