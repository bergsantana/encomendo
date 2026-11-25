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
    val nameIssue: String? = null,
    val slugIssue: String? = null,
    /** The link belongs to another store; the screen asks for another in its own words. */
    val slugTaken: Boolean = false,
    val opening: String? = null,
    val creating: Boolean = false,
    val loggingOut: Boolean = false,
    val logOutFailed: Boolean = false,
    val error: PickerError? = null,
) {
    /** Whether something runs; every button waits for it. */
    val busy: Boolean get() = opening != null || creating || loggingOut

    /** Whether Criar loja is enabled: both fields filled in and nothing running. */
    val canCreate: Boolean get() = !busy && name.isNotBlank() && slug.isNotBlank()

    /** Leaves out what was typed and the store names, so a logged state carries no business data. */
    override fun toString(): String =
        "StorePickerUiState(stores=${stores?.size}, opening=${opening != null}, creating=$creating, " +
            "loggingOut=$loggingOut, logOutFailed=$logOutFailed, error=$error, slugTaken=$slugTaken)"
}

/** PickerEffect is something the picker's screen does once, such as leaving. */
sealed interface PickerEffect {
    /** Back to the screen the picker was opened from: the store chosen is already open. */
    data object Close : PickerEffect
}

/** PickerError is a problem with the whole picker, shown under its title. */
sealed interface PickerError {
    /** The user no longer belongs to the store they chose. */
    data object NotAMember : PickerError

    /** The create form has fields to fix; each says what. */
    data object CheckFields : PickerError

    /** The call never reached the server. */
    data object Offline : PickerError

    /** Too many attempts; [retryAfterSeconds] is the wait, or null when the server gave none. */
    data class RateLimited(
        val retryAfterSeconds: Int?,
    ) : PickerError

    /** The server failed or did not answer (a 5xx). */
    data object ServerUnavailable : PickerError

    /** Anything else: an answer the app cannot use, or a failure on the device. */
    data object Unexpected : PickerError
}
