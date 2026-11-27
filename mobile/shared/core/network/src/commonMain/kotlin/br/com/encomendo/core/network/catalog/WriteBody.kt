package br.com.encomendo.core.network.catalog

import io.ktor.client.request.HttpRequestBuilder
import io.ktor.client.request.setBody
import io.ktor.http.ContentType
import io.ktor.http.contentType
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.encodeToJsonElement

/**
 * The JSON of catalog write bodies. Unlike the client's `ApiJson`, it keeps
 * explicit nulls, because the contract requires some fields (`categoryId`,
 * `assemblyDays` …) to be present even when null; and it leaves out a
 * property that holds its default, which is how a cost field the user did not
 * set ([OwnerFieldDto.Absent]) never reaches the server (ADR 0025).
 */
@PublishedApi
internal val WriteJson =
    Json {
        explicitNulls = true
        encodeDefaults = false
    }

/** Sends [value] as the JSON body, encoded with the write rules above. */
internal inline fun <reified T> HttpRequestBuilder.jsonBody(value: T) {
    contentType(ContentType.Application.Json)
    // A tree, so the client's own JSON settings (no explicit nulls) cannot drop a null the contract requires.
    val tree: JsonElement = WriteJson.encodeToJsonElement(value)
    setBody(tree)
}
