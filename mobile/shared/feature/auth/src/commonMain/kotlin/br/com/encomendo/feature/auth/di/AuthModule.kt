package br.com.encomendo.feature.auth.di

import br.com.encomendo.core.domain.session.LogIn
import br.com.encomendo.feature.auth.LoginViewModel
import org.koin.core.module.Module
import org.koin.core.module.dsl.factoryOf
import org.koin.core.module.dsl.viewModelOf
import org.koin.dsl.module

/** The auth graph's use cases and ViewModels; the repositories come from core:di's dataModule. */
val authModule: Module =
    module {
        factoryOf(::LogIn)
        viewModelOf(::LoginViewModel)
    }
