# Encomendo Android shell

The Android management app (ADR 0029): a Capacitor project whose WebView opens the hosted web admin. It has no screens of its own except an offline page and a "please update your WebView" page. Every management screen is the web admin's, so there is nothing to keep at parity.

- App id `br.com.encomendo.app`, the frozen native app's (`mobile/`), so the shell installs as an update of it.
- `versionCode` starts at **100**, above the native app's 1. Pass `VERSION_CODE` (and `VERSION_NAME`) for each release; the build refuses anything under 100.
- `minSdk` 26 as the native app, `targetSdk` and `compileSdk` 36 (Capacitor 8's template), Capacitor 8.
- Android System WebView 120 or later, with the features that bind Capacitor's bridge to the admin origin.

## Layout

| Path | What it is |
| --- | --- |
| `capacitor.config.ts` | The URL per build, the navigation allowlist (empty), the WebView flags and the minimum WebView. |
| `www/index.html` | The offline page: "Sem conexão com o servidor. Confira a internet e tente de novo." and "Tentar de novo". |
| `android/` | The native project. `cap add android` created it once; it is maintained source now. Never run `cap add` over it. |
| `android/app/src/main/java/br/com/encomendo/app/` | `MainActivity` (hardening, Back, the WebView check), `WebViewSupport`, `NativeBridgeLockdown` (plugins the page must not reach), `NavigationPolicy` and `LockedWebViewClient` (navigation and frame lock), `PhotoChoice` and `PhotoChooser` (photo input). |
| `android/app/src/{main,debug}/res/xml/network_security_config.xml` | HTTPS only in release; plain HTTP for the local stack in debug. |
| `*.test.ts`, `android/app/src/test/` | Vitest checks of the config and the Android files; JUnit tests of every decision above. |
| `../../../scripts/android-shell-docker.sh`, `android-shell-guard-test.sh` | Gradle in the SDK container; the release guard checks. |

`cap sync` writes files that are git-ignored and must not be edited: `android/app/src/main/assets/` (the synced config and `www/`), `android/capacitor-cordova-android-plugins/`, and `android/app/src/main/res/xml/config.xml`. It also rewrites `android/capacitor.settings.gradle` and `android/app/capacitor.build.gradle`, which are committed.

## Which URL a build opens

The URL is chosen at `cap sync` time, from `ENCOMENDO_SHELL_BUILD`:

| Build | `ENCOMENDO_SHELL_BUILD` | Opens | WebView debugging |
| --- | --- | --- | --- |
| release | `release`, or unset | `https://admin.encomendo.com.br` | off |
| debug | `debug` | `http://admin.localhost:8080` | on |

Unset means release, so a bare `cap sync` never leaves a release build pointing at the local stack. Gradle checks the synced config again before a release build. It stops unless the URL is exactly `https://admin.encomendo.com.br`, `allowNavigation` is empty, debugging, mixed content and cleartext are off, `useLegacyBridge` is not set, and the minimum WebView is 120. `pnpm --filter @encomendo/android test:guards` proves each refusal.

The `build:*` scripts below always sync and build together, so the two cannot drift.

## Building

`cap sync` runs on the host with Node. Gradle runs in the Android SDK container of `scripts/mobile-docker.sh` (same image and volumes), through `scripts/android-shell-docker.sh`. You need Docker, Node 22 or later and pnpm; no Android SDK.

From the repository root:

```bash
cd web
pnpm install --frozen-lockfile

pnpm --filter @encomendo/android build:debug      # web/apps/android/android/app/build/outputs/apk/debug/app-debug.apk
pnpm --filter @encomendo/android build:release    # .../apk/release/app-release.apk (signed; see Release build)
pnpm --filter @encomendo/android test             # Vitest: the config, the network configs, the manifest
pnpm --filter @encomendo/android test:android     # JUnit in the container
pnpm --filter @encomendo/android test:guards      # every release guard refuses what it must (leaves a release sync)
```

Any other Gradle task, from the repository root: `scripts/android-shell-docker.sh <task>`, for example `scripts/android-shell-docker.sh lint`. With a local Android SDK (Android Studio), sync with `pnpm --filter @encomendo/android sync:debug` or `sync:release` and open `web/apps/android/android`.

Debug builds carry no release signature. Android installs nothing unsigned, so Gradle signs them with the SDK's throwaway debug key, kept in the Gradle volume so each debug build installs over the last. They are for development only.

## Release build

### The keystore (once, by the person who keeps it)

If you already signed the native app, use its keystore instead of creating one; otherwise Android refuses the update.

The release key signs every update for as long as the app exists: an update signed with another key will not install over it. Create it outside the repository, keep a backup, and never commit it or its passwords (`*.keystore`, `*.jks` and `keystore.properties` are git-ignored, and gitleaks scans every commit).

From any directory:

```bash
mkdir -p ~/encomendo-keys
keytool -genkeypair -v \
  -keystore ~/encomendo-keys/encomendo-release.keystore \
  -storetype PKCS12 -keyalg RSA -keysize 4096 -validity 10000 \
  -alias encomendo
```

`keytool` asks for the store password and the name fields. With PKCS12 the key password is the store password. No JDK on the machine? Run it from the SDK image, from any directory:

```bash
mkdir -p ~/encomendo-keys
docker run --rm -it -u "$(id -u):$(id -g)" -v ~/encomendo-keys:/keys ghcr.io/cirruslabs/android-sdk:36 \
  keytool -genkeypair -v -keystore /keys/encomendo-release.keystore \
  -storetype PKCS12 -keyalg RSA -keysize 4096 -validity 10000 -alias encomendo
```

### Give the build the signing values

Either environment variables (the container script mounts the keystore read-only and passes the passwords by name). From `web/`:

```bash
export ENCOMENDO_KEYSTORE=~/encomendo-keys/encomendo-release.keystore
export ENCOMENDO_KEY_ALIAS=encomendo
read -rs ENCOMENDO_KEYSTORE_PASSWORD && export ENCOMENDO_KEYSTORE_PASSWORD
export ENCOMENDO_KEY_PASSWORD="$ENCOMENDO_KEYSTORE_PASSWORD"
VERSION_CODE=101 VERSION_NAME=0.2.1 pnpm --filter @encomendo/android build:release
```

or a `web/apps/android/android/keystore.properties` file, git-ignored, with the keys `storeFile`, `storePassword`, `keyAlias` and `keyPassword`. A relative `storeFile` is read from `web/apps/android/android/`. In the container only paths inside the repository exist, so with the container script prefer the environment variables. An environment variable wins over the file.

Without all four values, or with a `storeFile` that does not exist, the release build stops with a message that names what is missing. It never produces an unsigned release APK.

Check the result, from the repository root:

```bash
docker run --rm -v "$PWD/web/apps/android/android/app/build/outputs/apk/release":/apk ghcr.io/cirruslabs/android-sdk:36 \
  bash -c '$(ls -d /opt/android-sdk-linux/build-tools/* | tail -1)/apksigner verify --print-certs /apk/app-release.apk'
```

## Running a debug build on a phone

All commands from the repository root.

1. Start the built-image stack, which serves the admin at `admin.localhost:8080` through nginx: `make stack-up`. (`make dev-up` serves the admin from Vite on port 5173 and is not what the shell opens.)
2. Turn on USB debugging on the phone and connect it (USB, `usbipd` on WSL, or Wi-Fi with `adb pair` and `adb connect`). `adb devices` should list it.
3. Let the phone reach the stack: its `localhost` becomes this machine's.

   ```bash
   adb reverse tcp:8080 tcp:8080   # the admin, and the API behind it
   adb reverse tcp:9000 tcp:9000   # photos (Silo signs URLs for localhost:9000)
   ```

   The WebView resolves `admin.localhost` to the phone's loopback, as Chrome does. If a phone does not, the fallback is a debug-only nginx alias, which needs `docs/plans/m3_5-android-shell.md` amended first.
4. Install and open:

   ```bash
   adb install -r web/apps/android/android/app/build/outputs/apk/debug/app-debug.apk
   adb shell am start -n br.com.encomendo.app/.MainActivity
   ```

5. Inspect the page from Chrome on the computer: `chrome://inspect` (debug builds only).

`adb reverse` lasts until the phone disconnects; run it again after reconnecting.

## Sideloading a release on a tester's phone

Testers get signed release APKs directly (ADR 0029); the Play Store comes later.

- Send `app-release.apk` (a link or a file). On the phone, opening it asks to allow installs from that app (the browser or the file manager) once.
- Over USB, from the folder that holds the APK: `adb install -r app-release.apk`.
- Testers who have the frozen native app get the shell as an update: same id, higher `versionCode`. That works only if the release is signed with the same key as the native app's release; otherwise Android refuses the update and the native app must be uninstalled first.
- Each new release needs a higher `VERSION_CODE` than the last one installed.

## How the shell is locked down

- **Navigation.** Only the build's admin origin (scheme, host and port) loads in the app. `wa.me`, `api.whatsapp.com`, store pages (`*.encomendo.com.br`) and every other web, `mailto:` or `tel:` link open in another app, through `ACTION_VIEW` with `CATEGORY_BROWSABLE`, and only when the user tapped it (or a tapped link redirected). Other schemes (`intent:`, `file:`, `content:`, `data:`) are dropped. A `javascript:` link is not a navigation: it runs in the page, and the shell cannot stop it.
- **Frames.** Android asks the app about frame navigations only for non-http(s) schemes, so every request also passes `LockedWebViewClient.shouldInterceptRequest`: a frame or embedded document from any origin but the admin's is refused. The admin's nginx also sends `Content-Security-Policy: frame-src 'self'; frame-ancestors 'none'`.
- **Capacitor's file routes.** `/_capacitor_file_/`, `/_capacitor_content_/` and the HTTP interceptor routes would read any of the app's files (the WebView's cookie database included) for the page that asks. The shell refuses them at every origin, percent-encoded or not.
- **Native plugins.** The page reaches only `App` (`@capacitor/app`) and `SystemBars` (status and navigation bar style), and every plugin's base listener and permission methods, which do nothing for these. Capacitor 8 registers `CapacitorHttp`, `CapacitorCookies` and `WebView` itself whatever the config says, and `CapacitorCookies` shares the WebView's cookie jar, the HttpOnly refresh cookie included, with native HTTP. `NativeBridgeLockdown` replaces all three with stubs that have no methods. Before the first page loads it removes their JavaScript interfaces (`CapacitorCookiesAndroidInterface`, `CapacitorHttpAndroidInterface`) and leaves native HTTP with no cookie handler.
- **The WebView itself.** The admin opens only on a WebView 120 or later with `WEB_MESSAGE_LISTENER` and `DOCUMENT_START_SCRIPT`, and without the legacy bridge: there, Capacitor's bridge answers only the admin origin. On an older WebView, Capacitor would fall back to a JavaScript interface any frame can call, so the shell shows a page asking to update Android System WebView instead.
- **Cleartext.** Release refuses plain HTTP to every host and trusts only system certificate authorities. Debug allows it for `admin.localhost` and `localhost` only. `usesCleartextTraffic` is never set, and neither is Capacitor's `server.cleartext`, which would set it.
- **WebView settings.** `setAllowFileAccess(false)` and `setAllowContentAccess(false)`, so the page cannot load `file:` or `content:` URLs directly (Capacitor's routes above are closed separately). Mixed content is refused, and debugging is on only in a debuggable build. The admin uses the WebView's own `fetch` and cookie jar, so the refresh cookie and the `/auth` origin checks work exactly as in a browser (ADR 0010).
- **Backups.** Off, including device-to-device transfer: app data holds the refresh cookie.
- **Permissions.** Only `INTERNET`. The camera belongs to the camera app: the shell asks it for one photo with `ACTION_IMAGE_CAPTURE` and lends it one file in `cache/photos` through the `FileProvider`. No `CAMERA` permission is declared, because declaring it would make that intent need a runtime grant.

## Photos

The admin's photo input (`<input type="file" accept="image/jpeg,image/png,image/webp" multiple>`) gets the product's choice: "Adicionar fotos" asks "Da galeria" or "Tirar foto". Capacitor alone would offer only the file picker. An input with `capture` opens the camera directly; inputs for other file types go to Capacitor's picker. From the gallery, only other apps' `content:` URIs reach the page; a `file:` URI, or one of the shell's own `FileProvider`, is dropped.

## Back, the offline page and the WebView update page

- Back goes back in the admin's history (through `@capacitor/app`, so the page's own guards, such as a confirm-on-discard, still run). With no history left, the app goes to the background, as Android's own Back does, and keeps the session.
- When the admin cannot load, the WebView shows `www/index.html`. "Tentar de novo" loads the admin URL again. The offline page is never kept in the back history.
- On a WebView that cannot lock the bridge down: "Para abrir o Encomendo, atualize o {WebView} deste aparelho na Play Store e abra o app de novo." and "Atualizar na Play Store" (drafted, for review in `docs/product/design-document.md`).

## Device checks

These need a phone (or an emulator with KVM) and are done in step 3.5.5:

1. The debug build opens `http://admin.localhost:8080` through `adb reverse` (the `*.localhost` resolution), and login works (the refresh cookie survives an app restart).
2. Navigation stays locked: a tapped `wa.me` link opens WhatsApp or the browser, a store page link opens the browser, and the app stays on the admin. A `wa.me` link opened from script after a tap (`window.open` in a click handler) still opens.
3. In `chrome://inspect` (debug build), on the admin page:
   - `typeof CapacitorCookiesAndroidInterface === 'undefined'` and `typeof CapacitorHttpAndroidInterface === 'undefined'`;
   - `Capacitor.nativePromise('CapacitorHttp', 'post', { url: location.origin + '/auth/refresh', headers: { 'Content-Type': 'application/json' }, data: {} })` never resolves with a `Set-Cookie` (it never answers: the stub has no method);
   - `fetch('/_capacitor_file_/data/data/br.com.encomendo.app/app_webview/Default/Cookies')` answers 403;
   - a frame added with `document.body.append(Object.assign(document.createElement('iframe'), { src: 'https://example.com' }))` stays empty.
4. "Adicionar fotos" asks "Da galeria" or "Tirar foto"; both upload, several gallery photos at once included, and with `setAllowContentAccess(false)` the chosen photos still reach the page.
5. A photo taken while the system kills the app in the background (developer option "Don't keep activities") fails cleanly, without a stuck input.
6. Back: goes back inside the admin; from the first page it leaves the app; a confirm-on-discard in the admin still asks.
7. Offline: with the network off, launching shows the offline page; "Tentar de novo" with the network back opens the admin; Back from there leaves the app.
8. Release build: opens `https://admin.encomendo.com.br`, refuses cleartext, `chrome://inspect` does not list it, and the icon and splash show the logo mark.
9. Installed over the frozen native app as an update (same signing key).
10. The status bar and the keyboard do not cover the admin (Capacitor 8's system bar insets).
11. On a phone whose WebView is older than 120 (or with WebView updates uninstalled), the update page shows and "Atualizar na Play Store" opens the WebView's Play Store page.
