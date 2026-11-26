---
name: build-step
description: Build one planned step of an Encomendo milestone with tests, review it and tick its boxes in docs/build-plan.md. Use after /plan-milestone. Argument is the step id, for example `/build-step 2.3`.
model: sonnet
---

# Build one step

This runs on Sonnet because the plan and the written rules already hold the design. Follow the plan; do not redesign it.

Argument: a step id such as `2.3`. If it is missing, ask for it.

## 1. Load the step

- Open `docs/plans/m<n>-*.md` and read step `<n>.<m>`: goal, model, skill, files, tests, reviewers and dependencies.
- Confirm the steps it depends on are merged or present in this branch. If not, stop and say which.
- Read the `CLAUDE.md` of each app you will edit.
- If the step's **Model** is `opus` and this session is not on Opus, stop and tell the user to switch with `/model opus` (or restart with `claude --model opus`). Do not build a schema, RLS, auth or tenancy step on Sonnet.

## 2. Check ownership and blocks

- Do not edit `backend/cmd/api/main.go`, `backend/internal/adapter/http/router.go`, `docs/api/openapi.yaml` or the migrations folder unless this session owns them for the milestone. Otherwise describe the change needed and continue with the rest.
- If the plan marks the step as blocked on an open question, stop and ask.

## 3. Build

- Use the skill the plan names (`/new-migration`, `/new-bounded-context`, `/new-endpoint`) and follow it.
- Tests first for domain rules and for every bug. Add tests at every level the plan lists.
- Keep to the step's folders. If the work needs a design change, stop and tell the user to re-plan on Opus. Do not widen the scope.
- Rules from the root `CLAUDE.md` apply: no generated backend code, dependency rule, tenant isolation, no `AutoMigrate`, no internal data to clients, phases not status keys, money in integer cents, user text in Brazilian Portuguese with the wording from `docs/product/`.

## 4. Verify

- Run the narrowest command first (one package or one test), then `make lint test`, plus `make test-integration` if adapters changed. Show the output.
- Run the reviewers the plan lists. Fix real findings and rerun.

## 5. Record

- Tick only the boxes this step finished in `docs/build-plan.md`, and update the summary table counts in the same change. Mark partial work in words and leave its box unticked.
- Update `docs/api/openapi.yaml` for any endpoint touched, if you own it this milestone.
- Report what is done, what is not, the test output, and any follow-up for a web or Android counterpart. Commit only if the user asks.
