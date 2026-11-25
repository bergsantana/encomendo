package br.com.encomendo.core.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.ProvidableCompositionLocal
import androidx.compose.runtime.staticCompositionLocalOf
import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Permission

/**
 * The signed-in user's permissions for every screen below. The app root
 * provides them from the session; until it does, nothing gated is shown.
 */
val LocalGrantedPermissions: ProvidableCompositionLocal<GrantedPermissions> =
    staticCompositionLocalOf { GrantedPermissions.None }

/**
 * PermissionGate shows [content] only when the signed-in user has [required],
 * and [otherwise] (nothing, by default) when not. It only hides UI: the
 * backend checks every call (docs/architecture.md 5.4), and costs and margins
 * still appear only when the API sends them.
 */
@Composable
fun PermissionGate(
    required: Permission,
    otherwise: @Composable () -> Unit = {},
    content: @Composable () -> Unit,
) {
    if (LocalGrantedPermissions.current.allows(required)) content() else otherwise()
}
