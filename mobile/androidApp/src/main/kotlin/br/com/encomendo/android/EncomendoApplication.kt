package br.com.encomendo.android

import android.app.Application
import br.com.encomendo.core.di.sharedModules
import org.koin.android.ext.koin.androidContext
import org.koin.core.context.startKoin

/** Starts Koin with every shared module before any screen opens. */
class EncomendoApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        startKoin {
            androidContext(this@EncomendoApplication)
            // Release talks to the production API; debug to the local stack (androidApp/build.gradle.kts).
            modules(sharedModules(apiBaseUrl = BuildConfig.API_BASE_URL))
        }
    }
}
