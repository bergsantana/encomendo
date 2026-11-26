package br.com.encomendo.feature.members

import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.ui.failure.FailureMessage

// How the invite form changes as the user types and the API answers.

/**
 * The form after typing [email]: the email's mark goes, and with it
 * "Confira os campos destacados.", but not a rate limit's wait.
 */
internal fun InviteFormUi.typed(email: String): InviteFormUi =
    copy(
        email = email,
        emailIssue = null,
        error = error.takeIf { it is TeamMessage.General && it.message is FailureMessage.RateLimited },
    )

/**
 * The form once an invitation to [sentTo] went out: empty for the next one,
 * keeping the role, as on the web. An email changed while it was sent stays.
 */
internal fun InviteFormUi.sent(sentTo: String): InviteFormUi =
    copy(email = if (email == sentTo) "" else email, emailIssue = null, error = null)

/** The form after the API refused an invitation: the email marked where it was the reason. */
internal fun InviteFormUi.refused(failure: Failure): InviteFormUi =
    refusedFor(failure).let { if (it.emailIssue != null) it.focusingEmail() else it }

/** The form with the email marked by the caller, the keyboard sent to it once more. */
internal fun InviteFormUi.focusingEmail(): InviteFormUi = copy(emailFocusRequests = emailFocusRequests + 1)

private fun InviteFormUi.refusedFor(failure: Failure): InviteFormUi =
    when (failure) {
        is Failure.AlreadyMember -> copy(emailIssue = EmailIssue.AlreadyOnTeam, error = TeamMessage.CheckFields)
        is Failure.InvalidInput -> {
            val issue = failure.issues.firstOrNull { it.field == "email" }?.let { EmailIssue.Server(it.message) }
            copy(emailIssue = issue, error = TeamMessage.CheckFields)
        }
        else -> copy(error = failure.toTeamMessage())
    }
