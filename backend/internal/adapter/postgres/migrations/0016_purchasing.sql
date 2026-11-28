-- Milestone 5: purchasing and receiving (ADR 0031; docs/plans/m5-code-design.md
-- section 2).
--
-- "A comprar" is not stored: it is read from the orders (bought items still
-- to buy, of released orders). A purchase line is written when it is sent
-- to a supplier in a pedido de compra (C-0001), or received at the counter.
--
-- Every table here is internal (suppliers, costs, purchasing; ADR 0024):
--   - staff_only, a tenant AND a staff user, so a store page scope reads and
--     writes nothing, whatever its query;
--   - no client policy, ever;
--   - no audit trigger: audit_log is read by every staff role, viewers
--     included, and these rows name suppliers and their WhatsApp (as
--     suppliers and order_item_sourcing, 0009 and 0013, are not audited);
--   - costs in purchase_line_costs, apart from the lines.
--
-- Referential actions run as the table owner and bypass RLS: the
-- ON DELETE SET NULL (order_item_id) on a line is guarded by the order
-- repository, which refuses any write without a staff scope (security
-- review of 5.1). A line whose order item was removed keeps its row, its
-- order and its own snapshot; a null order_item_id means "removed", not a
-- restock (milestone 6 gives restocks their own column).
--
-- Grants: SELECT and INSERT, and UPDATE only on the columns a use case
-- changes, so the database keeps a line's order, item, supplier and snapshot
-- fixed. Receipts and their lines and photos are append-only. Nothing is
-- deleted: lines are cancelled.

-- +goose Up

-- The supplier's promised date and the price paid are staff data: they live
-- in the staff-only sourcing, never in order_items (which has a client read
-- policy, 0013). 0013 grants UPDATE on sourcing by column.
ALTER TABLE order_item_sourcing
    ADD COLUMN promised_date   date,
    ADD COLUMN paid_cost_cents bigint CHECK (paid_cost_cents BETWEEN 0 AND 100000000000);
GRANT UPDATE (promised_date, paid_cost_cents) ON order_item_sourcing TO encomendo_app;

-- A history entry of a supplier's promised date (the C-number of a move
-- rides in detail, as every other field does).
ALTER TABLE order_status_history DROP CONSTRAINT order_status_history_kind_check;
ALTER TABLE order_status_history ADD CONSTRAINT order_status_history_kind_check
    CHECK (kind IN ('created', 'status', 'item', 'backup_supplier', 'payment', 'payment_voided', 'released',
        'date', 'changed', 'tracking_link', 'cancelled', 'promised_date'));

-- A line's item must belong to the line's order.
ALTER TABLE order_items ADD CONSTRAINT order_items_tenant_order_id_key UNIQUE (tenant_id, order_id, id);

-- One counter per store; a send takes the next number with
-- INSERT … ON CONFLICT DO UPDATE … RETURNING, as order_counters.
CREATE TABLE purchase_counters (
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    last_number int         NOT NULL DEFAULT 0 CHECK (last_number >= 0),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id)
);

-- A pedido de compra: what the store sent one supplier, with the message as
-- sent. The text never holds the client's name, phone, address or order
-- number (docs/product/design-document.md, System rules). Actors have no
-- foreign key, so the record outlives a removed account (as payments).
CREATE TABLE supplier_orders (
    tenant_id      uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id             uuid        NOT NULL,
    number         int         NOT NULL CHECK (number >= 1),
    supplier_id    uuid        NOT NULL,
    supplier_name  text        NOT NULL CHECK (char_length(supplier_name) BETWEEN 1 AND 120),
    supplier_phone text        NOT NULL CHECK (supplier_phone ~ '^55[1-9]{2}[0-9]{8,9}$'),
    sent_at        timestamptz NOT NULL,
    sent_by        uuid        NOT NULL,
    promised_date  date,
    in_transit_at  timestamptz,
    undone_at      timestamptz,
    undone_by      uuid,
    message_text   text        NOT NULL CHECK (char_length(message_text) BETWEEN 1 AND 20000),
    message_url    text        NOT NULL CHECK (message_url LIKE 'https://wa.me/%'),
    version        int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, number),
    FOREIGN KEY (tenant_id, supplier_id) REFERENCES suppliers (tenant_id, id),
    CHECK ((undone_at IS NULL) = (undone_by IS NULL))
);
CREATE INDEX supplier_orders_tenant_sent_idx ON supplier_orders (tenant_id, sent_at DESC, id DESC);

