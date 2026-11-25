package br.com.encomendo.feature.auth

/** Returns [typed] as CreateStore will send it: trimmed and lower-cased. */
internal fun storeLink(typed: String): String = typed.trim().lowercase()

/**
 * True when [typed] fits the contract's `Slug` (3 to 40 lower-case letters,
 * digits and single hyphens). Only to word the hint under the field, as the
 * web does; the API still decides, reserved words included.
 */
internal fun looksLikeAStoreLink(typed: String): Boolean {
    val link = storeLink(typed)
    return link.length in SHORTEST..LONGEST && SLUG.matches(link)
}

private val SLUG = Regex("^[a-z0-9]+(-[a-z0-9]+)*$")
private const val SHORTEST = 3
private const val LONGEST = 40
