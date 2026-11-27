package br.com.encomendo.feature.catalogo

import br.com.encomendo.core.domain.catalog.Photo

/** What the photo buttons of a screen do; the item form and the kit editor share them. */
data class PhotoActions(
    val onAdd: (PhotoSource) -> Unit,
    val onMakeCover: (Photo) -> Unit,
    val onRemove: (Photo) -> Unit,
)
