plugins {
    alias(libs.plugins.kotlin.multiplatform)
    alias(libs.plugins.android.kmp.library)
}

kotlin {
    jvmToolchain(21)

    android {
        namespace = "br.com.encomendo.core.di"
        compileSdk =
            libs.versions.android.compileSdk
                .get()
                .toInt()
        minSdk =
            libs.versions.android.minSdk
                .get()
                .toInt()
        withHostTestBuilder {} // runs commonTest on the JVM
    }

    sourceSets {
        commonMain.dependencies {
            implementation(project(":shared:core:database"))
            implementation(project(":shared:core:domain"))
            implementation(project(":shared:core:security"))
            implementation(project(":shared:core:data"))
            implementation(project(":shared:core:navigation"))
            implementation(project(":shared:feature:auth"))
            implementation(project(":shared:feature:catalogo"))
            implementation(project(":shared:feature:members"))
            api(libs.koin.core)
            implementation(libs.kotlinx.coroutines.core)
        }
        androidMain.dependencies {
            implementation(project(":shared:core:platform"))
            implementation(libs.koin.android)
        }
        commonTest.dependencies {
            implementation(project(":shared:core:testing"))
            implementation(libs.ktor.client.mock)
            implementation(libs.kotlin.test)
            implementation(libs.kotlinx.coroutines.test)
        }
    }
}
