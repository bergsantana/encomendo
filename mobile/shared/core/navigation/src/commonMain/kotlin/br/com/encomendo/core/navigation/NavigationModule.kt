package br.com.encomendo.core.navigation

import br.com.encomendo.core.domain.session.ObserveSignedIn
import org.koin.core.module.Module
import org.koin.core.module.dsl.factoryOf
import org.koin.core.module.dsl.viewModelOf
import org.koin.dsl.module

/** The app shell: the session use case and the ViewModel that picks the graph. */
val navigationModule: Module =
    module {
        factoryOf(::ObserveSignedIn)
        viewModelOf(::RootViewModel)
    }
