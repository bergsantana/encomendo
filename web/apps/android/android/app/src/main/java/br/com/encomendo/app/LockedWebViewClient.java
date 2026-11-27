package br.com.encomendo.app;

import android.content.ActivityNotFoundException;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebView;
import com.getcapacitor.Bridge;
import com.getcapacitor.BridgeWebViewClient;
import com.getcapacitor.Logger;
import java.io.ByteArrayInputStream;
import java.util.Collections;

/**
 * LockedWebViewClient keeps the WebView on the admin origin. Every navigation goes through
 * {@link NavigationPolicy}: the admin stays (and still passes Capacitor's own checks), WhatsApp, store
 * pages and other tapped links open in another app, and anything else is dropped. Every request goes
 * through it too, so frames of other sites and Capacitor's file routes never load. Capacitor's
 * default would let any {@code server.allowNavigation} host in and hand any scheme to
 * {@code ACTION_VIEW}.
 *
 * <p>It also keeps the offline page out of the back stack, so Back from it, or from the admin after
 * "Tentar de novo", leaves the app instead of returning to a failed page.
 */
final class LockedWebViewClient extends BridgeWebViewClient {

    private final Bridge bridge;
    private final NavigationPolicy policy;
    private boolean clearHistoryOnNextAdminPage;

    LockedWebViewClient(Bridge bridge, NavigationPolicy policy) {
        super(bridge);
        this.bridge = bridge;
        this.policy = policy;
    }

    @Override
    public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
        Uri url = request.getUrl();
        boolean userInitiated = request.hasGesture() || request.isRedirect();
        switch (policy.decide(url.toString(), request.isForMainFrame(), userInitiated)) {
            case STAY:
                // Capacitor still refuses its internal proxy routes as documents.
                return super.shouldOverrideUrlLoading(view, request);
            case OPEN_OUTSIDE:
                openOutside(view.getContext(), url);
                return true;
            case RETRY:
                clearHistoryOnNextAdminPage = true;
                view.loadUrl(bridge.getAppUrl());
                return true;
            case BLOCK:
            default:
                Logger.warn("Navigation blocked: " + url.getScheme() + "://" + url.getHost());
                return true;
        }
    }

    @Override
    public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
        if (policy.refuseRequest(request.getUrl().toString(), request.isForMainFrame(), request.getRequestHeaders())) {
            Logger.warn("Request refused: " + request.getUrl().getScheme() + "://" + request.getUrl().getHost());
            return refused();
        }
        return super.shouldInterceptRequest(view, request);
    }

    @Override
    public void onPageFinished(WebView view, String url) {
        super.onPageFinished(view, url);
        String errorUrl = bridge.getErrorUrl();
        boolean offlinePage = errorUrl != null && errorUrl.equals(url);
        boolean adminAfterRetry = clearHistoryOnNextAdminPage && policy.decide(url, true, false) == NavigationPolicy.Decision.STAY;
        if (offlinePage || adminAfterRetry) {
            clearHistoryOnNextAdminPage = false;
            view.clearHistory();
        }
    }

    private static WebResourceResponse refused() {
        return new WebResourceResponse(
            "text/plain",
            "utf-8",
            403,
            "Forbidden",
            Collections.emptyMap(),
            new ByteArrayInputStream(new byte[0])
        );
    }

    private static void openOutside(Context context, Uri url) {
        Intent intent = new Intent(Intent.ACTION_VIEW, url);
        // Only activities that declare themselves safe for links from the web may receive it.
        intent.addCategory(Intent.CATEGORY_BROWSABLE);
        try {
            context.startActivity(intent);
        } catch (ActivityNotFoundException e) {
            Logger.warn("No app opens " + url.getScheme() + " links");
        }
    }
}
