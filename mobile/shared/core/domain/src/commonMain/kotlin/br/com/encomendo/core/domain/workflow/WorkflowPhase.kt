package br.com.encomendo.core.domain.workflow

/**
 * The fixed order phases (docs/adr/0004-status-workflows.md).
 *
 * Each store names its own statuses on top of these phases. The app decides
 * colors and rules by phase only, never by a status label or key.
 */
enum class WorkflowPhase(
    val apiValue: String,
) {
    Draft("draft"),
    AwaitingDeposit("awaiting_deposit"),
    Confirmed("confirmed"),
    Sourcing("sourcing"),
    Assembling("assembling"),
    Ready("ready"),
    HandedOver("handed_over"),
    Cancelled("cancelled"),
    ;

    companion object {
        /** Returns the phase for an API value, or null for a phase this app version does not know yet. */
        fun fromApi(value: String): WorkflowPhase? = entries.firstOrNull { it.apiValue == value }
    }
}
