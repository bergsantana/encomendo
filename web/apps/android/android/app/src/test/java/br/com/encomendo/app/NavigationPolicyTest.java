package br.com.encomendo.app;

import static br.com.encomendo.app.NavigationPolicy.Decision.BLOCK;
import static br.com.encomendo.app.NavigationPolicy.Decision.OPEN_OUTSIDE;
import static br.com.encomendo.app.NavigationPolicy.Decision.RETRY;
import static br.com.encomendo.app.NavigationPolicy.Decision.STAY;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertThrows;

import org.junit.Test;

public class NavigationPolicyTest {

    private final NavigationPolicy release = new NavigationPolicy("https://admin.encomendo.com.br", "https://localhost");
    private final NavigationPolicy debug = new NavigationPolicy("http://admin.localhost:8080", "https://localhost");

    @Test
    public void theAdminOriginStaysInTheApp() {
        assertEquals(STAY, release.decide("https://admin.encomendo.com.br/", true));
        assertEquals(STAY, release.decide("https://admin.encomendo.com.br/catalogo?aba=kits#x", true));
        assertEquals(STAY, release.decide("https://ADMIN.encomendo.com.br:443/hoje", true));
        assertEquals(STAY, debug.decide("http://admin.localhost:8080/equipe", true));
    }

    @Test
    public void anotherSchemeOrPortOfTheAdminHostIsNotTheAdmin() {
        assertEquals(OPEN_OUTSIDE, release.decide("http://admin.encomendo.com.br/", true));
        assertEquals(OPEN_OUTSIDE, release.decide("https://admin.encomendo.com.br:8443/", true));
        assertEquals(OPEN_OUTSIDE, debug.decide("http://admin.localhost:9000/", true));
        assertEquals(OPEN_OUTSIDE, debug.decide("https://admin.localhost:8080/", true));
    }

    @Test
    public void whatsAppAndStorePagesOpenOutside() {
        assertEquals(OPEN_OUTSIDE, release.decide("https://wa.me/5511999999999?text=Ol%C3%A1", true));
        assertEquals(OPEN_OUTSIDE, release.decide("https://api.whatsapp.com/send?phone=5511999999999", true));
        assertEquals(OPEN_OUTSIDE, release.decide("https://altarpm.encomendo.com.br/", true));
        assertEquals(OPEN_OUTSIDE, release.decide("https://conta.encomendo.com.br/", true));
        assertEquals(OPEN_OUTSIDE, release.decide("https://encomendo.com.br/", true));
        assertEquals(OPEN_OUTSIDE, debug.decide("http://altarpm.localhost:8080/", true));
    }

    @Test
    public void lookalikeHostsAreNotTheAdmin() {
        assertEquals(OPEN_OUTSIDE, release.decide("https://admin.encomendo.com.br.evil.example/", true));
        assertEquals(OPEN_OUTSIDE, release.decide("https://evil.example/admin.encomendo.com.br", true));
        assertEquals(OPEN_OUTSIDE, release.decide("https://admin.encomendo.com.br@evil.example/", true));
        assertEquals(BLOCK, release.decide("https://evil.example\\@admin.encomendo.com.br/", true));
    }

    @Test
    public void mailAndPhoneLinksOpenOutside() {
        assertEquals(OPEN_OUTSIDE, release.decide("mailto:loja@example.com", true));
        assertEquals(OPEN_OUTSIDE, release.decide("tel:+5511999999999", true));
    }

    @Test
    public void schemesThatReachIntoAppsAreBlocked() {
        assertEquals(BLOCK, release.decide("intent://scan/#Intent;scheme=zxing;end", true));
        assertEquals(BLOCK, release.decide("file:///data/data/br.com.encomendo.app/", true));
        assertEquals(BLOCK, release.decide("content://br.com.encomendo.app.fileprovider/photos/a.jpg", true));
        assertEquals(BLOCK, release.decide("javascript:alert(1)", true));
        assertEquals(BLOCK, release.decide("data:text/html,<p>x</p>", true));
        assertEquals(BLOCK, release.decide("whatsapp://send?phone=5511999999999", true));
        assertEquals(BLOCK, release.decide("not a url", true));
        assertEquals(BLOCK, release.decide(null, true));
    }

    @Test
    public void framesNeverLeaveTheAppOrLoadAnotherSite() {
        assertEquals(STAY, release.decide("https://admin.encomendo.com.br/frame", false));
        assertEquals(BLOCK, release.decide("https://wa.me/5511999999999", false));
        assertEquals(BLOCK, release.decide("https://altarpm.encomendo.com.br/", false));
    }

    @Test
    public void blobUrlsStayOnlyWhenTheAdminMadeThem() {
        assertEquals(STAY, release.decide("blob:https://admin.encomendo.com.br/8c3d1f2a-1b2c-4d5e-8f90-123456789abc", true));
        assertEquals(BLOCK, release.decide("blob:https://evil.example/8c3d1f2a-1b2c-4d5e-8f90-123456789abc", true));
    }

    @Test
    public void tryAgainOnTheOfflinePageReloadsTheAdmin() {
        assertEquals(RETRY, release.decide("https://localhost" + NavigationPolicy.RETRY_PATH, true));
        assertEquals(RETRY, debug.decide("https://localhost/retry", true));
    }

    @Test
    public void theLocalOriginServesNothingElse() {
        assertEquals(BLOCK, release.decide("https://localhost/", true));
        assertEquals(BLOCK, release.decide("https://localhost/index.html", true));
        assertEquals(BLOCK, release.decide("https://localhost/retry", false));
        assertEquals(OPEN_OUTSIDE, release.decide("http://localhost/retry", true));
    }

    @Test
    public void thePolicyNeedsAbsoluteWebOrigins() {
        assertThrows(IllegalArgumentException.class, () -> new NavigationPolicy("admin.encomendo.com.br", "https://localhost"));
        assertThrows(IllegalArgumentException.class, () -> new NavigationPolicy("https://admin.encomendo.com.br", "file:///android_asset"));
    }
}
