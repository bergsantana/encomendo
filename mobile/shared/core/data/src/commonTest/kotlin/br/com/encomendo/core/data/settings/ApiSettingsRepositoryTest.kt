package br.com.encomendo.core.data.settings

import br.com.encomendo.core.data.members.storeCalls
import br.com.encomendo.core.data.session.createSessionApiClient
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.scope
import br.com.encomendo.core.domain.settings.StoreSettings
import br.com.encomendo.core.network.ApiClientConfig
import br.com.encomendo.core.network.SessionTurns
import br.com.encomendo.core.network.settings.SettingsApi
import br.com.encomendo.core.security.SessionTokens
import br.com.encomendo.core.security.StoredSession
import br.com.encomendo.core.security.TokenStore
import br.com.encomendo.core.testing.FakeSecretCipher
import br.com.encomendo.core.testing.temporaryPreferencesDataStore
import br.com.encomendo.core.testing.testSession
import io.ktor.client.engine.mock.MockEngine
import io.ktor.client.engine.mock.MockRequestHandler
import io.ktor.client.engine.mock.respond
import io.ktor.http.HttpHeaders
import io.ktor.http.HttpStatusCode
import io.ktor.http.headersOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class ApiSettingsRepositoryTest {
    @Test
    fun readsWhatTheStoreTurnedOn() =
        runTest {
            val settings =
                repository {
                    respond(
                        """
                        {"stockEnabled": true, "productionEnabled": false, "purchaseBeforeDeposit": false,
                         "depositPercent": 50, "assemblyDays": 3, "safetyMarginDays": 2, "whatsappPhone": null,
                         "currency": "BRL", "version": 4}
                        """.trimIndent(),
                        HttpStatusCode.OK,
                        headersOf(HttpHeaders.ContentType, "application/json"),
                    )
                }

            assertEquals(
                StoreSettings(stockEnabled = true, productionEnabled = false, assemblyDays = 3, safetyMarginDays = 2),
                settings.settings(),
            )
        }

    @Test
    fun aRefusalIsAFailure() =
        runTest {
            val settings =
                repository {
                    respond(
                        """{"title":"Recusado","status":403,"code":"forbidden"}""",
                        HttpStatusCode.Forbidden,
                        headersOf(HttpHeaders.ContentType, "application/problem+json"),
                    )
                }

            assertFailsWith<Failure.Forbidden> { settings.settings() }
        }

    private suspend fun TestScope.repository(handler: MockRequestHandler): ApiSettingsRepository {
        val store =
            TokenStore(
                temporaryPreferencesDataStore(backgroundScope),
                FakeSecretCipher(),
                StandardTestDispatcher(testScheduler),
            )
        store.write(StoredSession(SessionTokens("access", "refresh"), testSession()))
        val turns = SessionTurns()
        turns.screensServe(testSession().scope)
        val client =
            createSessionApiClient(MockEngine(handler), ApiClientConfig(baseUrl = "https://api.test"), store, turns)
        return ApiSettingsRepository(SettingsApi(client), storeCalls(store, client, turns))
    }
}
