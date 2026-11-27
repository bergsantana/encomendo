package br.com.encomendo.core.domain.catalog

import br.com.encomendo.core.domain.money.Money
import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.domain.session.Role
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class OwnerFieldTest {
    private val reais = Money(cents = 41000, currency = "BRL")

    @Test
    fun aHiddenFieldIsNotAFieldWithNothingRecorded() {
        // The screens show the cost column only when the API sent the field; null is "sem custo", not "hidden".
        assertFalse(OwnerField.Hidden.isShown)
        assertTrue(OwnerField.Shown(null).isShown)
        assertTrue(OwnerField.Shown(reais).isShown)
        assertNull(OwnerField.Hidden.valueOrNull)
        assertNull(OwnerField.Shown(null).valueOrNull)
        assertEquals(reais, OwnerField.Shown(reais).valueOrNull)
    }

    @Test
    fun onlyTheOwnerSeesCosts() {
        assertTrue(Role.Owner.seesCosts())
        assertFalse(Role.Admin.seesCosts())
        assertFalse(Role.Staff.seesCosts())
        assertFalse(Role.Viewer.seesCosts())
    }

    @Test
    fun thePermissionKeysAreTheContractsOwn() {
        assertEquals("catalog:read", Permission.CatalogRead.key)
        assertEquals("catalog:write", Permission.CatalogWrite.key)
        assertTrue(GrantedPermissions(setOf(Permission("catalog:read"))).allows(Permission.CatalogRead))
    }

    @Test
    fun theModesAreFoundByTheKeysTheApiSends() {
        assertEquals(SourcingMode.InHouse, SourcingMode.fromKey("in_house"))
        assertEquals(SourcingMode.ToOrder, SourcingMode.fromKey("to_order"))
        assertNull(SourcingMode.fromKey("teleport"), "a mode this app does not know is not guessed")
        assertEquals(SwapDiscount.Lose, SwapDiscount.fromKey("lose"))
        assertNull(SwapDiscount.fromKey("other"))
    }
}
