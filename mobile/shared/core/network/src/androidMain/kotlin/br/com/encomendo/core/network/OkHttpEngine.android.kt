package br.com.encomendo.core.network

import io.ktor.client.engine.HttpClientEngine
import io.ktor.client.engine.okhttp.OkHttp

/** Returns the engine the Android app passes to [createApiClient]; tests pass a MockEngine instead. */
fun okHttpEngine(): HttpClientEngine = OkHttp.create()
