# 0004 — Fixed order phases, store-chosen status lists

- Status: accepted
- Date: 2026-11-24

## Context

Encomendo serves many niches. A motorcycle shop, a bakery and a furniture workshop name the steps of an order differently and skip different ones. The rules (no purchases before the deposit, no assembly before every item is checked) must stay the same for all of them.

## Decision

- The order lifecycle has fixed phases in code: `draft`, `awaiting_deposit`, `confirmed`, `sourcing`, `assembling`, `ready`, `handed_over`, plus `cancelled`. Every rule hangs on a phase.
- A status workflow is an ordered list of named statuses, each mapped to one phase. Every workflow contains `draft`, `awaiting_deposit`, `ready` and `handed_over`.
- Workflows are global presets stored as data. Each store picks one; the niche template suggests a default.
- An order keeps the workflow version it was created with. Published versions never change.
- Code branches on the phase, never on a status key or label. Clients show the label and pick the badge color from the phase.

## Consequences

- Every preset needs tests for its allowed and forbidden moves.
- Letting stores rename statuses or build their own lists is an open question (`docs/architecture.md` section 12). The data model already allows it.
