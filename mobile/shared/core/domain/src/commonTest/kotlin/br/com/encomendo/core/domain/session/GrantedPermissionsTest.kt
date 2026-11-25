package br.com.encomendo.core.domain.session

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class GrantedPermissionsTest {
    // Names as docs/architecture.md 4.2 lists them; the real ones come from the /auth contract.
    private val writeOrders = Permission("order:write")
    private val readFinance = Permission("finance:read")

    @Test
    fun allowsOnlyWhatWasGranted() {
        val granted = GrantedPermissions(setOf(writeOrders))

        assertTrue(granted.allows(writeOrders))
        assertFalse(granted.allows(readFinance))
    }

    @Test
    fun grantsNothingUntilTheSessionSaysSo() {
        // Fail closed: a screen shown before the permissions load must not offer any gated action.
        assertFalse(GrantedPermissions.None.allows(writeOrders))
        assertFalse(GrantedPermissions.None.allows(readFinance))
    }

    @Test
    fun comparesPermissionsByTheirApiKey() {
        assertTrue(GrantedPermissions(setOf(Permission("order:write"))).allows(writeOrders))
        assertFalse(GrantedPermissions(setOf(Permission("Order:Write"))).allows(writeOrders))
    }

    @Test
    fun equalGrantsAreEqualAndDoNotFollowTheSourceCollection() {
        // Grants travel in UI state: equal keys must compare equal, and a later change to the
        // caller's collection must not silently change what is allowed.
        val source = mutableSetOf(writeOrders)
        val granted = GrantedPermissions(source)

        source += readFinance

        assertEquals(GrantedPermissions(listOf(writeOrders)), granted)
        assertEquals(GrantedPermissions(setOf(writeOrders)).hashCode(), granted.hashCode())
        assertFalse(granted.allows(readFinance))
    }
}
