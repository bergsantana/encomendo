package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.supplier.CreateSupplier
import br.com.encomendo.core.domain.supplier.Supplier
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/**
 * NewSupplierController runs "Novo fornecedor" inside the item form (ADR 0025): the same empty-field, length and
 * number checks as the web, then the supplier is created and handed to [onCreated] with the slot that asked for it.
 * It changes only [form], in [scope], which is the ViewModel's. The dialog cannot be closed while it saves, so a
 * supplier created behind the user's back is never lost.
 */
class NewSupplierController(
    private val scope: CoroutineScope,
    private val createSupplier: CreateSupplier,
    private val form: StateSlot<NewSupplierForm?>,
    private val onCreated: (SupplierSlot, Supplier) -> Unit,
) {
    /** "Novo fornecedor" was chosen for [slot]: opens its form. */
    fun open(slot: SupplierSlot) = form.update { NewSupplierForm(slot) }

    /** The "Nome do fornecedor" changed; its mark goes. */
    fun onName(name: String) = edit { it.copy(name = name, nameIssue = null) }

    /** The "WhatsApp do fornecedor" changed; its mark goes. */
    fun onPhone(phone: String) = edit { it.copy(phone = phone, phoneIssue = null) }

    /** "Cancelar" in "Novo fornecedor"; ignored while the supplier is being saved. */
    fun close() = form.update { if (it?.saving == true) it else null }

    /** "Adicionar fornecedor": creates it and hands it over. Empty and wrong fields are marked, never sent. */
    fun add() {
        val dialog = form.value?.takeUnless { it.saving } ?: return
        when (val read = dialog.read()) {
            is SupplierRead.Marked -> form.update { read.form }
            is SupplierRead.Ready -> create(dialog, read)
        }
    }

    private fun create(
        dialog: NewSupplierForm,
        read: SupplierRead.Ready,
    ) {
        form.update { dialog.copy(saving = true, message = null) }
        scope.launch {
            try {
                val created = createSupplier(read.draft)
                form.update { null }
                onCreated(dialog.slot, created)
            } catch (failure: Failure) {
                form.update { it?.refusedBy(failure) }
            }
        }
    }

    private fun edit(change: (NewSupplierForm) -> NewSupplierForm) = form.update { it?.let(change) }
}

/** What "Novo fornecedor" says about a refusal: the server's words next to its fields, or one above them. */
private fun NewSupplierForm.refusedBy(failure: Failure): NewSupplierForm {
    val cleared = copy(saving = false, message = null)
    return when (failure) {
        is Failure.AlreadyTaken ->
            if (failure.field == "name") {
                cleared.copy(nameIssue = SupplierProblem.NameTaken)
            } else {
                cleared.copy(message = failure.toCatalogMessage())
            }
        is Failure.InvalidInput -> cleared.withServerIssues(failure)
        else -> cleared.copy(message = failure.toCatalogMessage())
    }
}

private fun NewSupplierForm.withServerIssues(failure: Failure.InvalidInput): NewSupplierForm {
    val name =
        failure.issues
            .firstOrNull { it.field == "name" }
            ?.message
            ?.let(SupplierProblem::Server)
    val phone =
        failure.issues
            .firstOrNull { it.field == "whatsappPhone" }
            ?.message
            ?.let(SupplierProblem::Server)
    return if (name == null && phone == null) {
        copy(message = failure.toCatalogMessage())
    } else {
        copy(nameIssue = name, phoneIssue = phone)
    }
}
