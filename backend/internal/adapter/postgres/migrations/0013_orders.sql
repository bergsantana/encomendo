-- Orders (milestone 4; ADR 0030, docs/plans/m4-code-design.md sections 2.2
-- and 1.8). Three RLS tiers:
--
--   - tenant_isolation on the order and what it is made of: counters,
--     orders, lines, items, payments and the status history.
--   - staff_only (ADR 0024) on what a store page scope must never read:
--     order_item_sourcing (supplier and cost at order time) and
--     order_messages (the client's phone and the WhatsApp texts). A scope
--     with a tenant and no user reads zero rows from them.
--   - client_read, a second SELECT-only policy on orders, order_lines,
--     order_items and payments: with no tenant set, a client account reads
--     the orders linked to it, across stores, and writes nothing
--     (docs/architecture.md section 5.5, item 7). History, messages,
--     counters and sourcing have no client policy, ever. The client area
--     (milestone 7) still answers only its own view type, never these
--     columns as they are (docs/architecture.md section 2, rule 8).
--
-- The tracking token is stored only as its SHA-256 (backend/CLAUDE.md), and
-- audit_row (0005) leaves every *_hash column out of the audit. Messages
-- never hold a tracking link either: the app stores them with the link
-- replaced (ADR 0030, Amendments). Sourcing and messages are not audited:
-- every staff role reads audit_log, while costs are the owner's.

-- +goose Up
-- One counter per store; the order's insert transaction takes the next
-- number with INSERT … ON CONFLICT DO UPDATE … RETURNING, so two orders
-- never share one.
CREATE TABLE order_counters (
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    last_number int         NOT NULL DEFAULT 0 CHECK (last_number >= 0),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id)
);

CREATE TABLE orders (
    tenant_id               uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id                      uuid        NOT NULL,
    number                  int         NOT NULL CHECK (number >= 1),
    customer_id             uuid        NOT NULL,
    -- The workflow version the order was made with, for life (ADR 0004).
    workflow_id             uuid        NOT NULL,
    workflow_version        int         NOT NULL,
    -- The last real status: a cancelled order keeps the one it left, and
    -- cancelled_at says it is cancelled (cancelled is never a status row).
    status_key              text        NOT NULL,
    release_date            date,
    released_by             uuid,       -- only for a release without the deposit
    deposit_confirmed_cents bigint      CHECK (deposit_confirmed_cents BETWEEN 0 AND 100000000000),
    delivery_date           date        NOT NULL, -- the promise to the client
    promise_changed_by_hand boolean     NOT NULL DEFAULT false,
    assembly_days           int         NOT NULL CHECK (assembly_days BETWEEN 0 AND 60),
    safety_margin_days      int         NOT NULL CHECK (safety_margin_days BETWEEN 0 AND 60),
    deposit_percent         int         NOT NULL CHECK (deposit_percent BETWEEN 0 AND 100),
    currency                char(3)     NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    subtotal_cents          bigint      NOT NULL CHECK (subtotal_cents BETWEEN 0 AND 5000000000000),
    freight_cents           bigint      NOT NULL DEFAULT 0 CHECK (freight_cents BETWEEN 0 AND 100000000000),
    discount_cents          bigint      NOT NULL DEFAULT 0 CHECK (discount_cents >= 0 AND discount_cents <= subtotal_cents),
    handover                text        NOT NULL CHECK (handover IN ('pickup', 'delivery')),
    -- The order's own copy of the address (1.8): no foreign key, so a
    -- customer's address can be edited or removed without touching it.
    delivery_address_id     uuid,
    delivery_address        jsonb,
    handed_over_as          text        CHECK (handed_over_as IN ('pickup', 'delivery')),
    handed_over_at          timestamptz,
    cancelled_at            timestamptz,
    cancel_reason           text        CHECK (char_length(cancel_reason) BETWEEN 1 AND 500),
    tracking_token_hash     bytea       NOT NULL CHECK (octet_length(tracking_token_hash) = 32),
    -- Milestone 7 links an order to a client account; no grant before then.
    client_account_id       uuid        REFERENCES client_accounts (id) ON DELETE SET NULL,
    created_by              uuid,       -- null for a store page order (milestone 7)
    version                 int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, number),
    FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id),
    FOREIGN KEY (workflow_id, workflow_version, status_key)
        REFERENCES workflow_statuses (workflow_id, workflow_version, key),
    CHECK ((cancelled_at IS NULL) = (cancel_reason IS NULL)),
    CHECK ((handed_over_at IS NULL) = (handed_over_as IS NULL)),
    CHECK (cancelled_at IS NULL OR handed_over_at IS NULL),
    CHECK (handover = 'pickup' OR (delivery_address_id IS NOT NULL AND delivery_address IS NOT NULL)),
    CHECK (handover = 'delivery' OR freight_cents = 0)
);
-- Unique across stores, so a token can never match two orders; a lookup
-- still runs in the store of the subdomain and RLS hides every other store.
CREATE UNIQUE INDEX orders_tracking_token_idx ON orders (tracking_token_hash);
CREATE INDEX orders_tenant_active_idx ON orders (tenant_id, delivery_date, id)
    WHERE cancelled_at IS NULL AND handed_over_at IS NULL;
