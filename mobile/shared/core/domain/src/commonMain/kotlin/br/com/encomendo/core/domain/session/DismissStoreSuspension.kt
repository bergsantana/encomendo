package br.com.encomendo.core.domain.session

/** DismissStoreSuspension says the user has read why they are on the store picker. */
class DismissStoreSuspension(
    private val notice: StoreSuspensionNotice,
) {
    /** Clears the notice. */
    operator fun invoke() = notice.dismiss()
}
