package br.com.encomendo.feature.auth

/**
 * AccountUiState is the management app bar: who is signed in ([userName]),
 * in which store ([storeName], null when none is selected), and the account
 * menu with Trocar de loja and Sair.
 */
data class AccountUiState(
    val userName: String = "",
    val storeName: String? = null,
    /** Two stores or more: the menu offers "Trocar de loja", as on the web. */
    val canSwitchStore: Boolean = false,
    val menuOpen: Boolean = false,
    val loggingOut: Boolean = false,
    val logOutFailed: Boolean = false,
) {
    /** Leaves out the user's name, so a logged state carries no personal data. */
    override fun toString(): String =
        "AccountUiState(store=${storeName != null}, menuOpen=$menuOpen, loggingOut=$loggingOut, " +
            "logOutFailed=$logOutFailed)"
}
