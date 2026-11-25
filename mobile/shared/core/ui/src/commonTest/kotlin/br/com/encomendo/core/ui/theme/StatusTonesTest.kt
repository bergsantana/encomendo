package br.com.encomendo.core.ui.theme

import br.com.encomendo.core.domain.workflow.WorkflowPhase
import kotlin.test.Test
import kotlin.test.assertEquals

class StatusTonesTest {
    @Test
    fun matchesTheWebMapping() {
        // Same table as web/packages/ui/src/components.test.ts.
        val expected =
            mapOf(
                WorkflowPhase.Draft to StatusTone.Planned,
                WorkflowPhase.AwaitingDeposit to StatusTone.Planned,
                WorkflowPhase.Confirmed to StatusTone.Planned,
                WorkflowPhase.Sourcing to StatusTone.Waiting,
                WorkflowPhase.Assembling to StatusTone.Assembly,
                WorkflowPhase.Ready to StatusTone.Ready,
                WorkflowPhase.HandedOver to StatusTone.Delivered,
                WorkflowPhase.Cancelled to StatusTone.Neutral,
            )

        assertEquals(expected, WorkflowPhase.entries.associateWith(::toneFor))
    }
}
