package br.com.encomendo.core.navigation

import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Permission
import kotlin.test.Test
import kotlin.test.assertEquals

class MenuEntryTest {
    @Test
    fun equipeIsOnlyForRolesThatManageTheTeam() {
        // Dono and Admin have members:manage; Funcionário and Visualizador don't see the entry
        // (design document, "Equipe").
        val staff = GrantedPermissions(listOf(Permission("order:write")))
        val admin = GrantedPermissions(listOf(Permission("order:write"), Permission.MembersManage))

        assertEquals(listOf(MenuEntry.Hoje), MenuEntry.allowedBy(staff))
        assertEquals(listOf(MenuEntry.Hoje, MenuEntry.Equipe), MenuEntry.allowedBy(admin))
    }

    @Test
    fun hojeIsOpenToEveryRole() {
        assertEquals(listOf(MenuEntry.Hoje), MenuEntry.allowedBy(GrantedPermissions.None))
    }

    @Test
    fun eachEntryLeadsToItsScreen() {
        assertEquals(HojeDestination, MenuEntry.Hoje.destination)
        assertEquals(MembersDestination, MenuEntry.Equipe.destination)
    }
}
