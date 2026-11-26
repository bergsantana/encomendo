# Milestone 2 — Identity and tenancy: the rest of the plan

Written 2026-11-25 on Opus with `/plan-milestone 2`. Checked again against `main` on 2026-11-26 (`cab175e`). Build each step with `/build-step 2.<n>`.

Milestone 2 is mostly built. On `main` today: the contract, migrations, RLS suite, domain, app services, `adapter/auth`, data adapters (PRs up to #31), every web screen including Equipe, and every Android screen including Equipe (up to #36). This plan covers what is left. It replaces the 5b and 5c split in `docs/parallel-sessions.md`:

| Old name | Steps here |
| --- | --- |
| `m2-integrate` 5b, HTTP | 2.1 to 2.4 |
| Suspended stores (new, Q1) | 2.5 backend, 2.9 web, 2.10 Android |
| `m2-integrate` 5c, wiring and end to end | 2.6 and 2.7 |
| Web, after 5c | 2.8 |
| Android members (two PRs) | done in #35 (the team and the menu drawer, ADR 0020) and #36 (invitations) |
| Close the milestone | 2.11 |

## The user's answers

None of the open questions in `docs/architecture.md` section 12 (payments, store page SEO, custom domains, status lists) touches milestone 2. These questions did. The user answered all of them on 2026-11-26.

| # | Question | Answer | Where it lands |
| --- | --- | --- | --- |
| Q1 | What does a suspended store stop? | Every admin request, at once | New steps 2.5, 2.9 and 2.10, then 2.7 and 2.11 |
| Q2 | Should store names sort the way Portuguese sorts them? | Yes, but later, in milestone 8 | A milestone 8 line, written in 2.11 |
| Q3 | Where does the tenant plan go? | With billing, not in milestone 2 | 2.11 |
| Q4 | Who runs the Android check against the real API? | The session, with the emulator in Docker | 2.11 |
| Q5 | What do staff read when a store is suspended? | "Esta loja está suspensa. Fale com o suporte do Encomendo." | 2.5, 2.9 and 2.10 |
| Q6 | Does the store picker show a suspended store? | Yes, marked "Suspensa", and it cannot be entered | 2.5, 2.9 and 2.10 |
| Q7 | How does the emulator container get KVM? | Try Docker first | 2.11 |

### What the answers mean

- **Q1, Q5 and Q6: suspended stores.**
  - Today `tenants.status` can be `suspended` (`0004_tenancy`), only SQL sets it, and no code reads it.
  - From step 2.5, every tenant-scoped admin route and switch refuse a suspended store, and a refresh leaves the session without a current store. Web and Android show the Q5 message.
  - In "Escolha a loja", the store stays in the list with "Suspensa" beside it. Tapping it shows the Q5 message. The membership list in the contract gains a status field.
  - No support channel is defined yet, so the message names none.
  - The wording goes into `docs/product/design-document.md` beside "Escolha a loja", because user text must come from the product documents.
  - What a suspension does to the store page and the client area is decided in milestone 7.
- **Q2: sort order.** The store picker keeps the byte order for now, so "Água" comes after "Zebra". 2.11 adds a milestone 8 line: compare names with a pt-BR collation in the repository and the fake, and state the order in the port contract suite.
- **Q3: the tenant plan.** 2.11 does three things:
  - takes the plan out of the milestone 2 domain box and ticks the box;
  - adds "plans and billing" to the open questions in `docs/architecture.md` section 12;
  - logs the change in the build plan's decision log.
- **Q4 and Q7: the Android check.**
  - Docker here runs inside the WSL VM. That VM has nested virtualization off: no `vmx` or `svm` CPU flags, and no `/dev/kvm` on the host or in a container, checked on 2026-11-26.
  - So the emulator in Docker runs without acceleration at first. Recent emulator builds usually refuse x86_64 images without it, and when they do start, they are very slow.
  - 2.11 gives this attempt a timebox. If it cannot finish the walkthrough, the session asks the user to turn on nested virtualization:
    - add `nestedVirtualization=true` under `[wsl2]` in `%UserProfile%\.wslconfig`;
    - run `wsl --shutdown`, which ends the session;
    - load `kvm_intel` or `kvm_amd`;
    - run the container with `--device /dev/kvm`.

### Assumed unless the user objects

- **The `Select("*")` line in `backend/CLAUDE.md` is reworded in 2.11.** The new wording is "use a map, or `Select` with the column names". The repositories cannot use `Select("*")`, because `encomendo_app` may update only some columns.
- **Merging:**
  - 2.1 to 2.7 merge with `gh` once CI is green and the reviewers find nothing blocking. The user approved this for `m2-integrate` on 2026-11-25.
  - The Android fixes in 2.11 merge the same way, because the mobile approval covered milestone 2 auth and members.
  - Nobody approved merging web work, or Android work beyond auth and members. Ask at 2.8, 2.9 and 2.10.

## Decisions in this plan

- **The store page middleware moves to milestone 7.** `docs/parallel-sessions.md` put it in 5b. But finding a store by its slug without a tenant set is "a separate need of milestone 7 and is decided there" (ADR 0010), `docs/build-plan.md` already lists `middleware/storefront.go` under milestone 7, and milestone 2 has no `/api/v1/store/*` route to mount it on. Milestone 2 still proves the audience rule: authn on admin routes refuses a client token (step 2.1).
- **Step 2.2 writes ADR 0021 (client IP and rate limits).** ADR 0020 is the Android menu, from #35. The new ADR decides which proxy hops to trust, where per-email limits run, what a Redis failure does to a limit, and the key format.
- **Step 2.5 writes ADR 0022 (suspended stores).** It turns the answers to Q1, Q5 and Q6 into the rules the code follows.

## Ownership

Parallel sessions ended on 2026-11-25. The single milestone 2 session owns `backend/cmd/api/main.go`, `backend/internal/adapter/http/router.go`, `docs/api/openapi.yaml` and the migrations folder. No step here adds a migration: 2.5 reads the existing `tenants.status` column. If parallel sessions resume, 2.3, 2.4 and 2.5 own `router.go` and `openapi.yaml`, in that order, and 2.6 owns `main.go`.

## Steps

| Step | Goal | Model |
| --- | --- | --- |
| 2.1 | Problem codes, authn and the cross-origin refusal | opus |
| 2.2 | Rate limits, with ADR 0021 | opus |
| 2.3 | `/auth/*` handlers, cookies and routes | opus |
| 2.4 | Tenancy, members and invitations handlers | sonnet |
| 2.5 | Refuse suspended stores, with ADR 0022 | opus |
| 2.6 | Configuration, keys, wiring and Mailpit | opus |
| 2.7 | End-to-end "Done when" checks and the coverage gate | sonnet |
| 2.8 | Web admin against the real API | sonnet |
| 2.9 | Web: suspended stores | sonnet |
| 2.10 | Android: suspended stores | sonnet |
| 2.11 | Android against the real API in a Docker emulator, and close the milestone | sonnet |

2.10 needs only the contract from 2.5, so it can run beside 2.6 to 2.9. 2.8 and 2.11 need 2.6.

---

### 2.1 Problem codes, authn and the cross-origin refusal

- **Goal:** the middleware and error mapping every M2 route stands on, tested on their own with `httptest` and not mounted yet.
- **Ticks:** nothing alone. It is part of the HTTP box, which ticks in 2.6.
- **Model:** opus. Decides 401 versus 5xx, fail-closed authn and CSRF, where a mistake logs everyone out or lets a token through.
- **Skill:** none.
- **Files:** `adapter/http/respond/problem.go`, `adapter/http/middleware/{authn,crossorigin,deadline}.go`. No owned file.
- **Work:**
  - The ADR 0010 problem codes: `unauthenticated`, `invalid_credentials`, `tenant_required`, `email_taken`, `slug_taken`, `rate_limited`, `version_conflict`.
  - ADR 0019 mappings: `shared.ErrUnavailable` and `context.DeadlineExceeded` → 503 `not_ready` with `Retry-After`, and a canceled request handled on its own and checked first.
  - ADR 0009 mappings: `ErrScopeMismatch` → 403, logged at warn; `ErrTenantRequired` → 403 `tenant_required`.
  - `authn(audience)` calls `Verify` with the route's audience, then `Denylist.Denied`. It checks the **error first**, because `Denied` returns `(false, err)` (PR #31), and answers 503 on that error, never 401 and never a pass (ADR 0013). Then `authz.WithPrincipal`. It logs the refusal reason at debug and never the `Authorization` header.
  - Cross-origin refusal for `/auth/*` (ADR 0010):
    - `Content-Type` parsed as a media type, which must be `application/json`;
    - an `Origin` other than the admin origin is 403;
    - `Sec-Fetch-Site: same-site` or `cross-site` is 403;
    - neither header passes.
  - A deadline middleware for `/auth/*`, longer than `DefaultMaxHashWait` plus one hash (ADR 0019).
- **Tests:** handler tests with fake verifier and denylist. Cover:
  - missing, expired, wrong `aud`, `alg: none`, unknown `kid`, tampered and denylisted tokens, each 401;
  - a client-audience token on the backoffice audience, 401;
  - a denylist error, 503;
  - the native-client case from ADR 0013: no `Origin`, no `Sec-Fetch-Site`, `application/json; charset=utf-8`, passes;
  - every problem mapping, including a commit conflict after cancellation, where the cancellation wins.
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer`.
- **Depends on:** nothing unmerged.

### 2.2 Rate limits, with ADR 0021

- **Goal:** Redis-backed limits for every route the contract marks as limited, with `Retry-After` on every 429.
- **Ticks:** part of the HTTP box (2.6).
- **Model:** opus. Trusting the wrong `X-Forwarded-For` hop lets anyone pick their own IP and skip every limit.
- **Skill:** none.
- **Files:** `adapter/redis/ratelimiter.go` (sliding window, Lua in an embedded `.lua` file, as architecture section 4 lays out), `adapter/http/middleware/ratelimit.go`, ADR 0021. No owned file.
- **ADR 0021 decides:**
  - **Client IP.** Production is Caddy → nginx → api for the admin host and Caddy → api for the API host. Development is nginx → api. The recommended rule: walk `X-Forwarded-For` from the right, skip addresses in a configured list of trusted proxy CIDRs, and take the first untrusted one. Fall back to `RemoteAddr`. Never take the leftmost value.
  - **Per-email and per-user limits** run in the handler after decoding, through a small `Limiter` interface next to the handlers. Per-IP limits run in the middleware. Email keys are trimmed and lower-cased (ADR 0010).
  - **Invitation caps:** 20 per store, 50 per inviter and 3 per email per day (`openapi.yaml`, `inviteMember`). None of this exists in `app/tenancy` today. Recommended: daily windows in the same limiter, keyed after authn.
  - **When Redis fails.** Recommended: fail open with an error log, as architecture section 4 asks of the cache. The argon2id memory cap (ADR 0019) still bounds login floods. The denylist is the exception: it fails closed (ADR 0013).
  - **Keys:** `g:rl:{route}:{kind}:{value}`, hashing emails so they never sit in Redis in clear.
- **Tests:**
  - unit: IP extraction with a spoofed leftmost hop, one and two trusted hops, IPv6 and a malformed header;
  - integration (`test/integration/redis`): sliding window edges, `Retry-After` ≥ 1 and in whole seconds, Redis stopped means pass and log;
  - middleware: a 429 carries `Retry-After`.
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer`.
- **Depends on:** 2.1, for the `rate_limited` code.

### 2.3 `/auth/*` handlers, cookies and routes

- **Goal:** the nine `/auth/*` operations served from the router, with the cookie and transport rules of ADR 0010 and the failure answers of ADR 0013.
- **Ticks:** part of the HTTP box (2.6).
- **Model:** opus. Cookie names, transport mixing and refresh answers decide whether a session can be stolen or wrongly ended.
- **Skill:** `/new-endpoint` for the handler, DTO and test layout. Its OpenAPI step changes only the two fixes below.
- **Files:** `adapter/http/handler/auth*.go`, `handler/dto/`, `router.go` (owned), `docs/api/openapi.yaml` (owned).
- **Work:**
  - The operations: `signUp`, `logIn`, `refreshSession`, `switchTenant`, `logOut`, `requestPasswordReset`, `confirmPasswordReset`, `previewInvitation`, `acceptInvitation`. Each sits behind the 2.1 cross-origin check, the deadline and the 2.2 limits.
  - **Cookies:**
    - the name comes from config, `__Host-encomendo_refresh` or `encomendo_refresh`;
    - `HttpOnly; Secure; SameSite=Strict; Path=/` with no `Domain`, and no `Secure` on the plain name only;
    - only the configured name is read and counted, and more than one is 401.
  - **`refreshTransport`:**
    - `cookie` by default, `body` on request;
    - refresh, switch and logout answer the way the token came;
    - a cookie and a body token together is 400.
  - **Logout** revokes the family from any token of it, even one a refresh just rotated (ADR 0016). It denylists the access `jti` when one is sent, clears the cookie, and always answers 204.
  - **Switch** validates the body and the membership before rotating (ADR 0013).
  - Login, refresh and switch read memberships under the user-only scope (PR #31 handoff).
  - **Router:** mount `/auth/*` with its middleware, and make the fallback keep problem+json.
    - Wrap each method-qualified route (`POST /auth/login`) in the cross-origin check, never a `/auth/` prefix. Otherwise `GET /auth/login` answers `400` for its `Content-Type` instead of `405`.
    - Every 401 carries `WWW-Authenticate: Bearer`, `invalid_credentials` included, so the handler tests must not expect it absent on `/auth/login`.
  - **`openapi.yaml`:**
    - add 503 with `Retry-After` on `signUp`, `logIn`, `confirmPasswordReset` and `acceptInvitation`, and 409 `email_taken` on `acceptInvitation` (PR #31 handoff);
    - add the cross-origin answers that 2.1 built to every `/auth/*` operation: 403 `forbidden` on all of them (only `switchTenant` lists a 403 today), and 400 `invalid` for a wrong `Content-Type` on `logOut` and `previewInvitation`, the two that list no 400. `logOut`'s "always answers 204" then needs "after the cross-origin check" (ADR 0010, amended in 2.1).
- **Tests:**
  - handler tests with fake use cases, validated against `openapi.yaml`. This needs `kin-openapi` as a test-only dependency, justified in the PR;
  - each cookie rule, with the plain-name and production-name cases;
  - both transports, and mixing them;
  - refresh answers 5xx and not 401 when the store fails;
  - logout with a just-rotated refresh token revokes the family (asked by the web session);
  - a 429 with `Retry-After` on every limited `/auth` route;
  - router tests: every `/auth` route has the cross-origin check, and unmatched paths are problem+json.
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer`.
- **Depends on:** 2.1, 2.2.

### 2.4 Tenancy, members and invitations handlers

- **Goal:** the nine `/api/v1/admin/*` operations served behind authn with the backoffice audience.
- **Ticks:** part of the HTTP box (2.6).
- **Model:** sonnet. The authorization is already in `app/tenancy`, and `/new-endpoint` holds the handler shape. The mounting rules come from 2.1 and are tested here.
- **Skill:** `/new-endpoint`, from step 3 of it on. The contract is already merged.
- **Files:** `adapter/http/handler/{tenants,settings,members,invitations}.go`, `handler/dto/`, `router.go` (owned).
- **Work:**
  - The operations: `createTenant`, `getSettings`, `updateSettings`, `listMembers`, `removeMember`, `changeMemberRole`, `listInvitations`, `inviteMember`, `revokeInvitation`.
  - The 2.2 invitation caps on `inviteMember`.
  - Cursor pages, where a forged cursor is 400.
  - `version_conflict` on settings.
  - `tenant_required` for a token without a store, everywhere except `createTenant`.
  - The invitation link never appears in a response.
- **Tests:**
  - handler tests for success, 400, 401, 403, 404 and 409, each validated against `openapi.yaml`;
  - router tests: every admin route has authn, a client-audience token is 401, and no token is 401;
  - the 429 caps on `inviteMember`.
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer` (route mounting).
- **Depends on:** 2.3.

### 2.5 Refuse suspended stores, with ADR 0022

- **Goal:** a suspended store is refused at once on every tenant-scoped admin route and on switch, and the store picker can show it as suspended (Q1, Q5, Q6).
- **Ticks:**
  - this step adds three boxes to the milestone 2 section of `docs/build-plan.md`, for the backend, web and Android halves;
  - it ticks the backend box.
- **Model:** opus. A check that fails open, or a cache that hides a suspension, lets a stopped store keep working. A check that fails closed by mistake locks out every store.
- **Skill:** `/new-endpoint`, for the contract change and the handler tests.
- **Files:**
  - `docs/adr/0022-*.md`;
  - `docs/product/design-document.md`, beside "Escolha a loja": "Suspensa" and the Q5 message;
  - the app layer: a small port that reads a store's status, its fake and its contract suite, plus the switch and refresh changes in `app/identity`;
  - `adapter/postgres`, to read the status;
  - `adapter/http/middleware/`, for the check after authn;
  - `respond/problem.go`, for `tenant_suspended`;
  - `docs/api/openapi.yaml` (owned) and `router.go` (owned).
- **ADR 0022 decides:**
  - **Where the check runs.** Recommended: a middleware after authn on every tenant-scoped admin route. `createTenant`, logout, password reset and the auth routes that need no store are not affected.
  - **"At once".** Only SQL suspends a store, so no event can clear a cache. Recommended: read the status uncached, one primary-key read per request. A cache would delay a suspension by its TTL.
  - **A failed read.** Answer 503 `not_ready`, never pass the request. The check fails closed, like the denylist (ADR 0013).
  - **The answers:**
    - an admin request for a suspended store is 403 `tenant_suspended`;
    - so is switching into a suspended store;
    - refreshing a session whose store was suspended returns a session with no current store, so the client lands on the picker. Unlike a removed member's store (ADR 0010), it stays in the list, marked `suspended`;
    - accepting an invitation to a suspended store is refused, because it writes into the store.
  - **The contract.** Each membership in the login, refresh and switch answers gains `status` (`active` or `suspended`), so the picker can mark the store.
- **Tests:**
  - middleware: an active store passes, a suspended one is 403 `tenant_suspended`, and a failed read is 503 and never a pass;
  - router: every tenant-scoped admin route has the check, with the route list read from the router, as 2.4 does for authn;
  - use cases:
    - switching into a suspended store;
    - refreshing after a suspension;
    - accepting an invitation to a suspended store;
  - the status read's port contract, against the fake and against Postgres (`make test-integration`);
  - handler tests validated against `openapi.yaml`.
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer`.
- **Depends on:** 2.5.

### 2.6 Configuration, keys, wiring and Mailpit

- **Goal:** the API serves every M2 route with real adapters, refuses unsafe settings outside development, and the dev stack catches mail.
- **Ticks:**
  - the HTTP box;
  - the `smtp.go` and Mailpit box.
- **Model:** opus. Signing keys, refusals and the composition root are where a leak or a fallback key spreads to every store.
- **Skill:** none.
- **Files:**
  - `platform/config`;
  - `cmd/api/main.go` (owned);
  - `deploy/docker-compose.dev.yml`, `deploy/docker-compose.yml`, `.env.example`, `deploy/prod.env.example`;
  - `deploy/web/nginx.conf.template`.
- **Work:**
  - **Config:**
    - the signing seed, verify-only seeds and issuer;
    - the cookie name;
    - the admin origin;
    - trusted proxy CIDRs (ADR 0021);
    - SMTP settings and `RequireTLS`;
    - the reset-code HMAC key.
  - **Refusals outside `development` and `test`:**
    - the dev seed's kid, as signing or verify-only key;
    - a missing signing seed, with no fallback key;
    - the plain cookie name;
    - `RequireTLS=false`;
    - an `http://` admin origin.
  - **Refused everywhere:** an HTTP write timeout at or under `middleware.AuthDeadline` (10 seconds), which would cut the connection before a busy `/auth` call sends its 503 (step 2.1 review).
  - **`main.go`:**
    - build every repository, `adapter/inproc` port, the denylist, the limiter, the hasher, the issuer, SMTP, the clock, IDs, services and handlers;
    - log the signing kid and every verify-only kid at start;
    - call `SMTP.Wait()` on shutdown.
  - **The dev and CI stacks:**
    - Mailpit;
    - the fixed dev seed;
    - Redis `maxmemory-policy noeviction`, with a comment that the dev Redis has no persistence (PR #31 handoff).
  - **nginx** (asked by the web session): `Referrer-Policy: same-origin`, and `Cache-Control: no-store` on `index.html`.
- **Tests:**
  - config tests for each refusal and each default;
  - a test that the default argon2id costs pass their bounds (ADR 0019);
  - `make stack-up`, then `/readyz` and one sign-up through nginx by hand, with the output in the PR.
- **Reviewers:** `go-reviewer`, `tenancy-security-reviewer`.
- **Depends on:** 2.4.

### 2.7 End-to-end "Done when" checks and the coverage gate

- **Goal:** `test/e2e` proves the backend "Done when" items against the built stack, in the existing CI `e2e` job.
- **Ticks:**
  - "A staff token cannot read another tenant's rows through the API or the database; an expired or tampered token is rejected on every route";
  - "Coverage gate passes with real code behind it".
- **Model:** sonnet. The checks are written down, and the stack exists after 2.6.
- **Skill:** none.
- **Files:** `backend/test/e2e/`, `.github/workflows/backend.yml` (add Mailpit to the e2e stack).
- **Work:**
  - The full flow: sign up, create a store, switch, invite, read the link from Mailpit's API, accept, change role, remove.
  - A second store: its staff token reads nothing of the first through every admin route. The database half is the merged RLS suite.
  - An expired, tampered, `alg: none` and wrong-audience token on **every** route in `openapi.yaml`, each answered 401. Read the route list from the spec, so a new route cannot skip the check.
  - A store suspended by SQL during the run:
    - the very next request on each tenant-scoped admin route is 403 `tenant_suspended`;
    - a switch into it is refused;
    - the login answer marks it `suspended`.
- **Tests:** these are the tests. Run them with `go test -tags e2e ./test/e2e/...` against `make stack-up`, then run the coverage gate.
- **Reviewers:** `go-reviewer`.
- **Depends on:** 2.6.

### 2.8 Web admin against the real API

- **Goal:** the admin app's auth and Equipe flows work against the real stack, and the Playwright suite stops stubbing the API.
- **Ticks:** the web half of "Sign-up → create tenant → log in works on web and Android".
- **Model:** sonnet. The screens exist. This step finds and fixes the differences.
- **Skill:** none.
- **Files:** `web/apps/admin/`, `web/packages/api/`, `web/e2e/admin.spec.ts`.
- **Work:**
  - Run `make stack-up`.
  - Walk through sign-up, create a store, log in, switch, forgot password (code from Mailpit), invite, accept, change role and remove.
  - Fix what differs from the stubs.
  - Replace the `page.route` stubs in `web/e2e/admin.spec.ts` with the real stack.
- **Tests:** Playwright against the stack, with unit tests for any fix.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 2.6.

### 2.9 Web: suspended stores

- **Goal:** "Escolha a loja" shows a suspended store with "Suspensa" beside it, and the admin app shows the Q5 message whenever the API answers `tenant_suspended`.
- **Ticks:** the web suspended-store box that 2.5 adds.
- **Model:** sonnet. 2.5 sets the contract, the wording and the behaviour.
- **Skill:** none.
- **Files:**
  - `web/packages/api/`: types generated again from `openapi.yaml`, and the problem mapping;
  - `web/apps/admin/src/features/auth/`: the store picker;
  - the session store.
- **Work:**
  - Mark a suspended store "Suspensa" in the picker. Tapping it shows "Esta loja está suspensa. Fale com o suporte do Encomendo." and does not switch.
  - On a `tenant_suspended` answer anywhere in the app, show the same message and go back to the picker.
- **Tests:**
  - unit tests for the picker and the problem mapping;
  - a Playwright case against the real stack, with the store suspended by SQL.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 2.8, for the stub-free Playwright suite.

### 2.10 Android: suspended stores

- **Goal:** the same as 2.9, on Android.
- **Ticks:** the Android suspended-store box that 2.5 adds.
- **Model:** sonnet. Same sources as 2.9.
- **Skill:** none.
- **Files:**
  - the store picker in `mobile/shared/feature/auth/` (`StorePicker*`);
  - `tenant_suspended`:
    - in `core:network`'s `ApiError`;
    - as a `Failure` in `core:domain`;
    - with its message in `core:ui`'s `FailureMessage` (#34);
  - the membership DTO.
- **Work:**
  - The same picker mark, message and return to the picker as 2.9.
  - The session renews after `tenant_suspended`, as it already does after `forbidden`.
- **Tests:**
  - view model and repository tests against a Ktor `MockEngine`;
  - `./gradlew check` through `scripts/mobile-docker.sh`.
- **Reviewers:** `mobile-reviewer`.
- **Depends on:** 2.5. It needs only the merged contract.

### 2.11 Android against the real API in a Docker emulator, and close the milestone

- **Goal:** the Android flows work against the real stack, driven by this session in an emulator running in Docker (Q4, Q7), and every milestone 2 box is ticked.
- **Ticks:**
  - "Sign-up → create tenant → log in works on web and Android against the same endpoints";
  - "Members: invite, change role and remove on web and Android";
  - the tenancy domain box, once the plan moves to billing (Q3).
- **Model:** sonnet. A check, one script and a docs pass.
- **Skill:** none.
- **Files:**
  - `scripts/android-emulator-docker.sh`, which is new;
  - `mobile/` fixes;
  - `docs/build-plan.md`;
  - `docs/parallel-sessions.md`;
  - `docs/architecture.md` sections 4.2 and 12;
  - `backend/CLAUDE.md`, the `Select("*")` line.
- **Work:**
  - **The emulator.** The script builds on the SDK image that `scripts/mobile-docker.sh` already uses. It installs the emulator and one x86_64 system image into the SDK volume and starts the emulator headless.
    - Run the container with `--network host`. Then `10.0.2.2:8081`, which the debug build calls, is the container's loopback, which is the WSL host's, where `make stack-up` listens.
    - The session drives the app with `adb` (input, `uiautomator dump`, `screencap`) and reads the screenshots.
    - The PR justifies the emulator as a new development tool.
  - **Without KVM first (Q7).** There is no `/dev/kvm` today, so the first try runs without acceleration. If the emulator refuses the image, or cannot finish the walkthrough within an hour, stop. Then ask the user to turn on nested virtualization (see Q4 and Q7), and run again with `--device /dev/kvm`.
  - **The walkthrough.**
    - The flows: sign up, create a store, log in, renew, switch, forgot password with the code from Mailpit, and the Equipe flows.
    - A store suspended by SQL, which the picker must show as "Suspensa".
    - Equipe's keyboard inset and focus.
    - Fix what differs.
  - **Docs:**
    - Add the glossary rows for Equipe and papel, Convite, Esqueci a senha and loja suspensa to `docs/architecture.md` 4.2.
    - Take the plan out of the domain box (Q3). Add "plans and billing" to `docs/architecture.md` section 12, and log the change in the build plan's decision log.
    - Add the milestone 8 line for pt-BR store sorting (Q2).
    - Update the build-plan summary.
    - Reword the `Select("*")` line in `backend/CLAUDE.md`.
    - In `docs/parallel-sessions.md`, record milestone 2 as done and the store page middleware as moved to milestone 7.
- **Tests:**
  - unit tests for any fix;
  - `./gradlew check` and `:androidApp:assembleDebug`;
  - the walkthrough, recorded in the PR with its screenshots.
- **Reviewers:** `mobile-reviewer`.
- **Depends on:** 2.6, 2.10.
