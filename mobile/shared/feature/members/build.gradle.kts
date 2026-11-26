plugins {
    alias(libs.plugins.kotlin.multiplatform)
    alias(libs.plugins.android.kmp.library)
    alias(libs.plugins.compose.multiplatform)
    alias(libs.plugins.compose.compiler)
}

kotlin {
    jvmToolchain(21)

    android {
        namespace = "br.com.encomendo.feature.members"
        compileSdk =
            libs.versions.android.compileSdk
                .get()
                .toInt()
        minSdk =
            libs.versions.android.minSdk
                .get()
                .toInt()
        androidResources { enable = true } // Compose Multiplatform string resources
        withHostTestBuilder {} // runs commonTest on the JVM
    }

    sourceSets {
        commonMain.dependencies {
            implementation(project(":shared:core:domain"))
            implementation(project(":shared:core:ui"))
            implementation(libs.compose.components.resources)
            implementation(libs.jetbrains.lifecycle.viewmodel.compose)
            implementation(libs.jetbrains.lifecycle.runtime.compose)
            api(libs.koin.core) // membersModule is a Koin Module
            implementation(libs.koin.compose.viewmodel)
        }
        commonTest.dependencies {
            implementation(project(":shared:core:testing"))
            implementation(libs.kotlin.test)
            implementation(libs.kotlinx.coroutines.test)
        }
    }
}

compose.resources {
    packageOfResClass = "br.com.encomendo.feature.members.resources"
}
