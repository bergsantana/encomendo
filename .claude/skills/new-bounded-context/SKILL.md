---
name: new-bounded-context
description: Scaffold a new bounded context in Encomendo's backend (domain, app with ports and fakes, adapters, contract tests, wiring) by hand, following the architecture. Use when starting a context listed in docs/architecture.md or one approved by an ADR.
model: opus
---

# New bounded context

Every file here is written by hand. Generators are not allowed in the backend.

## 0. Is it approved?

The context must appear in `docs/architecture.md` section 4.2, or have an accepted ADR in `docs/adr/`. If not, write the ADR proposal (problem, options, decision, consequences) and stop for approval. Don't invent a context to fit a feature.

## 1. Domain: `internal/domain/<ctx>/`

```text
doc.go           package comment: what the context owns and its rules
errors.go        sentinel errors: var ErrX = errors.New("<ctx>: …")
<aggregate>.go   aggregate root: unexported fields, New(...) validates, methods change state
<value>.go       value objects (immutable, validated on construction)
*_test.go        one per file, table-driven, written first
```

- Standard library and other `internal/domain` packages only. Prefer referring to other contexts by typed IDs from `shared`.
- Time arrives as a `now time.Time` argument. IDs arrive from the caller.
- Aggregates that are updated carry a `version int`.

## 2. Application: `internal/app/<ctx>/`

```text
doc.go
ports.go         UseCases (driving) + Repository and other driven ports
inputs.go        input, view, filter types (no domain structs leak out)
service.go       NewService(...) + use cases
queries.go       cached or read-only queries, if any
service_test.go
fake/            hand-written in-memory fakes of every driven port
```

- Each use case follows "How a write use case runs" in `backend/CLAUDE.md`.
- Add the context's permissions to `identity.Permission` and the role map, with tests for each role.

## 3. Contract tests: `test/contract/<ctx>/`

One suite per driven port, as a function that takes a constructor, so the same cases run against the fake and the real adapter:

```go
// RunRepositoryContract checks that any Repository behaves like the
// Postgres one: not-found errors, optimistic locking, list ordering.
func RunRepositoryContract(t *testing.T, newRepo func(t *testing.T) ctx.Repository)
```

## 4. Adapters

- `adapter/postgres/models/<ctx>.go` and `adapter/postgres/<ctx>_repo.go`
- `adapter/http/handler/<ctx>.go` and `adapter/http/handler/dto/<ctx>.go`
- Tables: use the `new-migration` skill.
- Endpoints: use the `new-endpoint` skill for each route.

## 5. Wiring

Ask the owner of `cmd/api/main.go` to add the constructor calls, or add them yourself if you own it this milestone: adapter → service → handler, in the same order as the existing contexts.

## 6. Finish

- Update the glossary in the root `CLAUDE.md` if the context introduces UI terms.
- If this is the first context built fully to standard, record it under "Reference implementation" in `backend/CLAUDE.md`.
- Run `make lint test test-integration` and ask `go-reviewer` and `tenancy-security-reviewer` to review.
