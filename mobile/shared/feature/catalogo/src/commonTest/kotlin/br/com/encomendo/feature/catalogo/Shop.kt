package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.CostChange
import br.com.encomendo.core.domain.catalog.Item
import br.com.encomendo.core.domain.catalog.ItemDraft
import br.com.encomendo.core.domain.catalog.ItemRepository
import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.catalog.SupplierLinkDraft
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.supplier.Supplier
import br.com.encomendo.core.platform.PhotoPicker
import br.com.encomendo.core.testing.FakeCatalogRepository
import br.com.encomendo.core.testing.FakeCatalogState
import br.com.encomendo.core.testing.FakePhotoPicker
import br.com.encomendo.core.testing.FakeSessionRepository
import br.com.encomendo.core.testing.FakeSettingsRepository
import br.com.encomendo.core.testing.FakeSupplierRepository
import br.com.encomendo.core.testing.testSession
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.launchIn
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent

// The Catálogo ViewModel tests' world: one store's catalog behind fakes, seen by the owner, an admin or a viewer.

/** Who is signed in, which decides what the API sends (costs) and what the screens offer (editing). */
internal enum class Who(
    val role: Role,
    val permissions: Set<String>,
) {
    /** Sees costs and writes. */
    Owner(Role.Owner, setOf("catalog:read", "catalog:write")),

    /** Writes, but is sent no cost. */
    Admin(Role.Admin, setOf("catalog:read", "catalog:write")),

    /** Reads only. */
    Viewer(Role.Viewer, setOf("catalog:read")),
}

internal const val BENCH_ID = "item-bench"
internal const val SADDLE_ID = "item-saddle"
internal const val SUPPLIER_ID = "supplier-couros"
internal const val OTHER_SUPPLIER_ID = "supplier-eixo"
internal const val SAVED_AT = 1_700_000_000_000L

/** A store's catalog behind fakes, seen by [who]. Seeded with two suppliers; tests add the items they need. */
@OptIn(ExperimentalCoroutinesApi::class) // runCurrent
internal class Shop(
    val scope: TestScope,
    val who: Who,
) {
    val state =
        FakeCatalogState().apply {
            seesCosts = who == Who.Owner
            suppliers =
                listOf(
                    Supplier(SUPPLIER_ID, "Couros Paulista", "5511987654321", version = 1),
                    Supplier(OTHER_SUPPLIER_ID, "Eixo Forte", "5511912345678", version = 1),
                )
        }
    val catalog = FakeCatalogRepository(state)
    val suppliers = FakeSupplierRepository(state)
    val settings = FakeSettingsRepository()
    val picker = FakePhotoPicker()
    val changes = CatalogChanges()
    val sessions =
        FakeSessionRepository(
            testSession(permissions = who.permissions).copy(role = who.role),
        )

    /** The change notices the screens published, in order. */
    val published = mutableListOf<CatalogChange>()

    init {
        changes.changes.onEach { published += it }.launchIn(scope.backgroundScope)
    }

    /**
     * Adds an item as the server would hold it, through the fake's own rules: the owner's items carry a cost of
     * R$ 40,00, and the other roles' carry none.
     */
    suspend fun seedItem(
        name: String,
        configure: ItemDraft.() -> ItemDraft = { this },
    ): Item {
        val cost = if (state.seesCosts) CostChange.Set(4_000) else CostChange.Keep
        return catalog.createItem(draft(name, cost = cost).configure())
    }

    fun observeSession(): ObserveSession = ObserveSession(sessions)

    fun items(): ItemsViewModel =
        ItemsViewModel(observeSession(), catalog, catalog, changes).also { collect(it.uiState) }

    fun categories(): CategoriesViewModel =
        CategoriesViewModel(observeSession(), catalog, changes).also {
            collect(it.uiState)
        }

    fun attributes(): AttributesViewModel =
        AttributesViewModel(observeSession(), catalog, changes).also {
            collect(it.uiState)
        }

    fun form(
        itemId: String? = null,
        items: ItemRepository = catalog,
        picker: PhotoPicker = this.picker,
    ): ItemFormViewModel =
        ItemFormViewModel(
            itemId = itemId,
            observeSession = observeSession(),
            repositories = ItemFormRepositories(items, catalog, catalog, suppliers, catalog, settings),
            picker = picker,
            changes = changes,
        ).also { collect(it.uiState) }

    private fun <T> collect(flow: kotlinx.coroutines.flow.StateFlow<T>) {
        // As the screen does: the state is shared while someone watches it.
        scope.backgroundScope.launch { flow.collect {} }
    }
}

/** Runs everything queued, then returns [read] of the result. */
@OptIn(ExperimentalCoroutinesApi::class) // runCurrent
internal fun <T> Shop.settle(read: () -> T): T {
    scope.runCurrent()
    return read()
}

/** A draft of an item bought from [SUPPLIER_ID]; tests change what they care about. */
internal fun draft(
    name: String,
    priceCents: Long = 10_000,
    cost: CostChange = CostChange.Keep,
): ItemDraft =
    ItemDraft(
        name = name,
        description = "",
        categoryId = null,
        sourcingMode = SourcingMode.ToOrder,
        priceCents = priceCents,
        productionDays = null,
        productionCost = CostChange.Keep,
        mainSupplier = SupplierLinkDraft(SUPPLIER_ID, leadDays = 7, cost = cost),
        backupSupplier = null,
        attributeValueIds = emptyList(),
        options = emptyList(),
        published = false,
    )

/** Records the drafts a form sends, and refuses them with [refuse] when set, in front of the fake. */
internal class RecordingItems(
    private val delegate: ItemRepository,
) : ItemRepository by delegate {
    val drafts = mutableListOf<ItemDraft>()
    var refuse: Failure? = null

    override suspend fun createItem(draft: ItemDraft): Item {
        drafts += draft
        refuse?.let { throw it }
        return delegate.createItem(draft)
    }

    override suspend fun replaceItem(
        id: String,
        version: Int,
        draft: ItemDraft,
    ): Item {
        drafts += draft
        refuse?.let { throw it }
        return delegate.replaceItem(id, version, draft)
    }
}

/** Types a name and a price into the form. */
internal fun fill(
    form: ItemFormViewModel,
    name: String,
    price: String,
) = form.onEdit(null) { it.copy(name = name, price = price) }

/** A photo the picker can hand out: any JPEG passes the fake API. */
internal fun jpeg() = PickedPhoto(ByteArray(10), "image/jpeg")