CREATE INDEX orders_tenant_customer_idx ON orders (tenant_id, customer_id, created_at DESC);
CREATE INDEX orders_tenant_created_idx ON orders (tenant_id, created_at DESC, id);
-- Not led by tenant_id on purpose: the client area reads one account's
-- orders across stores, with no tenant set (docs/architecture.md 5.5).
CREATE INDEX orders_client_idx ON orders (client_account_id) WHERE client_account_id IS NOT NULL;

CREATE TABLE order_lines (
    tenant_id          uuid        NOT NULL,
    id                 uuid        NOT NULL,
    order_id           uuid        NOT NULL,
    position           int         NOT NULL CHECK (position >= 1),
    kit_id             uuid,       -- a snapshot: no foreign key, the kit may change or be archived
    kit                jsonb,      -- KitSnapshot: name and swaps; both null for a loose item
    price_cents        bigint      NOT NULL CHECK (price_cents BETWEEN 0 AND 100000000000),
    -- The kit's own day overrides at order time, null when it had none.
    assembly_days      int         CHECK (assembly_days BETWEEN 0 AND 60),
    safety_margin_days int         CHECK (safety_margin_days BETWEEN 0 AND 60),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id) ON DELETE CASCADE,
    CHECK ((kit_id IS NULL) = (kit IS NULL))
);
CREATE INDEX order_lines_tenant_order_idx ON order_lines (tenant_id, order_id, position);

-- What the client may see of each item: the ItemSnapshot (name, unit price,
-- quantity, options, lead days) and where it stands. No supplier, no cost.
CREATE TABLE order_items (
    tenant_id     uuid        NOT NULL,
    id            uuid        NOT NULL,
    order_id      uuid        NOT NULL,
    line_id       uuid        NOT NULL,
    position      int         NOT NULL CHECK (position >= 1),
    snapshot      jsonb       NOT NULL,
    sourcing_mode text        NOT NULL CHECK (sourcing_mode IN ('to_order', 'stock', 'in_house')),
    quantity      int         NOT NULL CHECK (quantity BETWEEN 1 AND 99),
    status        text        NOT NULL CHECK (status IN ('to_buy', 'sent_to_supplier', 'in_transit', 'received',
        'planned', 'in_production', 'ready', 'installed')),
    check_result  text        CHECK (check_result IN ('ok', 'defective')),
    expected_date date        NOT NULL,
    arrived_on    date,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, line_id) REFERENCES order_lines (tenant_id, id) ON DELETE CASCADE,
    -- An in-house item never takes a bought item's status, and the reverse.
    CHECK (CASE WHEN sourcing_mode = 'in_house'
        THEN status IN ('planned', 'in_production', 'ready', 'installed')
        ELSE status IN ('to_buy', 'sent_to_supplier', 'in_transit', 'received', 'installed') END)
);
CREATE INDEX order_items_tenant_order_idx ON order_items (tenant_id, order_id, position);
CREATE INDEX order_items_tenant_line_idx ON order_items (tenant_id, line_id);

