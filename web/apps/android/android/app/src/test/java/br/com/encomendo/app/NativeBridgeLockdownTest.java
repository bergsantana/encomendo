package br.com.encomendo.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import com.getcapacitor.Plugin;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.net.CookieHandler;
import java.net.CookieManager;
import java.util.List;
import java.util.stream.Collectors;
import org.junit.After;
import org.junit.Test;

public class NativeBridgeLockdownTest {

    @After
    public void resetCookieHandler() {
        CookieHandler.setDefault(null);
    }

    @Test
    public void nativeHttpLosesTheSharedCookieJar() {
        // What CapacitorCookies.load() does: a CookieManager subclass backed by the WebView's cookies.
        CookieHandler.setDefault(new CookieManager() {});

        NativeBridgeLockdown.isolateNativeCookies();

        assertNull(CookieHandler.getDefault());
    }

    @Test
    public void theStubsTakeTheNamesOfTheCorePluginsThePageMustNotReach() {
        List<String> names = NativeBridgeLockdown.STUBS.stream()
            .map((stub) -> stub.getAnnotation(CapacitorPlugin.class))
            .map((annotation) -> {
                assertNotNull("every stub is a @CapacitorPlugin", annotation);
                return annotation.name();
            })
            .collect(Collectors.toList());

        // Bridge.registerAllPlugins() registers these whatever the config says; SystemBars and the
        // app's own @capacitor/app stay.
        assertEquals(List.of("CapacitorHttp", "CapacitorCookies", "WebView"), names);
    }

    @Test
    public void theStubsOfferThePageNothingToCall() {
        for (Class<?> stub : NativeBridgeLockdown.STUBS) {
            for (Method method : stub.getMethods()) {
                // Every plugin inherits Plugin's listener and permission methods; a stub has no events
                // and asks for no permission, so they do nothing. Nothing else may be callable.
                if (method.getDeclaringClass() == Plugin.class) {
                    continue;
                }
                assertNull(stub.getSimpleName() + "." + method.getName(), method.getAnnotation(PluginMethod.class));
            }
        }
    }

    @Test
    public void capacitorCanCreateTheStubsByReflection() throws ReflectiveOperationException {
        for (Class<?> stub : NativeBridgeLockdown.STUBS) {
            assertTrue(stub.getSimpleName(), Modifier.isPublic(stub.getModifiers()));
            assertNotNull(stub.getConstructor());
        }
    }

    @Test
    public void bothJavaScriptInterfacesAreRemoved() {
        assertEquals(List.of("CapacitorCookiesAndroidInterface", "CapacitorHttpAndroidInterface"), NativeBridgeLockdown.INTERFACES);
    }
}
