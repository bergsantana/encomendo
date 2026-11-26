package br.com.encomendo.core.navigation

import androidx.navigation.NavDestination
import androidx.navigation.NavDestination.Companion.hasRoute
import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.navigation.resources.Res
import br.com.encomendo.core.navigation.resources.menu_equipe
import br.com.encomendo.core.navigation.resources.menu_hoje
import org.jetbrains.compose.resources.StringResource

/**
 * MenuEntry is one entry of the management menu, in the web admin's order
 * (`DashboardLayout.vue`): Hoje · Pedidos · Compras · … · Financeiro, then
 * Equipe. Each feature adds its entry when it ships. [permission] hides the
 * entry from roles without it; the screen and the API check it again
 * (docs/adr/0020-android-management-menu.md).
 */
internal enum class MenuEntry(
    val label: StringResource,
    val permission: Permission?,
) {
    Hoje(Res.string.menu_hoje, permission = null),
    Equipe(Res.string.menu_equipe, Permission.MembersManage),
    ;

    /** Where the entry leads. */
    val destination: Any
        get() =
            when (this) {
                Hoje -> HojeDestination
                Equipe -> MembersDestination
            }

    /** True when [current] is this entry's screen, so the menu marks it. */
    fun isAt(current: NavDestination?): Boolean =
        when (this) {
            Hoje -> current?.hasRoute<HojeDestination>()
            Equipe -> current?.hasRoute<MembersDestination>()
        } == true

    /** The entries [permissions] allow, in menu order. */
    companion object {
        fun allowedBy(permissions: GrantedPermissions): List<MenuEntry> =
            entries.filter { entry -> entry.permission?.let(permissions::allows) ?: true }
    }
}
