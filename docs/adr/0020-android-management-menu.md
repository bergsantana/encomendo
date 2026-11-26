# 0020 — The Android management menu is a navigation drawer

- Status: accepted
- Date: 2026-11-25

## Context

The web admin has a menu on the left: Hoje · Pedidos · Compras · Produção · Estoque · Catálogo · Clientes · Financeiro, then Equipe (`web/apps/admin/src/layouts/DashboardLayout.vue`). An entry with a `permission` shows only to roles that have it, and its route sends anyone without it back to Hoje (`router/guards.ts`). Hoje is open to every role.

Until now the Android management graph had one screen, Hoje, under the account bar. Equipe is its first screen behind a permission (`members:manage`, owner and admin). `docs/architecture.md` does not say what shape the Android menu takes.

## Decision

- **A modal navigation drawer.** Material 3's `ModalNavigationDrawer`, opened by a "Menu" button at the start of the account bar.
  - It holds the same entries as the web menu, in the same order and with the same labels. Each feature adds its entry when it ships (`MenuEntry` in `core:navigation`).
  - A bottom bar was rejected because it holds five entries at most, and the menu will have nine.
  - Tabs were rejected because they would crowd the workshop screens.
- **Entries follow the session's permissions.** An entry with a permission shows only when the session grants it (`MenuEntry.allowedBy`, fed from `LocalGrantedPermissions`). As on the web, this only hides UI: the API checks every call.
- **A screen behind a permission leaves for Hoje once the session loses it.** Its route sits inside a `PermissionGate` whose fallback pops back to Hoje. A role changed on this device, or refused by the API, renews the session first (`ChangeMemberRole`, `RemoveMember`), so the screen leaves as soon as the session catches up, as the web's `session.refresh()` does. A change made on another device shows at the next renewal: the app's next start, or a refusal.
- **Entries open on top of Hoje.** Back from any entry goes to Hoje, and Back from Hoje leaves the app. Tapping the open entry again does nothing.
- **"Menu" is a text button** until the app has an icon set. The account bar's 56 dp targets apply to it.

## Consequences

- Each management feature adds a `MenuEntry`, a destination and its route in `backofficeNavGraph`, guarded by a `PermissionGate` when the web route has a `meta.permission`.
- Produção and Estoque, shown only when the store turns them on, will need a second kind of filter (by store settings). It joins `MenuEntry.allowedBy` when those features ship.
- The drawer's coroutine scope only opens and closes the drawer. No request runs in it (ADR 0018).
- Leaving Equipe while a change to one's own row is saved cancels it with the screen, renewal included, since the screen's requests end with it (ADR 0018). If the server made the change anyway, Equipe stays in the menu until the next renewal: the next start, or the `forbidden` that opening Equipe then gets.
