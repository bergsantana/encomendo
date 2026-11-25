# 0005 — Room 3 and the Android-KMP library plugin

- Status: accepted
- Date: 2026-11-24

## Context

The architecture names "Room KMP" for the Android app's offline cache, and the build uses Android Gradle Plugin 9. Since the architecture was written:

- Room 3.0 became the multiplatform Room. It lives in a new package (`androidx.room3`), generates only Kotlin through KSP, and requires a `SQLiteDriver`.
- AGP 9 no longer lets the Kotlin Multiplatform plugin share a module with `com.android.library` or `com.android.application`. Shared modules use `com.android.kotlin.multiplatform.library`.

## Decision

- Use Room 3 (`androidx.room3`) with `BundledSQLiteDriver` from `androidx.sqlite:sqlite-bundled`.
- Shared modules apply `com.android.kotlin.multiplatform.library` and configure the Android target in `kotlin { android { … } }`. Only `androidApp` applies `com.android.application`.
- Milestone 1 configures each module directly instead of through `build-logic` convention plugins: with seven small modules the duplication is a few lines, and it avoids compiling Kotlin 2.4 plugin APIs with Gradle's embedded Kotlin. Move the shared setup into `build-logic` once duplication starts to hurt.
- Static analysis uses detekt 2 (`dev.detekt`), because detekt 1.x does not support Kotlin 2.3+, plus ktlint through `org.jlleitschuh.gradle.ktlint`.

## Consequences

- No Room 2 code or docs apply; examples must use the `androidx.room3` imports.
- Adding iOS later means adding iOS targets to each shared module (or to the convention plugin, if one exists by then); the shared code does not change.
- detekt 2 is still an alpha; pin it and update deliberately.
