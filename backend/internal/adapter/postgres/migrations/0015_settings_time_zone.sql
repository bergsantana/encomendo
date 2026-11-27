-- The store's time zone (milestone 4; ADR 0030): it decides which day
-- "today" is for dates and alerts. One zone per Brazilian offset; a store
-- starts on Brasília time.

-- +goose Up
ALTER TABLE tenant_settings ADD COLUMN time_zone text NOT NULL DEFAULT 'America/Sao_Paulo'
    CHECK (time_zone IN ('America/Noronha', 'America/Sao_Paulo', 'America/Manaus', 'America/Rio_Branco'));

GRANT UPDATE (time_zone) ON tenant_settings TO encomendo_app;

-- +goose Down
REVOKE UPDATE (time_zone) ON tenant_settings FROM encomendo_app;
ALTER TABLE tenant_settings DROP COLUMN time_zone;
