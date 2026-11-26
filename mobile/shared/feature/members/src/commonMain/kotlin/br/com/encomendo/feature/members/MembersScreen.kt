package br.com.encomendo.feature.members

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.core.domain.members.Invitation
import br.com.encomendo.core.domain.members.Member
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.feature.members.resources.Res
import br.com.encomendo.feature.members.resources.invitations_load_more
import br.com.encomendo.feature.members.resources.members_load_more
import br.com.encomendo.feature.members.resources.members_loading
import br.com.encomendo.feature.members.resources.members_retry
import br.com.encomendo.feature.members.resources.members_title
import org.jetbrains.compose.resources.StringResource
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel

/** MembersRoute connects [MembersViewModel] to [MembersScreen]. */
@Composable
fun MembersRoute(
    modifier: Modifier = Modifier,
    viewModel: MembersViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val actions =
        remember(viewModel) {
            MembersActions(
                onRetry = viewModel::onRetry,
                onLoadMore = viewModel::onLoadMore,
                onPickRole = viewModel::onPickRole,
                onSaveRole = viewModel::onSaveRole,
                onAskRemove = viewModel::onAskRemove,
                onCancel = viewModel::onCancel,
                onConfirm = viewModel::onConfirm,
                onInviteEmailChange = viewModel::onInviteEmailChange,
                onInviteRoleChange = viewModel::onInviteRoleChange,
                onSendInvitation = viewModel::onSendInvitation,
                onResend = viewModel::onResend,
                onRevoke = viewModel::onRevoke,
                onLoadMoreInvitations = viewModel::onLoadMoreInvitations,
            )
        }
    MembersScreen(state = state, actions = actions, modifier = modifier)
}

/** MembersActions are what the user can do on [MembersScreen]. */
data class MembersActions(
    val onRetry: () -> Unit,
    val onLoadMore: () -> Unit,
    val onPickRole: (Member, Role) -> Unit,
    val onSaveRole: (Member) -> Unit,
    val onAskRemove: (Member) -> Unit,
    val onCancel: () -> Unit,
    val onConfirm: () -> Unit,
    val onInviteEmailChange: (String) -> Unit,
    val onInviteRoleChange: (Role) -> Unit,
    val onSendInvitation: () -> Unit,
    val onResend: (Invitation) -> Unit,
    val onRevoke: (Invitation) -> Unit,
    val onLoadMoreInvitations: () -> Unit,
)

/**
 * MembersScreen is Equipe: who is on the store's team and with which role,
 * with the role to change and a removal on every row but the owner's, then
 * the invite form and the pending invitations
 * (docs/product/design-document.md, "Equipe"). What went through and what
 * failed stay above the list, so they show and are read out however far
 * down the row that changed was.
 */
@Composable
fun MembersScreen(
    state: MembersUiState,
    actions: MembersActions,
    modifier: Modifier = Modifier,
) {
    Column(modifier = modifier.fillMaxSize()) {
        Column(
            modifier = Modifier.padding(start = 16.dp, top = 16.dp, end = 16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text(
                text = stringResource(Res.string.members_title),
                style = MaterialTheme.typography.headlineSmall,
                modifier = Modifier.semantics { heading() },
            )
            TeamMessages(state = state, onRetry = actions.onRetry)
        }
        TeamList(state = state, actions = actions, modifier = Modifier.weight(1f))
    }
}

/** The members, then "Carregar mais" while the team runs past what was read. */
@Composable
private fun TeamList(
    state: MembersUiState,
    actions: MembersActions,
    modifier: Modifier,
) {
    LazyColumn(
        // The invite form sits at the foot of the list: the keyboard must not cover it (edge to edge, API 30+).
        modifier = modifier.fillMaxWidth().imePadding(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        items(state.members, key = { it.userId }) { member ->
            MemberCard(member = member, state = state, actions = actions)
        }
        if (state.hasMore) {
            item(key = "more") {
                LoadMoreButton(Res.string.members_load_more, enabled = !state.loadingMore, onClick = actions.onLoadMore)
            }
        }
        item(key = "invite") { InviteForm(form = state.invite, actions = actions) }
        if (state.invitations.isNotEmpty()) {
            item(key = "invitations") { InvitationsTitle() }
            // Keyed by email: "Reenviar" can give an invitation a new id while its row stays.
            items(state.invitations, key = { "invitation:${it.key}" }) { invitation ->
                InvitationCard(invitation = invitation, state = state, actions = actions)
            }
            if (state.invitationsHaveMore) {
                item(key = "more-invitations") {
                    LoadMoreButton(
                        Res.string.invitations_load_more,
                        enabled = !state.loadingMoreInvitations,
                        onClick = actions.onLoadMoreInvitations,
                    )
                }
            }
        }
    }
}

/** "Carregar mais" or "Carregar mais convites", waiting while that page is read. */
@Composable
private fun LoadMoreButton(
    label: StringResource,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    OutlinedButton(onClick = onClick, enabled = enabled, modifier = Modifier.heightIn(min = TOUCH_TARGET)) {
        Text(stringResource(label))
    }
}

/** The status line, the message at the top, "Tentar de novo", and "Carregando a equipe…" while nothing shows. */
@Composable
private fun TeamMessages(
    state: MembersUiState,
    onRetry: () -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        state.notice?.let { notice ->
            Text(text = notice.text(), style = MaterialTheme.typography.bodyLarge, modifier = Modifier.announced())
        }
        state.error?.let { error ->
            Text(
                text = error.text(),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.announced(),
            )
        }
        if (state.canRetry) {
            Button(onClick = onRetry, modifier = Modifier.heightIn(min = TOUCH_TARGET)) {
                Text(stringResource(Res.string.members_retry))
            }
        }
        if (state.loading && state.members.isEmpty()) {
            Text(
                text = stringResource(Res.string.members_loading),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                style = MaterialTheme.typography.bodyLarge,
            )
        }
    }
}

/** Read out once it appears or changes, without interrupting. */
internal fun Modifier.announced(): Modifier = semantics { liveRegion = LiveRegionMode.Polite }

/** The height of every button on Equipe, so each is easy to hit. */
internal val TOUCH_TARGET = 48.dp
