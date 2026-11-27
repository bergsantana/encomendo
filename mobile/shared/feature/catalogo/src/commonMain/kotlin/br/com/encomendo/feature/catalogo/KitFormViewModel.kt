package br.com.encomendo.feature.catalogo

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.catalog.ArchiveKit
import br.com.encomendo.core.domain.catalog.ChangePhotos
import br.com.encomendo.core.domain.catalog.Fetched
import br.com.encomendo.core.domain.catalog.GetKit
import br.com.encomendo.core.domain.catalog.ItemFilter
import br.com.encomendo.core.domain.catalog.ItemSummary
import br.com.encomendo.core.domain.catalog.Kit
import br.com.encomendo.core.domain.catalog.KitDraft
import br.com.encomendo.core.domain.catalog.ListAttributes
import br.com.encomendo.core.domain.catalog.ListCategories
import br.com.encomendo.core.domain.catalog.ListItems
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.core.domain.catalog.PhotoOwnerKind
import br.com.encomendo.core.domain.catalog.SaveKit
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.niche.NicheWords
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.domain.settings.GetStoreSettings
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

/** KitFormUseCases are what the kit editor reads and writes besides photos, taken as one input. */
class KitFormUseCases(
    val getKit: GetKit,
    val saveKit: SaveKit,
    val archiveKit: ArchiveKit,
    val listItems: ListItems,
    val listCategories: ListCategories,
    val listAttributes: ListAttributes,
    val getSettings: GetStoreSettings,
)

/**
 * KitFormViewModel backs the kit editor, as the web's kit drawer (docs/product/design-document.md, "Catálogo",
 * "Kits"): the same fields, the same checks before anything is sent, the server's own words next to the fields it
 * refuses, and a photo change that goes through at once. [kitId] is the kit to edit, or null for a new one. Roles
 * without `catalog:write` read the kit and change nothing, and so does an editor whose store settings or items
 * cannot be read offline, since it could not save anyway.
 *
 * Totals, discount, margin and days are the API's (ADR 0025): the state keeps the kit as the API last returned it
 * and never works any of them out. After "Salvar" the editor stays open on the saved kit, as on the web, so the new
 * totals show and a new kit can take photos. Leaving with something typed asks "Descartar as alterações?" first.
 */
