# 0011 — Staff invitations, password reset and email in milestone 2

- Status: accepted; "No context cycle" refined by `m2-domain`
- Date: 2026-11-25

## Context

Milestone 2 must let an owner invite staff and change their roles on web and Android, and the Android plan lists a "forgot password" screen. `docs/architecture.md` 4.4 sends staff invites and password resets by email (`adapter/notify/smtp.go`), but the build plan only lists the SMTP notifier in milestone 7. Neither invitations nor password resets have a table in section 7, and "Convite" is not in the glossary.

The project owner decided on 2026-11-25:

- Email goes out in milestone 2, through SMTP.
- Milestone 2 also lets the owner or an admin remove a member. Changing a role to viewer still leaves read access, so removal is the only way to cut off someone who left.

## Decision

### Email

- `adapter/notify/smtp.go` lands in milestone 2 (`m2-integrate`), on the standard library's `net/smtp`, with no new dependency. The development compose stack gets a mail catcher (Mailpit).
- Message text is pt-BR, plain text, and lives in the notify adapter. Store and person names go in as plain text and are never presented as links.
- Each context sends through its own small port: `tenancy` for invitations, `identity` for reset codes.

### Invitations ("Convite" → `tenancy.Invitation`)

- **Who and what.** An owner or admin (`members:manage`) invites an email with any role but owner.
- **The link** is `https://{admin host}/convite#token=…`. The token sits in the fragment, so it never reaches a server log. It lasts 7 days and works once. Re-inviting the same email replaces the pending invitation (new token, new expiry). Inviting someone already on the team answers `409 already_member`.
- **Limits.** Per day: at most 20 invitations per store, 50 per inviting user across all their stores, and 3 per (store, email), so neither a new store nor one account with many stores can turn Encomendo into a mail relay. The per-email limit is per store, so a hostile store cannot use up a person's quota and block a real store's invitation. Creating stores is limited too: 5 per user per day. All answer `429 rate_limited`.
- **Invitation tokens carry their tenant.** The token is the tenant id plus 32 random bytes; only the SHA-256 of the random part is stored. Accepting reads the tenant id from the token, scopes the transaction to that tenant and looks up the hash under RLS. No policy and no `SECURITY DEFINER` function lets anyone read invitations across tenants. The tenant id is not a secret; the random part is. The accept transaction does only what accepting needs, because its scope comes from a token presented before login.
- **Accepting** (`POST /auth/invitations/accept`) belongs to `identity`, because it creates or checks the account and returns an `AuthSession`.
  - A new email gives a name and a password. An existing account must give its password, so holding a link never logs anyone in as someone else.
  - It runs as two transactions (ADR 0009): one scoped to the tenant inside the token uses up the invitation and adds the membership; one scoped to the user builds the session.
  - `POST /auth/invitations/preview` tells the app which form to show.
  - Wrong, expired, revoked and used tokens all answer `404 not_found`.
  - So does an invitation whose sender no longer holds `members:manage` in that store.
- **If the invitation email cannot be sent**, the invitation stays and the call answers `503 email_unavailable`. Inviting again replaces it. The outbox relay arrives in milestone 6 and can take this over.

### Team management re-reads the caller's role

- Invite, revoke invitation, change role and remove member check `members:manage` against the caller's membership as read inside their own transaction, not against the role in the access token. A demoted or removed admin therefore loses these powers at once, not 15 minutes later.
- Removing or demoting a member also revokes the pending invitations that member sent. Together with the accept-time check above, nobody can keep a way back in through an invitation sent before they lost access.
- **Removing a member** (`DELETE /api/v1/admin/members/{userId}`) deletes the membership and keeps the account. Their session in that store ends at the next refresh.
- The owner can never be removed, and no one becomes owner by role change or invitation. Ownership transfer is out of scope.

### Password reset ("Esqueci a senha")

