plugins {
    alias(libs.plugins.kotlin.multiplatform)
    alias(libs.plugins.android.kmp.library)
    alias(libs.plugins.compose.multiplatform)
    alias(libs.plugins.compose.compiler)
}

kotlin {
    jvmToolchain(21)

    android {
        namespace = "br.com.encomendo.feature.hoje"
        compileSdk =
            libs.versions.android.compileSdk
                .get()
                .toInt()
        minSdk =
            libs.versions.android.minSdk
                .get()
                .toInt()
        androidResources { enable = true } // Compose Multiplatform string resources
    }

    sourceSets {
        commonMain.dependencies {
            implementation(project(":shared:core:ui"))
            implementation(libs.compose.components.resources)
        }
    }
}

compose.resources {
    packageOfResClass = "br.com.encomendo.feature.hoje.resources"
}
