package br.com.encomendo.app;

import static br.com.encomendo.app.NavigationPolicy.Decision.BLOCK;
import static br.com.encomendo.app.NavigationPolicy.Decision.OPEN_OUTSIDE;
import static br.com.encomendo.app.NavigationPolicy.Decision.RETRY;
import static br.com.encomendo.app.NavigationPolicy.Decision.STAY;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertThrows;
import static org.junit.Assert.assertTrue;

import java.util.Map;
import org.junit.Test;

public class NavigationPolicyTest {

    private final NavigationPolicy release = new NavigationPolicy("https://admin.encomendo.com.br", "https://localhost");
    private final NavigationPolicy debug = new NavigationPolicy("http://admin.localhost:8080", "https://localhost");

    private static final Map<String, String> FRAME = Map.of("Upgrade-Insecure-Requests", "1", "Accept", "text/html");
    private static final Map<String, String> FETCH = Map.of("Accept", "application/json");

    /** tap is a navigation in the page that the user started. */
    private static NavigationPolicy.Decision tap(NavigationPolicy policy, String url) {
        return policy.decide(url, true, true);
    }

    @Test
    public void theAdminOriginStaysInTheApp() {
        assertEquals(STAY, tap(release, "https://admin.encomendo.com.br/"));
        assertEquals(STAY, tap(release, "https://admin.encomendo.com.br/catalogo?aba=kits#x"));
        assertEquals(STAY, tap(release, "https://ADMIN.encomendo.com.br:443/hoje"));
        assertEquals(STAY, tap(debug, "http://admin.localhost:8080/equipe"));
        assertEquals(STAY, release.decide("https://admin.encomendo.com.br/hoje", true, false));
    }

    @Test
    public void anotherSchemeOrPortOfTheAdminHostIsNotTheAdmin() {
        assertEquals(OPEN_OUTSIDE, tap(release, "http://admin.encomendo.com.br/"));
        assertEquals(OPEN_OUTSIDE, tap(release, "https://admin.encomendo.com.br:8443/"));
        assertEquals(OPEN_OUTSIDE, tap(debug, "http://admin.localhost:9000/"));
        assertEquals(OPEN_OUTSIDE, tap(debug, "https://admin.localhost:8080/"));
    }

    @Test
    public void whatsAppAndStorePagesOpenOutside() {
        assertEquals(OPEN_OUTSIDE, tap(release, "https://wa.me/5511999999999?text=Ol%C3%A1"));
        assertEquals(OPEN_OUTSIDE, tap(release, "https://api.whatsapp.com/send?phone=5511999999999"));
        assertEquals(OPEN_OUTSIDE, tap(release, "https://altarpm.encomendo.com.br/"));
        assertEquals(OPEN_OUTSIDE, tap(release, "https://conta.encomendo.com.br/"));
        assertEquals(OPEN_OUTSIDE, tap(release, "https://encomendo.com.br/"));
        assertEquals(OPEN_OUTSIDE, tap(debug, "http://altarpm.localhost:8080/"));
    }

    @Test
    public void onlyATapOrItsRedirectOpensAnotherApp() {
        // userInitiated is hasGesture() || isRedirect(): a script navigating by itself has neither.
        assertEquals(BLOCK, release.decide("https://wa.me/5511999999999", true, false));
        assertEquals(BLOCK, release.decide("tel:+5511999999999", true, false));
    }

    @Test
    public void lookalikeHostsAreNotTheAdmin() {
        assertEquals(OPEN_OUTSIDE, tap(release, "https://admin.encomendo.com.br.evil.example/"));
        assertEquals(OPEN_OUTSIDE, tap(release, "https://evil.example/admin.encomendo.com.br"));
        assertEquals(OPEN_OUTSIDE, tap(release, "https://admin.encomendo.com.br@evil.example/"));
        assertEquals(BLOCK, tap(release, "https://evil.example\\@admin.encomendo.com.br/"));
    }

    @Test
    public void mailAndPhoneLinksOpenOutside() {
        assertEquals(OPEN_OUTSIDE, tap(release, "mailto:loja@example.com"));
        assertEquals(OPEN_OUTSIDE, tap(release, "tel:+5511999999999"));
    }

    @Test
    public void schemesThatReachIntoAppsAreBlocked() {
        assertEquals(BLOCK, tap(release, "intent://scan/#Intent;scheme=zxing;end"));
        assertEquals(BLOCK, tap(release, "file:///data/data/br.com.encomendo.app/"));
        assertEquals(BLOCK, tap(release, "content://br.com.encomendo.app.fileprovider/photos/a.jpg"));
        assertEquals(BLOCK, tap(release, "data:text/html,<p>x</p>"));
        assertEquals(BLOCK, tap(release, "whatsapp://send?phone=5511999999999"));
        assertEquals(BLOCK, tap(release, "not a url"));
        assertEquals(BLOCK, tap(release, null));
    }

