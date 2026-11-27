package br.com.encomendo.feature.catalogo

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.catalog.Attribute
import br.com.encomendo.core.domain.catalog.AttributeDraft
import br.com.encomendo.core.domain.catalog.AttributeRepository
import br.com.encomendo.core.domain.catalog.AttributeValue
import br.com.encomendo.core.domain.catalog.AttributeValueDraft
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Permission
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** ValueProblem is what is wrong with a value typed under "Valores". */
enum class ValueProblem {
    /** "Informe o valor." */
    Missing,

    /** "Este valor já existe neste campo." */
    Duplicate,
}

/** AttributeRename is the rename of one field under way. */
data class AttributeRename(
    val attributeId: String,
    val name: String,
    val problem: NameProblem? = null,
)

/**
 * RemovalQuestion is "Remover {valor}? Ele sai de {n} itens e kits.", asked before taking away a value, or a
 * whole field, that items and kits use. [valueId] is null for a field; [count] is how many picks the server counted.
 */
data class RemovalQuestion(
    val attributeId: String,
    val valueId: String?,
    val name: String,
    val count: Int,
)

/**
 * AttributesUiState is "Campos dos itens": each field with its values, the "Novo campo" name, the value being
 * typed under each field, and the question a removal asks first.
 */
data class AttributesUiState(
    val attributes: List<Attribute> = emptyList(),
    val loading: Boolean = true,
    val offlineSince: Long? = null,
    val canWrite: Boolean = false,
    val newName: String = "",
    val newProblem: NameProblem? = null,
    /** The value being typed under each field, by field id. */
    val valueTexts: Map<String, String> = emptyMap(),
    val valueProblems: Map<String, ValueProblem> = emptyMap(),
    val rename: AttributeRename? = null,
    val question: RemovalQuestion? = null,
    /** The field being changed; every button waits meanwhile. */
    val savingId: String? = null,
    val adding: Boolean = false,
    val message: CatalogMessage? = null,
) {
    /** One change at a time. */
    val busy: Boolean get() = savingId != null || adding
}

/**
 * AttributesViewModel backs "Campos": fields such as "Serve em" and their values, each change made at once. It
 * asks first before removing a value or a field that items or kits use (docs/product/design-document.md,
 * "Campos"), using the counts the API sent.
 */