- **Requesting.** `POST /auth/password-reset {email}` always answers 202, so it never reveals who has an account. The answer never waits for SMTP: the code is stored in the transaction, and the notify adapter sends it off the request path, so timing does not reveal it either. A failure to send is logged and still answers 202.
  - *Accepted in `m2-domain`:* a known email stores a code, an unknown one stores nothing, and a careful prober can time that write. It tells no more than sign-up's `409 email_taken` already does (ADR 0010), and the limits of 5 requests per email per hour and per IP bound it. Hiding it would need a write for every unknown email.
- **The code** has 6 digits, lasts 10 minutes and allows 5 wrong attempts. A new request replaces the old code. Each account also allows 10 wrong codes per 24 hours across all its codes, so requesting new codes does not buy more guesses. Requests are limited to 5 per email per hour.
- **Counting** is one atomic statement: `UPDATE … SET attempts = attempts + 1 WHERE … AND attempts < 5 RETURNING …`.
- **Storage.** The code is stored as an HMAC-SHA256 under a server key, not a plain hash, because six digits are cheap to brute-force from a leaked table.
- **Confirming.** `POST /auth/password-reset/confirm {email, code, password}` sets the password and revokes every refresh family of the account. A wrong, expired or used-up code answers `400 invalid_code`.
- A code works the same on web and Android, with no deep links.
- **Accepted trade-off:** anyone who knows an email can spend its 10 wrong codes and block password reset for that account for up to 24 hours. Logging in with the current password still works, and the budget is what stops guessing. Support can clear the counter by hand.

### Store addresses

The reserved slugs are the labels of the configured admin and API hosts, plus `www`, `api`, `admin`, `app`, `conta`, `mail` and `static`. That is the list the store app uses in `web/apps/store/src/site.ts`. A test keeps both lists in step.

### New tables (owned by `m2-db`)

- `invitations` in `0004_tenancy`: tenant-scoped with RLS; `email citext`, `role`, `token_hash`, `invited_by`, `expires_at`, `accepted_at`, `revoked_at`; at most one pending invitation per `(tenant_id, email)`.
- `password_reset_codes` in `0002_identity`: global like `users` and `refresh_tokens`; `user_id`, `code_hash`, `expires_at`, `attempts`, `used_at`.

### No context cycle

`identity` reads memberships from `tenancy` (login, switch, accept), and `tenancy` reads users by email from `identity` (invite). Each side exposes a read-only query type that depends only on its own repositories. `adapter/inproc` implements the other side's port on top of it. `main.go` builds both query types before either service, so no service needs the other service.

*Refined in `m2-domain`:* accepting also writes in `tenancy` (it uses the invitation up and adds the membership). `app/tenancy.Lookup` does that next to its queries. Like them it depends only on `tenancy`'s repositories and the event publisher, so the rule above still holds: `identity.Directory` and `tenancy.Lookup` come first in `main.go`, then both services. Accepting reads the invitation in one short transaction scoped to its store before argon2id runs, then uses it up in a second one scoped to the store, then builds the session in a third scoped to the user (ADR 0013: argon2id never runs inside a transaction).

## Consequences

- `m2-integrate` adds `smtp.go`, the SMTP and admin-host settings in `platform/config`, the HMAC key, and Mailpit in `deploy/docker-compose.dev.yml`. Production needs an SMTP provider; choosing one is a deployment task.
- Web gets a forgot-password screen in milestone 2 too, for parity with Android. Both apps get the members screens: list, invite, change role, remove.
- The invitation link opens the web admin app, which hosts the accept screen on any device. Android can claim the link with App Links later; that needs `assetlinks.json` on the admin host and is not part of milestone 2.
- Staff emails are still not verified at sign-up. An invitation proves only that someone could read the invited mailbox. Email verification is out of scope for milestone 2.
- The glossary gains "Convite" → `tenancy.Invitation`, "Equipe, membro" → `tenancy.Membership`, and "Papel" → `shared.Role`.
