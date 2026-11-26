package br.com.encomendo.feature.auth

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import br.com.encomendo.core.domain.failure.Failure
import br.com.encomendo.core.domain.session.CreateStore
import br.com.encomendo.core.domain.session.LogOut
import br.com.encomendo.core.domain.session.ObserveSession
import br.com.encomendo.core.domain.session.SwitchStore
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * StorePickerViewModel backs the store picker: choose one of the user's
 * stores, create another and open it, or log out. Once the session moves,
 * the root shows the right screens; choosing the store already open sends
 * [PickerEffect.Close] instead, since nothing would move.
 */
class StorePickerViewModel(
    observeSession: ObserveSession,
    private val switchStore: SwitchStore,
    private val createStore: CreateStore,
    private val logOut: LogOut,
) : ViewModel() {
    private val form = MutableStateFlow(StorePickerUiState())
    private val once = Channel<PickerEffect>(Channel.BUFFERED)

    /** The picker as the screen shows it. */
    val uiState: StateFlow<StorePickerUiState> =
        combine(observeSession(), form) { session, local ->
            local.copy(
                stores =
                    session?.memberships.orEmpty().map {
                        StoreChoice(
                            it.tenant.id,
                            it.tenant.name,
                            it.tenant.slug,
                        )
                    },
                currentStoreId = session?.tenant?.id,
            )
        }.stateIn(viewModelScope, SharingStarted.Eagerly, StorePickerUiState())

    /** What the screen does once, such as closing (mobile/CLAUDE.md, Layers). */
    val effects: Flow<PickerEffect> = once.receiveAsFlow()

    /** The user typed the store's name. */
    fun onNameChange(name: String) =
        form.update { it.copy(name = name, nameIssue = null, error = it.error.afterTyping(it.slugIssue)) }

    /** The user typed the store's link. */
    fun onSlugChange(slug: String) =
        form.update { it.copy(slug = slug, slugIssue = null, error = it.error.afterTyping(it.nameIssue)) }

    /** The user chose the store [tenantId]. Does nothing while something runs. */
    fun onChoose(tenantId: String) {
        if (form.value.busy) return
        if (tenantId == uiState.value.currentStoreId) {
            // The session wouldn't change, so the root would never take the picker away.
            once.trySend(PickerEffect.Close)
            return
        }
        form.update { it.copy(opening = tenantId, error = null) }
        viewModelScope.launch {
            try {
                switchStore(tenantId)
                // Stays busy: the root swaps to that store's screens and clears this ViewModel.
            } catch (failure: Failure) {
                form.update { it.copy(opening = null).showingChoice(failure) }
            }
        }
    }

    /** The user tapped Criar loja. Marks empty fields instead of sending; does nothing while something runs. */
    fun onCreate() {
        val typed = form.value
        if (!typed.canCreate) return
        val nameIssue = FieldProblem.Missing.takeIf { typed.name.isBlank() }
        val slugIssue = FieldProblem.Missing.takeIf { typed.slug.isBlank() }
        if (nameIssue != null || slugIssue != null) {
            form.update { it.copy(nameIssue = nameIssue, slugIssue = slugIssue, error = FormError.CheckFields) }
            return
        }
        form.update { it.copy(creating = true, error = null, nameIssue = null, slugIssue = null) }
        viewModelScope.launch {
            try {
                createStore(name = typed.name, slug = typed.slug)
            } catch (failure: Failure) {
                form.update { it.copy(creating = false).showingCreation(failure) }
            }
        }
    }

    /** The user tapped Sair. */
    fun onLogOut() {
        if (form.value.busy) return
        form.update { it.copy(loggingOut = true, logOutFailed = false) }
        viewModelScope.launch {
            try {
                logOut()
            } catch (_: Failure) {
                form.update { it.copy(loggingOut = false, logOutFailed = true) }
            }
        }
    }
}

/** Choosing a store: only a lost membership has words of its own; the create form stays as it was. */
private fun StorePickerUiState.showingChoice(failure: Failure): StorePickerUiState =
    when (failure) {
        is Failure.Forbidden -> copy(error = FormError.NotAMember)
        // An input issue here is about no field of the form: don't mark one.
        is Failure.InvalidInput, is Failure.AlreadyTaken -> copy(error = FormError.Unexpected)
        else -> copy(error = failure.toFormError())
    }

/** Creating a store: fields the server refused are marked, anything else as every form words it. */
private fun StorePickerUiState.showingCreation(failure: Failure): StorePickerUiState =
    when (failure) {
        is Failure.AlreadyTaken ->
            copy(
                slugIssue =
                    if (failure.field ==
                        SLUG_FIELD
                    ) {
                        FieldProblem.Taken
                    } else {
                        slugIssue
                    },
                error = FormError.CheckFields,
            )
        is Failure.InvalidInput ->
            copy(
                nameIssue = failure.problemFor(NAME_FIELD),
                slugIssue = failure.problemFor(SLUG_FIELD),
                error = FormError.CheckFields,
            )
        else -> copy(error = failure.toFormError())
    }

/** The field names of `CreateTenantRequest` in docs/api/openapi.yaml. */
private const val NAME_FIELD = "name"
private const val SLUG_FIELD = "slug"
