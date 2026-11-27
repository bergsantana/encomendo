package br.com.encomendo.app;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

public class WebViewSupportTest {

    private static WebViewSupport.Facts facts(String version, boolean listener, boolean documentStart, boolean legacy) {
        return new WebViewSupport.Facts(version, 120, listener, documentStart, legacy);
    }

    @Test
    public void aRecentWebViewWithBothFeaturesRunsTheAdmin() {
        assertTrue(WebViewSupport.supported(facts("131.0.6778.135", true, true, false)));
        assertTrue(WebViewSupport.supported(facts("120.0.6099.230", true, true, false)));
    }

    @Test
    public void aMissingFeatureOrTheLegacyBridgeRefusesIt() {
        assertFalse(WebViewSupport.supported(facts("131.0.6778.135", false, true, false)));
        assertFalse(WebViewSupport.supported(facts("131.0.6778.135", true, false, false)));
        assertFalse(WebViewSupport.supported(facts("131.0.6778.135", true, true, true)));
    }

    @Test
    public void anOldOrUnknownWebViewRefusesIt() {
        assertFalse(WebViewSupport.supported(facts("119.0.6045.193", true, true, false)));
        assertFalse(WebViewSupport.supported(facts(null, true, true, false)));
        assertFalse(WebViewSupport.supported(facts("beta", true, true, false)));
    }

    @Test
    public void theMajorVersionIsTheFirstNumber() {
        assertEquals(131, WebViewSupport.majorVersion("131.0.6778.135"));
        assertEquals(87, WebViewSupport.majorVersion("87"));
        assertEquals(-1, WebViewSupport.majorVersion(""));
        assertEquals(-1, WebViewSupport.majorVersion("12345678.0"));
    }
}
