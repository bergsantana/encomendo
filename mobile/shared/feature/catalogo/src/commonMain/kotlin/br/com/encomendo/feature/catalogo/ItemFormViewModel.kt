package br.com.encomendo.feature.catalogo

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.catalog.ArchiveItem
import br.com.encomendo.core.domain.catalog.ChangePhotos
import br.com.encomendo.core.domain.catalog.Fetched
import br.com.encomendo.core.domain.catalog.GetItem
import br.com.encomendo.core.domain.catalog.Item
import br.com.encomendo.core.domain.catalog.ItemDraft
import br.com.encomendo.core.domain.catalog.ListAttributes
import br.com.encomendo.core.domain.catalog.ListCategories
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.core.domain.catalog.PhotoOwnerKind
import br.com.encomendo.core.domain.catalog.SaveItem
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.catalog.isShown
import br.com.encomendo.core.domain.catalog.seesCosts
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.domain.settings.GetStoreSettings
import br.com.encomendo.core.domain.settings.StoreSettings
import br.com.encomendo.core.domain.supplier.CreateSupplier
import br.com.encomendo.core.domain.supplier.ListSuppliers
import br.com.encomendo.core.domain.supplier.Supplier
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

/** ItemFormUseCases are what the item form reads and writes besides photos and suppliers, taken as one input. */
class ItemFormUseCases(
    val getItem: GetItem,
    val saveItem: SaveItem,
    val archiveItem: ArchiveItem,
    val listCategories: ListCategories,
    val listAttributes: ListAttributes,
    val listSuppliers: ListSuppliers,
    val getSettings: GetStoreSettings,
)

/**
 * ItemFormViewModel backs the item editor, as the web's item drawer (docs/product/design-document.md, "Catálogo",
 * "Item form"): the same fields, the same checks before anything is sent, the server's own words next to the
 * fields it refuses, and a photo change that goes through at once. [itemId] is the item to edit, or null for a new
 * one. Roles without `catalog:write` read the item and change nothing, and so does an editor whose store settings
 * cannot be read offline, since it could not save anyway.
 *
 * The cost fields exist only when the API sends costs to this role, and a cost the owner left alone is not sent,
 * so the server keeps it (ADR 0025). Leaving with something typed asks "Descartar as alterações?" first. The photos
 * and "Novo fornecedor" have their own controllers over parts of this state.
 */
