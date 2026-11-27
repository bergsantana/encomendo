package br.com.encomendo.app;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/**
 * PhotoChoice decides how the shell answers a page's file input, from its {@code accept} list and
 * its {@code capture} attribute. It guarantees that an images-only input gets the product's choice of
 * "Da galeria" or "Tirar foto", and that the camera is offered only when the input accepts the JPEG it
 * produces. Pure Java, so the JVM unit tests cover it.
 *
 * <p>Product rule: on the phone, "Adicionar fotos" asks "Da galeria" or "Tirar foto"
 * (docs/product/design-document.md, Fotos).
 */
record PhotoChoice(Kind kind, String[] galleryTypes) {

    /** Kind is what the shell shows for one file input. */
    enum Kind {
        /** NOT_PHOTOS leaves the input to Capacitor's own file picker. */
        NOT_PHOTOS,
        /** ASK offers "Da galeria" or "Tirar foto". */
        ASK,
        /** CAMERA opens the camera directly, as the page's {@code capture} attribute asks. */
        CAMERA,
        /** GALLERY opens the picker only: the input does not take JPEG, so the camera would not fit. */
        GALLERY,
    }

    /** forInput reads a file input's accept types and capture flag. */
    static PhotoChoice forInput(String[] acceptTypes, boolean capture) {
        List<String> types = new ArrayList<>();
        for (String raw : acceptTypes == null ? new String[0] : acceptTypes) {
            String type = raw == null ? "" : raw.trim().toLowerCase(Locale.ROOT);
            if (!type.isEmpty()) {
                types.add(type);
            }
        }
        // An empty accept list means any file; anything that is not an image MIME type is a document.
        if (types.isEmpty() || !types.stream().allMatch(t -> t.startsWith("image/"))) {
            return new PhotoChoice(Kind.NOT_PHOTOS, new String[0]);
        }
        boolean takesJpeg = types.contains("image/*") || types.contains("image/jpeg") || types.contains("image/jpg");
        Kind kind = !takesJpeg ? Kind.GALLERY : capture ? Kind.CAMERA : Kind.ASK;
        return new PhotoChoice(kind, types.toArray(new String[0]));
    }

    /**
     * acceptsPicked says whether a URI the gallery returned may go to the page. Only another app's
     * {@code content:} URIs do: a {@code file:} URI, or one of this app's own FileProvider (whose
     * files the picker app could name to make the page upload them), is dropped.
     */
    static boolean acceptsPicked(String scheme, String authority, String ownFileProvider) {
        return "content".equals(scheme) && authority != null && !authority.isEmpty() && !authority.equals(ownFileProvider);
    }
}
