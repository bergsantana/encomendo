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
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.feature.auth.resources.Res
import br.com.encomendo.feature.auth.resources.login_missing_email
import br.com.encomendo.feature.auth.resources.signup_email_taken
import br.com.encomendo.feature.auth.resources.signup_have_account
import br.com.encomendo.feature.auth.resources.signup_missing_name
import br.com.encomendo.feature.auth.resources.signup_missing_password
import br.com.encomendo.feature.auth.resources.signup_submitting
import br.com.encomendo.feature.auth.resources.signup_title
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel

/** SignUpRoute connects [SignUpViewModel] to [SignUpScreen]; [onHaveAccount] goes back to the login. */
@Composable
fun SignUpRoute(
    onHaveAccount: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: SignUpViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    val actions =
        remember(viewModel, onHaveAccount) {
            SignUpActions(
                onNameChange = viewModel::onNameChange,
                onEmailChange = viewModel::onEmailChange,
                onPasswordChange = viewModel::onPasswordChange,
                onPasswordVisibilityToggle = viewModel::onPasswordVisibilityToggle,
                onSubmit = viewModel::onSubmit,
                onHaveAccount = onHaveAccount,
            )
        }
    SignUpScreen(state = state, actions = actions, modifier = modifier)
}

/** SignUpActions are what the user can do on [SignUpScreen]. */
data class SignUpActions(
    val onNameChange: (String) -> Unit,
    val onEmailChange: (String) -> Unit,
    val onPasswordChange: (String) -> Unit,
    val onPasswordVisibilityToggle: () -> Unit,
    val onSubmit: () -> Unit,
    val onHaveAccount: () -> Unit,
)

/**
 * SignUpScreen creates a staff account ("Criar conta"): name, email and
 * password, in the design document's words. The new account has no store,
 * so the app then offers to create one. Stateless.
 */
@Composable
fun SignUpScreen(
    state: SignUpUiState,
    actions: SignUpActions,
    modifier: Modifier = Modifier,
) {
    val enabled = !state.submitting
    Column(
        modifier = modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        FormTitle(stringResource(Res.string.signup_title))
        FormBanner(state.error)
        NameField(
            FieldUi(state.name, state.nameIssue?.text(missing = Res.string.signup_missing_name), enabled),
            actions.onNameChange,
        )
        EmailField(
            FieldUi(
                state.email,
                state.emailIssue?.text(missing = Res.string.login_missing_email, taken = Res.string.signup_email_taken),
                enabled,
            ),
            actions.onEmailChange,
        )
        PasswordField(
            field =
                FieldUi(
                    state.password,
                    state.passwordIssue?.text(missing = Res.string.signup_missing_password),
                    enabled,
                ),
            visible = state.passwordVisible,
            new = true,
            actions = PasswordActions(actions.onPasswordChange, actions.onPasswordVisibilityToggle, actions.onSubmit),
        )
        BusyButton(
            label = stringResource(Res.string.signup_title),
            busyLabel = stringResource(Res.string.signup_submitting),
            busy = state.submitting,
            enabled = state.canSubmit,
            onClick = actions.onSubmit,
        )
        TextButton(onClick = actions.onHaveAccount, enabled = enabled, modifier = Modifier.heightIn(min = 56.dp)) {
            Text(stringResource(Res.string.signup_have_account))
        }
    }
}
