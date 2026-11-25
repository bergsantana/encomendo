# 0009 — Shared application ports, the request scope and `authz`

- Status: accepted
- Date: 2026-11-25

## Context

`docs/architecture.md` 4.3 puts the ports every use case needs (`TxManager`, `Cache`, `Clock`, `IDGenerator`, `EventPublisher`, `authz.Require`) in `internal/app/shared`. Three gaps showed up when writing them:

- `internal/app/shared` and `internal/domain/shared` are both package `shared`, and every use case imports both.
- The `TxManager` must set `app.tenant_id`, `app.user_id` or `app.client_account_id` for RLS, but nothing said where it reads them from. Staff requests have a principal; store page requests have a tenant from the subdomain and no principal; tenant creation needs the id of a tenant that does not exist yet.
- `Principal` carries a role, and `identity` owns the permissions, but `identity` imports `shared`, so `shared` cannot import `identity`.

## Decision

- `internal/app/shared` is always imported as `appshared`, and its fakes (`internal/app/shared/fake`) as `sharedfake`. golangci-lint's `importas` enforces both. `authz` is its own package, `internal/app/shared/authz`, so calls read `authz.Require(ctx, identity.PermMembersManage)`.
- A `Scope{TenantID, UserID, ClientAccountID}` travels in the context (`appshared.WithScope`, `appshared.ScopeFrom`). `TxManager.WithinTx` reads only the scope. The zero scope sees no tenant rows, so a forgotten scope fails closed.
- `authz.WithPrincipal` stores the principal **and** derives the scope from it: a staff principal sets its own tenant and user and replaces any earlier scope; a client principal keeps the scope's tenant (the store page it is on) and adds its account. The authn middleware calls only `WithPrincipal`, so the RLS scope can never be wider than the token. Store page middleware calls `WithScope` with the subdomain's tenant. A use case may set a scope itself when it creates a tenant.
- `authz.Require(ctx, perm)` returns the caller as well as the error, so use cases do not read the context twice: `by, err := authz.Require(ctx, …)`. `authz.RequireUser` is for staff use cases that work with or without a store (creating a store). A staff session without a store gets `authz.ErrTenantRequired`, which matches `shared.ErrForbidden`.
- `shared.Role` and `shared.Audience` live in `domain/shared` with `Principal`. `identity.Permission`, the role → permission map (`identity.Allows`) and its tests live in `domain/identity`. Milestone 2's `m2-contract` step lands `permission.go`, because `authz` needs it; the rest of `domain/identity` follows in `m2-domain`.
- Cache keys are a `appshared.Key` type built only by `TenantKey(tenant, parts…)` (`t:{id}:…`) or `GlobalKey(parts…)` (`g:…`). Parts are limited to `[A-Za-z0-9._-]`, so no part can contain the separator or a Redis glob character and reach another tenant's keys. `DeleteByPrefix(p)` deletes `p` and `p:*` only.
- An unreachable cache is a miss (`Get`) or a no-op (`Set`, `Delete*`) that the adapter logs. The port returns errors only for programming mistakes (zero key, non-positive TTL, values JSON cannot carry), because the app layer does not log.
- `EventPublisher.Publish(ctx, e)` needs the transaction in `ctx` (`appshared.ErrNoTransaction` otherwise). Events are structs with exported fields and an `EventName()` method, stored as JSON.
- `domain/shared.NewV7(unixMilli, random)` builds a UUIDv7 as a pure function, so the real generator and `sharedfake.IDs` produce the same, valid layout.
- `sharedfake.TxManager` holds events back until `fn` succeeds, so "an event exists only if its change committed" holds in use-case tests. It does not roll back other fakes' writes; rollback is tested against Postgres.
- Contract suites live in `test/contract/<ctx>/` as importable `Run…Contract` functions. The fakes run them in `make test`, and real adapters run them in `test/integration`. The unit job now includes `./test/contract/...`.

## Consequences

- Every app file imports `appshared` by that name; the linter catches drift between parallel sessions.
- The Postgres `TxManager` (`m2-db`) reads `appshared.ScopeFrom(ctx)` and must pass `RunTxManagerContract` and, with the outbox, `RunEventPublisherContract`.
- The Redis cache (milestone 3) must pass `RunCacheContract`, including the prefix and glob rules.
