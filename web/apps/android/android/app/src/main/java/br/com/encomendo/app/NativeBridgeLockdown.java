package br.com.encomendo.app;

import android.webkit.WebView;
import com.getcapacitor.Plugin;
import com.getcapacitor.annotation.CapacitorPlugin;
import java.net.CookieHandler;
import java.util.ArrayList;
import java.util.List;
import java.util.function.Function;

/**
 * NativeBridgeLockdown takes Capacitor's native HTTP, cookie and server-path plugins away from the
 * hosted admin. It guarantees that page script, an XSS or a bad dependency included, can never:
 * - make a native request that carries the WebView's cookies and read the answer's
 *   {@code Set-Cookie}, which would hand it the HttpOnly refresh cookie (ADR 0010);
 * - read, overwrite or delete cookies through the {@code CapacitorCookiesAndroidInterface} and
 *   {@code CapacitorHttpAndroidInterface} JavaScript interfaces, which Capacitor adds to every frame;
 * - point the local server at another folder of the app's files.
 *
 * <p>Capacitor 8 registers {@code CapacitorCookies}, {@code CapacitorHttp} and {@code WebView} itself,
 * whatever the config says, and {@code CapacitorCookies.load()} shares the WebView cookie jar with
 * every native {@code HttpURLConnection}. So MainActivity registers the stubs below under the same
 * names: the bridge's plugin map then holds a stub with no methods, and a call from the page finds
 * nothing to run. Each stub's {@code load()} runs while the bridge is built, before the first page
 * loads, and undoes what the real plugin's {@code load()} did. Capacitor instantiates them by
 * reflection, so they and this class are public.
 */
public final class NativeBridgeLockdown {

    /** INTERFACES are the JavaScript interfaces Capacitor's cookie and HTTP plugins add to the WebView. */
    static final List<String> INTERFACES = List.of("CapacitorCookiesAndroidInterface", "CapacitorHttpAndroidInterface");

    /** STUBS replace, by plugin name, every core plugin the page must not reach. */
    static final List<Class<? extends Plugin>> STUBS = List.of(NoNativeHttp.class, NoNativeCookies.class, NoServerPath.class);

    private NativeBridgeLockdown() {}

    /** isolateNativeCookies leaves native HTTP with no cookie handler, so it never sees the WebView's cookies. */
    static void isolateNativeCookies() {
        CookieHandler.setDefault(null);
    }

    /** apply removes the interfaces and the shared cookie handler. Removing an interface twice is harmless. */
    static void apply(WebView webView) {
        for (String name : INTERFACES) {
            webView.removeJavascriptInterface(name);
        }
        isolateNativeCookies();
    }

    /**
     * notInPlace returns the names of the stubs the bridge does not hold, given a lookup from plugin
     * name to the instance registered under it (null when none). Capacitor's
     * {@code Bridge.registerPlugin} only logs a plugin that fails to load, so a broken stub would
     * leave the core plugin live without a sound; MainActivity refuses to show the admin unless this
     * list is empty.
     */
    static List<String> notInPlace(Function<String, Object> instanceByName) {
        List<String> missing = new ArrayList<>();
        for (Class<? extends Plugin> stub : STUBS) {
            String name = stub.getAnnotation(CapacitorPlugin.class).name();
            Object instance = instanceByName.apply(name);
            if (instance == null || instance.getClass() != stub) {
                missing.add(name);
            }
        }
        return missing;
    }

    /** LockedPlugin is a plugin with no methods; loading it applies the lockdown. */
    abstract static class LockedPlugin extends Plugin {

        @Override
        public void load() {
            apply(getBridge().getWebView());
        }
    }

    /** NoNativeHttp takes the place of CapacitorHttp. */
    @CapacitorPlugin(name = "CapacitorHttp")
    public static final class NoNativeHttp extends LockedPlugin {}

    /** NoNativeCookies takes the place of CapacitorCookies. */
    @CapacitorPlugin(name = "CapacitorCookies")
    public static final class NoNativeCookies extends LockedPlugin {}

    /** NoServerPath takes the place of the WebView plugin (live updates' setServerBasePath and friends). */
    @CapacitorPlugin(name = "WebView")
    public static final class NoServerPath extends LockedPlugin {}
}
