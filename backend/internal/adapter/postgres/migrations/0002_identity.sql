-- Identity: staff users, client accounts, their login codes, refresh tokens
-- and password reset codes.
--
-- These tables are global, not tenant-scoped, so they have no RLS: a staff
-- user belongs to many stores, a client account reads orders from every
-- store, and login, refresh and password reset find their row by email or
-- token hash before any store is known (ADR 0010, ADR 0011). Only
-- app/identity reads them. Secrets are stored only as hashes.

-- +goose Up
CREATE TABLE users (
    id            uuid        PRIMARY KEY,                 -- UUIDv7, generated in Go
    email         citext      NOT NULL UNIQUE CHECK (char_length(email) BETWEEN 3 AND 254),
    name          text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    password_hash text        NOT NULL,                    -- argon2id, PHC string
    version       int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Client accounts (milestone 7) are optional and passwordless: they log in
-- with an emailed code or with Google (docs/architecture.md 5.3).
CREATE TABLE client_accounts (
    id         uuid        PRIMARY KEY,
    name       text        CHECK (char_length(name) BETWEEN 1 AND 100),
    version    int         NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- One way to log in to a client account: a verified email or a Google sub.
CREATE TABLE client_identities (
    id                uuid        PRIMARY KEY,
    client_account_id uuid        NOT NULL REFERENCES client_accounts (id) ON DELETE CASCADE,
    email             citext      UNIQUE CHECK (char_length(email) BETWEEN 3 AND 254),
    google_sub        text        UNIQUE CHECK (char_length(google_sub) BETWEEN 1 AND 255),
    created_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(email, google_sub) = 1)
);

CREATE INDEX client_identities_account_idx ON client_identities (client_account_id);

-- One-time login codes, sent to an email that may not have an account yet.
CREATE TABLE client_login_codes (
    id         uuid        PRIMARY KEY,
    email      citext      NOT NULL CHECK (char_length(email) BETWEEN 3 AND 254),
    code_hash  bytea       NOT NULL CHECK (octet_length(code_hash) = 32), -- HMAC-SHA256
    expires_at timestamptz NOT NULL,
    attempts   int         NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX client_login_codes_email_idx ON client_login_codes (email, created_at DESC);

-- Refresh tokens rotate on every use. A family is one login; presenting a
-- token that was already used revokes the family, except for a retry within
-- 2 minutes while its successor (replaced_by) is unused (ADR 0013).
-- tenant_id is the store the token was issued for; its foreign key is added
-- by 0004_tenancy, which creates tenants.
CREATE TABLE refresh_tokens (
    id                uuid        PRIMARY KEY,
    family_id         uuid        NOT NULL,
    user_id           uuid        REFERENCES users (id) ON DELETE CASCADE,
    client_account_id uuid        REFERENCES client_accounts (id) ON DELETE CASCADE,
    tenant_id         uuid,
    token_hash        bytea       NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32), -- SHA-256
    expires_at        timestamptz NOT NULL,
    used_at           timestamptz,
    replaced_by       uuid        REFERENCES refresh_tokens (id) ON DELETE SET NULL,
    revoked_at        timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(user_id, client_account_id) = 1),
    CHECK (client_account_id IS NULL OR tenant_id IS NULL), -- a client session never has a store
    CHECK (replaced_by IS NULL OR used_at IS NOT NULL)
);

CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id) WHERE user_id IS NOT NULL;
CREATE INDEX refresh_tokens_client_account_idx ON refresh_tokens (client_account_id) WHERE client_account_id IS NOT NULL;
CREATE INDEX refresh_tokens_expires_idx ON refresh_tokens (expires_at);

-- "Esqueci a senha" codes (ADR 0011): 6 digits, stored as an HMAC-SHA256
-- under a server key, 10 minutes, 5 wrong attempts. Only the newest code of
-- a user counts, so a new request replaces the old one without an update.
-- Old rows stay for a day: the per-account budget of 10 wrong codes per 24
-- hours sums attempts over them.
CREATE TABLE password_reset_codes (
    id         uuid        PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash  bytea       NOT NULL CHECK (octet_length(code_hash) = 32),
    expires_at timestamptz NOT NULL,
    attempts   int         NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX password_reset_codes_user_idx ON password_reset_codes (user_id, created_at DESC);

-- No DELETE on users in milestone 2: staff accounts are never deleted yet.
GRANT SELECT, INSERT, UPDATE ON users TO encomendo_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON client_accounts TO encomendo_app;
GRANT SELECT, INSERT, DELETE ON client_identities TO encomendo_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON client_login_codes TO encomendo_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON refresh_tokens TO encomendo_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON password_reset_codes TO encomendo_app;

-- +goose Down
DROP TABLE password_reset_codes;
DROP TABLE refresh_tokens;
DROP TABLE client_login_codes;
DROP TABLE client_identities;
DROP TABLE client_accounts;
DROP TABLE users;
