package br.com.encomendo.feature.members

import androidx.compose.runtime.Composable
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.ui.failure.text
import br.com.encomendo.feature.members.resources.Res
import br.com.encomendo.feature.members.resources.members_lost_permission
import br.com.encomendo.feature.members.resources.members_not_on_team
import br.com.encomendo.feature.members.resources.members_removed
import br.com.encomendo.feature.members.resources.members_role_changed
import br.com.encomendo.feature.members.resources.role_admin
import br.com.encomendo.feature.members.resources.role_owner
import br.com.encomendo.feature.members.resources.role_staff
import br.com.encomendo.feature.members.resources.role_viewer
import org.jetbrains.compose.resources.stringResource

/**
 * How the apps call [this] role: product wording, the same in every store
 * and niche (ADR 0017). A role the app does not know yet shows its key.
 */
@Composable
internal fun Role.label(): String =
    when (this) {
        Role.Owner -> stringResource(Res.string.role_owner)
        Role.Admin -> stringResource(Res.string.role_admin)
        Role.Staff -> stringResource(Res.string.role_staff)
        Role.Viewer -> stringResource(Res.string.role_viewer)
        else -> key
    }

/** The status line for [this] change, with the member's name. */
@Composable
internal fun TeamNotice.text(): String =
    when (this) {
        is TeamNotice.RoleChanged -> stringResource(Res.string.members_role_changed, member.name, member.role.label())
        is TeamNotice.Removed -> stringResource(Res.string.members_removed, member.name)
    }

/** The text for [this] failure, at the top or in a row. */
@Composable
internal fun TeamMessage.text(): String =
    when (this) {
        TeamMessage.LostPermission -> stringResource(Res.string.members_lost_permission)
        TeamMessage.NotOnTeam -> stringResource(Res.string.members_not_on_team)
        is TeamMessage.General -> message.text()
    }
