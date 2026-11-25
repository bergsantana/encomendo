-- Row level security for the tables of 0004_tenancy and 0006_platform, and
-- their grants to encomendo_app (ADR 0002, ADR 0014). Later migrations write
-- the policies of their own tables in the same file.
--
-- Every policy reads the scope through the 0005 helpers, never a direct
-- cast. FORCE binds the table owner too. Postgres ORs permissive policies,
-- so every extra policy below is SELECT only and applies only when no tenant
-- is set: a store's own transactions stay inside that store.

-- +goose Up
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenants
    USING (id = current_tenant_id())
    WITH CHECK (id = current_tenant_id());
-- The store picker (ADR 0010): with no tenant set, a user reads the stores
-- they belong to. Writes stay on tenant_isolation alone.
CREATE POLICY member_stores ON tenants FOR SELECT
    USING (current_tenant_id() IS NULL AND EXISTS (
        SELECT 1 FROM memberships m WHERE m.tenant_id = tenants.id AND m.user_id = current_user_id()));

ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON memberships
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());
-- With no tenant set, a user reads their own memberships (ADR 0010). Never
-- folded into tenant_isolation: with WITH CHECK, "tenant or me" would let
-- user-scoped code add itself to any store.
CREATE POLICY own_memberships ON memberships FOR SELECT
    USING (current_tenant_id() IS NULL AND user_id = current_user_id());

ALTER TABLE tenant_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_settings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_settings
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE invitations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON invitations
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_log
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE outbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON outbox_events
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

-- Least privilege. UPDATE only on the columns the use cases change, so no
-- row can move to another tenant and a store cannot change its own slug or
-- lift its own suspension (status is the platform's). No DELETE on stores,
-- settings or invitations in milestone 2: a store is never deleted yet, and
-- invitations are revoked, not deleted. The audit log and the outbox are
-- append-only; the app never reads the outbox.
GRANT SELECT, INSERT ON tenants TO encomendo_app;
GRANT UPDATE (name, niche, version, updated_at) ON tenants TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON memberships TO encomendo_app;
GRANT UPDATE (role, version, updated_at) ON memberships TO encomendo_app;
GRANT SELECT, INSERT ON tenant_settings TO encomendo_app;
GRANT UPDATE (status_workflow_id, status_workflow_version, stock_enabled, production_enabled,
    purchase_before_deposit, deposit_percent, assembly_days, safety_margin_days, whatsapp_phone,
    version, updated_at) ON tenant_settings TO encomendo_app;
GRANT SELECT, INSERT ON invitations TO encomendo_app;
GRANT UPDATE (role, token_hash, invited_by, expires_at, accepted_at, revoked_at, updated_at)
    ON invitations TO encomendo_app;
GRANT SELECT, INSERT ON audit_log TO encomendo_app;
GRANT INSERT ON outbox_events TO encomendo_app;

-- +goose Down
REVOKE ALL ON tenants, memberships, tenant_settings, invitations, audit_log, outbox_events FROM encomendo_app;

DROP POLICY tenant_isolation ON outbox_events;
ALTER TABLE outbox_events NO FORCE ROW LEVEL SECURITY;
ALTER TABLE outbox_events DISABLE ROW LEVEL SECURITY;

DROP POLICY tenant_isolation ON audit_log;
ALTER TABLE audit_log NO FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_log DISABLE ROW LEVEL SECURITY;

DROP POLICY tenant_isolation ON invitations;
ALTER TABLE invitations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE invitations DISABLE ROW LEVEL SECURITY;

DROP POLICY tenant_isolation ON tenant_settings;
ALTER TABLE tenant_settings NO FORCE ROW LEVEL SECURITY;
ALTER TABLE tenant_settings DISABLE ROW LEVEL SECURITY;

DROP POLICY own_memberships ON memberships;
DROP POLICY tenant_isolation ON memberships;
ALTER TABLE memberships NO FORCE ROW LEVEL SECURITY;
ALTER TABLE memberships DISABLE ROW LEVEL SECURITY;

DROP POLICY member_stores ON tenants;
DROP POLICY tenant_isolation ON tenants;
ALTER TABLE tenants NO FORCE ROW LEVEL SECURITY;
ALTER TABLE tenants DISABLE ROW LEVEL SECURITY;
