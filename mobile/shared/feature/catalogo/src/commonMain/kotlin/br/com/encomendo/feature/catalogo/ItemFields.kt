package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.CustomizationOption
import br.com.encomendo.core.domain.catalog.Item
import br.com.encomendo.core.domain.catalog.OptionChoice
import br.com.encomendo.core.domain.catalog.SourcingMode
import br.com.encomendo.core.domain.catalog.SupplierItem
import br.com.encomendo.core.domain.catalog.valueOrNull

/**
 * SupplierFields is "Fornecedor principal" or "Fornecedor reserva" as typed: the supplier chosen, the lead days and
 * the cost. [costTouched] says the owner edited the cost; until then the write leaves the cost out and the server
 * keeps it (ADR 0025).
 */
data class SupplierFields(
    /** The supplier's id, or null while none is chosen. */
    val supplierId: String? = null,
    /** "Prazo do fornecedor (dias)", as typed. */
    val leadDays: String = "",
    /** "Custo", as typed; only the owner sees it. */
    val cost: String = "",
    /** The owner edited [cost]. */
    val costTouched: Boolean = false,
)

/** ChoiceFields is one "Escolha" of an option as typed; [key] identifies it on screen. */
data class ChoiceFields(
    val key: Int,
    /** The server's id of the choice this keeps; null for a new one. */
    val id: String?,
    val label: String,
    /** "Acréscimo no preço", as typed; blank is zero. */
    val extraPrice: String = "",
    /** "Dias a mais", as typed; blank is zero. */
    val extraDays: String = "",
)

/** OptionFields is one "Opções do cliente" entry as typed; [key] identifies it on screen. */
data class OptionFields(
    val key: Int,
    /** The server's id of the option this keeps; null for a new one. */
    val id: String?,
    val name: String,
    /** "Obrigatória". */
    val required: Boolean = false,
    val choices: List<ChoiceFields> = emptyList(),
)

/**
 * ItemFields is the item form as the user typed it: text stays text until "Salvar", where it is read into the
 * values the API takes. Editing never touches the server.
 */
