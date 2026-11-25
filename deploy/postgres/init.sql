-- Creates the database and its roles for local development and CI.
--
-- LOCAL AND CI ONLY. These passwords are public on purpose. Production roles
-- are created by the infrastructure with secrets from the secret store.
--
-- encomendo_owner  owns the tables and runs migrations (cmd/migrate)
-- encomendo_app    the API: no superuser, no BYPASSRLS, no DDL (docs/architecture.md 5.5)

CREATE ROLE encomendo_owner LOGIN PASSWORD 'encomendo_owner_dev';
CREATE ROLE encomendo_app LOGIN PASSWORD 'encomendo_app_dev'
  NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOINHERIT;

\ir database.sqlinc
