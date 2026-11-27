plugins {
    alias(libs.plugins.kotlin.multiplatform)
    alias(libs.plugins.android.kmp.library)
    alias(libs.plugins.compose.multiplatform)
    alias(libs.plugins.compose.compiler)
}

kotlin {
    jvmToolchain(21)

    android {
        namespace = "br.com.encomendo.feature.catalogo"
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
            implementation(project(":shared:core:platform")) // PhotoPicker
            implementation(libs.compose.ui.backhandler) // BackHandler: Back is "Cancelar" and asks before discarding
            implementation(libs.compose.components.resources)
            implementation(libs.jetbrains.lifecycle.viewmodel.compose)
            implementation(libs.jetbrains.lifecycle.runtime.compose)
            api(libs.koin.core) // catalogoModule is a Koin Module
            implementation(libs.koin.compose.viewmodel)
            implementation(libs.kotlinx.datetime) // "Mostrando o catálogo salvo em …": an instant as a local date
        }
        commonTest.dependencies {
            implementation(project(":shared:core:testing"))
            implementation(libs.kotlin.test)
            implementation(libs.kotlinx.coroutines.test)
            implementation(libs.turbine)
        }
    }
}

compose.resources {
    packageOfResClass = "br.com.encomendo.feature.catalogo.resources"
}
