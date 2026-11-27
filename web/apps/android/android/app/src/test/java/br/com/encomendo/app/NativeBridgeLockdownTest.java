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
import java.util.HashMap;
import java.util.List;
import java.util.Map;
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
    public void theLockdownIsInPlaceWhenEveryNameHoldsItsStub() {
        Map<String, Object> plugins = Map.of(
            "CapacitorHttp", new NativeBridgeLockdown.NoNativeHttp(),
            "CapacitorCookies", new NativeBridgeLockdown.NoNativeCookies(),
            "WebView", new NativeBridgeLockdown.NoServerPath(),
            "App", new Object()
        );

        assertEquals(List.of(), NativeBridgeLockdown.notInPlace(plugins::get));
    }

    @Test
    public void aCorePluginThatSurvivedItsStubIsReported() {
        // Bridge.registerPlugin only logs a stub that failed to load, leaving the core plugin's handle.
        Map<String, Object> plugins = new HashMap<>();
        plugins.put("CapacitorHttp", new Object());
        plugins.put("CapacitorCookies", new NativeBridgeLockdown.NoNativeCookies());

        assertEquals(List.of("CapacitorHttp", "WebView"), NativeBridgeLockdown.notInPlace(plugins::get));
    }

    @Test
    public void aStubUnderAnotherStubsNameDoesNotCount() {
        Map<String, Object> plugins = Map.of(
            "CapacitorHttp", new NativeBridgeLockdown.NoNativeCookies(),
            "CapacitorCookies", new NativeBridgeLockdown.NoNativeHttp(),
            "WebView", new NativeBridgeLockdown.NoServerPath()
        );

        assertEquals(List.of("CapacitorHttp", "CapacitorCookies"), NativeBridgeLockdown.notInPlace(plugins::get));
    }

    @Test
    public void bothJavaScriptInterfacesAreRemoved() {
        assertEquals(List.of("CapacitorCookiesAndroidInterface", "CapacitorHttpAndroidInterface"), NativeBridgeLockdown.INTERFACES);
    }
}
