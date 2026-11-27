package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.KitSummary
import br.com.encomendo.core.domain.catalog.isShown

/**
 * KitsUiState is the Kits list: the rows read so far, "Mostrar arquivados", whether the rows are the device's copy,
 * and what the last change said. Rows carry a margin only when the API sent it, so a screen shows "Margem" exactly
 * when [showsMargin] is true.
 */
data class KitsUiState(
    /** The rows read so far, by name. */
    val kits: List<KitSummary> = emptyList(),
    /** "Mostrar arquivados". */
    val includeArchived: Boolean = false,
    /** The first page is being read: "Carregando os kits…" while no row shows. */
    val loading: Boolean = true,
    /** The list runs past what was read: "Carregar mais". */
    val hasMore: Boolean = false,
    /** The next page is being read. */
    val loadingMore: Boolean = false,
    /** When the rows shown were saved on the device, in epoch milliseconds, if the server did not answer. */
    val offlineSince: Long? = null,
    /** The role may change the catalog (`catalog:write`): the buttons and the "No portal" switch show. */
    val canWrite: Boolean = false,
    /** The row whose "No portal" switch is being saved. */
    val savingId: String? = null,
    /** The last change that went through, for the status line. */
    val notice: KitNotice? = null,
    /** Why the last read or change failed. */
    val message: CatalogMessage? = null,
) {
    /** True when the API sent a margin on a row: the owner's line shows. */
    val showsMargin: Boolean get() = kits.any { it.margin.isShown }

    /** The read ended with nothing to show and no failure. */
    val isEmpty: Boolean get() = !loading && kits.isEmpty() && message == null

    /** The first read failed, so "Tentar de novo" shows. */
    val canRetry: Boolean get() = message != null && kits.isEmpty() && !loading
}
