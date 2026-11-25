package br.com.encomendo.core.network

import io.ktor.client.call.NoTransformationFoundException
import io.ktor.client.call.body
import io.ktor.client.statement.HttpResponse
import io.ktor.serialization.ContentConvertException

/**
 * Reads a 2xx body as [T]. An answer that does not match the contract is an
 * [ApiError] with code `unknown`, the one failure type callers handle, rather
 * than a decoding exception no screen expects.
 *
 * Only the decoder's exception name is kept: its message quotes the JSON
 * around the fault, and that JSON may hold tokens or personal data.
 */
internal suspend inline fun <reified T> HttpResponse.decoded(): T =
    try {
        body()
    } catch (error: ContentConvertException) {
        throw undecodable(error)
    } catch (error: NoTransformationFoundException) {
        throw undecodable(error)
    }

internal fun HttpResponse.undecodable(error: Exception): ApiError =
    ApiError(
        status = status.value,
        code = ApiErrorCode.Unknown,
        cause = IllegalStateException("undecodable answer: ${error::class.simpleName}"),
    )
