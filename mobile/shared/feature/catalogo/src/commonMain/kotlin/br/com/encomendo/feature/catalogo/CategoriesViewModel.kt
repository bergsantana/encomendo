package br.com.encomendo.feature.catalogo

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.catalog.Category
import br.com.encomendo.core.domain.catalog.ChangeCategories
import br.com.encomendo.core.domain.catalog.ListCategories
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Permission
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** NameProblem is what is wrong with a name typed for a category or a field. */
enum class NameProblem {
    /** "Informe o nome da categoria." or "Informe o nome do campo." */
    Missing,

    /** "Já existe uma categoria com este nome." or "Já existe um campo com este nome." */
    Taken,
}

/** CategoryEdit is the rename under way on one row. */
data class CategoryEdit(
    val categoryId: String,
    /** The name the category has now, which the field is labelled with: "Novo nome de {categoria}". */
    val originalName: String,
    val name: String,
    val problem: NameProblem? = null,
)

/**
 * CategoriesUiState is "Categorias": the list in the store's order, the "Nova categoria" field and the rename
 * under way. A category that items use cannot be deleted; its row says so.
 */
data class CategoriesUiState(
    val categories: List<Category> = emptyList(),
    val loading: Boolean = true,
    val offlineSince: Long? = null,
    val canWrite: Boolean = false,
    /** "Nome da categoria" for a new category. */
    val newName: String = "",
    val newProblem: NameProblem? = null,
    val edit: CategoryEdit? = null,
    /** The row being changed; every button waits meanwhile. */
    val savingId: String? = null,
    /** Set while "Nova categoria" is being saved. */
    val adding: Boolean = false,
    /** The category a refused deletion was about; its row says why. */
    val inUseId: String? = null,
    /** Why the last read or change failed, at the top. */
    val message: CatalogMessage? = null,
) {
    /** One change at a time. */
    val busy: Boolean get() = savingId != null || adding
}

/**
 * CategoriesViewModel backs "Categorias" with the web's behavior: add, rename, move and delete, each at once. A
 * delete the server refuses because items use the category says so on its row (docs/product/design-document.md).
 */
@Suppress("TooManyFunctions") // One function per thing the user can do in the dialog, as the web's store.
class CategoriesViewModel(
    observeSession: ObserveSession,
    private val listCategories: ListCategories,
    private val changeCategories: ChangeCategories,
    private val changes: CatalogChanges,
) : ViewModel() {
    private val state = MutableStateFlow(CategoriesUiState())

    /** The screen as it shows now. */
    val uiState: StateFlow<CategoriesUiState> = state.asStateFlow()

    init {
        viewModelScope.launch {
            observeSession().collect { session ->
                val canWrite = session?.permissions?.allows(Permission.CatalogWrite) == true
                state.update { it.copy(canWrite = canWrite) }
            }
        }
        read()
    }

    /** "Tentar de novo". */
    fun onRetry() = read()

    /** The "Nome da categoria" of the new category changed. */
    fun onNewNameChange(name: String) = state.update { it.copy(newName = name, newProblem = null) }

    /** "Nova categoria". An empty name is marked, never sent. */
    fun onAdd() {
        val now = state.value
        if (!now.canWrite || now.busy) return
        val name = now.newName.trim()
        if (name.isEmpty()) {
            state.update { it.copy(newProblem = NameProblem.Missing) }
            return
        }
        state.update { it.copy(adding = true, message = null) }
        viewModelScope.launch {
            try {
                changeCategories.create(name)
                state.update { it.copy(newName = "") }
                changed()
            } catch (failure: Failure) {
                state.update {
                    if (failure is Failure.AlreadyTaken) {
                        it.copy(newProblem = NameProblem.Taken)
                    } else {
                        it.copy(message = failure.toCatalogMessage())
                    }
                }
            } finally {
                state.update { it.copy(adding = false) }
            }
        }
    }

    /** "Renomear": opens the row's name for editing. */
    fun onStartRename(category: Category) {
        if (state.value.canWrite && !state.value.busy) {
            state.update { it.copy(edit = CategoryEdit(category.id, category.name, category.name), inUseId = null) }
        }
    }

    /** The name being edited changed. */
    fun onRenameChange(name: String) = state.update { s -> s.copy(edit = s.edit?.copy(name = name, problem = null)) }

    /** "Cancelar" a rename. */
    fun onCancelRename() = state.update { it.copy(edit = null) }

    /** "Salvar" a rename. */
    fun onSaveRename() {
        val now = state.value
        val edit = now.edit?.takeUnless { now.busy } ?: return
        val name = edit.name.trim()
        if (name.isEmpty()) {
            state.update { it.copy(edit = edit.copy(problem = NameProblem.Missing)) }
            return
        }
        change(edit.categoryId, onTaken = { state.update { it.copy(edit = edit.copy(problem = NameProblem.Taken)) } }) {
            changeCategories.rename(edit.categoryId, name)
            state.update { it.copy(edit = null) }
        }
    }

    /** "Mover para cima" or "Mover para baixo": [category] goes one place [up] or down. */
    fun onMove(
        category: Category,
        up: Boolean,
    ) {
        val now = state.value
        val index = now.categories.indexOfFirst { it.id == category.id }
        val target = if (up) index - 1 else index + 1
        if (!now.canWrite || now.busy || target !in now.categories.indices) return
        change(category.id) { changeCategories.move(category.id, target) }
    }

    /** "Excluir". Refused while items use the category. */
    fun onDelete(category: Category) {
        if (!state.value.canWrite || state.value.busy) return
        change(category.id) { changeCategories.delete(category.id) }
    }

    /**
     * Runs one change to the row [id]. Whatever went through is read again and published for the Itens list;
     * [onTaken] says what a name already in use looks like, and a category in use marks its row.
     */
    private fun change(
        id: String,
        onTaken: () -> Unit = {},
        work: suspend () -> Unit,
    ) {
        state.update { it.copy(savingId = id, message = null, inUseId = null) }
        viewModelScope.launch {
            try {
                work()
                changed()
            } catch (failure: Failure) {
                when (failure) {
                    is Failure.AlreadyTaken -> onTaken()
                    is Failure.CategoryInUse -> state.update { it.copy(inUseId = id) }
                    else -> state.update { it.copy(message = failure.toCatalogMessage()) }
                }
            } finally {
                state.update { it.copy(savingId = null) }
            }
        }
    }

    private suspend fun changed() {
        changes.publish(CatalogChange.Taxonomy)
        readNow()
    }

    private fun read() {
        state.update { it.copy(loading = true, message = null) }
        viewModelScope.launch {
            try {
                readNow()
            } finally {
                state.update { it.copy(loading = false) }
            }
        }
    }

    private suspend fun readNow() {
        try {
            val fetched = listCategories()
            state.update { it.copy(categories = fetched.value, offlineSince = fetched.cachedAtEpochMillis) }
        } catch (failure: Failure) {
            state.update { it.copy(message = failure.toCatalogMessage()) }
        }
    }
}
