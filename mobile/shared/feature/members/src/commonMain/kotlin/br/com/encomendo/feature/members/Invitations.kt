package br.com.encomendo.feature.members

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.domain.members.Invitation
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.feature.members.resources.Res
import br.com.encomendo.feature.members.resources.invitations_resend
import br.com.encomendo.feature.members.resources.invitations_resend_to
import br.com.encomendo.feature.members.resources.invitations_revoke
import br.com.encomendo.feature.members.resources.invitations_revoke_to
import br.com.encomendo.feature.members.resources.invitations_title
import br.com.encomendo.feature.members.resources.invitations_valid_until
import br.com.encomendo.feature.members.resources.invite_email
import br.com.encomendo.feature.members.resources.invite_role
import br.com.encomendo.feature.members.resources.invite_send
import br.com.encomendo.feature.members.resources.invite_title
import org.jetbrains.compose.resources.stringResource
import androidx.compose.ui.semantics.Role as SemanticsRole

/**
 * InviteForm is "Convidar para a equipe": an email and a role, Funcionário
 * unless another is chosen, and "Enviar convite". An empty email is marked on
 * tap, never by a disabled button, as on every staff form.
 */
@Composable
internal fun InviteForm(
    form: InviteFormUi,
    actions: MembersActions,
) {
    val emailFocus = remember { FocusRequester() }
    // As on the web, each refusal about the email takes the keyboard to it, once: not again when the form
    // scrolls back into view or the phone turns, since the last request served is saved with the form.
    var focusServed by rememberSaveable { mutableIntStateOf(form.emailFocusRequests) }
    LaunchedEffect(form.emailFocusRequests) {
        if (form.emailFocusRequests != focusServed) {
            focusServed = form.emailFocusRequests
            emailFocus.requestFocus()
        }
    }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            text = stringResource(Res.string.invite_title),
            style = MaterialTheme.typography.titleLarge,
            modifier = Modifier.semantics { heading() },
        )
        form.error?.let { RowErrorText(it) }
        OutlinedTextField(
            value = form.email,
            onValueChange = actions.onInviteEmailChange,
            label = { Text(stringResource(Res.string.invite_email)) },
            singleLine = true,
            isError = form.emailIssue != null,
            supportingText = form.emailIssue?.let { issue -> { Text(issue.text(), modifier = Modifier.announced()) } },
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email, imeAction = ImeAction.Send),
            keyboardActions = KeyboardActions(onSend = { actions.onSendInvitation() }),
            modifier = Modifier.fillMaxWidth().focusRequester(emailFocus),
        )
        InviteRoleChoice(selected = form.role, enabled = !form.sending, onPick = actions.onInviteRoleChange)
        Button(
            onClick = actions.onSendInvitation,
            enabled = !form.sending,
            modifier = Modifier.fillMaxWidth().heightIn(min = TOUCH_TARGET),
        ) { Text(stringResource(Res.string.invite_send)) }
    }
}

/** "Papel" for the invitation: Admin, Funcionário or Visualizador. */
@Composable
private fun InviteRoleChoice(
    selected: Role,
    enabled: Boolean,
    onPick: (Role) -> Unit,
) {
    var open by remember { mutableStateOf(false) }
    Box {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            // Read out once, before the choice it names.
            Text(text = stringResource(Res.string.invite_role), style = MaterialTheme.typography.labelLarge)
            OutlinedButton(
                onClick = { open = true },
                enabled = enabled,
                modifier = Modifier.heightIn(min = TOUCH_TARGET).semantics { role = SemanticsRole.DropdownList },
            ) { Text(selected.label()) }
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            Role.Invitable.forEach { role ->
                DropdownMenuItem(
                    text = { Text(role.label()) },
                    onClick = {
                        open = false
                        onPick(role)
                    },
                    modifier = Modifier.heightIn(min = TOUCH_TARGET),
                )
            }
        }
    }
}

/** The "Convites pendentes" heading, shown while there is any. */
@Composable
internal fun InvitationsTitle() {
    Text(
        text = stringResource(Res.string.invitations_title),
        style = MaterialTheme.typography.titleLarge,
        modifier = Modifier.padding(top = 8.dp).semantics { heading() },
    )
}

/**
 * InvitationCard is one pending invitation: email, role and "Vale até", with
 * "Reenviar" (a new link and expiry) and "Cancelar convite", each read out
 * with the email.
 */
@Composable
internal fun InvitationCard(
    invitation: Invitation,
    state: MembersUiState,
    actions: MembersActions,
) {
    // One change at a time: every row waits while one is saved.
    val enabled = state.saving == null
    OutlinedCard(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(text = invitation.email, style = MaterialTheme.typography.titleMedium)
            Text(text = invitation.role.label(), style = MaterialTheme.typography.bodyLarge)
            Text(
                text = stringResource(Res.string.invitations_valid_until, ptBrDate(invitation.expiresAt)),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                val resend = stringResource(Res.string.invitations_resend_to, invitation.email)
                OutlinedButton(
                    onClick = { actions.onResend(invitation) },
                    enabled = enabled,
                    modifier = Modifier.heightIn(min = TOUCH_TARGET).semantics { contentDescription = resend },
                ) { Text(stringResource(Res.string.invitations_resend)) }
                val revoke = stringResource(Res.string.invitations_revoke_to, invitation.email)
                OutlinedButton(
                    onClick = { actions.onRevoke(invitation) },
                    enabled = enabled,
                    modifier = Modifier.heightIn(min = TOUCH_TARGET).semantics { contentDescription = revoke },
                ) { Text(stringResource(Res.string.invitations_revoke)) }
            }
            state.errorIn(RowKey.OfInvitation(invitation.key))?.let { RowErrorText(it) }
        }
    }
}
