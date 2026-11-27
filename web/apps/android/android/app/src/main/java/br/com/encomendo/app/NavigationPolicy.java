package br.com.encomendo.app;

import java.net.URI;
import java.net.URISyntaxException;
import java.util.Locale;
import java.util.Set;

/**
 * NavigationPolicy decides where a navigation inside the WebView may go. It guarantees that only the
 * admin origin of this build (scheme, host and port of {@code server.url}) loads in the app: the
 * hosted admin carries the session and Capacitor's native bridge, so no other page may run there.
 *
 * <p>Product rule: WhatsApp ({@code wa.me}, {@code api.whatsapp.com}), the store pages
 * ({@code *.encomendo.com.br}) and every other web page open outside the app (ADR 0029). Pure Java,
 * with no Android types, so the JVM unit tests cover every decision.
 */
final class NavigationPolicy {

    /** Decision is what the WebView client does with one navigation. */
    enum Decision {
        /** STAY loads the URL in the app; only the admin origin gets it. */
        STAY,
        /** OPEN_OUTSIDE hands the URL to WhatsApp, the browser or the dialer, never the WebView. */
        OPEN_OUTSIDE,
        /** RETRY is the offline page's "Tentar de novo": load the admin URL again. */
        RETRY,
        /** BLOCK drops the navigation. */
        BLOCK,
    }

    /** RETRY_PATH is the offline page's "Tentar de novo" link (www/index.html), on the shell's local origin. */
    static final String RETRY_PATH = "/retry";

    // Schemes another app may receive. Others (intent:, file:, content:, javascript:, …) could reach
    // into this or another app's internals, so they are dropped.
    private static final Set<String> OUTSIDE_SCHEMES = Set.of("https", "http", "mailto", "tel");

    private final Origin admin;
    private final Origin local;

    /**
     * @param adminUrl the build's {@code server.url}, the only origin allowed in the app
     * @param localUrl the origin Capacitor serves the bundled offline page from ({@code https://localhost})
     */
    NavigationPolicy(String adminUrl, String localUrl) {
        this.admin = Origin.parse(adminUrl);
        this.local = Origin.parse(localUrl);
        if (admin == null || local == null) {
            throw new IllegalArgumentException("NavigationPolicy needs absolute http(s) URLs");
        }
    }

    /** decide returns the decision for a navigation to {@code url}, in the main frame or in a frame. */
    Decision decide(String url, boolean mainFrame) {
        URI uri = parse(url);
        if (uri == null || uri.getScheme() == null) {
            return Decision.BLOCK;
        }
        String scheme = uri.getScheme().toLowerCase(Locale.ROOT);
        if (scheme.equals("blob")) {
            // A blob URL carries the origin that made it: only the admin's own stay.
            return admin.equals(Origin.parse(uri.getSchemeSpecificPart())) ? Decision.STAY : Decision.BLOCK;
        }
        Origin target = Origin.of(uri);
        if (admin.equals(target)) {
            return Decision.STAY;
        }
        if (local.equals(target)) {
            return mainFrame && RETRY_PATH.equals(uri.getPath()) ? Decision.RETRY : Decision.BLOCK;
        }
        // A frame never launches another app; only a tap on a link in the page itself does.
        if (!mainFrame || !OUTSIDE_SCHEMES.contains(scheme)) {
            return Decision.BLOCK;
        }
        return Decision.OPEN_OUTSIDE;
    }

    private static URI parse(String url) {
        if (url == null) {
            return null;
        }
        try {
            return new URI(url);
        } catch (URISyntaxException e) {
            return null;
        }
    }

    /** Origin is a scheme, host and effective port, compared without case on scheme and host. */
    private record Origin(String scheme, String host, int port) {
        static Origin parse(String url) {
            URI uri = NavigationPolicy.parse(url);
            return uri == null ? null : of(uri);
        }

        static Origin of(URI uri) {
            if (uri.getScheme() == null || uri.getHost() == null) {
                return null;
            }
            String scheme = uri.getScheme().toLowerCase(Locale.ROOT);
            if (!scheme.equals("https") && !scheme.equals("http")) {
                return null;
            }
            int port = uri.getPort() != -1 ? uri.getPort() : scheme.equals("https") ? 443 : 80;
            return new Origin(scheme, uri.getHost().toLowerCase(Locale.ROOT), port);
        }
    }
}
