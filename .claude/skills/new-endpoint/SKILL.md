---
name: new-endpoint
description: Add or change an Encomendo API endpoint end to end, from the OpenAPI contract through domain, use case, repository and handler to the web and mobile clients, with tests at every layer. Use when a feature needs a new or changed route.
---

# New endpoint

Work from the contract inward, then outward to the clients. Each step ends with its tests passing.

## 1. Contract

- Add or change the path in `docs/api/openapi.yaml`. If you don't own the spec this milestone, write the proposed YAML in your PR description or a message and stop until it's merged.
- Follow `docs/architecture.md` section 6: `/api/v1/admin/*` for staff, `/api/v1/store/*` for the public store page (tenant from the Host subdomain), `/api/v1/me/*` for client accounts. Problem+json errors with a stable `code`, cursor pagination, `Idempotency-Key` on creating POSTs, `409` with `version_conflict` for lock failures. Endpoints that produce a WhatsApp message return the `wa.me` URL.
- Store page, tracking and client area schemas are separate from admin schemas and contain no supplier, cost, margin or "Pode atrasar". Order responses carry the status key, label, client label and phase.

## 2. Domain (only if a rule changes)

- Tests first in `internal/domain/<ctx>/*_test.go`, table-driven, covering the allowed and the forbidden cases. Anything that depends on order status is tested against every preset workflow, and branches on the phase only.
- Then the method or value object. Unexported fields, validating constructor, no I/O.

## 3. Use case

- Add the method to `UseCases` in `internal/app/<ctx>/ports.go`, plus input and view types in `inputs.go`.
- Tests first in `service_test.go` with fakes from `internal/app/<ctx>/fake/`: permission denied, invalid input, happy path, event published, cache invalidated (if the context caches).
- Implement it following "How a write use case runs" in `backend/CLAUDE.md`.
- If you added a driven port method, add it to the fake **and** to the contract suite in `test/contract/`.

## 4. Repository

- Implement any new port method in `adapter/postgres/<ctx>_repo.go`. Follow the GORM rules in `backend/CLAUDE.md`.
- Integration test in `test/integration/postgres/`. If the query touches a new table, use the `new-migration` skill first.

## 5. HTTP

- Request and response DTOs and mappers in `adapter/http/handler/dto/`.
- The handler decodes (size limit, `DisallowUnknownFields`), calls one use case, maps and responds. No business rules.
- Register the route in `router.go` (ask its owner if it isn't you).
- `httptest` tests with a fake use case: success, validation error, 401, 403, 404, 409 where it applies. Validate the response against `openapi.yaml`. For store page and client routes, add the client-visibility check (no internal fields).

## 6. Web

```bash
pnpm api:types
```

- Function in `packages/api/<ctx>.ts`, store action, view or component, pt-BR text, in `apps/admin` or `apps/store` as the endpoint's audience requires.
- Vitest with an MSW handler for the new route.

## 7. Android (KMP), management endpoints only

- Skip this step for store page and client area endpoints: those are web only.
- Android only: no iOS code.
- DTO in `core:network`, mapping in `core:data`, repository method, use case in `core:domain`, ViewModel and screen in the feature module, Koin wiring.
- Tests: MockEngine for the call, a ViewModel test with Turbine and a fake repository.

## 8. Finish

- Run `make lint test`, plus `make test-integration` if adapters changed.
- Ask `go-reviewer`, then `web-reviewer` and `mobile-reviewer` for their parts. Use `tenancy-security-reviewer` if the endpoint is on a store page, tracking or client area route, or touches auth.
- Fix the blocking findings and re-run the checks. Paste the final output in the PR description.
