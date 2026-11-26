# deploy/

Docker Compose for development and production. Decision: `docs/adr/0007-compose-dev-and-deploy.md`.

| Path | What it is |
| --- | --- |
| `docker-compose.yml` | Base stack (built images). CI and e2e use it as is. |
| `docker-compose.dev.yml` | Development overlay: hot reload. |
| `docker-compose.prod.yml` | Production stack for one host. |
| `api.Dockerfile`, `web.Dockerfile` | Production images (built by `release.yml` on a `v*` tag). |
| `web.dev.Dockerfile` | Vite dev servers for development. |
| `web/nginx.conf.template` | nginx config for the web image; `ADMIN_HOST` is filled in at start. |
| `caddy/Caddyfile` | TLS and reverse proxy in production. |
| `postgres/` | `init.sql` (dev roles), `init-prod.sh` (prod roles from env), `database.sqlinc` (shared grants). |
| `prod.env.example` | Template for `deploy/.env.prod`. |

## Development

```bash
make setup      # .env and web dependencies, once
make dev-up     # postgres, redis, migrate, api, Vite admin and store, with hot reload
```

- Admin: http://localhost:5173 · Store: http://altarpm.localhost:5174 · API: http://localhost:8081
- Edit `backend/` and the api rebuilds; edit `web/` and Vite reloads. Changing `web/pnpm-lock.yaml` rebuilds the web images.
- `make stack-up` starts the built-image stack behind nginx instead (admin at `admin.localhost:8080`), as CI does.
- Several worktrees at once: `scripts/worktree-env.sh <slot 1-9>` gives each its own project name and ports.

## Production

On the host (Docker with Compose v2), once:

```bash
cp deploy/prod.env.example deploy/.env.prod   # then fill it in; openssl rand -hex 24 makes a secret
```

Point `ADMIN_HOST` and `API_HOST` DNS records at the host and open ports 80 and 443. Deploy a release:

```bash
# set VERSION in deploy/.env.prod to the tag without the v (v1.2.3 -> 1.2.3)
docker compose --env-file deploy/.env.prod -f deploy/docker-compose.prod.yml pull
docker compose --env-file deploy/.env.prod -f deploy/docker-compose.prod.yml up -d
```

`up` runs the `migrate` service first; the API starts only after it succeeds. `make prod-config` checks the file against your env file.

- The Postgres roles are created from the env file only when the data volume is empty. Changing a password later means `ALTER ROLE` by hand and updating the env file.
- Backups are not set up yet (milestone 8: backup and restore drill).
- Store pages on `{slug}.encomendo.com.br` need wildcard TLS, which comes with milestone 7.

### Client addresses and IPv6

The API trusts `X-Forwarded-For` only from Caddy and nginx, by their fixed addresses on the compose network (`172.30.0.10` and `.11`, `fd00:30::10` and `::11`; ADR 0021). The production network has IPv6 on, so an IPv6 client reaches Caddy with its own address. Otherwise Docker's userland proxy would hand Caddy the bridge gateway's address for every IPv6 client, and they would all share one rate-limit key.

For that, the host's Docker needs IPv6 with ip6tables, in `/etc/docker/daemon.json`:

```json
{ "ipv6": true, "ip6tables": true }
```

If the API ever logs "a client resolved to a private address", clients are arriving under an internal address, such as the bridge gateway's: check this setting and `TRUSTED_PROXIES`.

### Secrets

`deploy/.env.prod` holds `AUTH_SIGNING_SEED` and `AUTH_RESET_CODE_KEY` (`openssl rand -base64 32` each). The API refuses the development values outside development (ADRs 0011 and 0019). To rotate the signing key, follow ADR 0019: move the old seed into `AUTH_VERIFY_SEEDS`, deploy, then drop it after 15 minutes and 30 seconds.