@Suppress("TooManyFunctions") // One function per thing the user can do on the form, as the web's item store.
class ItemFormViewModel(
    private val itemId: String?,
    private val observeSession: ObserveSession,
    private val useCases: ItemFormUseCases,
    changePhotos: ChangePhotos,
    createSupplier: CreateSupplier,
    picker: PhotoPicker,
    private val changes: CatalogChanges,
) : ViewModel() {
    private val form = MutableStateFlow(ItemFormUiState(itemId = itemId))
    private val effectChannel = Channel<ItemFormEffect>(Channel.BUFFERED)

    private val photoController =
        PhotoController(
            scope = viewModelScope,
            kind = PhotoOwnerKind.Item,
            useCases = changePhotos,
            picker = picker,
            slot = StateSlot({ form.value.photos }) { change -> form.update { it.copy(photos = change(it.photos)) } },
        )

    private val supplierController =
        NewSupplierController(
            scope = viewModelScope,
            createSupplier = createSupplier,
            form =
                StateSlot({
                    form.value.newSupplier
                }) { change -> form.update { it.copy(newSupplier = change(it.newSupplier)) } },
            onCreated = ::supplierCreated,
        )

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
        form.update { state ->
            val issues = if (field == null) state.issues else state.issues - field
            state.copy(fields = change(state.fields), issues = issues)
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

    /** "Cancelar" or the system Back: leaves, asking first when something typed is not saved. */
    fun onCancel() = askBeforeLeaving(DiscardAction.Close)

    /** "Arquivar". Refused while a kit holds the item; asks first when something typed is not saved. */
    fun onArchive() = askBeforeLeaving(DiscardAction.Archive)

    /** "Restaurar"; asks first when something typed is not saved. */
    fun onRestore() = askBeforeLeaving(DiscardAction.Restore)

    /** "Continuar editando". */
    fun onKeepEditing() = form.update { it.copy(discard = null) }

    /** "Descartar": goes on with what asked first, without saving what was typed. */
    fun onDiscard() {
        val action = form.value.discard ?: return
        form.update { it.copy(discard = null) }
        perform(action)
    }

    /** "Adicionar fotos" with [source]. Nothing happens when the user backs out. */
    fun onAddPhoto(source: PhotoSource) = photoController.add(source)

    /** "Usar como capa": [photo] goes first. */
    fun onMakeCover(photo: Photo) = photoController.makeCover(photo)

    /** "Remover foto". */
    fun onRemovePhoto(photo: Photo) = photoController.remove(photo)

    /** "Novo fornecedor" was chosen for [slot]: opens its form. */
    fun onOpenNewSupplier(slot: SupplierSlot) {
        if (form.value.editable) supplierController.open(slot)
    }

    /** The "Nome do fornecedor" changed; its mark goes. */
    fun onNewSupplierName(name: String) = supplierController.onName(name)

    /** The "WhatsApp do fornecedor" changed; its mark goes. */
    fun onNewSupplierPhone(phone: String) = supplierController.onPhone(phone)

    /** "Cancelar" in "Novo fornecedor"; ignored while it saves. */
    fun onCloseNewSupplier() = supplierController.close()

    /** "Adicionar fornecedor": creates it and picks it for the slot that asked. */
    fun onAddSupplier() = supplierController.add()

    private fun supplierCreated(
        slot: SupplierSlot,
        created: Supplier,
    ) = form.update { state ->
        state.copy(
            suppliers = (state.suppliers + created).sortedBy { it.name.lowercase() },
            fields = state.fields.withSupplier(slot, created.id),
            issues = state.issues - ItemField.Supplier(slot),
        )
    }

    private fun askBeforeLeaving(action: DiscardAction) {
        val state = form.value
        if (state.saving) return
        if (state.editable && state.dirty) form.update { it.copy(discard = action) } else perform(action)
    }

    private fun perform(action: DiscardAction) {
        when (action) {
            DiscardAction.Close -> effectChannel.trySend(ItemFormEffect.Close)
            DiscardAction.Archive -> archive()
            DiscardAction.Restore -> restore()
        }
    }

    private fun archive() =
        leaveCatalogue(CatalogChange::ItemArchived) { useCases.archiveItem(it.id, requireVersion()) }

    private fun restore() =
        leaveCatalogue(CatalogChange::ItemRestored) { useCases.archiveItem.restore(it.id, requireVersion()) }

    private fun persist(draft: ItemDraft) {
        form.update { it.copy(saving = true, message = null) }
        viewModelScope.launch {
            try {
                if (itemId == null) {
                    useCases.saveItem.create(draft)
                } else {
                    useCases.saveItem.replace(itemId, requireVersion(), draft)
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
        notice: (String) -> CatalogChange,
        change: suspend (Item) -> Item,
    ) {
        val state = form.value
        val item = state.item
        if (!state.editable || state.saving || item == null) return
        form.update { it.copy(saving = true, message = null, discard = null) }
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

    private fun requireVersion(): Int = requireNotNull(form.value.version) { "an item is saved against its version" }

    private fun load() {
        form.update { ItemFormUiState(itemId = itemId) }
        viewModelScope.launch {
            try {
                // Nothing edits the form while it loads, so reading from the state as it was is safe.
                form.value = readEverything(form.value)
                form.value.let { photoController.open(it.item?.id, it.item?.photos.orEmpty(), enabled = it.canWrite) }
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
        val fetched = itemId?.let { useCases.getItem(it) }
        val item = fetched?.value
        val mayWrite = session?.permissions?.allows(Permission.CatalogWrite) == true
        val settings = if (mayWrite) settingsOrNull(fetched) else null
        val canWrite = settings != null
        val fields = item?.toFields() ?: ItemFields()
        return state.copy(
            canWrite = canWrite,
            item = item,
            offlineSince = fetched?.cachedAtEpochMillis,
            fields = fields,
            initialFields = fields,
            categories = useCases.listCategories().value,
            attributes = useCases.listAttributes().value,
            suppliers = if (canWrite) allSuppliers() else emptyList(),
            modes = modesOf(settings, item?.sourcingMode),
            showsCosts =
                item?.let { it.cost.isShown || it.margin.isShown || it.productionCost.isShown }
                    ?: (session?.role?.seesCosts() == true),
            archived = item?.archived == true,
            version = item?.version,
        )
    }

    /**
     * Reads the store's settings for an editor. Offline with the item itself read from the device's copy, the editor
     * could not save anyway, so the item opens read-only (null) with its offline banner instead of failing.
     */
    private suspend fun settingsOrNull(item: Fetched<Item>?): StoreSettings? =
        try {
            useCases.getSettings()
        } catch (failure: Failure.Offline) {
            if (item?.fromCache == true) null else throw failure
        }

    private suspend fun allSuppliers(): List<Supplier> {
        val all = mutableListOf<Supplier>()
        var cursor: String? = null
        do {
            val page = useCases.listSuppliers(cursor = cursor, limit = SUPPLIER_PAGE).value
            all += page.items
            cursor = page.nextCursor
        } while (cursor != null)
        return all
    }

    private companion object {
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
