package br.com.encomendo.feature.members

import androidx.compose.runtime.Composable
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.ui.failure.text
import br.com.encomendo.feature.members.resources.Res
import br.com.encomendo.feature.members.resources.invitations_not_pending
import br.com.encomendo.feature.members.resources.invitations_resent
import br.com.encomendo.feature.members.resources.invitations_revoked
import br.com.encomendo.feature.members.resources.invite_already_member
import br.com.encomendo.feature.members.resources.invite_check_fields
import br.com.encomendo.feature.members.resources.invite_missing_email
import br.com.encomendo.feature.members.resources.invite_not_sent
import br.com.encomendo.feature.members.resources.invite_sent
import br.com.encomendo.feature.members.resources.members_lost_permission
import br.com.encomendo.feature.members.resources.members_not_on_team
import br.com.encomendo.feature.members.resources.members_removed
import br.com.encomendo.feature.members.resources.members_role_changed
import br.com.encomendo.feature.members.resources.role_admin
import br.com.encomendo.feature.members.resources.role_owner
import br.com.encomendo.feature.members.resources.role_staff
import br.com.encomendo.feature.members.resources.role_viewer
import kotlinx.datetime.LocalDate
import kotlinx.datetime.TimeZone
import kotlinx.datetime.format.char
import kotlinx.datetime.toLocalDateTime
import org.jetbrains.compose.resources.stringResource
import kotlin.time.Instant

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

/** The status line for [this] change, with the member's name or the invitation's email. */
@Composable
internal fun TeamNotice.text(): String =
    when (this) {
        is TeamNotice.RoleChanged -> stringResource(Res.string.members_role_changed, member.name, member.role.label())
        is TeamNotice.Removed -> stringResource(Res.string.members_removed, member.name)
        is TeamNotice.InvitationSent -> stringResource(Res.string.invite_sent, invitation.email)
        is TeamNotice.InvitationResent -> stringResource(Res.string.invitations_resent, invitation.email)
        is TeamNotice.InvitationRevoked -> stringResource(Res.string.invitations_revoked, invitation.email)
    }

/** The text for [this] failure, at the top, in a row or above the invite form. */
@Composable
internal fun TeamMessage.text(): String =
    when (this) {
        TeamMessage.LostPermission -> stringResource(Res.string.members_lost_permission)
        TeamMessage.NotOnTeam -> stringResource(Res.string.members_not_on_team)
        TeamMessage.NotPending -> stringResource(Res.string.invitations_not_pending)
        TeamMessage.AlreadyOnTeam -> stringResource(Res.string.invite_already_member)
        TeamMessage.NotSent -> stringResource(Res.string.invite_not_sent)
        TeamMessage.CheckFields -> stringResource(Res.string.invite_check_fields)
        is TeamMessage.General -> message.text()
    }

/** The text under the invite form's email for [this] issue. */
@Composable
internal fun EmailIssue.text(): String =
    when (this) {
        EmailIssue.Missing -> stringResource(Res.string.invite_missing_email)
        EmailIssue.AlreadyOnTeam -> stringResource(Res.string.invite_already_member)
        is EmailIssue.Server -> message
    }

/**
 * Returns [instant] as a pt-BR date, dd/mm/aaaa, in the phone's time zone,
 * as the web's `formatDate` shows "Vale até". The API calculates the date;
 * the app only shows it.
 */
internal fun ptBrDate(
    instant: Instant,
    zone: TimeZone = TimeZone.currentSystemDefault(),
): String = PT_BR_DATE.format(instant.toLocalDateTime(zone).date)

private val PT_BR_DATE =
    LocalDate.Format {
        day()
        char('/')
        monthNumber()
        char('/')
        year()
    }