@Suppress("TooManyFunctions") // One function per thing the user can do in the dialog, as the web's store.
class AttributesViewModel(
    observeSession: ObserveSession,
    private val attributes: AttributeRepository,
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

    /** The "Nome do campo" of the new field changed. */
    fun onNewNameChange(name: String) = state.update { it.copy(newName = name, newProblem = null) }

    /** "Novo campo". An empty name is marked, never sent. */
    fun onAddField() {
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
                attributes.createAttribute(AttributeDraft(name, emptyList()))
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

    /** The value being typed under [attribute] changed. */
    fun onValueTextChange(
        attribute: Attribute,
        text: String,
    ) = state.update {
        it.copy(valueTexts = it.valueTexts + (attribute.id to text), valueProblems = it.valueProblems - attribute.id)
    }

    /** "Adicionar valor" under [attribute]. An empty or repeated value is marked, never sent. */
    fun onAddValue(attribute: Attribute) {
        val now = state.value
        if (!now.canWrite || now.busy) return
        val label = now.valueTexts[attribute.id].orEmpty().trim()
        val problem =
            when {
                label.isEmpty() -> ValueProblem.Missing
                attribute.values.any { it.label.equals(label, ignoreCase = true) } -> ValueProblem.Duplicate
                else -> null
            }
        if (problem != null) {
            state.update { it.copy(valueProblems = it.valueProblems + (attribute.id to problem)) }
            return
        }
        replace(
            attribute,
            attribute.name,
            attribute.values.map(AttributeValue::toDraft) + AttributeValueDraft(null, label),
        ) {
            state.update { it.copy(valueTexts = it.valueTexts - attribute.id) }
        }
    }

    /** "Remover valor": takes [value] off [attribute], asking first while items or kits use it. */
    fun onRemoveValue(
        attribute: Attribute,
        value: AttributeValue,
    ) {
        val now = state.value
        if (!now.canWrite || now.busy) return
        if (value.usageCount > 0) {
            state.update { it.copy(question = RemovalQuestion(attribute.id, value.id, value.label, value.usageCount)) }
        } else {
            removeValue(attribute, value.id)
        }
    }

    /** "Excluir campo": deletes [attribute], asking first while items or kits use one of its values. */
    fun onDeleteField(attribute: Attribute) {
        val now = state.value
        if (!now.canWrite || now.busy) return
        // Each value's count is how many items and kits picked it; an item picking two values counts twice.
        val count = attribute.values.sumOf(AttributeValue::usageCount)
        if (count > 0) {
            state.update { it.copy(question = RemovalQuestion(attribute.id, valueId = null, attribute.name, count)) }
        } else {
            deleteField(attribute.id)
        }
    }

    /** "Cancelar" on the question. */
    fun onCancelQuestion() = state.update { it.copy(question = null) }

    /** "Remover" on the question. */
    fun onConfirmQuestion() {
        val question = state.value.question?.takeUnless { state.value.busy } ?: return
        state.update { it.copy(question = null) }
        val attribute = state.value.attributes.firstOrNull { it.id == question.attributeId } ?: return
        if (question.valueId == null) deleteField(attribute.id) else removeValue(attribute, question.valueId)
    }

    /** "Renomear" on [attribute]. */
    fun onStartRename(attribute: Attribute) {
        if (state.value.canWrite && !state.value.busy) {
            state.update { it.copy(rename = AttributeRename(attribute.id, attribute.name)) }
        }
    }

    /** The name being edited changed. */
    fun onRenameChange(name: String) =
        state.update { s -> s.copy(rename = s.rename?.copy(name = name, problem = null)) }

    /** "Cancelar" a rename. */
    fun onCancelRename() = state.update { it.copy(rename = null) }

    /** "Salvar" a rename. */
    fun onSaveRename() {
        val now = state.value
        val rename = now.rename?.takeUnless { now.busy }
        val attribute = now.attributes.firstOrNull { it.id == rename?.attributeId }
        if (rename == null || attribute == null) return
        val name = rename.name.trim()
        if (name.isEmpty()) {
            state.update { it.copy(rename = rename.copy(problem = NameProblem.Missing)) }
            return
        }
        replace(attribute, name, attribute.values.map(AttributeValue::toDraft), onTaken = {
            state.update { it.copy(rename = rename.copy(problem = NameProblem.Taken)) }
        }) {
            state.update { it.copy(rename = null) }
        }
    }

    private fun removeValue(
        attribute: Attribute,
        valueId: String,
    ) = replace(attribute, attribute.name, attribute.values.filter { it.id != valueId }.map(AttributeValue::toDraft)) {}

    private fun deleteField(id: String) = change(id) { attributes.deleteAttribute(id) }

    private fun replace(
        attribute: Attribute,
        name: String,
        values: List<AttributeValueDraft>,
        onTaken: () -> Unit = {},
        onDone: () -> Unit,
    ) = change(attribute.id, onTaken) {
        attributes.replaceAttribute(attribute.id, AttributeDraft(name, values))
        onDone()
    }

    /** Runs one change to the field [id], then reads the fields again and tells the Itens list. */
    private fun change(
        id: String,
        onTaken: () -> Unit = {},
        work: suspend () -> Unit,
    ) {
        state.update { it.copy(savingId = id, message = null) }
        viewModelScope.launch {
            try {
                work()
                changed()
            } catch (failure: Failure) {
                if (failure is Failure.AlreadyTaken) {
                    onTaken()
                } else {
                    state.update { it.copy(message = failure.toCatalogMessage()) }
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
            val fetched = attributes.attributes()
            state.update { it.copy(attributes = fetched.value, offlineSince = fetched.cachedAtEpochMillis) }
        } catch (failure: Failure) {
            state.update { it.copy(message = failure.toCatalogMessage()) }
        }
    }
}

private fun AttributeValue.toDraft(): AttributeValueDraft = AttributeValueDraft(id, label)
