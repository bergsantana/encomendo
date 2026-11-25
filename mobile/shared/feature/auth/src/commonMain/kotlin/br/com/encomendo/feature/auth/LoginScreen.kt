package br.com.encomendo.feature.auth

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.autofill.ContentType
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.contentType
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import br.com.encomendo.feature.auth.resources.Res
import br.com.encomendo.feature.auth.resources.login_email
import br.com.encomendo.feature.auth.resources.login_error_offline
import br.com.encomendo.feature.auth.resources.login_error_rate_limited_minutes
import br.com.encomendo.feature.auth.resources.login_error_rate_limited_seconds
import br.com.encomendo.feature.auth.resources.login_error_server
import br.com.encomendo.feature.auth.resources.login_error_unexpected
import br.com.encomendo.feature.auth.resources.login_error_wrong_credentials
import br.com.encomendo.feature.auth.resources.login_password
import br.com.encomendo.feature.auth.resources.login_password_hide
import br.com.encomendo.feature.auth.resources.login_password_hide_description
import br.com.encomendo.feature.auth.resources.login_password_show
import br.com.encomendo.feature.auth.resources.login_password_show_description
import br.com.encomendo.feature.auth.resources.login_submit
import br.com.encomendo.feature.auth.resources.login_submitting
import br.com.encomendo.feature.auth.resources.login_title
import org.jetbrains.compose.resources.pluralStringResource
import org.jetbrains.compose.resources.stringResource
import org.koin.compose.viewmodel.koinViewModel

/** LoginRoute connects [LoginViewModel] to [LoginScreen]. */
@Composable
fun LoginRoute(
    modifier: Modifier = Modifier,
    viewModel: LoginViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    // Remembered, so the screen can skip recomposing when only the state changes.
    val actions =
        remember(viewModel) {
            LoginActions(
                onEmailChange = viewModel::onEmailChange,
                onPasswordChange = viewModel::onPasswordChange,
                onPasswordVisibilityToggle = viewModel::onPasswordVisibilityToggle,
                onSubmit = viewModel::onSubmit,
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
)

/**
 * LoginScreen is where a signed-out business user starts: email, password and
 * Entrar. It is stateless; the fields offer the device's saved passwords.
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
        Text(
            text = stringResource(Res.string.login_title),
            style = MaterialTheme.typography.headlineSmall,
            modifier = Modifier.semantics { heading() },
        )
        EmailField(state = state, onChange = actions.onEmailChange)
        PasswordField(state = state, actions = actions)
        state.error?.let { error ->
            Text(
                text = error.message(),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyLarge,
                modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
            )
        }
        SubmitButton(submitting = state.submitting, enabled = state.canSubmit, onClick = actions.onSubmit)
    }
}

@Composable
private fun EmailField(
    state: LoginUiState,
    onChange: (String) -> Unit,
) {
    val focus = LocalFocusManager.current
    OutlinedTextField(
        value = state.email,
        onValueChange = onChange,
        label = { Text(stringResource(Res.string.login_email)) },
        isError = state.emailIssue != null,
        supportingText = state.emailIssue?.let { issue -> { Issue(issue) } },
        singleLine = true,
        enabled = !state.submitting,
        keyboardOptions =
            KeyboardOptions(
                keyboardType = KeyboardType.Email,
                imeAction = ImeAction.Next,
                autoCorrectEnabled = false,
            ),
        keyboardActions = KeyboardActions(onNext = { focus.moveFocus(FocusDirection.Down) }),
        modifier =
            Modifier.fillMaxWidth().semantics {
                contentType = ContentType.Username + ContentType.EmailAddress
            },
    )
}

@Composable
private fun PasswordField(
    state: LoginUiState,
    actions: LoginActions,
) {
    OutlinedTextField(
        value = state.password,
        onValueChange = actions.onPasswordChange,
        label = { Text(stringResource(Res.string.login_password)) },
        isError = state.passwordIssue != null,
        supportingText = state.passwordIssue?.let { issue -> { Issue(issue) } },
        trailingIcon = {
            VisibilityToggle(
                visible = state.passwordVisible,
                enabled = !state.submitting,
                onToggle = actions.onPasswordVisibilityToggle,
            )
        },
        singleLine = true,
        enabled = !state.submitting,
        visualTransformation = if (state.passwordVisible) VisualTransformation.None else PasswordVisualTransformation(),
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done),
        keyboardActions = KeyboardActions(onDone = { actions.onSubmit() }),
        modifier = Modifier.fillMaxWidth().semantics { contentType = ContentType.Password },
    )
}

/** A field's issue, announced when it appears: the field lost focus while the login ran. */
@Composable
private fun Issue(text: String) {
    Text(text = text, modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite })
}

@Composable
private fun VisibilityToggle(
    visible: Boolean,
    enabled: Boolean,
    onToggle: () -> Unit,
) {
    val description =
        stringResource(
            if (visible) Res.string.login_password_hide_description else Res.string.login_password_show_description,
        )
    TextButton(
        onClick = onToggle,
        enabled = enabled,
        modifier = Modifier.semantics { contentDescription = description },
    ) {
        Text(stringResource(if (visible) Res.string.login_password_hide else Res.string.login_password_show))
    }
}

@Composable
private fun SubmitButton(
    submitting: Boolean,
    enabled: Boolean,
    onClick: () -> Unit,
) {
    val submittingLabel = stringResource(Res.string.login_submitting)
    Button(
        onClick = onClick,
        enabled = enabled,
        modifier =
            Modifier
                .fillMaxWidth()
                .heightIn(min = 56.dp)
                .semantics { if (submitting) contentDescription = submittingLabel },
    ) {
        if (submitting) {
            CircularProgressIndicator(modifier = Modifier.size(24.dp), strokeWidth = 2.dp)
        } else {
            Text(stringResource(Res.string.login_submit))
        }
    }
}

@Composable
private fun LoginError.message(): String =
    when (this) {
        LoginError.WrongCredentials -> stringResource(Res.string.login_error_wrong_credentials)
        LoginError.Offline -> stringResource(Res.string.login_error_offline)
        LoginError.ServerUnavailable -> stringResource(Res.string.login_error_server)
        LoginError.Unexpected -> stringResource(Res.string.login_error_unexpected)
        is LoginError.RateLimited -> rateLimitMessage(retryAfterSeconds)
    }

@Composable
private fun rateLimitMessage(retryAfterSeconds: Int?): String {
    // Every 429 carries Retry-After (ADR 0013); a minute is a safe guess if one ever does not, as on web.
    val seconds = retryAfterSeconds ?: SECONDS_PER_MINUTE
    val minutes = minutesToWait(seconds)
    return if (minutes == null) {
        pluralStringResource(Res.plurals.login_error_rate_limited_seconds, seconds, seconds)
    } else {
        pluralStringResource(Res.plurals.login_error_rate_limited_minutes, minutes, minutes)
    }
}

/**
 * Returns the wait in whole minutes, rounded up, from a minute on; null below
 * that, where seconds read better. The web admin uses the same thresholds.
 * Rounding up never tells the user to come back before the limit resets.
 */
internal fun minutesToWait(seconds: Int): Int? =
    if (seconds < MINUTES_FROM_SECONDS) null else (seconds + SECONDS_PER_MINUTE - 1) / SECONDS_PER_MINUTE

private const val MINUTES_FROM_SECONDS = 60
private const val SECONDS_PER_MINUTE = 60
