-- Helpers for RLS policies and the audit trigger function.
--
-- The TxManager sets app.tenant_id, app.user_id and app.client_account_id
-- transaction-local on every transaction, to '' when unset (ADR 0009). On a
-- pooled connection that set one before, current_setting(…, true) returns
-- '' rather than NULL, and ''::uuid raises an error. These helpers turn ''
-- into NULL, so a missing scope matches no row. Policies read the settings
-- only through them.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION current_tenant_id() RETURNS uuid
    LANGUAGE sql STABLE PARALLEL SAFE
    AS $$ SELECT NULLIF(pg_catalog.current_setting('app.tenant_id', true), '')::uuid $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION current_user_id() RETURNS uuid
    LANGUAGE sql STABLE PARALLEL SAFE
    AS $$ SELECT NULLIF(pg_catalog.current_setting('app.user_id', true), '')::uuid $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION current_client_account_id() RETURNS uuid
    LANGUAGE sql STABLE PARALLEL SAFE
    AS $$ SELECT NULLIF(pg_catalog.current_setting('app.client_account_id', true), '')::uuid $$;
-- +goose StatementEnd

-- audit_row records a change to a tenant table in audit_log (0006_platform):
-- which row, what it was and became, and app.user_id as the actor. It runs
-- as the caller, so the audit row passes the same RLS check as the change.
-- Secret hashes are left out of the copies. A delete cascading from a
-- deleted store is not recorded, because the store's log goes with it.
-- +goose StatementBegin
CREATE FUNCTION audit_row() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    tenant uuid;
    old_row jsonb;
    new_row jsonb;
BEGIN
    IF TG_OP = 'DELETE' THEN
        tenant := OLD.tenant_id;
        IF NOT EXISTS (SELECT 1 FROM tenants WHERE id = tenant) THEN
            RETURN NULL;
        END IF;
    ELSE
        tenant := NEW.tenant_id;
    END IF;
    IF TG_OP <> 'INSERT' THEN
        old_row := to_jsonb(OLD) - 'token_hash';
    END IF;
    IF TG_OP <> 'DELETE' THEN
        new_row := to_jsonb(NEW) - 'token_hash';
    END IF;
    INSERT INTO audit_log (tenant_id, table_name, action, actor_user_id, old_row, new_row)
    VALUES (tenant, TG_TABLE_NAME, lower(TG_OP), current_user_id(), old_row, new_row);
    RETURN NULL;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION current_tenant_id(), current_user_id(), current_client_account_id(), audit_row() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION current_tenant_id(), current_user_id(), current_client_account_id() TO encomendo_app;

-- +goose Down
DROP FUNCTION audit_row();
DROP FUNCTION current_client_account_id();
DROP FUNCTION current_user_id();
DROP FUNCTION current_tenant_id();
