-- Catálogo: categories, item fields ("Serve em") and their values, items,
-- customization options, kits with their items and swap alternatives,
-- photos, and the in-house items' own costs (milestone 3; ADR 0024, ADR 0025).
-- Each table carries its RLS policy and grants here (ADR 0014).
--
-- Two policy forms:
--   - tenant_isolation, as every tenant table: the store page (milestone 7)
--     reads published items and kits from these tables;
--   - staff_only on item_costs: a row is visible only with a tenant AND a
--     staff user set, so a store page scope (tenant, no user) reads zero
--     rows whatever its query (ADR 0024).
--
-- Nothing in the catalog is deleted: items and kits are archived (ADR 0025),
-- so encomendo_app has no DELETE on them. Child rows (values, options,
-- kit items, alternatives, photos, links) are replaced as their aggregate is
-- saved, so those have DELETE.
--
-- Rows hanging on an item or a kit cascade with it. The app can never
-- delete an item or a kit, so the cascade runs only when a store is
-- deleted (tenants ON DELETE CASCADE). The item's category is the exception:
-- deleting a category in use must fail (category_in_use).
--
-- Audit: categories, attributes, items and kits are audited. item_costs is
-- not: audit_log keeps whole rows and a store page scope can read audit_log,
-- so auditing a staff-only table would copy costs where the store page can
-- see them. Staff-only tables join the audit once audit_log reads are
-- staff-only too (audit views, milestone 8).

-- +goose Up
CREATE TABLE categories (
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id         uuid        NOT NULL,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120 AND name = btrim(name)),
    position   int         NOT NULL CHECK (position >= 0),
    version    int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id)
);
-- Names are unique per store ignoring case (name_taken).
CREATE UNIQUE INDEX categories_tenant_name_key ON categories (tenant_id, lower(name));

CREATE TABLE attributes (
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id         uuid        NOT NULL,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120 AND name = btrim(name)),
    position   int         NOT NULL CHECK (position >= 0),
    version    int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX attributes_tenant_name_key ON attributes (tenant_id, lower(name));

-- Values keep their ids across edits, because items and kits point at them.
-- Labels are unique within the field in the domain: the field is saved as
-- one aggregate, and an index here would refuse a swap of two labels.
CREATE TABLE attribute_values (
    tenant_id    uuid        NOT NULL,
    id           uuid        NOT NULL,
    attribute_id uuid        NOT NULL,
    label        text        NOT NULL CHECK (char_length(label) BETWEEN 1 AND 80 AND label = btrim(label)),
    position     int         NOT NULL CHECK (position >= 0),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, attribute_id) REFERENCES attributes (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX attribute_values_tenant_attribute_idx ON attribute_values (tenant_id, attribute_id, position);

CREATE TABLE items (
    tenant_id       uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id              uuid        NOT NULL,
    name            text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120 AND name = btrim(name)),
    description     text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    -- Null is "Sem categoria". A category in use cannot be deleted (23503 →
    -- category_in_use).
    category_id     uuid,
    sourcing_mode   text        NOT NULL CHECK (sourcing_mode IN ('to_order', 'stock', 'in_house')),
    price_cents     bigint      NOT NULL CHECK (price_cents BETWEEN 1 AND 100000000000),
    currency        char(3)     NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    production_days int         CHECK (production_days BETWEEN 1 AND 365),
    -- The days the item needs before it is ready: the main supplier's lead
    -- time, or production_days for in-house items, kept by the catalog
    -- repository with the item (ADR 0025). It holds no supplier and no cost,
    -- so the store page can compute dates without reading staff-only tables.
    ready_days      int         NOT NULL CHECK (ready_days BETWEEN 0 AND 365),
    published       boolean     NOT NULL DEFAULT false,
    archived_at     timestamptz,
    version         int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, category_id) REFERENCES categories (tenant_id, id),
    CHECK ((sourcing_mode = 'in_house') = (production_days IS NOT NULL)),
    CHECK (NOT (published AND archived_at IS NOT NULL))
);
-- Keyset pages by name, then id (the Itens list), and the category filter.
CREATE INDEX items_tenant_name_idx ON items (tenant_id, name, id);
CREATE INDEX items_tenant_category_idx ON items (tenant_id, category_id);

