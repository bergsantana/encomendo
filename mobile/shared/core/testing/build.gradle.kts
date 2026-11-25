plugins {
    alias(libs.plugins.kotlin.multiplatform)
    alias(libs.plugins.android.kmp.library)
}

// Hand-written fakes and test helpers shared by every module's commonTest (mobile/CLAUDE.md, Testing).
kotlin {
    jvmToolchain(21)

    android {
        namespace = "br.com.encomendo.core.testing"
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
            api(project(":shared:core:domain"))
            api(project(":shared:core:security"))
            api(libs.kotlinx.coroutines.core)
        }
    }
}
