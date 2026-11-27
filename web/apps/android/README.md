# Encomendo Android shell

The Android management app (ADR 0029): a Capacitor project whose WebView opens the hosted web admin. It has no screens of its own except an offline page. Every management screen is the web admin's, so there is nothing to keep at parity.

- App id `br.com.encomendo.app`, the frozen native app's (`mobile/`), so the shell installs as an update of it.
- `versionCode` starts at **100**, above the native app's 1. Pass `VERSION_CODE` (and `VERSION_NAME`) for each release; the build refuses anything under 100.
- `minSdk` 26 as the native app, `targetSdk` and `compileSdk` 36, Capacitor 8.

## Layout

| Path | What it is |
| --- | --- |
| `capacitor.config.ts` | The URL per build, the navigation allowlist (empty), the WebView flags. |
| `www/index.html` | The offline page: "Sem conexão com o servidor. Confira a internet e tente de novo." and "Tentar de novo". |
| `android/` | The native project. `cap add android` created it once; it is maintained source now. Never run `cap add` over it. |
| `android/app/src/main/java/br/com/encomendo/app/` | `MainActivity` (hardening, Back), `NavigationPolicy` and `LockedWebViewClient` (navigation lock), `PhotoChoice` and `PhotoChooser` (photo input). |
| `android/app/src/{main,debug}/res/xml/network_security_config.xml` | HTTPS only in release; plain HTTP for the local stack in debug. |
| `*.test.ts`, `android/app/src/test/` | Vitest checks of the config and the Android files; JUnit tests of the navigation and photo decisions. |

`cap sync` writes files that are git-ignored and must not be edited: `android/app/src/main/assets/` (the synced config and `www/`), `android/capacitor-cordova-android-plugins/`, and `android/app/src/main/res/xml/config.xml`. It also rewrites `android/capacitor.settings.gradle` and `android/app/capacitor.build.gradle`, which are committed.

## Which URL a build opens

The URL is chosen at `cap sync` time, from `ENCOMENDO_SHELL_BUILD`:

| Build | `ENCOMENDO_SHELL_BUILD` | Opens | WebView debugging |
| --- | --- | --- | --- |
| release | `release`, or unset | `https://admin.encomendo.com.br` | off |
| debug | `debug` | `http://admin.localhost:8080` | on |

Unset means release, so a bare `cap sync` never leaves a release build pointing at the local stack. Gradle checks it again: a release build stops if the last sync was not a release sync (plain HTTP, a `localhost` host, or debugging on).

The `build:*` scripts below always sync and build together, so the two cannot drift.

## Building

`cap sync` runs on the host with Node. Gradle runs in the Android SDK container of `scripts/mobile-docker.sh` (same image and volumes), through `scripts/android-shell-docker.sh`. You need Docker, Node 22 or later and pnpm; no Android SDK.

```bash
cd web
pnpm install --frozen-lockfile

pnpm --filter @encomendo/android build:debug      # android/app/build/outputs/apk/debug/app-debug.apk
pnpm --filter @encomendo/android build:release    # android/app/build/outputs/apk/release/app-release.apk (signed)
pnpm --filter @encomendo/android test             # Vitest: the config, the network configs, the manifest
pnpm --filter @encomendo/android test:android     # JUnit in the container: NavigationPolicy, PhotoChoice
```

Any other Gradle task: `scripts/android-shell-docker.sh <task>`, for example `scripts/android-shell-docker.sh lint`. With a local Android SDK (Android Studio), sync with `pnpm --filter @encomendo/android sync:debug` or `sync:release` and open `web/apps/android/android`.

Debug builds carry no release signature. Android installs nothing unsigned, so Gradle signs them with the SDK's throwaway debug key, kept in the Gradle volume so each debug build installs over the last. They are for development only.

## Release build

### Create the keystore (once, by the person who keeps it)

The release key signs every update for as long as the app exists: an update signed with another key will not install over it. Create it outside the repository, keep a backup, and never commit it or its passwords (`*.keystore`, `*.jks` and `keystore.properties` are git-ignored, and gitleaks scans every commit).

```bash
keytool -genkeypair -v \
  -keystore ~/encomendo-keys/encomendo-release.keystore \
  -storetype PKCS12 -keyalg RSA -keysize 4096 -validity 10000 \
  -alias encomendo
```

`keytool` asks for the store password and the name fields. With PKCS12 the key password is the store password. No JDK on the machine? Run it from the SDK image:

```bash
docker run --rm -it -u "$(id -u):$(id -g)" -v ~/encomendo-keys:/keys ghcr.io/cirruslabs/android-sdk:36 \
  keytool -genkeypair -v -keystore /keys/encomendo-release.keystore \
  -storetype PKCS12 -keyalg RSA -keysize 4096 -validity 10000 -alias encomendo
```

### Give the build the signing values

Either environment variables (the container script mounts the keystore read-only and passes the passwords by name):