    @Test
    public void framesNeverLeaveTheAppOrLoadAnotherSite() {
        assertEquals(STAY, release.decide("https://admin.encomendo.com.br/frame", false, true));
        assertEquals(BLOCK, release.decide("https://wa.me/5511999999999", false, true));
        assertEquals(BLOCK, release.decide("https://altarpm.encomendo.com.br/", false, true));
    }

    @Test
    public void blobUrlsStayOnlyWhenTheAdminMadeThem() {
        assertEquals(STAY, tap(release, "blob:https://admin.encomendo.com.br/8c3d1f2a-1b2c-4d5e-8f90-123456789abc"));
        assertEquals(BLOCK, tap(release, "blob:https://evil.example/8c3d1f2a-1b2c-4d5e-8f90-123456789abc"));
    }

    @Test
    public void tryAgainOnTheOfflinePageReloadsTheAdmin() {
        assertEquals(RETRY, tap(release, "https://localhost" + NavigationPolicy.RETRY_PATH));
        assertEquals(RETRY, tap(debug, "https://localhost/retry"));
    }

    @Test
    public void theLocalOriginServesNothingElse() {
        assertEquals(BLOCK, tap(release, "https://localhost/"));
        assertEquals(BLOCK, tap(release, "https://localhost/index.html"));
        assertEquals(BLOCK, release.decide("https://localhost/retry", false, true));
        assertEquals(OPEN_OUTSIDE, tap(release, "http://localhost/retry"));
    }

    @Test
    public void thePolicyNeedsAbsoluteWebOrigins() {
        assertThrows(IllegalArgumentException.class, () -> new NavigationPolicy("admin.encomendo.com.br", "https://localhost"));
        assertThrows(IllegalArgumentException.class, () -> new NavigationPolicy("https://admin.encomendo.com.br", "file:///android_asset"));
    }

    @Test
    public void framesOfAnotherSiteAreRefusedWhateverTheirScheme() {
        assertTrue(release.refuseRequest("https://altarpm.encomendo.com.br/", false, FRAME));
        assertTrue(release.refuseRequest("https://evil.example/", false, Map.of("Sec-Fetch-Dest", "iframe")));
        assertTrue(release.refuseRequest("http://admin.encomendo.com.br/", false, FRAME));
        assertTrue(release.refuseRequest("https://evil.example/a|b", false, FRAME));
        assertFalse(release.refuseRequest("https://admin.encomendo.com.br/frame", false, FRAME));
    }

    @Test
    public void subresourcesOfOtherOriginsStillLoad() {
        // Photos come from the storage host, signed; the API is the admin's own origin.
        assertFalse(debug.refuseRequest("http://localhost:9000/photos/a.jpg?X-Amz-Signature=abc", false, Map.of("Accept", "image/*")));
        assertFalse(release.refuseRequest("https://admin.encomendo.com.br/api/v1/catalog/items", false, FETCH));
        assertFalse(release.refuseRequest("https://cdn.example/a|b.png", false, FETCH));
        assertFalse(release.refuseRequest("https://evil.example/", false, null));
    }

    @Test
    public void theMainFrameIsLeftToTheNavigationDecision() {
        assertFalse(release.refuseRequest("https://admin.encomendo.com.br/", true, FRAME));
        assertFalse(release.refuseRequest("https://localhost/index.html", true, FRAME));
    }

    @Test
    public void capacitorsFileAndContentRoutesAreRefusedEverywhere() {
        String cookies = "/_capacitor_file_/data/user/0/br.com.encomendo.app/app_webview/Default/Cookies";
        assertTrue(release.refuseRequest("https://admin.encomendo.com.br" + cookies, false, FETCH));
        assertTrue(release.refuseRequest("https://admin.encomendo.com.br" + cookies, true, FRAME));
        assertTrue(release.refuseRequest("https://admin.encomendo.com.br/%5Fcapacitor_file_/data/x", false, FETCH));
        assertTrue(release.refuseRequest("https://admin.encomendo.com.br/_capacitor_content_/media/1", false, FETCH));
        assertTrue(release.refuseRequest("https://admin.encomendo.com.br/_capacitor_http_interceptor_?u=x", false, FETCH));
        assertTrue(release.refuseRequest("https://admin.encomendo.com.br/_capacitor_https_interceptor_/x", false, FETCH));
        assertTrue(release.refuseRequest("https://localhost/_capacitor_file_/x", false, FETCH));
        assertTrue(release.refuseRequest("https://admin.encomendo.com.br/%zz", false, FETCH));
        assertTrue(release.refuseRequest(null, false, FETCH));
        assertFalse(release.refuseRequest("https://admin.encomendo.com.br/assets/_capacitor_file_.js", false, FETCH));
    }
}
