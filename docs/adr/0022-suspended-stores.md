# 0022 — Suspended stores

- Status: accepted
- Date: 2026-11-26

## Context

A store can be `suspended` (`tenants.status`, `0004_tenancy`), and only the platform sets that, by SQL. Until now no code read it, so a suspended store worked like an active one. On 2026-11-26 the user decided (plan `docs/plans/m2-identity-tenancy.md`, Q1, Q5 and Q6):

- a suspended store is refused on every admin request, at once;
- staff read "Esta loja está suspensa. Fale com o suporte do Encomendo.";
- the store picker keeps the store, marked "Suspensa", and it cannot be entered.

What a suspension does to the store page and the client area is decided in milestone 7.

## Decision

### The answer

- `tenancy.ErrTenantSuspended` matches `shared.ErrForbidden`, and the API answers `403 tenant_suspended`. Its title is the message above, which the apps show.
- The apps then go back to the store picker.

### Where it is checked

- **Every tenant-scoped admin request.** A middleware runs after authn on every `/api/v1/admin/*` route except creating a store. It reads the selected store's status through `apptenancy.Service.CheckStore`, in its own transaction under the caller's scope, so RLS still applies.
  - **Uncached:** only SQL suspends a store, so no event could clear a cached status. A cache would delay a suspension by its TTL, so the check costs one primary-key read per request instead.
  - **Fails closed:** a read that fails answers `503 not_ready` with `Retry-After`, never a pass, as an unreadable denylist does (ADR 0013).
  - **No store selected:** a session without a store skips the check. The use cases answer `tenant_required` as before.
- **Requests in flight.** The check runs in its own transaction, so a request already past it when a suspension commits still finishes. That is accepted as "at once".
- **Selecting a store.** A suspended store is never the selected store of a session:
  - login with a hint for a suspended store, or with a suspended store as the only one, gives a session without a store;
  - refreshing a session whose store was suspended gives a session without a store, so the client lands on the picker;
  - switching into a suspended store is `403 tenant_suspended`, and the token stays as it was (ADR 0013).
- **Invitations.** Previewing or accepting an invitation to a suspended store is `403 tenant_suspended`, because accepting writes into the store. A suspension that commits between accept's read and its join still adds the membership. That is harmless: the session selects no suspended store, and every admin route refuses it.
- **Creating a store** is not checked: it works without a store and writes only into the new one. So the owner of a suspended store can create another one. Whether a suspension should stop the person as well as the store is a product question for when suspensions have a reason, such as billing.

### The store picker

Each membership in a session answer gains `status`, `active` or `suspended`, so the apps can mark the store. A suspended store stays in the list, unlike a store the user was removed from (ADR 0010). Otherwise an owner whose only store is suspended would see no store and think the data was gone.

## Consequences

- `apptenancy.TenantSummary` and `appidentity.TenantSummary` carry `Suspended`, and `adapter/inproc` maps it.
- The access token's store never names a suspended store, because sessions never select one. A token issued before the suspension still names it, and the per-request check stops it.
- `docs/product/design-document.md` gets the wording beside "Escolha a loja".
- Web (step 2.9) and Android (step 2.10) mark the store, show the message, and go back to the picker on `tenant_suspended`.
