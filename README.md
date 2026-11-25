# Encomendo

Venda primeiro. Compre o necessário. Entregue com confiança.

SaaS for small Brazilian businesses that sell made-to-order products: the client builds an order on the store page, the business buys or makes only what was sold, and everyone knows when it will be ready.

| Path | What |
| --- | --- |
| `backend/` | Go modular monolith (ports and adapters), Postgres with RLS, Redis |
| `web/` | Vue 3 workspace: `apps/admin` (management) and `apps/store` (store pages and client area) |
| `mobile/` | Kotlin Multiplatform management app, Android only for now |
| `deploy/` | Docker Compose stack, Dockerfiles, Postgres init |
| `docs/` | Architecture, product docs, ADRs, OpenAPI contract |

Start with `docs/architecture.md` and `CLAUDE.md`.

## Requirements

- Go 1.27+, Docker with Compose
- Node 22+ and pnpm 10 (`corepack enable`)
- JDK 21 and the Android SDK (API 36) for `mobile/`

## First time

```bash
cd backend && go mod tidy && cd ..   # creates backend/go.sum; commit it
make setup                           # .env from .env.example + web deps
make dev-up                          # postgres, redis, migrations, api, web
```

Then open:

- Admin: http://admin.localhost:8080
- A store page: http://altarpm.localhost:8080 (any `<slug>.localhost`)
- API readiness: http://localhost:8081/readyz

## Everyday commands

```bash
make test               # backend, web and mobile unit tests
make test-integration   # backend integration tests (Docker)
make lint               # every linter
make coverage           # backend coverage gate
make api-types          # regenerate web API types after editing docs/api/openapi.yaml
```

Per app:

- Backend: `make -C backend test`, `make -C backend run` (export the variables from `.env` first: `set -a; . ./.env; set +a`)
- Web: `cd web && pnpm --filter @encomendo/admin dev` (http://localhost:5173), `pnpm --filter @encomendo/store dev` (http://altarpm.localhost:5174)
- Android: `cd mobile && ./gradlew check :androidApp:assembleDebug`

## Parallel work

Several Claude Code sessions can work at once, each in its own worktree (`claude --worktree <name>`). Give each worktree its own `COMPOSE_PROJECT_NAME` and ports in `.env`. See "Working in parallel sessions" in `CLAUDE.md`.
