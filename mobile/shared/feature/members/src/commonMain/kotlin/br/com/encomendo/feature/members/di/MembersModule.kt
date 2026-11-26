package br.com.encomendo.feature.members.di

import br.com.encomendo.core.domain.members.ChangeMemberRole
import br.com.encomendo.core.domain.members.ListMembers
import br.com.encomendo.core.domain.members.RemoveMember
import br.com.encomendo.feature.members.MembersViewModel
import org.koin.core.module.Module
import org.koin.core.module.dsl.factoryOf
import org.koin.core.module.dsl.viewModelOf
import org.koin.dsl.module

/** Equipe: the team's use cases and the screen's ViewModel. The repositories come from core:di. */
val membersModule: Module =
    module {
        factoryOf(::ListMembers)
        factoryOf(::ChangeMemberRole)
        factoryOf(::RemoveMember)
        viewModelOf(::MembersViewModel)
    }
