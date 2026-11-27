package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.domain.catalog.Photo
import br.com.encomendo.core.ui.PhotoImage
import br.com.encomendo.core.ui.TouchTarget
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
import br.com.encomendo.feature.catalogo.resources.photos_uploading
import org.jetbrains.compose.resources.stringResource

/**
 * "Fotos": the photos with a thumbnail, "Usar como capa" and "Remover foto"; they change at once, not with "Salvar".
 * A photo belongs to a saved item or kit, so before it is saved the section only says to save it first.
 */
@Composable
internal fun PhotosEditor(
    photos: PhotosState,
    actions: PhotoActions,
) {
    SectionTitle(stringResource(Res.string.photos_title))
    if (photos.ownerId == null) {
        // The upload names the owner's id.
        Text(stringResource(Res.string.photos_save_first), style = MaterialTheme.typography.bodyLarge)
        return
    }
    photos.message?.let { MessageText(message = it) }
    photos.photos.forEachIndexed { index, photo -> PhotoRow(index, photo, photos, actions) }
    if (photos.uploading) {
        Text(
            text = stringResource(Res.string.photos_uploading),
            style = MaterialTheme.typography.bodyLarge,
            modifier = Modifier.announced(),
        )
    }
    AddPhotoButton(enabled = photos.canChange, onPick = actions.onAdd)
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
    photos: PhotosState,
    actions: PhotoActions,
) {
    val number = stringResource(Res.string.photo_number, index + 1)
    val makeCover = stringResource(Res.string.photo_make_cover)
    val remove = stringResource(Res.string.photo_remove)
    val isCover = index == 0
    OutlinedCard(modifier = Modifier.fillMaxWidth()) {
        Row(modifier = Modifier.padding(12.dp), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            PhotoImage(
                url = photo.url,
                contentDescription = number,
                modifier = Modifier.size(THUMBNAIL).clip(MaterialTheme.shapes.small),
            )
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(number, style = MaterialTheme.typography.titleMedium)
                    if (isCover) {
                        Text(
                            stringResource(Res.string.photo_cover_mark),
                            color = MaterialTheme.colorScheme.primary,
                        )
                    }
                }
                if (!isCover) {
                    SecondaryButton(
                        text = makeCover,
                        onClick = { actions.onMakeCover(photo) },
                        modifier = Modifier.semantics { contentDescription = "$makeCover: $number" },
                        enabled = photos.canChange,
                    )
                }
                SecondaryButton(
                    text = remove,
                    onClick = { actions.onRemove(photo) },
                    modifier = Modifier.semantics { contentDescription = "$remove: $number" },
                    enabled = photos.canChange,
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
            PhotoSourceItem(stringResource(Res.string.photos_gallery)) {
                open = false
                onPick(PhotoSource.Gallery)
            }
            PhotoSourceItem(stringResource(Res.string.photos_camera)) {
                open = false
                onPick(PhotoSource.Camera)
            }
        }
    }
}

@Composable
private fun PhotoSourceItem(
    label: String,
    onClick: () -> Unit,
) {
    DropdownMenuItem(
        text = { Text(label, style = MaterialTheme.typography.bodyLarge) },
        onClick = onClick,
        modifier = Modifier.heightIn(min = TouchTarget),
    )
}

private val THUMBNAIL = 96.dp
