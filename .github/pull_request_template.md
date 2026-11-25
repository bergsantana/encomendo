## What and why

<!-- One bounded context or one concern. Link the issue or milestone. -->

## Checks

- [ ] `make lint test` passes (paste the tail of the output below); `make test-integration` too if adapters changed
- [ ] Tests at every level this change touches; a bug fix starts with a failing test
- [ ] `docs/api/openapi.yaml` matches every endpoint added or changed, and `pnpm api:types` was run
- [ ] Management feature: web admin and Android screens both updated, or a follow-up is linked
- [ ] Store page / tracking / client area: no supplier, cost, margin or "Pode atrasar" in the responses
- [ ] Decision not covered by `docs/architecture.md`? ADR added in `docs/adr/`
- [ ] New dependency? One-line justification:

## Test output

```text

```
