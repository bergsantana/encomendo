package br.com.encomendo.feature.auth.di

import br.com.encomendo.core.domain.session.LogIn
import br.com.encomendo.core.domain.session.LogOut
import br.com.encomendo.feature.auth.AccountViewModel
import br.com.encomendo.feature.auth.LoginViewModel
import org.koin.core.module.Module
import org.koin.core.module.dsl.factoryOf
import org.koin.core.module.dsl.viewModelOf
import org.koin.dsl.module

/**
 * The auth graph's and the account bar's use cases and ViewModels. The
 * repositories come from core:di's dataModule, ObserveSession from the
 * navigation module.
 */
val authModule: Module =
    module {
        factoryOf(::LogIn)
        factoryOf(::LogOut)
        viewModelOf(::LoginViewModel)
        viewModelOf(::AccountViewModel)
    }
