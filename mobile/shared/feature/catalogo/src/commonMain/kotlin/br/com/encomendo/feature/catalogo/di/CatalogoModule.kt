package br.com.encomendo.feature.catalogo.di

import br.com.encomendo.core.domain.catalog.ArchiveItem
import br.com.encomendo.core.domain.catalog.ArchiveKit
import br.com.encomendo.core.domain.catalog.ChangeAttributes
import br.com.encomendo.core.domain.catalog.ChangeCategories
import br.com.encomendo.core.domain.catalog.ChangePhotos
import br.com.encomendo.core.domain.catalog.GetItem
import br.com.encomendo.core.domain.catalog.GetKit
import br.com.encomendo.core.domain.catalog.ListAttributes
import br.com.encomendo.core.domain.catalog.ListCategories
import br.com.encomendo.core.domain.catalog.ListItems
import br.com.encomendo.core.domain.catalog.ListKits
import br.com.encomendo.core.domain.catalog.SaveItem
import br.com.encomendo.core.domain.catalog.SaveKit
import br.com.encomendo.core.domain.catalog.SetItemPublished
import br.com.encomendo.core.domain.catalog.SetKitPublished
import br.com.encomendo.core.domain.niche.NicheWords
import br.com.encomendo.core.domain.settings.GetStoreSettings
import br.com.encomendo.core.domain.supplier.CreateSupplier
import br.com.encomendo.core.domain.supplier.ListSuppliers
import br.com.encomendo.feature.catalogo.AttributesViewModel
import br.com.encomendo.feature.catalogo.CatalogChanges
import br.com.encomendo.feature.catalogo.CategoriesViewModel
import br.com.encomendo.feature.catalogo.ItemFormUseCases
import br.com.encomendo.feature.catalogo.ItemFormViewModel
import br.com.encomendo.feature.catalogo.ItemsViewModel
import br.com.encomendo.feature.catalogo.KitFormUseCases
import br.com.encomendo.feature.catalogo.KitFormViewModel
import br.com.encomendo.feature.catalogo.KitsViewModel
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
        factoryOf(::ListKits)
        factoryOf(::GetKit)
        factoryOf(::SaveKit)
        factoryOf(::SetKitPublished)
        factoryOf(::ArchiveKit)
        factoryOf(::KitFormUseCases)
        // Until the store's template exists (milestone 7), every store reads the default niche words (ADR 0025).
        single { NicheWords.Default }
        viewModelOf(::ItemsViewModel)
        viewModelOf(::KitsViewModel)
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
        // The kit to edit comes from the route: `parametersOf(kitId)`, null for a new kit.
        viewModel { parameters ->
            KitFormViewModel(
                kitId = parameters.getOrNull(),
                observeSession = get(),
                useCases = get(),
                changePhotos = get(),
                picker = get(),
                changes = get(),
                niche = get(),
            )
        }
    }