-- One bought item of one order, sent in a pedido de compra or received at
-- the counter, with its own snapshot of what was bought. At most one open
-- line (sent or in transit) per order item.
CREATE TABLE purchase_lines (
    tenant_id         uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id                uuid        NOT NULL,
    supplier_order_id uuid,       -- null for an item received at the counter
    supplier_id       uuid        NOT NULL,
    order_id          uuid        NOT NULL,
    order_item_id     uuid,       -- null once the item was removed from its order
    catalog_item_id   uuid        NOT NULL,
    name              text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    quantity          int         NOT NULL CHECK (quantity BETWEEN 1 AND 99),
    choices           jsonb       NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(choices) = 'array'),
    status            text        NOT NULL CHECK (status IN ('sent', 'in_transit', 'received', 'defective', 'cancelled')),
    promised_date     date,       -- the line's own date; null follows its pedido de compra
    check_note        text        NOT NULL DEFAULT '' CHECK (char_length(check_note) <= 500),
    cancel_reason     text        CHECK (cancel_reason IN ('undone', 'resolved')),
    received_on       date,
    position          int         NOT NULL CHECK (position >= 0),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, supplier_order_id) REFERENCES supplier_orders (tenant_id, id),
    FOREIGN KEY (tenant_id, supplier_id) REFERENCES suppliers (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id) REFERENCES orders (tenant_id, id),
    FOREIGN KEY (tenant_id, order_id, order_item_id) REFERENCES order_items (tenant_id, order_id, id)
        ON DELETE SET NULL (order_item_id),
    CHECK ((status = 'cancelled') = (cancel_reason IS NOT NULL)),
    CHECK (supplier_order_id IS NOT NULL OR status IN ('received', 'defective')),
    CHECK ((status IN ('received', 'defective')) = (received_on IS NOT NULL)),
    CHECK (status <> 'defective' OR check_note <> '')
);
CREATE UNIQUE INDEX purchase_lines_open_item_idx ON purchase_lines (tenant_id, order_item_id)
    WHERE status IN ('sent', 'in_transit');
CREATE INDEX purchase_lines_tenant_order_idx ON purchase_lines (tenant_id, order_id);
CREATE INDEX purchase_lines_tenant_supplier_order_idx ON purchase_lines (tenant_id, supplier_order_id, position);
CREATE INDEX purchase_lines_tenant_status_idx ON purchase_lines (tenant_id, status) WHERE status IN ('sent', 'in_transit');

-- The owner's costs of a line, unit amounts: the cost at order time, copied
-- from order_item_sourcing whoever sends, and the price paid.
CREATE TABLE purchase_line_costs (
    tenant_id       uuid        NOT NULL,
    line_id         uuid        NOT NULL,
    currency        text        NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    cost_cents      bigint      CHECK (cost_cents BETWEEN 0 AND 100000000000),
    paid_cost_cents bigint      CHECK (paid_cost_cents BETWEEN 0 AND 100000000000),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, line_id),
    FOREIGN KEY (tenant_id, line_id) REFERENCES purchase_lines (tenant_id, id) ON DELETE CASCADE
);

-- One save on Receber itens.
CREATE TABLE receipts (
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id          uuid        NOT NULL,
    received_by uuid        NOT NULL,
    received_at timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id)
);

-- Each line a receipt checked; a line is received once.
CREATE TABLE receipt_lines (
    tenant_id    uuid        NOT NULL,
    receipt_id   uuid        NOT NULL,
    line_id      uuid        NOT NULL,
    check_result text        NOT NULL CHECK (check_result IN ('ok', 'defective')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, receipt_id, line_id),
    UNIQUE (tenant_id, line_id),
    FOREIGN KEY (tenant_id, receipt_id) REFERENCES receipts (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, line_id) REFERENCES purchase_lines (tenant_id, id) ON DELETE CASCADE
);

