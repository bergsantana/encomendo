package br.com.encomendo.app;

import static org.junit.Assert.assertArrayEquals;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

public class PhotoChoiceTest {

    @Test
    public void theAdminPhotoInputAsksGalleryOrCamera() {
        // web/apps/admin/src/features/catalogo/components/PhotosEditor.vue
        PhotoChoice choice = PhotoChoice.forInput(new String[] { "image/jpeg", "image/png", "image/webp" }, false);
        assertEquals(PhotoChoice.Kind.ASK, choice.kind());
        assertArrayEquals(new String[] { "image/jpeg", "image/png", "image/webp" }, choice.galleryTypes());
    }

    @Test
    public void anyImageAsksGalleryOrCamera() {
        assertEquals(PhotoChoice.Kind.ASK, PhotoChoice.forInput(new String[] { "image/*" }, false).kind());
        assertEquals(PhotoChoice.Kind.ASK, PhotoChoice.forInput(new String[] { " IMAGE/JPEG " }, false).kind());
    }

    @Test
    public void captureOpensTheCameraDirectly() {
        assertEquals(PhotoChoice.Kind.CAMERA, PhotoChoice.forInput(new String[] { "image/*" }, true).kind());
    }

    @Test
    public void imagesWithoutJpegOfferOnlyTheGallery() {
        assertEquals(PhotoChoice.Kind.GALLERY, PhotoChoice.forInput(new String[] { "image/png" }, false).kind());
        assertEquals(PhotoChoice.Kind.GALLERY, PhotoChoice.forInput(new String[] { "image/png" }, true).kind());
    }

    @Test
    public void onlyAnotherAppsContentGoesToThePage() {
        String own = "br.com.encomendo.app.fileprovider";
        assertTrue(PhotoChoice.acceptsPicked("content", "com.android.providers.media.documents", own));
        assertFalse(PhotoChoice.acceptsPicked("content", own, own));
        assertFalse(PhotoChoice.acceptsPicked("file", "", own));
        assertFalse(PhotoChoice.acceptsPicked("content", null, own));
        assertFalse(PhotoChoice.acceptsPicked("content", "", own));
        assertFalse(PhotoChoice.acceptsPicked(null, "com.android.providers.media.documents", own));
    }

    @Test
    public void otherInputsAreLeftToCapacitor() {
        assertEquals(PhotoChoice.Kind.NOT_PHOTOS, PhotoChoice.forInput(new String[] {}, false).kind());
        assertEquals(PhotoChoice.Kind.NOT_PHOTOS, PhotoChoice.forInput(new String[] { "" }, false).kind());
        assertEquals(PhotoChoice.Kind.NOT_PHOTOS, PhotoChoice.forInput(null, false).kind());
        assertEquals(PhotoChoice.Kind.NOT_PHOTOS, PhotoChoice.forInput(new String[] { "application/pdf" }, false).kind());
        assertEquals(PhotoChoice.Kind.NOT_PHOTOS, PhotoChoice.forInput(new String[] { "image/*", ".pdf" }, false).kind());
        assertEquals(PhotoChoice.Kind.NOT_PHOTOS, PhotoChoice.forInput(new String[] { "video/*" }, true).kind());
    }
}
