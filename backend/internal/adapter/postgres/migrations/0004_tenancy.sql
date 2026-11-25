-- Tenancy: stores, their team, their settings and invitations to join them.
--
-- Every table here is tenant-scoped. RLS, its policies and the grants to
-- encomendo_app come in 0007_rls_policies, after the helper functions of
-- 0005, so no table is ever granted without RLS (ADR 0014).

-- +goose Up
CREATE TABLE tenants (
    id         uuid        PRIMARY KEY,
    -- The store page address, {slug}.encomendo.com.br. The Go domain also
    -- refuses the labels of the configured admin and API hosts (ADR 0011);
    -- this check keeps the fixed list out even if a bug skips it.
    slug       text        NOT NULL UNIQUE
        CHECK (char_length(slug) BETWEEN 3 AND 40
           AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'
           AND slug NOT IN ('www', 'api', 'admin', 'app', 'conta', 'mail', 'static')),
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    niche      text        CHECK (niche ~ '^[a-z][a-z0-9_]{1,62}$'), -- template key, chosen from milestone 7
    status     text        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    version    int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- A refresh token keeps the store it was issued for; a deleted store leaves
-- the session without one (ADR 0010).
ALTER TABLE refresh_tokens
    ADD CONSTRAINT refresh_tokens_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE SET NULL;

-- "Equipe": who belongs to a store and with which role. A store has exactly
-- one owner: the index allows at most one, and app/tenancy never removes or
-- demotes it (ADR 0011).
CREATE TABLE memberships (
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       text        NOT NULL CHECK (role IN ('owner', 'admin', 'staff', 'viewer')),
    version    int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id)
);

CREATE UNIQUE INDEX memberships_one_owner ON memberships (tenant_id) WHERE role = 'owner';
-- Not led by tenant_id on purpose: the store picker reads a user's
-- memberships with no tenant set (ADR 0010).
CREATE INDEX memberships_user_idx ON memberships (user_id);

CREATE TABLE tenant_settings (
    tenant_id               uuid        PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    -- The workflow new orders take (ADR 0004). Never null: a store cannot
    -- exist without one (ADR 0006).
    status_workflow_id      uuid        NOT NULL,
    status_workflow_version int         NOT NULL,
    stock_enabled           boolean     NOT NULL DEFAULT false,
    production_enabled      boolean     NOT NULL DEFAULT false,
    purchase_before_deposit boolean     NOT NULL DEFAULT false,
    deposit_percent         int         NOT NULL DEFAULT 50 CHECK (deposit_percent BETWEEN 0 AND 100),
    assembly_days           int         NOT NULL DEFAULT 3 CHECK (assembly_days BETWEEN 0 AND 365),
    safety_margin_days      int         NOT NULL DEFAULT 2 CHECK (safety_margin_days BETWEEN 0 AND 365),
    currency                char(3)     NOT NULL DEFAULT 'BRL' CHECK (currency ~ '^[A-Z]{3}$'),
    whatsapp_phone          text        CHECK (whatsapp_phone ~ '^55[1-9]{2}[0-9]{8,9}$'), -- digits for wa.me
    version                 int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (status_workflow_id, status_workflow_version) REFERENCES status_workflows (id, version)
);

-- "Convite" (ADR 0011). The token is the tenant id plus 32 random bytes;
-- only the SHA-256 of the random part is stored, and accepting looks it up
-- under the tenant inside the token, so no read crosses stores. A store has
-- at most one pending invitation per email: inviting again replaces it with
-- a new token and expiry.
CREATE TABLE invitations (
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    id          uuid        NOT NULL,
    email       citext      NOT NULL CHECK (char_length(email) BETWEEN 3 AND 254),
    role        text        NOT NULL CHECK (role IN ('admin', 'staff', 'viewer')), -- never owner
    token_hash  bytea       NOT NULL CHECK (octet_length(token_hash) = 32),
    -- The sender's account. Their membership may go; removing or demoting
    -- them revokes what they sent, and accepting re-checks their role.
    invited_by  uuid        NOT NULL REFERENCES users (id),
    expires_at  timestamptz NOT NULL,
    accepted_at timestamptz,
    revoked_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, token_hash),
    CHECK (num_nonnulls(accepted_at, revoked_at) <= 1)
);

CREATE UNIQUE INDEX invitations_one_pending ON invitations (tenant_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;
CREATE INDEX invitations_tenant_created_idx ON invitations (tenant_id, created_at DESC, id DESC);
CREATE INDEX invitations_tenant_sender_idx ON invitations (tenant_id, invited_by)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

-- +goose Down
DROP TABLE invitations;
DROP TABLE tenant_settings;
DROP TABLE memberships;
ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_tenant_id_fkey;
DROP TABLE tenants;
