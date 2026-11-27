-- A store's customers and their addresses (milestone 4; ADR 0030).
--
-- A customer belongs to one store: the same person buying from two stores is
-- two rows, never merged, and a client account (milestone 7) links to orders,
-- not to these rows (docs/architecture.md section 4.2). Both tables use
-- tenant_isolation and are audited; neither has a client policy, so the
-- client area never reads a store's customer records.
--
-- Customers are never deleted in milestone 4: orders point at them.
-- Addresses are replaced as a set when a customer is saved, so they have
-- DELETE; an order keeps its own copy of the delivery address (0013), so
-- removing one never touches a past order.

-- +goose Up
CREATE TABLE customers (
    tenant_id         uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id                uuid        NOT NULL,
    name              text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120 AND name = btrim(name)),
    -- Digits for wa.me links: 55 + DDD + number, unique in the store.
    whatsapp_phone    text        NOT NULL CHECK (whatsapp_phone ~ '^55[1-9]{2}[0-9]{8,9}$'),
    email             citext      CHECK (email IS NULL OR char_length(email) BETWEEN 3 AND 254),
    notes             text        NOT NULL DEFAULT '' CHECK (char_length(notes) <= 2000),
    -- Set in milestone 7, when a logged-in client orders; encomendo_app may
    -- neither insert nor update it until then (see 0013 orders).
    client_account_id uuid        REFERENCES client_accounts (id) ON DELETE SET NULL,
    version           int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, whatsapp_phone)
);
-- Keyset pages by name (ADR 0027).
CREATE INDEX customers_tenant_name_idx ON customers (tenant_id, lower(name), id);

CREATE TABLE customer_addresses (
    tenant_id   uuid        NOT NULL,
    id          uuid        NOT NULL,
    customer_id uuid        NOT NULL,
    position    int         NOT NULL CHECK (position BETWEEN 1 AND 10),
    label       text        NOT NULL CHECK (char_length(label) BETWEEN 1 AND 40),
    cep         char(8)     NOT NULL CHECK (cep ~ '^[0-9]{8}$'),
    street      text        NOT NULL CHECK (char_length(street) BETWEEN 1 AND 200),
    number      text        NOT NULL DEFAULT '' CHECK (char_length(number) <= 20),
    complement  text        NOT NULL DEFAULT '' CHECK (char_length(complement) <= 100),
    district    text        NOT NULL DEFAULT '' CHECK (char_length(district) <= 100),
    city        text        NOT NULL CHECK (char_length(city) BETWEEN 1 AND 100),
    state       char(2)     NOT NULL CHECK (state IN ('AC', 'AL', 'AP', 'AM', 'BA', 'CE', 'DF', 'ES', 'GO',
        'MA', 'MT', 'MS', 'MG', 'PA', 'PB', 'PR', 'PE', 'PI', 'RJ', 'RN', 'RS', 'RO', 'RR', 'SC', 'SP', 'SE', 'TO')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, customer_id, position) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (tenant_id, customer_id) REFERENCES customers (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE customers ENABLE ROW LEVEL SECURITY;
ALTER TABLE customers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON customers
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE customer_addresses ENABLE ROW LEVEL SECURITY;
ALTER TABLE customer_addresses FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON customer_addresses
    USING (tenant_id = current_tenant_id())
    WITH CHECK (tenant_id = current_tenant_id());

CREATE TRIGGER customers_audit AFTER INSERT OR UPDATE OR DELETE ON customers
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER customer_addresses_audit AFTER INSERT OR UPDATE OR DELETE ON customer_addresses
    FOR EACH ROW EXECUTE FUNCTION audit_row();

GRANT SELECT ON customers TO encomendo_app;
GRANT INSERT (tenant_id, id, name, whatsapp_phone, email, notes, version, created_at, updated_at)
    ON customers TO encomendo_app;
GRANT UPDATE (name, whatsapp_phone, email, notes, version, updated_at) ON customers TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON customer_addresses TO encomendo_app;
GRANT UPDATE (position, label, cep, street, number, complement, district, city, state, updated_at)
    ON customer_addresses TO encomendo_app;

-- +goose Down
DROP TABLE customer_addresses;
DROP TABLE customers;
