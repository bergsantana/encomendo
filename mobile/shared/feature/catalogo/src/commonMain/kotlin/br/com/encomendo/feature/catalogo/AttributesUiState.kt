package br.com.encomendo.feature.catalogo

/** ValueProblem is what is wrong with a value typed under "Valores". */
enum class ValueProblem {
    /** "Informe o valor." */
    Missing,

    /** "Este valor já existe neste campo." */
    Duplicate,
}

/** ValueEditor is one value of a field as typed. [usageCount] is how many items and kits picked it, from the API. */
data class ValueEditor(
    val key: Int,
    /** The server's id of the value this keeps, so the items that picked it keep it; null for a new value. */
    val id: String?,
    val label: String,
    val usageCount: Int = 0,
    val problem: ValueProblem? = null,
)

/**
 * AttributeEditor is one field of "Campos" as a draft: the name and values as typed, until "Salvar campo" saves it.
 * [id] is null for a field that was never saved ("Novo campo").
 */
data class AttributeEditor(
    val key: Int,
    val id: String?,
    val name: String,
    val values: List<ValueEditor> = emptyList(),
    val nameProblem: NameProblem? = null,
) {
    /** Returns the editor with the value [key] set by [change]. */
    fun withValue(
        key: Int,
        change: (ValueEditor) -> ValueEditor,
    ): AttributeEditor = copy(values = values.map { if (it.key == key) change(it) else it })

    /** Returns the editor without the value [key]. */
    fun withoutValue(key: Int): AttributeEditor = copy(values = values.filter { it.key != key })

    /** A key for a value added to this draft. */
    fun nextValueKey(): Int = (values.maxOfOrNull(ValueEditor::key) ?: 0) + 1

    /**
     * Returns the editor with its problems marked, as the web does before saving: an empty name, an empty value, a
     * value repeated in the field. Null when there is none, so the draft can be saved.
     */
    fun withProblems(): AttributeEditor? {
        val seen = mutableSetOf<String>()
        val marked =
            values.map { value ->
                val label = value.label.trim()
                val problem =
                    when {
                        label.isEmpty() -> ValueProblem.Missing
                        !seen.add(label.lowercase()) -> ValueProblem.Duplicate
                        else -> null
                    }
                value.copy(problem = problem)
            }
        val nameProblem = if (name.isBlank()) NameProblem.Missing else null
        val clean = nameProblem == null && marked.all { it.problem == null }
        return if (clean) null else copy(values = marked, nameProblem = nameProblem)
    }
}

/**
 * RemovalQuestion is what "Campos" asks before taking away something that items and kits use: a value ("Remover
 * {valor}? Ele sai de {n} itens e kits.") or a whole field ("Remover {campo}? Seus valores saem dos itens e kits que
 * os usam."), whose words name no count: the API counts each value, and adding them would count an item twice.
 */
sealed interface RemovalQuestion {
    /** The draft the question is about. */
    val editorKey: Int

    /** The name the question quotes. */
    val name: String

    /** A value picked by [count] items and kits. */
    data class OfValue(
        override val editorKey: Int,
        val valueKey: Int,
        override val name: String,
        val count: Int,
    ) : RemovalQuestion

    /** A field one of whose values is in use. */
    data class OfField(
        override val editorKey: Int,
        override val name: String,
    ) : RemovalQuestion
}

/**
 * AttributesUiState is "Campos dos itens": each field as a draft, the question a removal asks first, and what the
 * last read or save said.
 */
data class AttributesUiState(
    val editors: List<AttributeEditor> = emptyList(),
    val loading: Boolean = true,
    val offlineSince: Long? = null,
    val canWrite: Boolean = false,
    /** The field being saved or deleted; every button waits meanwhile. */
    val savingKey: Int? = null,
    val question: RemovalQuestion? = null,
    val message: CatalogMessage? = null,
) {
    /** One change at a time. */
    val busy: Boolean get() = savingKey != null

    /** A key for a field added to the screen. */
    fun nextKey(): Int = (editors.maxOfOrNull(AttributeEditor::key) ?: 0) + 1
}
