# Milestone 3.5 — Android shell

Written 2026-11-27 on Opus, after the user accepted ADR 0029. Build each step with `/build-step 3.5.<n>`.

The Android management app becomes a Capacitor shell that opens the hosted web admin. Every existing admin screen gets a phone layout, so the shell is useful from day one, and milestone 4 onward builds each management screen once, in Vue.

## Blocked

Nothing. The questions that could stop this milestone were answered on 2026-11-27 (below). None of the open questions in `docs/architecture.md` section 12 applies.

## The user's answers (2026-11-27)

| # | Question | Answer |
| --- | --- | --- |
| Q1 | Which URL do builds open? | Release opens `https://admin.encomendo.com.br`; debug opens the local stack through `adb reverse` |
| Q2 | Signing and distribution? | Signed release APKs, with a keystore the user creates and keeps, read from outside the repository; testers sideload them. Google Play later |
| Q3 | App id? | Reuse `br.com.encomendo.app`, so the shell installs as an update of the frozen native app |
| Q4 | May Claude merge its own PRs? | Yes, once CI is green and the reviewers find nothing blocking; workflow and scanner config still wait for the user |
| Q5 | Where does the shell live? | `web/apps/android`, a third app in the pnpm workspace |
| Q6 | Plan shape? | This milestone 3.5 first, then milestone 4 web-only, with phone layouts |
| Q7 | Milestone 3's last box? | Reworded to "web admin and Android shell", and checked here in step 3.5.5 |
| Q8 | The native app's CI? | `mobile.yml` keeps building the frozen app until it is deleted |

## Decisions in this plan

- **Photos through the file input, not a camera plugin.**
  - Capacitor's Android WebView answers `<input type="file" accept="image/*">` with the system chooser (camera or gallery), and `uploadPhoto` already takes a file.
  - `@capacitor/camera` is added only if step 3.5.4 finds the chooser insufficient on a real phone.
  - This keeps the admin free of native-only code.
- **Debug host.** nginx answers the admin only for `ADMIN_HOST` (`admin.localhost` locally); any other host gets the store page.
  - So the debug shell opens `http://admin.localhost:8080`, with `adb reverse tcp:8080 tcp:8080` and `adb reverse tcp:9000 tcp:9000` for photos on Silo.
  - Chromium, and therefore the Android WebView, resolves `*.localhost` to loopback; step 3.5.2 verifies this on the device.
  - If it doesn't hold, the fallback is a debug-only nginx alias. That would need this plan amended, not an ad-hoc fix.
- **Phone size means 390 × 844.**
  - It is tested with a Playwright phone project and with Vitest where layouts branch.
  - Workshop sizes (18 px text, 56 px targets, 64 px primary action) apply only to the screens the product marks for workshop use, starting in milestone 5.
- **The shell has no UI of its own** except an offline error page with the shared wording: "Sem conexão com o servidor. Confira a internet e tente de novo." and "Tentar de novo".

## Ownership

This is the only session in milestone 3.5. No step needs `main.go`, `router.go`, `openapi.yaml` or a migration. If one turns out to, it stops and says so.

## Steps

| Step | Goal | Model |
| --- | --- | --- |
| 3.5.1 | Accept ADR 0029; docs and this plan | opus (done, in this PR) |
| 3.5.2 | The Capacitor shell | opus |
| 3.5.3 | Phone layouts I: auth, store picker, menu, Hoje, Equipe | sonnet |
| 3.5.4 | Phone layouts II: Catálogo | sonnet |
| 3.5.5 | Shell walkthrough on a device, release build, and close milestones 3 and 3.5 | sonnet |

3.5.3 and 3.5.4 need only the web admin, so they can run beside 3.5.2. 3.5.5 needs all three.

Steps marked opus need `/model opus` (or `claude --model opus`).

---

### 3.5.1 Accept ADR 0029; docs and this plan

