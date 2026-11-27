#!/usr/bin/env bash
# Gives this worktree its own Docker project name and host ports, so several
# worktrees can run `make dev-up` at once.
# Usage: scripts/worktree-env.sh <slot>   (slot 0..9; main checkout uses 0)
set -euo pipefail

slot="${1:?usage: scripts/worktree-env.sh <slot 0-9>}"
case "$slot" in [0-9]) ;; *) echo "slot must be a single digit" >&2; exit 1 ;; esac

root="$(git rev-parse --show-toplevel)"
name="$(basename "$root")"
[ -f "$root/.env" ] || cp "$root/.env.example" "$root/.env"

pg=$((5432 + slot * 10)); rd=$((6379 + slot * 10)); api=$((8081 + slot * 10)); web=$((8080 + slot * 10))
adm=$((5173 + slot * 10)); sto=$((5174 + slot * 10))

set_var() { # set_var KEY VALUE: replace the line or append it
  if grep -q "^$1=" "$root/.env"; then sed -i "s|^$1=.*|$1=$2|" "$root/.env"; else echo "$1=$2" >> "$root/.env"; fi
}

set_var COMPOSE_PROJECT_NAME "enc-${name//[^a-zA-Z0-9]/-}"
set_var POSTGRES_PORT "$pg"
set_var REDIS_PORT "$rd"
set_var API_PORT "$api"
set_var WEB_PORT "$web"
set_var ADMIN_PORT "$adm"
set_var STORE_PORT "$sto"
set_var MAILPIT_PORT "$((8025 + slot * 10))"
set_var MAILPIT_SMTP_PORT "$((1025 + slot * 10))"
set_var SMTP_ADDR "localhost:$((1025 + slot * 10))"
set_var ADMIN_ORIGIN "http://localhost:${adm}"
set_var DATABASE_URL "postgres://encomendo_app:encomendo_app_dev@localhost:${pg}/encomendo?sslmode=disable"
set_var MIGRATE_DATABASE_URL "postgres://encomendo_owner:encomendo_owner_dev@localhost:${pg}/encomendo?sslmode=disable"
set_var REDIS_URL "redis://localhost:${rd}/0"
set_var MINIO_PORT "$((9000 + slot * 10))"
set_var STORAGE_ENDPOINT "localhost:$((9000 + slot * 10))"
set_var STORAGE_PUBLIC_ENDPOINT "localhost:$((9000 + slot * 10))"

echo "worktree $name: postgres=$pg redis=$rd api=$api web=$web admin=$adm store=$sto"
