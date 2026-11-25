# 0009 — Shared application ports, the request scope and `authz`

- Status: accepted
- Date: 2026-11-25

## Context

`docs/architecture.md` 4.3 puts the ports every use case needs (`TxManager`, `Cache`, `Clock`, `IDGenerator`, `EventPublisher`, `authz.Require`) in `internal/app/shared`. Writing them left several gaps:

- `internal/app/shared` and `internal/domain/shared` are both package `shared`, and every use case imports both.
- The `TxManager` must set `app.tenant_id`, `app.user_id` or `app.client_account_id` for RLS, but nothing said where it reads them from. Staff requests have a principal. Store page requests have a tenant from the subdomain and no principal. Tenant creation needs the id of a tenant that does not exist yet.
- Postgres ORs permissive policies, so the combination of settings decides what a transaction can read, not each setting alone.
- `Principal` carries a role, and `identity` owns the permissions, but `identity` imports `shared`, so `shared` cannot import `identity`.
- Events and cached views go through JSON, and the domain's value types had no JSON form.

## Decision

### Naming

- `internal/app/shared` is always imported as `appshared`, and its fakes (`internal/app/shared/fake`) as `sharedfake`. golangci-lint's `importas` enforces both.
- `authz` is its own package, `internal/app/shared/authz`, so calls read `authz.Require(ctx, identity.PermMembersManage)`.
- `shared.Role` and `shared.Audience` live in `domain/shared` with `Principal`. `identity.Permission`, the role → permission map (`identity.Allows`) and its tests live in `domain/identity`. The `m2-contract` step lands `permission.go` because `authz` needs it; the rest of `domain/identity` follows in `m2-domain`.

### The scope

- A `Scope{TenantID, UserID, ClientAccountID}` travels in the context (`appshared.WithScope`, `appshared.ScopeFrom`). `TxManager.WithinTx` reads only the scope.
- **A client account never shares a scope with a tenant or a user.** `Scope.Validate` refuses that mix, and `WithinTx` returns `appshared.ErrInvalidScope` without starting a transaction. With both set, RLS would read the whole store plus the client's orders at every other store (architecture 5.5, item 7: the client area sets `app.client_account_id` *instead of* `app.tenant_id`).
- `authz.WithPrincipal` stores the principal and derives the scope from it:
  - A staff principal replaces the scope with its own tenant and user.
  - A client principal on a store page keeps the store's tenant scope and adds nothing. Checkout writes the account id as a column value.
  - A client principal anywhere else gets a scope of its account only.
- The store page middleware sets the subdomain's tenant with `WithScope` and runs **before** authn. Store page routes attach client principals only; a staff token there is rejected. `m2-integrate` adds a router test for the order and the audience.
- `authz.Require` and `authz.RequireUser` also check that the scope is still the one the staff caller's token gives, and return `authz.ErrScopeMismatch` (403) otherwise. A middleware that replaces the scope on a staff request therefore fails closed instead of letting RLS and authorization judge different tenants.
- Only these set a scope themselves:
  - the store page middleware;
  - tenant creation, after `RequireUser`, for the new tenant;
  - login, refresh, tenant switch and invitation acceptance, for the user they just authenticated, and for acceptance the invitation's tenant.

  The list is in the `Scope` doc comment; adding to it needs a review by `tenancy-security-reviewer`.
- Every transaction sets all three settings with `set_config(…, true)`, to `''` when unset. On a pooled connection that once had a value, `current_setting(…, true)` returns `''`, not NULL, and `''::uuid` raises an error. The helpers in `0005_rls_helpers` (`current_tenant_id()` and friends) turn `''` into NULL, and every policy uses them. The zero scope then sees no tenant rows, as architecture 5.5 item 4 intends. The RLS suite checks a zero-scope transaction right after a scoped one on the same connection.
- A `WithinTx` inside another joins it through a savepoint. When the inner `fn` fails, its writes and events are undone and the error goes to the outer `fn`, which decides. The inner call must keep the outer scope (`ErrInvalidScope` otherwise), so a nested call can never change RLS halfway through a transaction.

### Errors

- Errors that mean "the calling code is wrong" match no domain sentinel, so `respond/problem.go` answers 500 and logs them instead of a silent 400: `ErrNoTransaction`, `ErrInvalidScope`, `ErrInvalidCacheKey`, `ErrInvalidTTL`, `ErrInvalidCacheValue`, `ErrInvalidEvent`, `ErrEventWithoutTenant`.
- `shared.ErrInvalidPrincipal` matches `shared.ErrUnauthenticated`: principals come only from verified tokens, so a principal that does not fit together is a bad token (401).

### Values through JSON

- Ids marshal as their canonical string (the unset id as `""`). `Money` marshals as `{"cents":…,"currency":…}` (the zero value as `null`) and validates on the way back.
- `sharedfake.Events` records the payload and a copy decoded from it, so use-case tests see what a consumer will read.

### Ports

- **Cache keys** are an `appshared.Key` built only by `TenantKey(tenant, parts…)` (`t:{id}:…`) or `GlobalKey(parts…)` (`g:…`). Parts are limited to `[A-Za-z0-9._-]`, so no part can hold the separator or a Redis glob character and reach another tenant's keys. `DeleteByPrefix(p)` deletes `p` and `p:*` only.
- **Cache failures never fail a request.** An unreachable cache is a miss or a no-op. A stored value that no longer decodes into the caller's type (a deploy changed it) is a miss, the entry is dropped, and `dst` stays untouched. The adapter logs both.
- **Events need a transaction and a tenant.** `EventPublisher.Publish` returns `ErrNoTransaction` outside one, and `ErrEventWithoutTenant` when the transaction's scope has no tenant, because `outbox_events` is a tenant table under RLS. Work done before a store exists (sign-up, password reset) publishes nothing. An event published in a failed nested call is dropped with it.
- **IDs.** `domain/shared.NewV7(unixMilli, random)` builds a UUIDv7 as a pure function, so the real generator and `sharedfake.IDs` produce the same layout. Ids keep their order even if the wall clock steps back. `Clock.Now` makes no such promise.
- **Fakes and contract suites.** `sharedfake.TxManager` emulates savepoints for staged work (events). It does not roll back other fakes' writes; rollback is tested against Postgres.
- Contract suites live in `test/contract/<ctx>/` as importable `Run…Contract` functions. The fakes run them in `make test` (the unit job now includes `./test/contract/...`), and real adapters run them in `test/integration`.

## Consequences

- Every app file imports `appshared` by that name, and the linter catches drift between parallel sessions.
- The Postgres `TxManager` (`m2-db`) reads `appshared.ScopeFrom(ctx)`, sets all three settings on every transaction, uses savepoints for nested calls, and must pass `RunTxManagerContract`. The outbox publisher must pass `RunEventPublisherContract`.
- The Redis cache (milestone 3) must pass `RunCacheContract`, including the prefix, glob and stale-entry rules.
- `respond/problem.go` gains `ErrScopeMismatch` and `ErrTenantRequired` mappings in `m2-integrate`.
