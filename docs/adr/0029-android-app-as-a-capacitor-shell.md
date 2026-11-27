# 0029 — Android management app as a Capacitor shell around the web admin

- Status: **accepted** by the user on 2026-11-27, with the answers below.
- Date: 2026-11-27
- Supersedes: the native-app parts of `docs/architecture.md` section 8.2, the meaning of rule 6 (management parity), ADR 0005 (Room 3), ADR 0015, ADR 0016, ADR 0018 and ADR 0020 to ADR 0022 as they apply to Android, and ADR 0026.

## Context

The management side ships as two apps against one API:

- the Vue web admin (`web/apps/admin`);
- a native Kotlin Multiplatform Android app (`mobile/`), with Compose UI, Room, Ktor and Koin.

Rule 6 says a management feature is done only when both screens work.

After milestone 3, the cost of that rule is visible. On Android, milestone 3 took the data layer, Itens and Kits (steps 3.14 to 3.16). That was about as much building and review as the web side (steps 3.11 to 3.13), and every Android screen copied a web screen's rules and wording.

Milestones 4 to 7 add many more management screens. Each one would be built twice:

- M4: Hoje, Pedidos, Clientes, Financeiro and settings;
- M5: Compras and receiving with photos;
- M6: Estoque and Produção;
- M7: the portal builder.

The user asked for the quickest route to v1 and whether a web build in a native shell would be quicker. Two things argued for native in the first place, and they are the cost of leaving it:

- **Offline.** Recent orders, the receiving checklist and the production board are cached. Receiving writes, photos included, are queued and retried with WorkManager and an `Idempotency-Key` (architecture 8.2, build plan M4–M6).
- **Workshop screens.** Receiving and production are used with dirty hands and gloves: 18 sp text, 56 dp targets and a 64 dp primary action, with native camera flows.

Android emulator checks are also blocked on this machine today: a Windows update (KB5127648) removed nested virtualization on AMD. That is not a reason for this ADR, but it slows native testing.

## Options

1. **Stay native, as today.** Every management screen is built twice. Offline and workshop behaviour stay native.
2. **Capacitor shell loading the hosted admin** (`server.url`). The app is a native Android project whose WebView opens the production admin URL, so the API sees an ordinary same-origin browser.
3. **Capacitor shell with the web build bundled in the app.** The page runs on its own origin (`https://localhost`) and calls the API cross-origin.
4. **A PWA only** (installable web app, no store build). This was considered and set aside: the camera and file access are weaker on some devices, and there's no Play Store listing.

## Decision

Take **option 2**, built in a short **milestone 3.5** before milestone 4 (plan: `docs/plans/m3_5-android-shell.md`).

The user's answers (2026-11-27):

- **Distribution:** private first. Testers sideload release APKs, and the Google Play Store comes later.
- **Signing:** the user creates and keeps the release keystore. The build reads it from local files or environment variables, and it is never committed. Debug builds are unsigned.
- **App id:** the shell reuses `br.com.encomendo.app`, so testers get it as an update of the frozen native app, and the later Play listing keeps one id. The shell's `versionCode` starts above the native app's.
- **URL:** release builds open `https://admin.encomendo.com.br`. Debug builds open the local stack through `adb reverse`.
- **Location:** `web/apps/android`, a third app in the pnpm workspace.
- **Native CI:** `mobile.yml` keeps building the frozen app until it is deleted.
- **Milestone 3's last box** is reworded to "created in the web admin and in the Android shell", and is checked with the shell in milestone 3.5.

The parts below are the decision as proposed and accepted.

- **The shell.** A Capacitor project at `web/apps/android`, wrapping the hosted admin at `https://admin.encomendo.com.br`.
  - The web admin stays the only management UI. Android stops being a second implementation.
  - Native plugins only where the web falls short: the back button and status bar (`@capacitor/app`), and links that must leave the app, such as `wa.me` and the store page. Photos use the admin's file input, which Capacitor's WebView answers with the system camera and picker. `@capacitor/camera` is added only if that proves insufficient. `uploadPhoto` in `packages/api` already takes a file.
- **Auth is unchanged.** The hosted page is same-origin with the API, so the refresh cookie, the `/auth/*` origin checks (ADR 0010) and the session rules (ADR 0012, ADR 0013) work as they do in a browser. No CORS, no body-token transport and no backend change.
- **Phone-ready admin.** The admin screens get responsive layouts:
  - a narrow-screen menu;
  - drawers that become full-screen forms;
  - tables that become cards;
  - the workshop sizes (18 px text, 56 px targets, 64 px primary action) on the screens marked for workshop use, from M5.

  These are web components, built once, and tested with Vitest and Playwright at phone sizes.
