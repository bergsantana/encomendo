package br.com.encomendo.app;

/**
 * WebViewSupport decides whether the phone's WebView can run the admin safely. It guarantees the
 * admin is opened only where Capacitor's bridge is bound to the admin origin: with
 * {@code WEB_MESSAGE_LISTENER} and {@code DOCUMENT_START_SCRIPT} missing, or with the legacy bridge,
 * Capacitor falls back to a JavaScript interface that every frame can call. Pure Java, so the JVM
 * unit tests cover it.
 */
final class WebViewSupport {

    private WebViewSupport() {}

    /** Facts are what MainActivity reads from the phone and the synced config. */
    record Facts(String webViewVersionName, int minMajorVersion, boolean messageListener, boolean documentStartScript, boolean legacyBridge) {}

    /** supported is true only when every condition holds; an unknown WebView version fails. */
    static boolean supported(Facts facts) {
        int major = majorVersion(facts.webViewVersionName());
        return (
            major >= facts.minMajorVersion() &&
            facts.messageListener() &&
            facts.documentStartScript() &&
            !facts.legacyBridge()
        );
    }

    /** majorVersion reads "120.0.6099.230" as 120, and anything it cannot read as -1. */
    static int majorVersion(String versionName) {
        if (versionName == null) {
            return -1;
        }
        int end = 0;
        while (end < versionName.length() && Character.isDigit(versionName.charAt(end))) {
            end++;
        }
        if (end == 0 || end > 6) {
            return -1;
        }
        return Integer.parseInt(versionName.substring(0, end));
    }
}
