package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.photo_cover_mark
import br.com.encomendo.feature.catalogo.resources.photo_make_cover
import br.com.encomendo.feature.catalogo.resources.photo_number
import br.com.encomendo.feature.catalogo.resources.photo_remove
import br.com.encomendo.feature.catalogo.resources.photos_add
import br.com.encomendo.feature.catalogo.resources.photos_camera
import br.com.encomendo.feature.catalogo.resources.photos_gallery
import br.com.encomendo.feature.catalogo.resources.photos_hint
import br.com.encomendo.feature.catalogo.resources.photos_save_first
import br.com.encomendo.feature.catalogo.resources.photos_title
import org.jetbrains.compose.resources.stringResource

/** "Fotos": the item's photos with "Usar como capa" and "Remover foto"; they change at once, not with "Salvar". */
@Composable
internal fun PhotosEditor(
    state: ItemFormUiState,
    actions: ItemFormActions,
) {
    SectionTitle(stringResource(Res.string.photos_title))
    if (state.isNew) {
        // A photo belongs to a saved item: the upload names the item's id.
        Text(stringResource(Res.string.photos_save_first), style = MaterialTheme.typography.bodyLarge)
        return
    }
    state.photoMessage?.let { MessageText(message = it) }
    state.photos.forEachIndexed { index, photo -> PhotoRow(index, photo, state, actions) }
    AddPhotoButton(enabled = !state.photoBusy, onPick = actions.onAddPhoto)
    Text(
        text = stringResource(Res.string.photos_hint),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun PhotoRow(
    index: Int,
    photo: Photo,
    state: ItemFormUiState,
    actions: ItemFormActions,
) {
    val number = stringResource(Res.string.photo_number, index + 1)
    val makeCover = stringResource(Res.string.photo_make_cover)
    val remove = stringResource(Res.string.photo_remove)
    OutlinedCard(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(number, style = MaterialTheme.typography.titleMedium)
                if (index ==
                    0
                ) {
                    Text(stringResource(Res.string.photo_cover_mark), color = MaterialTheme.colorScheme.primary)
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (index > 0) {
                    SecondaryButton(
                        text = makeCover,
                        onClick = { actions.onMakeCover(photo) },
                        modifier = Modifier.semantics { contentDescription = "$makeCover: $number" },
                        enabled = !state.photoBusy,
                    )
                }
                SecondaryButton(
                    text = remove,
                    onClick = { actions.onRemovePhoto(photo) },
                    modifier = Modifier.semantics { contentDescription = "$remove: $number" },
                    enabled = !state.photoBusy,
                )
            }
        }
    }
}

/** "Adicionar fotos", which asks where the photo comes from: the gallery or the camera. */
@Composable
private fun AddPhotoButton(
    enabled: Boolean,
    onPick: (PhotoSource) -> Unit,
) {
    var open by remember { mutableStateOf(false) }
    Box {
        PrimaryButton(stringResource(Res.string.photos_add), { open = true }, enabled = enabled)
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            DropdownMenuItem(
                text = { Text(stringResource(Res.string.photos_gallery), style = MaterialTheme.typography.bodyLarge) },
                onClick = {
                    open = false
                    onPick(PhotoSource.Gallery)
                },
                modifier = Modifier.heightIn(min = TOUCH_TARGET),
            )
            DropdownMenuItem(
                text = { Text(stringResource(Res.string.photos_camera), style = MaterialTheme.typography.bodyLarge) },
                onClick = {
                    open = false
                    onPick(PhotoSource.Camera)
                },
                modifier = Modifier.heightIn(min = TOUCH_TARGET),
            )
        }
    }
}