- **Offline in v1: none.** The phone needs a connection, as the browser does. Without one it shows the existing "Sem conexão com o servidor…" message. The requirements for cached orders, an offline receiving checklist and queued writes move to a later decision. If they come back, it is as a service worker and IndexedDB in the web admin, or by moving to option 3.
- **Parity, redefined.** Rule 6 becomes: a management feature is done when the web admin works at desktop and phone sizes, and the Capacitor shell builds and opens it. There is no second implementation to keep at parity.
- **The existing native app** (M2 auth and Equipe, M3 Catálogo) is frozen, not deleted:
  - it stays in `mobile/`, builds in CI and gets no new features;
  - it is removed once the shell reaches the Play Store, in its own PR.
  - ADR 0026 and the Android-specific ADRs then become historical.
- **As built in step 3.5.2** (2026-11-27; details in `web/apps/android/README.md`):
  - Back with no WebView history moves the app to the background, as Android's own Back does.
  - Debug builds are signed with the SDK debug key, kept in the Gradle volume, because Android cannot install an unsigned app. "Unsigned" above means no release key.
  - compileSdk and targetSdk are 36, Capacitor 8's template; minSdk is 26, as the frozen app.
  - There is no `CAMERA` permission. The admin's file input gets the choice "Da galeria" or "Tirar foto", and the camera app takes the photo into a `FileProvider` file limited to `cache/photos`. Declaring `CAMERA` would make that need a runtime grant.
  - **Security hardening**, because the hosted page carries the session and Capacitor's bridge:
    - Capacitor 8 registers its `CapacitorHttp`, `CapacitorCookies` and `WebView` plugins whatever the config says, and `CapacitorCookies` shares the WebView cookie jar with native HTTP. Page script could then read the HttpOnly refresh cookie. The shell replaces the three with stubs that have no methods, removes their JavaScript interfaces before the first page loads, and leaves native HTTP with no cookie handler. Only `App` and `SystemBars` stay callable.
    - Capacitor's `/_capacitor_file_/` and `/_capacitor_content_/` routes are refused, since they would read the app's files, the cookie database included.
    - Navigation and frames stay on the admin origin, and links open outside only on a tap. The admin's nginx sends `Content-Security-Policy: frame-src 'self'; frame-ancestors 'none'`.
    - The admin opens only on a WebView 120 or later with `WEB_MESSAGE_LISTENER` and `DOCUMENT_START_SCRIPT`, so the bridge answers the admin origin alone. On an older WebView an update page shows instead.
    - Cleartext is allowed only in debug, for the local stack hosts. Backups are off. A release build checks its synced config and signing values and refuses anything else.

## What it would take

About three plan steps, then less work per screen in M4 to M7.

1. **Shell and build.**
   - The Capacitor project, app id and icon, the hosted URL per environment (debug opens the local stack's admin host; the emulator or phone reaches it through `adb reverse` and a host name the stack's nginx answers to), and the camera plugin wired into photo upload.
   - A Gradle build of the shell.
   - CI needs a new or changed job in `.github/workflows/`, which the bot token cannot push. **The user makes that edit.**
2. **Phone layouts for the screens that exist:** login and store picker, Hoje, Equipe and the Catálogo. Each gets Vitest and Playwright runs at 390 × 844.
3. **Docs:**
   - `docs/architecture.md`: 8.2 and the parity map in 8.4;
   - `CLAUDE.md`: rule 6, the `mobile/` description and model routing;
   - `mobile/CLAUDE.md`: marked frozen;
   - `docs/build-plan.md`: the Mobile boxes of M4 to M7 replaced by "phone layout" boxes on the web side, and the offline boxes moved to an open question.

## Consequences

**Gains**
- Every management screen from M4 on is built, reviewed and tested once, in Vue. That is the main saving.
- Web and Android can never disagree on wording, rules or behaviour, because there is one implementation.
- Auth, cookies and the backend stay exactly as they are.

**Costs and risks**
- **No offline use in v1.** Workshops with weak signal can't receive items or move production stages without a connection. This is the largest product trade-off, and **the user should confirm it against how real shops will use the phone.**
- **Weaker native feel.** Scrolling, transitions and the keyboard are the WebView's. The camera goes through a plugin, not CameraX.
- **Play Store policy.** Google Play's minimum-functionality policy can refuse apps that only wrap a website. The shell's native camera and back handling, and a business-account login, usually satisfy it. If not, the app can be distributed privately (managed Google Play or a direct APK) to store staff.
- **Sunk work.** The native Android work is shelved: M2's auth, store picker and Equipe (PRs #12 to #36, #47, #49) and M3's Catálogo (#57, #64, #68). Their lessons stay: owner-only costs, the photo flow, wording.
- **New web work.** The responsive layouts are new web work for every existing and future admin screen. They are smaller than a native rebuild, but not free.
- **Reversible later, at a cost.** Option 3 (a bundled build with offline storage) could be added later. It needs the Android-style body-token refresh, secure storage for the token, and a CORS and `/auth` origin allowance for the shell's origin, which is a backend security change with its own ADR.

## Not decided here

- Whether offline receiving and production come back after v1. If they do, it is as option 3 or a service worker, with its own ADR.
- When the frozen native app is removed (after the shell reaches the Play Store).
- The Play Store listing itself: store assets, privacy policy, and review against the minimum-functionality policy.
