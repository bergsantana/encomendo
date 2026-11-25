package br.com.encomendo.feature.auth

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class StoreLinkTest {
    // Only words the hint, as on the web's picker: the API still decides (docs/api/openapi.yaml, Slug).

    @Test
    fun aLinkIsWrittenAsItWillBeSent() {
        assertEquals("alta-rpm", storeLink(" Alta-RPM "))
    }

    @Test
    fun acceptsWhatTheContractAccepts() {
        assertTrue(looksLikeAStoreLink("alta-rpm"))
        assertTrue(looksLikeAStoreLink("abc"))
        assertTrue(looksLikeAStoreLink("a".repeat(40)))
        assertTrue(looksLikeAStoreLink(" Alta-RPM "))
    }

    @Test
    fun refusesWhatTheContractRefuses() {
        assertFalse(looksLikeAStoreLink("ab"))
        assertFalse(looksLikeAStoreLink("a".repeat(41)))
        assertFalse(looksLikeAStoreLink("alta--rpm"))
        assertFalse(looksLikeAStoreLink("-alta"))
        assertFalse(looksLikeAStoreLink("alta rpm"))
        assertFalse(looksLikeAStoreLink("peças"))
    }
}
