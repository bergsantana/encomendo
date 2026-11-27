package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.ui.failure.FailureMessage
import br.com.encomendo.core.ui.failure.toFailureMessage

/**
 * CatalogMessage is why a Catálogo read or change failed, in the design document's words for Catálogo
 * (`docs/product/design-document.md`). A failure it has no words of its own for is a [General] message, worded as
 * on every staff screen.
 */
sealed interface CatalogMessage {
    /** "Confira os campos destacados.", above the form. */
    data object CheckFields : CatalogMessage

    /** "Outra pessoa alterou este item. Recarregue para ver a versão atual." with "Recarregar". */
    data object Conflict : CatalogMessage

    /** "Outra pessoa alterou este kit. Recarregue para ver a versão atual." with "Recarregar". */
    data object KitConflict : CatalogMessage

    /** "Cada kit pode ter até 8 fotos." */
    data object KitPhotoLimit : CatalogMessage

    /**
     * "Este item está no kit {kit}. Tire-o do kit antes de arquivar.", or with no kit named "Este item está em um
     * kit…", or with several "Este item está nos kits {A}, {B} e {C}…".
     */
    data class ItemInKit(
        val kits: List<String>,
    ) : CatalogMessage

    /**
     * "{item} está arquivado e não pode entrar no kit." once for each archived item the API names, or with none named
     * "Um item do kit está arquivado e não pode entrar no kit."
     */
    data class KitItemsArchived(
        val items: List<String>,
    ) : CatalogMessage

    /** "Esta categoria tem itens. Mude os itens de categoria antes de excluir." */
    data object CategoryInUse : CatalogMessage

    /** "Esta foto passa de 10 MB.", from the picker, which refuses a big file before it is read. */
    data object PhotoTooBig : CatalogMessage

    /** "Use uma foto JPEG, PNG ou WebP de até 10 MB.", from the API, which looks at the file's content. */
    data object PhotoInvalid : CatalogMessage

    /** "Cada item pode ter até 8 fotos." */
    data object PhotoLimit : CatalogMessage

    /** "A foto não foi enviada. Tente de novo." */
    data object PhotoNotUploaded : CatalogMessage

    /** Any other failure, in the words every staff screen uses. */
    data class General(
        val message: FailureMessage,
    ) : CatalogMessage
}

/**
 * Returns the message for a failure of a Catálogo read or change on an item or, with [subject] `Kit`, on a kit: the
 * sentences that name which are chosen here, so the ViewModel decides them and its tests prove them.
 */
internal fun Failure.toCatalogMessage(subject: CatalogSubject = CatalogSubject.Item): CatalogMessage =
    when (this) {
        is Failure.VersionConflict ->
            if (subject == CatalogSubject.Kit) CatalogMessage.KitConflict else CatalogMessage.Conflict
        is Failure.ItemInKit -> CatalogMessage.ItemInKit(kits.map { it.name })
        is Failure.ItemArchived -> CatalogMessage.KitItemsArchived(items.distinctBy { it.id }.map { it.name })
        is Failure.CategoryInUse -> CatalogMessage.CategoryInUse
        is Failure.PhotoInvalid -> CatalogMessage.PhotoInvalid
        is Failure.PhotoLimit ->
            if (subject == CatalogSubject.Kit) CatalogMessage.KitPhotoLimit else CatalogMessage.PhotoLimit
        is Failure.PhotoNotUploaded -> CatalogMessage.PhotoNotUploaded
        else -> CatalogMessage.General(toFailureMessage())
    }

/** Joins names as a Brazilian reads a list: "A", "A e B", "A, B e C". */
internal fun List<String>.joinedWithE(): String =
    when (size) {
        0, 1 -> joinToString("")
        else -> dropLast(1).joinToString(", ") + " e " + last()
    }
