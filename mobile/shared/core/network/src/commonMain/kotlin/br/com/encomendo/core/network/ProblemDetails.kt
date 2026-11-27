package br.com.encomendo.core.network

import io.ktor.client.statement.HttpResponse
import io.ktor.client.statement.bodyAsText
import io.ktor.http.HttpHeaders
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/** The fields of an RFC 9457 problem+json body that the app reads (docs/api/openapi.yaml, `Problem`). */
@Serializable
internal data class ProblemBody(
    val title: String? = null,
    val code: String? = null,
    val detail: String? = null,
    val errors: List<FieldIssueBody> = emptyList(),
    val references: List<ReferenceBody> = emptyList(),
)

@Serializable
internal data class ReferenceBody(
    val id: String,
    val name: String,
)

@Serializable
internal data class FieldIssueBody(
    val field: String,
    val message: String,
)

/**
 * Reads a failed response as an [ApiError]. A body that is not a problem (a
 * proxy's HTML page, an empty 502) still gives an error, with code
 * [ApiErrorCode.Unknown] and no title.
 */
internal suspend fun HttpResponse.toApiError(json: Json): ApiError {
    val problem = decodeProblem(json, bodyAsText())
    return ApiError(
        status = status.value,
        code = problem?.code?.let(::ApiErrorCode) ?: ApiErrorCode.Unknown,
        title = problem?.title,
        detail = problem?.detail,
        fieldIssues = problem?.errors.orEmpty().map { FieldIssue(field = it.field, message = it.message) },
        references = problem?.references.orEmpty().map { ErrorReference(id = it.id, name = it.name) },
        retryAfterSeconds = headers[HttpHeaders.RetryAfter]?.let(::wholeSeconds),
    )
}

/**
 * Reads `Retry-After` as the contract sends it: whole seconds, at least 1.
 * HTTP also allows a date there; the app shows no number rather than a wrong one.
 */
private fun wholeSeconds(header: String): Int? = header.trim().toIntOrNull()?.takeIf { it >= 1 }

private fun decodeProblem(
    json: Json,
    body: String,
): ProblemBody? =
    try {
        json.decodeFromString<ProblemBody>(body)
    } catch (_: IllegalArgumentException) {
        // SerializationException is an IllegalArgumentException: the body was not a problem.
        null
    }
