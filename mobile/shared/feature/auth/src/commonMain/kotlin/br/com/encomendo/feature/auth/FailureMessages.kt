package br.com.encomendo.feature.auth

import androidx.compose.runtime.Composable
import br.com.encomendo.core.ui.failure.text
import br.com.encomendo.feature.auth.resources.Res
import br.com.encomendo.feature.auth.resources.form_check_fields
import br.com.encomendo.feature.auth.resources.login_error_wrong_credentials
import br.com.encomendo.feature.auth.resources.picker_error_not_member
import org.jetbrains.compose.resources.StringResource
import org.jetbrains.compose.resources.stringResource

/** The text above a form for [this] error, in the design document's words. */
@Composable
internal fun FormError.message(): String =
    when (this) {
        FormError.CheckFields -> stringResource(Res.string.form_check_fields)
        FormError.WrongCredentials -> stringResource(Res.string.login_error_wrong_credentials)
        FormError.NotAMember -> stringResource(Res.string.picker_error_not_member)
        is FormError.General -> message.text()
    }

/**
 * The text under a field for [this] problem: [missing] when it was left
 * empty, [taken] when it is someone else's, [wrong] when it was refused for a
 * reason the screen has words for, the server's words otherwise.
 */
@Composable
internal fun FieldProblem.text(
    missing: StringResource,
    taken: StringResource? = null,
    wrong: StringResource? = null,
): String =
    when (this) {
        FieldProblem.Missing -> stringResource(missing)
        FieldProblem.Taken -> stringResource(requireNotNull(taken) { "a field that can be taken needs its words" })
        FieldProblem.Wrong -> stringResource(requireNotNull(wrong) { "a field that can be wrong needs its words" })
        is FieldProblem.Server -> message
    }