-- Staff-only (ADR 0024, ADR 0030): an item's supplier, its backup and the
-- cost at order time. Not a foreign key to suppliers: the snapshot outlives
-- a renamed or removed supplier.
CREATE TABLE order_item_sourcing (
    tenant_id            uuid        NOT NULL,
    order_item_id        uuid        NOT NULL,
    supplier_id          uuid,       -- null for an in-house item
    supplier_name        text        NOT NULL DEFAULT '',
    cost_cents           bigint      CHECK (cost_cents BETWEEN 0 AND 100000000000), -- per unit; null when unknown
    backup_supplier_id   uuid,
    backup_supplier_name text,
    backup_lead_days     int         CHECK (backup_lead_days BETWEEN 0 AND 365),
    backup_cost_cents    bigint      CHECK (backup_cost_cents BETWEEN 0 AND 100000000000),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, order_item_id),
    FOREIGN KEY (tenant_id, order_item_id) REFERENCES order_items (tenant_id, id) ON DELETE CASCADE,
    CHECK ((backup_supplier_id IS NULL) = (backup_lead_days IS NULL)),
    CHECK ((backup_supplier_id IS NULL) = (backup_supplier_name IS NULL))
);

-- Payments recorded by hand (ADR 0023). A wrong one is voided, never deleted.
CREATE TABLE payments (
    tenant_id    uuid        NOT NULL,
    id           uuid        NOT NULL,
    order_id     uuid        NOT NULL,
    kind         text        NOT NULL CHECK (kind IN ('deposit', 'balance')),
    amount_cents bigint      NOT NULL CHECK (amount_cents BETWEEN 1 AND 5000000000000),
    currency     char(3)     NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    method       text        NOT NULL CHECK (method IN ('pix', 'dinheiro', 'cartao_credito', 'cartao_debito',
        'transferencia', 'outro')),
    recorded_by  uuid,
    recorded_at  timestamptz NOT NULL,
    voided_at    timestamptz,
    voided_by    uuid,
    void_reason  text        CHECK (char_length(void_reason) BETWEEN 1 AND 500),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id) ON DELETE CASCADE,
    CHECK ((voided_at IS NULL) = (void_reason IS NULL))
);
CREATE INDEX payments_tenant_order_idx ON payments (tenant_id, order_id, recorded_at);
CREATE INDEX payments_tenant_recorded_idx ON payments (tenant_id, recorded_at);

-- Append-only: who did what to the order, for staff. No client policy: it
-- holds staff ids and reasons. It stays tenant_isolation, not staff-only,
-- because a store page order (milestone 7) writes its "created" entry.
CREATE TABLE order_status_history (
    tenant_id  uuid        NOT NULL,
    id         uuid        NOT NULL,
    order_id   uuid        NOT NULL,
    kind       text        NOT NULL CHECK (kind IN ('created', 'status', 'item', 'backup_supplier', 'payment',
        'payment_voided', 'released', 'date', 'changed', 'tracking_link', 'cancelled')),
    by_user    uuid,       -- null for a store page order
    at         timestamptz NOT NULL,
    detail     jsonb       NOT NULL, -- the HistoryEntry fields of its kind; never a supplier, phone or token
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX order_status_history_tenant_order_idx ON order_status_history (tenant_id, order_id, at DESC, id);

-- Staff-only: every wa.me message made for the order. The text never holds
-- a tracking link (ADR 0030, Amendments); it holds the client's phone.
CREATE TABLE order_messages (
    tenant_id  uuid        NOT NULL,
    id         uuid        NOT NULL,
    order_id   uuid        NOT NULL,
    kind       text        NOT NULL CHECK (kind IN ('tracking_link', 'created', 'date_changed', 'changed')),
    to_phone   text        NOT NULL CHECK (to_phone ~ '^55[1-9]{2}[0-9]{8,9}$'),
    text       text        NOT NULL CHECK (char_length(text) BETWEEN 1 AND 4000),
    url        text        NOT NULL CHECK (url LIKE 'https://wa.me/%'),
    by_user    uuid,
    at         timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX order_messages_tenant_order_idx ON order_messages (tenant_id, order_id, at DESC);

-- Row level security.
ALTER TABLE order_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_counters FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON order_counters
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON orders
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());
CREATE POLICY client_read ON orders FOR SELECT
    USING (current_tenant_id() IS NULL AND client_account_id IS NOT NULL
        AND client_account_id = current_client_account_id());

ALTER TABLE order_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON order_lines
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());
CREATE POLICY client_read ON order_lines FOR SELECT
    USING (current_tenant_id() IS NULL AND EXISTS (SELECT 1 FROM orders o
        WHERE o.tenant_id = order_lines.tenant_id AND o.id = order_lines.order_id
          AND o.client_account_id IS NOT NULL AND o.client_account_id = current_client_account_id()));