CREATE TABLE item_attribute_values (
    tenant_id  uuid        NOT NULL,
    item_id    uuid        NOT NULL,
    value_id   uuid        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, item_id, value_id),
    FOREIGN KEY (tenant_id, item_id) REFERENCES items (tenant_id, id) ON DELETE CASCADE,
    -- Removing a value removes it from every item (ADR 0025).
    FOREIGN KEY (tenant_id, value_id) REFERENCES attribute_values (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX item_attribute_values_tenant_value_idx ON item_attribute_values (tenant_id, value_id);

-- choices holds only the shape ADR 0024 defines, validated in
-- domain/catalog: [{"id", "label", "extraPriceCents", "extraDays"}].
CREATE TABLE customization_options (
    tenant_id  uuid        NOT NULL,
    id         uuid        NOT NULL,
    item_id    uuid        NOT NULL,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80 AND name = btrim(name)),
    required   boolean     NOT NULL,
    position   int         NOT NULL CHECK (position >= 0),
    choices    jsonb       NOT NULL CHECK (jsonb_typeof(choices) = 'array' AND jsonb_array_length(choices) BETWEEN 1 AND 50),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, item_id) REFERENCES items (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX customization_options_tenant_item_idx ON customization_options (tenant_id, item_id, position);

-- The own cost of an in-house item, the owner's only (ADR 0025). Suppliers'
-- costs live in supplier_items (0009). Staff-only.
CREATE TABLE item_costs (
    tenant_id  uuid        NOT NULL,
    item_id    uuid        NOT NULL,
    cost_cents bigint      NOT NULL CHECK (cost_cents BETWEEN 0 AND 100000000000),
    currency   char(3)     NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, item_id),
    FOREIGN KEY (tenant_id, item_id) REFERENCES items (tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE kits (
    tenant_id          uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id                 uuid        NOT NULL,
    name               text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120 AND name = btrim(name)),
    description        text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    price_cents        bigint      NOT NULL CHECK (price_cents BETWEEN 1 AND 100000000000),
    currency           char(3)     NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    -- What a client's swap does to the price (ADR 0023): keep the discount,
    -- the default, or lose it.
    swap_discount      text        NOT NULL DEFAULT 'keep' CHECK (swap_discount IN ('keep', 'lose')),
    -- Null uses the store's default (tenant_settings).
    assembly_days      int         CHECK (assembly_days BETWEEN 0 AND 60),
    safety_margin_days int         CHECK (safety_margin_days BETWEEN 0 AND 60),
    published          boolean     NOT NULL DEFAULT false,
    archived_at        timestamptz,
    version            int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    CHECK (NOT (published AND archived_at IS NOT NULL))
);
CREATE INDEX kits_tenant_name_idx ON kits (tenant_id, name, id);

-- An item appears once per kit.
CREATE TABLE kit_items (
    tenant_id  uuid        NOT NULL,
    kit_id     uuid        NOT NULL,
    item_id    uuid        NOT NULL,
    quantity   int         NOT NULL CHECK (quantity BETWEEN 1 AND 99),
    swappable  boolean     NOT NULL,
    position   int         NOT NULL CHECK (position >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, kit_id, item_id),
    FOREIGN KEY (tenant_id, kit_id) REFERENCES kits (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, item_id) REFERENCES items (tenant_id, id) ON DELETE CASCADE
);
-- "Kits that hold this item", for archiving (item_in_kit).
CREATE INDEX kit_items_tenant_item_idx ON kit_items (tenant_id, item_id);

-- The items a swappable kit item may be swapped for ("Pode trocar por").
CREATE TABLE kit_item_alternatives (
    tenant_id           uuid        NOT NULL,
    kit_id              uuid        NOT NULL,
    item_id             uuid        NOT NULL,
    alternative_item_id uuid        NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, kit_id, item_id, alternative_item_id),
    FOREIGN KEY (tenant_id, kit_id, item_id) REFERENCES kit_items (tenant_id, kit_id, item_id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, alternative_item_id) REFERENCES items (tenant_id, id) ON DELETE CASCADE,
    CHECK (alternative_item_id <> item_id)
);
CREATE INDEX kit_item_alternatives_tenant_alternative_idx ON kit_item_alternatives (tenant_id, alternative_item_id);

