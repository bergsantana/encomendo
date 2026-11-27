package br.com.encomendo.app;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.provider.MediaStore;
import android.webkit.ValueCallback;
import android.webkit.WebView;
import androidx.activity.result.ActivityResult;
import androidx.activity.result.ActivityResultLauncher;
import androidx.activity.result.contract.ActivityResultContracts;
import androidx.appcompat.app.AlertDialog;
import androidx.core.content.FileProvider;
import com.getcapacitor.Bridge;
import com.getcapacitor.BridgeWebChromeClient;
import com.getcapacitor.Logger;
import java.io.File;
import java.io.IOException;
import java.util.ArrayList;
import java.util.List;

/**
 * PhotoChooser answers the admin's photo inputs ({@code <input type="file" accept="image/…">}) with
 * the product's choice, "Da galeria" or "Tirar foto". Capacitor alone offers only the file picker
 * unless the page sets {@code capture}. Every other input is left to Capacitor.
 *
 * <p>It guarantees the page's callback is answered exactly once per input, and that the camera app
 * gets access to one new file in {@code cache/photos} and nothing else (xml/file_paths).
 */
final class PhotoChooser extends BridgeWebChromeClient {

    private static final String PHOTO_DIR = "photos";

    private interface ResultHandler {
        void handle(ActivityResult result);
    }

    private final Bridge bridge;
    private final ActivityResultLauncher<Intent> launcher;
    private ResultHandler pending;

    /** PhotoChooser must be created in the activity's onCreate, as it registers for activity results. */
    PhotoChooser(Bridge bridge) {
        super(bridge);
        this.bridge = bridge;
        this.launcher = bridge.registerForActivityResult(new ActivityResultContracts.StartActivityForResult(), (result) -> {
            ResultHandler handler = pending;
            pending = null;
            if (handler != null) {
                handler.handle(result);
            }
        });
    }

    /** clearPhotos deletes the camera files of earlier sessions; the page has read or dropped them by now. */
    static void clearPhotos(Context context) {
        File[] files = new File(context.getCacheDir(), PHOTO_DIR).listFiles();
        if (files == null) {
            return;
        }
        for (File file : files) {
            if (!file.delete()) {
                Logger.warn("Could not delete an old photo");
            }
        }
    }

    @Override
    public boolean onShowFileChooser(WebView webView, ValueCallback<Uri[]> callback, FileChooserParams params) {
        PhotoChoice choice = PhotoChoice.forInput(params.getAcceptTypes(), params.isCaptureEnabled());
        if (choice.kind() == PhotoChoice.Kind.NOT_PHOTOS) {
            return super.onShowFileChooser(webView, callback, params);
        }
        boolean multiple = params.getMode() == FileChooserParams.MODE_OPEN_MULTIPLE;
        boolean hasCamera = hasCameraApp(bridge.getActivity());
        if (choice.kind() == PhotoChoice.Kind.GALLERY || !hasCamera) {
            openGallery(callback, choice.galleryTypes(), multiple);
        } else if (choice.kind() == PhotoChoice.Kind.CAMERA) {
            openCamera(callback);
        } else {
            ask(callback, choice.galleryTypes(), multiple);
        }
        return true;
    }

    private void ask(ValueCallback<Uri[]> callback, String[] types, boolean multiple) {
        Activity activity = bridge.getActivity();
        String[] options = { activity.getString(R.string.photo_from_gallery), activity.getString(R.string.photo_from_camera) };
        new AlertDialog.Builder(activity)
            .setTitle(R.string.photo_chooser_title)
            .setItems(options, (dialog, which) -> {
                if (which == 0) {
                    openGallery(callback, types, multiple);
                } else {
                    openCamera(callback);
                }
            })
            // Back or a tap outside: the page hears "nothing chosen" and can open the input again.
            .setOnCancelListener((dialog) -> callback.onReceiveValue(null))
            .show();
    }

    private void openGallery(ValueCallback<Uri[]> callback, String[] types, boolean multiple) {
        Intent intent = new Intent(Intent.ACTION_GET_CONTENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType(types.length == 1 ? types[0] : "image/*");
        if (types.length > 1) {
            intent.putExtra(Intent.EXTRA_MIME_TYPES, types);
        }
        intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, multiple);
        launch(intent, callback, (result) -> callback.onReceiveValue(chosenUris(result)));
    }

    private void openCamera(ValueCallback<Uri[]> callback) {
        Context context = bridge.getContext();
        File file;
        Uri uri;
        try {
            File dir = new File(context.getCacheDir(), PHOTO_DIR);
            if (!dir.isDirectory() && !dir.mkdirs()) {
                throw new IOException("cannot create the photo folder");
            }
            file = File.createTempFile("photo_", ".jpg", dir);
            uri = FileProvider.getUriForFile(context, context.getPackageName() + ".fileprovider", file);
        } catch (IOException | IllegalArgumentException e) {
            Logger.error("No file for the camera: " + e.getMessage());
            callback.onReceiveValue(null);
            return;
        }
        Intent intent = new Intent(MediaStore.ACTION_IMAGE_CAPTURE);
        intent.putExtra(MediaStore.EXTRA_OUTPUT, uri);
        // The grant travels with the ClipData, for this one file only.
        intent.setClipData(ClipData.newRawUri("", uri));
        intent.addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION | Intent.FLAG_GRANT_READ_URI_PERMISSION);
        launch(intent, callback, (result) -> {
            if (result.getResultCode() == Activity.RESULT_OK && file.length() > 0) {
                callback.onReceiveValue(new Uri[] { uri });
                return;
            }
            if (!file.delete()) {
                Logger.warn("Could not delete an unused photo file");
            }
            callback.onReceiveValue(null);
        });
    }

    private void launch(Intent intent, ValueCallback<Uri[]> callback, ResultHandler handler) {
        if (pending != null) {
            // Only one chooser at a time; the WebView never asks for two, but the callback must still be answered.
            callback.onReceiveValue(null);
            return;
        }
        pending = handler;
        try {
            launcher.launch(intent);
        } catch (ActivityNotFoundException e) {
            pending = null;
            callback.onReceiveValue(null);
        }
    }

    private Uri[] chosenUris(ActivityResult result) {
        Intent data = result.getData();
        if (result.getResultCode() != Activity.RESULT_OK || data == null) {
            return null;
        }
        List<Uri> picked = new ArrayList<>();
        ClipData clip = data.getClipData();
        if (clip != null && clip.getItemCount() > 0) {
            for (int i = 0; i < clip.getItemCount(); i++) {
                picked.add(clip.getItemAt(i).getUri());
            }
        } else if (data.getData() != null) {
            picked.add(data.getData());
        }
        String ownFileProvider = bridge.getContext().getPackageName() + ".fileprovider";
        List<Uri> accepted = new ArrayList<>();
        for (Uri uri : picked) {
            if (uri != null && PhotoChoice.acceptsPicked(uri.getScheme(), uri.getAuthority(), ownFileProvider)) {
                accepted.add(uri);
            } else {
                Logger.warn("Dropped a picked file that is not another app's content");
            }
        }
        return accepted.isEmpty() ? null : accepted.toArray(new Uri[0]);
    }

    @SuppressWarnings("QueryPermissionsNeeded") // AndroidManifest.xml declares the IMAGE_CAPTURE query.
    private static boolean hasCameraApp(Activity activity) {
        return new Intent(MediaStore.ACTION_IMAGE_CAPTURE).resolveActivity(activity.getPackageManager()) != null;
    }
}
