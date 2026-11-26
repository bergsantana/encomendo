package br.com.encomendo.feature.auth

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.feature.auth.resources.Res
import br.com.encomendo.feature.auth.resources.login_missing_email
import br.com.encomendo.feature.auth.resources.reset_back_to_login
import br.com.encomendo.feature.auth.resources.reset_change_password
import br.com.encomendo.feature.auth.resources.reset_changing_password
import br.com.encomendo.feature.auth.resources.reset_code
import br.com.encomendo.feature.auth.resources.reset_code_resent
import br.com.encomendo.feature.auth.resources.reset_code_sent
import br.com.encomendo.feature.auth.resources.reset_intro
import br.com.encomendo.feature.auth.resources.reset_missing_code
import br.com.encomendo.feature.auth.resources.reset_missing_password
import br.com.encomendo.feature.auth.resources.reset_new_password
import br.com.encomendo.feature.auth.resources.reset_send_another_code
import br.com.encomendo.feature.auth.resources.reset_send_code
import br.com.encomendo.feature.auth.resources.reset_sending_code
import br.com.encomendo.feature.auth.resources.reset_title
import br.com.encomendo.feature.auth.resources.reset_use_another_email
import br.com.encomendo.feature.auth.resources.reset_wrong_code
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel

/**
 * ForgotPasswordRoute connects [ForgotPasswordViewModel] to
 * [ForgotPasswordScreen]. [onBackToLogin] leaves; [onPasswordChanged] goes
 * back to the login, which says the password changed.
 */
@Composable
fun ForgotPasswordRoute(
    onBackToLogin: () -> Unit,
    onPasswordChanged: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: ForgotPasswordViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val changed by rememberUpdatedState(onPasswordChanged)
    LaunchedEffect(viewModel) {
        viewModel.effects.collect { effect ->
            when (effect) {
                ForgotPasswordEffect.PasswordChanged -> changed()
            }
        }
    }
    val actions =
        remember(viewModel, onBackToLogin) {
            ForgotPasswordActions(
                onEmailChange = viewModel::onEmailChange,
                onSendCode = viewModel::onSendCode,
                onCodeChange = viewModel::onCodeChange,
                onPasswordChange = viewModel::onPasswordChange,
                onPasswordVisibilityToggle = viewModel::onPasswordVisibilityToggle,
                onChangePassword = viewModel::onChangePassword,
                onSendAnotherCode = viewModel::onSendAnotherCode,
                onUseAnotherEmail = viewModel::onUseAnotherEmail,
                onBackToLogin = onBackToLogin,
            )
        }
    ForgotPasswordScreen(state = state, actions = actions, modifier = modifier)
}

/** ForgotPasswordActions are what the user can do on [ForgotPasswordScreen]. */
data class ForgotPasswordActions(
    val onEmailChange: (String) -> Unit,
    val onSendCode: () -> Unit,
    val onCodeChange: (String) -> Unit,
    val onPasswordChange: (String) -> Unit,
    val onPasswordVisibilityToggle: () -> Unit,
    val onChangePassword: () -> Unit,
    val onSendAnotherCode: () -> Unit,
    val onUseAnotherEmail: () -> Unit,
    val onBackToLogin: () -> Unit,
)

/**
 * ForgotPasswordScreen is "Esqueci a senha", in the design document's words:
 * the email first, then the emailed code and a new password. Stateless.
 */
@Composable
fun ForgotPasswordScreen(
    state: ForgotPasswordUiState,
    actions: ForgotPasswordActions,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        FormTitle(stringResource(Res.string.reset_title))
        FormBanner(state.error)
        // Focus follows the step, as on the web: the button just tapped is gone, so the field to fill gets it.
        var cameFromCode by rememberSaveable { mutableStateOf(false) }
        if (state.sentTo == null) {
            EmailStep(state, actions, focusOnEntry = cameFromCode)
        } else {
            LaunchedEffect(Unit) { cameFromCode = true }
            CodeStep(state, state.sentTo, actions)
        }
        LinkButton(
            stringResource(Res.string.reset_back_to_login),
            enabled = !state.submitting,
            onClick = actions.onBackToLogin,
        )
    }
}

@Composable
private fun EmailStep(
    state: ForgotPasswordUiState,
    actions: ForgotPasswordActions,
    focusOnEntry: Boolean,
) {
    val focus = remember { FocusRequester() }
    if (focusOnEntry) LaunchedEffect(Unit) { focus.requestFocus() }
    Text(stringResource(Res.string.reset_intro))
    EmailField(
        field =
            FieldUi(
                value = state.email,
                issue = state.emailIssue?.text(missing = Res.string.login_missing_email),
                enabled = !state.submitting,
            ),
        onChange = actions.onEmailChange,
        modifier = Modifier.focusRequester(focus),
        onDone = actions.onSendCode,
    )
    BusyButton(
        label = stringResource(Res.string.reset_send_code),
        busyLabel = stringResource(Res.string.reset_sending_code),
        busy = state.submitting,
        enabled = !state.submitting,
        onClick = actions.onSendCode,
    )
}

@Composable
private fun CodeStep(
    state: ForgotPasswordUiState,
    sentTo: String,
    actions: ForgotPasswordActions,
) {
    val focus = remember { FocusRequester() }
    LaunchedEffect(Unit) { focus.requestFocus() }
    val notice =
        when (state.notice) {
            ResetNotice.CodeResent -> Res.string.reset_code_resent
            ResetNotice.CodeSent, null -> Res.string.reset_code_sent
        }
    Text(stringResource(notice, sentTo), modifier = Modifier.announced())
    CodeField(state, actions.onCodeChange, Modifier.focusRequester(focus))
    PasswordField(
        field =
            FieldUi(
                value = state.password,
                issue = state.passwordIssue?.text(missing = Res.string.reset_missing_password),
                enabled = !state.submitting,
            ),
        visible = state.passwordVisible,
        new = true,
        actions =
            PasswordActions(
                actions.onPasswordChange,
                actions.onPasswordVisibilityToggle,
                actions.onChangePassword,
            ),
        label = Res.string.reset_new_password,
    )
    BusyButton(
        label = stringResource(Res.string.reset_change_password),
        busyLabel = stringResource(Res.string.reset_changing_password),
        busy = state.submitting,
        enabled = !state.submitting,
        onClick = actions.onChangePassword,
    )
    LinkButton(
        stringResource(Res.string.reset_send_another_code),
        enabled = !state.submitting,
        onClick = actions.onSendAnotherCode,
    )
    LinkButton(
        stringResource(Res.string.reset_use_another_email),
        enabled = !state.submitting,
        onClick = actions.onUseAnotherEmail,
    )
}

@Composable
private fun CodeField(
    state: ForgotPasswordUiState,
    onChange: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val next = LocalFocusManager.current
    val issue = state.codeIssue?.text(missing = Res.string.reset_missing_code, wrong = Res.string.reset_wrong_code)
    OutlinedTextField(
        value = state.code,
        onValueChange = onChange,
        label = { Text(stringResource(Res.string.reset_code)) },
        isError = issue != null,
        supportingText = issue?.let { text -> { Issue(text) } },
        singleLine = true,
        enabled = !state.submitting,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword, imeAction = ImeAction.Next),
        keyboardActions = KeyboardActions(onNext = { next.moveFocus(FocusDirection.Down) }),
        modifier = modifier.fillMaxWidth(),
    )
}

/** A secondary action, 56 dp high like every target on these screens. */
@Composable
private fun LinkButton(
    text: String,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    TextButton(onClick = onClick, enabled = enabled, modifier = Modifier.heightIn(min = 56.dp)) { Text(text) }
}