CREATE TABLE kit_attribute_values (
    tenant_id  uuid        NOT NULL,
    kit_id     uuid        NOT NULL,
    value_id   uuid        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, kit_id, value_id),
    FOREIGN KEY (tenant_id, kit_id) REFERENCES kits (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, value_id) REFERENCES attribute_values (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX kit_attribute_values_tenant_value_idx ON kit_attribute_values (tenant_id, value_id);

-- A photo of an item or a kit. The object key is the one the API issued,
-- t/{tenant}/{items|kits}/{owner}/{photo id} (ADR 0025): the check below holds
-- it to its own tenant, owner and id, so no row can point at another
-- store's object, and the primary key makes each key single-use.
CREATE TABLE photos (
    tenant_id    uuid        NOT NULL,
    id           uuid        NOT NULL,
    item_id      uuid,
    kit_id       uuid,
    object_key   text        NOT NULL,
    content_type text        NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
    size_bytes   bigint      NOT NULL CHECK (size_bytes BETWEEN 1 AND 10485760),
    position     int         NOT NULL CHECK (position BETWEEN 0 AND 7),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    UNIQUE (object_key),
    FOREIGN KEY (tenant_id, item_id) REFERENCES items (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, kit_id) REFERENCES kits (tenant_id, id) ON DELETE CASCADE,
    CHECK (num_nonnulls(item_id, kit_id) = 1),
    CHECK (object_key = 't/' || tenant_id || '/'
        || CASE WHEN item_id IS NOT NULL THEN 'items/' || item_id ELSE 'kits/' || kit_id END
        || '/' || id)
);
CREATE INDEX photos_tenant_item_idx ON photos (tenant_id, item_id, position);
CREATE INDEX photos_tenant_kit_idx ON photos (tenant_id, kit_id, position);

ALTER TABLE categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE categories FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON categories
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE attributes ENABLE ROW LEVEL SECURITY;
ALTER TABLE attributes FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON attributes
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE attribute_values ENABLE ROW LEVEL SECURITY;
ALTER TABLE attribute_values FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON attribute_values
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE items ENABLE ROW LEVEL SECURITY;
ALTER TABLE items FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON items
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE item_attribute_values ENABLE ROW LEVEL SECURITY;
ALTER TABLE item_attribute_values FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON item_attribute_values
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE customization_options ENABLE ROW LEVEL SECURITY;
ALTER TABLE customization_options FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON customization_options
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE item_costs ENABLE ROW LEVEL SECURITY;
ALTER TABLE item_costs FORCE ROW LEVEL SECURITY;
CREATE POLICY staff_only ON item_costs
    USING (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL)
    WITH CHECK (tenant_id = current_tenant_id() AND current_user_id() IS NOT NULL);

ALTER TABLE kits ENABLE ROW LEVEL SECURITY;
ALTER TABLE kits FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON kits
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE kit_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE kit_items FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON kit_items
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE kit_item_alternatives ENABLE ROW LEVEL SECURITY;
ALTER TABLE kit_item_alternatives FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON kit_item_alternatives
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE kit_attribute_values ENABLE ROW LEVEL SECURITY;
ALTER TABLE kit_attribute_values FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON kit_attribute_values
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE photos ENABLE ROW LEVEL SECURITY;
ALTER TABLE photos FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON photos
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

CREATE TRIGGER categories_audit AFTER INSERT OR UPDATE OR DELETE ON categories
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER attributes_audit AFTER INSERT OR UPDATE OR DELETE ON attributes
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER items_audit AFTER INSERT OR UPDATE OR DELETE ON items
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER kits_audit AFTER INSERT OR UPDATE OR DELETE ON kits
    FOR EACH ROW EXECUTE FUNCTION audit_row();

-- Least privilege. UPDATE only on the columns the use cases change, never
-- tenant_id or id, so no row can move to another store.
GRANT SELECT, INSERT, DELETE ON categories TO encomendo_app;
GRANT UPDATE (name, position, version, updated_at) ON categories TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON attributes TO encomendo_app;
GRANT UPDATE (name, position, version, updated_at) ON attributes TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON attribute_values TO encomendo_app;
GRANT UPDATE (label, position, updated_at) ON attribute_values TO encomendo_app;
GRANT SELECT, INSERT ON items TO encomendo_app;
GRANT UPDATE (name, description, category_id, sourcing_mode, price_cents, production_days, ready_days,
    published, archived_at, version, updated_at) ON items TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON item_attribute_values TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON customization_options TO encomendo_app;
GRANT UPDATE (name, required, position, choices, updated_at) ON customization_options TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON item_costs TO encomendo_app;
GRANT UPDATE (cost_cents, updated_at) ON item_costs TO encomendo_app;
GRANT SELECT, INSERT ON kits TO encomendo_app;
GRANT UPDATE (name, description, price_cents, swap_discount, assembly_days, safety_margin_days,
    published, archived_at, version, updated_at) ON kits TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON kit_items TO encomendo_app;
GRANT UPDATE (quantity, swappable, position, updated_at) ON kit_items TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON kit_item_alternatives TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON kit_attribute_values TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON photos TO encomendo_app;
GRANT UPDATE (position, updated_at) ON photos TO encomendo_app;

-- +goose Down
DROP TABLE photos;
DROP TABLE kit_attribute_values;
DROP TABLE kit_item_alternatives;
DROP TABLE kit_items;
DROP TABLE kits;
DROP TABLE item_costs;
DROP TABLE customization_options;
DROP TABLE item_attribute_values;
DROP TABLE items;
DROP TABLE attribute_values;
DROP TABLE attributes;
DROP TABLE categories;
