package br.com.encomendo.feature.catalogo.di

import br.com.encomendo.feature.catalogo.AttributesViewModel
import br.com.encomendo.feature.catalogo.CatalogChanges
import br.com.encomendo.feature.catalogo.CategoriesViewModel
import br.com.encomendo.feature.catalogo.ItemFormRepositories
import br.com.encomendo.feature.catalogo.ItemFormViewModel
import br.com.encomendo.feature.catalogo.ItemsViewModel
import org.koin.core.module.Module
import org.koin.core.module.dsl.factoryOf
import org.koin.core.module.dsl.viewModel
import org.koin.core.module.dsl.viewModelOf
import org.koin.dsl.module

/**
 * Catálogo: the Itens list, the item form, Categorias and Campos. The repositories, the photo picker and the
 * session come from core:di. [CatalogChanges] is shared by the screens so the list learns what they changed; it
 * keeps nothing, so one for the process leaks nothing between stores.
 */
val catalogoModule: Module =
    module {
        single { CatalogChanges() }
        factoryOf(::ItemFormRepositories)
        viewModelOf(::ItemsViewModel)
        viewModelOf(::CategoriesViewModel)
        viewModelOf(::AttributesViewModel)
        // The item to edit comes from the route: `parametersOf(itemId)`, null for a new item.
        viewModel { parameters ->
            ItemFormViewModel(
                itemId = parameters.getOrNull(),
                observeSession = get(),
                repositories = get(),
                picker = get(),
                changes = get(),
            )
        }
    }
