package br.com.encomendo.app;

import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Bundle;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.widget.Button;
import android.widget.TextView;
import androidx.activity.OnBackPressedCallback;
import androidx.webkit.WebViewFeature;
import com.getcapacitor.BridgeActivity;
import com.getcapacitor.CapConfig;
import com.getcapacitor.Logger;
import com.getcapacitor.PluginHandle;
import java.util.List;

/**
 * MainActivity is the whole shell: one WebView on the hosted admin (ADR 0029). On top of Capacitor's
 * bridge it guarantees that:
 * - the admin opens only on a WebView that binds the bridge to the admin origin (WebViewSupport);
 *   otherwise the phone is asked to update its WebView;
 * - the page cannot reach Capacitor's native HTTP, cookie or server-path plugins (NativeBridgeLockdown),
 *   and the app closes before the admin runs if any stub failed to take a core plugin's place;
 * - navigation and frames stay on the admin origin, and tapped links open outside the app
 *   (LockedWebViewClient);
 * - photo inputs offer "Da galeria" or "Tirar foto" (PhotoChooser);
 * - the WebView reads no file: or content: URL, refuses mixed content, and can be debugged only in a
 *   debuggable (debug) build;
 * - Back goes back in the admin's history, through @capacitor/app, and leaves the app when there is none.
 */
public class MainActivity extends BridgeActivity {

    private static final String DEFAULT_WEBVIEW_PACKAGE = "com.google.android.webview";

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        // Registered before the bridge is built, so the stubs replace the core plugins by name.
        for (Class<? extends com.getcapacitor.Plugin> stub : NativeBridgeLockdown.STUBS) {
            registerPlugin(stub);
        }
        super.onCreate(savedInstanceState);
        if (bridge == null || isFinishing()) {
            // BridgeActivity showed its own "no WebView" screen, load() showed the update page, or the
            // native bridge lockdown failed and the app is closing.
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

    /** load builds the bridge, which loads the admin, only on a WebView that can lock the bridge down. */
    @Override
    protected void load() {
        CapConfig config = CapConfig.loadDefault(this);
        PackageInfo webViewPackage = WebView.getCurrentWebViewPackage();
        WebViewSupport.Facts facts = new WebViewSupport.Facts(
            webViewPackage == null ? null : webViewPackage.versionName,
            config.getMinWebViewVersion(),
            WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER),
            WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT),
            config.isUsingLegacyBridge()
        );
        if (!WebViewSupport.supported(facts)) {
            Logger.warn("WebView cannot lock the bridge down: " + facts);
            showUpdateWebView(webViewPackage);
            return;
        }
        super.load();
        refuseUnlessLockedDown();
    }

    /**
     * refuseUnlessLockedDown closes the app if any core plugin survived its stub. The bridge has asked
     * the WebView to load the admin by now, but nothing has arrived: this runs in the same main-thread
     * task, so stopping here means no admin script ever runs beside a live native cookie jar.
     */
    private void refuseUnlessLockedDown() {
        List<String> missing = NativeBridgeLockdown.notInPlace((name) -> {
            PluginHandle handle = bridge.getPlugin(name);
            return handle == null ? null : handle.getInstance();
        });
        if (missing.isEmpty()) {
            return;
        }
        Logger.error("Native bridge lockdown failed; core plugins still live: " + missing);
        WebView webView = bridge.getWebView();
        webView.stopLoading();
        webView.loadUrl("about:blank");
        finish();
    }

    private void showUpdateWebView(PackageInfo webViewPackage) {
        String packageName = webViewPackage == null ? DEFAULT_WEBVIEW_PACKAGE : webViewPackage.packageName;
        setContentView(R.layout.webview_update);
        TextView message = findViewById(R.id.webview_update_message);
        message.setText(getString(R.string.webview_update_message, webViewLabel(packageName)));
        Button action = findViewById(R.id.webview_update_action);
        action.setOnClickListener((view) -> {
            Intent intent = new Intent(Intent.ACTION_VIEW, Uri.parse("https://play.google.com/store/apps/details?id=" + packageName));
            intent.addCategory(Intent.CATEGORY_BROWSABLE);
            try {
                startActivity(intent);
            } catch (ActivityNotFoundException e) {
                Logger.warn("No app opens the Play Store link");
            }
        });
    }

    private String webViewLabel(String packageName) {
        try {
            PackageManager packages = getPackageManager();
            return packages.getApplicationLabel(packages.getApplicationInfo(packageName, 0)).toString();
        } catch (PackageManager.NameNotFoundException e) {
            return "Android System WebView";
        }
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
