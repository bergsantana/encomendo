package br.com.encomendo.core.network.catalog

import br.com.encomendo.core.network.ApiError
import br.com.encomendo.core.network.ApiErrorCode
import io.ktor.client.HttpClient
import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.request.header
import io.ktor.client.request.put
import io.ktor.client.request.setBody
import io.ktor.http.ContentType
import io.ktor.http.HttpHeaders
import io.ktor.http.content.ByteArrayContent
import io.ktor.http.isSuccess
import kotlinx.io.IOException

/**
 * Creates the HTTP client that talks to the photo storage host (ADR 0024).
 * It has no Auth plugin, no default URL and no JSON: the storage host is not
 * the API, so it must never see the session's bearer token, and the signed
 * URL it is given already says who may upload and what.
 */
fun createStorageClient(
    engine: HttpClientEngine,
    requestTimeoutMillis: Long = UPLOAD_TIMEOUT_MILLIS,
): HttpClient =
    HttpClient(engine) {
        // A photo of up to 10 MB on a weak signal takes longer than an API call.
        install(HttpTimeout) { this.requestTimeoutMillis = requestTimeoutMillis }
    }

/**
 * PhotoStorageApi sends a photo's bytes to the signed URL the API issued
 * (`POST /api/v1/admin/catalog/photo-uploads`). Build it over
 * [createStorageClient], never over the API client.
 */
class PhotoStorageApi(
    private val client: HttpClient,
) {
    /**
     * Sends [bytes] with `PUT` to [uploadUrl] carrying exactly [headers] and nothing else of ours: no
     * `Authorization`. `Content-Type` is taken from [headers]. Any answer but 2xx throws [ApiError] with code
     * [ApiErrorCode.UploadFailed], and a request that never got there throws one with [ApiErrorCode.Network].
     */
    suspend fun put(
        uploadUrl: String,
        headers: Map<String, String>,
        bytes: ByteArray,
    ) {
        val contentType =
            headers.entries
                .firstOrNull { it.key.equals(HttpHeaders.ContentType, ignoreCase = true) }
                ?.value
                ?.let(ContentType::parse)
        try {
            val response =
                client.put(uploadUrl) {
                    // Content-Type and Content-Length are the body's to set; the engine refuses them as headers.
                    headers
                        .filterKeys { !it.isBodyHeader() }
                        .forEach { (name, value) -> header(name, value) }
                    setBody(ByteArrayContent(bytes, contentType))
                }
            if (!response.status.isSuccess()) {
                throw ApiError(status = response.status.value, code = ApiErrorCode.UploadFailed)
            }
        } catch (error: IOException) {
            throw ApiError(status = 0, code = ApiErrorCode.Network, cause = error)
        }
    }
}

private fun String.isBodyHeader(): Boolean =
    equals(HttpHeaders.ContentType, ignoreCase = true) || equals(HttpHeaders.ContentLength, ignoreCase = true)

private const val UPLOAD_TIMEOUT_MILLIS = 120_000L
