package br.com.encomendo.feature.auth

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.feature.auth.resources.Res
import br.com.encomendo.feature.auth.resources.login_create_account
import br.com.encomendo.feature.auth.resources.login_forgot_password
import br.com.encomendo.feature.auth.resources.login_missing_email
import br.com.encomendo.feature.auth.resources.login_missing_password
import br.com.encomendo.feature.auth.resources.login_password_changed
import br.com.encomendo.feature.auth.resources.login_submit
import br.com.encomendo.feature.auth.resources.login_submitting
import br.com.encomendo.feature.auth.resources.login_title
import org.jetbrains.compose.resources.StringResource
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel

/**
 * LoginNavigation is where the login leads: sign-up, Esqueci a senha, and back
 * from a reset with [passwordChanged] set until [onNoticeShown].
 */
data class LoginNavigation(
    val onCreateAccount: () -> Unit,
    val onForgotPassword: () -> Unit,
    val passwordChanged: Boolean = false,
    val onNoticeShown: () -> Unit = {},
)

/** LoginRoute connects [LoginViewModel] to [LoginScreen]. */
@Composable
fun LoginRoute(
    navigation: LoginNavigation,
    modifier: Modifier = Modifier,
    viewModel: LoginViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    LaunchedEffect(navigation.passwordChanged) {
        if (navigation.passwordChanged) {
            viewModel.onPasswordChanged()
            navigation.onNoticeShown()
        }
    }
    // Remembered, so the screen can skip recomposing when only the state changes.
    val actions =
        remember(viewModel, navigation.onCreateAccount, navigation.onForgotPassword) {
            LoginActions(
                onEmailChange = viewModel::onEmailChange,
                onPasswordChange = viewModel::onPasswordChange,
                onPasswordVisibilityToggle = viewModel::onPasswordVisibilityToggle,
                onSubmit = viewModel::onSubmit,
                onCreateAccount = {
                    viewModel.onLeave()
                    navigation.onCreateAccount()
                },
                onForgotPassword = {
                    viewModel.onLeave()
                    navigation.onForgotPassword()
                },
            )
        }
    LoginScreen(state = state, actions = actions, modifier = modifier)
}

/** LoginActions are what the user can do on [LoginScreen]. */
data class LoginActions(
    val onEmailChange: (String) -> Unit,
    val onPasswordChange: (String) -> Unit,
    val onPasswordVisibilityToggle: () -> Unit,
    val onSubmit: () -> Unit,
    val onCreateAccount: () -> Unit,
    val onForgotPassword: () -> Unit,
)

/**
 * LoginScreen is where a signed-out business user starts: email, password and
 * Entrar, or Criar conta. It is stateless; the fields offer the device's
 * saved passwords.
 */
@Composable
fun LoginScreen(
    state: LoginUiState,
    actions: LoginActions,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier = modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        FormTitle(stringResource(Res.string.login_title))
        state.notice?.let { Text(stringResource(it.text()), modifier = Modifier.announced()) }
        FormBanner(state.error)
        EmailField(
            field =
                FieldUi(
                    value = state.email,
                    issue = state.emailIssue?.text(missing = Res.string.login_missing_email),
                    enabled = !state.submitting,
                ),
            onChange = actions.onEmailChange,
        )
        PasswordField(
            field =
                FieldUi(
                    value = state.password,
                    issue = state.passwordIssue?.text(missing = Res.string.login_missing_password),
                    enabled = !state.submitting,
                ),
            visible = state.passwordVisible,
            new = false,
            actions = PasswordActions(actions.onPasswordChange, actions.onPasswordVisibilityToggle, actions.onSubmit),
        )
        BusyButton(
            label = stringResource(Res.string.login_submit),
            busyLabel = stringResource(Res.string.login_submitting),
            busy = state.submitting,
            enabled = state.canSubmit,
            onClick = actions.onSubmit,
        )
        TextButton(
            onClick = actions.onForgotPassword,
            enabled = !state.submitting,
            modifier = Modifier.heightIn(min = 56.dp),
        ) {
            Text(stringResource(Res.string.login_forgot_password))
        }
        TextButton(
            onClick = actions.onCreateAccount,
            enabled = !state.submitting,
            modifier = Modifier.heightIn(min = 56.dp),
        ) {
            Text(stringResource(Res.string.login_create_account))
        }
    }
}

private fun LoginNotice.text(): StringResource =
    when (this) {
        LoginNotice.PasswordChanged -> Res.string.login_password_changed
    }
