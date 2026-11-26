package br.com.encomendo.feature.auth.di

import br.com.encomendo.core.domain.session.CreateStore
import br.com.encomendo.core.domain.session.LogIn
import br.com.encomendo.core.domain.session.LogOut
import br.com.encomendo.core.domain.session.SignUp
import br.com.encomendo.core.domain.session.SwitchStore
import br.com.encomendo.feature.auth.AccountViewModel
import br.com.encomendo.feature.auth.LoginViewModel
import br.com.encomendo.feature.auth.SignUpViewModel
import br.com.encomendo.feature.auth.StorePickerViewModel
import org.koin.core.module.Module
import org.koin.core.module.dsl.factoryOf
import org.koin.core.module.dsl.viewModelOf
import org.koin.dsl.module

/**
 * The use cases and ViewModels of the auth graph, the store picker and the
 * account bar. The repositories come from core:di's dataModule,
 * ObserveSession from the navigation module.
 */
val authModule: Module =
    module {
        factoryOf(::LogIn)
        factoryOf(::SignUp)
        factoryOf(::LogOut)
        factoryOf(::SwitchStore)
        // A new one per picker: it remembers a store created but not yet opened, for the retry.
        factoryOf(::CreateStore)
        viewModelOf(::LoginViewModel)
        viewModelOf(::SignUpViewModel)
        viewModelOf(::AccountViewModel)
        viewModelOf(::StorePickerViewModel)
    }
