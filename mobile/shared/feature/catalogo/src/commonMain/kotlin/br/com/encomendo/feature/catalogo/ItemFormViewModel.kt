package br.com.encomendo.feature.catalogo

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.catalog.AttributeRepository
import br.com.encomendo.core.domain.catalog.CategoryRepository
import br.com.encomendo.core.domain.catalog.Item
import br.com.encomendo.core.domain.catalog.ItemDraft
import br.com.encomendo.core.domain.catalog.ItemRepository
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.core.domain.catalog.PhotoOwner
import br.com.encomendo.core.domain.catalog.PhotoOwnerKind
import br.com.encomendo.core.domain.catalog.PhotoRepository
import br.com.encomendo.core.domain.catalog.PickedPhoto
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.catalog.isShown
import br.com.encomendo.core.domain.catalog.seesCosts
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.domain.settings.SettingsRepository
import br.com.encomendo.core.domain.settings.StoreSettings
import br.com.encomendo.core.domain.supplier.Supplier
import br.com.encomendo.core.domain.supplier.SupplierRepository
import br.com.encomendo.core.platform.PhotoPicker
import br.com.encomendo.core.ui.failure.FailureMessage
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** ItemFormRepositories are what the item form reads and writes, so [ItemFormViewModel] takes them as one input. */
class ItemFormRepositories(
    val items: ItemRepository,
    val categories: CategoryRepository,
    val attributes: AttributeRepository,
    val suppliers: SupplierRepository,
    val photos: PhotoRepository,
    val settings: SettingsRepository,
)

/**
 * ItemFormViewModel backs the item editor, as the web's item drawer (docs/product/design-document.md, "Catálogo",
 * "Item form"): the same fields, the same checks before anything is sent, the server's own words next to the
 * fields it refuses, and a photo change that goes through at once. [itemId] is the item to edit, or null for a new
 * one. Roles without `catalog:write` read the item and change nothing.
 *
 * The cost fields exist only when the API sends costs to this role, and a cost the owner left alone is not sent,
 * so the server keeps it (ADR 0025).
 */
