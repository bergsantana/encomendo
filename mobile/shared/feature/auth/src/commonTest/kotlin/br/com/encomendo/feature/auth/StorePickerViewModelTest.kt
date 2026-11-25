package br.com.encomendo.feature.auth

import app.cash.turbine.test
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.failure.InputIssue
import br.com.encomendo.core.domain.session.CreateStore
import br.com.encomendo.core.domain.session.LogOut
import br.com.encomendo.core.domain.session.Membership
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.Role
import br.com.encomendo.core.domain.session.Session
import br.com.encomendo.core.domain.session.SwitchStore
import br.com.encomendo.core.domain.session.TenantSummary
import br.com.encomendo.core.testing.AltaRpm
import br.com.encomendo.core.testing.FakeAuthRepository
import br.com.encomendo.core.testing.FakeSessionRepository
import br.com.encomendo.core.testing.FakeTenancyRepository
import br.com.encomendo.core.testing.testSession
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import kotlin.test.AfterTest
import kotlin.test.BeforeTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class) // setMain / resetMain / runCurrent / advanceUntilIdle
class StorePickerViewModelTest {
    private val main = StandardTestDispatcher()
    private val motoPecas =
        TenantSummary(id = "0190f3c4-0000-7000-8000-000000000002", name = "Moto Peças", slug = "moto-pecas")

    @BeforeTest
    fun useTestMain() = Dispatchers.setMain(main)

    @AfterTest
    fun restoreMain() = Dispatchers.resetMain()

    @Test
    fun listsEveryStoreOfTheSession() =
        runTest(main) {
            val viewModel = picker(session = twoStores())
            runCurrent()

            assertEquals(
                listOf(
                    StoreChoice(AltaRpm.id, "Alta RPM", "alta-rpm"),
                    StoreChoice(motoPecas.id, "Moto Peças", "moto-pecas"),
                ),
                viewModel.uiState.value.stores,
            )
        }

    @Test
    fun choosingAStoreOpensItAndStaysBusyUntilTheScreenGoes() =
        runTest(main) {
            // The root swaps to that store's screens and clears this ViewModel; until then nothing flickers.
            val opening = CompletableDeferred<Session>()
            val tenancy = FakeTenancyRepository(switchAnswer = { opening.await() })
            val viewModel = picker(session = twoStores(), tenancy = tenancy)

            viewModel.onChoose(motoPecas.id)
            runCurrent()
            assertEquals(motoPecas.id, viewModel.uiState.value.opening)

            viewModel.onChoose(AltaRpm.id) // a second tap while the first opens
            opening.complete(testSession(tenant = motoPecas))
            advanceUntilIdle()

            assertEquals(listOf(motoPecas.id), tenancy.switches)
            assertEquals(motoPecas.id, viewModel.uiState.value.opening)
        }

    @Test
    fun choosingTheStoreAlreadyOpenJustClosesThePicker() =
        runTest(main) {
            // From "Trocar de loja": switching to the same store would change nothing the root sees, and
            // the picker would spin forever. It closes instead, and spends no token.
            val tenancy = FakeTenancyRepository()
            val viewModel = picker(session = twoStores().copy(tenant = AltaRpm), tenancy = tenancy)
            runCurrent()

            viewModel.effects.test {
                viewModel.onChoose(AltaRpm.id)
                assertEquals(PickerEffect.Close, awaitItem())
            }
            assertEquals(emptyList(), tenancy.switches)
            assertNull(viewModel.uiState.value.opening)
        }

    @Test
    fun showsNothingUntilTheSessionIsRead() =
        runTest(main) {
            // Otherwise the first frame says "Crie sua loja" to a member of several stores.
            val viewModel = picker(session = twoStores())

            assertNull(viewModel.uiState.value.stores)
            runCurrent()
            assertEquals(
                2,
                viewModel.uiState.value.stores
                    ?.size,
            )
        }

