pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "encomendo"

include(
    ":androidApp",
    ":shared:core:domain",
    ":shared:core:ui",
    ":shared:core:database",
    ":shared:core:security",
    ":shared:core:data",
    ":shared:core:testing",
    ":shared:core:network",
    ":shared:core:platform",
    ":shared:core:navigation",
    ":shared:core:di",
    ":shared:feature:auth",
    ":shared:feature:hoje",
    ":shared:feature:members",
)
