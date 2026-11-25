# Running Claude sessions in parallel

How to split the build plan across worktree sessions without collisions. `docs/build-plan.md` tracks status; this file tracks **who works on what, in which order**.

## Rules

- One session = one worktree = one branch = one PR. Start sessions with `claude --worktree <name>`; branch from an up-to-date `main`.
- A session edits only the area its prompt names. It does not touch the files with one owner per milestone (`CLAUDE.md`, "Working in parallel sessions"), except the session that owns them in the table below.
- A session that needs a change in a file it does not own writes the change in its PR description. The integration session applies it.
- Sessions on a wave start only after the previous wave is merged. Sessions inside a wave never depend on each other's unmerged code.
- Sessions need no running stack: integration tests use testcontainers on random ports. Only a session that runs `make dev-up` needs `scripts/worktree-env.sh <slot>` first (slot 1 to 9); it gives the worktree its own project name and ports, including the Vite ports.
- Every PR: `make lint test` (plus `make test-integration` for adapters), the matching reviewer agent, and its build-plan boxes ticked.

## Milestone 2 — Identity and tenancy

| Wave | Session | Owns | Scope |
| --- | --- | --- | --- |
| 1 | `m2-contract` | `docs/api/openapi.yaml` | `domain/shared` (typed IDs, `Money`, `Page`, `Principal`), `app/shared` ports (`TxManager`, `Cache`, `Clock`, `IDGenerator`, `EventPublisher`, `authz.Require`), OpenAPI for `/auth/*` and tenancy, `make api-types`. No adapters. |
| 2 | `m2-db` | `migrations/` | `0002`, `0003` (ADR 0006), `0004`, `0005`, `0014`, `0015`; Postgres `TxManager` that sets `app.tenant_id`; RLS test suite. |
| 2 | `m2-domain` | none | `domain/identity`, `domain/tenancy`, `app/identity`, `app/tenancy`, fakes, port contract suites run against the fakes. |
| 2 | `m2-auth` | none | `adapter/auth`: EdDSA JWT with `kid` rotation, refresh rotation with reuse detection, Argon2id; the security tests. |
| 2 | `m2-web-auth` | none | `packages/api` auth client, session store, login, sign-up, tenant picker, route guards. Uses the merged OpenAPI. |
| 2 | `m2-mobile-auth` | none | Ktor client, `TokenStore`, auth graph, login, sign-up, tenant picker, `PermissionGate`. |
| 3 | `m2-integrate` | `main.go`, `router.go`, `openapi.yaml` | Postgres repositories for identity and tenancy, `authn` middleware, CORS, rate limit, handlers, wiring, contract suites against real adapters, end-to-end "Done when" checks. |

## Later milestones (plan, refine when you get there)

- **M3:** `m3-contract` (OpenAPI, then) → `m3-catalog`, `m3-supplier`, `m3-web-catalogo`, `m3-mobile-catalogo` → `m3-integrate`. Migrations `0006`, `0007` go to one session.
- **M4:** `m4-workflow` (pure domain + `0003` tests), `m4-schedule` (pure functions) and `m4-customer` in parallel → `m4-order` → `m4-contract` → web and mobile screens in parallel → `m4-integrate`. Build payments as manual; ask before PIX.
- **M5, M6:** same shape: contract, backend contexts, web and mobile in parallel, integrate.
- **M7:** blocked on SEO and custom domains. Ask first.

## Prompt template

> You are the `<session>` session for milestone N. Read `CLAUDE.md`, the `CLAUDE.md` of the app you touch, `docs/parallel-sessions.md`, and the sections of `docs/architecture.md` and `docs/product/` your scope names. Scope: <row from the table>. Do not edit files you do not own. Write tests first. Finish with `make lint test`, the matching reviewer agent, and a PR with the build-plan boxes ticked. Do not merge.
