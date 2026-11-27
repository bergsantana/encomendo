# Encomendo

Multi-tenant SaaS for small Brazilian businesses that sell made-to-order products: the client builds an order, the business buys or makes only what was sold, and everyone knows when it will be ready.

- `backend/`: one Go modular monolith, one JSON API.
- `web/`: Vue workspace with two apps. `admin` is the management app; `store` is each tenant's store page at `{slug}.encomendo.com.br`, plus the client area (Minhas encomendas).
- `web/apps/android`: the Android **management** app, a Capacitor shell that opens the hosted web admin (ADR 0029). Clients never use it.
- `mobile/`: the earlier native Kotlin Multiplatform Android app, **frozen** since ADR 0029. It keeps building in CI and gets no new features; it is removed once the shell reaches the Play Store.

## Sources of truth

- `docs/architecture.md`: layers, schema, auth, RLS, status workflows, testing, CI and build order. Follow it. If something is ambiguous, pick the option that keeps the dependency rule, tenant isolation and testability intact, and record the choice as an ADR in `docs/adr/`.
- `docs/product/`: what the product does and why (design document, product overview). Read the section for a feature before building it. When product behaviour disagrees with the architecture, the design document wins.
- `docs/api/openapi.yaml`: the API contract, written by hand.
- Each app has its own `CLAUDE.md` with its conventions: `backend/`, `web/` (the Android shell included), `mobile/` (frozen). Read it before editing that app.

## Commands

```bash
make dev-up            # docker compose with hot reload: postgres, redis, api, Vite admin + store
make stack-up          # built-image stack behind nginx, as CI and e2e run it
make lint              # all linters
make test              # unit tests, all apps
make test-integration  # testcontainers: repos, RLS, cache
make migrate           # apply SQL migrations as encomendo_owner
```

Run the narrowest command that proves your change while iterating (one package, one test), and the full `make lint test` before you call the work done.

## Non-negotiable rules

These summarize `docs/architecture.md` section 2. Read it for the full list.

1. **No generated code in `backend/`.** No sqlc, gorm/gen, mockgen, mockery, oapi-codegen, wire or fx. GORM is a library and is allowed. Web and mobile may generate (openapi-typescript, Room/KSP); never hand-edit a generated file.
2. **Dependency rule:** `adapter → app → domain`, never the reverse. Only `cmd/api/main.go` knows every concrete type.
3. **Tenant isolation is enforced three times:** JWT principal (or the store page subdomain) → app-layer authorization → Postgres RLS. A client account crosses tenants only to read its own orders.
4. **GORM never creates schema.** No `AutoMigrate`. Schema comes only from SQL migrations.
5. **Every feature ships with tests**, at the levels in `backend/CLAUDE.md`. Every bug fix starts with a failing test that reproduces it.
6. **Management on every device:** a management feature is done when its web admin screen works at desktop and phone sizes, and the Android shell opens it (ADR 0029). There is no second implementation. Don't add features to the frozen `mobile/` app. The store page and client area are web only. Don't add iOS targets, files or CI jobs.
7. **Clients never see internal data.** Store page, tracking and client area responses never contain suppliers, costs, margins or "Pode atrasar".
8. **WhatsApp through `wa.me` links only.** No paid WhatsApp API.
9. **Rules hang on phases, names come from data.** Each store picks its own status list. Code branches on the order's `workflow.Phase`, never on a status key or label. Niche words come from the template.

If a rule blocks you, stop and explain the trade-off. Never weaken a rule to get green: no skipped tests, no lowered coverage gate, no `//nolint` or `eslint-disable` without a comment saying why.

## Language

- Code identifiers, comments, commits and docs: **English**. Feature folders follow the pt-BR menu, as in the architecture (`hoje`, `pedidos`, `compras`, `catalogo` …); the code inside them is still English.
- Everything a user sees: **Brazilian Portuguese**, with the exact wording in `docs/product/` ("Precisa chegar até", "Pode atrasar", "Margem de segurança", "Marcar como enviada ao fornecedor").
- Status names come from the order's status workflow. Niche words come from the tenant's template: Peças / Produtos / Módulos, Moto / Ambiente / Telhado, Montagem / Instalação. Never hard-code either.

## Domain glossary

The full table is in `docs/architecture.md` section 4.2. The names you will use most:

