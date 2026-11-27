plugins {
    alias(libs.plugins.kotlin.multiplatform)
    alias(libs.plugins.android.kmp.library)
}

// Small interfaces over platform APIs (the photo picker for now), each with a thin actual in androidMain,
// wired in core:di's platformModule (mobile/CLAUDE.md, expect / actual).
kotlin {
    jvmToolchain(21)

    android {
        namespace = "br.com.encomendo.core.platform"
        compileSdk =
            libs.versions.android.compileSdk
                .get()
                .toInt()
        minSdk =
            libs.versions.android.minSdk
                .get()
                .toInt()
        androidResources { enable = true } // the camera's FileProvider paths
    }

    sourceSets {
        commonMain.dependencies {
            api(project(":shared:core:domain"))
        }
        androidMain.dependencies {
            // ActivityResultContracts: the system photo picker and the camera.
            implementation(libs.androidx.activity)
            implementation(libs.kotlinx.coroutines.core)
        }
    }
}
