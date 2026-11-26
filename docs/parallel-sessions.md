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
| 1 | `m2-contract` | `docs/api/openapi.yaml` | `domain/shared` (typed IDs, `Money`, `Page`, `Principal`, `Role`), `domain/identity` permissions and role map, `app/shared` ports (`TxManager` + `Scope`, `Cache`, `Clock`, `IDGenerator`, `EventPublisher`), `authz`, fakes and their contract suites, OpenAPI for `/auth/*` and tenancy, `make api-types`. ADRs 0009–0011. No adapters. |
| 2 | `m2-db` | `migrations/` | `0002` (with `password_reset_codes`, and `refresh_families` plus `replaced_by`, `superseded_at` and `access_jti` on `refresh_tokens` for ADR 0013), `0003` (ADR 0006), `0004` (with `invitations`, ADR 0011), `0005` (helpers that read `''` as NULL), `0006_platform`, `0007_rls_policies` (with the user-scope `SELECT` policies on `memberships` and `tenants`, ADR 0010; numbered as they land, ADR 0014); Postgres `TxManager` that sets all three RLS settings from `appshared.Scope` on every transaction, uses savepoints and passes `RunTxManagerContract`; outbox publisher passing `RunEventPublisherContract`; RLS test suite. |
| 2 | `m2-domain` | none | The rest of `domain/identity` (user, client account, email, password policy, password reset code), `domain/tenancy` (tenant, slug, membership, invitation, settings, plan), `app/identity`, `app/tenancy` (including RemoveMember, invitations and password reset, ADR 0011), fakes, port contract suites run against the fakes. |
| 2 | `m2-auth` | none | `adapter/auth`: EdDSA JWT with `kid` rotation, refresh rotation with reuse detection and the 2-minute retry window (ADR 0013), Argon2id; the security tests. |
| 2 | `m2-web-auth` | none | `packages/api` auth client, session store, login, sign-up, tenant picker, forgot password, accept invitation, route guards, members (list, invite, change role, remove). Uses the merged OpenAPI. |
| 2 | `m2-mobile-auth` | none | Ktor client, `TokenStore`, auth graph, login, sign-up, tenant picker, forgot password, `PermissionGate`, members (list, invite, change role, remove). Refresh transport `body` (ADR 0010). |
| 3 | `m2-integrate` (three PRs: 5a, 5b, 5c; see below) | `main.go`, `router.go`, `openapi.yaml` | Postgres repositories for identity and tenancy, `authn` middleware, no CORS but a cross-origin refusal on `/auth/*` (ADR 0010), store page middleware before authn and client tokens only on store routes (ADR 0009), 5xx and never 401 when a dependency fails, `Retry-After` on every 429, a native-client test on the cross-origin check (ADR 0013), rate limits, handlers, the new problem codes in `respond/problem.go`, `adapter/notify/smtp.go` and Mailpit in the dev stack (ADR 0011), wiring, contract suites against real adapters, end-to-end "Done when" checks. |

`m2-integrate` is the largest step, so it runs as three PRs, in this order, each green on its own:

- **5a, data adapters:** the Postgres identity and tenancy repositories, `adapter/inproc`, the Redis denylist, `adapter/notify/smtp.go`, and the system clock and ID generator. No HTTP and no wiring.
- **5b, HTTP:** the problem codes, `authn`, the cross-origin refusal, the store page middleware, rate limits, the handlers and `openapi.yaml`.
- **5c, wiring and end to end:** the configuration (keys, SMTP, hosts), `main.go`, Mailpit in the dev stack, and `test/e2e` for the "Done when" checks.

## Later milestones (plan, refine when you get there)

- **M3:** `m3-contract` (OpenAPI, then) → `m3-catalog`, `m3-supplier`, `m3-web-catalogo`, `m3-mobile-catalogo` → `m3-integrate`. The catalog and supplier migrations, numbered as they land (ADR 0014), go to one session.
- **M4:** `m4-workflow` (pure domain + `0003` tests), `m4-schedule` (pure functions) and `m4-customer` in parallel → `m4-order` → `m4-contract` → web and mobile screens in parallel → `m4-integrate`. Build payments as manual; ask before PIX.
- **M5, M6:** same shape: contract, backend contexts, web and mobile in parallel, integrate.
- **M7:** blocked on SEO and custom domains. Ask first.

## Prompt template

> You are the `<session>` session for milestone N. Read `CLAUDE.md`, the `CLAUDE.md` of the app you touch, `docs/parallel-sessions.md`, and the sections of `docs/architecture.md` and `docs/product/` your scope names. Scope: <row from the table>. Do not edit files you do not own. Write tests first. Finish with `make lint test`, the matching reviewer agent, and a PR with the build-plan boxes ticked. Do not merge.