| UI (pt-BR) | Code |
| --- | --- |
| Pedido, item do pedido | `order.Order`, `order.Item` |
| Lista de status da loja | `workflow.StatusWorkflow`; rules hang on `workflow.Phase` |
| Item do catálogo | `catalog.Item` |
| Sob encomenda / Estoque / Produção própria | `catalog.SourcingMode`: `to_order` / `stock` / `in_house` |
| Kit pronto, trocar item | `catalog.Kit`, kit swap (`kit_swap.go`) |
| Monte o seu | custom order, priced by the storefront Preview |
| Fornecedor, prazo, fornecedor reserva | `supplier.Supplier`, `supplier.SupplierItem` |
| Compras, pedido ao fornecedor | `purchasing.PurchaseLine`, `purchasing.SupplierOrder` |
| Receber itens, Tudo certo / Com defeito | `purchasing.Receipt` (`ok` / `defective`) |
| Sinal / Saldo | `PaymentKind`: `deposit` / `balance` |
| Precisa chegar até | `ArrivalLimit` |
| Pode atrasar / Atrasada | `Alert`: `at_risk` / `late` |
| Margem de segurança | `SafetyMargin` |
| Entrega / Retirada | `Handover`: `delivery` / `pickup` |
| Cliente (of one store) / conta do cliente | `customer.Customer` / `identity.ClientAccount` |
| Página da loja, Minhas encomendas | `storefront`, `clientarea` |
| Equipe, membro, papel: Dono / Admin / Funcionário / Visualizador | `tenancy.Membership`, `shared.Role` (`owner` / `admin` / `staff` / `viewer`, ADR 0017) |
| Convite (para a equipe) | `tenancy.Invitation` (ADR 0011) |
| Esqueci a senha, código | password reset code in `identity` (ADR 0011) |
| Categoria; campo do item ("Serve em") e seus valores | `catalog.Category`; `catalog.Attribute` with ordered values (ADR 0025) |
| Opções do cliente, escolha | `catalog.CustomizationOption`, its choices (extra price, extra days; ADR 0024) |
| Fornecedor principal / fornecedor reserva | `supplier.SupplierItem` with role `main` / `backup`, held by `catalog.Item` (ADR 0025) |
| Cliente pode trocar, pode trocar por | swappable `catalog.KitItem` and its alternatives (ADR 0025) |
| Manter o desconto / cobrar a soma avulsa | kit swap mode `keep` / `lose` (ADR 0023) |
| Arquivar / Restaurar | `Archive` / `Restore` on items and kits; nothing in the catalog is deleted (ADR 0025) |

For a term that isn't listed, don't invent a context or a name on your own. Ask, record the decision as an ADR, and add the term here.

## Clean code (every app)

- One package or module per bounded context. Names say what the code does: `order.Service`, never `utils`, `common`, `helpers`, `manager`.
- Small interfaces next to the code that consumes them. Constructor injection. No globals, no service locators.
- Functions around 40 lines at most. More than four parameters means an input type.
- Return early; keep the happy path unindented.
- Prefer idiomatic code for the language over textbook patterns. Add an abstraction only when it has a second implementation or a test needs it.
- Money is integer cents plus a currency, everywhere. Never floats.
- Prices, deposits and dates are calculated by the backend only. Clients display them.

## Comments

- Every exported or public identifier has a doc comment that starts with its name and says what it guarantees.
- Comments explain **why** (a rule, a trap, a decision), not what the next line does.
- Link the rule a non-obvious check enforces: `// Product rule: the supplier message never carries the client's name or phone (docs/product/design-document.md, System rules).`
- No commented-out code. `TODO` only with an issue number: `// TODO(#42): …`.

## Working in parallel sessions

Several sessions may run at once, each in its own git worktree. To avoid collisions:

- Work only inside the area your task names (one bounded context, or one app's feature folder).
- These files have **one owner per milestone**; if you are not the owner, describe the change you need instead of editing them:
  - `backend/cmd/api/main.go`
  - `backend/internal/adapter/http/router.go`
  - `docs/api/openapi.yaml`
  - `backend/internal/adapter/postgres/migrations/`
- Local services: set `COMPOSE_PROJECT_NAME` to your worktree name before `make dev-up`, so containers and volumes don't clash.
- Branch from `main`, keep PRs small, and rebase on `main` before opening one.

## Model routing

Spend the strongest model where a mistake spreads or leaks; use Sonnet where a written rule or skill already holds the design.

- Defaults are set in `.claude/`: subagents without a `model` run on Sonnet (`CLAUDE_CODE_SUBAGENT_MODEL`); `tenancy-security-reviewer` runs on Opus at high effort; the other reviewers on Sonnet. Skill `model` applies only when the skill is invoked as `/name`: `/new-migration` and `/new-bounded-context` use Opus, `/new-endpoint` uses Sonnet.
- When you spawn an agent, pass `model` explicitly if the default is wrong for the task: Opus for planning or reviewing schema, RLS, auth, tenancy or cross-context design; Sonnet for implementation that follows a plan or skill; Haiku for pure lookups.
- Main session: plan a milestone on Opus (or `/model opusplan`), write the plan down, then execute on Sonnet.

## Definition of done

- `make lint test` passes, plus `make test-integration` when adapters changed. Show the output; don't just say it passed.
- Tests exist at every level the change touches.
- `docs/api/openapi.yaml` matches every endpoint added or changed.
- Management features: the web admin screen works at desktop and phone sizes (tested at 390 × 844), and the Android shell still builds.
- Any decision not covered by `docs/architecture.md` has an ADR.
- No new dependency without a one-line justification in the PR description.

## Commits and PRs

- Conventional Commits, scoped by context or app: `feat(order): add pickup handover`, `fix(web/catalogo): keep filter on reload`.
- One bounded context or one concern per PR. Use the PR template.

## Open questions: ask, don't decide

The list is in `docs/architecture.md` section 12: payments (manual or PIX), store page SEO, custom domains, and whether stores may rename statuses or build their own lists. If your task depends on one, stop and ask.
