package br.com.encomendo.core.domain.session

import br.com.encomendo.core.domain.failure.Failure
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertSame

class CreateStoreTest {
    @Test
    fun createsTheStoreThenOpensIt() =
        runTest {
            val tenancy = RecordingTenancy()

            val session = CreateStore(tenancy)(name = "Alta RPM", slug = "alta-rpm")

            assertEquals(listOf("Alta RPM" to "alta-rpm"), tenancy.created)
            assertEquals(listOf(NEW_STORE.id), tenancy.switches)
            assertSame(tenancy.session, session)
        }

    @Test
    fun tidiesWhatWasTyped() =
        runTest {
            // The link is an address: lower case, no stray spaces from the keyboard.
            val tenancy = RecordingTenancy()

            CreateStore(tenancy)(name = "  Alta RPM ", slug = " Alta-RPM ")

            assertEquals(listOf("Alta RPM" to "alta-rpm"), tenancy.created)
        }

    @Test
    fun aRetryAfterTheSwitchFailedOnlyOpensTheStore() =
        runTest {
            // Creating it again would say its own link is taken (as the web's createStore does).
            val tenancy = RecordingTenancy(switchFailures = 1)
            val createStore = CreateStore(tenancy)
            assertFailsWith<Failure.Offline> { createStore(name = "Alta RPM", slug = "alta-rpm") }

            createStore(name = "Alta RPM", slug = "alta-rpm")

            assertEquals(1, tenancy.created.size)
            assertEquals(listOf(NEW_STORE.id, NEW_STORE.id), tenancy.switches)
        }

    @Test
    fun aRetryWithAnotherLinkCreatesAnotherStore() =
        runTest {
            val tenancy = RecordingTenancy(switchFailures = 1)
            val createStore = CreateStore(tenancy)
            assertFailsWith<Failure.Offline> { createStore(name = "Alta RPM", slug = "alta-rpm") }

            createStore(name = "Alta RPM", slug = "altarpm")

            assertEquals(listOf("Alta RPM" to "alta-rpm", "Alta RPM" to "altarpm"), tenancy.created)
        }

    private class RecordingTenancy(
        private var switchFailures: Int = 0,
    ) : TenancyRepository {
        val created = mutableListOf<Pair<String, String>>()
        val switches = mutableListOf<String>()
        val session =
            Session(
                user =
                    User(
                        id = "0190f3c4-0000-7000-8000-0000000000aa",
                        name = "Ana Souza",
                        email = "ana@altarpm.com.br",
                    ),
                tenant = NEW_STORE,
                role = Role("owner"),
                permissions = GrantedPermissions.None,
                memberships = listOf(Membership(NEW_STORE, Role("owner"))),
            )

        override suspend fun createStore(
            name: String,
            slug: String,
        ): TenantSummary {
            created += name to slug
            return NEW_STORE.copy(slug = slug)
        }

        override suspend fun switchTo(tenantId: String): Session {
            switches += tenantId
            if (switchFailures > 0) {
                switchFailures--
                throw Failure.Offline()
            }
            return session
        }
    }

    private companion object {
        val NEW_STORE = TenantSummary(id = "0190f3c4-0000-7000-8000-000000000009", name = "Alta RPM", slug = "alta-rpm")
    }
}
