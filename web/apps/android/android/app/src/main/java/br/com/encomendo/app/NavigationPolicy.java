package br.com.encomendo.app;

import com.getcapacitor.Bridge;
import java.net.URI;
import java.net.URISyntaxException;
import java.io.UnsupportedEncodingException;
import java.net.URLDecoder;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;

/**
 * NavigationPolicy decides where a navigation inside the WebView may go, and which requests the
 * WebView may not load at all. It guarantees that only the admin origin of this build (scheme, host
 * and port of {@code server.url}) loads in the app, in the page or in a frame: the hosted admin
 * carries the session and Capacitor's native bridge, so no other page may run there.
 *
 * <p>Product rule: WhatsApp ({@code wa.me}, {@code api.whatsapp.com}), the store pages
 * ({@code *.encomendo.com.br}) and every other web page open outside the app (ADR 0029), and only when
 * the user tapped a link (or a tapped link redirected). Pure Java, with no Android types, so the JVM
 * unit tests cover every decision.
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

    // Schemes another app may receive. Others (intent:, file:, content:, …) could reach into this or
    // another app's internals, so they are dropped. (javascript: links never get here: they run in
    // the page itself, which the shell cannot stop.)
    private static final Set<String> OUTSIDE_SCHEMES = Set.of("https", "http", "mailto", "tel");

    // Capacitor's proxy routes read the app's own files and content URIs for whatever page asks, and
    // the WebView's cookie database is one of those files. The admin never uses them.
    private static final List<String> INTERNAL_ROUTES = List.of(
        Bridge.CAPACITOR_FILE_START,
        Bridge.CAPACITOR_CONTENT_START,
        Bridge.CAPACITOR_HTTP_INTERCEPTOR_START,
        Bridge.CAPACITOR_HTTPS_INTERCEPTOR_START
    );

    // Sec-Fetch-Dest values of a page or frame load; a fetch, an image or a script is none of them.
    private static final Set<String> DOCUMENT_DESTINATIONS = Set.of("document", "iframe", "frame", "embed", "object");

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

    /**
     * decide returns the decision for a navigation to {@code url}, in the main frame or in a frame.
     *
     * @param userInitiated the WebView's "has a gesture, or is a redirect"; without it a page could
     *     open WhatsApp or the browser on its own
     */
    Decision decide(String url, boolean mainFrame, boolean userInitiated) {
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
        if (!mainFrame || !userInitiated || !OUTSIDE_SCHEMES.contains(scheme)) {
            return Decision.BLOCK;
        }
        return Decision.OPEN_OUTSIDE;
    }

    /**
     * refuseRequest says whether the WebView must not load a request at all. Android asks
     * shouldOverrideUrlLoading about frames only for non-http(s) schemes, so an http(s) frame of
     * another site is refused here, where every request passes. Capacitor's internal file and
     * content routes are refused at every origin.
     *
     * @param headers the request headers, which tell a frame load from a fetch or an image
     */
    boolean refuseRequest(String url, boolean mainFrame, Map<String, String> headers) {
        if (url == null) {
            return true;
        }
        String path = decodedPath(rawPath(url));
        if (path == null || INTERNAL_ROUTES.stream().anyMatch(path::startsWith)) {
            return true;
        }
        if (mainFrame || !isDocument(headers)) {
            return false;
        }
        // A frame whose URL java.net.URI cannot read is refused too: frames fail closed. A fetch or an
        // image with such a URL (a "|" in a query, say) still loads.
        URI uri = parse(url);
        return uri == null || !admin.equals(Origin.of(uri));
    }

    /**
     * decodedPath undoes percent-encoding, as Capacitor's Uri.getPath() does before it matches its
     * routes, so "/%5Fcapacitor_file_/" is caught too. It returns null for a broken escape.
     */
    private static String decodedPath(String rawPath) {
        try {
            // The Charset overload needs API 33; the name overload works from minSdk 26.
            return URLDecoder.decode(rawPath.replace("+", "%2B"), "UTF-8");
        } catch (IllegalArgumentException | UnsupportedEncodingException e) {
            return null;
        }
    }

    /** rawPath is the path of an absolute URL, read as text so that any URL the WebView sends has one. */
    private static String rawPath(String url) {
        int start = url.indexOf("://");
        if (start < 0) {
            return "";
        }
        int slash = url.indexOf('/', start + 3);
        if (slash < 0) {
            return "";
        }
        int end = url.length();
        for (char stop : new char[] { '?', '#' }) {
            int at = url.indexOf(stop, slash);
            if (at >= 0 && at < end) {
                end = at;
            }
        }
        return url.substring(slash, end);
    }

    private static boolean isDocument(Map<String, String> headers) {
        if (headers == null) {
            return false;
        }
        for (Map.Entry<String, String> header : headers.entrySet()) {
            String name = header.getKey() == null ? "" : header.getKey();
            String value = header.getValue() == null ? "" : header.getValue().trim().toLowerCase(Locale.ROOT);
            // Chromium sends Upgrade-Insecure-Requests on navigations only, frames included.
            if (name.equalsIgnoreCase("Upgrade-Insecure-Requests")) {
                return true;
            }
            if (name.equalsIgnoreCase("Sec-Fetch-Dest") && DOCUMENT_DESTINATIONS.contains(value)) {
                return true;
            }
        }
        return false;
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
