package br.com.encomendo.core.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.runtime.Composable
import androidx.compose.runtime.ProvidableCompositionLocal
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.ContentScale
import coil3.ImageLoader
import coil3.compose.AsyncImage

/**
 * LocalPhotoLoader is the image loader of photo thumbnails. The app root provides one whose HTTP client has no Auth
 * plugin, so the session's bearer token never reaches the storage host that signed photo URLs point at (ADR 0024,
 * ADR 0026). Until it does, [PhotoImage] draws nothing: it never falls back to a loader nobody checked.
 */
val LocalPhotoLoader: ProvidableCompositionLocal<ImageLoader?> = staticCompositionLocalOf { null }

/**
 * PhotoImage shows the photo at the signed [url] filling [modifier], cropped to fit. [contentDescription] is read
 * out for the picture; null marks it decorative, as when the row beside it already says what it is.
 */
@Composable
fun PhotoImage(
    url: String,
    contentDescription: String?,
    modifier: Modifier = Modifier,
) {
    val loader = LocalPhotoLoader.current
    if (loader == null) {
        Box(modifier)
    } else {
        AsyncImage(
            model = url,
            contentDescription = contentDescription,
            imageLoader = loader,
            contentScale = ContentScale.Crop,
            modifier = modifier,
        )
    }
}
