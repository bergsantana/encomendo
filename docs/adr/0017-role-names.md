# 0017 — What the four roles are called

- Status: accepted
- Date: 2026-11-25

## Context

A store's team has four roles in code: `owner`, `admin`, `staff` and `viewer` (`shared.Role`, docs/architecture.md 5.4). The Members screens ("Equipe") on web and Android will show them, and invitations offer every role but owner. The docs named none of them in pt-BR; only "Dono" appeared, once, in the design document. CLAUDE.md asks for a decision and an ADR before a new term is used.

## Decision

The apps show the roles as:

| Code | Shown as |
| --- | --- |
| `owner` | Dono |
| `admin` | Admin |
| `staff` | Funcionário |
| `viewer` | Visualizador |

- The names are product wording, the same in every store and niche; they do not come from the template.
- They live in `docs/product/design-document.md` ("Naming", "Roles"), next to what each role can do. Web and Android copy them from there.
- Code keeps the English values from the contract, and never branches on the shown name.

## Consequences

- The web Members feature and Android's `feature/members` map `Role` to these names in one place each, with a test.
- "Funcionário" is grammatically masculine; it is read as generic. A neutral name can replace it later by changing that one map on each app and this document.
