# 0001 — Ports and adapters in a modular monolith

- Status: accepted
- Date: 2026-11-24

## Context

Encomendo has many bounded contexts (catalog, orders, purchasing, production …), strict tenant isolation, and a rule that every backend line is written by hand. It is built by one developer with several AI sessions in parallel, so boundaries must be visible in the folder structure and checked by tools, not remembered.

## Decision

- One Go module and one deployable (`cmd/api`), split into `internal/domain`, `internal/app` and `internal/adapter`.
- Dependencies point inward only: `adapter → app → domain`. The domain imports only the standard library and other domain packages.
- The app layer owns the ports (interfaces) next to the use cases that consume them. Adapters implement them. `cmd/api/main.go` is the only place that wires concrete types.
- Cross-context calls go through ports implemented in `adapter/inproc`, never by importing another context's service.
- `depguard` in `backend/.golangci.yml` enforces the import rules in CI.

## Consequences

- Each context can be worked on in its own worktree with little overlap.
- Wiring in `main.go` is long and explicit. That is accepted: no DI framework (wire, fx) is allowed.
- Splitting a context into its own service later means replacing an `inproc` adapter with a network one.
