-- Extensions and schema access for the application role.
--
-- The roles themselves (encomendo_owner, encomendo_app) are created outside
-- migrations: by deploy/postgres/init.sql locally and in CI, and by the
-- infrastructure in production, because creating roles needs a superuser.
-- Table grants are given table by table in later migrations; nothing here
-- grants blanket write access.

-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

REVOKE ALL ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO encomendo_app;

-- +goose Down
REVOKE USAGE ON SCHEMA public FROM encomendo_app;
GRANT USAGE ON SCHEMA public TO PUBLIC;
DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS pgcrypto;
