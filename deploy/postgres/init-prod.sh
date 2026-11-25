#!/bin/sh
# Production first-boot setup: creates the two roles with passwords from the
# environment, then the database and grants shared with development.
# Runs once, when the data volume is empty (Postgres entrypoint rule).
set -eu

: "${ENCOMENDO_OWNER_PASSWORD:?set ENCOMENDO_OWNER_PASSWORD}"
: "${ENCOMENDO_APP_PASSWORD:?set ENCOMENDO_APP_PASSWORD}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
  -v owner_pw="$ENCOMENDO_OWNER_PASSWORD" -v app_pw="$ENCOMENDO_APP_PASSWORD" <<'SQL'
CREATE ROLE encomendo_owner LOGIN PASSWORD :'owner_pw';
CREATE ROLE encomendo_app LOGIN PASSWORD :'app_pw'
  NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOINHERIT;

\ir /docker-entrypoint-initdb.d/database.sqlinc
SQL
