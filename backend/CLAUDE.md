# Backend (Go)

Modular monolith, ports and adapters. Module `github.com/bergsantana/encomendo/backend`. The full design is in `docs/architecture.md` sections 4–7 and 9; this file holds the rules you need on every change.

## Commands

```bash
go test -race -shuffle=on -count=1 ./internal/... ./test/contract/...  # unit, plus port contracts against the fakes
go test ./internal/domain/order/... -run TestOrder                       # one package while iterating
make test-integration                                                    # testcontainers: postgres, rls, redis
make lint                                                                # gofmt, go vet, golangci-lint
go run ./cmd/migrate up                                                  # apply migrations locally
```

## Layers and what each may import

| Layer | Path | May import | Never |
| --- | --- | --- | --- |
| Domain | `internal/domain/<ctx>` | Go standard library and other `internal/domain` packages (`shared`, `workflow`, `schedule` …) | GORM, Redis, `net/http`, `log/slog`, any `app` or `adapter` package |
| Application | `internal/app/<ctx>` | `domain/...`, `app/shared` | adapters, GORM, Redis, another context's `app` service |
| Adapters | `internal/adapter/...` | `app/...`, `domain/...`, drivers and libraries | business rules |
| Composition root | `cmd/api/main.go` | everything | logic beyond wiring |

- The **domain** holds the rules: aggregates validate in constructors, keep fields unexported and change state only through methods. It has no I/O, no logging, and no `time.Now()`: time comes in as an argument.
- The **app layer** owns the ports (interfaces) and the use cases. Driving port: `UseCases`. Driven ports: `Repository`, `StockReserver`, `Cache`, `TxManager` and so on, in `ports.go`.
- One context never calls another context's service directly. It goes through a port implemented in `adapter/inproc/`.
- If two contexts would need ports to each other, that is a cycle in `main.go`. Break it with an outbox event handled by `cmd/worker`, or stop and ask. Don't wire around it with setters.
- **Adapters** translate and nothing more. A handler decodes, calls one use case, maps the result and responds.

## How a write use case runs

Every write follows these steps, in this order:

```go
// Advance moves the order to status to, a later status in the order's own
// workflow. The domain checks the guard of every phase it enters.
func (s *Service) Advance(ctx context.Context, id shared.OrderID, to workflow.StatusKey) (OrderView, error) {
	by, err := authz.Require(ctx, identity.PermOrderWrite) // 1. authorize; returns the caller
	if err != nil {
		return OrderView{}, err
	}
	var out *order.Order
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error { // 2. one transaction; sets app.tenant_id for RLS
		o, err := s.repo.ByID(ctx, id)
		if err != nil {
			return fmt.Errorf("load order %s: %w", id, err)
		}
		if err := o.Advance(to, by, s.clock.Now()); err != nil { // 3. the domain decides
			return err
		}
		if err := s.repo.Update(ctx, o); err != nil { // optimistic lock on version
			return fmt.Errorf("save order %s: %w", id, err)
		}
		out = o
		return s.events.Publish(ctx, order.Advanced{ID: id, To: to, Phase: o.Phase()}) // outbox, same transaction
	})
	if err != nil {
		return OrderView{}, err
	}
	// 4. After commit, invalidate cached reads here. Orders are never cached,
	//    but a catalog write would call s.cache.DeleteByPrefix at this point.
	return toView(out), nil // 5. a view, never the aggregate
}
```

`authz` is `internal/app/shared/authz`. The ports come from `internal/app/shared`, always imported as `appshared` (its fakes as `sharedfake`), because `internal/domain/shared` is imported next to it; golangci-lint enforces both aliases (ADR 0009). The transaction's RLS scope comes from the context: the authn middleware sets it through `authz.WithPrincipal`, and `authz.Require` refuses a scope that no longer matches the caller. Only the use cases listed on `appshared.Scope` set a scope themselves (ADR 0009). A client account never shares a scope with a tenant.

## Errors

