# 0006 — Land the workflow tables in milestone 2

- Status: accepted
- Date: 2026-11-24

## Context

`0004_tenancy` makes `tenant_settings.status_workflow_id` reference `status_workflows`, which `0003_workflows` creates. The build order puts identity and tenancy (milestone 2) before the workflow domain (milestone 4). `docs/build-plan.md` listed two ways out: ship `0003` early, or make the column nullable and add the foreign key later.

## Decision

Ship `0003_workflows.sql` in milestone 2: the global tables (`status_workflows`, `workflow_statuses`) and the launch-preset seed. The workflow **domain code, validation and use cases** still land in milestone 4.

- `status_workflow_id` and `status_workflow_version` stay `not null`, with a composite foreign key. A tenant can never exist without a workflow.
- Sign-up in milestone 2 assigns the preset the niche template suggests; until templates exist (milestone 7) it uses the default preset.
- The migration numbering in `docs/architecture.md` section 7 does not change.

## Consequences

- Milestone 2 owns `0003`. Its integration test checks that the seed loads and that tenants can read but not write the global tables.
- Phase names in the seed must match `workflow.Phase` (ADR 0004). Milestone 4 adds a test that loads every seeded workflow through `validate.go`.
- A nullable column would have let a tenant exist with no workflow, which breaks the rule that every order takes the store's current workflow.
