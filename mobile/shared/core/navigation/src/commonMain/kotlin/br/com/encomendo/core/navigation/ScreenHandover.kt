package br.com.encomendo.core.navigation

/**
 * ScreenHandover is told each time the root has replaced one session's
 * screens with the next one's: a login, a logout, a store switch. [scope]
 * is the scope the new screens serve (`Session.scope`), or null while signed
 * out. Only those screens may send that scope's token (ADR 0018); the old
 * screens are cleared by then, their work cancelled.
 */
fun interface ScreenHandover {
    /** The previous screens are cleared; screens serving [scope] take over. */
    fun screensReplaced(scope: String?)
}