- Shared sentinels live in `domain/shared/errors.go` (`ErrNotFound`, `ErrConflict`, `ErrForbidden`, `ErrInvalid`). Each context's own sentinels live in its `errors.go`: `var ErrInvalidTransition = errors.New("order: invalid status transition")`.
- Wrap with context when you cross a boundary: `fmt.Errorf("reserve stock for order %s: %w", id, err)`. Never wrap twice with the same words.
- Compare with `errors.Is` and `errors.As`, never by string.
- Never swallow an error. If you ignore one deliberately, assign it to `_` with a comment saying why.
- Only `adapter/http/respond/problem.go` maps errors to status codes and RFC 9457 `code` values. Those codes are part of the contract: add new ones, never rename existing ones.
- `panic` only in `main` for impossible startup states. Never in request paths.

## Naming and style

- Packages: short, lowercase, singular (`order`, `catalog`). Avoid stutter in what you export: `order.New`, `order.Status`, not `order.NewOrder`.
- No `Get` prefix on getters: `o.Total()`. Short receiver names (`o *Order`, `s *Service`).
- Constructors: `NewService(repo Repository, tx appshared.TxManager, …) *Service`. Required dependencies are arguments, never setters.
- `context.Context` is the first parameter of every port method and use case. Never store it in a struct.
- No `init()`, no package-level mutable state, no naked returns.
- Goroutines only where a use case needs them, always with `errgroup` and context cancellation.
- Functions around 40 lines. If a function needs a comment for each block, split it.

## Comments (Go specifics)

- Every package has a `doc.go` with a package comment: what the context owns and which rules it enforces.
- Doc comments start with the identifier's name and state the contract, including the errors returned:

```go
// Advance moves the order to status to, a later status in the order's own
// workflow, on behalf of by. It returns ErrInvalidTransition when to is not
// later in the workflow (or not in the same phase, for a move back), and
// ErrGuardFailed when a phase it enters is not ready, such as sourcing
// before the deposit is recorded.
func (o *Order) Advance(to workflow.StatusKey, by shared.Principal, now time.Time) error
```

- A rule that comes from the product gets a pointer to it, so a reviewer can check it:
  `// The client never sees "Pode atrasar" (docs/product/design-document.md, System rules).`

## GORM

- Persistence models with `gorm` tags live in `adapter/postgres/models/`, one file per context, each with `toDomain()` and `fromDomain()`. Domain structs never carry ORM tags.
- Repositories get the transaction from `ctx` through `conn.go`. They never take a `*gorm.DB` or a transaction parameter.
- `SkipDefaultTransaction: true`. The TxManager owns transactions.
- No `gorm.Model` embed and no `DeletedAt`, so there is no hidden soft-delete filter.
- `Updates(struct)` skips zero values. Use a map, or `Select` with the column names, when a field must become 0, "" or false. Never `Select("*")`: `encomendo_app` may update only some columns.
- Optimistic locking: `Where("version = ?", v)`, and return `shared.ErrVersionConflict` (it matches `shared.ErrConflict`) when `RowsAffected == 0`. The API answers `409 version_conflict`.
- `Preload` only what the use case returns. Use `Raw` only where the builder gets unclear (reports, stock sums), with bound parameters, never `fmt.Sprintf` into SQL.
- Map Postgres errors in `errors.go`: 23505 → conflict, 23503 → invalid reference, 40001 → retryable conflict.

## Redis

- Keys always start with the tenant: `t:{tenantID}:catalog:item:{id}`. `cache.go` refuses a key without that prefix, because RLS does not protect Redis.
- Cache only hot reads that are safe to be slightly stale: store page catalog and kits, portal config, item detail, status workflow presets. TTL 5–10 min plus explicit invalidation after writes.
- Never cache order or customer data that depends on the caller's permissions.
- A Redis error is logged and the read falls back to Postgres. It never fails the request.

## Product rules that live in backend code

These come from `docs/product/` and `docs/architecture.md` section 4.2. Each one needs a test:

