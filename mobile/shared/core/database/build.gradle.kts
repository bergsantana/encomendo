plugins {
    alias(libs.plugins.kotlin.multiplatform)
    alias(libs.plugins.android.kmp.library)
    alias(libs.plugins.ksp)
    alias(libs.plugins.room)
}

kotlin {
    jvmToolchain(21)

    android {
        namespace = "br.com.encomendo.core.database"
        compileSdk =
            libs.versions.android.compileSdk
                .get()
                .toInt()
        minSdk =
            libs.versions.android.minSdk
                .get()
                .toInt()
        withHostTestBuilder {} // runs the in-memory Room and migration tests on the JVM
    }

    sourceSets {
        commonMain.dependencies {
            api(libs.room.runtime)
            implementation(libs.sqlite.bundled)
            implementation(libs.kotlinx.coroutines.core)
        }
        // Host tests open real SQLite files on the JVM; the Android build of the bundled driver only has
        // Android native libraries, so the JVM build replaces it there (see the exclude below).
        getByName("androidHostTest").dependencies {
            implementation(libs.sqlite.bundled.jvm)
            implementation(libs.kotlin.test)
            implementation(libs.kotlinx.coroutines.test)
        }
    }
}

room3 {
    // Exported schemas are committed, so every migration can be tested against them.
    schemaDirectory("$projectDir/schemas")
}

dependencies {
    add("kspAndroid", libs.room.compiler)
}

configurations.matching { it.name.startsWith("androidHostTest") }.configureEach {
    exclude(group = "androidx.sqlite", module = "sqlite-bundled-android")
}