- **Goal:** ADR 0029 is accepted, and the docs follow it.
- **Ticks:** "ADR 0029 accepted, and the docs follow it".
- **Files:**
  - `docs/adr/0029-*`;
  - `CLAUDE.md` (the app list, rule 6, the definition of done);
  - `web/CLAUDE.md`;
  - `mobile/CLAUDE.md` (frozen);
  - `docs/architecture.md` (section 2 rule 7, 8.2, 8.2a, 8.4, 11);
  - `docs/build-plan.md` (milestone 3.5, M3's reworded last box, the M4–M8 phone boxes, the offline open question, the summary, the decisions log);
  - this plan.
- **Reviewers:** none (docs only).
- **Status:** done in the PR that adds this plan.

### 3.5.2 The Capacitor shell

- **Goal:** an Android app at `web/apps/android` that opens the admin in a WebView, locked to the admin host, with a reproducible signed release build.
- **Ticks:** "`web/apps/android`: the Capacitor shell, with app id …".
- **Model:** opus. The WebView is where a session could leak: navigation to other hosts, cleartext, debugging, file access, external links.
- **Skill:** none.
- **Files:**
  - `web/apps/android/`:
    - `package.json`;
    - `capacitor.config.ts`;
    - the Capacitor `android/` project, created by `cap add android` and then committed and maintained as source;
    - `www/` with the offline error page;
    - `README.md`;
  - `web/pnpm-workspace.yaml` if needed;
  - `.gitignore`, for build output and any local keystore or properties files.
- **Work:**
  1. **App identity:** app id `br.com.encomendo.app`, the app name "Encomendo", an icon from the logo mark, and a `versionCode` above the frozen app's highest. The native app reads `VERSION_CODE` from the environment and defaults to 1, so start at 100 and document it.
  2. **URL per build:** release opens `https://admin.encomendo.com.br`; debug opens `http://admin.localhost:8080` (see Decisions).
     - Choose the URL at build time, for example an env var read by `capacitor.config.ts` at `cap sync`, or Android build types with a manifest placeholder.
     - Document the choice in the README.
  3. **Locked navigation:**
     - `server.allowNavigation` (or the WebView client) keeps the app on the admin host only;
     - `wa.me`, `api.whatsapp.com`, store pages (`*.encomendo.com.br`) and every other host open outside the app, in WhatsApp or the browser;
     - test this; Capacitor opens unlisted hosts externally by default, so verify it rather than assume it.
  4. **WebView hardening:**
     - cleartext allowed only in debug, through a debug-only network security config, as the frozen native app does;
     - WebView debugging off in release;
     - no file or content access beyond the photo chooser;
     - mixed content refused.
  5. **The back button:** `@capacitor/app`. It goes back in the WebView history when there is any, otherwise it leaves the app. A confirm-on-discard inside the admin still runs, because it is the page's own handling.
  6. **Offline error page:** `server.errorPath` (or equivalent) shows the local page with the shared wording and "Tentar de novo", which reloads the admin.
  7. **Release signing:**
     - Gradle reads `ENCOMENDO_KEYSTORE`, `ENCOMENDO_KEYSTORE_PASSWORD`, `ENCOMENDO_KEY_ALIAS` and `ENCOMENDO_KEY_PASSWORD` (or a `keystore.properties` outside version control);
     - a release build without them fails with a clear message;
     - never commit a keystore or a password, and keep key-like literals out of tests too (gitleaks scans every commit).
  8. **Builds:**
     - `pnpm --filter android build:debug` and `build:release`, or equivalent scripts, running `cap sync` then Gradle;
     - Gradle runs in the SDK container that `scripts/mobile-docker.sh` uses; `cap sync` runs on the host with Node;
     - document both.
  9. **README:**
     - building debug and release;
     - creating a keystore (`keytool`), for the user to run;
     - installing with `adb install`;
     - `adb reverse` for the local stack;
     - sideloading on a tester's phone.
- **New dependencies:** `@capacitor/core`, `@capacitor/cli`, `@capacitor/android` and `@capacitor/app`. Justify them in the PR.
- **Tests:**
  - a Vitest (or Node) test that reads `capacitor.config.ts` and pins the release URL, the app id and the navigation allowlist;
  - a test, or a small Android instrumentation-free check, that the debug and release network security configs differ as described;
  - the debug and release Gradle builds pass in the container (release with a throwaway keystore created at test time, never committed).
- **Reviewers:** `tenancy-security-reviewer` (WebView, cookies, external links, signing) and `web-reviewer`.
- **Depends on:** 3.5.1.

### 3.5.3 Phone layouts I: auth, store picker, menu, Hoje, Equipe

- **Goal:** the admin's existing non-catalog screens work at 390 × 844.
- **Ticks:** "Phone layouts: login, sign-up, …".
- **Model:** sonnet.
- **Skill:** none.
- **Files:**
  - `web/apps/admin/src/layouts/` (the narrow-screen menu: a top bar and a menu drawer);
  - `features/auth`, `features/hoje` and `features/members`;
  - `web/packages/ui` for shared responsive pieces (for example `BaseDialog` as a full-screen sheet on narrow screens);
  - the Playwright config (a phone project) and specs.
- **Work:**
  - at phone width, the menu is a drawer behind a menu button;
  - the account bar fits;
  - forms are full width, with 48 px controls;
  - tables become cards (Equipe, pending invitations);
  - dialogs become full-screen sheets;
  - nothing scrolls sideways.
  - Keep `web/CLAUDE.md`'s rules: one primary button per screen, icon plus word for status, visible focus, and labels.
- **Tests:**
  - Vitest for any layout branch;
  - Playwright at 390 × 844: login, store picker, the menu opening and navigating, Equipe invite and role change, and Hoje;
  - the existing desktop specs keep passing.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 3.5.1.

### 3.5.4 Phone layouts II: Catálogo

- **Goal:** Itens and Kits work at 390 × 844, photos included.
- **Ticks:** "Phone layouts: Catálogo …".
- **Model:** sonnet.
- **Skill:** none.
- **Files:** `web/apps/admin/src/features/catalogo/` and shared `packages/ui` pieces from 3.5.3.
- **Work:**
  - the Itens and Kits tables become cards with the same fields (cost and margin only when the API sends them);
  - the item and kit drawers become full-screen forms;
  - the Categorias and Campos dialogs become sheets;
  - the photo editor uses `<input type="file" accept="image/*">`, so the shell offers the camera or gallery;
  - the tab row and the filters fit.
- **Tests:**
  - Playwright at 390 × 844: the M3 flow (item with a new supplier, a lead time, a cost and a photo, a kit with a swap), and an admin seeing no cost;
  - Vitest for the card layouts;
  - the desktop Catálogo specs keep passing.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 3.5.3, for the shared pieces.

### 3.5.5 Shell walkthrough on a device, release build, and close milestones 3 and 3.5

- **Goal:** prove the shell against the local stack on a real phone, or an emulator if KVM works; build a signed release APK by following the README; and close both milestones.
- **Ticks:**
  - the release-build box (its CI half only if the user has added the workflow job);
  - milestone 3.5's "Done when";
  - milestone 3's reworded last box.
- **Model:** sonnet.
- **Skill:** none.
- **Work:**
  - **The device.** KVM is unavailable on this machine (Windows update KB5127648 on AMD, `Nested features: 0x0`), so prefer a physical phone through `adb`, over USB with `usbipd` or over Wi-Fi with `adb pair` / `adb connect`. Use the Docker emulator only if `/dev/kvm` exists.
  - **The walkthrough, with screenshots in `docs/plans/m3_5-walkthrough/`:**
    - log in;
    - create an item with a new supplier, a lead time and a photo from the camera in the shell, and check it on the web;
    - edit a web-created item in the shell;
    - as an admin, see no cost;
    - open a `wa.me` link, which leaves the app;
    - turn off the network and see the offline page, then "Tentar de novo";
    - check the back button.
  - **The M2 device checks**, if the phone allows: the keyboard inset on Equipe, and switching between two stores.
  - **The release build:** build a signed release APK by following the README with a throwaway keystore (never committed), install it, and check that it refuses cleartext and opens the production URL. Don't log in to production unless the user says so.
  - **CI:** write the workflow job for the shell (Node plus the Android SDK, a debug build, with the release build only if secrets exist) as `docs/plans/m3_5-ci-job.yml` for the user to copy into `.github/workflows/`. The token cannot push workflow files.
  - **Docs:**
    - tick the boxes and update the summary;
    - remove "(partial…)" from milestone 3's last box;
    - add a result note at the top of this plan.
- **Tests:** the walkthrough; `make lint test`; the web checks; Playwright on desktop and phone; the shell builds.
- **Reviewers:** `web-reviewer`.
- **Depends on:** 3.5.2, 3.5.3 and 3.5.4.

## After milestone 3.5

Plan milestone 4 (`/plan-milestone 4`) with the new parity rule: each screen ships once, in Vue, tested at desktop and phone sizes.
