package br.com.encomendo.core.domain.session

import kotlin.test.Test
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class GrantedPermissionsTest {
    private val writeOrders = Permission("order:write")
    private val readCosts = Permission("cost:read")

    @Test
    fun allowsOnlyWhatWasGranted() {
        val granted = GrantedPermissions(setOf(writeOrders))

        assertTrue(granted.allows(writeOrders))
        assertFalse(granted.allows(readCosts))
    }

    @Test
    fun grantsNothingUntilTheSessionSaysSo() {
        // Fail closed: a screen shown before the permissions load must not offer costs or margins.
        assertFalse(GrantedPermissions.None.allows(writeOrders))
        assertFalse(GrantedPermissions.None.allows(readCosts))
    }

    @Test
    fun comparesPermissionsByTheirApiKey() {
        assertTrue(GrantedPermissions(setOf(Permission("order:write"))).allows(writeOrders))
        assertFalse(GrantedPermissions(setOf(Permission("Order:Write"))).allows(writeOrders))
    }
}
