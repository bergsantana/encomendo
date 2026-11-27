-- audit_log reads become staff-only (security review of step 3.4).
--
-- Until now audit_log had tenant_isolation, so a scope with a tenant and no
-- user could read every audited row of its store: membership rows, the
-- invitee's email on invitations, and (from 0008) unpublished item and kit
-- drafts. Store page requests (milestone 7) and invitation preview and
-- acceptance run in such a scope.
--
-- Writes keep the tenant check alone: audit_row() runs as the caller, so
-- every scope that may change an audited table may also write its audit row.
-- Reads need a staff user too, as staff_only does (ADR 0024). The trigger
-- never reads audit_log (no RETURNING), so INSERT needs no SELECT policy.
--
-- Staff-only tables (item_costs, suppliers, supplier_items) still stay out of
-- the audit: every staff role reads audit_log, viewers included, and costs
-- are the owner's only (ADR 0025).

-- +goose Up
DROP POLICY tenant_isolation ON audit_log;
CREATE POLICY audit_write ON audit_log FOR INSERT
    WITH CHECK (tenant_id = current_tenant_id());
CREATE POLICY staff_read ON audit_log FOR SELECT
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

-- +goose Down
DROP POLICY staff_read ON audit_log;
DROP POLICY audit_write ON audit_log;
CREATE POLICY tenant_isolation ON audit_log
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());
