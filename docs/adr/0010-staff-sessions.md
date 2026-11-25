# 0010 — Staff sessions: stores, refresh transport and permissions

- Status: accepted; amended by ADR 0013 (a 2-minute retry window for lost answers; server failures are never 401)
- Date: 2026-11-25

## Context

ADR 0003 fixes the token formats. Writing the `/auth/*` contract left several questions open:

- **Stores.** A user signs up before any store exists, and one user may belong to several stores (the "tenant picker").
- **Refresh token delivery.** The web app must never be able to read the refresh token (httpOnly cookie, ADR 0003). Android has no cookie jar worth trusting and keeps tokens in the Keystore.
- **Refresh and stores.** `refresh_tokens` rows carry `tenant_id` (`docs/architecture.md` section 7), so a refresh keeps the store it was issued for.
- **Permissions.** The apps gate menus with `usePermission` and `PermissionGate`, but the role → permission map lives in Go.
- **The picker's data.** It needs the names of every store the user belongs to, while RLS shows one tenant at a time.
- **Shared domain.** Store pages live on subdomains of the same registrable domain as the admin app, so `SameSite` does not separate them.

## Decision

### Sessions and stores

- **A session may have no store.** Sign-up returns a session without `tenant`, `role` or permissions, and so does login for a user with zero or several stores and no `tenantId` hint. The access token then has `aud=backoffice` and no `tid` or `role`. Store routes answer `403 tenant_required`; only creating a store and switching work. Login takes an optional `tenantId`: the last store used on that device.
- **Switching stores is a refresh.** `POST /auth/switch-tenant {tenantId}` presents the refresh token, rotates it inside its family with the new `tenant_id`, and returns a session for that store. After creating a store, the app switches to it. Creating a store does not issue tokens, so `tenancy` never depends on `identity`'s token issuer.
- **Every session-starting call returns the same `AuthSession`.** It holds the access token, `expiresIn` (900 s), the user, the selected store, the role, the permissions for that role, and every membership. There is no separate "who am I" endpoint: the web app calls `/auth/refresh` on load, and Android on start.
- **Reading the store list under RLS.** Login, refresh, switch and the second transaction of accept run with a user-only scope (ADR 0009) and read through two extra policies, owned by `m2-db`. Both are `SELECT` only and apply only when no tenant is set, so a store's own transactions stay strictly inside that store:
  - `memberships`: `FOR SELECT USING (current_tenant_id() IS NULL AND user_id = current_user_id())`.
  - `tenants`: `FOR SELECT USING (current_tenant_id() IS NULL AND EXISTS (SELECT 1 FROM memberships m WHERE m.tenant_id = tenants.id AND m.user_id = current_user_id()))`.

  Writes stay on the tenant policy alone. A single policy of the form `tenant_id = current OR user_id = me` is never used, because with `WITH CHECK` it would let any user-scoped code insert a membership into any store. The RLS suite adds three checks: a user scope reads only that user's memberships and stores; a user scope cannot insert, update or delete memberships or tenants; and a tenant scope does not see the user's memberships in other stores.
- Finding a store by its slug for the store page, without a tenant set, is a separate need of milestone 7 and is decided there.

### The refresh token

- **Transport.** Sign-up, login and invitation acceptance take `refreshTransport`:
  - `cookie` (the default) sets the refresh cookie and leaves the token out of the body.
  - `body` returns `refreshToken` in the JSON.

  Calls that present a refresh token (refresh, switch, logout) answer the way it came, so a script on the web page cannot ask for the cookie's token in a readable body. Sending both the cookie and a body token is `400 invalid`.
