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

    /** "Este item está no kit {kit}. Tire-o do kit antes de arquivar." */
    data class ItemInKit(
        val kits: List<String>,
    ) : CatalogMessage

    /** "Esta categoria tem itens. Mude os itens de categoria antes de excluir." */
    data object CategoryInUse : CatalogMessage

    /** "Esta foto passa de 10 MB." */
    data object PhotoTooBig : CatalogMessage

    /** "Use uma foto JPEG, PNG ou WebP." */
    data object PhotoWrongType : CatalogMessage

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
 * Returns the message for a failure of a Catálogo change. A photo the API found invalid is [CatalogMessage.PhotoWrongType]
 * here; the form tells a file over 10 MB apart before it asks.
 */
internal fun Failure.toCatalogMessage(): CatalogMessage =
    when (this) {
        is Failure.VersionConflict -> CatalogMessage.Conflict
        is Failure.ItemInKit -> CatalogMessage.ItemInKit(kits.map { it.name })
        is Failure.CategoryInUse -> CatalogMessage.CategoryInUse
        is Failure.PhotoInvalid -> CatalogMessage.PhotoWrongType
        is Failure.PhotoLimit -> CatalogMessage.PhotoLimit
        is Failure.PhotoNotUploaded -> CatalogMessage.PhotoNotUploaded
        else -> CatalogMessage.General(toFailureMessage())
    }
