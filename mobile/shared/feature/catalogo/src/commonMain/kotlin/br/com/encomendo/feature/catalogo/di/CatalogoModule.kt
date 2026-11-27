package br.com.encomendo.feature.catalogo.di

import br.com.encomendo.core.domain.catalog.ArchiveItem
import br.com.encomendo.core.domain.catalog.ChangeAttributes
import br.com.encomendo.core.domain.catalog.ChangeCategories
import br.com.encomendo.core.domain.catalog.ChangePhotos
import br.com.encomendo.core.domain.catalog.GetItem
import br.com.encomendo.core.domain.catalog.ListAttributes
import br.com.encomendo.core.domain.catalog.ListCategories
import br.com.encomendo.core.domain.catalog.ListItems
import br.com.encomendo.core.domain.catalog.SaveItem
import br.com.encomendo.core.domain.catalog.SetItemPublished
import br.com.encomendo.core.domain.settings.GetStoreSettings
import br.com.encomendo.core.domain.supplier.CreateSupplier
import br.com.encomendo.core.domain.supplier.ListSuppliers
import br.com.encomendo.feature.catalogo.AttributesViewModel
import br.com.encomendo.feature.catalogo.CatalogChanges
import br.com.encomendo.feature.catalogo.CategoriesViewModel
import br.com.encomendo.feature.catalogo.ItemFormUseCases
import br.com.encomendo.feature.catalogo.ItemFormViewModel
import br.com.encomendo.feature.catalogo.ItemsViewModel
import org.koin.core.module.Module
import org.koin.core.module.dsl.factoryOf
import org.koin.core.module.dsl.viewModel
import org.koin.core.module.dsl.viewModelOf
import org.koin.dsl.module

/**
 * Catálogo: the catalog and supplier use cases and the screens' ViewModels (the Itens list, the item form, Categorias
 * and Campos). The repositories, the photo picker and the session come from core:di. [CatalogChanges] is shared by
 * the screens so the list learns what they changed; it keeps nothing, so one for the process leaks nothing between
 * stores.
 */
val catalogoModule: Module =
    module {
        single { CatalogChanges() }
        factoryOf(::ListItems)
        factoryOf(::GetItem)
        factoryOf(::SaveItem)
        factoryOf(::SetItemPublished)
        factoryOf(::ArchiveItem)
        factoryOf(::ListCategories)
        factoryOf(::ChangeCategories)
        factoryOf(::ListAttributes)
        factoryOf(::ChangeAttributes)
        factoryOf(::ChangePhotos)
        factoryOf(::ListSuppliers)
        factoryOf(::CreateSupplier)
        factoryOf(::GetStoreSettings)
        factoryOf(::ItemFormUseCases)
        viewModelOf(::ItemsViewModel)
        viewModelOf(::CategoriesViewModel)
        viewModelOf(::AttributesViewModel)
        // The item to edit comes from the route: `parametersOf(itemId)`, null for a new item.
        viewModel { parameters ->
            ItemFormViewModel(
                itemId = parameters.getOrNull(),
                observeSession = get(),
                useCases = get(),
                changePhotos = get(),
                createSupplier = get(),
                picker = get(),
                changes = get(),
            )
        }
    }
