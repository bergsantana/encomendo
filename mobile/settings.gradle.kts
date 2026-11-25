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
    ":shared:core:navigation",
    ":shared:core:di",
    ":shared:feature:hoje",
)
