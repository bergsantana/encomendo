package br.com.encomendo.core.di

import br.com.encomendo.core.database.databaseBuilder
import org.koin.android.ext.koin.androidContext
import org.koin.core.module.Module
import org.koin.dsl.module

/** Android bindings; needs androidContext() from startKoin in the Application. */
actual val platformModule: Module =
    module {
        single { databaseBuilder(androidContext()) }
    }
