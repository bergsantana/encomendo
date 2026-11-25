# Mobile (Kotlin Multiplatform, Android)

The **management** app for owners, admins and staff, with every feature of the web admin app. It is also used in the workshop. Clients never use it: the store page and client area are web only. The full layout is in `docs/architecture.md` section 8.2.

**Android only in v1; iOS is postponed.** Don't add iOS targets, an `iosApp/` folder or macOS CI jobs. Do write shared code as if iOS existed: keep code in `commonMain` and platform APIs behind small `expect`/`actual` interfaces, so adding iOS later means a shell app and a few `actual` files, not a rewrite.

## Commands

```bash
./gradlew check                                  # detekt, ktlint, Android lint and all tests
./gradlew :shared:core:domain:testAndroidHostTest   # one module's tests while iterating
./gradlew ktlintFormat                           # fix formatting
./gradlew :androidApp:assembleDebug
```

No Android SDK on the machine? Run any of these in a container: `scripts/mobile-docker.sh check` (same arguments as `./gradlew`). It is how Claude sessions verify mobile changes.

Release builds talk to `https://api.encomendo.com.br`. Debug builds talk to the local stack (`make dev-up`) at `http://10.0.2.2:8081`, the host machine as the emulator sees it. On a phone, run `adb reverse tcp:8081 tcp:8081` and build with `-Pencomendo.apiBaseUrl=http://localhost:8081`. Release builds refuse plain HTTP on every Android version (`androidApp/src/main/res/xml/network_security_config.xml`; Android 8.x would allow it by default). Debug builds replace that file to allow it to those two hosts only.

Each module configures its own plugins (see `shared/core/ui/build.gradle.kts` for a Compose module, `shared/core/domain/build.gradle.kts` for a plain one). Copy the closest one when you add a module, and add it to `settings.gradle.kts`.

## Libraries

Ktor client (OkHttp engine) with kotlinx.serialization, Room 3 (`androidx.room3`, the multiplatform Room) with `BundledSQLiteDriver`, DataStore, Koin, AndroidX Lifecycle ViewModel and Navigation Compose (multiplatform builds), Coil 3, kotlinx-datetime.

**Do not use** Hilt, Retrofit, Room 2 (`androidx.room`), `java.time` or any Android-only API in `commonMain`. Adding a library needs a one-line justification in the PR.

Shared modules use the Android-KMP library plugin (`com.android.kotlin.multiplatform.library`) with the `kotlin { android { … } }` block, never `com.android.library`; only `androidApp` applies `com.android.application`. See `docs/adr/0005-room3-and-agp9.md`.

## Layers

`XScreen` (Compose) → `XViewModel` exposing `StateFlow<XUiState>` → use case in `core:domain` → repository interface in `core:domain` → implementation in `core:data` (uses `core:network` and `core:database`)

- **`core:domain`** is `commonMain` only, with no platform or library imports beyond coroutines and kotlinx-datetime. Its tests run on the JVM in seconds.
- **Screens** are stateless: they take a `UiState` and callbacks. A thin `XRoute` composable connects the ViewModel to the screen, so screens can be previewed and tested without Koin. Navigation keys are `XDestination` objects in `core:navigation`, so they never clash with an `XRoute`.
- **ViewModels** expose one `StateFlow<XUiState>` and functions for user actions. One-shot effects (navigate, open a wa.me link, show a toast) go through a `Channel`-backed `Flow`, never through state flags you have to reset.
- **`XUiState`** is an immutable `data class`, or a `sealed interface` for Loading / Content / Error.
- **Repositories** return domain models, never DTOs or Room entities. Map at the boundary in `core:data`.
- Inject dispatchers. Never hard-code `Dispatchers.IO` in a class; tests replace them.
- Feature modules follow the menu (`hoje`, `pedidos`, `compras`, `producao`, `estoque`, `catalogo`, `clientes`, `financeiro` …), matching the web admin features one to one. Code inside them is English.

## expect / actual

- Code goes in `commonMain` unless it touches a platform API.
- Platform code sits behind an interface in `commonMain` (for example `PhotoPicker`, `LinkOpener`), with a small `actual` in `androidMain`, wired in `core:di`'s `platformModule`.
- Keep `actual` files thin: no business logic, no mapping.

## Kotlin style

- Follow ktlint and detekt. Fix their findings; don't suppress them without a comment saying why.
- `val` over `var`, and immutable collections in public APIs.
- No `!!`. Handle the null case, or use `requireNotNull` with a message where null would be a bug.
- Errors: the network layer maps problem+json to a typed `ApiError`. Repositories throw `Failure` (`core:domain`), mapped from `ApiError` in `core:data` by its code; they never return `Result`. ViewModels catch `Failure` and nothing broader. Never catch `CancellationException`.
- KDoc on public classes, functions and every `expect` declaration.
- Money is `Long` cents plus a currency code. Format with the pt-BR locale only in the UI layer.

## Auth and offline

- Tokens go through `core:security`'s `TokenStore` (Keystore + DataStore). Never put a token in plain DataStore, a log or a crash report.
- `TokenStore` keeps the whole session (user, store, role, permissions) sealed with its tokens and replaces them together, so offline cold starts still know what the user may do (`docs/adr/0015-android-keeps-the-session-with-its-tokens.md`).
- Every call that spends the refresh token (the client's refresh, switch-tenant, a refresh on start) takes its turn in `core:network`'s `SessionTurns`, one per process: two at once would leave one holding a superseded token, and its next use revokes the family (ADR 0013).
- Sair clears `TokenStore` first and tells the server without waiting, so leaving works offline (`docs/adr/0016-android-logout-clears-the-device-first.md`).
- The Ktor Auth bearer plugin refreshes once on a 401. It logs out only when the server refuses the refresh (400, 401, 403); offline, 429 and 5xx keep the session (`docs/adr/0012-refresh-failure-keeps-offline-sessions.md`).
- After login the app opens the management graph, filtered by permissions, or the store picker when the session has no store.
- Offline: recent orders, the receiving checklist and the production board are cached in Room. Writes made offline, including receiving photos, queue and retry with WorkManager, always with an `Idempotency-Key`.

## Product and design rules

- Management parity with web admin: same endpoints, same wording, same rules. Check the parity map in `docs/architecture.md` section 8.4.
- All UI text is pt-BR from string resources. Niche words come from the tenant's template config.
- **Status labels come from the order's status workflow**, which each store picks. Show the label the API sends and choose the badge color from the status's **phase** through the shared theme tokens (they match web). Never branch on a label or key.
- Status is always icon + word. Alerts ("Pode atrasar", "Atrasada") sit on top with their own icon and word.
- Costs and margins show only when the API sends them (owner). Never compute or cache them for other roles.
- Prices, deposits, dates and alerts come from the API. The app never calculates them.
- WhatsApp is a `wa.me` link the API returns. Open it through the platform `LinkOpener`.
- Workshop screens (receiving, production board): 18 sp body, 56 dp touch targets, 64 dp primary action. They are used with dirty hands and gloves.
- The portal builder previews the store page in a WebView of the preview URL; it does not render the store page itself.

## Testing

- `commonTest`: kotlin.test + Turbine + coroutines-test for ViewModels, use cases and repositories, with fakes from `core:testing`. No mocking library; fakes are written by hand.
- `core:network`: Ktor `MockEngine`, including problem+json errors and the 401 → refresh path.
- `core:database`: in-memory Room tests, including migrations.
- Compose Multiplatform UI tests for the order and receiving screens. Test status badges with at least two workflows.
- Shared tests run on the JVM and Android.
- Every bug fix starts with a failing test.
