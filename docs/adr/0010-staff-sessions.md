# 0010 — Staff sessions: stores, refresh transport and permissions

- Status: accepted
- Date: 2026-11-25

## Context

ADR 0003 fixes the token formats. Writing the `/auth/*` contract left open how a session relates to stores, how the web app and the Android app receive the refresh token, and how the apps know what to show:

- A user signs up before any store exists, and one user may belong to several stores ("tenant picker").
- The web app must never be able to read the refresh token (httpOnly cookie, ADR 0003); Android has no cookie jar worth trusting and keeps tokens in the Keystore.
- `refresh_tokens` rows carry `tenant_id` (`docs/architecture.md` section 7), so a refresh keeps the store it was issued for.
- The apps gate menus with `usePermission` and `PermissionGate`, but the role → permission map lives in Go.

## Decision

- **A session may have no store.** Sign-up, and login for a user with zero or several stores and no `tenantId` hint, return a session without `tenant`, `role` or permissions. Its access token has `aud=backoffice` and no `tid` or `role`. Store routes answer `403 tenant_required`; only creating a store and switching work. Login takes an optional `tenantId` (the last store used on that device).
- **Switching stores is a refresh.** `POST /auth/switch-tenant {tenantId}` presents the refresh token, rotates it inside its family with the new `tenant_id`, and returns a session for that store. After creating a store, the app switches to it. Creating a store does not issue tokens, so `tenancy` never depends on `identity`'s token issuer.
- **Every session-starting call returns the same `AuthSession`:** access token, `expiresIn` (900 s), user, selected store, role, permissions for that role, and every membership. There is no separate "who am I" endpoint: the web app calls `/auth/refresh` on load and Android on start.
- **Refresh transport.** Sign-up, login and invitation acceptance take `refreshTransport`: `cookie` (default) sets `encomendo_refresh` (`HttpOnly; Secure; SameSite=Strict; Path=/auth`, host-only, so store subdomains never receive it) and omits it from the body; `body` returns `refreshToken` in the JSON. Calls that present a refresh token (refresh, switch, logout) answer the way it came. A script on the web page therefore cannot ask for the cookie's token in a readable body.
- **CSRF.** Cookie-authenticated calls require `Content-Type: application/json` and a JSON body, which forces a CORS preflight from any other origin. Store pages share the registrable domain, so SameSite alone is not enough.
- **Role freshness.** The role in the access token can be up to 15 minutes old. Refresh and switch always read the membership again; a removed member's refresh returns a session without that store.
- **Reuse.** Presenting a refresh token that was already rotated revokes its whole family and answers `401 unauthenticated`, the same as any other bad token. The API does not tell the client which case it was.
- **Logout** revokes the family, clears the cookie, adds the access token's `jti` to the Redis denylist when one is sent, and always answers 204.
- New problem codes: `unauthenticated` (401, any bad or missing token), `invalid_credentials` (401, wrong email or password, the same for an unknown email), `tenant_required` (403), `email_taken` (409, sign-up), `slug_taken` (409), `rate_limited` (429 with `Retry-After`), and `version_conflict` (409) from section 6.

## Consequences

- `adapter/auth` (`m2-auth`) issues tokens without `tid`/`role` and verifies that both are present or both absent (`shared.NewBackofficePrincipal` refuses anything else).
- `app/identity` needs a port to read a user's memberships and tenant names, implemented in `adapter/inproc` on top of a `tenancy` query (ADR 0011 explains how that avoids a cycle).
- The web session store keeps the `AuthSession` in memory; Android stores the refresh token from the body in `TokenStore`.
- Telling the user "your session was used elsewhere" is impossible by design. That is accepted: the app sends them to login either way.