data class ItemFields(
    val name: String = "",
    val description: String = "",
    val categoryId: String? = null,
    val mode: SourcingMode = SourcingMode.ToOrder,
    /** "Preço", as typed. */
    val price: String = "",
    val main: SupplierFields = SupplierFields(),
    /** "Fornecedor reserva"; null while it is not added. */
    val backup: SupplierFields? = null,
    /** "Dias de produção", as typed. */
    val productionDays: String = "",
    /** "Custo de produção", as typed; only the owner sees it. */
    val productionCost: String = "",
    val productionCostTouched: Boolean = false,
    /** The ids of the field values the item picks. */
    val attributeValueIds: Set<String> = emptySet(),
    val options: List<OptionFields> = emptyList(),
    /** "Mostrar na página da loja". */
    val published: Boolean = false,
    /**
     * The cost text the item had stored for each supplier it was loaded with, so choosing supplier A, then B, then A
     * again shows A's cost again: a cost belongs to the item and one supplier (ADR 0025).
     */
    val storedCosts: Map<String, String> = emptyMap(),
) {
    /** Returns the fields with the main supplier set by [change]. */
    fun withMain(change: (SupplierFields) -> SupplierFields): ItemFields = copy(main = change(main))

    /** Returns the fields with the backup supplier set by [change]; ignored while there is no backup. */
    fun withBackup(change: (SupplierFields) -> SupplierFields): ItemFields = copy(backup = backup?.let(change))

    /** Returns the fields with the option [key] set by [change]. */
    fun withOption(
        key: Int,
        change: (OptionFields) -> OptionFields,
    ): ItemFields = copy(options = options.map { if (it.key == key) change(it) else it })

    /** Returns the fields with the choice [choiceKey] of option [optionKey] set by [change]. */
    fun withChoice(
        optionKey: Int,
        choiceKey: Int,
        change: (ChoiceFields) -> ChoiceFields,
    ): ItemFields =
        withOption(optionKey) { option ->
            option.copy(choices = option.choices.map { if (it.key == choiceKey) change(it) else it })
        }

    /** Returns the fields with a new, empty option. */
    fun withNewOption(): ItemFields {
        val option = OptionFields(key = nextOptionKey(options), id = null, name = "", choices = listOf(emptyChoice(1)))
        return copy(options = options + option)
    }

    /** Returns the fields with a new, empty choice on option [optionKey]. */
    fun withNewChoice(optionKey: Int): ItemFields =
        withOption(optionKey) { option ->
            val key = (option.choices.maxOfOrNull(ChoiceFields::key) ?: 0) + 1
            option.copy(choices = option.choices + emptyChoice(key))
        }

    /** Returns the fields with the supplier of [slot] set by [change]. */
    fun withSlot(
        slot: SupplierSlot,
        change: (SupplierFields) -> SupplierFields,
    ): ItemFields = if (slot == SupplierSlot.Main) withMain(change) else withBackup(change)

    /**
     * Returns the fields with [supplierId] chosen for [slot]. The cost shown is the one stored for that supplier, or
     * none for a supplier new to the item, and counts as untouched, so a save leaves it to the server (ADR 0025).
     */
    fun withSupplier(
        slot: SupplierSlot,
        supplierId: String,
    ): ItemFields =
        withSlot(slot) { fields ->
            if (fields.supplierId == supplierId) {
                fields
            } else {
                val stored = storedCosts[supplierId].orEmpty()
                fields.copy(supplierId = supplierId, cost = stored, costTouched = false)
            }
        }

    /** Returns the fields with a backup supplier row, which "Remover fornecedor reserva" takes away again. */
    fun withBackupAdded(): ItemFields = copy(backup = backup ?: SupplierFields())

    /** Returns the fields with the value [valueId] picked or not picked. */
    fun withValuePicked(
        valueId: String,
        picked: Boolean,
    ): ItemFields = copy(attributeValueIds = if (picked) attributeValueIds + valueId else attributeValueIds - valueId)
}

/** Returns the fields the form of [this] item starts with; costs show as the API sent them. */
internal fun Item.toFields(): ItemFields =
    ItemFields(
        name = name,
        description = description,
        categoryId = categoryId,
        mode = sourcingMode,
        price = moneyInputText(price),
        main = mainSupplier.toFields(),
        backup = backupSupplier?.toFields(),
        productionDays = productionDays?.toString().orEmpty(),
        productionCost = productionCost.valueOrNull?.let(::moneyInputText).orEmpty(),
        attributeValueIds = attributeValueIds.toSet(),
        options = options.mapIndexed { index, option -> option.toFields(key = index + 1) },
        published = published,
        storedCosts = storedCosts(),
    )

private fun Item.storedCosts(): Map<String, String> =
    listOfNotNull(mainSupplier, backupSupplier)
        .mapNotNull { link -> link.cost.valueOrNull?.let { link.supplierId to moneyInputText(it) } }
        .toMap()

private fun SupplierItem?.toFields(): SupplierFields =
    if (this == null) {
        SupplierFields()
    } else {
        SupplierFields(supplierId, leadDays.toString(), cost.valueOrNull?.let(::moneyInputText).orEmpty())
    }

private fun CustomizationOption.toFields(key: Int): OptionFields =
    OptionFields(key, id, name, required, choices.mapIndexed { index, choice -> choice.toFields(index + 1) })

private fun OptionChoice.toFields(key: Int): ChoiceFields =
    ChoiceFields(key, id, label, moneyInputText(extraPrice), extraDays.toString())

private fun nextOptionKey(options: List<OptionFields>): Int = (options.maxOfOrNull(OptionFields::key) ?: 0) + 1

private fun emptyChoice(key: Int) = ChoiceFields(key = key, id = null, label = "")