-- Item photos (with a line) and invoice photos (without), under the key the
-- API issued: t/{tenant}/receipts/{photo id}, so no row can point at another
-- store's object.
CREATE TABLE receipt_photos (
    tenant_id    uuid        NOT NULL,
    id           uuid        NOT NULL,
    receipt_id   uuid        NOT NULL,
    line_id      uuid,
    object_key   text        NOT NULL,
    content_type text        NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
    size_bytes   bigint      NOT NULL CHECK (size_bytes BETWEEN 1 AND 10485760),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    UNIQUE (object_key),
    FOREIGN KEY (tenant_id, receipt_id) REFERENCES receipts (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, receipt_id, line_id) REFERENCES receipt_lines (tenant_id, receipt_id, line_id)
        ON DELETE CASCADE,
    CHECK (object_key = 't/' || tenant_id || '/receipts/' || id)
);
CREATE INDEX receipt_photos_tenant_receipt_idx ON receipt_photos (tenant_id, receipt_id);

ALTER TABLE purchase_counters ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_counters FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON purchase_counters
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

ALTER TABLE supplier_orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE supplier_orders FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON supplier_orders
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

ALTER TABLE purchase_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON purchase_lines
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

ALTER TABLE purchase_line_costs ENABLE ROW LEVEL SECURITY;
ALTER TABLE purchase_line_costs FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON purchase_line_costs
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

ALTER TABLE receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE receipts FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON receipts
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

ALTER TABLE receipt_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE receipt_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON receipt_lines
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

ALTER TABLE receipt_photos ENABLE ROW LEVEL SECURITY;
ALTER TABLE receipt_photos FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON receipt_photos
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

GRANT SELECT, INSERT ON purchase_counters TO encomendo_app;
GRANT UPDATE (last_number, updated_at) ON purchase_counters TO encomendo_app;
GRANT SELECT, INSERT ON supplier_orders TO encomendo_app;
GRANT UPDATE (promised_date, in_transit_at, undone_at, undone_by, version, updated_at)
    ON supplier_orders TO encomendo_app;
GRANT SELECT, INSERT ON purchase_lines TO encomendo_app;
GRANT UPDATE (status, promised_date, check_note, cancel_reason, received_on, updated_at)
    ON purchase_lines TO encomendo_app;
GRANT SELECT, INSERT ON purchase_line_costs TO encomendo_app;
GRANT UPDATE (paid_cost_cents, updated_at) ON purchase_line_costs TO encomendo_app;
GRANT SELECT, INSERT ON receipts TO encomendo_app;
GRANT SELECT, INSERT ON receipt_lines TO encomendo_app;
GRANT SELECT, INSERT ON receipt_photos TO encomendo_app;

-- +goose Down
DROP TABLE receipt_photos;
DROP TABLE receipt_lines;
DROP TABLE receipts;
DROP TABLE purchase_line_costs;
DROP TABLE purchase_lines;
DROP TABLE supplier_orders;
DROP TABLE purchase_counters;
ALTER TABLE order_items DROP CONSTRAINT order_items_tenant_order_id_key;
-- A promised_date history entry cannot survive the old CHECK.
DELETE FROM order_status_history WHERE kind = 'promised_date';
ALTER TABLE order_status_history DROP CONSTRAINT order_status_history_kind_check;
ALTER TABLE order_status_history ADD CONSTRAINT order_status_history_kind_check
    CHECK (kind IN ('created', 'status', 'item', 'backup_supplier', 'payment', 'payment_voided', 'released',
        'date', 'changed', 'tracking_link', 'cancelled'));
REVOKE UPDATE (promised_date, paid_cost_cents) ON order_item_sourcing FROM encomendo_app;
ALTER TABLE order_item_sourcing DROP COLUMN paid_cost_cents, DROP COLUMN promised_date;
