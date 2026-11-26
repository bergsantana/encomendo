package br.com.encomendo.feature.auth

/** StoreChoice is one store the user belongs to, as the picker lists it. */
data class StoreChoice(
    val id: String,
    val name: String,
    /** The store's address label: `{slug}.encomendo.com.br`. */
    val slug: String,
)

/**
 * StorePickerUiState is the store picker: the user's [stores] to choose from,
 * and the form that creates another ([name], [slug] and their issues). At most
 * one thing runs at a time: opening a store ([opening] holds its id), creating
 * one, or logging out.
 */
data class StorePickerUiState(
    /** Null until the session is read, so the first frame never shows the wrong title. */
    val stores: List<StoreChoice>? = null,
    /** The store open now, when reached from "Trocar de loja"; choosing it again just closes the picker. */
    val currentStoreId: String? = null,
    val name: String = "",
    val slug: String = "",
    val nameIssue: FieldProblem? = null,
    val slugIssue: FieldProblem? = null,
    val opening: String? = null,
    val creating: Boolean = false,
    val loggingOut: Boolean = false,
    val logOutFailed: Boolean = false,
    val error: FormError? = null,
) {
    /** Whether something runs; every button waits for it. */
    val busy: Boolean get() = opening != null || creating || loggingOut

    /** Whether Criar loja can be tapped: whenever nothing runs. Empty fields are marked on tap. */
    val canCreate: Boolean get() = !busy

    /** Leaves out what was typed and the store names, so a logged state carries no business data. */
    override fun toString(): String =
        "StorePickerUiState(stores=${stores?.size}, opening=${opening != null}, creating=$creating, " +
            "loggingOut=$loggingOut, logOutFailed=$logOutFailed, error=$error, nameIssue=${nameIssue != null}, " +
            "slugIssue=${slugIssue != null})"
}

/** PickerEffect is something the picker's screen does once, such as leaving. */
sealed interface PickerEffect {
    /** Back to the screen the picker was opened from: the store chosen is already open. */
    data object Close : PickerEffect
}
