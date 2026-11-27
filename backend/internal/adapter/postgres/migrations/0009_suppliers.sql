-- Suppliers and the link between an item and its suppliers (milestone 3;
-- ADR 0024, ADR 0025). Both tables hold internal data (supplier names,
-- phones, lead times by supplier and costs), so both are staff-only: a row
-- is visible only with a tenant AND a staff user set, and a store page scope
-- (tenant, no user) reads zero rows whatever its query (ADR 0024). They have
-- no client policy, ever (docs/architecture.md section 5.5).
--
-- supplier_items is written by the catalog repository together with its
-- item, which keeps items.ready_days in step (ADR 0025). A cost belongs to
-- the item and supplier pair, not to the main or backup slot.
--
-- Neither table is audited yet: audit_log keeps whole rows, and every staff
-- role reads it while supplier costs are the owner's (see 0008_catalog).

-- +goose Up
CREATE TABLE suppliers (
    tenant_id      uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id             uuid        NOT NULL,
    name           text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120 AND name = btrim(name)),
    -- Digits for wa.me links (milestone 5): 55 + DDD + number.
    whatsapp_phone text        NOT NULL CHECK (whatsapp_phone ~ '^55[1-9]{2}[0-9]{8,9}$'),
    version        int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id)
);
CREATE INDEX suppliers_tenant_name_idx ON suppliers (tenant_id, name, id);

CREATE TABLE supplier_items (
    tenant_id   uuid        NOT NULL,
    item_id     uuid        NOT NULL,
    supplier_id uuid        NOT NULL,
    role        text        NOT NULL CHECK (role IN ('main', 'backup')),
    lead_days   int         NOT NULL CHECK (lead_days BETWEEN 0 AND 365),
    -- Null when no cost is recorded: an admin links a supplier without
    -- seeing or setting its cost (ADR 0025).
    cost_cents  bigint      CHECK (cost_cents BETWEEN 0 AND 100000000000),
    currency    char(3)     NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    -- One main and at most one backup per item, and never the same supplier
    -- twice ("O fornecedor reserva precisa ser outro.").
    PRIMARY KEY (tenant_id, item_id, role),
    UNIQUE (tenant_id, item_id, supplier_id),
    -- The app deletes neither items nor suppliers; these cascade only when a
    -- store is deleted (see 0008_catalog).
    FOREIGN KEY (tenant_id, item_id) REFERENCES items (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, supplier_id) REFERENCES suppliers (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX supplier_items_tenant_supplier_idx ON supplier_items (tenant_id, supplier_id);

ALTER TABLE suppliers ENABLE ROW LEVEL SECURITY;
ALTER TABLE suppliers FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON suppliers
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

ALTER TABLE supplier_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE supplier_items FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON supplier_items
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

-- Suppliers are never deleted in milestone 3. An item's supplier links are
-- replaced as the item is saved (a main and backup swap is a delete and two
-- inserts), so supplier_items has DELETE.
GRANT SELECT, INSERT ON suppliers TO encomendo_app;
GRANT UPDATE (name, whatsapp_phone, version, updated_at) ON suppliers TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON supplier_items TO encomendo_app;
GRANT UPDATE (lead_days, cost_cents, updated_at) ON supplier_items TO encomendo_app;

-- +goose Down
DROP TABLE supplier_items;
DROP TABLE suppliers;