ALTER TABLE order_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_items FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON order_items
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());
CREATE POLICY client_read ON order_items FOR SELECT
    USING (current_tenant_id() IS NULL AND EXISTS (SELECT 1 FROM orders o
        WHERE o.tenant_id = order_items.tenant_id AND o.id = order_items.order_id
          AND o.client_account_id IS NOT NULL AND o.client_account_id = current_client_account_id()));

ALTER TABLE order_item_sourcing ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_item_sourcing FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON order_item_sourcing
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

ALTER TABLE payments ENABLE ROW LEVEL SECURITY;
ALTER TABLE payments FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON payments
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());
CREATE POLICY client_read ON payments FOR SELECT
    USING (current_tenant_id() IS NULL AND EXISTS (SELECT 1 FROM orders o
        WHERE o.tenant_id = payments.tenant_id AND o.id = payments.order_id
          AND o.client_account_id IS NOT NULL AND o.client_account_id = current_client_account_id()));

ALTER TABLE order_status_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_status_history FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON order_status_history
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE order_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_messages FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON order_messages
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

-- Audit: the order and what the client may see of it. Not sourcing (costs)
-- nor messages (phones and texts), and never the token hash (audit_row
-- drops *_hash columns).
CREATE TRIGGER orders_audit AFTER INSERT OR UPDATE OR DELETE ON orders
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER order_lines_audit AFTER INSERT OR UPDATE OR DELETE ON order_lines
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER order_items_audit AFTER INSERT OR UPDATE OR DELETE ON order_items
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER payments_audit AFTER INSERT OR UPDATE OR DELETE ON payments
    FOR EACH ROW EXECUTE FUNCTION audit_row();

-- Grants: only the columns a use case changes. Nothing here is ever
-- deleted by the app but lines, items and their sourcing, which a change
-- replaces; payments, history and messages are kept for good.
GRANT SELECT, INSERT ON order_counters TO encomendo_app;
GRANT UPDATE (last_number, updated_at) ON order_counters TO encomendo_app;
GRANT SELECT, INSERT ON orders TO encomendo_app;
GRANT UPDATE (status_key, release_date, released_by, deposit_confirmed_cents, delivery_date,
    promise_changed_by_hand, assembly_days, safety_margin_days, deposit_percent, subtotal_cents,
    freight_cents, discount_cents, handover, delivery_address_id, delivery_address, handed_over_as,
    handed_over_at, cancelled_at, cancel_reason, tracking_token_hash, version, updated_at)
    ON orders TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON order_lines TO encomendo_app;
GRANT UPDATE (position, kit_id, kit, price_cents, assembly_days, safety_margin_days, updated_at)
    ON order_lines TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON order_items TO encomendo_app;
GRANT UPDATE (position, snapshot, sourcing_mode, quantity, status, check_result, expected_date, arrived_on,
    updated_at) ON order_items TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON order_item_sourcing TO encomendo_app;
GRANT UPDATE (supplier_id, supplier_name, cost_cents, backup_supplier_id, backup_supplier_name,
    backup_lead_days, backup_cost_cents, updated_at) ON order_item_sourcing TO encomendo_app;
GRANT SELECT, INSERT ON payments TO encomendo_app;
GRANT UPDATE (voided_at, voided_by, void_reason, updated_at) ON payments TO encomendo_app;
GRANT SELECT, INSERT ON order_status_history TO encomendo_app;
GRANT SELECT, INSERT ON order_messages TO encomendo_app;

-- +goose Down
DROP TABLE order_messages;
DROP TABLE order_status_history;
DROP TABLE payments;
DROP TABLE order_item_sourcing;
DROP TABLE order_items;
DROP TABLE order_lines;
DROP TABLE orders;
DROP TABLE order_counters;
