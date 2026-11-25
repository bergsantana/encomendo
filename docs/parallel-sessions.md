# Running Claude sessions in parallel

How the work is split across worktree sessions without collisions. `docs/build-plan.md` tracks status; this file tracks **who works on what, in which order**.

## The model: one session per app

Three long-running sessions, one per app, each in its own worktree:

| Session | Worktree | Owns |
| --- | --- | --- |
| Backend | `backend` | `backend/`, `docs/api/openapi.yaml`, `main.go`, `router.go`, the migrations folder, `docs/adr/` for backend decisions |
| Web | `web` | `web/` |
| Mobile | `mobile` | `mobile/` |

One backend session means nothing fights over the shared files in `CLAUDE.md` ("Working in parallel sessions"). The price is that backend work is sequential, so the backend session is the pace-setter and the other two follow its contract.

## Rules

- **Contract first.** For every feature the backend session merges the OpenAPI change (and, when the shapes are new, `make api-types`) in a small PR **before** the implementation. Web and mobile start on a feature only after its contract is on `main`. Until the endpoints exist they work against mocks (MSW on web, Ktor `MockEngine` on mobile) built from the contract.
- **One PR per concern**, even inside a long session: branch from an up-to-date `main`, open the PR, and start the next piece on a new branch once it is merged. Never stack unmerged PRs.
- **Stay in your app.** Web and mobile do not edit `backend/` or `docs/api/openapi.yaml`. If the contract is wrong or missing, write the change you need in a short note and tell the user; the backend session applies it.
- **Parity.** A management feature is done when web and mobile both work against the same endpoints (`CLAUDE.md`, rule 6). Tick the web and mobile boxes in `docs/build-plan.md` separately.
- **Keep sessions small in context.** Start a fresh session for each milestone (`claude --worktree backend` again, or `/clear`). The plan, the ADRs and the build plan carry the state, not the conversation.
- **Sync often.** `git fetch && git rebase origin/main` before each new piece of work and before each PR. Expect small conflicts in `docs/build-plan.md` checkboxes.
- **Stacks.** Tests need no running stack: integration tests use testcontainers on random ports. A session that runs `make dev-up` needs `scripts/worktree-env.sh <slot>` first (slot 1 to 9), which gives its worktree its own project name and ports. Mobile sessions verify with `scripts/mobile-docker.sh check`.
- **Every PR:** `make lint test` (plus `make test-integration` for adapters), the matching reviewer agent (`go-reviewer`, `tenancy-security-reviewer`, `web-reviewer`, `mobile-reviewer`), and its build-plan boxes ticked. Sessions do not merge.

## Milestone 2 — Identity and tenancy

**Backend session**, in this order, one PR each:

1. Contract: `domain/shared` (typed IDs, `Money`, `Page`, `Principal`), `app/shared` ports, OpenAPI for `/auth/*` and tenancy, `make api-types`. **Merge before web and mobile start.**
2. Migrations `0002`, `0003` (ADR 0006), `0004`, `0005`, `0014`, `0015`, the Postgres `TxManager`, and the RLS test suite.
3. `domain/identity`, `domain/tenancy`, `app/identity`, `app/tenancy`, fakes, port contract suites.
4. `adapter/auth`: EdDSA JWT with `kid` rotation, refresh rotation with reuse detection, Argon2id, security tests.
5. Postgres repositories, `authn` middleware, CORS, rate limit on `/auth`, handlers, wiring in `main.go` and `router.go`, "Done when" checks.

**Web session**, after step 1 is merged: `packages/api` auth client with refresh once on 401, session store, login, sign-up, tenant picker, route guards, `usePermission`. Mocked until step 5 lands, then run against the real API with `make dev-up`.

**Mobile session**, after step 1 is merged: Ktor client with bearer and refresh, `TokenStore`, auth graph, login, sign-up, tenant picker, forgot password, `PermissionGate`. Mocked until step 5 lands, then run against the real API.

## Later milestones

Same shape: the backend session merges the contract for the milestone, then builds it in sequence; web and mobile build their screens against the contract and mocks, and switch to the real API when the backend finishes.

- **M3 Catalog and suppliers:** contract; migrations `0006`, `0007`; `catalog`, `supplier`; cache; photo storage. Web: Catálogo. Mobile: catalog data layer and screens.
- **M4 Customers, orders and deadlines:** `workflow` and `schedule` first (pure domain, quick), then `customer`, then `order`. Web: Hoje, Pedidos, Clientes, Financeiro. Mobile: the same. Build payments as manual; ask before PIX.
- **M5, M6:** same shape.
- **M7:** blocked on SEO and custom domains. Ask first.

If the backend becomes the bottleneck in a milestone with independent contexts (for example `catalog` and `supplier`), add a second backend worktree for one context. Give it a different set of files and keep the migrations and `main.go` changes with the first one.

## Prompt template

> You are the `<app>` session for milestone N. Read `CLAUDE.md`, `<app>/CLAUDE.md`, `docs/parallel-sessions.md`, and the sections of `docs/architecture.md` and `docs/product/` that your scope names. Scope: <list from this file>. Stay in your app. Write tests first. For each piece, open one PR with `make lint test`, the matching reviewer agent, and the build-plan boxes ticked. Do not merge.