    @Test
    fun aStoreThatNoLongerHasTheUserSaysSo() =
        runTest(main) {
            val viewModel =
                picker(
                    session = twoStores(),
                    tenancy = FakeTenancyRepository(switchAnswer = { throw Failure.Forbidden() }),
                )

            viewModel.onChoose(motoPecas.id)
            advanceUntilIdle()

            assertEquals(PickerError.NotAMember, viewModel.uiState.value.error)
            assertNull(viewModel.uiState.value.opening)
        }

    @Test
    fun createsAStoreFromTheFormAndStaysBusy() =
        runTest(main) {
            val tenancy = FakeTenancyRepository()
            val viewModel = picker(session = testSession(tenant = null), tenancy = tenancy)

            viewModel.onNameChange("Alta RPM")
            viewModel.onSlugChange("Alta-RPM ")
            runCurrent()
            assertTrue(viewModel.uiState.value.canCreate)
            viewModel.onCreate()
            advanceUntilIdle()

            assertEquals(listOf("Alta RPM" to "alta-rpm"), tenancy.created)
            assertEquals(listOf("0190f3c4-0000-7000-8000-000000000009"), tenancy.switches)
            assertTrue(viewModel.uiState.value.creating)
        }

    @Test
    fun aSecondTapOnCriarLojaIsIgnored() =
        runTest(main) {
            val creating = CompletableDeferred<TenantSummary>()
            val tenancy = FakeTenancyRepository(createAnswer = { _, _ -> creating.await() })
            val viewModel = filledForm(tenancy)

            viewModel.onCreate()
            runCurrent()
            viewModel.onCreate()
            creating.complete(
                TenantSummary(id = "0190f3c4-0000-7000-8000-000000000009", name = "Alta RPM", slug = "alta-rpm"),
            )
            advanceUntilIdle()

            assertEquals(1, tenancy.created.size)
        }

    @Test
    fun typingKeepsARateLimitsWaitButClearsOtherErrors() =
        runTest(main) {
            val tenancy =
                FakeTenancyRepository(createAnswer = {
                    _,
                    _,
                    ->
                    throw Failure.RateLimited(retryAfterSeconds = 30)
                })
            val viewModel = filledForm(tenancy)
            viewModel.onCreate()
            advanceUntilIdle()

            viewModel.onNameChange("Alta RPM Motos")
            runCurrent()
            assertEquals(PickerError.RateLimited(30), viewModel.uiState.value.error, "typing doesn't shorten the wait")

            tenancy.createAnswer = { _, _ -> throw Failure.AlreadyTaken(field = "slug") }
            viewModel.onCreate()
            advanceUntilIdle()
            assertEquals(PickerError.CheckFields, viewModel.uiState.value.error)
            viewModel.onSlugChange("altarpm")
            runCurrent()
            assertNull(viewModel.uiState.value.error)
        }

    @Test
    fun cannotCreateUntilBothFieldsAreFilled() =
        runTest(main) {
            val viewModel = picker(session = testSession(tenant = null))
            runCurrent()
            assertFalse(viewModel.uiState.value.canCreate)

            viewModel.onNameChange("Alta RPM")
            runCurrent()
            assertFalse(viewModel.uiState.value.canCreate)

            viewModel.onSlugChange("  ")
            runCurrent()
            assertFalse(viewModel.uiState.value.canCreate)
        }

    @Test
    fun aTakenLinkIsMarkedOnTheLinkField() =
        runTest(main) {
            val viewModel =
                filledForm(
                    FakeTenancyRepository(createAnswer = {
                        _,
                        _,
                        ->
                        throw Failure.AlreadyTaken(field = "slug")
                    }),
                )

            viewModel.onCreate()
            advanceUntilIdle()

            val failed = viewModel.uiState.value
            assertTrue(failed.slugTaken)
            assertEquals(PickerError.CheckFields, failed.error)
            assertFalse(failed.creating)

            viewModel.onSlugChange("altarpm")
            runCurrent()
            assertFalse(viewModel.uiState.value.slugTaken, "a new link clears it")
        }

