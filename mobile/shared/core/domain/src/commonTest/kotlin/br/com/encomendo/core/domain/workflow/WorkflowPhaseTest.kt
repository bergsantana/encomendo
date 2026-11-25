package br.com.encomendo.core.domain.workflow

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class WorkflowPhaseTest {
    @Test
    fun readsEveryPhaseTheApiSends() {
        val apiValues =
            listOf(
                "draft",
                "awaiting_deposit",
                "confirmed",
                "sourcing",
                "assembling",
                "ready",
                "handed_over",
                "cancelled",
            )

        assertEquals(apiValues, apiValues.map { WorkflowPhase.fromApi(it)?.apiValue })
    }

    @Test
    fun returnsNullForUnknownPhasesInsteadOfCrashing() {
        assertNull(WorkflowPhase.fromApi("Compras")) // a status label is never a phase
        assertNull(WorkflowPhase.fromApi(""))
    }
}
