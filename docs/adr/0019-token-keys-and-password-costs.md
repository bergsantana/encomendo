# 0019 — Access-token keys, password hashing costs and token checks

- Status: accepted; refines ADR 0003
- Date: 2026-11-25

## Context

ADR 0003 chose JWT access tokens signed with Ed25519, named by `kid`, and argon2id for staff passwords. It left open:

- how a key is configured, named and kept out of logs;
- how a key rotation runs while more than one API instance is up, and what to do after a leak;
- which argon2id costs to use, and how much memory and waiting hashing may take;
- what Verify does with input the issuer never writes.

`internal/adapter/auth` needs all four.

## Decision

### Keys

- **Format:** a key is the 32-byte Ed25519 seed in padded standard base64, as `openssl rand -base64 32` prints it. Any 32 random bytes are a valid seed. A seed of one repeated byte, such as an all-`A` placeholder, is refused.
- **The key set:** one key signs. The others are verify-only, and only their public halves are kept.
- **The kid:** the RFC 7638 thumbprint of the public key (its RFC 8037 JWK). The configuration never names a kid, and every instance computes the same one.
- **One use:** this key set signs access tokens and nothing else. Tokens carry `typ: at+jwt` (RFC 9068), and Verify refuses any other `typ`.
- **Never printed:** `auth.Key`, `auth.KeySet` and `auth.AccessTokens` print their kids only, through every `fmt` verb and both slog handlers. The private key is held only by a signing function, so even reflection over an unexported config field shows an address, not the key. A test checks every verb and handler.

### Rotation

A planned rotation takes three deploys:

1. Add the next key as verify-only on every instance.
2. Sign with the next key, and keep the old one verify-only.
3. Drop the old key, at least `identity.DenylistWindow` (15 minutes 30 seconds) after step 2 is everywhere.

Step 1 exists because, during a rolling deploy, an instance that does not yet know the new key would refuse tokens that an updated instance signed. With a single instance, steps 1 and 2 can be one deploy.

Verify-only keys are configured as seeds, so an operator only moves a value from one setting to the other. A configuration leak already exposes the signing key, so keeping a seed adds nothing to that leak. The real risk is forgetting step 3; at startup the API logs every kid it verifies (`KeySet.KIDs`), so a leftover key shows.

### After a leak

1. **Replace the key everywhere at once.** Replace the signing key with a new one, and drop the leaked key on every instance in the same deploy. Skip the three steps above: either stop every instance first, or accept that clients get 401s until the last instance is updated. Each client refreshes once on a 401 (ADRs 0012 and 0013). Refresh tokens are opaque and do not depend on the key, so the leak costs each client one refresh, not a login.
2. **Repair what forged tokens changed.** A forged owner token can change data that outlives the key, such as inviting the attacker to a store. Estimate how long the key was exposed. Then audit the invitations, memberships and role changes made in that time (`audit_log`), revert them, and revoke the refresh families that their acceptances created.

### Tokens

- **Library:** golang-jwt v5, which `docs/architecture.md` names, with the algorithm pinned to EdDSA.
- **`aud`:** one string, never an array. A token issued for both audiences would pass both route checks.
- **Verify needs an audience:** the caller names the one the route takes, `backoffice` or `client`. Any other value is a wiring bug and answers 500, not 401, because a 401 would send every client into a refresh loop.
- **Time claims:**
  - `iat` is required, and refused when it is more than the 30-second leeway in the future;
  - `exp` is required;
  - `nbf` is checked when present.
- **Lifetime:** `exp − iat` must be more than zero and at most `identity.AccessTokenLifetime`, on Issue (`appidentity.CheckAccessClaims`, which the fake issuer runs too) and on Verify. Revocation only denylists tokens issued within `identity.DenylistWindow`, so a longer-lived token would escape it.
- **Headers:** a token with a `crit` header is refused. RFC 7515 says an unknown `crit` extension must be refused, and this verifier knows none.
- **Before parsing:** a token over 2 KiB is refused, and so is any byte outside base64url and `.`. Go's base64 decoder skips line breaks even in strict mode, so a line break inside the signature would be a second spelling of a valid token. A fuzz test found this.
- **Refusals:** every refusal is `auth.ErrInvalidToken`, a 401. The reason goes into the error as text, never wrapped, so no part of a bad token can turn into a 400. The reason is capped at 200 bytes, because it can quote claim text a forger chose.

