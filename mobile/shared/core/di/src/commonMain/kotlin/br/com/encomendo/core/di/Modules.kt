package br.com.encomendo.core.di

import br.com.encomendo.core.database.EncomendoDatabase
import br.com.encomendo.core.database.buildDatabase
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.IO
import org.koin.core.module.Module
import org.koin.dsl.module

/**
 * Platform bindings: the Room builder and, later, the camera, link opener and
 * token store. Each platform provides its own.
 */
expect val platformModule: Module

/** The offline cache and its DAOs. */
val databaseModule: Module =
    module {
        single<EncomendoDatabase> { buildDatabase(builder = get(), queryContext = Dispatchers.IO) }
        single { get<EncomendoDatabase>().syncStateDao() }
    }

/** Every shared module, in the order Koin should load them. Feature modules join as they ship. */
val sharedModules: List<Module> = listOf(platformModule, databaseModule)