- Store page, tracking and client area responses never contain supplier, cost, margin or "Pode atrasar". They use their own view types, not admin types with fields left empty.
- Costs and margins are visible only to the owner, through a domain policy function. Other roles' view types leave them out.
- The supplier message (`supplier_message.go`, sent by wa.me link) never carries the client's name or phone.
- A store never sees a client's orders at other stores. A client account reads only its own linked orders and writes nothing through the client policy.
- An order item keeps a snapshot of item name, price, sourcing mode and chosen options. Later catalog edits never change past orders.
- Purchase lines are generated only after the deposit, unless the tenant turned on `purchase_before_deposit`. Every line points to one order item or to a stock restock.
- **Status workflows:** code branches on `workflow.Phase`, never on a status key or label. An order keeps the workflow version it was created with; changing the store's workflow affects new orders only. Every preset passes `workflow` validation.
- Delivery date = latest item ready date + assembly days + safety margin (`schedule`). Arrival limit = delivery − assembly days. Alerts sit on top of the status; they are not statuses.
- Kit swap price follows the kit's own swap mode (ADR 0023, ADR 0025), always at single-item prices times quantity. `keep` (the default): kit price − removed item × quantity + new item × quantity, so the kit discount stays. `lose`: the sum of the resulting items' single-item prices × quantity, so the discount is lost. A kit with no swaps costs its own price in both modes.
- Changing the delivery date records the change and returns a wa.me link to warn the client. Nothing notifies the client automatically.
- Tracking tokens and login codes are stored only as hashes.

## Logging

- `log/slog` only, structured, in adapters and `platform`. The domain and app layers return errors instead of logging them.
- Never log tokens, refresh tokens, password hashes or full request bodies. Mask customer email and phone.

## Testing

| Level | Where | Rules |
| --- | --- | --- |
| Domain unit | next to the code, `*_test.go` | Table-driven. Phase guards and allowed/forbidden moves for **every preset workflow**; `workflow` validation; item transitions; kit swap pricing; deadline and alerts (`schedule`), including the Kit Scrambler example (15 + 3 + 2 = day 20, arrival deadline day 17). Fuzz slugs, money parsing, and customer names in supplier messages. |
| Use case | `internal/app/<ctx>/service_test.go` | Hand-written fakes from `internal/app/<ctx>/fake/`, fixed clock, fixed ID generator. Cover authorization denied, the happy path, events published and cache invalidated. |
| Port contract | `test/contract/` | One suite per port, run against the fake **and** the real adapter. A fake that fails its contract is a bug. |
| Repository | `test/integration/postgres/` | testcontainers, migrations applied, connected as `encomendo_app`. |
| RLS | `test/integration/rls/` | Two seeded tenants and two client accounts. Five cases per tenant table: select, insert with the other tenant's id, update, delete, and no tenant set (expects zero rows). Staff-only tables (supplier, cost, purchasing data; ADR 0024) add a sixth: a store page scope, a tenant with no user, reads zero rows and writes none. Tables with a client policy also check another client's orders (zero rows) and any write as a client (rejected). |
| Cache | `test/integration/redis/` | Key prefix, TTL, invalidation, Redis down falls back to Postgres. |
| HTTP | `internal/adapter/http/**` | `httptest` with fake use cases. Status codes, problem+json, DTO validation, auth middleware, tenant from the Host subdomain. Responses validated against `docs/api/openapi.yaml`. |
| Client visibility | `internal/adapter/http/**` | Store page, tracking and client area responses contain no supplier, cost, margin or `at_risk` field, for every endpoint. |
| Auth | `internal/adapter/auth/` | Expired token, wrong `aud`, `alg: none`, unknown `kid`, refresh-token reuse revokes the family; client token on admin routes and staff token on `/me`; login-code expiry and attempt limit; Google token with the wrong audience; tracking-token guessing and cross-tenant lookup. |
| E2E | `test/e2e/` | Go client against docker compose. |

- Name tests by behavior: `TestOrder_Advance_RejectsSourcingWithoutDeposit`.
- Use `t.Run` subtests, and `t.Parallel()` where the test has no shared state.
- No `time.Sleep`. Use the fixed clock or poll with a deadline.
- Build test data with `internal/testkit` builders: `testkit.NewOrder().WithItems(2).Build()`.
- Assertions use the standard library. Adding testify or go-cmp needs an ADR.
- Coverage gates: ≥ 90% on `internal/domain` and `internal/app`, ≥ 80% overall. Never lower them.

## Migrations

Use the `new-migration` skill (`.claude/skills/new-migration/`). In short: goose SQL files only, Up and Down, every tenant table gets `tenant_id`, an index starting with `tenant_id`, composite foreign keys `(tenant_id, id)`, `ENABLE` and `FORCE ROW LEVEL SECURITY`, and RLS tests.

## Reference implementation

_To fill in after milestone 2: the first context built fully to this standard (for example `tenancy`). New contexts copy its shape._
