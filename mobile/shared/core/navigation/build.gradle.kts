plugins {
    alias(libs.plugins.kotlin.multiplatform)
    alias(libs.plugins.android.kmp.library)
    alias(libs.plugins.compose.multiplatform)
    alias(libs.plugins.compose.compiler)
    alias(libs.plugins.kotlin.serialization)
}

kotlin {
    jvmToolchain(21)

    android {
        namespace = "br.com.encomendo.core.navigation"
        compileSdk =
            libs.versions.android.compileSdk
                .get()
                .toInt()
        minSdk =
            libs.versions.android.minSdk
                .get()
                .toInt()
    }

    sourceSets {
        commonMain.dependencies {
            implementation(project(":shared:core:ui"))
            implementation(project(":shared:feature:hoje"))
            implementation(libs.jetbrains.navigation.compose)
            implementation(libs.kotlinx.serialization.core)
        }
    }
}
