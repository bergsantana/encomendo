package br.com.encomendo.core.ui.theme

import br.com.encomendo.core.domain.workflow.WorkflowPhase

/**
 * Returns the badge color family for an order phase. Matches toneForPhase in
 * web/packages/ui so a status looks the same on web and Android.
 */
fun toneFor(phase: WorkflowPhase): StatusTone =
    when (phase) {
        WorkflowPhase.Draft,
        WorkflowPhase.AwaitingDeposit,
        WorkflowPhase.Confirmed,
        -> StatusTone.Planned
        WorkflowPhase.Sourcing -> StatusTone.Waiting
        WorkflowPhase.Assembling -> StatusTone.Assembly
        WorkflowPhase.Ready -> StatusTone.Ready
        WorkflowPhase.HandedOver -> StatusTone.Delivered
        WorkflowPhase.Cancelled -> StatusTone.Neutral
    }