@Suppress("TooManyFunctions") // One function per thing the user can do on the form, as the web's item store.
class ItemFormViewModel(
    private val itemId: String?,
    private val observeSession: ObserveSession,
    private val repositories: ItemFormRepositories,
    private val picker: PhotoPicker,
    private val changes: CatalogChanges,
) : ViewModel() {
    private val form = MutableStateFlow(ItemFormUiState(itemId = itemId))
    private val effectChannel = Channel<ItemFormEffect>(Channel.BUFFERED)

    /** The form as it shows now. */
    val uiState: StateFlow<ItemFormUiState> = form.asStateFlow()

    /** What the form asks its screen to do, once each. */
    val effects: Flow<ItemFormEffect> = effectChannel.receiveAsFlow()

    init {
        load()
    }

    /** "Tentar de novo" and "Recarregar": reads the item and the choices again, dropping what was typed. */
    fun onReload() = load()

    /** The user typed or picked in [field]; its mark goes. Ignored for roles that cannot write. */
    fun onEdit(
        field: ItemField?,
        change: (ItemFields) -> ItemFields,
    ) {
        if (!form.value.editable) return
        form.update {
            it.copy(
                fields = change(it.fields),
                issues =
                    if (field ==
                        null
                    ) {
                        it.issues
                    } else {
                        it.issues - field
                    },
            )
        }
    }

    /** "Origem" changed. The marks of the fields the old mode had go with them. */
    fun onModeChange(mode: SourcingMode) {
        if (!form.value.editable) return
        form.update { state ->
            val kept = state.issues.filterKeys { it is ItemField.Name || it is ItemField.Price || it.isOption }
            state.copy(fields = state.fields.copy(mode = mode), issues = kept)
        }
    }

    /** "Salvar". An empty or wrong field is marked, never sent. */
    fun onSave() {
        val state = form.value
        if (!state.editable || state.saving) return
        when (val read = state.fields.toDraft(seesCosts = state.showsCosts)) {
            is DraftResult.Problems ->
                form.update {
                    it.copy(
                        issues = read.issues,
                        message = CatalogMessage.CheckFields,
                    )
                }
            is DraftResult.Ready -> persist(read.draft)
        }
    }

    /** "Arquivar". Refused while a kit holds the item. */
    fun onArchive() =
        leaveCatalogue(
            change = { item -> repositories.items.archiveItem(item.id, requireVersion()) },
            notice = CatalogChange::ItemArchived,
        )

    /** "Restaurar". */
    fun onRestore() =
        leaveCatalogue(
            change = { item -> repositories.items.restoreItem(item.id, requireVersion()) },
            notice = CatalogChange::ItemRestored,
        )

    /** "Adicionar fotos" with [source]. Nothing happens when the user backs out. */
    fun onAddPhoto(source: PhotoSource) {
        val state = form.value
        val id = itemId
        if (id == null || !state.canChangePhotos) return
        if (state.photos.size >= MAX_PHOTOS) {
            form.update { it.copy(photoMessage = CatalogMessage.PhotoLimit) }
            return
        }
        changingPhotos {
            val picked = pick(source) ?: return@changingPhotos
            val photo =
                try {
                    repositories.photos.uploadPhoto(PhotoOwner(PhotoOwnerKind.Item, id), picked)
                } catch (_: Failure.PhotoInvalid) {
                    // The picker refuses a file over 10 MB too, but the API's own check decides the type.
                    val message =
                        if (picked.isOverLimit()) CatalogMessage.PhotoTooBig else CatalogMessage.PhotoWrongType
                    form.update { it.copy(photoMessage = message) }
                    return@changingPhotos
                }
            form.update { it.copy(photos = (it.photos + photo).sortedBy(Photo::position)) }
        }
    }

    /** "Usar como capa": [photo] goes first. */
    fun onMakeCover(photo: Photo) {
        val state = form.value
        val id = itemId
        if (id == null || !state.canChangePhotos || state.photos.firstOrNull()?.id == photo.id) return
        changingPhotos {
            val order = listOf(photo.id) + state.photos.map(Photo::id).filter { it != photo.id }
            val ordered = repositories.photos.orderPhotos(PhotoOwner(PhotoOwnerKind.Item, id), order)
            form.update { it.copy(photos = ordered) }
        }
    }

    /** "Remover foto". */
    fun onRemovePhoto(photo: Photo) {
        val id = itemId
        if (id == null || !form.value.canChangePhotos) return
        changingPhotos {
            repositories.photos.deletePhoto(PhotoOwner(PhotoOwnerKind.Item, id), photo.id)
            form.update { it.copy(photos = it.photos.filter { kept -> kept.id != photo.id }) }
        }
    }

    /** "Novo fornecedor" was chosen for [slot]: opens its form. */
    fun onOpenNewSupplier(slot: SupplierSlot) {
        if (form.value.editable) form.update { it.copy(newSupplier = NewSupplierForm(slot)) }
    }

    /** The "Nome do fornecedor" changed; its mark goes. */
    fun onNewSupplierName(name: String) = editNewSupplier { it.copy(name = name, nameIssue = null) }

    /** The "WhatsApp do fornecedor" changed; its mark goes. */
    fun onNewSupplierPhone(phone: String) = editNewSupplier { it.copy(phone = phone, phoneIssue = null) }

    /** "Cancelar" in "Novo fornecedor". */
    fun onCloseNewSupplier() = form.update { it.copy(newSupplier = null) }

    /** "Adicionar fornecedor": creates it and picks it for the slot that asked. */
    fun onAddSupplier() {
        val dialog = form.value.newSupplier?.takeUnless { it.saving } ?: return
        when (val read = dialog.read()) {
            is SupplierRead.Marked -> form.update { it.copy(newSupplier = read.form) }
            is SupplierRead.Ready -> createSupplier(dialog, read)
        }
    }

    private fun createSupplier(
        dialog: NewSupplierForm,
        read: SupplierRead.Ready,
    ) {
        form.update { it.copy(newSupplier = dialog.copy(saving = true, message = null)) }
        viewModelScope.launch {
            try {
                val created = repositories.suppliers.create(read.draft)
                form.update { state ->
                    val suppliers = (state.suppliers + created).sortedBy { it.name.lowercase() }
                    state.copy(
                        suppliers = suppliers,
                        fields = state.fields.withSupplier(dialog.slot, created.id),
                        issues = state.issues - ItemField.Supplier(dialog.slot),
                        newSupplier = null,
                    )
                }
            } catch (failure: Failure) {
                form.update { it.copy(newSupplier = it.newSupplier?.refusedBy(failure)) }
            }
        }
    }

    private fun editNewSupplier(change: (NewSupplierForm) -> NewSupplierForm) =
        form.update { state -> state.newSupplier?.let { state.copy(newSupplier = change(it)) } ?: state }

    private fun persist(draft: ItemDraft) {
        form.update { it.copy(saving = true, message = null) }
        viewModelScope.launch {
            try {
                if (itemId == null) {
                    repositories.items.createItem(draft)
                } else {
                    repositories.items.replaceItem(itemId, requireVersion(), draft)
                }
                changes.publish(CatalogChange.ItemSaved)
                effectChannel.send(ItemFormEffect.Close)
            } catch (failure: Failure) {
                form.update { it.refusedBy(failure) }
            } finally {
                form.update { it.copy(saving = false) }
            }
        }
    }

    /** Archives or restores the item and leaves the form, saying what happened to [Item.name]. */
    private fun leaveCatalogue(
        change: suspend (Item) -> Item,
        notice: (String) -> CatalogChange,
    ) {
        val state = form.value
        val item = state.item
        if (!state.editable || state.saving || item == null) return
        form.update { it.copy(saving = true, message = null) }
        viewModelScope.launch {
            try {
                changes.publish(notice(change(item).name))
                effectChannel.send(ItemFormEffect.Close)
            } catch (failure: Failure) {
                form.update { it.copy(message = failure.toCatalogMessage()) }
            } finally {
                form.update { it.copy(saving = false) }
            }
        }
    }

    private fun changingPhotos(change: suspend () -> Unit) {
        form.update { it.copy(photoBusy = true, photoMessage = null) }
        viewModelScope.launch {
            try {
                change()
            } catch (failure: Failure) {
                form.update { it.copy(photoMessage = failure.toCatalogMessage()) }
            } finally {
                form.update { it.copy(photoBusy = false) }
            }
        }
    }

    /** Asks the picker; a file over 10 MB is refused by the picker itself, and says so under "Fotos". */
    private suspend fun pick(source: PhotoSource): PickedPhoto? =
        try {
            when (source) {
                PhotoSource.Gallery -> picker.pickFromGallery()
                PhotoSource.Camera -> picker.takePhoto()
            }
        } catch (_: Failure.PhotoInvalid) {
            form.update { it.copy(photoMessage = CatalogMessage.PhotoTooBig) }
            null
        }

    private fun requireVersion(): Int = requireNotNull(form.value.version) { "an item is saved against its version" }

    private fun load() {
        form.update {
            ItemFormUiState(itemId = itemId, canWrite = it.canWrite, newSupplier = null)
        }
        viewModelScope.launch {
            try {
                // Nothing edits the form while it loads, so reading from the state as it was is safe.
                form.value = readEverything(form.value)
            } catch (failure: Failure) {
                form.update { it.copy(loadMessage = failure.toCatalogMessage()) }
            } finally {
                form.update { it.copy(loading = false) }
            }
        }
    }

    /** Reads the item and what its pickers offer; a role that cannot write needs no suppliers or settings. */
    private suspend fun readEverything(state: ItemFormUiState): ItemFormUiState {
        val session = observeSession().first()
        val canWrite = session?.permissions?.allows(Permission.CatalogWrite) == true
        val fetched = itemId?.let { repositories.items.item(it) }
        val item = fetched?.value
        val settings = if (canWrite) repositories.settings.settings() else null
        return state.copy(
            canWrite = canWrite,
            item = item,
            offlineSince = fetched?.cachedAtEpochMillis,
            fields = item?.toFields() ?: ItemFields(),
            categories = repositories.categories.categories().value,
            attributes = repositories.attributes.attributes().value,
            suppliers = if (canWrite) allSuppliers() else emptyList(),
            modes = modesOf(settings, item?.sourcingMode),
            showsCosts =
                item?.let { it.cost.isShown || it.margin.isShown || it.productionCost.isShown }
                    ?: (session?.role?.seesCosts() == true),
            archived = item?.archived == true,
            version = item?.version,
            photos = item?.photos.orEmpty(),
        )
    }

    private suspend fun allSuppliers(): List<Supplier> {
        val all = mutableListOf<Supplier>()
        var cursor: String? = null
        do {
            val page = repositories.suppliers.page(query = null, cursor = cursor, limit = SUPPLIER_PAGE).value
            all += page.items
            cursor = page.nextCursor
        } while (cursor != null)
        return all
    }

    private companion object {
        const val MAX_PHOTOS = 8
        const val SUPPLIER_PAGE = 100
    }
}

