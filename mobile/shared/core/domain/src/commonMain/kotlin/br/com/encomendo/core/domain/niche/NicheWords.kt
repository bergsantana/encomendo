package br.com.encomendo.core.domain.niche

/**
 * NicheWords are the words that change from one kind of business to another, such as the kit's "Dias de Montagem".
 * Until the store's template exists (milestone 7) every store reads [Default]; screens take their niche words from
 * here and never write them as literals (ADR 0025, "Niche words until templates exist"). The template then
 * replaces the default set without a screen changing.
 */
data class NicheWords(
    /** The step after the parts arrive: "Montagem" here, "Instalação" for another niche. */
    val montagem: String,
) {
    /** Holds the word set every store reads until templates exist. */
    companion object {
        /** The default word set. */
        val Default: NicheWords = NicheWords(montagem = "Montagem")
    }
}
