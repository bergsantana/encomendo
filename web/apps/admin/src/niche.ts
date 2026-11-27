/**
 * niche holds the words that change from one kind of business to another.
 * Until the store's template exists (milestone 7) every store reads this
 * default set; components take their niche words from here and never write
 * them as literals (ADR 0025, "Niche words until templates exist").
 */
export const niche = {
  /** montagem is the step after the parts arrive, such as the kit's "Dias de Montagem". */
  montagem: 'Montagem',
} as const
