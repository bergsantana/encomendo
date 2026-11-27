package br.com.encomendo.core.testing

import br.com.encomendo.core.domain.catalog.Fetched
import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.paging.Page
import br.com.encomendo.core.domain.supplier.Supplier
import br.com.encomendo.core.domain.supplier.SupplierDraft
import br.com.encomendo.core.domain.supplier.SupplierRepository
import br.com.encomendo.core.platform.PhotoPicker

/**
 * FakeSupplierRepository serves the suppliers of a [FakeCatalogState], in memory: names are unique ignoring
 * case, a stale `version` is a conflict, and [FakeCatalogState.failure] and [FakeCatalogState.offlineSince]
 * apply as for the catalog.
 */
class FakeSupplierRepository(
    private val state: FakeCatalogState = FakeCatalogState(),
) : SupplierRepository {
    override suspend fun page(
        query: String?,
        cursor: String?,
        limit: Int,
    ): Fetched<Page<Supplier>> {
        val rows = state.suppliers.filter { query == null || it.name.contains(query, ignoreCase = true) }
        return state.read(pageOf(rows, cursor, limit))
    }

    override suspend fun supplier(id: String): Fetched<Supplier> =
        state.read(state.suppliers.firstOrNull { it.id == id } ?: throw Failure.NotFound())

    override suspend fun create(draft: SupplierDraft): Supplier {
        state.check()
        if (state.suppliers.any { it.name.equals(draft.name, ignoreCase = true) }) throw Failure.AlreadyTaken("name")
        val created = Supplier(state.newId(), draft.name, draft.whatsappPhone, version = 1)
        state.suppliers = state.suppliers + created
        return created
    }

    override suspend fun replace(
        id: String,
        version: Int,
        draft: SupplierDraft,
    ): Supplier {
        state.check()
        val current = state.suppliers.firstOrNull { it.id == id } ?: throw Failure.NotFound()
        if (current.version != version) throw Failure.VersionConflict()
        val changed = current.copy(name = draft.name, whatsappPhone = draft.whatsappPhone, version = version + 1)
        state.suppliers = state.suppliers.map { if (it.id == id) changed else it }
        return changed
    }
}

/**
 * FakePhotoPicker answers with the photos a test queues, as the platform picker would after the user chose:
 * [gallery] and [camera] hold what the next calls return, and null means the user backed out.
 */
class FakePhotoPicker : PhotoPicker {
    /** The photos the next gallery picks return, in order; none left means the user backed out. */
    val gallery = ArrayDeque<PickedPhoto?>()

    /** The photos the next camera shots return, in order; none left means the user backed out. */
    val camera = ArrayDeque<PickedPhoto?>()

    /** How many times the gallery was opened. */
    var galleryOpened: Int = 0
        private set

    /** How many times the camera was opened. */
    var cameraOpened: Int = 0
        private set

    override suspend fun pickFromGallery(): PickedPhoto? {
        galleryOpened++
        return gallery.removeFirstOrNull()
    }

    override suspend fun takePhoto(): PickedPhoto? {
        cameraOpened++
        return camera.removeFirstOrNull()
    }
}
