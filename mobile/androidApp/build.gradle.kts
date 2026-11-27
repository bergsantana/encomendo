plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.compose.compiler)
}

android {
    namespace = "br.com.encomendo.android"
    compileSdk =
        libs.versions.android.compileSdk
            .get()
            .toInt()

    defaultConfig {
        applicationId = "br.com.encomendo.app"
        minSdk =
            libs.versions.android.minSdk
                .get()
                .toInt()
        targetSdk =
            libs.versions.android.targetSdk
                .get()
                .toInt()
        versionCode = (System.getenv("VERSION_CODE") ?: "1").toInt()
        versionName = System.getenv("VERSION_NAME") ?: "0.1.0"
        // The production API has its own host; `api` is a reserved store slug (ADR 0011).
        buildConfigField("String", "API_BASE_URL", "\"https://api.encomendo.com.br\"")
    }

    // Release signing comes only from environment secrets in release.yml.
    val releaseKeystore = System.getenv("ANDROID_KEYSTORE_PATH")
    signingConfigs {
        if (releaseKeystore != null) {
            create("release") {
                storeFile = file(releaseKeystore)
                storePassword = System.getenv("ANDROID_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("ANDROID_KEY_ALIAS")
                keyPassword = System.getenv("ANDROID_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        debug {
            // The local stack (make dev-up). 10.0.2.2 is the host machine as the emulator sees it;
            // on a phone, run `adb reverse tcp:8081 tcp:8081` and pass -Pencomendo.apiBaseUrl=http://localhost:8081.
            val localApi = providers.gradleProperty("encomendo.apiBaseUrl").getOrElse("http://10.0.2.2:8081")
            buildConfigField("String", "API_BASE_URL", "\"$localApi\"")
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            if (releaseKeystore != null) signingConfig = signingConfigs.getByName("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }

    buildFeatures {
        compose = true
        buildConfig = true // API_BASE_URL
    }
}

dependencies {
    implementation(project(":shared:core:ui"))
    implementation(project(":shared:core:navigation"))
    implementation(project(":shared:core:di"))
    implementation(project(":shared:core:platform")) // MainActivity attaches itself to the photo picker
    implementation(libs.androidx.activity.compose)
    implementation(libs.koin.android)
}
