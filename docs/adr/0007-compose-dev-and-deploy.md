# 0007 — Docker Compose for development and deployment, all under `deploy/`

- Status: accepted
- Date: 2026-11-24

## Context

The project runs on one host to start with. Development needs hot reload and parallel worktrees; deployment needs pinned images, secrets from the environment and TLS. `deploy/docker-compose.yml` already built images for CI and e2e, but there was no hot-reload setup and no production file.

## Decision

Everything lives in `deploy/`, with one base file and two ways to use it:

| File | Use |
| --- | --- |
| `docker-compose.yml` | Base: postgres, redis, migrate, api, web (nginx with the built apps). What CI and e2e run (`make stack-up`). |
| `docker-compose.dev.yml` | Overlay for development (`make dev-up`): `api` rebuilt on change, Vite dev servers for admin and store with hot reload, nginx off. Uses `docker compose watch`; no extra tool such as air. |
| `docker-compose.prod.yml` | Standalone production stack. Images from GHCR by tag, secrets from `deploy/.env.prod`, only Caddy published, Postgres roles created from environment secrets (`postgres/init-prod.sh`). |

- One nginx config for every environment: `web/nginx.conf.template` takes `ADMIN_HOST` at container start.
- Development and production share `postgres/database.sqlinc`, so grants cannot drift between them.
- Caddy terminates TLS with per-host certificates (HTTP-01) for the admin and API hosts.

## Consequences

- Wildcard certificates for `{slug}.encomendo.com.br` need a DNS-01 challenge and a Caddy build with the DNS provider's module. That belongs to milestone 7 and depends on the open question about custom domains.
- `release.yml` has a `migrate` job that runs against `MIGRATE_DATABASE_URL`, which assumes a database the GitHub runner can reach. With this stack Postgres is on the private network and the `migrate` service runs on every `up`, so that job should be replaced by a deploy step once the host is chosen. Not changed here.
- One host means no zero-downtime deploys and no database failover. Revisit when that matters.