### Passwords

- **Costs:** argon2id with the second recommended option of RFC 9106: 64 MiB, 3 passes, 4 lanes, a 16-byte salt and a 32-byte key.
- **Format:** each hash is written as a PHC string, `$argon2id$v=19$m=65536,t=3,p=4$salt$key`. Each hash carries its own costs, so raising the costs later keeps old hashes working.
- **The first change of costs** must add rehash on login. Until then, an unknown email (hashed at the current costs) and an account still on the old costs take different times, and that difference tells an attacker which emails have accounts.
- **Costs only go up.** A stored hash that needs more memory than the whole budget is refused. So lowering the memory, or the number of slots, below what the stored hashes need would refuse every login with a 500. Lowering either needs a rehash of every account first.
- **Memory:** the slots are memory, not a count of hashes. Hashing holds at most 4 times the configured memory at once, 256 MiB at the defaults, however many logins arrive. Each hash takes the memory its own costs name. Go's garbage collector can let the process grow to about twice that.
- **Waiting:** a call waits for its memory at most 3 seconds, and less if its request ends first. After that it answers `auth.ErrBusy`, which matches `shared.ErrUnavailable` and becomes a 503 with `Retry-After`. Without the limit, a flood of logins that stays under the per-IP and per-email rate limits would queue every request behind it.
- **Ended requests:** a request that has already ended gets no work.
- **Reading stored hashes:** stored hashes are read strictly, and only the spelling the hasher writes is accepted. The bounds are:
  - passes and lanes up to 16 each;
  - memory up to the whole budget, so a corrupt row cannot take more memory than all the slots together;
  - salt of 8 to 64 bytes;
  - key of 16 to 64 bytes.

  Anything else is `auth.ErrUnreadableHash`, a 500 and never a wrong password.
- **Accepted: a tampered row can be slow.** A tampered row within these bounds (the whole budget, 16 passes) takes about 2 seconds per login of that account, and logins behind it wait. Anyone who can write that row can already set a password of their own, so the bounds only stop a corrupt row from taking more memory than hashing may.

### Secrets

The 32 bytes of refresh tokens, invitation links and reset codes come from `crypto/rand`, with no fallback. Refresh tokens and invitation links are stored as SHA-256 (`domain/shared.Secret`). Reset codes are stored as HMAC-SHA256 under a server key (`domain/identity`, ADR 0011).

## Consequences

- **What `m2-integrate` adds:**
  - the configuration: the signing seed, the verify-only seeds (a comma-separated list that may be empty), and the issuer (`iss`);
  - a startup log line with the signing kid and every verify-only kid;
  - a fixed development seed in the dev and CI stacks. Outside `development` and `test`, the config loader refuses that seed's kid (`ParseKey(seed).KID()`, compared with `KeySet.KIDs()`), as signing and as verify-only key, and a test proves it. Otherwise anyone with the repository could sign owner tokens for every store. A missing signing seed fails startup; there is no fallback key;
  - in `respond/problem.go`:
    - 401 for `shared.ErrUnauthenticated`;
    - 503 with `Retry-After` for `shared.ErrUnavailable`, and for `context.DeadlineExceeded`, which a request deadline gives in place of `ErrBusy`;
    - its own handling for a canceled request, which the Postgres `TxManager` now reports as the context's error. A commit that fails with a real conflict after the context ended matches both, and the cancellation comes first;
  - a deadline on `/auth/*` longer than `DefaultMaxHashWait` plus a hash, so a busy hasher answers `ErrBusy` before the deadline;
  - in `docs/api/openapi.yaml`: 503 with `Retry-After` on the calls that hash a password (`/auth/signup`, `/auth/login`, `/auth/password-reset/confirm`, `/auth/invitations/accept`).
- **The authn middleware** calls `Verify` with its route's audience, then checks the `jti` against the Redis denylist. It logs a refusal's reason at debug level and never logs the `Authorization` header.
- **The default costs** run once in a test, so a bound that refused them fails in CI and not at the first login.
