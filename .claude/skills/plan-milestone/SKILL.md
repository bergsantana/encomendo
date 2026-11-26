---
name: plan-milestone
description: Plan one Encomendo milestone from docs/build-plan.md into ordered, model-tagged steps and write the plan to docs/plans/. Use before building a milestone. Argument is the milestone number, for example `/plan-milestone 2`.
model: opus
---

# Plan a milestone

Planning is where a wrong schema, RLS or cross-context decision gets baked in, so this runs on Opus. It writes a plan and stops. It never writes feature code.

Argument: the milestone number (1 to 8). If it is missing, ask for it.

## 1. Read

- The milestone section in `docs/build-plan.md`, its "Done when" list and its "Blocked" note.
- `docs/architecture.md`: section 4 (contexts), 5 (auth and RLS), 7 (schema), 8 (clients), 9 (testing) and 11 (build order), limited to what the milestone touches.
- The matching section of `docs/product/` for each feature, and the ADRs in `docs/adr/`.
- `backend/CLAUDE.md`, `web/CLAUDE.md` and `mobile/CLAUDE.md` for the apps the milestone touches.

## 2. Stop on open questions

If the milestone depends on a question in `docs/architecture.md` section 12 (payments, store page SEO, custom domains, renaming or building status lists), list it at the top of the plan as **Blocked** and ask the user. Do not choose an answer. Plan the unblocked steps only.

## 3. Write the plan

Write `docs/plans/m<n>-<short-name>.md`. Split the milestone into steps that each fit one PR and one bounded context or one app's feature folder. Number them `<n>.1`, `<n>.2`, and so on. Order them by the dependency rule: domain, app, adapters, HTTP, then web and Android.

For every step write:

- **Goal** in one sentence, and the boxes in `docs/build-plan.md` it ticks.
- **Model**: `opus` or `sonnet`, with a one-line reason. Use Opus for schema, RLS, auth, tenancy, token handling and cross-context design. Use Sonnet where a skill or a written rule already holds the design. Never plan Haiku for code.
- **Skill** to run, if one applies: `/new-migration`, `/new-bounded-context` or `/new-endpoint`.
- **Files and owner**: the folders it touches. Flag steps that need `main.go`, `router.go`, `openapi.yaml` or the migrations folder, and say who owns them this milestone.
- **Tests** at each level the change touches (domain, app with fakes, contract, integration, RLS, handler, web, Android).
- **Reviewers** to run: `go-reviewer`, `web-reviewer`, `mobile-reviewer`, and `tenancy-security-reviewer` for any step that touches migrations, repositories, auth, middleware, cache, store page, tracking or the client area.
- **Depends on** the earlier steps it needs.

Keep the web and Android steps for a management feature as separate steps against the same endpoints, so parity stays visible.

## 4. Record decisions

If you had to settle something `docs/architecture.md` does not cover, write an ADR in `docs/adr/` and add a line to the "Decisions log" in `docs/build-plan.md`. Do not decide anything listed in section 12 silently.

## 5. Hand over

Show the user the step list as a short table (step, goal, model) and the path of the plan. Say which steps need `/model opus` or `claude --model opus` before they start, and that `/build-step <n.m>` builds one step on Sonnet. Do not start building.