- **The cookie** is `__Host-encomendo_refresh`: `HttpOnly; Secure; SameSite=Strict; Path=/`, with no `Domain`. The `__Host-` prefix makes browsers refuse a cookie of that name set by a store subdomain, which blocks cookie tossing (a script on any `*.encomendo.com.br` page planting its own refresh cookie). Losing the `/auth` path restriction is the price, and it is acceptable: the cookie is httpOnly, and only the `/auth` handlers read it. A request with more than one cookie of the configured name is `401`. A local stack over plain HTTP, where browsers refuse `__Host-` cookies, uses `encomendo_refresh` without `Secure`. An explicit setting chooses between the two, never the request's scheme: TLS ends at Caddy, so the API always sees plain HTTP. The production name is the default, and the config loader refuses the plain one outside development. The server reads and counts only the configured name, so a planted `encomendo_refresh` cookie in production is ignored rather than turned into a forced logout. The client-account cookie on `.encomendo.com.br` (milestone 7) gets a different name.
- **One refresh at a time.** Presenting a refresh token that was already rotated revokes its whole family and answers `401 unauthenticated`, the same as any other bad token. The API never says which case it was. So clients serialize refreshes:
  - The web app holds a Web Lock (`navigator.locks`, name `encomendo-refresh`) around `/auth/refresh`, and tabs take turns. Each turn sends the cookie the previous turn set.
  - Android holds a mutex in the Ktor auth plugin.

  There is no grace window for parallel refreshes. ADR 0013 adds a 2-minute retry window for an answer lost after the server rotated the token.
- **Logout** revokes the family, clears the cookie, adds the access token's `jti` to the Redis denylist when one is sent, and always answers 204.

### CSRF and CORS

- **No CORS.** The web apps call the API on their own origin: the admin host (nginx in development, Caddy in production) routes `/api` and `/auth`. Android does not need CORS. So the API sends no `Access-Control-Allow-*` headers at all, and never `Access-Control-Allow-Credentials`. The `CORS` item of `m2-integrate` becomes "refuse cross-origin browser calls".
- **Every `/auth/*` call is checked.** That covers the cookie calls and the session-starting calls, which also blocks login CSRF:
  - `Content-Type` must be `application/json`, parsed as a media type, with a JSON body.
  - A browser request whose `Origin` is not the admin origin is `403 forbidden`, and so is one whose `Sec-Fetch-Site` is `same-site` or `cross-site`.
  - Native clients send neither header and pass.
- **Client accounts (milestone 7)** log in from store pages and `conta.encomendo.com.br`, on those hosts' own origin. Their endpoints get their own path prefix with their own same-origin rule. The staff rule is never widened to `*.encomendo.com.br`.

### Permissions and limits

- **Role freshness.** The role in the access token can be up to 15 minutes old. Refresh and switch always read the membership again, and a removed member's refresh returns a session without that store. Team management does not rely on the token's role at all (ADR 0011).
- **Enumeration.** Sign-up answers `409 email_taken`. Without email verification it cannot be hidden, so this is accepted. It is limited per IP and per email. Login answers `invalid_credentials` for an unknown email too, and verifies against a dummy argon2id hash so that its timing does not tell either.
- **Email keys.** Every per-email limit keys on the email trimmed and lower-cased, as the database compares it (`citext`).

### New problem codes

- `unauthenticated` (401): any bad or missing token.
- `invalid_credentials` (401): wrong email or password, the same answer for an unknown email.
- `tenant_required` (403).
- `email_taken` (409): sign-up.
- `slug_taken` (409).
- `rate_limited` (429, with `Retry-After`).
- `version_conflict` (409): from section 6.

## Consequences

- `adapter/auth` (`m2-auth`) issues tokens without `tid`/`role` and verifies that both are present or both absent (`shared.NewBackofficePrincipal` refuses anything else).
- `app/identity` needs a port to read a user's memberships and store names, implemented in `adapter/inproc` on top of a `tenancy` query. ADR 0011 explains how that avoids a cycle.
- `m2-db` writes the two user-scope policies above and their RLS tests. `m2-integrate` writes the Origin and media-type checks, the cookie rules and their HTTP tests.
- The web session store keeps the `AuthSession` in memory and wraps refresh in the Web Lock. Android stores the refresh token from the body in `TokenStore` and serializes refresh.
- Telling the user "your session was used elsewhere" is impossible by design. That is accepted: the app sends them to login either way.
