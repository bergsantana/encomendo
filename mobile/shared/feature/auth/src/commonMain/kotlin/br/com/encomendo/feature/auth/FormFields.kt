package br.com.encomendo.feature.auth

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
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
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import br.com.encomendo.feature.auth.resources.Res
import br.com.encomendo.feature.auth.resources.login_email
import br.com.encomendo.feature.auth.resources.login_password
import br.com.encomendo.feature.auth.resources.login_password_hide
import br.com.encomendo.feature.auth.resources.login_password_hide_description
import br.com.encomendo.feature.auth.resources.login_password_show
import br.com.encomendo.feature.auth.resources.login_password_show_description
import br.com.encomendo.feature.auth.resources.password_hint
import br.com.encomendo.feature.auth.resources.signup_name
import org.jetbrains.compose.resources.StringResource
import org.jetbrains.compose.resources.stringResource

/**
 * FieldUi is what one form field shows: what was typed, the text of its issue
 * if any, and whether it can be edited. Not a data class, so what was typed
 * never reaches a log through toString.
 */
internal class FieldUi(
    val value: String,
    val issue: String?,
    val enabled: Boolean,
)

/** PasswordActions are what the user can do on a password field. */
internal data class PasswordActions(
    val onChange: (String) -> Unit,
    val onVisibilityToggle: () -> Unit,
    val onDone: () -> Unit,
)

/** A screen's title, read as a heading. */
@Composable
internal fun FormTitle(text: String) {
    Text(text = text, style = MaterialTheme.typography.headlineSmall, modifier = Modifier.semantics { heading() })
}

/** The form error above the fields, announced when it appears, as the design document places it. */
@Composable
internal fun FormBanner(error: FormError?) {
    error ?: return
    Text(
        text = error.message(),
        color = MaterialTheme.colorScheme.error,
        style = MaterialTheme.typography.bodyLarge,
        modifier = Modifier.announced(),
    )
}

/** Announces the text when it appears or changes, for text that shows after a tap. */
internal fun Modifier.announced(): Modifier = semantics { liveRegion = LiveRegionMode.Polite }

/** The person's name, for a new account ("Nome"). */
@Composable
internal fun NameField(
    field: FieldUi,
    onChange: (String) -> Unit,
) {
    val focus = LocalFocusManager.current
    OutlinedTextField(
        value = field.value,
        onValueChange = onChange,
        label = { Text(stringResource(Res.string.signup_name)) },
        isError = field.issue != null,
        supportingText = field.issue?.let { issue -> { Issue(issue) } },
        singleLine = true,
        enabled = field.enabled,
        keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Words, imeAction = ImeAction.Next),
        keyboardActions = KeyboardActions(onNext = { focus.moveFocus(FocusDirection.Down) }),
        modifier = Modifier.fillMaxWidth().semantics { contentType = ContentType.PersonFullName },
    )
}

/**
 * The account's email ("E-mail"), offered the device's saved logins. With
 * [onDone] the keyboard's action sends the form, for a form with no field
 * after this one; without it, the action moves to the next field.
 */
@Composable
internal fun EmailField(
    field: FieldUi,
    onChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    onDone: (() -> Unit)? = null,
) {
    val focus = LocalFocusManager.current
    OutlinedTextField(
        value = field.value,
        onValueChange = onChange,
        label = { Text(stringResource(Res.string.login_email)) },
        isError = field.issue != null,
        supportingText = field.issue?.let { issue -> { Issue(issue) } },
        singleLine = true,
        enabled = field.enabled,
        keyboardOptions =
            KeyboardOptions(
                keyboardType = KeyboardType.Email,
                imeAction = if (onDone == null) ImeAction.Next else ImeAction.Done,
                autoCorrectEnabled = false,
            ),
        keyboardActions =
            KeyboardActions(onNext = { focus.moveFocus(FocusDirection.Down) }, onDone = { onDone?.invoke() }),
        modifier =
            modifier.fillMaxWidth().semantics {
                contentType = ContentType.Username + ContentType.EmailAddress
            },
    )
}

/**
 * The password ("Senha", or [label]), with Mostrar/Ocultar: gloves make typos. A
 * [new] password carries the rule as its hint ("Pelo menos 8 caracteres.")
 * and is offered to the device's password manager to save.
 */
@Composable
internal fun PasswordField(
    field: FieldUi,
    visible: Boolean,
    new: Boolean,
    actions: PasswordActions,
    label: StringResource = Res.string.login_password,
) {
    val hint = if (new) stringResource(Res.string.password_hint) else null
    OutlinedTextField(
        value = field.value,
        onValueChange = actions.onChange,
        label = { Text(stringResource(label)) },
        isError = field.issue != null,
        // The rule stays under a new password even when it was refused, as on the web: that is when it helps.
        supportingText =
            if (field.issue == null && hint == null) {
                null
            } else {
                {
                    Column {
                        field.issue?.let { Issue(it) }
                        hint?.let { Text(it) }
                    }
                }
            },
        trailingIcon = {
            VisibilityToggle(
                visible = visible,
                enabled = field.enabled,
                onToggle = actions.onVisibilityToggle,
            )
        },
        singleLine = true,
        enabled = field.enabled,
        visualTransformation = if (visible) VisualTransformation.None else PasswordVisualTransformation(),
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done),
        keyboardActions = KeyboardActions(onDone = { actions.onDone() }),
        modifier =
            Modifier.fillMaxWidth().semantics {
                contentType = if (new) ContentType.NewPassword else ContentType.Password
            },
    )
}

/** A field's issue, announced when it appears: the field lost focus while the form was sent. */
@Composable
internal fun Issue(text: String) {
    Text(text = text, modifier = Modifier.announced())
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