    @Test
    fun fieldIssuesShowNextToTheirFields() =
        runTest(main) {
            val issues =
                listOf(
                    InputIssue(field = "name", message = "Informe o nome."),
                    InputIssue(field = "slug", message = "Esse link é reservado."),
                )
            val viewModel =
                filledForm(
                    FakeTenancyRepository(createAnswer = {
                        _,
                        _,
                        ->
                        throw Failure.InvalidInput(issues)
                    }),
                )

            viewModel.onCreate()
            advanceUntilIdle()

            val failed = viewModel.uiState.value
            assertEquals("Informe o nome.", failed.nameIssue)
            assertEquals("Esse link é reservado.", failed.slugIssue)
            assertEquals(PickerError.CheckFields, failed.error)
        }

    @Test
    fun aServerFailureSaysTheServerDidNotAnswer() =
        runTest(main) {
            val viewModel =
                filledForm(
                    FakeTenancyRepository(createAnswer = {
                        _,
                        _,
                        ->
                        throw Failure.Unexpected(status = 503, code = "not_ready")
                    }),
                )

            viewModel.onCreate()
            advanceUntilIdle()

            assertEquals(PickerError.ServerUnavailable, viewModel.uiState.value.error)
        }

    @Test
    fun noSignalSaysSo() =
        runTest(main) {
            val viewModel =
                picker(
                    session = twoStores(),
                    tenancy = FakeTenancyRepository(switchAnswer = { throw Failure.Offline() }),
                )

            viewModel.onChoose(motoPecas.id)
            advanceUntilIdle()

            assertEquals(PickerError.Offline, viewModel.uiState.value.error)
        }

    @Test
    fun logsOutFromThePicker() =
        runTest(main) {
            // A new account without a store has no other way out (the web's picker has Sair too).
            val auth = FakeAuthRepository()
            val viewModel = picker(session = testSession(tenant = null), auth = auth)

            viewModel.onLogOut()
            advanceUntilIdle()

            assertEquals(1, auth.logouts)
            assertTrue(viewModel.uiState.value.loggingOut)
        }

    @Test
    fun aFailedLogoutSaysSo() =
        runTest(main) {
            val auth = FakeAuthRepository(logOutAnswer = { throw Failure.Unexpected(status = 0, code = "storage") })
            val viewModel = picker(session = testSession(tenant = null), auth = auth)

            viewModel.onLogOut()
            advanceUntilIdle()

            assertTrue(viewModel.uiState.value.logOutFailed)
            assertFalse(viewModel.uiState.value.loggingOut)
        }

    @Test
    fun neverPrintsWhatWasTyped() =
        runTest(main) {
            val viewModel = filledForm(FakeTenancyRepository())
            runCurrent()

            val printed = viewModel.uiState.value.toString()
            assertFalse(printed.contains("Alta RPM"), printed)
            assertFalse(printed.contains("alta-rpm"), printed)
        }

    private fun picker(
        session: Session,
        tenancy: FakeTenancyRepository = FakeTenancyRepository(),
        auth: FakeAuthRepository = FakeAuthRepository(),
    ): StorePickerViewModel =
        StorePickerViewModel(
            observeSession = ObserveSession(FakeSessionRepository(session)),
            switchStore = SwitchStore(tenancy),
            createStore = CreateStore(tenancy),
            logOut = LogOut(auth),
        )

    private fun filledForm(tenancy: FakeTenancyRepository): StorePickerViewModel =
        picker(session = testSession(tenant = null), tenancy = tenancy).apply {
            onNameChange("Alta RPM")
            onSlugChange("alta-rpm")
        }

    private fun twoStores(): Session =
        testSession(tenant = null).copy(
            memberships = listOf(Membership(AltaRpm, Role("owner")), Membership(motoPecas, Role("staff"))),
        )
}
