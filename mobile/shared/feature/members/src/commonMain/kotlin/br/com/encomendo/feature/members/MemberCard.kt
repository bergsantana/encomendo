package br.com.encomendo.feature.members

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.feature.members.resources.Res
import br.com.encomendo.feature.members.resources.members_ask_own_role
import br.com.encomendo.feature.members.resources.members_ask_remove
import br.com.encomendo.feature.members.resources.members_ask_remove_self
import br.com.encomendo.feature.members.resources.members_cancel
import br.com.encomendo.feature.members.resources.members_change_role
import br.com.encomendo.feature.members.resources.members_change_role_of
import br.com.encomendo.feature.members.resources.members_confirm
import br.com.encomendo.feature.members.resources.members_confirm_remove
import br.com.encomendo.feature.members.resources.members_name_you
import br.com.encomendo.feature.members.resources.members_remove
import br.com.encomendo.feature.members.resources.members_remove_of
import br.com.encomendo.feature.members.resources.members_role_of
import org.jetbrains.compose.resources.stringResource
import androidx.compose.ui.semantics.Role as SemanticsRole

/**
 * MemberCard is one person on the team: name ("(você)" on one's own row),
 * email and role. The owner's row shows "Dono" and nothing to change: the
 * owner's role never changes and the owner is never removed (ADR 0011).
 */
@Composable
internal fun MemberCard(
    member: Member,
    state: MembersUiState,
    actions: MembersActions,
) {
    val isMe = state.isMe(member)
    OutlinedCard(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                text = if (isMe) stringResource(Res.string.members_name_you, member.name) else member.name,
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                text = member.email,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (member.canChange) {
                MemberControls(member = member, isMe = isMe, state = state, actions = actions)
            } else {
                Text(text = member.role.label(), style = MaterialTheme.typography.bodyLarge)
            }
            state.rowError?.takeIf { it.userId == member.userId }?.let { error ->
                Text(
                    text = error.message.text(),
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.announced(),
                )
            }
        }
    }
}

/**
 * The role choice, then either the row's question or its buttons. Cancelling
 * puts focus back where the question came from, as on the web: the role's
 * choice for a role question, "Remover" for a removal.
 */
@Composable
private fun MemberControls(
    member: Member,
    isMe: Boolean,
    state: MembersUiState,
    actions: MembersActions,
) {
    // One change at a time: every row waits while one is saved.
    val enabled = state.savingUserId == null
    val question = state.question?.takeIf { it.userId == member.userId }
    val roleFocus = remember { FocusRequester() }
    val removeFocus = remember { FocusRequester() }
    var cancelled by remember { mutableStateOf<QuestionKind?>(null) }
    LaunchedEffect(question, cancelled) {
        val back = cancelled.takeIf { question == null } ?: return@LaunchedEffect
        cancelled = null
        (if (back == QuestionKind.OwnRole) roleFocus else removeFocus).requestFocus()
    }
    RoleChoice(
        member = member,
        selected = state.draftFor(member),
        // The question is about the role it shows, so the choice waits while it is asked.
        enabled = enabled && question == null,
        onPick = { actions.onPickRole(member, it) },
        modifier = Modifier.focusRequester(roleFocus),
    )
    if (question != null) {
        val onCancel = {
            cancelled = question.kind
            actions.onCancel()
        }
        MemberQuestion(
            member = member,
            kind = question.kind,
            isMe = isMe,
            enabled = enabled,
            actions = actions.copy(onCancel = onCancel),
        )
    } else {
        RowButtons(
            member = member,
            hasDraft = state.hasDraft(member),
            enabled = enabled,
            actions = actions,
            removeFocus = removeFocus,
        )
    }
}

/** "Alterar papel" while the row has a draft, and "Remover", each read out with the person's name. */
@Composable
private fun RowButtons(
    member: Member,
    hasDraft: Boolean,
    enabled: Boolean,
    actions: MembersActions,
    removeFocus: FocusRequester,
) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        if (hasDraft) {
            val description = stringResource(Res.string.members_change_role_of, member.name)
            Button(
                onClick = { actions.onSaveRole(member) },
                enabled = enabled,
                modifier = Modifier.heightIn(min = TOUCH_TARGET).semantics { contentDescription = description },
            ) { Text(stringResource(Res.string.members_change_role)) }
        }
        val description = stringResource(Res.string.members_remove_of, member.name)
        OutlinedButton(
            onClick = { actions.onAskRemove(member) },
            enabled = enabled,
            modifier =
                Modifier
                    .heightIn(min = TOUCH_TARGET)
                    .focusRequester(removeFocus)
                    .semantics { contentDescription = description },
        ) { Text(stringResource(Res.string.members_remove)) }
    }
}

/**
 * The role as a choice of Admin, Funcionário and Visualizador, read out as
 * "Papel de {nome}". Picking only drafts it, so moving through the choices
 * never saves a role step by step (WCAG 3.2.2).
 */
@Composable
private fun RoleChoice(
    member: Member,
    selected: Role,
    enabled: Boolean,
    onPick: (Role) -> Unit,
    modifier: Modifier = Modifier,
) {
    var open by remember { mutableStateOf(false) }
    val description = stringResource(Res.string.members_role_of, member.name)
    val label = selected.label()
    Box {
        OutlinedButton(
            onClick = { open = true },
            enabled = enabled,
            modifier =
                modifier.heightIn(min = TOUCH_TARGET).semantics {
                    contentDescription = description
                    stateDescription = label
                    role = SemanticsRole.DropdownList
                },
        ) { Text(label) }
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

/**
 * The question a change asks first, with "Cancelar" and the change itself.
 * Focus moves to "Cancelar", the safe answer, since the button just tapped
 * is gone, as on the web.
 */
@Composable
private fun MemberQuestion(
    member: Member,
    kind: QuestionKind,
    isMe: Boolean,
    enabled: Boolean,
    actions: MembersActions,
) {
    val cancel = remember { FocusRequester() }
    LaunchedEffect(kind) { cancel.requestFocus() }
    val text =
        when {
            kind == QuestionKind.OwnRole -> stringResource(Res.string.members_ask_own_role)
            isMe -> stringResource(Res.string.members_ask_remove_self)
            else -> stringResource(Res.string.members_ask_remove, member.name)
        }
    Text(text = text, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.announced())
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        OutlinedButton(
            onClick = actions.onCancel,
            enabled = enabled,
            modifier = Modifier.heightIn(min = TOUCH_TARGET).focusRequester(cancel),
        ) { Text(stringResource(Res.string.members_cancel)) }
        Button(
            onClick = actions.onConfirm,
            enabled = enabled,
            modifier = Modifier.heightIn(min = TOUCH_TARGET),
        ) {
            Text(
                stringResource(
                    if (kind == QuestionKind.OwnRole) Res.string.members_confirm else Res.string.members_confirm_remove,
                ),
            )
        }
    }
}
