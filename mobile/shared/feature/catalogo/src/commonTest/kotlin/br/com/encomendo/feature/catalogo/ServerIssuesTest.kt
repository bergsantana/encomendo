package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.failure.InputIssue
import kotlin.test.Test
import kotlin.test.assertEquals

class ServerIssuesTest {
    private val fields =
        ItemFields()
            .withNewOption()
            .withNewChoice(1)
            .withNewOption()

    private fun placed(vararg names: String) = placeServerIssues(names.map { InputIssue(it, "texto de $it") }, fields)

    @Test
    fun placesTheItemsOwnFields() {
        val result = placed("name", "priceCents", "productionDays", "productionCostCents")

        assertEquals(
            setOf(ItemField.Name, ItemField.Price, ItemField.ProductionDays, ItemField.ProductionCost),
            result.placed.keys,
        )
        assertEquals(FieldIssue.Server("texto de name"), result.placed[ItemField.Name])
        assertEquals(emptyList(), result.unplaced)
    }

    @Test
    fun placesTheSuppliersBySlot() {
        val result = placed("mainSupplier.supplierId", "mainSupplier.leadDays", "backupSupplier.costCents", "leadDays")

        assertEquals(
            setOf(
                ItemField.Supplier(SupplierSlot.Main),
                ItemField.LeadDays(SupplierSlot.Main),
                ItemField.Cost(SupplierSlot.Backup),
            ),
            result.placed.keys,
            "a bare leadDays is the main supplier's, and the first word about a field wins",
        )
    }

    @Test
    fun placesOptionsAndChoicesByTheirPositionInTheSentForm() {
        val result =
            placed(
                "options[0].name",
                "options[0].choices",
                "options[0].choices[1].label",
                "options[1].choices[0].extraDays",
            )

        assertEquals(
            setOf(
                ItemField.OptionName(1),
                ItemField.OptionChoices(1),
                ItemField.ChoiceLabel(1, 2),
                ItemField.ChoiceDays(2, 1),
            ),
            result.placed.keys,
        )
    }

    @Test
    fun leavesWhatTheFormHasNoPlaceFor() {
        val result = placed("somethingNew", "options[7].name", "options[0].choices[9].label")

        assertEquals(emptyMap(), result.placed)
        assertEquals(3, result.unplaced.size)
    }
}
