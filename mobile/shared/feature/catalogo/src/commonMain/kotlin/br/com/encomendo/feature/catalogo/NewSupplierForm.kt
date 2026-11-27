package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.supplier.SupplierDraft

/**
 * NewSupplierForm is "Novo fornecedor": the name and WhatsApp typed inside the item form, for the supplier slot
 * [slot] that will take it (ADR 0025, Q5). [toString] leaves out what was typed.
 */
data class NewSupplierForm(
    val slot: SupplierSlot,
    val name: String = "",
    val phone: String = "",
    val nameIssue: SupplierProblem? = null,
    val phoneIssue: SupplierProblem? = null,
    /** Why the last "Adicionar fornecedor" failed beyond the two fields. */
    val message: CatalogMessage? = null,
    val saving: Boolean = false,
) {
    override fun toString(): String = "NewSupplierForm(slot=$slot, saving=$saving)"
}

/** SupplierProblem is what is wrong with a field of "Novo fornecedor". */
sealed interface SupplierProblem {
    /** "Informe o nome do fornecedor." */
    data object NameMissing : SupplierProblem

    /** "Use um nome de até 120 caracteres." */
    data object NameTooLong : SupplierProblem

    /** "Use um nome sem quebras de linha ou caracteres de controle." */
    data object NameControlCharacters : SupplierProblem

    /** "Já existe um fornecedor com este nome." */
    data object NameTaken : SupplierProblem

    /** "Informe o WhatsApp do fornecedor." */
    data object PhoneMissing : SupplierProblem

    /** "Confira o número: use DDD e 8 ou 9 dígitos." */
    data object PhoneInvalid : SupplierProblem

    /** What the API refused, in its own words. */
    data class Server(
        val message: String,
    ) : SupplierProblem {
        // The server's words may quote what was typed.
        override fun toString(): String = "Server"
    }
}

private const val MAX_NAME = 120
private const val COUNTRY = "55"
private val NATIONAL_NUMBER = Regex("^[1-9]{2}[0-9]{8,9}$")

/** The form's fields read as a [SupplierDraft], or null with each problem marked in the returned form. */
internal sealed interface SupplierRead {
    data class Ready(
        val draft: SupplierDraft,
    ) : SupplierRead

    data class Marked(
        val form: NewSupplierForm,
    ) : SupplierRead
}

/**
 * Reads the form the way the web does: the same empty-field, length and number checks, before anything is sent.
 * The phone is typed with or without "(11)" and the dash; the draft carries digits only with the country code.
 */
internal fun NewSupplierForm.read(): SupplierRead {
    val trimmed = name.trim()
    val nameProblem =
        when {
            trimmed.isEmpty() -> SupplierProblem.NameMissing
            trimmed.length > MAX_NAME -> SupplierProblem.NameTooLong
            trimmed.any { it.isISOControl() } -> SupplierProblem.NameControlCharacters
            else -> null
        }
    val digits = phone.filter(Char::isDigit)
    val national =
        if (digits.startsWith(COUNTRY) &&
            digits.length > NATIONAL_MAX
        ) {
            digits.removePrefix(COUNTRY)
        } else {
            digits
        }
    val phoneProblem =
        when {
            digits.isEmpty() -> SupplierProblem.PhoneMissing
            !NATIONAL_NUMBER.matches(national) -> SupplierProblem.PhoneInvalid
            else -> null
        }
    if (nameProblem != null || phoneProblem != null) {
        return SupplierRead.Marked(copy(nameIssue = nameProblem, phoneIssue = phoneProblem, message = null))
    }
    return SupplierRead.Ready(SupplierDraft(name = trimmed, whatsappPhone = COUNTRY + national))
}

private const val NATIONAL_MAX = 11
