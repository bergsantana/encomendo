package br.com.encomendo.feature.catalogo

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.catalog.Attribute
import br.com.encomendo.core.domain.catalog.AttributeDraft
import br.com.encomendo.core.domain.catalog.AttributeValueDraft
import br.com.encomendo.core.domain.catalog.ChangeAttributes
import br.com.encomendo.core.domain.catalog.ListAttributes
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Permission
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * AttributesViewModel backs "Campos": fields such as "Serve em" and their values. As on the web, a field is edited
 * as a draft, its name and values typed and added and removed, and nothing is saved until "Salvar campo". It asks
 * first before taking away a value, or a whole field, that items or kits use (docs/product/design-document.md,
 * "Campos"), using the counts the API sent.
 */
@Suppress("TooManyFunctions") // One function per thing the user can do in the dialog, as the web's store.
class AttributesViewModel(
    observeSession: ObserveSession,
    private val listAttributes: ListAttributes,
    private val changeAttributes: ChangeAttributes,
    private val changes: CatalogChanges,
) : ViewModel() {
    private val state = MutableStateFlow(AttributesUiState())

    /** The screen as it shows now. */
    val uiState: StateFlow<AttributesUiState> = state.asStateFlow()

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

    /** "Novo campo": a blank field, to be named and saved with "Salvar campo". */
    fun onNewField() {
        if (!state.value.canWrite) return
        state.update { it.copy(editors = it.editors + AttributeEditor(key = it.nextKey(), id = null, name = "")) }
    }

    /** The "Nome do campo" of [editor] changed. */
    fun onNameChange(
        editor: AttributeEditor,
        name: String,
    ) = editing(editor) { it.copy(name = name, nameProblem = null) }

    /** The text of one value of [editor] changed. */
    fun onValueChange(
        editor: AttributeEditor,
        value: ValueEditor,
        label: String,
    ) = editing(editor) { it.withValue(value.key) { v -> v.copy(label = label, problem = null) } }

    /** "Adicionar valor": a blank value at the end of [editor]. */
    fun onAddValue(editor: AttributeEditor) =
        editing(editor) { it.copy(values = it.values + ValueEditor(key = it.nextValueKey(), id = null, label = "")) }

    /** "Remover valor": takes [value] off [editor]'s draft, asking first while items or kits use it. */
    fun onRemoveValue(
        editor: AttributeEditor,
        value: ValueEditor,
    ) {
        if (!state.value.canWrite) return
        if (value.usageCount > 0) {
            val question = RemovalQuestion.OfValue(editor.key, value.key, value.label, value.usageCount)
            state.update { it.copy(question = question) }
        } else {
            editing(editor) { it.withoutValue(value.key) }
        }
    }

    /** "Excluir campo": deletes [editor]'s field, asking first while items or kits use one of its values. */
    fun onDeleteField(editor: AttributeEditor) {
        val now = state.value
        if (!now.canWrite || now.busy) return
        // The API counts each value, and adding the counts would count an item twice: the question names no count.
        if (editor.values.any { it.usageCount > 0 }) {
            state.update { it.copy(question = RemovalQuestion.OfField(editor.key, editor.name)) }
        } else {
            deleteField(editor)
        }
    }

    /** "Cancelar" on the question. */
    fun onCancelQuestion() = state.update { it.copy(question = null) }

    /** "Remover" on the question. */
    fun onConfirmQuestion() {
        val now = state.value
        val question = now.question?.takeUnless { now.busy } ?: return
        state.update { it.copy(question = null) }
        val editor = now.editors.firstOrNull { it.key == question.editorKey } ?: return
        when (question) {
            is RemovalQuestion.OfValue -> editing(editor) { it.withoutValue(question.valueKey) }
            is RemovalQuestion.OfField -> deleteField(editor)
        }
    }

    /** "Salvar campo": checks the draft, then creates or replaces the field. Empty and repeated values are marked. */
    fun onSaveField(editor: AttributeEditor) {
        val now = state.value
        if (!now.canWrite || now.busy) return
        val marked = editor.withProblems()
        if (marked != null) {
            replaceEditor(marked)
            return
        }
        val values = editor.values.map { AttributeValueDraft(it.id, it.label.trim()) }
        val draft = AttributeDraft(editor.name.trim(), values)
        state.update { it.copy(savingKey = editor.key, message = null) }
        viewModelScope.launch {
            try {
                val id = editor.id
                val saved = if (id == null) changeAttributes.create(draft) else changeAttributes.replace(id, draft)
                replaceEditor(saved.toEditor(editor.key))
                changes.publish(CatalogChange.Taxonomy)
            } catch (failure: Failure) {
                if (failure is Failure.AlreadyTaken) {
                    replaceEditor(editor.copy(nameProblem = NameProblem.Taken))
                } else {
                    state.update { it.copy(message = failure.toCatalogMessage()) }
                }
            } finally {
                state.update { it.copy(savingKey = null) }
            }
        }
    }

    private fun deleteField(editor: AttributeEditor) {
        val id = editor.id
        if (id == null) {
            state.update { it.copy(editors = it.editors.filter { other -> other.key != editor.key }) }
            return
        }
        state.update { it.copy(savingKey = editor.key, message = null) }
        viewModelScope.launch {
            try {
                changeAttributes.delete(id)
                state.update { it.copy(editors = it.editors.filter { other -> other.key != editor.key }) }
                changes.publish(CatalogChange.Taxonomy)
            } catch (failure: Failure) {
                state.update { it.copy(message = failure.toCatalogMessage()) }
            } finally {
                state.update { it.copy(savingKey = null) }
            }
        }
    }

    private fun editing(
        editor: AttributeEditor,
        change: (AttributeEditor) -> AttributeEditor,
    ) {
        if (!state.value.canWrite) return
        state.update { s -> s.copy(editors = s.editors.map { if (it.key == editor.key) change(it) else it }) }
    }

    private fun replaceEditor(editor: AttributeEditor) =
        state.update { s -> s.copy(editors = s.editors.map { if (it.key == editor.key) editor else it }) }

    private fun read() {
        state.update { it.copy(loading = true, message = null) }
        viewModelScope.launch {
            try {
                val fetched = listAttributes()
                state.update { s ->
                    val editors = fetched.value.mapIndexed { index, attribute -> attribute.toEditor(key = index + 1) }
                    s.copy(editors = editors, offlineSince = fetched.cachedAtEpochMillis)
                }
            } catch (failure: Failure) {
                state.update { it.copy(message = failure.toCatalogMessage()) }
            } finally {
                state.update { it.copy(loading = false) }
            }
        }
    }
}

private fun Attribute.toEditor(key: Int): AttributeEditor =
    AttributeEditor(
        key = key,
        id = id,
        name = name,
        values = values.mapIndexed { index, value -> ValueEditor(index + 1, value.id, value.label, value.usageCount) },
    )