```bash
export ENCOMENDO_KEYSTORE=~/encomendo-keys/encomendo-release.keystore
export ENCOMENDO_KEY_ALIAS=encomendo
read -rs ENCOMENDO_KEYSTORE_PASSWORD && export ENCOMENDO_KEYSTORE_PASSWORD
export ENCOMENDO_KEY_PASSWORD="$ENCOMENDO_KEYSTORE_PASSWORD"
VERSION_CODE=101 VERSION_NAME=0.2.1 pnpm --filter @encomendo/android build:release
```

or a `web/apps/android/android/keystore.properties` file, git-ignored, with the keys `storeFile`, `storePassword`, `keyAlias` and `keyPassword`. A relative `storeFile` is read from `web/apps/android/android/`. In the container only paths inside the repository exist, so with the container script prefer the environment variables. An environment variable wins over the file.

Without all four values, or with a `storeFile` that does not exist, the release build stops with a message that names what is missing. It never produces an unsigned release APK.

Check the result:

```bash
docker run --rm -v "$PWD/web/apps/android/android/app/build/outputs/apk/release":/apk ghcr.io/cirruslabs/android-sdk:36 \
  bash -c '$(ls -d /opt/android-sdk-linux/build-tools/* | tail -1)/apksigner verify --print-certs /apk/app-release.apk'
```

## Running a debug build on a phone

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
- Over USB: `adb install -r app-release.apk`.
- Testers who have the frozen native app get the shell as an update: same id, higher `versionCode`. That works only if the release is signed with the same key as the native app's release; otherwise Android refuses the update and the native app must be uninstalled first.
- Each new release needs a higher `VERSION_CODE` than the last one installed.

## How the shell is locked down

- **Navigation.** Only the build's admin origin (scheme, host and port) loads in the app. `wa.me`, `api.whatsapp.com`, store pages (`*.encomendo.com.br`) and every other web, `mailto:` or `tel:` link open in another app, through `ACTION_VIEW` with `CATEGORY_BROWSABLE`. Other schemes (`intent:`, `file:`, `content:`, `javascript:`) and any frame that tries to load another site are dropped. `NavigationPolicyTest` covers each case.
- **Cleartext.** Release refuses plain HTTP to every host and trusts only system certificate authorities. Debug allows it for `admin.localhost` and `localhost` only. `usesCleartextTraffic` is never set, and neither is Capacitor's `server.cleartext`, which would set it.
- **WebView.** No `file:` or `content:` URL access, mixed content refused, debugging only in a debuggable build. Capacitor's native HTTP and cookie plugins are off, so the admin uses the WebView's own `fetch` and cookie jar, and the refresh cookie and `/auth` origin checks work exactly as in a browser (ADR 0010).
- **Backups.** Off, including device-to-device transfer: app data holds the refresh cookie.
- **Permissions.** Only `INTERNET`. The camera belongs to the camera app: the shell asks it for one photo with `ACTION_IMAGE_CAPTURE` and lends it one file in `cache/photos` through the `FileProvider`. No `CAMERA` permission is declared, because declaring it would make that intent need a runtime grant.

## Photos

The admin's photo input (`<input type="file" accept="image/jpeg,image/png,image/webp" multiple>`) gets the product's choice: "Adicionar fotos" asks "Da galeria" or "Tirar foto". Capacitor alone would offer only the file picker. An input with `capture` opens the camera directly; inputs for other file types go to Capacitor's picker.

## Back and the offline page

- Back goes back in the admin's history (through `@capacitor/app`, so the page's own guards, such as a confirm-on-discard, still run). With no history left, the app goes to the background, as Android's own Back does, and keeps the session.
- When the admin cannot load, the WebView shows `www/index.html`. "Tentar de novo" loads the admin URL again. The offline page is never kept in the back history.

## Device checks

These need a phone (or an emulator with KVM) and are done in step 3.5.5:

1. The debug build opens `http://admin.localhost:8080` through `adb reverse` (the `*.localhost` resolution), and login works (the refresh cookie survives an app restart).
2. Navigation stays locked: a `wa.me` link opens WhatsApp or the browser, a store page link opens the browser, and the app stays on the admin.
3. "Adicionar fotos" asks "Da galeria" or "Tirar foto"; both upload, several gallery photos at once included, and with `setAllowContentAccess(false)` the chosen photos still reach the page.
4. A photo taken while the system kills the app in the background (developer option "Don't keep activities") fails cleanly, without a stuck input.
5. Back: goes back inside the admin; from the first page it leaves the app; a confirm-on-discard in the admin still asks.
6. Offline: with the network off, launching shows the offline page; "Tentar de novo" with the network back opens the admin; Back from there leaves the app.
7. Release build: opens `https://admin.encomendo.com.br`, refuses cleartext, `chrome://inspect` does not list it, and the icon and splash show the logo mark.
8. Installed over the frozen native app as an update (same signing key).
9. The status bar and the keyboard do not cover the admin (Capacitor 8's system bar insets).