private val ItemField.isOption: Boolean
    get() =
        this is ItemField.OptionName ||
            this is ItemField.OptionChoices ||
            this is ItemField.ChoiceLabel ||
            this is ItemField.ChoicePrice ||
            this is ItemField.ChoiceDays

/** The "Origem" choices: always Sob encomenda, the other two when the store turned them on or the item has it. */
private fun modesOf(
    settings: StoreSettings?,
    current: SourcingMode?,
): List<SourcingMode> =
    SourcingMode.entries.filter { mode ->
        when (mode) {
            SourcingMode.ToOrder -> true
            SourcingMode.Stock -> settings?.stockEnabled == true || current == mode
            SourcingMode.InHouse -> settings?.productionEnabled == true || current == mode
        }
    }

/** What a refused "Salvar" says: the server's field problems next to their fields, or the failure above the form. */
private fun ItemFormUiState.refusedBy(failure: Failure): ItemFormUiState {
    if (failure !is Failure.InvalidInput) return copy(message = failure.toCatalogMessage())
    val placed = placeServerIssues(failure.issues, fields)
    val unplaced = placed.unplaced.firstOrNull()
    val message =
        unplaced?.let { CatalogMessage.General(FailureMessage.Refused(it.message)) } ?: CatalogMessage.CheckFields
    return copy(issues = placed.placed, message = message)
}

/** What "Novo fornecedor" says about a refusal: the server's words next to its fields, or one above them. */
private fun NewSupplierForm.refusedBy(failure: Failure): NewSupplierForm {
    val cleared = copy(saving = false, message = null)
    return when {
        failure is Failure.AlreadyTaken && failure.field == "name" ->
            cleared.copy(
                nameIssue = SupplierProblem.NameTaken,
            )
        failure is Failure.InvalidInput -> {
            val name =
                failure.issues
                    .firstOrNull { it.field == "name" }
                    ?.message
                    ?.let(SupplierProblem::Server)
            val phone =
                failure.issues.firstOrNull { it.field == "whatsappPhone" }?.message?.let(
                    SupplierProblem::Server,
                )
            if (name == null && phone == null) {
                cleared.copy(message = failure.toCatalogMessage())
            } else {
                cleared.copy(nameIssue = name, phoneIssue = phone)
            }
        }
        else -> cleared.copy(message = failure.toCatalogMessage())
    }
}

private const val MAX_PHOTO_BYTES = 10 * 1024 * 1024

/** True when the file is over the API's 10 MB limit. */
private fun PickedPhoto.isOverLimit(): Boolean = bytes.size > MAX_PHOTO_BYTES
