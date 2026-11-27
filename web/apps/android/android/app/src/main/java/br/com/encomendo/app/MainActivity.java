package br.com.encomendo.app;

import android.content.pm.ApplicationInfo;
import android.os.Bundle;
import android.webkit.WebSettings;
import android.webkit.WebView;
import androidx.activity.OnBackPressedCallback;
import com.getcapacitor.BridgeActivity;

/**
 * MainActivity is the whole shell: one WebView on the hosted admin (ADR 0029). On top of Capacitor's
 * bridge it guarantees that:
 * - navigation stays on the admin origin, and other links open outside the app (LockedWebViewClient);
 * - photo inputs offer "Da galeria" or "Tirar foto" (PhotoChooser);
 * - the WebView reads no file: or content: URL, refuses mixed content, and can be debugged only in a
 *   debuggable (debug) build;
 * - Back goes back in the admin's history, through @capacitor/app, and leaves the app when there is none.
 */
public class MainActivity extends BridgeActivity {

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        if (bridge == null) {
            // BridgeActivity showed its own "no WebView" screen.
            return;
        }
        WebView webView = bridge.getWebView();
        harden(webView.getSettings());
        // Capacitor sets this from capacitor.config.json; a debuggable flag in the APK is the final word.
        WebView.setWebContentsDebuggingEnabled(isDebuggable());

        // The offline page is served on Capacitor's local origin (https://localhost), not the admin's.
        NavigationPolicy policy = new NavigationPolicy(bridge.getAppUrl(), bridge.getScheme() + "://" + bridge.getHost());
        bridge.setWebViewClient(new LockedWebViewClient(bridge, policy));
        webView.setWebChromeClient(new PhotoChooser(bridge));
        PhotoChooser.clearPhotos(this);
        leaveAppAtRoot(webView);
    }

    private static void harden(WebSettings settings) {
        // The admin is a web page on its own origin; it never reads device files or other apps' content.
        // Photos still reach it: the chooser hands the WebView their content URIs directly.
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(false);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
    }

    private boolean isDebuggable() {
        return (getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0;
    }

    /**
     * leaveAppAtRoot runs before @capacitor/app's own Back handler, which goes back in the WebView or
     * tells the page, but does nothing when there is no history. Here, with history, Back is handed to
     * @capacitor/app; without it, the app moves to the background as Android's own Back does, keeping
     * the session for when the user returns.
     */
    private void leaveAppAtRoot(WebView webView) {
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                if (!webView.canGoBack()) {
                    moveTaskToBack(true);
                    return;
                }
                setEnabled(false);
                getOnBackPressedDispatcher().onBackPressed();
                setEnabled(true);
            }
        });
    }
}