@Suppress("TooManyFunctions") // One function per thing the user can do on the editor, as the web's kit store.
class KitFormViewModel(
    private val kitId: String?,
    private val observeSession: ObserveSession,
    private val useCases: KitFormUseCases,
    changePhotos: ChangePhotos,
    picker: PhotoPicker,
    private val changes: CatalogChanges,
    niche: NicheWords,
) : ViewModel() {
    private val form = MutableStateFlow(KitFormUiState(kitId = kitId, niche = niche))
    private val effectChannel = Channel<KitFormEffect>(Channel.BUFFERED)

    private val photoController =
        PhotoController(
            scope = viewModelScope,
            kind = PhotoOwnerKind.Kit,
            useCases = changePhotos,
            picker = picker,
            slot = StateSlot({ form.value.photos }) { change -> form.update { it.copy(photos = change(it.photos)) } },
        )

    /** The editor as it shows now. */
    val uiState: StateFlow<KitFormUiState> = form.asStateFlow()

    /** What the editor asks its screen to do, once each. */
    val effects: Flow<KitFormEffect> = effectChannel.receiveAsFlow()

    init {
        load()
    }

    /** "Tentar de novo" and "Recarregar": reads the kit and the choices again, dropping what was typed. */
    fun onReload() = load()

    /** The user typed or picked in [field]; its mark goes. Ignored for roles that cannot write. */
    fun onEdit(
        field: KitField?,
        change: (KitFields) -> KitFields,
    ) {
        if (!form.value.canEdit) return
        form.update { state ->
            val issues = if (field == null) state.issues else state.issues - field
            state.copy(fields = change(state.fields), issues = issues)
        }
    }

    /** "Item a adicionar" was picked; null is back at "Escolha o item". */
    fun onPickItemToAdd(itemId: String?) {
        if (form.value.canEdit) form.update { it.copy(itemToAdd = itemId) }
    }

    /** "Adicionar item": the picked item joins the kit with a quantity of 1. */
    fun onAddItem() {
        val state = form.value
        val item = state.addable.firstOrNull { it.id == state.itemToAdd }
        if (!state.canEdit || item == null) return
        form.update {
            it.copy(
                fields = it.fields.copy(items = it.fields.items + KitItemFields(item.id, item.name)),
                issues = it.issues - KitField.Items,
                itemToAdd = null,
            )
        }
    }

    /** "Remover do kit": the row goes, and so do the problems marked on it. */
    fun onRemoveItem(itemId: String) {
        if (!form.value.canEdit) return
        form.update { state ->
            state.copy(
                fields = state.fields.withoutRow(itemId),
                issues = state.issues - KitField.Items - KitField.Quantity(itemId) - KitField.Alternatives(itemId),
            )
        }
    }

    /** "Salvar". An empty or wrong field is marked, never sent. */
    fun onSave() {
        val state = form.value
        if (!state.editable || state.saving) return
        when (val read = state.fields.toDraft()) {
            is KitDraftResult.Problems ->
                form.update { it.copy(issues = read.issues, message = CatalogMessage.CheckFields, saved = false) }
            is KitDraftResult.Ready -> persist(state, read.draft)
        }
    }

    /** "Cancelar" or the system Back: leaves, asking first when something typed is not saved. */
    fun onCancel() = askBeforeLeaving(DiscardAction.Close)

    /** "Arquivar"; asks first when something typed is not saved. */
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

    private fun askBeforeLeaving(action: DiscardAction) {
        val state = form.value
        if (state.saving) return
        if (state.editable && state.dirty) form.update { it.copy(discard = action) } else perform(action)
    }

    private fun perform(action: DiscardAction) {
        when (action) {
            DiscardAction.Close -> effectChannel.trySend(KitFormEffect.Close)
            DiscardAction.Archive -> archive()
            DiscardAction.Restore -> restore()
        }
    }

    private fun archive() = leaveCatalogue(CatalogChange::KitArchived) { useCases.archiveKit(it.id, it.version) }

    private fun restore() =
        leaveCatalogue(CatalogChange::KitRestored) { useCases.archiveKit.restore(it.id, it.version) }

    private fun persist(
        sent: KitFormUiState,
        draft: KitDraft,
    ) {
        form.update { it.copy(saving = true, message = null, saved = false) }
        viewModelScope.launch {
            try {
                val kit = sent.kit
                val save = useCases.saveKit
                val saved = if (kit == null) save.create(draft) else save.replace(kit.id, kit.version, draft)
                showSaved(saved)
                changes.publish(CatalogChange.KitSaved)
            } catch (failure: Failure) {
                form.update { it.refusedBy(failure, sent.fields) }
            } finally {
                form.update { it.copy(saving = false) }
            }
        }
    }

    /** The editor stays open on [kit], as the API returned it, with its new totals and its photos. */
    private fun showSaved(kit: Kit) {
        val fields = kit.toFields()
        form.update { it.copy(kit = kit, fields = fields, initialFields = fields, issues = emptyMap(), saved = true) }
        photoController.open(kit.id, kit.photos, enabled = form.value.canWrite)
    }

    /** Archives or restores the kit and leaves the editor, saying what happened to [Kit.name]. */
    private fun leaveCatalogue(
        notice: (String) -> CatalogChange,
        change: suspend (Kit) -> Kit,
    ) {
        val state = form.value
        val kit = state.kit
        if (!state.editable || state.saving || kit == null) return
        form.update { it.copy(saving = true, message = null, saved = false, discard = null) }
        viewModelScope.launch {
            try {
                changes.publish(notice(change(kit).name))
                effectChannel.send(KitFormEffect.Close)
            } catch (failure: Failure) {
                form.update { it.copy(message = failure.toCatalogMessage(CatalogSubject.Kit)) }
            } finally {
                form.update { it.copy(saving = false) }
            }
        }
    }

    private fun load() {
        // A kit created in this session has an id of its own now: "Recarregar" reads that one, not a new kit.
        val id = form.value.kit?.id ?: kitId
        form.update { KitFormUiState(kitId = id, niche = it.niche) }
        viewModelScope.launch {
            try {
                // Nothing edits the form while it loads, so reading from the state as it was is safe.
                form.value = readEverything(form.value, id)
                form.value.let { photoController.open(it.kit?.id, it.kit?.photos.orEmpty(), enabled = it.canWrite) }
            } catch (failure: Failure) {
                form.update { it.copy(loadMessage = failure.toCatalogMessage(CatalogSubject.Kit)) }
            } finally {
                form.update { it.copy(loading = false) }
            }
        }
    }

    /** Reads the kit and what its pickers offer; offline from the device's copy, an editor opens read-only. */
    private suspend fun readEverything(
        state: KitFormUiState,
        id: String?,
    ): KitFormUiState {
        val session = observeSession().first()
        val fetched = id?.let { useCases.getKit(it) }
        val kit = fetched?.value
        val mayWrite = session?.permissions?.allows(Permission.CatalogWrite) == true
        // Every role reads the settings: the read-only view says "Padrão da loja" for days the kit leaves blank.
        val settings = cachedKitOrThrow(fetched) { useCases.getSettings() }
        val items = cachedKitOrThrow(fetched) { allItems() }
        val fields = kit?.toFields() ?: KitFields()
        return state.copy(
            canWrite = mayWrite && settings != null && items != null,
            kit = kit,
            offlineSince = fetched?.cachedAtEpochMillis,
            fields = fields,
            initialFields = fields,
            catalogItems = items.orEmpty(),
            categories = useCases.listCategories().value,
            attributes = useCases.listAttributes().value,
            defaultAssemblyDays = settings?.assemblyDays,
            defaultSafetyMarginDays = settings?.safetyMarginDays,
        )
    }

    /**
     * Runs [read]. Offline with the kit itself read from the device's copy, the editor could not save anyway, so the
     * kit opens read-only (null) with its offline banner instead of failing.
     */
    private suspend fun <T> cachedKitOrThrow(
        kit: Fetched<Kit>?,
        read: suspend () -> T,
    ): T? =
        try {
            read()
        } catch (failure: Failure.Offline) {
            if (kit?.fromCache == true) null else throw failure
        }

    private suspend fun allItems(): List<ItemSummary> {
        val all = mutableListOf<ItemSummary>()
        var cursor: String? = null
        do {
            val page = useCases.listItems(ItemFilter(includeArchived = true), cursor, ITEM_PAGE).value
            all += page.items
            cursor = page.nextCursor
        } while (cursor != null)
        return all
    }

    private companion object {
        const val ITEM_PAGE = 100
    }
}

/** What a refused "Salvar" says: the server's field problems next to their fields, or the failure above the form. */
private fun KitFormUiState.refusedBy(
    failure: Failure,
    sent: KitFields,
): KitFormUiState {
    if (failure !is Failure.InvalidInput) return copy(message = failure.toCatalogMessage(CatalogSubject.Kit))
    val placed = placeKitIssues(failure.issues, sent)
    val unplaced = placed.unplaced.firstOrNull()
    val message =
        unplaced?.let { CatalogMessage.General(FailureMessage.Refused(it.message)) } ?: CatalogMessage.CheckFields
    return copy(issues = placed.placed, message = message)
}
